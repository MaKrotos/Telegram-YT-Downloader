package bot

import (
	"YoutubeDownloader/internal/downloader"
	"YoutubeDownloader/internal/payment"
	"YoutubeDownloader/internal/storage"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tele "gopkg.in/telebot.v4"
)

// GenerateRequestID генерирует уникальный ID для запроса
func GenerateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// UpdateTransactionStatus обновляет статус транзакции
func UpdateTransactionStatus(db interface{}, chargeID, status string) error {
	sqlDB, ok := db.(*sql.DB)
	if !ok {
		return fmt.Errorf("неверный тип БД")
	}

	// Обновляем статус транзакции по charge_id
	_, err := sqlDB.Exec(`UPDATE transactions SET status = $1, updated_at = NOW() WHERE telegram_payment_charge_id = $2`, status, chargeID)
	if err != nil {
		return fmt.Errorf("ошибка обновления статуса транзакции: %v", err)
	}

	return nil
}

// SaveTransactionToDB сохраняет транзакцию в БД
func SaveTransactionToDB(db interface{}, trx interface{}) (int64, error) {
	sqlDB, ok := db.(*sql.DB)
	if !ok {
		return 0, fmt.Errorf("неверный тип БД")
	}

	transaction, ok := trx.(*payment.Transaction)
	if !ok {
		return 0, fmt.Errorf("неверный тип транзакции")
	}

	// Извлекаем URL из InvoicePayload (формат: "video|URL")
	url := ""
	if strings.HasPrefix(transaction.InvoicePayload, "video|") {
		url = strings.TrimPrefix(transaction.InvoicePayload, "video|")
	} else {
		url = transaction.InvoicePayload // Fallback
	}

	// Создаем pending транзакцию
	id, err := payment.CreatePendingTransaction(sqlDB, transaction.TelegramUserID, transaction.Amount, url)
	if err != nil {
		return 0, fmt.Errorf("ошибка сохранения транзакции: %v", err)
	}

	return id, nil
}

// GetCachedVideo получает видео из кэша
func GetCachedVideo(db interface{}, cacheKey string) (interface{}, error) {
	sqlDB, ok := db.(*sql.DB)
	if !ok {
		return nil, fmt.Errorf("неверный тип БД")
	}

	// Получаем видео из кэша по URL
	cache, err := storage.GetVideoFromCache(sqlDB, cacheKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения видео из кэша: %v", err)
	}

	if cache == nil {
		return nil, nil // Видео не найдено в кэше
	}

	// Создаем объект CachedVideo с file_id от Telegram
	cachedVideo := &CachedVideo{
		FilePath: cache.TelegramFileID, // Это file_id от Telegram, а не путь к файлу
		Title:    "Кэшированное видео",
		FileSize: 0, // Размер неизвестен для кэшированного видео
		Duration: "00:00:00",
	}

	return cachedVideo, nil
}

// SaveVideoToCache сохраняет видео в кэш
func SaveVideoToCache(db interface{}, cacheKey, fileID string) error {
	sqlDB, ok := db.(*sql.DB)
	if !ok {
		return fmt.Errorf("неверный тип БД")
	}

	// Сохраняем file_id от Telegram в кэш
	err := storage.SaveVideoToCache(sqlDB, cacheKey, fileID)
	if err != nil {
		return fmt.Errorf("ошибка сохранения видео в кэш: %v", err)
	}

	return nil
}

// DownloadVideo скачивает видео
func DownloadVideo(url string) (string, error) {
	// Используем реальную функцию скачивания из пакета downloader
	return downloader.DownloadYouTubeVideo(url)
}

// GetVideoInfo получает информацию о видео
func GetVideoInfo(videoPath string) (interface{}, error) {
	// Получаем информацию о файле
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить информацию о файле: %v", err)
	}

	// Получаем имя файла без расширения как заголовок
	fileName := filepath.Base(videoPath)
	title := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// Если название пустое, используем дефолтное
	if title == "" {
		title = "Скачанное видео"
	}

	// Получаем длительность видео (пока заглушка)
	duration := getVideoDuration(videoPath)

	return &VideoInfo{
		Title:    title,
		FileSize: fileInfo.Size(),
		Duration: duration,
	}, nil
}

