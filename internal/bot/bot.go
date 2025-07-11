package bot

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"YoutubeDownloader/internal/downloader"
	"YoutubeDownloader/internal/payment"

	tele "gopkg.in/telebot.v4"
)

type Bot struct {
	api                *tele.Bot
	adminID            string
	providerToken      string
	transactionService *payment.TransactionService
	channelUsername    string
	downloadLimiter    chan struct{}
	db                 *sql.DB
}

func NewBot(token, adminID, providerToken string, db *sql.DB) (*Bot, error) {
	// Для Telegram Stars provider token не нужен
	log.Printf("[BOT] Инициализация бота для Telegram Stars")

	// Проверяем, нужно ли использовать официальный API вместо локального
	useOfficialAPI := os.Getenv("USE_OFFICIAL_API") == "true"
	if useOfficialAPI {
		log.Printf("[BOT] Используем ОФИЦИАЛЬНЫЙ Telegram Bot API")
	} else {
		log.Printf("[BOT] Используем ЛОКАЛЬНЫЙ Telegram Bot API (aiogram/telegram-bot-api)")
	}

	channelUsername := os.Getenv("CHANNEL_USERNAME")
	maxWorkers := 3
	if mwStr := os.Getenv("MAX_DOWNLOAD_WORKERS"); mwStr != "" {
		if mw, err := strconv.Atoi(mwStr); err == nil && mw > 0 {
			maxWorkers = mw
		}
	}
	settings := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 60 * time.Second},
	}
	// Увеличиваем таймаут HTTP-клиента для отправки больших файлов
	settings.Client = &http.Client{
		Timeout: 120 * time.Second,
	}

	// Настройка URL для API
	if useOfficialAPI {
		// Используем официальный API Telegram
		settings.URL = "https://api.telegram.org"
		log.Printf("[BOT] URL API: %s", settings.URL)
	} else {
		// Используем локальный API
		if url := os.Getenv("TELEGRAM_API_URL"); url != "" {
			settings.URL = url
			log.Printf("[BOT] URL API: %s", settings.URL)
		} else {
			log.Printf("[BOT] Используем дефолтный локальный API URL")
		}
	}

	api, err := tele.NewBot(settings)
	if err != nil {
		return nil, err
	}

	log.Printf("[BOT] Бот успешно инициализирован")

	return &Bot{
		api:                api,
		adminID:            adminID,
		providerToken:      "", // Для Telegram Stars не нужен
		transactionService: payment.NewTransactionService(),
		channelUsername:    channelUsername,
		downloadLimiter:    make(chan struct{}, maxWorkers),
		db:                 db,
	}, nil
}

