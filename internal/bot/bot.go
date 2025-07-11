package bot

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"YoutubeDownloader/internal/downloader"
	"YoutubeDownloader/internal/payment"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"

	_ "github.com/lib/pq"
)

type Bot struct {
	api                *tgbotapi.BotAPI
	adminID            string
	providerToken      string
	transactionService *payment.TransactionService
	channelUsername    string        // username канала для подписки
	downloadLimiter    chan struct{} // семафор для ограничения потоков
	db                 *sql.DB       // база данных для кэша
}

func NewBot(token, adminID, providerToken string, db *sql.DB) (*Bot, error) {
	if providerToken == "" {
		providerToken = "XTR"
	}
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	channelUsername := os.Getenv("CHANNEL_USERNAME")
	maxWorkers := 3 // по умолчанию
	if mwStr := os.Getenv("MAX_DOWNLOAD_WORKERS"); mwStr != "" {
		if mw, err := strconv.Atoi(mwStr); err == nil && mw > 0 {
			maxWorkers = mw
		}
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
	b.api.Debug = true
	log.Printf("Авторизация прошла успешно: %s", b.api.Self.UserName)

	// Подключение к PostgreSQL
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPass, dbName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Ошибка подключения к PostgreSQL: %v", err)
	}
	defer db.Close()

	// Миграция через goose
	cmd := exec.Command("goose", "-dir", "./migrations", "postgres", dsn, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Ошибка миграции goose: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		b.handleUpdate(update)
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		log.Printf("Получено сообщение от %s: %s", update.Message.From.UserName, update.Message.Text)
		b.handleMessage(update.Message)
	}
	if update.Message != nil && update.Message.SuccessfulPayment != nil {
		b.handleSuccessfulPayment(update.Message)
	}
	if update.PreCheckoutQuery != nil {
		b.handlePreCheckout(update.PreCheckoutQuery)
	}
	if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	if msg.Text == "/start" {
		// Устанавливаем описание, about и аватарку
		err := b.setupBotProfile()
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "Ошибка при установке профиля бота: "+err.Error()))
		} else {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "👋 Добро пожаловать!\n\nЭтот бот позволяет скачивать видео с YouTube за Telegram Stars. Просто отправьте ссылку на видео YouTube или Shorts!"))
		}
		return
	}

	tiktokRegex := regexp.MustCompile(`(https?://)?(www\.)?(tiktok\.com|vm\.tiktok\.com)/[@\w\-?=&#./]+`)
	tiktokURL := tiktokRegex.FindString(msg.Text)
	if tiktokURL != "" {
		// Проверка админа
		if b.adminID != "" && b.adminID == toStr(msg.From.ID) {
			b.sendTikTokVideo(msg.Chat.ID, tiktokURL)
			return
		}
		// Проверка подписки
		if b.channelUsername != "" {
			isSub, err := b.CheckUserSubscriptionRaw(b.channelUsername, msg.From.ID)
			if err == nil && isSub {
				b.sendTikTokVideo(msg.Chat.ID, tiktokURL)
				return
			}
		}
		// Кнопки оплаты и подписки для TikTok
		var keyboardRows [][]tgbotapi.InlineKeyboardButton
		payRow1 := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Скачать TikTok за 1 звезду", fmt.Sprintf("pay_tiktok|%s", tiktokURL)),
		)
		payRow2 := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подписка на месяц за 30 звёзд", "pay_subscribe"),
		)
		payRow3 := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подписка на год за 200 звёзд", "pay_subscribe_year"),
		)
		payRow4 := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Навсегда за 1000 звёзд", "pay_subscribe_forever"),
		)
		keyboardRows = append(keyboardRows, payRow1, payRow2, payRow3, payRow4)
		if b.channelUsername != "" {
			subscribeRow := tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("Подписаться на канал", fmt.Sprintf("https://t.me/%s", strings.TrimPrefix(b.channelUsername, "@"))),
			)
			keyboardRows = append(keyboardRows, subscribeRow)
		}
		msgText := "Выберите способ оплаты:"
		if b.channelUsername != "" {
			msgText = fmt.Sprintf("Подписчики канала %s могут использовать бота бесплатно!\n\n%s", b.channelUsername, msgText)
		}
		msgConfig := tgbotapi.NewMessage(msg.Chat.ID, msgText)
		msgConfig.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
		b.api.Send(msgConfig)
		return
	}

	ytRegex := regexp.MustCompile(`(https?://)?(www\.)?(youtube\.com|youtu\.be)/[\w\-?=&#./]+`)
	url := ytRegex.FindString(msg.Text)
	if url == "" {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "Не обнаружено поддерживаемой ссылки. Пожалуйста, пришлите ссылку на видео YouTube или TikTok."))
		return
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
	// Новая проверка для коротких ссылок youtu.be
	if strings.Contains(url, "youtu.be/") {
		parts := strings.Split(url, "youtu.be/")
		if len(parts) > 1 && len(parts[1]) >= 5 {
			isVideo = true
		}
	}
	if !isVideo {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "Пожалуйста, отправьте ссылку на конкретное видео YouTube или Shorts."))
		return
	}
	if b.adminID != "" && b.adminID == toStr(msg.From.ID) {
		b.sendVideo(msg.Chat.ID, url)
		return
	}
	// Если задан канал, проверяем подписку
	if b.channelUsername != "" {
		isSub, err := b.CheckUserSubscriptionRaw(b.channelUsername, msg.From.ID)
		if err == nil && isSub {
			b.sendVideo(msg.Chat.ID, url)
			return
		}
	}
	// Кнопки оплаты и подписки
	var keyboardRows [][]tgbotapi.InlineKeyboardButton
	// Кнопки оплаты теперь каждая в отдельной строке
	payRow1 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Скачать за 1 звезду", fmt.Sprintf("pay_video|%s", url)),
	)
	payRow2 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Подписка на месяц за 30 звёзд", "pay_subscribe"),
	)
	payRow3 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Подписка на год за 200 звёзд", "pay_subscribe_year"),
	)
	payRow4 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Навсегда за 1000 звёзд", "pay_subscribe_forever"),
	)
	keyboardRows = append(keyboardRows, payRow1, payRow2, payRow3, payRow4)
	if b.channelUsername != "" {
		// Кнопка подписки на канал отдельной строкой
		subscribeRow := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Подписаться на канал", fmt.Sprintf("https://t.me/%s", strings.TrimPrefix(b.channelUsername, "@"))),
		)
		keyboardRows = append(keyboardRows, subscribeRow)
	}
	msgText := "Выберите способ оплаты:"
	if b.channelUsername != "" {
		msgText = fmt.Sprintf("Подписчики канала %s могут использовать бота бесплатно!\n\n%s", b.channelUsername, msgText)
	}
	msgConfig := tgbotapi.NewMessage(msg.Chat.ID, msgText)
	msgConfig.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	b.api.Send(msgConfig)
}

