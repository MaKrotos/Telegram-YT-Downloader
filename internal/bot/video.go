package bot

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"YoutubeDownloader/internal/payment"
	"YoutubeDownloader/internal/storage"

	tele "gopkg.in/telebot.v4"
)

// sendUniversalPayKeyboard отправляет универсальную платежную клавиатуру
func (b *Bot) sendUniversalPayKeyboard(c tele.Context, url string) error {
	logger := NewLogger("PAYMENT")

	// Создаем транзакцию для видео
	trx := &payment.Transaction{
		InvoicePayload:          "video|" + url,
		Amount:                  1, // 1 XTR
		TelegramUserID:          c.Sender().ID,
		Status:                  "pending",
		TelegramPaymentChargeID: "",
	}

	// Сохраняем транзакцию в БД
	id, err := SaveTransactionToDB(b.db, trx)
	if err != nil {
		logger.Error("Ошибка сохранения транзакции: %v", err)
		return c.Send(b.i18nManager.T(c.Sender(), "payment_error"))
	}

	// Создаем инлайн клавиатуру
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{
		{
			{
				Text: b.i18nManager.T(c.Sender(), "pay_1_star"),
				Data: CallbackPayVideo + "|" + strconv.FormatInt(id, 10),
			},
		},
	}}

	logger.Info("Отправлена платежная клавиатура для URL: %s", url)
	return c.Send(b.i18nManager.T(c.Sender(), "payment_required"), markup)
}

// sendPaymentKeyboardWithSubscriptions отправляет платежную клавиатуру с опциями подписки
func (b *Bot) sendPaymentKeyboardWithSubscriptions(c tele.Context, url string) error {
	logger := NewLogger("PAYMENT")

	// Создаем транзакцию для видео
	trx := &payment.Transaction{
		InvoicePayload:          "video|" + url,
		Amount:                  1, // 1 XTR
		TelegramUserID:          c.Sender().ID,
		Status:                  "pending",
		TelegramPaymentChargeID: "",
	}

	// Сохраняем транзакцию в БД
	id, err := SaveTransactionToDB(b.db, trx)
	if err != nil {
		logger.Error("Ошибка сохранения транзакции: %v", err)
		return c.Send(b.i18nManager.T(c.Sender(), "payment_error"))
	}

	// Создаем инлайн клавиатуру с опциями подписки
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{
		{
			{
				Text: b.i18nManager.T(c.Sender(), "subscribe_free"),
				Data: "subscribe_channel",
			},
		},
		{
			{
				Text: b.i18nManager.T(c.Sender(), "pay_1_star"),
				Data: CallbackPayVideo + "|" + strconv.FormatInt(id, 10),
			},
		},
		{
			{
				Text: b.i18nManager.T(c.Sender(), "monthly_subscription"),
				Data: CallbackPaySubscribe,
			},
		},
		{
			{
				Text: b.i18nManager.T(c.Sender(), "yearly_subscription"),
				Data: CallbackPaySubscribeYear,
			},
		},
		{
			{
				Text: b.i18nManager.T(c.Sender(), "forever_subscription"),
				Data: CallbackPaySubscribeForever,
			},
		},
	}}

	message := b.i18nManager.T(c.Sender(), "payment_options_message")

	logger.Info("Отправлена платежная клавиатура с подписками для URL: %s", url)
	return c.Send(message, markup)
}

// sendVideoInvoiceByDB отправляет инвойс для видео из БД
func (b *Bot) sendVideoInvoiceByDB(c tele.Context, trx *payment.Transaction) error {
	logger := NewLogger("INVOICE")

	invoice := &tele.Invoice{
		Title:       b.i18nManager.T(c.Sender(), "video_download_title"),
		Description: b.i18nManager.T(c.Sender(), "video_download_description"),
		Payload:     trx.InvoicePayload,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: b.i18nManager.T(c.Sender(), "download_star_label"), Amount: trx.Amount}},
	}

	logger.Info("Отправляем инвойс для видео: %s", trx.InvoicePayload)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		logger.Error("Ошибка отправки инвойса: %v", err)
		return c.Send(b.i18nManager.T(c.Sender(), "invoice_error", err))
	}

	return nil
}

