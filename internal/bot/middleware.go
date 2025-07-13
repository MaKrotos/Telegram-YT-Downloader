package bot

import (
	tele "gopkg.in/telebot.v4"
)

// Структура для определения типа сообщения
type messageTypeChecker struct {
	condition func(*tele.Message) bool
	msgType   string
}

// Структура для определения типа апдейта
type updateTypeChecker struct {
	condition  func(*tele.Update) bool
	updateType string
}

// setupMiddlewareLegacy настраивает middleware для бота (legacy версия)
func (b *Bot) setupMiddlewareLegacy() {
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

// handleAnyUpdateLegacy обработчик для всех типов апдейтов
func (b *Bot) handleAnyUpdateLegacy(c tele.Context) error {
	logger := NewLogger("LEGACY_HANDLER")

	// Логируем необработанные апдейты для отладки
	update := c.Update()
	logger.Info("Получен необработанный апдейт типа: %s", getUpdateType(&update))

	// Можно добавить обработку специфических типов апдейтов здесь
	// Например, логирование, статистика, уведомления и т.д.

	return nil
}

// Определение типа сообщения через красивый массив
func getMessageType(msg *tele.Message) string {
	checkers := []messageTypeChecker{
		{func(m *tele.Message) bool { return m.Text != "" }, "text"},
		{func(m *tele.Message) bool { return m.Photo != nil }, "photo"},
		{func(m *tele.Message) bool { return m.Video != nil }, "video"},
		{func(m *tele.Message) bool { return m.Audio != nil }, "audio"},
		{func(m *tele.Message) bool { return m.Document != nil }, "document"},
		{func(m *tele.Message) bool { return m.Sticker != nil }, "sticker"},
		{func(m *tele.Message) bool { return m.Voice != nil }, "voice"},
		{func(m *tele.Message) bool { return m.VideoNote != nil }, "video_note"},
		{func(m *tele.Message) bool { return m.Contact != nil }, "contact"},
		{func(m *tele.Message) bool { return m.Location != nil }, "location"},
		{func(m *tele.Message) bool { return m.Venue != nil }, "venue"},
		{func(m *tele.Message) bool { return m.Poll != nil }, "poll"},
		{func(m *tele.Message) bool { return m.Dice != nil }, "dice"},
		{func(m *tele.Message) bool { return m.Animation != nil }, "animation"},
		{func(m *tele.Message) bool { return m.Payment != nil }, "payment"},
		{func(m *tele.Message) bool { return m.Invoice != nil }, "invoice"},
		{func(m *tele.Message) bool { return m.Game != nil }, "game"},
		{func(m *tele.Message) bool { return m.ReplyTo != nil }, "reply"},
		{func(m *tele.Message) bool { return m.PinnedMessage != nil }, "pinned"},
		{func(m *tele.Message) bool { return m.WebAppData != nil }, "web_app"},
		{func(m *tele.Message) bool { return m.VideoChatStarted != nil }, "video_chat_started"},
		{func(m *tele.Message) bool { return m.VideoChatEnded != nil }, "video_chat_ended"},
		{func(m *tele.Message) bool { return m.VideoChatScheduled != nil }, "video_chat_scheduled"},
		{func(m *tele.Message) bool { return m.BoostAdded != nil }, "boost"},
	}

	for _, checker := range checkers {
		if checker.condition(msg) {
			return checker.msgType
		}
	}
	return "unknown"
}

// Определение типа апдейта через красивый массив
func getUpdateType(update *tele.Update) string {
	checkers := []updateTypeChecker{
		{func(u *tele.Update) bool { return u.Message != nil }, "message_" + getMessageType(update.Message)},
		{func(u *tele.Update) bool { return u.Callback != nil }, "callback_query"},
		{func(u *tele.Update) bool { return u.PreCheckoutQuery != nil }, "pre_checkout_query"},
		{func(u *tele.Update) bool { return u.ShippingQuery != nil }, "shipping_query"},
		{func(u *tele.Update) bool { return u.ChannelPost != nil }, "channel_post"},
		{func(u *tele.Update) bool { return u.EditedMessage != nil }, "edited_message"},
		{func(u *tele.Update) bool { return u.EditedChannelPost != nil }, "edited_channel_post"},
		{func(u *tele.Update) bool { return u.Poll != nil }, "poll"},
		{func(u *tele.Update) bool { return u.PollAnswer != nil }, "poll_answer"},
		{func(u *tele.Update) bool { return u.MyChatMember != nil }, "my_chat_member"},
		{func(u *tele.Update) bool { return u.ChatMember != nil }, "chat_member"},
		{func(u *tele.Update) bool { return u.ChatJoinRequest != nil }, "chat_join_request"},
	}

	for _, checker := range checkers {
		if checker.condition(update) {
			return checker.updateType
		}
	}
	return "unknown"
}