// getVideoDuration получает длительность видео
func getVideoDuration(videoPath string) string {
	// Пытаемся получить длительность с помощью ffprobe
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries", "format=duration", "-of", "csv=p=0", videoPath)
	output, err := cmd.Output()
	if err != nil {
		// Если ffprobe недоступен, возвращаем заглушку
		return "00:03:30"
	}

	// Парсим длительность из вывода ffprobe
	durationStr := strings.TrimSpace(string(output))
	if durationStr == "" {
		return "00:03:30"
	}

	// Конвертируем секунды в формат HH:MM:SS
	var duration float64
	_, err = fmt.Sscanf(durationStr, "%f", &duration)
	if err != nil {
		return "00:03:30"
	}

	hours := int(duration) / 3600
	minutes := (int(duration) % 3600) / 60
	seconds := int(duration) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// VideoInfo содержит информацию о видео
type VideoInfo struct {
	Title    string
	FileSize int64
	Duration string
}

// CachedVideo содержит информацию о кэшированном видео
type CachedVideo struct {
	FilePath string
	Title    string
	FileSize int64
	Duration string
}

// CheckUserSubscription проверяет подписку пользователя на канал
func CheckUserSubscription(bot interface{}, channelUsername string, userID int64) (bool, error) {
	// Убираем @ если есть
	channelUsername = strings.TrimPrefix(channelUsername, "@")

	// Логируем проверку подписки
	fmt.Printf("[SUBSCRIPTION] Проверяем подписку пользователя %d на канал %s\n", userID, channelUsername)

	// Пытаемся получить реальный bot объект
	teleBot, ok := bot.(*tele.Bot)
	if !ok {
		fmt.Printf("[SUBSCRIPTION] ОШИБКА: неверный тип bot объекта\n")
		return false, fmt.Errorf("неверный тип bot объекта")
	}

	fmt.Printf("[SUBSCRIPTION] Bot объект получен успешно\n")

	// Сначала проверяем, существует ли канал
	fmt.Printf("[SUBSCRIPTION] Ищем канал: %s\n", channelUsername)
	chat, err := teleBot.ChatByUsername(channelUsername)
	if err != nil {
		fmt.Printf("[SUBSCRIPTION] ОШИБКА: не удалось найти канал %s: %v\n", channelUsername, err)
		return false, fmt.Errorf("не удалось найти канал %s: %v (возможно, канал не существует или бот не добавлен)", channelUsername, err)
	}

	fmt.Printf("[SUBSCRIPTION] Канал найден: %s (ID: %d)\n", chat.Username, chat.ID)

	// Проверяем, является ли бот администратором канала
	fmt.Printf("[SUBSCRIPTION] Проверяем права бота в канале\n")
	botMember, err := teleBot.ChatMemberOf(chat, &tele.User{ID: teleBot.Me.ID})
	if err != nil {
		fmt.Printf("[SUBSCRIPTION] ОШИБКА: не удалось проверить права бота в канале %s: %v\n", channelUsername, err)
		return false, fmt.Errorf("не удалось проверить права бота в канале %s: %v", channelUsername, err)
	}

	fmt.Printf("[SUBSCRIPTION] Роль бота в канале: %s\n", botMember.Role)

	// Проверяем, есть ли у бота права на просмотр участников
	if botMember.Role != "administrator" && botMember.Role != "creator" {
		fmt.Printf("[SUBSCRIPTION] ОШИБКА: бот не является администратором канала %s (текущая роль: %s)\n", channelUsername, botMember.Role)
		return false, fmt.Errorf("бот не является администратором канала %s (текущая роль: %s)", channelUsername, botMember.Role)
	}

	// Делаем реальную проверку через Telegram API
	fmt.Printf("[SUBSCRIPTION] Проверяем статус пользователя %d в канале\n", userID)
	chatMember, err := teleBot.ChatMemberOf(chat, &tele.User{ID: userID})
	if err != nil {
		fmt.Printf("[SUBSCRIPTION] ОШИБКА: ошибка проверки подписки пользователя %d в канале %s: %v\n", userID, channelUsername, err)
		return false, fmt.Errorf("ошибка проверки подписки пользователя %d в канале %s: %v", userID, channelUsername, err)
	}

	// Проверяем статус пользователя
	// Статусы: "creator", "administrator", "member" - подписан
	// Статусы: "left", "kicked" - не подписан
	fmt.Printf("[SUBSCRIPTION] Статус пользователя %d в канале %s: %s\n", userID, channelUsername, chatMember.Role)

	switch chatMember.Role {
	case "creator", "administrator", "member":
		fmt.Printf("[SUBSCRIPTION] Пользователь %d ПОДПИСАН на канал %s (статус: %s)\n", userID, channelUsername, chatMember.Role)
		return true, nil
	case "left", "kicked":
		fmt.Printf("[SUBSCRIPTION] Пользователь %d НЕ подписан на канал %s (статус: %s)\n", userID, channelUsername, chatMember.Role)
		return false, nil
	default:
		fmt.Printf("[SUBSCRIPTION] Неизвестный статус пользователя %d в канале %s: %s\n", userID, channelUsername, chatMember.Role)
		return false, fmt.Errorf("неизвестный статус пользователя: %s", chatMember.Role)
	}
}
