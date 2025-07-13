package bot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"YoutubeDownloader/internal/downloader"
	"YoutubeDownloader/internal/payment"
	"YoutubeDownloader/internal/storage"
	"YoutubeDownloader/internal/utils"

	"crypto/md5"

	tele "gopkg.in/telebot.v4"
)

// Обработчик текстовых сообщений
func (b *Bot) handleMessage(c tele.Context) error {
	msg := c.Message()
	log.Printf("[EVENT] handleMessage: user_id=%d, text=%q", msg.Sender.ID, msg.Text)
	if msg.Text == "/start" {
		return c.Send("👋 Добро пожаловать!\n\nЭтот бот позволяет скачивать видео с разных сайтов за Telegram Stars. Просто отправьте ссылку на видео!")
	}

	// Тестовая команда для проверки инвойсов
	if msg.Text == "/test_invoice" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendTestInvoice(c)
	}

	// Тестовая команда для проверки PreCheckoutQuery
	if msg.Text == "/test_precheckout" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return c.Send("Отправьте тестовый инвойс и попробуйте оплатить его для проверки PreCheckoutQuery")
	}

	// Команда для проверки настроек бота
	if msg.Text == "/bot_info" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendBotInfo(c)
	}

	// Тестовая команда для отправки инвойса без PreCheckoutQuery
	if msg.Text == "/test_direct" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendDirectInvoice(c)
	}

	// Команда для проверки настроек API
	if msg.Text == "/api_info" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendAPIInfo(c)
	}

	// Команды для управления кэшем
	if msg.Text == "/cache_stats" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendCacheStats(c)
	}
	if strings.HasPrefix(msg.Text, "/cache_clean ") && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			return c.Send("Укажите количество дней после /cache_clean")
		}
		daysStr := strings.TrimSpace(parts[1])
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return c.Send("Количество дней должно быть числом")
		}
		return b.cleanOldCache(c, days)
	}
	if msg.Text == "/cache_clear" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.clearAllCache(c)
	}
	if msg.Text == "/active_downloads" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendActiveDownloads(c)
	}

	// --- Блок для админа ---
	if msg.Text == "/admin" && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		return b.sendAdminTransactionsMenu(c)
	}
	if strings.HasPrefix(msg.Text, "/refund ") && b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			return c.Send("Укажите charge_id после /refund")
		}
		chargeID := strings.TrimSpace(parts[1])
		var userID int64 = 0
		if len(parts) >= 3 {
			// Пробуем распарсить user_id
			parsed, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return c.Send("user_id должен быть числом")
			}
			userID = parsed
		}
		return b.handleAdminRefundWithUserID(c, chargeID, userID)
	}
	// --- Конец блока для админа ---

	// Универсальная регулярка для любой ссылки
	urlRegex := regexp.MustCompile(`https?://\S+`)
	url := urlRegex.FindString(msg.Text)
	if url == "" {
		return c.Send("Не обнаружено ссылки. Пожалуйста, пришлите ссылку на видео.")
	}

	if b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
		go b.sendVideo(c, url, "", 0)
		return nil
	}
	if b.channelUsername != "" {
		isSub, err := b.CheckUserSubscriptionRaw(b.channelUsername, msg.Sender.ID)
		if err == nil && isSub {
			go b.sendVideo(c, url, "", 0)
			return nil
		}
	}
	return b.sendUniversalPayKeyboard(c, url)
}

// Обработчик callback-запросов
func (b *Bot) handleCallback(c tele.Context) error {
	cb := c.Callback()
	log.Printf("[EVENT] handleCallback: user_id=%d, data=%q", cb.Sender.ID, cb.Data)
	data := cb.Data
	if data == "pay_subscribe" {
		return b.sendSubscribeInvoice(c, "month")
	}
	if data == "pay_subscribe_year" {
		return b.sendSubscribeInvoice(c, "year")
	}
	if data == "pay_subscribe_forever" {
		return b.sendSubscribeInvoice(c, "forever")
	}
	if strings.HasPrefix(data, "pay_video|") {
		idStr := strings.TrimPrefix(data, "pay_video|")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Send("Ошибка: некорректный id транзакции.")
		}
		trx, err := payment.GetTransactionByID(b.db, id)
		if err != nil {
			return c.Send("Ошибка: не удалось найти транзакцию.")
		}
		return b.sendVideoInvoiceByDB(c, trx)
	}
	if strings.HasPrefix(data, "pay_tiktok|") {
		url := strings.TrimPrefix(data, "pay_tiktok|")
		return b.sendTikTokInvoice(c, url)
	}
	// --- Обработка возврата для админа ---
	if strings.HasPrefix(data, "admin_refund|") && b.adminID != "" && b.adminID == toStr(c.Sender().ID) {
		chargeID := strings.TrimPrefix(data, "admin_refund|")
		return b.handleAdminRefund(c, chargeID)
	}
	// --- Конец блока ---
	return nil
}