// sendSubscribeInvoice отправляет инвойс для подписки
func (b *Bot) sendSubscribeInvoice(c tele.Context, period string) error {
	logger := NewLogger("SUBSCRIBE")

	var title, description string
	var amount int

	switch period {
	case "month":
		title = b.i18nManager.T(c.Sender(), "subscription_month")
		description = b.i18nManager.T(c.Sender(), "subscription_month_desc")
		amount = 100
	case "year":
		title = b.i18nManager.T(c.Sender(), "subscription_year")
		description = b.i18nManager.T(c.Sender(), "subscription_year_desc")
		amount = 500
	case "forever":
		title = b.i18nManager.T(c.Sender(), "subscription_forever")
		description = b.i18nManager.T(c.Sender(), "subscription_forever_desc")
		amount = 1000
	default:
		return c.Send(b.i18nManager.T(c.Sender(), "unknown_subscription"))
	}

	invoice := &tele.Invoice{
		Title:       title,
		Description: description,
		Payload:     "subscribe|" + period,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: title + " ⭐", Amount: amount}},
	}

	logger.Info("Отправляем инвойс для подписки: %s (%d XTR)", period, amount)

	// Для Telegram Stars отправляем без provider token
	_, err := b.api.Send(c.Sender(), invoice)
	if err != nil {
		logger.Error("Ошибка отправки инвойса подписки: %v", err)
		return c.Send(b.i18nManager.T(c.Sender(), "invoice_error", err))
	}

	return nil
}

