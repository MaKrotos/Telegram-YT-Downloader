package bot

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"YoutubeDownloader/internal/payment"
	"YoutubeDownloader/internal/storage"

	tele "gopkg.in/telebot.v4"
)

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
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND] "+chargeID)
				return nil
			}
			b.transactionService.MarkRefunded(chargeID)
			return c.Send("Возврат выполнен для транзакции: " + chargeID)
		}
	}
	// Если не нашли транзакцию — пробуем сделать возврат с пустыми amount и userID
	err := payment.RefundStarPayment(0, chargeID, 0, "Возврат по запросу админа (id не найден)")
	if err != nil {
		b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND] "+chargeID)
		return nil
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
				b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND_USERID] "+chargeID)
				return nil
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
		b.sendError(c, "Произошла ошибка. Попробуйте позже.", err, "[ADMIN_REFUND_USERID] "+chargeID)
		return nil
	}
	return c.Send("Попытка возврата выполнена для транзакции: " + chargeID + " с user_id: " + strconv.FormatInt(userID, 10))
}

// Тестовая функция для отправки инвойса
func (b *Bot) sendTestInvoice(c tele.Context) error {
	log.Printf("[TEST] Отправляем тестовый инвойс")

	invoice := &tele.Invoice{
		Title:       "Тестовый инвойс",
		Description: "Тестирование платежной системы",
		Payload:     "test|123",
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Тест", Amount: 1}},
	}

	log.Printf("[TEST] Тестовый инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[TEST] Ошибка отправки тестового инвойса: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки тестового инвойса: %v", err))
	} else {
		log.Printf("[TEST] Тестовый инвойс отправлен успешно")
	}
	return c.Send("Тестовый инвойс отправлен успешно!")
}

// Тестовая функция для отправки инвойса без PreCheckoutQuery
func (b *Bot) sendDirectInvoice(c tele.Context) error {
	log.Printf("[TEST_DIRECT] Отправляем тестовый инвойс без PreCheckoutQuery")

	invoice := &tele.Invoice{
		Title:       "Тестовый инвойс без PreCheckoutQuery",
		Description: "Тестирование платежной системы без PreCheckoutQuery",
		Payload:     "test_direct|123",
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: "Тест", Amount: 1}},
	}

	log.Printf("[TEST_DIRECT] Тестовый инвойс: %+v", invoice)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		log.Printf("[TEST_DIRECT] Ошибка отправки тестового инвойса без PreCheckoutQuery: %v", err)
		return c.Send(fmt.Sprintf("Ошибка отправки тестового инвойса без PreCheckoutQuery: %v", err))
	} else {
		log.Printf("[TEST_DIRECT] Тестовый инвойс без PreCheckoutQuery отправлен успешно")
	}
	return c.Send("Тестовый инвойс без PreCheckoutQuery отправлен успешно!")
}

// Функция для получения информации о боте
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

// Функция для получения информации об API
func (b *Bot) sendAPIInfo(c tele.Context) error {
	useOfficialAPI := os.Getenv("USE_OFFICIAL_API") == "true"
	apiURL := os.Getenv("TELEGRAM_API_URL")

	var info string
	if useOfficialAPI {
		info = fmt.Sprintf("🌐 Информация об API:\n\n" +
			"✅ Используется ОФИЦИАЛЬНЫЙ Telegram Bot API\n" +
			"URL: https://api.telegram.org\n\n" +
			"💡 Преимущества официального API:\n" +
			"• Полная поддержка всех функций Telegram\n" +
			"• Корректная обработка PreCheckoutQuery\n" +
			"• Стабильная работа платежей\n\n" +
			"⚠️ Ограничения:\n" +
			"• Ограничения на размер файлов (50 МБ)\n" +
			"• Медленная отправка больших файлов\n\n" +
			"🔧 Для переключения на локальный API:\n" +
			"Установите USE_OFFICIAL_API=false в .env")
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
			"Попробуйте официальный API", apiURL)
	}

	return c.Send(info)
}

// Функция для отправки статистики кэша
func (b *Bot) sendCacheStats(c tele.Context) error {
	count, err := storage.GetCacheStats(b.db)
	if err != nil {
		return c.Send(fmt.Sprintf("Ошибка получения статистики кэша: %v", err))
	}

	info := fmt.Sprintf("📊 Статистика кэша:\n\n"+
		"📁 Всего записей в кэше: %d\n\n"+
		"🔧 Команды для управления:\n"+
		"/cache_clean <дни> - удалить записи старше N дней\n"+
		"/cache_clear - очистить весь кэш", count)

	return c.Send(info)
}

// Функция для очистки старого кэша
func (b *Bot) cleanOldCache(c tele.Context, days int) error {
	err := storage.CleanOldCache(b.db, days)
	if err != nil {
		return c.Send(fmt.Sprintf("Ошибка очистки кэша: %v", err))
	}

	return c.Send(fmt.Sprintf("✅ Удалены записи из кэша старше %d дней", days))
}

// Функция для полной очистки кэша
func (b *Bot) clearAllCache(c tele.Context) error {
	// Удаляем все записи из кэша
	query := `DELETE FROM video_cache`
	_, err := b.db.Exec(query)
	if err != nil {
		return c.Send(fmt.Sprintf("Ошибка очистки кэша: %v", err))
	}

	return c.Send("✅ Весь кэш очищен")
}

// Функция для отправки информации об активных скачиваниях
func (b *Bot) sendActiveDownloads(c tele.Context) error {
	b.downloadInfoMutex.RLock()
	defer b.downloadInfoMutex.RUnlock()

	if len(b.activeDownloads) == 0 {
		return c.Send("📊 Активных скачиваний нет")
	}

	var info strings.Builder
	info.WriteString(fmt.Sprintf("📊 Активные скачивания (%d):\n\n", len(b.activeDownloads)))

	for url, downloadInfo := range b.activeDownloads {
		duration := time.Since(downloadInfo.StartTime)
		info.WriteString(fmt.Sprintf("🔗 URL: %s\n", url))
		info.WriteString(fmt.Sprintf("👤 Пользователь: %d\n", downloadInfo.UserID))
		info.WriteString(fmt.Sprintf("🆔 Request ID: %s\n", downloadInfo.RequestID))
		info.WriteString(fmt.Sprintf("⏱️ Длительность: %s\n", duration.Round(time.Second)))
		info.WriteString("---\n")
	}

	return c.Send(info.String())
}