// Обработчик платежей
func (b *Bot) handlePayment(c tele.Context) error {
	log.Printf("[DEBUG] Вызван handlePayment")

	// Пробуем получить платеж разными способами
	var paymentInfo *tele.Payment

	// Сначала пробуем через c.Payment()
	paymentInfo = c.Payment()
	if paymentInfo == nil {
		// Если не получилось, пробуем через Message
		update := c.Update()
		if update.Message != nil && update.Message.Payment != nil {
			paymentInfo = update.Message.Payment
			log.Printf("[DEBUG] Платеж найден в Message")
		}
	}

	log.Printf("[EVENT] handlePayment: user_id=%d, paymentInfo=%+v", c.Sender().ID, paymentInfo)

	if paymentInfo == nil {
		log.Printf("[DEBUG] paymentInfo == nil, событие не обработано")
		return c.Send("Ошибка: информация об оплате не получена")
	}

	userID := c.Sender().ID
	payload := paymentInfo.Payload
	amount := paymentInfo.Total
	chargeID := paymentInfo.ProviderChargeID

	log.Printf("[PAYMENT] Получена оплата: user_id=%d, payload=%s, amount=%d, charge_id=%s", userID, payload, amount, chargeID)

	// Логируем все транзакции до обновления
	trxs2, err2 := payment.GetAllTransactionsFromDB(b.db)
	if err2 == nil {
		log.Printf("[DEBUG] Все транзакции до обновления: %+v", trxs2)
	} else {
		log.Printf("[DEBUG] Ошибка получения всех транзакций: %v", err2)
	}

	var id int64
	var url string
	if strings.HasPrefix(payload, "video|") {
		idStr := strings.TrimPrefix(payload, "video|")
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			log.Printf("[PAYMENT] Ошибка парсинга id: %v", err)
			return c.Send("Ошибка: некорректный идентификатор транзакции.")
		}
		log.Printf("[PAYMENT] Обновляем транзакцию: id=%d, charge_id=%s", id, chargeID)
		err = payment.UpdateTransactionAfterPayment(b.db, id, chargeID, "success")
		if err != nil {
			log.Printf("[DB] Ошибка обновления транзакции: %v", err)
		}
		trx, err := payment.GetTransactionByID(b.db, id)
		if err != nil {
			log.Printf("[PAYMENT] Ошибка поиска транзакции после оплаты: %v", err)
			// Логируем все транзакции после ошибки
			trxs3, err3 := payment.GetAllTransactionsFromDB(b.db)
			if err3 == nil {
				log.Printf("[DEBUG] Все транзакции после ошибки поиска: %+v", trxs3)
			}
			return c.Send("Ошибка: не удалось найти транзакцию после оплаты.")
		}
		url = trx.URL
		log.Printf("[PAYMENT] Запускаем скачивание: user_id=%d, trx_id=%d, url=%s", userID, id, url)
	}
	if strings.HasPrefix(payload, "tiktok|") {
		url = strings.TrimPrefix(payload, "tiktok|")
	}

	// Сохраняем транзакцию в БД
	var err error
	trx := &payment.Transaction{
		TelegramPaymentChargeID: chargeID,
		TelegramUserID:          userID,
		Amount:                  amount,
		InvoicePayload:          payload,
		Status:                  "success",
		Type:                    "stars",
		Reason:                  "",
		URL:                     url,
	}
	_, err = payment.InsertTransaction(b.db, trx)
	if err != nil {
		log.Printf("[DB] Ошибка сохранения транзакции: %v", err)
	}

	if strings.HasPrefix(payload, "video|") {
		go b.sendVideo(c, url, chargeID, amount)
		return c.Send("Оплата прошла успешно! Скачивание началось.")
	}
	if strings.HasPrefix(payload, "tiktok|") {
		go b.sendTikTokVideo(c, url, chargeID, amount)
		return c.Send("Оплата прошла успешно! Скачивание TikTok началось.")
	}
	if strings.HasPrefix(payload, "subscribe|") {
		period := strings.TrimPrefix(payload, "subscribe|")
		// TODO: записать подписку в БД
		return c.Send("Подписка активирована: " + period)
	}

	if strings.HasPrefix(payload, "test|") {
		log.Printf("[PAYMENT] Получен тестовый платеж: %s", payload)
		return c.Send("Тестовый платеж обработан успешно!")
	}

	if strings.HasPrefix(payload, "test_direct|") {
		log.Printf("[PAYMENT] Получен тестовый платеж без PreCheckoutQuery: %s", payload)
		return c.Send("Тестовый платеж без PreCheckoutQuery обработан успешно!")
	}

	log.Printf("[PAYMENT] Неизвестный тип payload: %s", payload)
	return c.Send("Оплата прошла успешно!")
}

