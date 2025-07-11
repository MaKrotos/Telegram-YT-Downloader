package bot

import (
	"database/sql"
	"fmt"
	"log"
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
	if providerToken == "" {
		providerToken = "XTR"
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
	if url := os.Getenv("TELEGRAM_API_URL"); url != "" {
		settings.URL = url
	}
	api, err := tele.NewBot(settings)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:                api,
		adminID:            adminID,
		providerToken:      providerToken,
		transactionService: payment.NewTransactionService(),
		channelUsername:    channelUsername,
		downloadLimiter:    make(chan struct{}, maxWorkers),
		db:                 db,
	}, nil
}

func (b *Bot) Run() {
	b.api.Handle(tele.OnText, b.handleMessage)
	b.api.Handle(tele.OnCallback, b.handleCallback)
	b.api.Handle(tele.OnPayment, b.handlePayment)
	b.api.Start()
}

func (b *Bot) handleMessage(c tele.Context) error {
	msg := c.Message()
	if msg.Text == "/start" {
		return c.Send("👋 Добро пожаловать!\n\nЭтот бот позволяет скачивать видео с YouTube за Telegram Stars. Просто отправьте ссылку на видео YouTube или Shorts!")
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

	tiktokRegex := regexp.MustCompile(`(https?://)?(www\.)?(tiktok\.com|vm\.tiktok\.com)/[@\w\-?=&#./]+`)
	tiktokURL := tiktokRegex.FindString(msg.Text)
	if tiktokURL != "" {
		if b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
			go b.sendTikTokVideo(c, tiktokURL, "", 0)
			return nil
		}
		if b.channelUsername != "" {
			isSub, err := b.CheckUserSubscriptionRaw(b.channelUsername, msg.Sender.ID)
			if err == nil && isSub {
				go b.sendTikTokVideo(c, tiktokURL, "", 0)
				return nil
			}
		}
		return b.sendTikTokPayKeyboard(c, tiktokURL)
	}

	ytRegex := regexp.MustCompile(`(https?://)?(www\.)?(youtube\.com|youtu\.be)/[\w\-?=&#./]+`)
	url := ytRegex.FindString(msg.Text)
	if url == "" {
		return c.Send("Не обнаружено поддерживаемой ссылки. Пожалуйста, пришлите ссылку на видео YouTube или TikTok.")
	}
	isVideo := false
	if strings.Contains(url, "watch?v=") {
		isVideo = true
	}
	if strings.Contains(url, "/shorts/") {
		parts := strings.Split(url, "/shorts/")
		if len(parts) > 1 && len(parts[1]) >= 5 {
			isVideo = true
		}
	}
	if strings.Contains(url, "youtu.be/") {
		parts := strings.Split(url, "youtu.be/")
		if len(parts) > 1 && len(parts[1]) >= 5 {
			isVideo = true
		}
	}
	if !isVideo {
		return c.Send("Пожалуйста, отправьте ссылку на конкретное видео YouTube или Shorts.")
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
	return b.sendYouTubePayKeyboard(c, url)
}

func (b *Bot) sendYouTubePayKeyboard(c tele.Context, url string) error {
	btns := [][]tele.InlineButton{
		{{Text: "Скачать за 1 звезду", Data: "pay_video|" + url}},
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

func (b *Bot) sendTikTokPayKeyboard(c tele.Context, url string) error {
	btns := [][]tele.InlineButton{
		{{Text: "Скачать TikTok за 1 звезду", Data: "pay_tiktok|" + url}},
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
	data := c.Callback().Data
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
		url := strings.TrimPrefix(data, "pay_video|")
		return b.sendVideoInvoice(c, url)
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

func (b *Bot) sendVideoInvoice(c tele.Context, url string) error {
	invoice := &tele.Invoice{
		Title:       "Скачать видео",
		Description: "Скачивание видео с YouTube за 1 звезду",
		Payload:     "video|" + url,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Видео", Amount: 100}},
	}
	_, err := b.api.Send(c.Sender(), invoice, b.providerToken)
	return err
}

func (b *Bot) sendTikTokInvoice(c tele.Context, url string) error {
	invoice := &tele.Invoice{
		Title:       "Скачать TikTok видео",
		Description: "Скачивание TikTok видео за 1 звезду",
		Payload:     "tiktok|" + url,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "TikTok", Amount: 100}},
	}
	_, err := b.api.Send(c.Sender(), invoice, b.providerToken)
	return err
}

func (b *Bot) sendSubscribeInvoice(c tele.Context, period string) error {
	var price int
	var label, desc string
	switch period {
	case "month":
		price = 3000
		label = "Подписка на месяц"
		desc = "Доступ ко всем загрузкам на 1 месяц"
	case "year":
		price = 20000
		label = "Подписка на год"
		desc = "Доступ ко всем загрузкам на 1 год"
	case "forever":
		price = 100000
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
	_, err := b.api.Send(c.Sender(), invoice, b.providerToken)
	return err
}

func (b *Bot) handlePayment(c tele.Context) error {
	paymentInfo := c.Payment()
	if paymentInfo == nil {
		return nil
	}
	userID := c.Sender().ID
	payload := paymentInfo.Payload
	amount := paymentInfo.Total
	chargeID := paymentInfo.ProviderChargeID
	trx := &payment.Transaction{
		TelegramPaymentChargeID: chargeID,
		TelegramUserID:          userID,
		Amount:                  amount,
		InvoicePayload:          payload,
		Status:                  "success",
		Type:                    "stars",
		Reason:                  "",
	}
	b.transactionService.AddTransaction(trx)

	if strings.HasPrefix(payload, "video|") {
		url := strings.TrimPrefix(payload, "video|")
		go b.sendVideo(c, url, chargeID, amount)
		return c.Send("Оплата прошла успешно! Скачивание началось.")
	}
	if strings.HasPrefix(payload, "tiktok|") {
		url := strings.TrimPrefix(payload, "tiktok|")
		go b.sendTikTokVideo(c, url, chargeID, amount)
		return c.Send("Оплата прошла успешно! Скачивание TikTok началось.")
	}
	if strings.HasPrefix(payload, "subscribe|") {
		period := strings.TrimPrefix(payload, "subscribe|")
		// TODO: записать подписку в БД
		return c.Send("Подписка активирована: " + period)
	}
	return c.Send("Оплата прошла успешно!")
}

func (b *Bot) sendVideo(c tele.Context, url string, chargeID string, amount int) {
	c.Send("Скачиваю видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		defer func() { <-b.downloadLimiter }()
		filename, err := downloader.DownloadYouTubeVideo(url)
		if err != nil {
			c.Send("Ошибка при скачивании видео: " + err.Error())
			payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка скачивания видео")
			c.Send("Произошла ошибка. Ваши средства будут возвращены в ближайшее время.")
			return
		}
		video := &tele.Video{File: tele.FromDisk(filename), Caption: "Ваше видео!"}
		err = c.Send(video)
		if err != nil {
			if info, statErr := os.Stat(filename); statErr == nil {
				sizeMB := float64(info.Size()) / 1024.0 / 1024.0
				c.Send(fmt.Sprintf("Ошибка при отправке видео: %s\nРазмер файла: %.2f МБ", err.Error(), sizeMB))
			} else {
				c.Send("Ошибка при отправке видео: " + err.Error())
			}
			payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка отправки видео")
			c.Send("Произошла ошибка. Ваши средства будут возвращены в ближайшее время.")
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
			c.Send("Ошибка при скачивании TikTok видео: " + err.Error())
			payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка скачивания TikTok видео")
			c.Send("Произошла ошибка. Ваши средства будут возвращены в ближайшее время.")
			return
		}
		video := &tele.Video{File: tele.FromDisk(filename), Caption: "Ваше TikTok видео!"}
		err = c.Send(video)
		if err != nil {
			if info, statErr := os.Stat(filename); statErr == nil {
				sizeMB := float64(info.Size()) / 1024.0 / 1024.0
				c.Send(fmt.Sprintf("Ошибка при отправке видео: %s\nРазмер файла: %.2f МБ", err.Error(), sizeMB))
			} else {
				c.Send("Ошибка при отправке видео: " + err.Error())
			}
			payment.RefundStarPayment(c.Sender().ID, chargeID, amount, "Ошибка отправки TikTok видео")
			c.Send("Произошла ошибка. Ваши средства будут возвращены в ближайшее время.")
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
				return c.Send("Ошибка возврата: " + err.Error())
			}
			b.transactionService.MarkRefunded(chargeID)
			return c.Send("Возврат выполнен для транзакции: " + chargeID)
		}
	}
	// Если не нашли транзакцию — пробуем сделать возврат с пустыми amount и userID
	err := payment.RefundStarPayment(0, chargeID, 0, "Возврат по запросу админа (id не найден)")
	if err != nil {
		return c.Send("Ошибка возврата: " + err.Error())
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
				return c.Send("Ошибка возврата: " + err.Error())
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
		return c.Send("Ошибка возврата: " + err.Error())
	}
	return c.Send("Попытка возврата выполнена для транзакции: " + chargeID + " с user_id: " + strconv.FormatInt(userID, 10))
}

func toStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
