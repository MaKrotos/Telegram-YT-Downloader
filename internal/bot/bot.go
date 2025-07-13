package bot

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

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
	downloadMutex      map[string]*sync.Mutex   // Мьютекс для каждого URL
	mutexMutex         sync.RWMutex             // Мьютекс для защиты map
	activeDownloads    map[string]*DownloadInfo // Активные скачивания
	downloadInfoMutex  sync.RWMutex             // Мьютекс для защиты activeDownloads
	db                 *sql.DB
}

// DownloadInfo содержит информацию об активном скачивании
type DownloadInfo struct {
	RequestID string
	UserID    int64
	StartTime time.Time
	Done      chan struct{} // Канал для сигнализации о завершении
	Error     error         // Ошибка, если скачивание не удалось
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
		downloadMutex:      make(map[string]*sync.Mutex),
		mutexMutex:         sync.RWMutex{},
		activeDownloads:    make(map[string]*DownloadInfo),
		downloadInfoMutex:  sync.RWMutex{},
		db:                 db,
	}, nil
}

func (b *Bot) Run() {
	// Настраиваем middleware
	b.setupMiddleware()

	// Регистрируем обработчики
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

// getURLMutex возвращает мьютекс для конкретного URL
func (b *Bot) getURLMutex(url string) *sync.Mutex {
	b.mutexMutex.RLock()
	mutex, exists := b.downloadMutex[url]
	b.mutexMutex.RUnlock()

	if !exists {
		b.mutexMutex.Lock()
		// Проверяем еще раз после получения блокировки на запись
		if mutex, exists = b.downloadMutex[url]; !exists {
			mutex = &sync.Mutex{}
			b.downloadMutex[url] = mutex
		}
		b.mutexMutex.Unlock()
	}

	return mutex
}

// cleanupURLMutex удаляет мьютекс для URL после завершения скачивания
func (b *Bot) cleanupURLMutex(url string) {
	b.mutexMutex.Lock()
	delete(b.downloadMutex, url)
	b.mutexMutex.Unlock()
}

// startDownload регистрирует начало скачивания
func (b *Bot) startDownload(url, requestID string, userID int64) *DownloadInfo {
	b.downloadInfoMutex.Lock()
	defer b.downloadInfoMutex.Unlock()

	downloadInfo := &DownloadInfo{
		RequestID: requestID,
		UserID:    userID,
		StartTime: time.Now(),
		Done:      make(chan struct{}),
	}

	b.activeDownloads[url] = downloadInfo
	log.Printf("[DOWNLOAD] [%s] Зарегистрировано активное скачивание для URL: %s", requestID, url)

	return downloadInfo
}

// finishDownload регистрирует завершение скачивания
func (b *Bot) finishDownload(url string, err error) {
	b.downloadInfoMutex.Lock()
	defer b.downloadInfoMutex.Unlock()

	if downloadInfo, exists := b.activeDownloads[url]; exists {
		downloadInfo.Error = err
		close(downloadInfo.Done)
		delete(b.activeDownloads, url)
		log.Printf("[DOWNLOAD] [%s] Завершено скачивание для URL: %s (ошибка: %v)", downloadInfo.RequestID, url, err)
	}
}

// waitForDownload ждет завершения активного скачивания
func (b *Bot) waitForDownload(url string, timeout time.Duration) (*DownloadInfo, error) {
	b.downloadInfoMutex.RLock()
	downloadInfo, exists := b.activeDownloads[url]
	b.downloadInfoMutex.RUnlock()

	if !exists {
		return nil, nil // Нет активного скачивания
	}

	log.Printf("[DOWNLOAD] Ожидание завершения скачивания URL: %s (начато пользователем %d)", url, downloadInfo.UserID)

	select {
	case <-downloadInfo.Done:
		return downloadInfo, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("таймаут ожидания скачивания")
	}
}

// isDownloadActive проверяет, активно ли скачивание для URL
func (b *Bot) isDownloadActive(url string) bool {
	b.downloadInfoMutex.RLock()
	defer b.downloadInfoMutex.RUnlock()

	_, exists := b.activeDownloads[url]
	return exists
}
