package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"YoutubeDownloader/internal/payment"
	"YoutubeDownloader/internal/storage"

	tele "gopkg.in/telebot.v4"
)

// sendUniversalPayKeyboard отправляет универсальную платежную клавиатуру
func (b *Bot) sendUniversalPayKeyboard(c tele.Context, url string) error {
	logger := NewLogger("PAYMENT")

	// Создаем транзакцию для видео
	trx := &payment.Transaction{
		InvoicePayload:          "video|" + url,
		Amount:                  1, // 1 XTR
		TelegramUserID:          c.Sender().ID,
		Status:                  "pending",
		TelegramPaymentChargeID: "",
	}

	// Сохраняем транзакцию в БД
	id, err := SaveTransactionToDB(b.db, trx)
	if err != nil {
		logger.Error("Ошибка сохранения транзакции: %v", err)
		return c.Send("Ошибка создания платежа. Попробуйте позже.")
	}

	// Создаем инлайн клавиатуру
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{
		{
			{
				Text: "💳 Оплатить 1 ⭐",
				Data: CallbackPayVideo + "|" + strconv.FormatInt(id, 10),
			},
		},
	}}

	logger.Info("Отправлена платежная клавиатура для URL: %s", url)
	return c.Send("🎬 Для скачивания видео необходимо оплатить 1 ⭐", markup)
}

// sendPaymentKeyboardWithSubscriptions отправляет платежную клавиатуру с опциями подписки
func (b *Bot) sendPaymentKeyboardWithSubscriptions(c tele.Context, url string) error {
	logger := NewLogger("PAYMENT")

	// Создаем транзакцию для видео
	trx := &payment.Transaction{
		InvoicePayload:          "video|" + url,
		Amount:                  1, // 1 XTR
		TelegramUserID:          c.Sender().ID,
		Status:                  "pending",
		TelegramPaymentChargeID: "",
	}

	// Сохраняем транзакцию в БД
	id, err := SaveTransactionToDB(b.db, trx)
	if err != nil {
		logger.Error("Ошибка сохранения транзакции: %v", err)
		return c.Send("Ошибка создания платежа. Попробуйте позже.")
	}

	// Создаем инлайн клавиатуру с опциями подписки
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{
		{
			{
				Text: "📢 ПОДПИСАТЬСЯ НА КАНАЛ (БЕСПЛАТНО)",
				Data: "subscribe_channel",
			},
		},
		{
			{
				Text: "💳 Оплатить 1 ⭐ за видео",
				Data: CallbackPayVideo + "|" + strconv.FormatInt(id, 10),
			},
		},
		{
			{
				Text: "📅 Подписка на месяц (5 ⭐)",
				Data: CallbackPaySubscribe,
			},
		},
		{
			{
				Text: "📅 Подписка на год (50 ⭐)",
				Data: CallbackPaySubscribeYear,
			},
		},
		{
			{
				Text: "♾️ Подписка навсегда (100 ⭐)",
				Data: CallbackPaySubscribeForever,
			},
		},
	}}

	message := `🎬 Для скачивания видео выберите один из вариантов:

📢 ПОДПИСАТЬСЯ НА КАНАЛ - БЕСПЛАТНО! 
   ⬆️ Нажмите кнопку выше для бесплатного скачивания ⬆️

💳 Разовое скачивание - 1 ⭐
📅 Подписка на месяц - 5 ⭐ (безлимитные скачивания)
📅 Подписка на год - 50 ⭐ (безлимитные скачивания)
♾️ Подписка навсегда - 100 ⭐ (безлимитные скачивания)

💡 Подписчики канала скачивают ВСЕ видео БЕСПЛАТНО!`

	logger.Info("Отправлена платежная клавиатура с подписками для URL: %s", url)
	return c.Send(message, markup)
}

// sendVideoInvoiceByDB отправляет инвойс для видео из БД
func (b *Bot) sendVideoInvoiceByDB(c tele.Context, trx *payment.Transaction) error {
	logger := NewLogger("INVOICE")

	invoice := &tele.Invoice{
		Title:       "Скачивание видео",
		Description: "Скачивание видео с YouTube и других платформ",
		Payload:     trx.InvoicePayload,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Скачивание ⭐", Amount: trx.Amount}},
	}

	logger.Info("Отправляем инвойс для видео: %s", trx.InvoicePayload)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		logger.Error("Ошибка отправки инвойса: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки инвойса: %v", err))
	}

	return nil
}

