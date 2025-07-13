package bot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"YoutubeDownloader/internal/payment"

	tele "gopkg.in/telebot.v4"
)

// handleMessage обрабатывает текстовые сообщения
func (b *Bot) handleMessage(c tele.Context) error {
	msg := c.Message()
	logger := NewLogger("MESSAGE")

	logger.Info("user_id=%d, text=%q", msg.Sender.ID, msg.Text)

	// Обработка команды /start
	if msg.Text == CmdStart {
		return c.Send(b.i18nManager.T(msg.Sender, "welcome"))
	}

	// Проверяем, является ли пользователь админом
	isAdmin := b.config.AdminID != "" && b.config.AdminID == toStr(msg.Sender.ID)

	// Обработка админских команд
	if isAdmin {
		handled, err := b.handleAdminCommands(c, msg)
		if err != nil {
			return err
		}
		if handled {
			return nil // Команда была обработана, не обрабатываем дальше
		}
	}

	// Обработка обычных команд пользователей
	handled, err := b.handleUserCommands(c, msg)
	if err != nil {
		return err
	}
	if handled {
		return nil // Команда была обработана, не обрабатываем дальше
	}

	// Обработка URL
	return b.handleURLMessage(c, msg, isAdmin)
}

// handleAdminCommands обрабатывает админские команды
// Возвращает (обработана_ли_команда, ошибка)
func (b *Bot) handleAdminCommands(c tele.Context, msg *tele.Message) (bool, error) {
	// Массив админских команд с их обработчиками
	adminCommands := []struct {
		command string
		handler func(tele.Context) error
	}{
		{CmdTestInvoice, b.sendTestInvoice},
		{CmdTestPreCheckout, func(c tele.Context) error {
			return c.Send(b.i18nManager.T(msg.Sender, "test_precheckout_instructions"))
		}},
		{CmdBotInfo, b.sendBotInfo},
		{CmdTestDirect, b.sendDirectInvoice},
		{CmdAPIInfo, b.sendAPIInfo},
		{CmdCacheStats, b.sendCacheStats},
		{CmdCacheClear, b.clearAllCache},
		{CmdActiveDownloads, b.sendActiveDownloads},
		{CmdAdmin, b.sendAdminTransactionsMenu},
		{"/test_subscription", b.testSubscription},
		{"/test_channel", b.testChannel},
		{"/config", b.showConfig},
		{"/fix_channel", b.fixChannelConfig},
	}

	// Проверяем точные совпадения команд
	for _, cmd := range adminCommands {
		if msg.Text == cmd.command {
			return true, cmd.handler(c)
		}
	}

	// Обработка команд с параметрами
	if strings.HasPrefix(msg.Text, CmdCacheClean) {
		return true, b.handleCacheCleanCommand(c, msg.Text)
	}
	if strings.HasPrefix(msg.Text, CmdRefund) {
		return true, b.handleRefundCommand(c, msg.Text)
	}

	return false, nil
}

// handleUserCommands обрабатывает команды пользователей
// Возвращает (обработана_ли_команда, ошибка)
func (b *Bot) handleUserCommands(c tele.Context, msg *tele.Message) (bool, error) {
	// Здесь можно добавить обработку команд для обычных пользователей
	return false, nil
}