func (b *Bot) Run() {
	// Middleware для логирования всех апдейтов
	b.api.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			update := c.Update()

			// Логируем только основные типы апдейтов
			if update.Message != nil {
				log.Printf("[UPDATE] Message: user_id=%d, text=%q", update.Message.Sender.ID, update.Message.Text)

				// Проверяем, есть ли платежная информация в сообщении
				if update.Message.Payment != nil {
					log.Printf("[UPDATE] Найден платеж в Message: %+v", update.Message.Payment)
					// Обрабатываем платеж прямо здесь
					return b.handlePayment(c)
				}
			}
			if update.Callback != nil {
				log.Printf("[UPDATE] CallbackQuery: user_id=%d, data=%q", update.Callback.Sender.ID, update.Callback.Data)
			}
			if update.PreCheckoutQuery != nil {
				log.Printf("[UPDATE] PreCheckoutQuery: user_id=%d", update.PreCheckoutQuery.Sender.ID)
				// Автоматически подтверждаем PreCheckoutQuery
				err := c.Accept()
				if err != nil {
					log.Printf("[PRECHECKOUT] Ошибка подтверждения: %v", err)
				} else {
					log.Printf("[PRECHECKOUT] PreCheckoutQuery подтвержден для user_id=%d", update.PreCheckoutQuery.Sender.ID)
				}
				return nil // Не передаем дальше, так как уже обработали
			}

			return next(c)
		}
	})

	b.api.Handle(tele.OnText, b.handleMessage)
	b.api.Handle(tele.OnCallback, b.handleCallback)
	b.api.Handle(tele.OnPayment, b.handlePayment)

	// Обработчик для ВСЕХ остальных типов апдейтов
	b.api.Handle(tele.OnForward, b.handleAnyUpdate)
	b.api.Handle(tele.OnReply, b.handleAnyUpdate)
	b.api.Handle(tele.OnEdited, b.handleAnyUpdate)
	b.api.Handle(tele.OnPhoto, b.handleAnyUpdate)
	b.api.Handle(tele.OnAudio, b.handleAnyUpdate)
	b.api.Handle(tele.OnAnimation, b.handleAnyUpdate)
	b.api.Handle(tele.OnDocument, b.handleAnyUpdate)
	b.api.Handle(tele.OnSticker, b.handleAnyUpdate)
	b.api.Handle(tele.OnVideo, b.handleAnyUpdate)
	b.api.Handle(tele.OnVoice, b.handleAnyUpdate)
	b.api.Handle(tele.OnVideoNote, b.handleAnyUpdate)
	b.api.Handle(tele.OnContact, b.handleAnyUpdate)
	b.api.Handle(tele.OnLocation, b.handleAnyUpdate)
	b.api.Handle(tele.OnVenue, b.handleAnyUpdate)
	b.api.Handle(tele.OnDice, b.handleAnyUpdate)
	b.api.Handle(tele.OnInvoice, b.handleAnyUpdate)
	b.api.Handle(tele.OnRefund, b.handleAnyUpdate)
	b.api.Handle(tele.OnGame, b.handleAnyUpdate)
	b.api.Handle(tele.OnPoll, b.handleAnyUpdate)
	b.api.Handle(tele.OnPollAnswer, b.handleAnyUpdate)
	b.api.Handle(tele.OnPinned, b.handleAnyUpdate)
	b.api.Handle(tele.OnChannelPost, b.handleAnyUpdate)
	b.api.Handle(tele.OnEditedChannelPost, b.handleAnyUpdate)
	b.api.Handle(tele.OnTopicCreated, b.handleAnyUpdate)
	b.api.Handle(tele.OnTopicReopened, b.handleAnyUpdate)
	b.api.Handle(tele.OnTopicClosed, b.handleAnyUpdate)
	b.api.Handle(tele.OnTopicEdited, b.handleAnyUpdate)
	b.api.Handle(tele.OnGeneralTopicHidden, b.handleAnyUpdate)
	b.api.Handle(tele.OnGeneralTopicUnhidden, b.handleAnyUpdate)
	b.api.Handle(tele.OnWriteAccessAllowed, b.handleAnyUpdate)
	b.api.Handle(tele.OnAddedToGroup, b.handleAnyUpdate)
	b.api.Handle(tele.OnUserJoined, b.handleAnyUpdate)
	b.api.Handle(tele.OnUserLeft, b.handleAnyUpdate)
	b.api.Handle(tele.OnUserShared, b.handleAnyUpdate)
	b.api.Handle(tele.OnChatShared, b.handleAnyUpdate)
	b.api.Handle(tele.OnNewGroupTitle, b.handleAnyUpdate)
	b.api.Handle(tele.OnNewGroupPhoto, b.handleAnyUpdate)
	b.api.Handle(tele.OnGroupPhotoDeleted, b.handleAnyUpdate)
	b.api.Handle(tele.OnGroupCreated, b.handleAnyUpdate)
	b.api.Handle(tele.OnSuperGroupCreated, b.handleAnyUpdate)
	b.api.Handle(tele.OnChannelCreated, b.handleAnyUpdate)
	b.api.Handle(tele.OnMigration, b.handleAnyUpdate)
	b.api.Handle(tele.OnMedia, b.handleAnyUpdate)
	b.api.Handle(tele.OnQuery, b.handleAnyUpdate)
	b.api.Handle(tele.OnInlineResult, b.handleAnyUpdate)
	b.api.Handle(tele.OnShipping, b.handleAnyUpdate)
	b.api.Handle(tele.OnCheckout, b.handleAnyUpdate)
	b.api.Handle(tele.OnMyChatMember, b.handleAnyUpdate)
	b.api.Handle(tele.OnChatMember, b.handleAnyUpdate)
	b.api.Handle(tele.OnChatJoinRequest, b.handleAnyUpdate)
	b.api.Handle(tele.OnProximityAlert, b.handleAnyUpdate)
	b.api.Handle(tele.OnAutoDeleteTimer, b.handleAnyUpdate)
	b.api.Handle(tele.OnWebApp, b.handleAnyUpdate)
	b.api.Handle(tele.OnVideoChatStarted, b.handleAnyUpdate)
	b.api.Handle(tele.OnVideoChatEnded, b.handleAnyUpdate)
	b.api.Handle(tele.OnVideoChatParticipants, b.handleAnyUpdate)
	b.api.Handle(tele.OnVideoChatScheduled, b.handleAnyUpdate)
	b.api.Handle(tele.OnBoost, b.handleAnyUpdate)
	b.api.Handle(tele.OnBoostRemoved, b.handleAnyUpdate)
	b.api.Handle(tele.OnBusinessConnection, b.handleAnyUpdate)
	b.api.Handle(tele.OnBusinessMessage, b.handleAnyUpdate)
	b.api.Handle(tele.OnEditedBusinessMessage, b.handleAnyUpdate)
	b.api.Handle(tele.OnDeletedBusinessMessages, b.handleAnyUpdate)

	b.api.Start()
}