// Универсальная клавиатура оплаты
func (b *Bot) sendUniversalPayKeyboard(c tele.Context, url string) error {
	// Создаём транзакцию в БД со статусом 'pending' и получаем id
	id, err := payment.CreatePendingTransaction(b.db, c.Sender().ID, 1, url)
	if err != nil {
		log.Printf("[DB] Ошибка создания pending транзакции: %v", err)
		return c.Send("Ошибка при подготовке оплаты. Попробуйте позже.")
	}
	// Логируем все транзакции после создания
	trxs1, err1 := payment.GetAllTransactionsFromDB(b.db)
	if err1 == nil {
		log.Printf("[DEBUG] Все транзакции после создания pending: %+v", trxs1)
	} else {
		log.Printf("[DEBUG] Ошибка получения всех транзакций: %v", err1)
	}
	btns := [][]tele.InlineButton{
		{{Text: "Скачать за 1 звезду", Data: fmt.Sprintf("pay_video|%d", id)}},
		{{Text: "Подписка на месяц за 30 звёзд", Data: "pay_subscribe"}},
		{{Text: "Подписка на год за 200 звёзд", Data: "pay_subscribe_year"}},
		{{Text: "Навсегда за 1000 звёзд", Data: "pay_subscribe_forever"}},
	}
	if b.channelUsername != "" {
		btns = append(btns, []tele.InlineButton{{Text: "Подписаться на канал", URL: "https://t.me/" + strings.TrimPrefix(b.channelUsername, "@")}})
	}
	msgText := "Выберите способ оплаты:"
	if b.channelUsername != "" {
		msgText = fmt.Sprintf("Подписчики канала %s могут использовать бота бесплатно!\n\n%s", b.channelUsername, msgText)
	}
	markup := &tele.ReplyMarkup{InlineKeyboard: btns}
	return c.Send(msgText, markup)
}

// Новая функция для отправки инвойса по данным из БД
func (b *Bot) sendVideoInvoiceByDB(c tele.Context, trx *payment.Transaction) error {
	log.Printf("[INVOICE] Создаём инвойс для user_id=%d, trx_id=%d, url=%s", trx.TelegramUserID, trx.ID, trx.URL)

	invoice := &tele.Invoice{
		Title:       "Скачать видео",
		Description: "Скачивание видео за 1 звезду",
		Payload:     fmt.Sprintf("video|%d", trx.ID),
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Видео", Amount: 1}},
	}
	log.Printf("[INVOICE] Отправляем инвойс: %+v", invoice)
	log.Printf("[INVOICE] Отправляем пользователю: %+v", c.Sender())

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[INVOICE] Ошибка отправки инвойса: %v", err)
	} else {
		log.Printf("[INVOICE] Инвойс отправлен успешно")
	}
	return err
}

