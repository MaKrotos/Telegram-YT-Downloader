package bot

import (
	"fmt"
	"strings"
	"time"

	"YoutubeDownloader/internal/payment"
	"YoutubeDownloader/internal/storage"

	tele "gopkg.in/telebot.v4"
)

// sendAdminTransactionsMenu отправляет меню транзакций для админа
func (b *Bot) sendAdminTransactionsMenu(c tele.Context) error {
	logger := NewLogger("ADMIN")

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
				Data: CallbackAdminRefund + "|" + trx.TelegramPaymentChargeID,
			}})
		}
	}

	if len(btns) == 0 {
		return c.Send("Нет транзакций для возврата.")
	}

	markup := &tele.ReplyMarkup{InlineKeyboard: btns}
	logger.Info("Отправлено меню транзакций для админа")
	return c.Send("Транзакции (нажмите для возврата):", markup)
}

// handleAdminRefund обрабатывает возврат средств админом
func (b *Bot) handleAdminRefund(c tele.Context, chargeID string) error {
	logger := NewLogger("ADMIN_REFUND")

	trxs := b.transactionService.GetAllTransactions()
	for _, trx := range trxs {
		if trx.TelegramPaymentChargeID == chargeID {
			// Делаем возврат всегда, независимо от статуса
			err := payment.RefundStarPayment(trx.TelegramUserID, trx.TelegramPaymentChargeID, trx.Amount, "Возврат по запросу админа")
			if err != nil {
				logger.LogErrorWithContext("Ошибка возврата средств", err, chargeID)
				return c.Send(fmt.Sprintf("❌ Возврат НЕ выполнен для транзакции %s\n\nОшибка: %v", chargeID, err))
			}

			b.transactionService.MarkRefunded(chargeID)
			logger.Info("Возврат выполнен для транзакции: %s", chargeID)
			return c.Send(fmt.Sprintf("✅ Возврат УСПЕШНО выполнен для транзакции %s\n\nПользователь: %d\nСумма: %d ⭐", chargeID, trx.TelegramUserID, trx.Amount))
		}
	}

	// Если не нашли транзакцию — пробуем сделать возврат с пустыми amount и userID
	err := payment.RefundStarPayment(0, chargeID, 0, "Возврат по запросу админа (id не найден)")
	if err != nil {
		logger.LogErrorWithContext("Ошибка возврата средств (id не найден)", err, chargeID)
		return c.Send(fmt.Sprintf("❌ Возврат НЕ выполнен для транзакции %s\n\nОшибка: %v\n\nПримечание: Транзакция не найдена в памяти бота", chargeID, err))
	}

	logger.Info("Попытка возврата выполнена для транзакции: %s", chargeID)
	return c.Send(fmt.Sprintf("⚠️ Попытка возврата выполнена для транзакции %s\n\nПримечание: Транзакция не найдена в памяти бота, но возврат отправлен в Telegram", chargeID))
}

// handleAdminRefundWithUserID обрабатывает возврат средств админом с указанием user_id
func (b *Bot) handleAdminRefundWithUserID(c tele.Context, chargeID string, userID int64) error {
	logger := NewLogger("ADMIN_REFUND_USERID")

	trxs := b.transactionService.GetAllTransactions()
	for _, trx := range trxs {
		if trx.TelegramPaymentChargeID == chargeID {
			if userID == 0 {
				userID = trx.TelegramUserID
			}

			err := payment.RefundStarPayment(userID, trx.TelegramPaymentChargeID, trx.Amount, "Возврат по запросу админа")
			if err != nil {
				logger.LogErrorWithContext("Ошибка возврата средств", err, chargeID)
				return c.Send(fmt.Sprintf("❌ Возврат НЕ выполнен для транзакции %s\n\nОшибка: %v\nПользователь: %d", chargeID, err, userID))
			}

			b.transactionService.MarkRefunded(chargeID)
			logger.Info("Возврат выполнен для транзакции: %s", chargeID)
			return c.Send(fmt.Sprintf("✅ Возврат УСПЕШНО выполнен для транзакции %s\n\nПользователь: %d\nСумма: %d ⭐", chargeID, userID, trx.Amount))
		}
	}

	if userID == 0 {
		return c.Send("❌ Возврат невозможен\n\nТранзакция не найдена в памяти бота и user_id не указан")
	}

	err := payment.RefundStarPayment(userID, chargeID, 0, "Возврат по запросу админа (user_id указан вручную)")
	if err != nil {
		logger.LogErrorWithContext("Ошибка возврата средств (user_id указан вручную)", err, chargeID)
		return c.Send(fmt.Sprintf("❌ Возврат НЕ выполнен для транзакции %s\n\nОшибка: %v\nПользователь: %d\n\nПримечание: Транзакция не найдена в памяти бота", chargeID, err, userID))
	}

	logger.Info("Попытка возврата выполнена для транзакции: %s с user_id: %d", chargeID, userID)
	return c.Send(fmt.Sprintf("⚠️ Попытка возврата выполнена для транзакции %s\n\nПользователь: %d\n\nПримечание: Транзакция не найдена в памяти бота, но возврат отправлен в Telegram", chargeID, userID))
}