// Обработчик для PreCheckoutQuery
func (b *Bot) handlePreCheckoutQuery(c tele.Context) error {
	preCheckout := c.PreCheckoutQuery()

	if preCheckout == nil {
		return nil
	}

	log.Printf("[PRECHECKOUT] Получен PreCheckoutQuery: user_id=%d", preCheckout.Sender.ID)

	// Автоматически подтверждаем все PreCheckoutQuery
	err := c.Accept()
	if err != nil {
		log.Printf("[PRECHECKOUT] Ошибка подтверждения: %v", err)
		return err
	} else {
		log.Printf("[PRECHECKOUT] PreCheckoutQuery подтвержден для user_id=%d", preCheckout.Sender.ID)
	}

	return nil
}

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

func (b *Bot) handleCallback(c tele.Context) error {
	cb := c.Callback()
	log.Printf("[EVENT] handleCallback: user_id=%d, data=%q", cb.Sender.ID, cb.Data)
	data := cb.Data
	// chatID := c.Sender().ID // удалено как неиспользуемое
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
	log.Printf("[VIDEO] Начинаем скачивание: url=%s, charge_id=%s, amount=%d", url, chargeID, amount)
	c.Send("Скачиваю видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		defer func() { <-b.downloadLimiter }()
		filename, err := downloader.DownloadYouTubeVideo(url)
		if err != nil {
			log.Printf("[VIDEO] Ошибка скачивания: %v", err)
			b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[DL] "+url)
			if chargeID != "" && amount > 0 {
				log.Printf("[VIDEO] Возврат средств: charge_id=%s, user_id=%d, amount=%d", chargeID, c.Sender().ID, amount)
				payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка скачивания видео")
			}
			return
		}
		video := &tele.Video{File: tele.FromDisk(filename), Caption: "Ваше видео!"}
		err = b.sendVideoWithRetry(c, video, url, 10)
		if err != nil {
			log.Printf("[VIDEO] Ошибка отправки: %v", err)
			if info, statErr := os.Stat(filename); statErr == nil {
				sizeMB := float64(info.Size()) / 1024.0 / 1024.0
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[SEND_VIDEO] "+url, fmt.Sprintf("Размер файла: %.2f МБ", sizeMB))
			} else {
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[SEND_VIDEO] "+url)
			}
			if chargeID != "" && amount > 0 {
				log.Printf("[VIDEO] Возврат средств: charge_id=%s, user_id=%d, amount=%d", chargeID, c.Sender().ID, amount)
				payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка отправки видео")
			}
			return
		}
		os.Remove(filename)
	default:
		c.Send("Сейчас много загрузок. Пожалуйста, подождите и попробуйте чуть позже.")
	}
}