func (b *Bot) sendTikTokInvoice(c tele.Context, url string) error {
	invoice := &tele.Invoice{
		Title:       "Скачать TikTok видео",
		Description: "Скачивание TikTok видео за 1 звезду",
		Payload:     "tiktok|" + url,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "TikTok", Amount: 1}},
	}
	log.Printf("[INVOICE] Отправляем TikTok инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[INVOICE] Ошибка отправки TikTok инвойса: %v", err)
	} else {
		log.Printf("[INVOICE] TikTok инвойс отправлен успешно")
	}
	return err
}

func (b *Bot) sendSubscribeInvoice(c tele.Context, period string) error {
	var price int
	var label, desc string
	switch period {
	case "month":
		price = 30
		label = "Подписка на месяц"
		desc = "Доступ ко всем загрузкам на 1 месяц"
	case "year":
		price = 200
		label = "Подписка на год"
		desc = "Доступ ко всем загрузкам на 1 год"
	case "forever":
		price = 1000
		label = "Навсегда"
		desc = "Пожизненный доступ ко всем загрузкам"
	default:
		return c.Send("Неизвестный тип подписки")
	}
	invoice := &tele.Invoice{
		Title:       label,
		Description: desc,
		Payload:     "subscribe|" + period,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: label, Amount: price}},
	}
	log.Printf("[INVOICE] Отправляем инвойс подписки: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[INVOICE] Ошибка отправки инвойса подписки: %v", err)
	} else {
		log.Printf("[INVOICE] Инвойс подписки отправлен успешно")
	}
	return err
}

// Функция с повтором отправки видео
func (b *Bot) sendVideoWithRetry(c tele.Context, video *tele.Video, url string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := c.Send(video)
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "Client.Timeout exceeded") {
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
		}
		lastErr = err
		break
	}
	return lastErr
}

