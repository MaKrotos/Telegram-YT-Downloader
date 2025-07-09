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
}

func NewBot(token, adminID, providerToken string) (*Bot, error) {
	if providerToken == "" {
		providerToken = "XTR"
	}
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:                api,
		adminID:            adminID,
		providerToken:      providerToken,
		transactionService: payment.NewTransactionService(),
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
	ytRegex := regexp.MustCompile(`(https?://)?(www\.)?(youtube\.com|youtu\.be)/[\w\-?=&#./]+`)
	url := ytRegex.FindString(msg.Text)
	if url == "" {
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
	if !isVideo {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "Пожалуйста, отправьте ссылку на конкретное видео YouTube или Shorts."))
		return
	}
	if b.adminID != "" && b.adminID == toStr(msg.From.ID) {
		b.sendVideo(msg.Chat.ID, url)
		return
	}
	// Кнопки оплаты
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Скачать за 1 звезду", fmt.Sprintf("pay_video|%s", url)),
			tgbotapi.NewInlineKeyboardButtonData("Подписка на месяц за 30 звёзд", "pay_subscribe"),
		),
	)
	msgConfig := tgbotapi.NewMessage(msg.Chat.ID, "Выберите способ оплаты:")
	msgConfig.ReplyMarkup = keyboard
	b.api.Send(msgConfig)
}

func (b *Bot) handleSuccessfulPayment(msg *tgbotapi.Message) {
	log.Printf("Успешная оплата! telegram_payment_charge_id: %s", msg.SuccessfulPayment.TelegramPaymentChargeID)
	url := msg.SuccessfulPayment.InvoicePayload
	chatID := msg.Chat.ID
	userID := msg.From.ID
	// Сообщаем пользователю
	b.api.Send(tgbotapi.NewMessage(chatID, "⭐️ Платёж успешно принят! Скачиваем видео..."))
	// Пробуем скачать и отправить видео
	err := b.sendVideo(chatID, url)
	sp := msg.SuccessfulPayment
	if err != nil {
		// Если не удалось — делаем возврат
		log.Printf("[ERROR] Ошибка скачивания видео: %v", err)
		errRefund := payment.RefundStarPayment(userID, sp.TelegramPaymentChargeID, sp.TotalAmount, "Не удалось скачать видео, возврат средств")
		if errRefund != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось скачать видео и вернуть средства. Обратитесь к администратору."))
		} else {
			b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось скачать видео. Ваши средства возвращены."))
		}
		return
	}
	// Если видео отправлено — записываем транзакцию
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
	if len(data) > 10 && data[:9] == "pay_video" {
		url := data[10:]
		err := b.sendStarsInvoice(chatID, "Скачивание видео YouTube", "Оплата 1 звезда за скачивание видео", url, []map[string]interface{}{{"label": "Скачивание", "amount": 1}})
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при выставлении счёта: "+err.Error()))
		}
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Выставлен счёт на скачивание"))
		return
	}
}

func (b *Bot) sendVideo(chatID int64, url string) error {
	msg := tgbotapi.NewMessage(chatID, "Скачиваю видео, пожалуйста, подождите...")
	b.api.Send(msg)
	filename, err := downloader.DownloadYouTubeVideo(url)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при скачивании видео: "+err.Error()))
		return err
	}
	videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filename))
	videoFile.Caption = "Ваше видео!"
	_, err = b.api.Send(videoFile)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(chatID, "Ошибка при отправке видео: "+err.Error()))
		return err
	}
	os.Remove(filename)
	return nil
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

func toStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
