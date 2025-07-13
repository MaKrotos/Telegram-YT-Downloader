package bot

import (
	"database/sql"
	"net/http"

	"YoutubeDownloader/internal/payment"

	tele "gopkg.in/telebot.v4"
)

// NewBot создает новый экземпляр бота
func NewBot(token, adminID, providerToken string, db *sql.DB) (*Bot, error) {
	config := NewBotConfig(token, adminID, providerToken)
	logger := NewLogger("BOT")

	logger.Info("Инициализация бота для Telegram Stars")
	logger.LogConfig(config)

	// Создаем настройки для Telegram API
	settings := tele.Settings{
		Token:  config.Token,
		Poller: &tele.LongPoller{Timeout: DefaultPollerTimeout},
		Client: &http.Client{Timeout: config.HTTPTimeout},
	}

	// Настройка URL для API
	if config.UseOfficialAPI {
		settings.URL = config.TelegramAPIURL
		logger.Info("Используем ОФИЦИАЛЬНЫЙ Telegram Bot API: %s", settings.URL)
	} else {
		if config.TelegramAPIURL != "" {
			settings.URL = config.TelegramAPIURL
			logger.Info("Используем ЛОКАЛЬНЫЙ Telegram Bot API: %s", settings.URL)
		} else {
			logger.Info("Используем дефолтный локальный API URL")
		}
	}

	api, err := tele.NewBot(settings)
	if err != nil {
		return nil, err
	}

	logger.Info("Бот успешно инициализирован")

	return &Bot{
		api:                api,
		config:             config,
		transactionService: payment.NewTransactionService(),
		downloadManager:    NewDownloadManager(config.MaxWorkers),
		db:                 db,
	}, nil
}

// Run запускает бота
func (b *Bot) Run() {
	logger := NewLogger("BOT")

	// Настраиваем middleware
	b.setupMiddleware()

	// Регистрируем основные обработчики
	b.registerHandlers()

	logger.Info("Запуск бота...")
	b.api.Start()
}

// registerHandlers регистрирует все обработчики
func (b *Bot) registerHandlers() {
	logger := NewLogger("HANDLERS")

	// Основные обработчики
	b.api.Handle(tele.OnText, b.handleMessage)
	b.api.Handle(tele.OnCallback, b.handleCallback)
	b.api.Handle(tele.OnPayment, b.handlePayment)

	// Регистрируем обработчики для всех остальных типов апдейтов
	b.registerAllUpdateHandlers()

	logger.Info("Все обработчики зарегистрированы")
}

// registerAllUpdateHandlers регистрирует обработчики для всех типов апдейтов
func (b *Bot) registerAllUpdateHandlers() {
	updateHandlers := []interface{}{
		tele.OnForward, tele.OnReply, tele.OnEdited, tele.OnPhoto, tele.OnAudio,
		tele.OnAnimation, tele.OnDocument, tele.OnSticker, tele.OnVideo,
		tele.OnVoice, tele.OnVideoNote, tele.OnContact, tele.OnLocation,
		tele.OnVenue, tele.OnDice, tele.OnInvoice, tele.OnRefund, tele.OnGame,
		tele.OnPoll, tele.OnPollAnswer, tele.OnPinned, tele.OnChannelPost,
		tele.OnEditedChannelPost, tele.OnTopicCreated, tele.OnTopicReopened,
		tele.OnTopicClosed, tele.OnTopicEdited, tele.OnGeneralTopicHidden,
		tele.OnGeneralTopicUnhidden, tele.OnWriteAccessAllowed, tele.OnAddedToGroup,
		tele.OnUserJoined, tele.OnUserLeft, tele.OnUserShared, tele.OnChatShared,
		tele.OnNewGroupTitle, tele.OnNewGroupPhoto, tele.OnGroupPhotoDeleted,
		tele.OnGroupCreated, tele.OnSuperGroupCreated, tele.OnChannelCreated,
		tele.OnMigration, tele.OnMedia, tele.OnQuery, tele.OnInlineResult,
		tele.OnShipping, tele.OnCheckout, tele.OnMyChatMember, tele.OnChatMember,
		tele.OnChatJoinRequest, tele.OnProximityAlert, tele.OnAutoDeleteTimer,
		tele.OnWebApp, tele.OnVideoChatStarted, tele.OnVideoChatEnded,
		tele.OnVideoChatParticipants, tele.OnVideoChatScheduled, tele.OnBoost,
		tele.OnBoostRemoved, tele.OnBusinessConnection, tele.OnBusinessMessage,
		tele.OnEditedBusinessMessage, tele.OnDeletedBusinessMessages,
	}

	for _, handlerType := range updateHandlers {
		b.api.Handle(handlerType, b.handleAnyUpdate)
	}
}

// setupMiddleware настраивает middleware для бота
func (b *Bot) setupMiddleware() {
	logger := NewLogger("MIDDLEWARE")

	b.api.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			update := c.Update()

			// Логируем обновления
			logger.LogUpdate(&update)

			// Обрабатываем платежи прямо в middleware
			if update.Message != nil && update.Message.Payment != nil {
				logger.Info("Найден платеж в Message: %+v", update.Message.Payment)
				return b.handlePayment(c)
			}

			// Автоматически подтверждаем PreCheckoutQuery
			if update.PreCheckoutQuery != nil {
				logger.Info("PreCheckoutQuery: user_id=%d", update.PreCheckoutQuery.Sender.ID)
				if err := c.Accept(); err != nil {
					logger.Error("Ошибка подтверждения PreCheckoutQuery: %v", err)
				} else {
					logger.Info("PreCheckoutQuery подтвержден для user_id=%d", update.PreCheckoutQuery.Sender.ID)
				}
				return nil // Не передаем дальше
			}

			return next(c)
		}
	})

	logger.Info("Middleware настроен")
}

// handleAnyUpdate обработчик для всех типов апдейтов
func (b *Bot) handleAnyUpdate(c tele.Context) error {
	logger := NewLogger("UPDATE_HANDLER")

	// Логируем необработанные апдейты для отладки
	update := c.Update()
	logger.Info("Получен необработанный апдейт типа: %s", getUpdateType(&update))

	// Можно добавить обработку специфических типов апдейтов здесь
	// Например, логирование, статистика, уведомления и т.д.

	return nil
}