func (b *Bot) sendVideo(c tele.Context, url string, chargeID string, amount int) {
	userID := c.Sender().ID
	requestID := fmt.Sprintf("req_%d_%s", userID, utils.RandomString(6))
	log.Printf("[VIDEO] [%s] Начинаем скачивание: user_id=%d, url=%s, charge_id=%s, amount=%d", requestID, userID, url, chargeID, amount)

	// Сначала проверяем кэш
	cache, err := storage.GetVideoFromCache(b.db, url)
	if err != nil {
		log.Printf("[VIDEO] [%s] Ошибка проверки кэша: %v", requestID, err)
	} else if cache != nil {
		log.Printf("[VIDEO] [%s] Найдено в кэше: file_id=%s", requestID, cache.TelegramFileID)

		// Отправляем видео из кэша
		video := &tele.Video{File: tele.File{FileID: cache.TelegramFileID}, Caption: "Ваше видео! (из кэша)"}
		err = b.sendVideoWithRetry(c, video, url, 10)
		if err != nil {
			log.Printf("[VIDEO] [%s] Ошибка отправки из кэша: %v", requestID, err)
			// Если отправка из кэша не удалась, удаляем запись из кэша и скачиваем заново
			storage.DeleteVideoFromCache(b.db, url)
			log.Printf("[VIDEO] [%s] Удалена запись из кэша, скачиваем заново", requestID)
		} else {
			log.Printf("[VIDEO] [%s] Успешно отправлено из кэша", requestID)
			return
		}
	}

	// Проверяем, активно ли скачивание этого URL
	if b.isDownloadActive(url) {
		log.Printf("[VIDEO] [%s] Обнаружено активное скачивание для URL: %s, ожидаем завершения", requestID, url)
		c.Send("Это видео уже скачивается другим пользователем. Ожидаю завершения...")

		// Ждем завершения скачивания (максимум 10 минут)
		downloadInfo, err := b.waitForDownload(url, 10*time.Minute)
		if err != nil {
			log.Printf("[VIDEO] [%s] Таймаут ожидания скачивания: %v", requestID, err)
			b.sendError(c, "Превышено время ожидания скачивания. Попробуйте позже.", err, "[TIMEOUT] "+url)
			return
		}

		// Проверяем, была ли ошибка при скачивании
		if downloadInfo.Error != nil {
			log.Printf("[VIDEO] [%s] Скачивание завершилось с ошибкой: %v", requestID, downloadInfo.Error)
			b.sendError(c, "Скачивание не удалось. Попробуйте позже.", downloadInfo.Error, "[DOWNLOAD_ERROR] "+url)
			return
		}

		// Проверяем кэш еще раз после завершения скачивания
		cache, err = storage.GetVideoFromCache(b.db, url)
		if err != nil {
			log.Printf("[VIDEO] [%s] Ошибка проверки кэша после ожидания: %v", requestID, err)
		} else if cache != nil {
			log.Printf("[VIDEO] [%s] Найдено в кэше после ожидания: file_id=%s", requestID, cache.TelegramFileID)

			// Отправляем видео из кэша
			video := &tele.Video{File: tele.File{FileID: cache.TelegramFileID}, Caption: "Ваше видео! (из кэша после ожидания)"}
			err = b.sendVideoWithRetry(c, video, url, 10)
			if err != nil {
				log.Printf("[VIDEO] [%s] Ошибка отправки из кэша после ожидания: %v", requestID, err)
				b.sendError(c, "Ошибка отправки видео. Попробуйте позже.", err, "[SEND_CACHE_ERROR] "+url)
			} else {
				log.Printf("[VIDEO] [%s] Успешно отправлено из кэша после ожидания", requestID)
			}
			return
		} else {
			log.Printf("[VIDEO] [%s] Видео не найдено в кэше после ожидания, начинаем скачивание", requestID)
		}
	}

	// Получаем мьютекс для этого URL
	urlMutex := b.getURLMutex(url)
	urlMutex.Lock()
	defer func() {
		urlMutex.Unlock()
		// Очищаем мьютекс через некоторое время
		go func() {
			time.Sleep(30 * time.Second)
			b.cleanupURLMutex(url)
		}()
	}()

	log.Printf("[VIDEO] [%s] Получена блокировка для URL: %s", requestID, url)

	// Регистрируем начало скачивания
	_ = b.startDownload(url, requestID, userID)
	defer func() {
		// Регистрируем завершение скачивания
		b.finishDownload(url, nil)
	}()

	// Убеждаемся, что папка tmp существует
	if err := os.MkdirAll("./tmp", 0755); err != nil {
		log.Printf("[VIDEO] [%s] Ошибка создания папки tmp: %v", requestID, err)
		b.sendError(c, "Ошибка подготовки к скачиванию.", err, "[TMP_DIR] "+url)
		if chargeID != "" && amount > 0 {
			log.Printf("[VIDEO] [%s] Возврат средств: charge_id=%s, user_id=%d, amount=%d", requestID, chargeID, userID, amount)
			payment.RefundStarPayment(userID, chargeID, amount, "Ошибка создания временной папки")
		}
		return
	}

	c.Send("Скачиваю видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		log.Printf("[VIDEO] [%s] Получен слот для скачивания", requestID)
		defer func() {
			<-b.downloadLimiter
			log.Printf("[VIDEO] [%s] Освобожден слот для скачивания", requestID)
		}()

		// Создаем уникальное имя файла с URL хешем для дополнительной изоляции
		urlHash := fmt.Sprintf("%x", md5.Sum([]byte(url)))[:8]
		filename, err := downloader.DownloadYouTubeVideoWithUserIDAndURL(url, userID, requestID, urlHash)
		if err != nil {
			log.Printf("[VIDEO] [%s] Ошибка скачивания: %v", requestID, err)
			b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[DL] "+url)
			if chargeID != "" && amount > 0 {
				log.Printf("[VIDEO] [%s] Возврат средств: charge_id=%s, user_id=%d, amount=%d", requestID, chargeID, userID, amount)
				payment.RefundStarPayment(userID, chargeID, amount, "Ошибка скачивания видео")
			}
			// Регистрируем ошибку скачивания
			b.finishDownload(url, err)
			return
		}

		// Проверяем, что файл действительно принадлежит этому пользователю и URL
		expectedPrefix := fmt.Sprintf("ytvideo_user%d_%s_%s", userID, requestID, urlHash)
		if !strings.Contains(filepath.Base(filename), expectedPrefix) {
			log.Printf("[VIDEO] [%s] КРИТИЧЕСКАЯ ОШИБКА: файл %s не принадлежит пользователю %d или URL %s", requestID, filename, userID, url)
			b.sendError(c, "Произошла критическая ошибка. Попробуйте позже.", fmt.Errorf("файл не принадлежит пользователю или URL"), "[FILE_OWNERSHIP] "+url)
			if chargeID != "" && amount > 0 {
				log.Printf("[VIDEO] [%s] Возврат средств: charge_id=%s, user_id=%d, amount=%d", requestID, chargeID, userID, amount)
				payment.RefundStarPayment(userID, chargeID, amount, "Ошибка принадлежности файла")
			}
			// Регистрируем ошибку скачивания
			b.finishDownload(url, fmt.Errorf("файл не принадлежит пользователю или URL"))
			return
		}

		video := &tele.Video{File: tele.FromDisk(filename), Caption: "Ваше видео!"}
		log.Printf("[VIDEO] [%s] Отправляем файл: %s", requestID, filename)
		err = b.sendVideoWithRetry(c, video, url, 10)
		if err != nil {
			log.Printf("[VIDEO] [%s] Ошибка отправки: %v", requestID, err)
			if info, statErr := os.Stat(filename); statErr == nil {
				sizeMB := float64(info.Size()) / 1024.0 / 1024.0
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[SEND_VIDEO] "+url, fmt.Sprintf("Размер файла: %.2f МБ", sizeMB))
			} else {
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[SEND_VIDEO] "+url)
			}
			if chargeID != "" && amount > 0 {
				log.Printf("[VIDEO] [%s] Возврат средств: charge_id=%s, user_id=%d, amount=%d", requestID, chargeID, userID, amount)
				payment.RefundStarPayment(userID, chargeID, amount, "Ошибка отправки видео")
			}
			// Регистрируем ошибку скачивания
			b.finishDownload(url, err)
			return
		}

		// Сохраняем file_id в кэш
		if video.File.FileID != "" {
			if err := storage.SaveVideoToCache(b.db, url, video.File.FileID); err != nil {
				log.Printf("[VIDEO] [%s] Ошибка сохранения в кэш: %v", requestID, err)
			} else {
				log.Printf("[VIDEO] [%s] Сохранено в кэш: file_id=%s", requestID, video.File.FileID)
			}
		}

		os.Remove(filename)
		log.Printf("[VIDEO] [%s] Успешно завершено скачивание для URL: %s", requestID, url)
	default:
		c.Send("Сейчас много загрузок. Пожалуйста, подождите и попробуйте чуть позже.")
	}
}