// sendSubscribeInvoice отправляет инвойс для подписки
func (b *Bot) sendSubscribeInvoice(c tele.Context, period string) error {
	logger := NewLogger("SUBSCRIBE")

	var title, description string
	var amount int

	switch period {
	case "month":
		title = "Подписка на месяц"
		description = "Подписка на месяц - безлимитные скачивания"
		amount = 5
	case "year":
		title = "Подписка на год"
		description = "Подписка на год - безлимитные скачивания"
		amount = 50
	case "forever":
		title = "Подписка навсегда"
		description = "Подписка навсегда - безлимитные скачивания"
		amount = 100
	default:
		return c.Send("Неизвестный период подписки")
	}

	invoice := &tele.Invoice{
		Title:       title,
		Description: description,
		Payload:     "subscribe|" + period,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: title + " ⭐", Amount: amount}},
	}

	logger.Info("Отправляем инвойс для подписки: %s (%d XTR)", period, amount)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		logger.Error("Ошибка отправки инвойса подписки: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки инвойса: %v", err))
	}

	return nil
}

// sendVideoWithRetry отправляет видео с повторными попытками
func (b *Bot) sendVideoWithRetry(c tele.Context, video *tele.Video, url string, maxRetries int) error {
	logger := NewLogger("VIDEO_SEND")

	for i := 0; i < maxRetries; i++ {
		err := c.Send(video)
		if err == nil {
			logger.Info("Видео успешно отправлено с попытки %d", i+1)
			return nil
		}

		logger.Warning("Попытка %d отправки видео не удалась: %v", i+1, err)
		if i < maxRetries-1 {
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}

	logger.Error("Все попытки отправки видео не удались")
	return fmt.Errorf("не удалось отправить видео после %d попыток", maxRetries)
}

// sendVideo обрабатывает скачивание и отправку видео
func (b *Bot) sendVideo(c tele.Context, url string, chargeID string, amount int) {
	logger := NewLogger("VIDEO")
	startTime := time.Now()

	logger.Info("Начинаем скачивание видео: %s", url)

	// Проверяем, не скачивается ли уже это видео
	if b.downloadManager.IsDownloadActive(url) {
		logger.Info("Видео уже скачивается, ожидаем завершения")
		c.Send("⏳ Видео уже скачивается, ожидаем завершения...")
		downloadInfo, err := b.downloadManager.WaitForDownload(url, b.config.DownloadTimeout)
		if err != nil {
			logger.Error("Ошибка ожидания скачивания: %v", err)
			c.Send("Произошла ошибка при ожидании скачивания видео.")
			return
		}
		if downloadInfo != nil && downloadInfo.Error != nil {
			logger.Error("Скачивание завершилось с ошибкой: %v", downloadInfo.Error)
			c.Send("Произошла ошибка при скачивании видео.")
			return
		}
	}

	// Получаем слот для скачивания
	if !b.downloadManager.AcquireDownloadSlot() {
		logger.Warning("Нет свободных слотов для скачивания")
		c.Send("Сейчас много запросов. Попробуйте позже.")
		return
	}
	defer b.downloadManager.ReleaseDownloadSlot()

	// Получаем мьютекс для URL
	mutex := b.downloadManager.GetURLMutex(url)
	mutex.Lock()
	defer func() {
		mutex.Unlock()
		b.downloadManager.CleanupURLMutex(url)
	}()

	// Регистрируем начало скачивания
	requestID := GenerateRequestID()
	_ = b.downloadManager.StartDownload(url, requestID, c.Sender().ID)
	defer b.downloadManager.FinishDownload(url, nil)

	// Проверяем кэш
	logger.Info("Проверяем кэш для URL: %s", url)
	cachedVideo, err := GetCachedVideo(b.db, url)
	if err != nil {
		logger.Warning("Ошибка получения из кэша: %v", err)
	} else if cachedVideo != nil {
		// Приведение типа для работы с кэшированным видео
		if cached, ok := cachedVideo.(*CachedVideo); ok {
			logger.Info("Найдено видео в кэше с file_id: %s", cached.FilePath)

			// Для кэшированного видео используем file_id от Telegram
			video := &tele.Video{
				File: tele.File{FileID: cached.FilePath}, // Используем FileID для кэшированного видео
			}

			// Отправляем кэшированное видео напрямую
			logger.Info("Отправляем кэшированное видео с file_id: %s", cached.FilePath)
			_, err := b.api.Send(c.Sender(), video)
			if err != nil {
				logger.Error("Ошибка отправки кэшированного видео: %v", err)
				// Если отправка по file_id не удалась, удаляем из кэша и скачиваем заново
				logger.Info("Удаляем недействительную запись из кэша")
				storage.DeleteVideoFromCache(b.db, url)
				// Продолжаем со скачиванием
			} else {
				logger.Info("Кэшированное видео успешно отправлено!")
				logger.LogPerformance("Отправка кэшированного видео", startTime)
				return
			}
		}
	}

	// Уведомляем пользователя о начале скачивания (только если видео не в кэше)
	c.Send("⏳ Видео качается...")

	// Скачиваем видео
	logger.Info("Скачиваем видео: %s", url)
	videoPath, err := DownloadVideo(url)
	if err != nil {
		logger.Error("Ошибка скачивания видео: %v", err)
		b.downloadManager.FinishDownload(url, err)
		c.Send("Ошибка скачивания видео. Попробуйте позже.")
		return
	}

	// Получаем информацию о видео
	videoInfo, err := GetVideoInfo(videoPath)
	if err != nil {
		logger.Error("Ошибка получения информации о видео: %v", err)
		b.downloadManager.FinishDownload(url, err)
		c.Send("Ошибка обработки видео.")
		return
	}

	// Отправляем видео
	if _, ok := videoInfo.(*VideoInfo); ok {
		video := &tele.Video{
			File: tele.FromDisk(videoPath),
		}

		// Отправляем видео напрямую через API для получения file_id
		sentMessage, err := b.api.Send(c.Sender(), video)
		if err != nil {
			logger.Error("Ошибка отправки видео: %v", err)
			b.downloadManager.FinishDownload(url, err)
			c.Send("Ошибка отправки видео.")
			return
		}

		// Сохраняем file_id в кэш, если видео было отправлено
		if sentMessage != nil && sentMessage.Video != nil && sentMessage.Video.FileID != "" {
			logger.Info("Сохраняем file_id в кэш: %s для URL: %s", sentMessage.Video.FileID, url)
			err = SaveVideoToCache(b.db, url, sentMessage.Video.FileID)
			if err != nil {
				logger.Warning("Ошибка сохранения file_id в кэш: %v", err)
			} else {
				logger.Info("File_id успешно сохранен в кэш")
			}
		} else {
			logger.Warning("Не удалось получить file_id для сохранения в кэш")
		}

		// Обновляем статистику транзакции
		if chargeID != "" {
			err = UpdateTransactionStatus(b.db, chargeID, "completed")
			if err != nil {
				logger.Error("Ошибка обновления статуса транзакции: %v", err)
			}
		}

		logger.LogPerformance("Полное скачивание и отправка видео", startTime)
	}
}

// CheckUserSubscriptionRaw проверяет подписку пользователя на канал через Telegram API
func (b *Bot) CheckUserSubscriptionRaw(channelUsername string, userID int64) (bool, error) {
	// channelUsername должен быть в формате "@yourchannel" или chat_id
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", b.api.Token)

	// Если нет @, добавим
	if !strings.HasPrefix(channelUsername, "@") && !strings.HasPrefix(channelUsername, "-") {
		channelUsername = "@" + channelUsername
	}

	data := map[string]interface{}{
		"chat_id": channelUsername,
		"user_id": userID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false, fmt.Errorf("ошибка маршалинга: %w", err)
	}
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("ошибка чтения ответа: %w", err)
	}
	var result struct {
		Ok     bool `json:"ok"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return false, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}
	if !result.Ok {
		return false, fmt.Errorf("ошибка Telegram API: %v", result.Description)
	}
	if result.Result.Status == "member" || result.Result.Status == "administrator" || result.Result.Status == "creator" {
		return true, nil
	}
	return false, nil
}

// sendError отправляет сообщение об ошибке
func (b *Bot) sendError(c tele.Context, userMsg string, err error, extraInfo ...string) {
	logger := NewLogger("ERROR")

	info := ""
	if len(extraInfo) > 0 {
		info = extraInfo[0]
	}
	logger.LogErrorWithContext(userMsg, err, info)

	c.Send(userMsg)
}
