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

	tele "gopkg.in/telebot.v3"
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
	b.api.Start()
}

func (b *Bot) handleMessage(c tele.Context) error {
	msg := c.Message()
	if msg.Text == "/start" {
		return c.Send("👋 Добро пожаловать!\n\nЭтот бот позволяет скачивать видео с YouTube за Telegram Stars. Просто отправьте ссылку на видео YouTube или Shorts!")
	}

	tiktokRegex := regexp.MustCompile(`(https?://)?(www\.)?(tiktok\.com|vm\.tiktok\.com)/[@\w\-?=&#./]+`)
	tiktokURL := tiktokRegex.FindString(msg.Text)
	if tiktokURL != "" {
		if b.adminID != "" && b.adminID == toStr(msg.Sender.ID) {
			go b.sendTikTokVideo(c, tiktokURL)
			return nil
		}
		if b.channelUsername != "" {
			isSub, err := b.CheckUserSubscriptionRaw(b.channelUsername, msg.Sender.ID)
			if err == nil && isSub {
				go b.sendTikTokVideo(c, tiktokURL)
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
		go b.sendVideo(c, url)
		return nil
	}
	if b.channelUsername != "" {
		isSub, err := b.CheckUserSubscriptionRaw(b.channelUsername, msg.Sender.ID)
		if err == nil && isSub {
			go b.sendVideo(c, url)
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
		return c.Send("Платежная система не реализована в этом примере.")
	}
	if data == "pay_subscribe_year" {
		return c.Send("Платежная система не реализована в этом примере.")
	}
	if strings.HasPrefix(data, "pay_video|") {
		url := strings.TrimPrefix(data, "pay_video|")
		go b.sendVideo(c, url)
		return c.Respond(&tele.CallbackResponse{Text: "Скачивание началось!"})
	}
	if strings.HasPrefix(data, "pay_tiktok|") {
		url := strings.TrimPrefix(data, "pay_tiktok|")
		go b.sendTikTokVideo(c, url)
		return c.Respond(&tele.CallbackResponse{Text: "Скачивание TikTok началось!"})
	}
	if data == "pay_subscribe_forever" {
		return c.Send("Платежная система не реализована в этом примере.")
	}
	return nil
}

func (b *Bot) sendVideo(c tele.Context, url string) {
	c.Send("Скачиваю видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		defer func() { <-b.downloadLimiter }()
		filename, err := downloader.DownloadYouTubeVideo(url)
		if err != nil {
			c.Send("Ошибка при скачивании видео: " + err.Error())
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
		}
		os.Remove(filename)
	default:
		c.Send("Сейчас много загрузок. Пожалуйста, подождите и попробуйте чуть позже.")
	}
}

func (b *Bot) sendTikTokVideo(c tele.Context, url string) {
	c.Send("Скачиваю TikTok видео, пожалуйста, подождите...")
	select {
	case b.downloadLimiter <- struct{}{}:
		defer func() { <-b.downloadLimiter }()
		filename, err := downloader.DownloadTikTokVideo(url)
		if err != nil {
			c.Send("Ошибка при скачивании TikTok видео: " + err.Error())
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

func toStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