func (b *Bot) sendTikTokVideo(c tele.Context, url string, chargeID string, amount int) {
	userID := c.Sender().ID
	requestID := fmt.Sprintf("tiktok_%d_%s", userID, utils.RandomString(6))
	log.Printf("[TIKTOK] [%s] Начинаем скачивание: user_id=%d, url=%s, charge_id=%s, amount=%d", requestID, userID, url, chargeID, amount)

	// Сначала проверяем кэш
	cache, err := storage.GetVideoFromCache(b.db, url)
	if err != nil {
		log.Printf("[TIKTOK] [%s] Ошибка проверки кэша: %v", requestID, err)
	} else if cache != nil {
		log.Printf("[TIKTOK] [%s] Найдено в кэше: file_id=%s", requestID, cache.TelegramFileID)

		// Отправляем видео из кэша
		video := &tele.Video{File: tele.File{FileID: cache.TelegramFileID}, Caption: "Ваше TikTok видео! (из кэша)"}
		err = b.sendVideoWithRetry(c, video, url, 10)
		if err != nil {
			log.Printf("[TIKTOK] [%s] Ошибка отправки из кэша: %v", requestID, err)
			// Если отправка из кэша не удалась, удаляем запись из кэша и скачиваем заново
			storage.DeleteVideoFromCache(b.db, url)
			log.Printf("[TIKTOK] [%s] Удалена запись из кэша, скачиваем заново", requestID)
		} else {
			log.Printf("[TIKTOK] [%s] Успешно отправлено из кэша", requestID)
			return
		}
	}

	// Проверяем, активно ли скачивание этого URL
	if b.isDownloadActive(url) {
		log.Printf("[TIKTOK] [%s] Обнаружено активное скачивание для URL: %s, ожидаем завершения", requestID, url)
		c.Send("Это TikTok видео уже скачивается другим пользователем. Ожидаю завершения...")

		// Ждем завершения скачивания (максимум 10 минут)
		downloadInfo, err := b.waitForDownload(url, 10*time.Minute)
		if err != nil {
			log.Printf("[TIKTOK] [%s] Таймаут ожидания скачивания: %v", requestID, err)
			b.sendError(c, "Превышено время ожидания скачивания. Попробуйте позже.", err, "[TIMEOUT_TIKTOK] "+url)
			return
		}

		// Проверяем, была ли ошибка при скачивании
		if downloadInfo.Error != nil {
			log.Printf("[TIKTOK] [%s] Скачивание завершилось с ошибкой: %v", requestID, downloadInfo.Error)
			b.sendError(c, "Скачивание не удалось. Попробуйте позже.", downloadInfo.Error, "[DOWNLOAD_ERROR_TIKTOK] "+url)
			return
		}

		// Проверяем кэш еще раз после завершения скачивания
		cache, err = storage.GetVideoFromCache(b.db, url)
		if err != nil {
			log.Printf("[TIKTOK] [%s] Ошибка проверки кэша после ожидания: %v", requestID, err)
		} else if cache != nil {
			log.Printf("[TIKTOK] [%s] Найдено в кэше после ожидания: file_id=%s", requestID, cache.TelegramFileID)

			// Отправляем видео из кэша
			video := &tele.Video{File: tele.File{FileID: cache.TelegramFileID}, Caption: "Ваше TikTok видео! (из кэша после ожидания)"}
			err = b.sendVideoWithRetry(c, video, url, 10)
			if err != nil {
				log.Printf("[TIKTOK] [%s] Ошибка отправки из кэша после ожидания: %v", requestID, err)
				b.sendError(c, "Ошибка отправки TikTok видео. Попробуйте позже.", err, "[SEND_CACHE_ERROR_TIKTOK] "+url)
			} else {
				log.Printf("[TIKTOK] [%s] Успешно отправлено из кэша после ожидания", requestID)
			}
			return
		} else {
			log.Printf("[TIKTOK] [%s] TikTok видео не найдено в кэше после ожидания, начинаем скачивание", requestID)
		}
	}

	// Получаем мьютекс для этого URL
	urlMutex := b.getURLMutex(url)
	urlMutex.Lock()
	defer func() {
		urlMutex.Unlock()
		// Очищаем мьютекс через некоторое время
		go func() {
			time.Sleep(30 * time.Second)
			b.cleanupURLMutex(url)
		}()
	}()

	log.Printf("[TIKTOK] [%s] Получена блокировка для URL: %s", requestID, url)

	// Регистрируем начало скачивания
	_ = b.startDownload(url, requestID, userID)
	defer func() {
		// Регистрируем завершение скачивания
		b.finishDownload(url, nil)
	}()

	// Убеждаемся, что папка tmp существует
	if err := os.MkdirAll("./tmp", 0755); err != nil {
		log.Printf("[TIKTOK] [%s] Ошибка создания папки tmp: %v", requestID, err)
		b.sendError(c, "Ошибка подготовки к скачиванию TikTok.", err, "[TMP_DIR_TIKTOK] "+url)
		if chargeID != "" && amount > 0 {
			log.Printf("[TIKTOK] [%s] Возврат средств: charge_id=%s, user_id=%d, amount=%d", requestID, chargeID, userID, amount)
			payment.RefundStarPayment(userID, chargeID, amount, "Ошибка создания временной папки для TikTok")
		}
		return
	}

	c.Send("Скачиваю TikTok видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		log.Printf("[TIKTOK] [%s] Получен слот для скачивания", requestID)
		defer func() {
			<-b.downloadLimiter
			log.Printf("[TIKTOK] [%s] Освобожден слот для скачивания", requestID)
		}()

		// Создаем уникальное имя файла с URL хешем для дополнительной изоляции
		urlHash := fmt.Sprintf("%x", md5.Sum([]byte(url)))[:8]
		filename, err := downloader.DownloadTikTokVideoWithUserIDAndURL(url, userID, requestID, urlHash)
		if err != nil {
			b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[TikTok] "+url)
			payment.RefundStarPayment(userID, chargeID, amount, "Ошибка скачивания TikTok видео")
			// Регистрируем ошибку скачивания
			b.finishDownload(url, err)
			return
		}

		// Проверяем, что файл действительно принадлежит этому пользователю и URL
		expectedPrefix := fmt.Sprintf("tiktok_user%d_%s_%s", userID, requestID, urlHash)
		if !strings.Contains(filepath.Base(filename), expectedPrefix) {
			log.Printf("[TIKTOK] [%s] КРИТИЧЕСКАЯ ОШИБКА: файл %s не принадлежит пользователю %d или URL %s", requestID, filename, userID, url)
			b.sendError(c, "Произошла критическая ошибка. Попробуйте позже.", fmt.Errorf("файл не принадлежит пользователю или URL"), "[FILE_OWNERSHIP_TIKTOK] "+url)
			if chargeID != "" && amount > 0 {
				log.Printf("[TIKTOK] [%s] Возврат средств: charge_id=%s, user_id=%d, amount=%d", requestID, chargeID, userID, amount)
				payment.RefundStarPayment(userID, chargeID, amount, "Ошибка принадлежности TikTok файла")
			}
			// Регистрируем ошибку скачивания
			b.finishDownload(url, fmt.Errorf("файл не принадлежит пользователю или URL"))
			return
		}

		video := &tele.Video{File: tele.FromDisk(filename), Caption: "Ваше TikTok видео!"}
		err = b.sendVideoWithRetry(c, video, url, 10)
		if err != nil {
			if info, statErr := os.Stat(filename); statErr == nil {
				sizeMB := float64(info.Size()) / 1024.0 / 1024.0
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[SEND_TIKTOK] "+url, fmt.Sprintf("Размер файла: %.2f МБ", sizeMB))
			} else {
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[SEND_TIKTOK] "+url)
			}
			payment.RefundStarPayment(userID, chargeID, amount, "Ошибка отправки TikTok видео")
			// Регистрируем ошибку скачивания
			b.finishDownload(url, err)
			return
		}

		// Сохраняем file_id в кэш
		if video.File.FileID != "" {
			if err := storage.SaveVideoToCache(b.db, url, video.File.FileID); err != nil {
				log.Printf("[TIKTOK] [%s] Ошибка сохранения в кэш: %v", requestID, err)
			} else {
				log.Printf("[TIKTOK] [%s] Сохранено в кэш: file_id=%s", requestID, video.File.FileID)
			}
		}

		os.Remove(filename)
		log.Printf("[TIKTOK] [%s] Успешно завершено скачивание для URL: %s", requestID, url)
	default:
		c.Send("Сейчас много загрузок. Пожалуйста, подождите и попробуйте чуть позже.")
	}
}