// sendTestInvoice отправляет тестовый инвойс
func (b *Bot) sendTestInvoice(c tele.Context) error {
	logger := NewLogger("TEST")
	logger.Info("Отправляем тестовый инвойс")

	invoice := &tele.Invoice{
		Title:       "Тестовый инвойс",
		Description: "Тестирование платежной системы",
		Payload:     "test|123",
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Тест", Amount: 1}},
	}

	logger.Info("Тестовый инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		logger.Error("Ошибка отправки тестового инвойса: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки тестового инвойса: %v", err))
	}

	logger.Info("Тестовый инвойс отправлен успешно")
	return c.Send("Тестовый инвойс отправлен успешно!")
}

// sendDirectInvoice отправляет тестовый инвойс без PreCheckoutQuery
func (b *Bot) sendDirectInvoice(c tele.Context) error {
	logger := NewLogger("TEST_DIRECT")
	logger.Info("Отправляем тестовый инвойс без PreCheckoutQuery")

	invoice := &tele.Invoice{
		Title:       "Тестовый инвойс без PreCheckoutQuery",
		Description: "Тестирование платежной системы без PreCheckoutQuery",
		Payload:     "test_direct|123",
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Тест", Amount: 1}},
	}

	logger.Info("Тестовый инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		logger.Error("Ошибка отправки тестового инвойса без PreCheckoutQuery: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки тестового инвойса без PreCheckoutQuery: %v", err))
	}

	logger.Info("Тестовый инвойс без PreCheckoutQuery отправлен успешно")
	return c.Send("Тестовый инвойс без PreCheckoutQuery отправлен успешно!")
}

// sendBotInfo отправляет информацию о боте
func (b *Bot) sendBotInfo(c tele.Context) error {
	info := fmt.Sprintf("🤖 Информация о боте:\n\n" +
		"💡 Для работы с платежами убедитесь, что:\n" +
		"1. Бот создан через @BotFather\n" +
		"2. Включены платежи в настройках бота\n" +
		"3. Используется правильная валюта (XTR)\n\n" +
		"🔧 Команды для тестирования:\n" +
		"/test_invoice - отправить тестовый инвойс\n" +
		"/test_precheckout - инструкции по тестированию\n" +
		"/api_info - информация об API\n\n" +
		"⚠️ Если PreCheckoutQuery не приходит:\n" +
		"1. Проверьте настройки бота в @BotFather\n" +
		"2. Убедитесь, что платежи включены\n" +
		"3. Попробуйте создать нового бота\n" +
		"4. Проверьте версию библиотеки telebot\n" +
		"5. Попробуйте переключиться на официальный API")

	return c.Send(info)
}