// handleURLMessage обрабатывает сообщения с URL
func (b *Bot) handleURLMessage(c tele.Context, msg *tele.Message, isAdmin bool) error {
	logger := NewLogger("URL_HANDLER")

	// Проверяем, есть ли текст в сообщении
	if strings.TrimSpace(msg.Text) == "" {
		logger.Info("Пустое сообщение - игнорируем")
		return nil // Игнорируем пустые сообщения
	}

	urlRegex := regexp.MustCompile(`https?://\S+`)
	url := urlRegex.FindString(msg.Text)

	if url == "" {
		logger.Info("URL не найден в сообщении: %q", msg.Text)
		return c.Send(b.i18nManager.T(msg.Sender, "no_url_found"))
	}

	logger.Info("Обрабатываем URL: %s для пользователя %d (админ: %t)", url, msg.Sender.ID, isAdmin)

	if isAdmin {
		logger.Info("Пользователь %d является админом — скачивание бесплатно", msg.Sender.ID)
		go b.sendVideo(c, url, "", 0)
		return nil
	}

	// ВСЕГДА проверяем подписку для не-админов
	logger.Info("Проверяем подписку для пользователя %d", msg.Sender.ID)

	if b.config.ChannelUsername == "" {
		logger.Warning("ChannelUsername не задан в конфиге! Подписка не может быть проверена, предлагаем оплату.")
		return b.sendPaymentKeyboardWithSubscriptions(c, url)
	}

	logger.Info("Проверяем подписку пользователя %d на канал %s", msg.Sender.ID, b.config.ChannelUsername)
	isSub, err := b.CheckUserSubscriptionRaw(b.config.ChannelUsername, msg.Sender.ID)
	if err != nil {
		logger.Warning("Ошибка проверки подписки пользователя %d на канал %s: %v", msg.Sender.ID, b.config.ChannelUsername, err)
		logger.Info("Из-за ошибки проверки подписки предлагаем оплату")
		return b.sendPaymentKeyboardWithSubscriptions(c, url)
	}

	if isSub {
		logger.Info("Пользователь %d подписан на канал %s — скачивание бесплатно", msg.Sender.ID, b.config.ChannelUsername)
		go b.sendVideo(c, url, "", 0)
		return nil
	}

	logger.Info("Пользователь %d НЕ подписан на канал %s — предлагаем оплату", msg.Sender.ID, b.config.ChannelUsername)
	return b.sendPaymentKeyboardWithSubscriptions(c, url)
}

// handleCacheCleanCommand обрабатывает команду очистки кэша
func (b *Bot) handleCacheCleanCommand(c tele.Context, text string) error {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return c.Send(b.i18nManager.T(c.Sender(), "invalid_days_format"))
	}

	daysStr := strings.TrimSpace(parts[1])
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		return c.Send(b.i18nManager.T(c.Sender(), "invalid_days"))
	}

	return b.cleanOldCache(c, days)
}

// handleRefundCommand обрабатывает команду возврата
func (b *Bot) handleRefundCommand(c tele.Context, text string) error {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return c.Send(b.i18nManager.T(c.Sender(), "invalid_charge_id"))
	}

	chargeID := strings.TrimSpace(parts[1])
	var userID int64 = 0

	if len(parts) >= 3 {
		parsed, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return c.Send(b.i18nManager.T(c.Sender(), "invalid_user_id"))
		}
		userID = parsed
	}

	return b.handleAdminRefundWithUserID(c, chargeID, userID)
}

// handleCallback обрабатывает callback запросы
func (b *Bot) handleCallback(c tele.Context) error {
	cb := c.Callback()
	logger := NewLogger("CALLBACK")

	logger.Info("user_id=%d, data=%q", cb.Sender.ID, cb.Data)

	data := cb.Data

	// Обработка подписки на канал
	if data == "subscribe_channel" {
		return b.handleChannelSubscription(c)
	}

	// Обработка проверки подписки
	if data == "check_subscription" {
		return b.handleCheckSubscription(c)
	}

	// Обработка подписок
	switch data {
	case CallbackPaySubscribe:
		return b.sendSubscribeInvoice(c, "month")
	case CallbackPaySubscribeYear:
		return b.sendSubscribeInvoice(c, "year")
	case CallbackPaySubscribeForever:
		return b.sendSubscribeInvoice(c, "forever")
	}

	// Обработка платежей за видео
	if strings.HasPrefix(data, CallbackPayVideo+"|") {
		return b.handleVideoPaymentCallback(c, data)
	}

	// Обработка админских возвратов
	if strings.HasPrefix(data, CallbackAdminRefund+"|") && b.config.AdminID != "" && b.config.AdminID == toStr(c.Sender().ID) {
		chargeID := strings.TrimPrefix(data, CallbackAdminRefund+"|")
		return b.handleAdminRefund(c, chargeID)
	}

	return nil
}