func (b *Bot) CheckUserSubscriptionRaw(channelUsername string, userID int64) (bool, error) {
	api := b.api
	chat, err := api.ChatByUsername(channelUsername)
	if err != nil {
		log.Printf("[SUB_CHECK] Ошибка поиска канала: %v", err)
		return false, err
	}
	member, err := api.ChatMemberOf(chat, &tele.User{ID: userID})
	if err != nil {
		log.Printf("[SUB_CHECK] Ошибка получения статуса: %v", err)
		return false, err
	}
	log.Printf("[SUB_CHECK] Статус пользователя: %s", member.Role)
	if member.Role == tele.Member || member.Role == tele.Administrator || member.Role == tele.Creator {
		log.Printf("[SUB_CHECK] Пользователь подписан на канал")
		return true, nil
	}
	log.Printf("[SUB_CHECK] Пользователь НЕ подписан на канал")
	return false, nil
}

func (b *Bot) sendError(c tele.Context, userMsg string, err error, extraInfo ...string) {
	// Пользователю — только общий текст
	_ = c.Send(userMsg)
	// Админу — подробности
	if b.adminID != "" {
		adminID, errParse := strconv.ParseInt(b.adminID, 10, 64)
		if errParse == nil {
			details := []string{"[ERROR] " + err.Error()}
			if len(extraInfo) > 0 {
				details = append(details, extraInfo...)
			}
			msg := strings.Join(details, "\n")
			_, _ = b.api.Send(&tele.User{ID: adminID}, msg)
		}
	}
}

func toStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