func (b *Bot) sendTikTokVideo(c tele.Context, url string, chargeID string, amount int) {
	c.Send("Скачиваю TikTok видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		defer func() { <-b.downloadLimiter }()
		filename, err := downloader.DownloadTikTokVideo(url)
		if err != nil {
			b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[TikTok] "+url)
			payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка скачивания TikTok видео")
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
			payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка отправки TikTok видео")
			return
		}
		os.Remove(filename)
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

func (b *Bot) sendAdminTransactionsMenu(c tele.Context) error {
	transactions := b.transactionService.GetAllTransactions()
	if len(transactions) == 0 {
		return c.Send("Транзакций нет.")
	}
	var btns [][]tele.InlineButton
	for _, trx := range transactions {
		// Показываем только успешные и не возвращённые
		if trx.Status == "success" {
			caption := fmt.Sprintf("%s | %d XTR | %d", trx.InvoicePayload, trx.Amount, trx.TelegramUserID)
			btns = append(btns, []tele.InlineButton{{
				Text: caption,
				Data: "admin_refund|" + trx.TelegramPaymentChargeID,
			}})
		}
	}
	if len(btns) == 0 {
		return c.Send("Нет транзакций для возврата.")
	}
	markup := &tele.ReplyMarkup{InlineKeyboard: btns}
	return c.Send("Транзакции (нажмите для возврата):", markup)
}

func (b *Bot) handleAdminRefund(c tele.Context, chargeID string) error {
	trxs := b.transactionService.GetAllTransactions()
	for _, trx := range trxs {
		if trx.TelegramPaymentChargeID == chargeID {
			// Делаем возврат всегда, независимо от статуса
			err := payment.RefundStarPayment(trx.TelegramUserID, trx.TelegramPaymentChargeID, trx.Amount, "Возврат по запросу админа")
			if err != nil {
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND] "+chargeID)
				return nil
			}
			b.transactionService.MarkRefunded(chargeID)
			return c.Send("Возврат выполнен для транзакции: " + chargeID)
		}
	}
	// Если не нашли транзакцию — пробуем сделать возврат с пустыми amount и userID
	err := payment.RefundStarPayment(0, chargeID, 0, "Возврат по запросу админа (id не найден)")
	if err != nil {
		b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND] "+chargeID)
		return nil
	}
	return c.Send("Попытка возврата выполнена для транзакции: " + chargeID)
}

// Новый обработчик возврата с возможностью указать user_id вручную
func (b *Bot) handleAdminRefundWithUserID(c tele.Context, chargeID string, userID int64) error {
	trxs := b.transactionService.GetAllTransactions()
	for _, trx := range trxs {
		if trx.TelegramPaymentChargeID == chargeID {
			if userID == 0 {
				userID = trx.TelegramUserID
			}
			err := payment.RefundStarPayment(userID, trx.TelegramPaymentChargeID, trx.Amount, "Возврат по запросу админа")
			if err != nil {
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND_USERID] "+chargeID)
				return nil
			}
			b.transactionService.MarkRefunded(chargeID)
			return c.Send("Возврат выполнен для транзакции: " + chargeID)
		}
	}
	if userID == 0 {
		return c.Send("Транзакция не найдена в памяти бота и user_id не указан — возврат невозможен.")
	}
	err := payment.RefundStarPayment(userID, chargeID, 0, "Возврат по запросу админа (user_id указан вручную)")
	if err != nil {
		b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND_USERID] "+chargeID)
		return nil
	}
	return c.Send("Попытка возврата выполнена для транзакции: " + chargeID + " с user_id: " + strconv.FormatInt(userID, 10))
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

