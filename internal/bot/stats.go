package bot

import (
	"database/sql"
	"time"
)

// Обновить статистику пользователя (увеличить счетчик сообщений, обновить last_active)
func UpdateUserStats(db *sql.DB, userID int64) error {
	_, err := db.Exec(`
		INSERT INTO user_stats (user_id, messages, last_active, created_at)
		VALUES ($1, 1, $2, $2)
		ON CONFLICT (user_id) DO UPDATE SET
			messages = user_stats.messages + 1,
			last_active = $2
	`, userID, time.Now())
	return err
}

// Обновить недельную активность пользователя
func UpdateWeeklyUserActivity(db *sql.DB, userID int64) error {
	_, err := db.Exec(`
		INSERT INTO weekly_user_activity (user_id, activity_date)
		VALUES ($1, $2)
		ON CONFLICT (user_id, activity_date) DO NOTHING
	`, userID, time.Now().Format("2006-01-02"))
	return err
}

// Увеличить общее количество пользователей, если пользователь новый
func IncrementTotalUsersIfNew(db *sql.DB, userID int64) error {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_stats WHERE user_id = $1)`, userID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		_, err := db.Exec(`UPDATE total_stats SET total_users = total_users + 1, updated_at = $1 WHERE id = 1`, time.Now())
		return err
	}
	return nil
}

// Увеличить счетчик сообщений в общей статистике
func IncrementTotalMessages(db *sql.DB) error {
	_, err := db.Exec(`UPDATE total_stats SET total_messages = total_messages + 1, updated_at = $1 WHERE id = 1`, time.Now())
	return err
}

// Увеличить счетчик скачиваний в общей и пользовательской статистике
func IncrementDownloads(db *sql.DB, userID int64) error {
	_, err := db.Exec(`UPDATE user_stats SET downloads = downloads + 1 WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE total_stats SET total_downloads = total_downloads + 1, updated_at = $1 WHERE id = 1`, time.Now())
	return err
}