func (b *Bot) handleSuccessfulPayment(msg *tgbotapi.Message) {
	log.Printf("Успешная оплата! telegram_payment_charge_id: %s", msg.SuccessfulPayment.TelegramPaymentChargeID)
	url := msg.SuccessfulPayment.InvoicePayload
	chatID := msg.Chat.ID
	userID := msg.From.ID
	b.api.Send(tgbotapi.NewMessage(chatID, "⭐️ Платёж успешно принят! Скачиваем видео..."))
	// Определяем тип ссылки
	if strings.Contains(url, "tiktok.com") || strings.Contains(url, "vm.tiktok.com") {
		err := b.sendTikTokVideo(chatID, url)
		sp := msg.SuccessfulPayment
		if err != nil {
			log.Printf("[ERROR] Ошибка скачивания TikTok видео: %v", err)
			errRefund := payment.RefundStarPayment(userID, sp.TelegramPaymentChargeID, sp.TotalAmount, "Не удалось скачать TikTok видео, возврат средств")
			if errRefund != nil {
				b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось скачать TikTok видео и вернуть средства. Обратитесь к администратору."))
			} else {
				b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось скачать TikTok видео. Ваши средства возвращены."))
			}
			return
		}
		trx := &payment.Transaction{
			TelegramPaymentChargeID: sp.TelegramPaymentChargeID,
			TelegramUserID:          userID,
			Amount:                  sp.TotalAmount,
			InvoicePayload:          sp.InvoicePayload,
			Status:                  "success",
			Type:                    "payment",
			Reason:                  "Оплата через Telegram Stars",
		}
		b.transactionService.AddTransaction(trx)
		return
	}
	// YouTube/Shorts
	err := b.sendVideo(chatID, url)
	sp := msg.SuccessfulPayment
	if err != nil {
		log.Printf("[ERROR] Ошибка скачивания видео: %v", err)
		errRefund := payment.RefundStarPayment(userID, sp.TelegramPaymentChargeID, sp.TotalAmount, "Не удалось скачать видео, возврат средств")
		if errRefund != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось скачать видео и вернуть средства. Обратитесь к администратору."))
		} else {
			b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось скачать видео. Ваши средства возвращены."))
		}
		return
	}
	trx := &payment.Transaction{
		TelegramPaymentChargeID: sp.TelegramPaymentChargeID,
		TelegramUserID:          userID,
		Amount:                  sp.TotalAmount,
		InvoicePayload:          sp.InvoicePayload,
		Status:                  "success",
		Type:                    "payment",
		Reason:                  "Оплата через Telegram Stars",
	}
	b.transactionService.AddTransaction(trx)
}