// Тестовая функция для отправки инвойса
func (b *Bot) sendTestInvoice(c tele.Context) error {
	log.Printf("[TEST] Отправляем тестовый инвойс")

	invoice := &tele.Invoice{
		Title:       "Тестовый инвойс",
		Description: "Тестирование платежной системы",
		Payload:     "test|123",
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Тест", Amount: 1}},
	}

	log.Printf("[TEST] Тестовый инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[TEST] Ошибка отправки тестового инвойса: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки тестового инвойса: %v", err))
	} else {
		log.Printf("[TEST] Тестовый инвойс отправлен успешно")
	}
	return c.Send("Тестовый инвойс отправлен успешно!")
}

// Тестовая функция для отправки инвойса без PreCheckoutQuery
func (b *Bot) sendDirectInvoice(c tele.Context) error {
	log.Printf("[TEST_DIRECT] Отправляем тестовый инвойс без PreCheckoutQuery")

	invoice := &tele.Invoice{
		Title:       "Тестовый инвойс без PreCheckoutQuery",
		Description: "Тестирование платежной системы без PreCheckoutQuery",
		Payload:     "test_direct|123",
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Тест", Amount: 1}},
	}

	log.Printf("[TEST_DIRECT] Тестовый инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[TEST_DIRECT] Ошибка отправки тестового инвойса без PreCheckoutQuery: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки тестового инвойса без PreCheckoutQuery: %v", err))
	} else {
		log.Printf("[TEST_DIRECT] Тестовый инвойс без PreCheckoutQuery отправлен успешно")
	}
	return c.Send("Тестовый инвойс без PreCheckoutQuery отправлен успешно!")
}

// Функция для получения информации о боте
func (b *Bot) sendBotInfo(c tele.Context) error {
	info := fmt.Sprintf("🤖 Информация о боте:\n\n" +
		"💡 Для работы с платежами убедитесь, что:\n" +
		"1. Бот создан через @BotFather\n" +
		"2. Включены платежи в настройках бота\n" +
		"3. Используется правильная валюта (XTR)\n\n" +
		"🔧 Команды для тестирования:\n" +
		"/test_invoice - отправить тестовый инвойс\n" +
		"/test_precheckout - инструкции по тестированию\n" +
		"/api_info - информация об API\n\n" +
		"⚠️ Если PreCheckoutQuery не приходит:\n" +
		"1. Проверьте настройки бота в @BotFather\n" +
		"2. Убедитесь, что платежи включены\n" +
		"3. Попробуйте создать нового бота\n" +
		"4. Проверьте версию библиотеки telebot\n" +
		"5. Попробуйте переключиться на официальный API")

	return c.Send(info)
}

// Функция для получения информации об API
func (b *Bot) sendAPIInfo(c tele.Context) error {
	useOfficialAPI := os.Getenv("USE_OFFICIAL_API") == "true"
	apiURL := os.Getenv("TELEGRAM_API_URL")

	var info string
	if useOfficialAPI {
		info = fmt.Sprintf("🌐 Информация об API:\n\n" +
			"✅ Используется ОФИЦИАЛЬНЫЙ Telegram Bot API\n" +
			"URL: https://api.telegram.org\n\n" +
			"💡 Преимущества официального API:\n" +
			"• Полная поддержка всех функций Telegram\n" +
			"• Корректная обработка PreCheckoutQuery\n" +
			"• Стабильная работа платежей\n\n" +
			"⚠️ Ограничения:\n" +
			"• Ограничения на размер файлов (50 МБ)\n" +
			"• Медленная отправка больших файлов\n\n" +
			"🔧 Для переключения на локальный API:\n" +
			"Установите USE_OFFICIAL_API=false в .env")
	} else {
		info = fmt.Sprintf("🏠 Информация об API:\n\n"+
			"✅ Используется ЛОКАЛЬНЫЙ Telegram Bot API\n"+
			"URL: %s\n\n"+
			"💡 Преимущества локального API:\n"+
			"• Поддержка больших файлов (до 2 ГБ)\n"+
			"• Быстрая отправка файлов\n"+
			"• Нет ограничений на размер\n\n"+
			"⚠️ Возможные проблемы:\n"+
			"• Неполная поддержка PreCheckoutQuery\n"+
			"• Проблемы с платежами Telegram Stars\n"+
			"• Нестабильная работа некоторых функций\n\n"+
			"🔧 Для переключения на официальный API:\n"+
			"Установите USE_OFFICIAL_API=true в .env\n\n"+
			"💡 Рекомендация для тестирования платежей:\n"+
			"Попробуйте официальный API", apiURL)
	}

	return c.Send(info)
}