// sendVideoWithRetry отправляет видео с повторными попытками
func (b *Bot) sendVideoWithRetry(c tele.Context, video *tele.Video, url string, maxRetries int) error {
	logger := NewLogger("VIDEO_SEND")

	for i := 0; i < maxRetries; i++ {
		err := c.Send(video)
		if err == nil {
			logger.Info("Видео успешно отправлено с попытки %d", i+1)
			return nil
		}

		logger.Warning("Попытка %d отправки видео не удалась: %v", i+1, err)
		if i < maxRetries-1 {
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}

	logger.Error("Все попытки отправки видео не удались")
	return fmt.Errorf("не удалось отправить видео после %d попыток", maxRetries)
}

// sendVideo обрабатывает скачивание и отправку видео
func (b *Bot) sendVideo(c tele.Context, url string, chargeID string, amount int) {
	logger := NewLogger("VIDEO")
	startTime := time.Now()

	logger.Info("Начинаем скачивание видео: %s", url)

	// Проверяем, не скачивается ли уже это видео
	if b.downloadManager.IsDownloadActive(url) {
		logger.Info("Видео уже скачивается, ожидаем завершения")
		c.Send(b.i18nManager.T(c.Sender(), "download_in_progress"))
		downloadInfo, err := b.downloadManager.WaitForDownload(url, b.config.DownloadTimeout)
		if err != nil {
			logger.Error("Ошибка ожидания скачивания: %v", err)
			c.Send(b.i18nManager.T(c.Sender(), "download_wait_error"))
			return
		}
		if downloadInfo != nil && downloadInfo.Error != nil {
			logger.Error("Скачивание завершилось с ошибкой: %v", downloadInfo.Error)
			c.Send(b.i18nManager.T(c.Sender(), "download_error", downloadInfo.Error.Error()))
			return
		}
	}

	// Получаем слот для скачивания
	if !b.downloadManager.AcquireDownloadSlot() {
		logger.Warning("Нет свободных слотов для скачивания")
		c.Send(b.i18nManager.T(c.Sender(), "too_many_requests"))
		return
	}
	defer b.downloadManager.ReleaseDownloadSlot()

	// Получаем мьютекс для URL
	mutex := b.downloadManager.GetURLMutex(url)
	mutex.Lock()
	defer func() {
		mutex.Unlock()
		b.downloadManager.CleanupURLMutex(url)
	}()

	// Регистрируем начало скачивания
	requestID := GenerateRequestID()
	_ = b.downloadManager.StartDownload(url, requestID, c.Sender().ID)
	defer b.downloadManager.FinishDownload(url, nil)

	// Создать уникальный ключ кэша на основе URL
	cacheKey := url

	// Проверяем кэш для конкретного URL
	logger.Info("Проверяем кэш для URL: %s (ключ: %s)", url, cacheKey)
	cachedVideo, err := GetCachedVideo(b.db, cacheKey)
	if err != nil {
		logger.Warning("Ошибка получения из кэша: %v", err)
	}

	if cachedVideo != nil {
		// Приведение типа для работы с кэшированным видео
		if cached, ok := cachedVideo.(*CachedVideo); ok {
			logger.Info("Найдено видео в кэше с file_id: %s для URL: %s", cached.FilePath, url)

			// Для кэшированного видео используем file_id от Telegram
			video := &tele.Video{
				File: tele.File{FileID: cached.FilePath}, // Используем FileID для кэшированного видео
			}

			// Отправляем кэшированное видео напрямую
			logger.Info("Отправляем кэшированное видео с file_id: %s для URL: %s", cached.FilePath, url)
			sentMessage, err := b.api.Send(c.Sender(), video)
			if err != nil {
				logger.Error("Ошибка отправки кэшированного видео: %v", err)
				// Если отправка по file_id не удалась, удаляем из кэша и скачиваем заново
				logger.Info("Удаляем недействительную запись из кэша для URL: %s", url)
				DeleteVideoFromCache(b.db, cacheKey)
				// Продолжаем со скачиванием
			} else {
				logger.Info("Кэшированное видео успешно отправлено для URL: %s!", url)
				logger.LogPerformance("Отправка кэшированного видео", startTime)

				// Дополнительная проверка - если получили file_id, сравниваем с кэшированным
				if sentMessage != nil && sentMessage.Video != nil && sentMessage.Video.FileID != "" {
					if sentMessage.Video.FileID != cached.FilePath {
						logger.Warning("File_id изменился! Кэшированный: %s, Полученный: %s", cached.FilePath, sentMessage.Video.FileID)
						logger.Info("Обновляем file_id в кэше для URL: %s", url)
						SaveVideoToCache(b.db, cacheKey, sentMessage.Video.FileID)
					}
				}

				// Обновляем статистику транзакции
				if chargeID != "" {
					err = UpdateTransactionStatus(b.db, chargeID, "completed")
					if err != nil {
						logger.Error("Ошибка обновления статуса транзакции: %v", err)
					}
				}

				// --- СТАТИСТИКА: увеличиваем счетчик скачиваний ---
				_ = IncrementDownloads(b.db, c.Sender().ID)
				// --- КОНЕЦ СТАТИСТИКИ ---

				return
			}
		}
	}

	// Уведомляем пользователя о начале скачивания (только если видео не в кэше)
	c.Send(b.i18nManager.T(c.Sender(), "download_started"))

	// Скачиваем видео
	logger.Info("Скачиваем видео: %s", url)

	// Создаем уникальный путь для каждого скачивания
	uniqueVideoPath, err := DownloadVideo(url)
	if err != nil {
		logger.Error("Ошибка скачивания видео: %v", err)
		b.downloadManager.FinishDownload(url, err)
		c.Send(b.i18nManager.T(c.Sender(), "download_error", err.Error()))
		return
	}

	// Проверяем, что файл действительно существует и уникален
	if uniqueVideoPath == "" {
		logger.Error("Получен пустой путь к видео")
		b.downloadManager.FinishDownload(url, fmt.Errorf("пустой путь к видео"))
		c.Send(b.i18nManager.T(c.Sender(), "download_error", "пустой путь к видео"))
		return
	}

	// Проверяем, что файл существует
	if _, err := os.Stat(uniqueVideoPath); os.IsNotExist(err) {
		logger.Error("Файл не существует по пути: %s", uniqueVideoPath)
		b.downloadManager.FinishDownload(url, fmt.Errorf("файл не существует"))
		c.Send(b.i18nManager.T(c.Sender(), "download_error", "файл не существует"))
		return
	}

	logger.Info("Видео скачано по пути: %s", uniqueVideoPath)

	// Получаем информацию о видео
	videoInfo, err := GetVideoInfo(uniqueVideoPath)
	if err != nil {
		logger.Error("Ошибка получения информации о видео: %v", err)
		b.downloadManager.FinishDownload(url, err)
		c.Send(b.i18nManager.T(c.Sender(), "download_error", err.Error()))
		return
	}

	// Отправляем видео
	if _, ok := videoInfo.(*VideoInfo); ok {
		fileSize := videoInfo.(*VideoInfo).FileSize
		duration := videoInfo.(*VideoInfo).Duration

		logger.Info("Информация о скачанном видео: URL=%s, размер=%d байт, длительность=%s", url, fileSize, duration)

		// ДОПОЛНИТЕЛЬНАЯ ПРОВЕРКА: проверяем, не является ли это дубликатом уже скачанного файла
		// Получаем хеш файла для проверки уникальности
		fileHash, err := getFileHash(uniqueVideoPath)
		if err != nil {
			logger.Warning("Не удалось получить хеш файла: %v", err)
		} else {
			logger.Info("Хеш файла для URL %s: %s", url, fileHash)

			// Проверяем, есть ли уже файл с таким хешем в кэше
			existingURL, err := getURLByFileHash(b.db, fileHash)
			if err != nil {
				logger.Warning("Ошибка проверки дубликата по хешу: %v", err)
			} else if existingURL != "" && existingURL != url {
				logger.Warning("ОБНАРУЖЕН ДУБЛИКАТ! Файл для URL %s уже существует для URL %s", url, existingURL)
				c.Send(fmt.Sprintf("⚠️ Обнаружен дубликат файла! Возможно, YouTube возвращает одинаковое видео для разных ссылок.\n\nПопробуйте:\n1. Очистить кэш: /clear_all_cache\n2. Использовать другие ссылки\n3. Проверить содержимое кэша: /show_cache"))
				return
			}
		}

		// Проверяем размер файла
		if fileSize > 50*1024*1024 { // 50 МБ
			logger.Warning("Видео слишком большое (%d байт), может не отправиться через Telegram API", fileSize)
			c.Send(b.i18nManager.T(c.Sender(), "file_too_large"))
		}

		video := &tele.Video{
			File: tele.FromDisk(uniqueVideoPath),
		}

		logger.Info("Пытаемся отправить видео размером %d байт для URL: %s", fileSize, url)

		// Отправляем видео напрямую через API для получения file_id
		sentMessage, err := b.api.Send(c.Sender(), video)
		if err != nil {
			logger.Error("Ошибка отправки видео: %v", err)
			b.downloadManager.FinishDownload(url, err)
			c.Send(b.i18nManager.T(c.Sender(), "send_error", err))
			return
		}

		logger.Info("Видео успешно отправлено для URL: %s! sentMessage: %+v", url, sentMessage)

		// Сохраняем file_id в кэш, если видео было отправлено
		if sentMessage != nil && sentMessage.Video != nil && sentMessage.Video.FileID != "" {
			logger.Info("Сохраняем file_id в кэш: %s для URL: %s", sentMessage.Video.FileID, url)

			// Логируем все доступные поля видео
			logger.Info("Video info - FileID: %s, FileSize: %d, Duration: %d, Width: %d, Height: %d",
				sentMessage.Video.FileID,
				sentMessage.Video.FileSize,
				sentMessage.Video.Duration,
				sentMessage.Video.Width,
				sentMessage.Video.Height)

			// Сохраняем также хеш файла для проверки дубликатов
			if fileHash != "" {
				err = saveFileHashToCache(b.db, url, fileHash)
				if err != nil {
					logger.Warning("Ошибка сохранения хеша файла в кэш: %v", err)
				} else {
					logger.Info("Хеш файла сохранен в кэш для URL: %s", url)
				}
			}

			err = SaveVideoToCache(b.db, cacheKey, sentMessage.Video.FileID)
			if err != nil {
				logger.Warning("Ошибка сохранения file_id в кэш: %v", err)
			} else {
				logger.Info("File_id успешно сохранен в кэш для URL: %s", url)
			}
		} else {
			logger.Warning("Не удалось получить file_id для сохранения в кэш для URL: %s", url)
		}

		// Обновляем статистику транзакции
		if chargeID != "" {
			err = UpdateTransactionStatus(b.db, chargeID, "completed")
			if err != nil {
				logger.Error("Ошибка обновления статуса транзакции: %v", err)
			}
		}

		// --- СТАТИСТИКА: увеличиваем счетчик скачиваний ---
		_ = IncrementDownloads(b.db, c.Sender().ID)
		// --- КОНЕЦ СТАТИСТИКИ ---

		logger.LogPerformance("Полное скачивание и отправка видео", startTime)
	}

	// ПРИНУДИТЕЛЬНАЯ ОЧИСТКА: удаляем временный файл после отправки
	if uniqueVideoPath != "" {
		logger.Info("Удаляем временный файл: %s", uniqueVideoPath)
		if err := os.Remove(uniqueVideoPath); err != nil {
			logger.Warning("Не удалось удалить временный файл %s: %v", uniqueVideoPath, err)
		} else {
			logger.Info("Временный файл успешно удален: %s", uniqueVideoPath)
		}
	}
}

// CheckUserSubscriptionRaw проверяет подписку пользователя на канал через Telegram API
func (b *Bot) CheckUserSubscriptionRaw(channelUsername string, userID int64) (bool, error) {
	// channelUsername должен быть в формате "@yourchannel" или chat_id
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", b.api.Token)

	// Если нет @, добавим
	if !strings.HasPrefix(channelUsername, "@") && !strings.HasPrefix(channelUsername, "-") {
		channelUsername = "@" + channelUsername
	}

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

// sendError отправляет сообщение об ошибке
func (b *Bot) sendError(c tele.Context, userMsg string, err error, extraInfo ...string) {
	logger := NewLogger("ERROR")

	info := ""
	if len(extraInfo) > 0 {
		info = extraInfo[0]
	}
	logger.LogErrorWithContext(userMsg, err, info)

	c.Send(userMsg)
}

// showCacheContents показывает содержимое кэша
func (b *Bot) showCacheContents(c tele.Context) error {
	logger := NewLogger("CACHE_VIEW")

	// Получаем статистику кэша
	count, err := storage.GetCacheStats(b.db)
	if err != nil {
		logger.Error("Ошибка получения статистики кэша: %v", err)
		return c.Send(fmt.Sprintf("Ошибка получения статистики кэша: %v", err))
	}

	// Получаем все записи кэша
	rows, err := b.db.Query(`SELECT url, telegram_file_id, created_at FROM video_cache ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		logger.Error("Ошибка получения содержимого кэша: %v", err)
		return c.Send(fmt.Sprintf("Ошибка получения содержимого кэша: %v", err))
	}
	defer rows.Close()

	var cacheInfo strings.Builder
	cacheInfo.WriteString(fmt.Sprintf("📊 **Статистика кэша:** %d записей\n\n", count))

	found := false
	for rows.Next() {
		var url, fileID, createdAt string
		err := rows.Scan(&url, &fileID, &createdAt)
		if err != nil {
			continue
		}

		// Обрезаем длинные URL для читаемости
		shortURL := url
		if len(shortURL) > 50 {
			shortURL = shortURL[:47] + "..."
		}

		// Обрезаем fileID для читаемости
		shortFileID := fileID
		if len(shortFileID) > 20 {
			shortFileID = shortFileID[:17] + "..."
		}

		cacheInfo.WriteString(fmt.Sprintf("🔗 **URL:** %s\n", shortURL))
		cacheInfo.WriteString(fmt.Sprintf("📁 **File ID:** %s\n", shortFileID))
		cacheInfo.WriteString(fmt.Sprintf("⏰ **Создан:** %s\n", createdAt))
		cacheInfo.WriteString("---\n")
		found = true
	}

	if !found {
		cacheInfo.WriteString("Кэш пуст")
	}

	return c.Send(cacheInfo.String())
}

// getFileHash получает MD5 хеш файла
func getFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("ошибка открытия файла: %w", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("ошибка чтения файла: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// getURLByFileHash получает URL по хешу файла из кэша
func getURLByFileHash(db *sql.DB, fileHash string) (string, error) {
	query := `SELECT url FROM file_hash_cache WHERE file_hash = $1 LIMIT 1`

	var url string
	err := db.QueryRow(query, fileHash).Scan(&url)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Хеш не найден
		}
		return "", fmt.Errorf("ошибка поиска URL по хешу: %w", err)
	}

	return url, nil
}

// saveFileHashToCache сохраняет хеш файла в кэш
func saveFileHashToCache(db *sql.DB, url, fileHash string) error {
	query := `INSERT INTO file_hash_cache (url, file_hash, created_at) VALUES ($1, $2, NOW()) 
			  ON CONFLICT (url) DO UPDATE SET 
			  file_hash = EXCLUDED.file_hash,
			  created_at = NOW()`

	_, err := db.Exec(query, url, fileHash)
	if err != nil {
		return fmt.Errorf("ошибка сохранения хеша файла в кэш: %w", err)
	}

	return nil
}

// checkFileDuplicates проверяет дубликаты файлов в кэше
func (b *Bot) checkFileDuplicates(c tele.Context) error {
	logger := NewLogger("DUPLICATES")

	// Получаем все дубликаты по хешу файла
	rows, err := b.db.Query(`
		SELECT fhc1.url as url1, fhc1.file_hash, fhc1.created_at as created1,
		       fhc2.url as url2, fhc2.created_at as created2
		FROM file_hash_cache fhc1
		JOIN file_hash_cache fhc2 ON fhc1.file_hash = fhc2.file_hash 
			AND fhc1.url < fhc2.url
		ORDER BY fhc1.created_at DESC
	`)
	if err != nil {
		logger.Error("Ошибка проверки дубликатов: %v", err)
		return c.Send(fmt.Sprintf("Ошибка проверки дубликатов: %v", err))
	}
	defer rows.Close()

	var duplicatesInfo strings.Builder
	duplicatesInfo.WriteString("🔍 **Проверка дубликатов файлов**\n\n")

	found := false
	for rows.Next() {
		var url1, fileHash, created1, url2, created2 string
		err := rows.Scan(&url1, &fileHash, &created1, &url2, &created2)
		if err != nil {
			continue
		}

		// Обрезаем длинные URL для читаемости
		shortURL1 := url1
		if len(shortURL1) > 40 {
			shortURL1 = shortURL1[:37] + "..."
		}

		shortURL2 := url2
		if len(shortURL2) > 40 {
			shortURL2 = shortURL2[:37] + "..."
		}

		// Обрезаем хеш для читаемости
		shortHash := fileHash
		if len(shortHash) > 16 {
			shortHash = shortHash[:16] + "..."
		}

		duplicatesInfo.WriteString(fmt.Sprintf("⚠️ **ДУБЛИКАТ НАЙДЕН!**\n"))
		duplicatesInfo.WriteString(fmt.Sprintf("🔗 **URL 1:** %s\n", shortURL1))
		duplicatesInfo.WriteString(fmt.Sprintf("🔗 **URL 2:** %s\n", shortURL2))
		duplicatesInfo.WriteString(fmt.Sprintf("📁 **Хеш:** %s\n", shortHash))
		duplicatesInfo.WriteString(fmt.Sprintf("⏰ **Создан 1:** %s\n", created1))
		duplicatesInfo.WriteString(fmt.Sprintf("⏰ **Создан 2:** %s\n", created2))
		duplicatesInfo.WriteString("---\n")
		found = true
	}

	if !found {
		duplicatesInfo.WriteString("✅ Дубликаты файлов не найдены!")
	} else {
		duplicatesInfo.WriteString("\n💡 **Рекомендации:**\n")
		duplicatesInfo.WriteString("• Очистите кэш: /clear_all_cache\n")
		duplicatesInfo.WriteString("• Проверьте содержимое кэша: /show_cache\n")
		duplicatesInfo.WriteString("• Возможно, YouTube возвращает одинаковые файлы для разных ссылок")
	}

	return c.Send(duplicatesInfo.String())
}

// cleanupTempFiles очищает все временные файлы
func (b *Bot) cleanupTempFiles(c tele.Context) error {
	logger := NewLogger("CLEANUP_TEMP")

	tmpDir := "./tmp"
	logger.Info("Очищаем временные файлы в директории: %s", tmpDir)

	// Получаем список всех файлов в tmp директории
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		logger.Error("Ошибка чтения директории %s: %v", tmpDir, err)
		return c.Send(fmt.Sprintf("Ошибка чтения директории временных файлов: %v", err))
	}

	deletedCount := 0
	totalSize := int64(0)

	for _, file := range files {
		if !file.IsDir() {
			filePath := filepath.Join(tmpDir, file.Name())

			// Получаем размер файла
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				logger.Warning("Не удалось получить информацию о файле %s: %v", filePath, err)
				continue
			}

			// Удаляем файл
			if err := os.Remove(filePath); err != nil {
				logger.Warning("Не удалось удалить файл %s: %v", filePath, err)
			} else {
				logger.Info("Удален временный файл: %s (размер: %d байт)", file.Name(), fileInfo.Size())
				deletedCount++
				totalSize += fileInfo.Size()
			}
		}
	}

	message := fmt.Sprintf("🧹 **Очистка временных файлов завершена!**\n\n📊 **Результат:**\n• Удалено файлов: %d\n• Освобождено места: %d байт (%.2f МБ)\n\n💡 Теперь каждое новое скачивание будет использовать уникальные пути!",
		deletedCount, totalSize, float64(totalSize)/1024/1024)

	logger.Info("Очистка завершена: удалено %d файлов, освобождено %d байт", deletedCount, totalSize)
	return c.Send(message)
}