func (b *Bot) handlePreCheckout(q *tgbotapi.PreCheckoutQuery) {
	preCheckoutConfig := tgbotapi.PreCheckoutConfig{
		PreCheckoutQueryID: q.ID,
		OK:                 true,
	}
	if _, err := b.api.Request(preCheckoutConfig); err != nil {
		log.Printf("Ошибка подтверждения pre_checkout_query: %v", err)
	}
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	chatID := cb.Message.Chat.ID
	// userID := cb.From.ID // больше не используется
	if data == "pay_subscribe" {
		err := b.sendStarsInvoice(chatID, "Подписка на месяц", "Оформление премиум-подписки на 30 дней", "subscribe", []map[string]interface{}{{"label": "Подписка на месяц", "amount": 30}})
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при выставлении счёта: "+err.Error()))
		}
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Выставлен счёт на подписку"))
		return
	}
	if data == "pay_subscribe_year" {
		err := b.sendStarsInvoice(chatID, "Подписка на год", "Оформление премиум-подписки на 365 дней", "subscribe_year", []map[string]interface{}{{"label": "Подписка на год", "amount": 200}})
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при выставлении счёта: "+err.Error()))
		}
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Выставлен счёт на годовую подписку"))
		return
	}
	if len(data) > 10 && data[:9] == "pay_video" {
		url := data[10:]
		err := b.sendStarsInvoice(chatID, "Скачивание видео YouTube", "Оплата 1 звезда за скачивание видео", url, []map[string]interface{}{{"label": "Скачивание", "amount": 1}})
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при выставлении счёта: "+err.Error()))
		}
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Выставлен счёт на скачивание"))
		return
	}
	if len(data) > 12 && data[:10] == "pay_tiktok" {
		url := data[11:]
		err := b.sendStarsInvoice(chatID, "Скачивание TikTok видео", "Оплата 1 звезда за скачивание TikTok видео", url, []map[string]interface{}{{"label": "Скачивание TikTok", "amount": 1}})
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при выставлении счёта: "+err.Error()))
		}
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Выставлен счёт на скачивание TikTok"))
		return
	}
	if data == "pay_subscribe_forever" {
		err := b.sendStarsInvoice(chatID, "Подписка навсегда", "Оформление бессрочной премиум-подписки", "subscribe_forever", []map[string]interface{}{{"label": "Подписка навсегда", "amount": 1000}})
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при выставлении счёта: "+err.Error()))
		}
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Выставлен счёт на бессрочную подписку"))
		return
	}
}

func (b *Bot) sendVideo(chatID int64, url string) error {
	// Сначала ищем file_id в кэше
	if fileID, err := b.getCachedFileID(url); err == nil && fileID != "" {
		videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FileID(fileID))
		videoFile.Caption = "Ваше видео! (из кэша)"
		_, err = b.api.Send(videoFile)
		return err // Возвращаем сразу, не ждём downloadLimiter
	}

	msg := tgbotapi.NewMessage(chatID, "Скачиваю видео, пожалуйста, подождите...")
	b.api.Send(msg)

	select {
	case b.downloadLimiter <- struct{}{}:
		go func() {
			defer func() { <-b.downloadLimiter }()
			filename, err := downloader.DownloadYouTubeVideo(url)
			if err != nil {
				b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при скачивании видео: "+err.Error()))
				return
			}
			videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filename))
			videoFile.Caption = "Ваше видео!"
			msgObj, err := b.api.Send(videoFile)
			if err == nil {
				if msgObj.Video != nil {
					b.saveCachedFileID(url, msgObj.Video.FileID)
				}
			}
			if err != nil {
				b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при отправке видео: "+err.Error()))
			}
			os.Remove(filename)
		}()
		return nil
	default:
		b.api.Send(tgbotapi.NewMessage(chatID, "Сейчас много загрузок. Пожалуйста, подождите и попробуйте чуть позже."))
		return fmt.Errorf("превышен лимит одновременных загрузок")
	}
}