// sendAPIInfo отправляет информацию об API
func (b *Bot) sendAPIInfo(c tele.Context) error {
	var info string
	if b.config.UseOfficialAPI {
		info = fmt.Sprintf("🌐 Информация об API:\n\n"+
			"✅ Используется ОФИЦИАЛЬНЫЙ Telegram Bot API\n"+
			"URL: %s\n\n"+
			"💡 Преимущества официального API:\n"+
			"• Полная поддержка всех функций Telegram\n"+
			"• Корректная обработка PreCheckoutQuery\n"+
			"• Стабильная работа платежей\n\n"+
			"⚠️ Ограничения:\n"+
			"• Ограничения на размер файлов (50 МБ)\n"+
			"• Медленная отправка больших файлов\n\n"+
			"🔧 Для переключения на локальный API:\n"+
			"Установите USE_OFFICIAL_API=false в .env", b.config.TelegramAPIURL)
	} else {
		info = fmt.Sprintf("🏠 Информация об API:\n\n"+
			"✅ Используется ЛОКАЛЬНЫЙ Telegram Bot API\n"+
			"URL: %s\n\n"+
			"💡 Преимущества локального API:\n"+
			"• Поддержка больших файлов (до 2 ГБ)\n"+
			"• Быстрая отправка файлов\n"+
			"• Нет ограничений на размер\n\n"+
			"⚠️ Возможные проблемы:\n"+
			"• Неполная поддержка PreCheckoutQuery\n"+
			"• Проблемы с платежами Telegram Stars\n"+
			"• Нестабильная работа некоторых функций\n\n"+
			"🔧 Для переключения на официальный API:\n"+
			"Установите USE_OFFICIAL_API=true в .env\n\n"+
			"💡 Рекомендация для тестирования платежей:\n"+
			"Попробуйте официальный API", b.config.TelegramAPIURL)
	}

	return c.Send(info)
}

// sendCacheStats отправляет статистику кэша
func (b *Bot) sendCacheStats(c tele.Context) error {
	logger := NewLogger("CACHE")

	count, err := storage.GetCacheStats(b.db)
	if err != nil {
		logger.Error("Ошибка получения статистики кэша: %v", err)
		return c.Send("Ошибка получения статистики кэша")
	}

	info := fmt.Sprintf("📊 Статистика кэша:\n\n"+
		"📁 Всего записей в кэше: %d\n\n"+
		"🔧 Команды для управления:\n"+
		"/cache_clean <дни> - удалить записи старше N дней\n"+
		"/cache_clear - очистить весь кэш", count)

	return c.Send(info)
}

// cleanOldCache очищает старые файлы кэша
func (b *Bot) cleanOldCache(c tele.Context, days int) error {
	logger := NewLogger("CACHE")

	err := storage.CleanOldCache(b.db, days)
	if err != nil {
		logger.Error("Ошибка очистки кэша: %v", err)
		return c.Send("Ошибка очистки кэша")
	}

	logger.Info("Очищены записи кэша старше %d дней", days)
	return c.Send(fmt.Sprintf("✅ Удалены записи из кэша старше %d дней", days))
}

// clearAllCache очищает весь кэш
func (b *Bot) clearAllCache(c tele.Context) error {
	logger := NewLogger("CACHE")

	// Удаляем все записи из кэша
	query := `DELETE FROM video_cache`
	_, err := b.db.Exec(query)
	if err != nil {
		logger.Error("Ошибка полной очистки кэша: %v", err)
		return c.Send("Ошибка очистки кэша")
	}

	logger.Info("Полностью очищен кэш")
	return c.Send("✅ Весь кэш очищен")
}

// sendActiveDownloads отправляет информацию об активных скачиваниях
func (b *Bot) sendActiveDownloads(c tele.Context) error {
	logger := NewLogger("DOWNLOADS")

	activeDownloads := b.downloadManager.GetActiveDownloads()
	if len(activeDownloads) == 0 {
		return c.Send("Активных скачиваний нет.")
	}

	var info strings.Builder
	info.WriteString(fmt.Sprintf("📥 Активные скачивания (%d):\n\n", len(activeDownloads)))

	for url, downloadInfo := range activeDownloads {
		duration := time.Since(downloadInfo.StartTime)
		info.WriteString(fmt.Sprintf("🔗 %s\n", url))
		info.WriteString(fmt.Sprintf("👤 Пользователь: %d\n", downloadInfo.UserID))
		info.WriteString(fmt.Sprintf("🆔 Request ID: %s\n", downloadInfo.RequestID))
		info.WriteString(fmt.Sprintf("⏱️ Время: %v\n\n", duration))
	}

	logger.Info("Отправлена информация об %d активных скачиваниях", len(activeDownloads))
	return c.Send(info.String())
}