// handleVideoPaymentCallback обрабатывает callback для платежа за видео
func (b *Bot) handleVideoPaymentCallback(c tele.Context, data string) error {
	idStr := strings.TrimPrefix(data, CallbackPayVideo+"|")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.Send(b.i18nManager.T(c.Sender(), "invalid_transaction_id"))
	}

	trx, err := payment.GetTransactionByID(b.db, id)
	if err != nil {
		return c.Send(b.i18nManager.T(c.Sender(), "transaction_not_found"))
	}

	return b.sendVideoInvoiceByDB(c, trx)
}

// handlePayment обрабатывает платежи
func (b *Bot) handlePayment(c tele.Context) error {
	logger := NewLogger("PAYMENT")
	logger.Debug("Вызван handlePayment")

	// Пробуем получить платеж разными способами
	var paymentInfo *tele.Payment

	// Сначала пробуем через c.Payment()
	paymentInfo = c.Payment()
	if paymentInfo == nil {
		// Если не получилось, пробуем через Message
		update := c.Update()
		if update.Message != nil && update.Message.Payment != nil {
			paymentInfo = update.Message.Payment
			logger.Debug("Платеж найден в Message")
		}
	}

	if paymentInfo == nil {
		logger.Debug("paymentInfo == nil, событие не обработано")
		return c.Send("Ошибка: информация об оплате не получена")
	}

	userID := c.Sender().ID
	payload := paymentInfo.Payload
	amount := paymentInfo.Total
	chargeID := paymentInfo.ProviderChargeID

	logger.LogPayment(userID, payload, chargeID, amount)

	// Логируем все транзакции до обновления
	trxs2, err2 := payment.GetAllTransactionsFromDB(b.db)
	if err2 == nil {
		logger.Debug("Все транзакции до обновления: %+v", trxs2)
	} else {
		logger.Debug("Ошибка получения всех транзакций: %v", err2)
	}

	// Обрабатываем платеж
	return b.processPayment(c, paymentInfo)
}

// processPayment обрабатывает платеж
func (b *Bot) processPayment(c tele.Context, paymentInfo *tele.Payment) error {
	logger := NewLogger("PAYMENT")

	payload := paymentInfo.Payload
	amount := paymentInfo.Total
	chargeID := paymentInfo.ProviderChargeID

	// Обновляем статус транзакции
	err := UpdateTransactionStatus(b.db, chargeID, "success")
	if err != nil {
		logger.Error("Ошибка обновления статуса транзакции: %v", err)
		return c.Send(b.i18nManager.T(c.Sender(), "error_processing_payment"))
	}

	// Логируем все транзакции после обновления
	trxs3, err3 := payment.GetAllTransactionsFromDB(b.db)
	if err3 == nil {
		logger.Debug("Все транзакции после обновления: %+v", trxs3)
	} else {
		logger.Debug("Ошибка получения всех транзакций после обновления: %v", err3)
	}

	// Обрабатываем разные типы платежей
	if strings.HasPrefix(payload, "video|") {
		return b.handleVideoPayment(c, payload, chargeID, amount)

	} else if strings.HasPrefix(payload, "subscribe|") {
		return b.handleSubscribePayment(c, payload, chargeID, amount)
	}

	logger.Warning("Неизвестный тип платежа: %s", payload)
	return c.Send(b.i18nManager.T(c.Sender(), "payment_processed"))
}

// handleVideoPayment обрабатывает платеж за видео
func (b *Bot) handleVideoPayment(c tele.Context, payload, chargeID string, amount int) error {
	url := strings.TrimPrefix(payload, "video|")
	go b.sendVideo(c, url, chargeID, amount)
	return c.Send(b.i18nManager.T(c.Sender(), "payment_accepted"))
}

