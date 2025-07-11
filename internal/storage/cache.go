package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// VideoCache представляет запись в кэше видео
type VideoCache struct {
	ID             int64
	URL            string
	TelegramFileID string
	CreatedAt      time.Time
}

// GetVideoFromCache получает file_id видео из кэша по URL
func GetVideoFromCache(db *sql.DB, url string) (*VideoCache, error) {
	query := `SELECT id, url, telegram_file_id, created_at FROM video_cache WHERE url = $1`

	var cache VideoCache
	err := db.QueryRow(query, url).Scan(&cache.ID, &cache.URL, &cache.TelegramFileID, &cache.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Видео не найдено в кэше
		}
		return nil, fmt.Errorf("ошибка получения видео из кэша: %v", err)
	}

	return &cache, nil
}

// SaveVideoToCache сохраняет file_id видео в кэш
func SaveVideoToCache(db *sql.DB, url, telegramFileID string) error {
	query := `INSERT INTO video_cache (url, telegram_file_id) VALUES ($1, $2) 
			  ON CONFLICT (url) DO UPDATE SET 
			  telegram_file_id = EXCLUDED.telegram_file_id,
			  created_at = NOW()`

	_, err := db.Exec(query, url, telegramFileID)
	if err != nil {
		return fmt.Errorf("ошибка сохранения видео в кэш: %v", err)
	}

	return nil
}

// DeleteVideoFromCache удаляет видео из кэша
func DeleteVideoFromCache(db *sql.DB, url string) error {
	query := `DELETE FROM video_cache WHERE url = $1`

	_, err := db.Exec(query, url)
	if err != nil {
		return fmt.Errorf("ошибка удаления видео из кэша: %v", err)
	}

	return nil
}

// CleanOldCache удаляет старые записи из кэша (старше указанного количества дней)
func CleanOldCache(db *sql.DB, daysOld int) error {
	query := `DELETE FROM video_cache WHERE created_at < NOW() - INTERVAL '$1 days'`

	result, err := db.Exec(query, daysOld)
	if err != nil {
		return fmt.Errorf("ошибка очистки старого кэша: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("[CACHE] Удалено %d старых записей из кэша\n", rowsAffected)

	return nil
}

// GetCacheStats возвращает статистику кэша
func GetCacheStats(db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM video_cache`

	var count int
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения статистики кэша: %v", err)
	}

	return count, nil
}