// formatBytesAdmin форматирует размер в байтах в читаемый вид (для админских функций)
func formatBytesAdmin(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// testSubscription тестирует проверку подписки
func (b *Bot) testSubscription(c tele.Context) error {
	logger := NewLogger("TEST_SUBSCRIPTION")

	if b.config.ChannelUsername == "" {
		return c.Send("❌ Канал не настроен в конфигурации")
	}

	userID := c.Sender().ID
	logger.Info("Тестируем проверку подписки для пользователя %d на канал %s", userID, b.config.ChannelUsername)

	// Тестируем проверку подписки
	isSub, err := b.CheckUserSubscriptionRaw(b.config.ChannelUsername, userID)

	if err != nil {
		errorMsg := fmt.Sprintf("❌ Ошибка проверки подписки:\n\n%s\n\n💡 Возможные причины:\n• Бот не добавлен в канал\n• Бот не является администратором\n• Неправильное имя канала\n• Канал приватный", err.Error())
		return c.Send(errorMsg)
	}

	if isSub {
		return c.Send(fmt.Sprintf("✅ Пользователь %d подписан на канал %s", userID, b.config.ChannelUsername))
	} else {
		return c.Send(fmt.Sprintf("❌ Пользователь %d НЕ подписан на канал %s", userID, b.config.ChannelUsername))
	}
}

// testChannel тестирует доступ к каналу
func (b *Bot) testChannel(c tele.Context) error {
	logger := NewLogger("TEST_CHANNEL")

	if b.config.ChannelUsername == "" {
		return c.Send("❌ Канал не настроен в конфигурации")
	}

	logger.Info("Тестируем доступ к каналу %s", b.config.ChannelUsername)

	// Пытаемся получить информацию о канале
	chat, err := b.api.ChatByUsername(b.config.ChannelUsername)
	if err != nil {
		errorMsg := fmt.Sprintf("❌ Не удалось найти канал %s:\n\n%s\n\n💡 Решения:\n• Добавьте бота в канал\n• Проверьте правильность имени канала\n• Убедитесь, что канал публичный", b.config.ChannelUsername, err.Error())
		return c.Send(errorMsg)
	}

	// Пытаемся получить информацию о боте в канале
	botMember, err := b.api.ChatMemberOf(chat, &tele.User{ID: b.api.Me.ID})
	if err != nil {
		errorMsg := fmt.Sprintf("⚠️ Канал найден, но не удалось проверить права бота:\n\n%s\n\n💡 Возможные причины:\n• Бот не добавлен в канал\n• Недостаточно прав у бота", err.Error())
		return c.Send(errorMsg)
	}

	info := fmt.Sprintf("✅ Канал найден:\n\n📢 Название: %s\n🆔 ID: %d\n👤 Тип: %s\n\n🤖 Роль бота: %s\n\n💡 Статус: %s",
		chat.Title, chat.ID, chat.Type, botMember.Role,
		func() string {
			if botMember.Role == "administrator" || botMember.Role == "creator" {
				return "✅ Бот может проверять подписки"
			} else {
				return "❌ Бот не может проверять подписки (нужны права администратора)"
			}
		}())

	return c.Send(info)
}

// showConfig показывает текущую конфигурацию бота
func (b *Bot) showConfig(c tele.Context) error {
	logger := NewLogger("CONFIG")

	info := fmt.Sprintf("⚙️ Конфигурация бота:\n\n"+
		"🤖 Admin ID: %s\n"+
		"📢 Channel Username: %s\n"+
		"🌐 Use Official API: %t\n"+
		"🔗 API URL: %s\n"+
		"👥 Max Workers: %d\n"+
		"⏱️ HTTP Timeout: %v\n"+
		"📥 Download Timeout: %v",
		b.config.AdminID,
		b.config.ChannelUsername,
		b.config.UseOfficialAPI,
		b.config.TelegramAPIURL,
		b.config.MaxWorkers,
		b.config.HTTPTimeout,
		b.config.DownloadTimeout)

	logger.Info("Показана конфигурация бота")
	return c.Send(info)
}