// Обработчик для ВСЕХ типов апдейтов
func (b *Bot) handleAnyUpdate(c tele.Context) error {
	// Просто возвращаем nil - апдейт обработан, но ничего не делаем
	return nil
}

// Вспомогательная функция для определения типа сообщения
func getMessageType(msg *tele.Message) string {
	if msg.Text != "" {
		return "text"
	} else if msg.Photo != nil {
		return "photo"
	} else if msg.Video != nil {
		return "video"
	} else if msg.Audio != nil {
		return "audio"
	} else if msg.Document != nil {
		return "document"
	} else if msg.Sticker != nil {
		return "sticker"
	} else if msg.Voice != nil {
		return "voice"
	} else if msg.VideoNote != nil {
		return "video_note"
	} else if msg.Contact != nil {
		return "contact"
	} else if msg.Location != nil {
		return "location"
	} else if msg.Venue != nil {
		return "venue"
	} else if msg.Poll != nil {
		return "poll"
	} else if msg.Dice != nil {
		return "dice"
	} else if msg.Animation != nil {
		return "animation"
	} else if msg.Payment != nil {
		return "payment"
	}
	return "unknown"
}

// Вспомогательная функция для определения типа апдейта
func getUpdateType(update *tele.Update) string {
	if update.Message != nil {
		// Определяем конкретный тип сообщения
		msg := update.Message
		if msg.Text != "" {
			return "message_text"
		} else if msg.Photo != nil {
			return "message_photo"
		} else if msg.Video != nil {
			return "message_video"
		} else if msg.Audio != nil {
			return "message_audio"
		} else if msg.Document != nil {
			return "message_document"
		} else if msg.Sticker != nil {
			return "message_sticker"
		} else if msg.Voice != nil {
			return "message_voice"
		} else if msg.VideoNote != nil {
			return "message_video_note"
		} else if msg.Contact != nil {
			return "message_contact"
		} else if msg.Location != nil {
			return "message_location"
		} else if msg.Venue != nil {
			return "message_venue"
		} else if msg.Poll != nil {
			return "message_poll"
		} else if msg.Dice != nil {
			return "message_dice"
		} else if msg.Animation != nil {
			return "message_animation"
		} else if msg.Payment != nil {
			return "message_payment"
		} else if msg.Invoice != nil {
			return "message_invoice"
		} else if msg.Game != nil {
			return "message_game"
		} else if msg.ReplyTo != nil {
			return "message_reply"
		} else if msg.PinnedMessage != nil {
			return "message_pinned"
		} else if msg.WebAppData != nil {
			return "message_web_app"
		} else if msg.VideoChatStarted != nil {
			return "message_video_chat_started"
		} else if msg.VideoChatEnded != nil {
			return "message_video_chat_ended"
		} else if msg.VideoChatScheduled != nil {
			return "message_video_chat_scheduled"
		} else if msg.BoostAdded != nil {
			return "message_boost"
		}
		return "message_unknown"
	} else if update.Callback != nil {
		return "callback_query"
	} else if update.PreCheckoutQuery != nil {
		return "pre_checkout_query"
	} else if update.ShippingQuery != nil {
		return "shipping_query"
	} else if update.ChannelPost != nil {
		return "channel_post"
	} else if update.EditedMessage != nil {
		return "edited_message"
	} else if update.EditedChannelPost != nil {
		return "edited_channel_post"
	} else if update.Poll != nil {
		return "poll"
	} else if update.PollAnswer != nil {
		return "poll_answer"
	} else if update.MyChatMember != nil {
		return "my_chat_member"
	} else if update.ChatMember != nil {
		return "chat_member"
	} else if update.ChatJoinRequest != nil {
		return "chat_join_request"
	}
	return "unknown"
}
