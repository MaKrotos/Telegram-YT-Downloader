package bot

import (
	"database/sql"
	"sync"
	"time"

	"YoutubeDownloader/internal/payment"

	tele "gopkg.in/telebot.v4"
)

// BotConfig содержит конфигурацию бота
type BotConfig struct {
	Token           string
	AdminID         string
	ProviderToken   string
	ChannelUsername string
	MaxWorkers      int
	UseOfficialAPI  bool
	TelegramAPIURL  string
	HTTPTimeout     time.Duration
	DownloadTimeout time.Duration
}

// Bot представляет основную структуру бота
type Bot struct {
	api                *tele.Bot
	config             *BotConfig
	transactionService *payment.TransactionService
	downloadManager    *DownloadManager
	db                 *sql.DB
}

// DownloadManager управляет скачиваниями
type DownloadManager struct {
	limiter         chan struct{}
	mutexMap        map[string]*sync.Mutex
	mutexMutex      sync.RWMutex
	activeDownloads map[string]*DownloadInfo
	downloadMutex   sync.RWMutex
}

// DownloadInfo содержит информацию об активном скачивании
type DownloadInfo struct {
	RequestID string
	UserID    int64
	StartTime time.Time
	Done      chan struct{}
	Error     error
}

// Handler интерфейс для обработчиков сообщений
type Handler interface {
	Handle(c tele.Context) error
	CanHandle(c tele.Context) bool
}

// MessageHandler обработчик текстовых сообщений
type MessageHandler struct {
	bot *Bot
}

// CallbackHandler обработчик callback запросов
type CallbackHandler struct {
	bot *Bot
}

// PaymentHandler обработчик платежей
type PaymentHandler struct {
	bot *Bot
}

// AdminHandler обработчик админских команд
type AdminHandler struct {
	bot *Bot
}

// Constants
const (
	DefaultMaxWorkers      = 3
	DefaultHTTPTimeout     = 120 * time.Second
	DefaultDownloadTimeout = 300 * time.Second
	DefaultPollerTimeout   = 60 * time.Second
)

// Command constants
const (
	CmdStart           = "/start"
	CmdAdmin           = "/admin"
	CmdTestInvoice     = "/test_invoice"
	CmdTestPreCheckout = "/test_precheckout"
	CmdBotInfo         = "/bot_info"
	CmdTestDirect      = "/test_direct"
	CmdAPIInfo         = "/api_info"
	CmdCacheStats      = "/cache_stats"
	CmdCacheClean      = "/cache_clean"
	CmdCacheClear      = "/cache_clear"
	CmdActiveDownloads = "/active_downloads"
	CmdRefund          = "/refund"
)

// Callback constants
const (
	CallbackPaySubscribe        = "pay_subscribe"
	CallbackPaySubscribeYear    = "pay_subscribe_year"
	CallbackPaySubscribeForever = "pay_subscribe_forever"
	CallbackPayVideo            = "pay_video"

	CallbackAdminRefund = "admin_refund"
)

// Error messages
const (
	ErrNoURLFound        = "Не обнаружено ссылки. Пожалуйста, пришлите ссылку на видео."
	ErrInvalidDays       = "Количество дней должно быть числом"
	ErrInvalidUserID     = "user_id должен быть числом"
	ErrInvalidChargeID   = "Укажите charge_id после /refund"
	ErrInvalidDaysFormat = "Укажите количество дней после /cache_clean"
)

// Success messages
const (
	MsgWelcome       = "👋 Добро пожаловать!\n\nЭтот бот позволяет скачивать видео с разных сайтов за Telegram Stars. Просто отправьте ссылку на видео!"
	MsgRefundSuccess = "Возврат выполнен для транзакции: %s"
	MsgRefundAttempt = "Попытка возврата выполнена для транзакции: %s"
)