// handleSubscribePayment обрабатывает платеж за подписку
func (b *Bot) handleSubscribePayment(c tele.Context, payload, chargeID string, amount int) error {
	period := strings.TrimPrefix(payload, "subscribe|")
	return c.Send(b.i18nManager.T(c.Sender(), "subscription_payment_accepted", period))
}

// handleChannelSubscription обрабатывает нажатие кнопки подписки на канал
func (b *Bot) handleChannelSubscription(c tele.Context) error {
	logger := NewLogger("CHANNEL_SUB")

	if b.config.ChannelUsername == "" {
		logger.Warning("ChannelUsername не задан в конфиге")
		return c.Send(b.i18nManager.T(c.Sender(), "channel_not_configured"))
	}

	// Убираем @ если есть
	channelUsername := strings.TrimPrefix(b.config.ChannelUsername, "@")

	// Создаем ссылку на канал
	channelLink := fmt.Sprintf("https://t.me/%s", channelUsername)

	message := b.i18nManager.T(c.Sender(), "subscribe_channel_message", channelLink)

	// Создаем клавиатуру с кнопкой перехода на канал
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{
		{
			{
				Text: b.i18nManager.T(c.Sender(), "go_to_channel"),
				URL:  channelLink,
			},
		},
		{
			{
				Text: b.i18nManager.T(c.Sender(), "check_subscription"),
				Data: "check_subscription",
			},
		},
	}}

	return c.Send(message, markup)
}

// handleCheckSubscription обрабатывает проверку подписки пользователя
func (b *Bot) handleCheckSubscription(c tele.Context) error {
	logger := NewLogger("CHECK_SUB")

	if b.config.ChannelUsername == "" {
		logger.Warning("ChannelUsername не задан в конфиге")
		return c.Send(b.i18nManager.T(c.Sender(), "channel_not_configured"))
	}

	logger.Info("Проверяем подписку пользователя %d на канал %s", c.Sender().ID, b.config.ChannelUsername)

	isSub, err := b.CheckUserSubscriptionRaw(b.config.ChannelUsername, c.Sender().ID)
	if err != nil {
		logger.Warning("Ошибка проверки подписки: %v", err)
		return c.Send(b.i18nManager.T(c.Sender(), "channel_not_configured"))
	}

	if isSub {
		logger.Info("Пользователь %d подписан на канал %s", c.Sender().ID, b.config.ChannelUsername)
		return c.Send(b.i18nManager.T(c.Sender(), "subscribed_success"))
	} else {
		logger.Info("Пользователь %d НЕ подписан на канал %s", c.Sender().ID, b.config.ChannelUsername)
		return c.Send(b.i18nManager.T(c.Sender(), "not_subscribed"))
	}
}

// fixChannelConfig помогает исправить конфигурацию канала
func (b *Bot) fixChannelConfig(c tele.Context) error {
	logger := NewLogger("FIX_CHANNEL")

	currentChannel := b.config.ChannelUsername
	logger.Info("Текущий канал в конфиге: %s", currentChannel)

	if currentChannel == "" {
		return c.Send("❌ CHANNEL_USERNAME не задан в конфиге!\n\nДобавьте в docker-compose.yml:\n``\n- CHANNEL_USERNAME=ваш_канал_без_собачки\n``")
	}

	// Убираем @ если есть
	channelUsername := strings.TrimPrefix(currentChannel, "@")

	message := fmt.Sprintf(`🔧 Диагностика канала:

📋 Текущий канал: %s
🔍 Ищем канал: %s

❌ Ошибка: Канал не найден!

💡 Возможные причины:
1. Канал не существует
2. Неправильное имя канала
3. Бот не добавлен в канал
4. Канал приватный

🛠️ Для исправления:
1. Создайте канал или используйте существующий
2. Добавьте бота как администратора
3. Укажите правильное имя канала в конфиге
4. Перезапустите бота

📝 Пример правильного конфига:
CHANNEL_USERNAME=ваш_канал_без_собачки
`, currentChannel, channelUsername)

	return c.Send(message)
}

// Utility function
func toStr(id int64) string {
	return fmt.Sprintf("%d", id)
}