func (b *Bot) sendTikTokVideo(chatID int64, url string) error {
	// Кэширование TikTok видео
	if fileID, err := b.getCachedFileID(url); err == nil && fileID != "" {
		videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FileID(fileID))
		videoFile.Caption = "Ваше TikTok видео! (из кэша)"
		_, err = b.api.Send(videoFile)
		return err
	}
	msg := tgbotapi.NewMessage(chatID, "Скачиваю TikTok видео, пожалуйста, подождите...")
	b.api.Send(msg)
	select {
	case b.downloadLimiter <- struct{}{}:
		go func() {
			defer func() { <-b.downloadLimiter }()
			filename, err := downloader.DownloadTikTokVideo(url)
			if err != nil {
				b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при скачивании TikTok видео: "+err.Error()))
				return
			}
			videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filename))
			videoFile.Caption = "Ваше TikTok видео!"
			msgObj, err := b.api.Send(videoFile)
			if err == nil {
				if msgObj.Video != nil {
					b.saveCachedFileID(url, msgObj.Video.FileID)
				}
			}
			if err != nil {
				b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при отправке видео: "+err.Error()))
			}
			os.Remove(filename)
		}()
		return nil
	default:
		b.api.Send(tgbotapi.NewMessage(chatID, "Сейчас много загрузок. Пожалуйста, подождите и попробуйте чуть позже."))
		return fmt.Errorf("превышен лимит одновременных загрузок")
	}
}

func (b *Bot) sendStarsInvoice(chatID int64, title, description, payload string, prices []map[string]interface{}) error {
	invoice := map[string]interface{}{
		"chat_id":               chatID,
		"title":                 title,
		"description":           description,
		"payload":               payload,
		"currency":              "XTR",
		"prices":                prices,
		"suggested_tip_amounts": []int{},
	}
	jsonData, err := json.Marshal(invoice)
	if err != nil {
		return fmt.Errorf("ошибка маршалинга инвойса: %w", err)
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendInvoice", b.api.Token)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка отправки инвойса: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return fmt.Errorf("ошибка декодирования ответа: %w", err)
	}
	if ok, exists := result["ok"].(bool); !exists || !ok {
		desc := result["description"]
		return fmt.Errorf("ошибка Telegram API: %v", desc)
	}
	return nil
}

// Устанавливает описание, about и аватарку бота через кастомные запросы к Bot API
func (b *Bot) setupBotProfile() error {
	// 1. Установить описание (description)
	descReq := tgbotapi.Params{
		"description": "Скачивайте видео с YouTube за Telegram Stars!", // Можно изменить текст
	}
	_, err := b.api.MakeRequest("setMyDescription", descReq)
	if err != nil {
		return fmt.Errorf("setMyDescription: %w", err)
	}

	// 2. Установить about (short description)
	aboutReq := tgbotapi.Params{
		"short_description": "YouTube → Telegram за 1 звезду!", // Можно изменить текст
	}
	_, err = b.api.MakeRequest("setMyShortDescription", aboutReq)
	if err != nil {
		return fmt.Errorf("setMyShortDescription: %w", err)
	}

	// 3. Установить аватарку (profile photo)
	// Установка аватарки отключена по требованию
	return nil
}

// CheckUserSubscriptionRaw проверяет подписку пользователя на канал через прямой HTTP-запрос к Bot API
func (b *Bot) CheckUserSubscriptionRaw(channelUsername string, userID int64) (bool, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", b.api.Token)
	// channelUsername должен быть в формате "@yourchannel"
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

func (b *Bot) getCachedFileID(url string) (string, error) {
	var fileID string
	err := b.db.QueryRow("SELECT telegram_file_id FROM video_cache WHERE url = $1", url).Scan(&fileID)
	if err != nil {
		return "", err
	}
	return fileID, nil
}

func (b *Bot) saveCachedFileID(url, fileID string) error {
	_, err := b.db.Exec("INSERT INTO video_cache (url, telegram_file_id) VALUES ($1, $2) ON CONFLICT (url) DO UPDATE SET telegram_file_id = EXCLUDED.telegram_file_id", url, fileID)
	return err
}

func toStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
