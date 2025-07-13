package bot

import (
	"fmt"
	"log"
	"time"

	tele "gopkg.in/telebot.v4"
)

// Logger предоставляет структурированное логирование
type Logger struct {
	prefix string
}

// NewLogger создает новый логгер с префиксом
func NewLogger(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

// Info логирует информационное сообщение
func (l *Logger) Info(format string, args ...interface{}) {
	log.Printf("[%s] %s", l.prefix, fmt.Sprintf(format, args...))
}

// Error логирует ошибку
func (l *Logger) Error(format string, args ...interface{}) {
	log.Printf("[%s] ERROR: %s", l.prefix, fmt.Sprintf(format, args...))
}

// Debug логирует отладочное сообщение
func (l *Logger) Debug(format string, args ...interface{}) {
	log.Printf("[%s] DEBUG: %s", l.prefix, fmt.Sprintf(format, args...))
}

// Warning логирует предупреждение
func (l *Logger) Warning(format string, args ...interface{}) {
	log.Printf("[%s] WARNING: %s", l.prefix, fmt.Sprintf(format, args...))
}

// LogUpdate логирует обновление Telegram
func (l *Logger) LogUpdate(update *tele.Update) {
	switch {
	case update.Message != nil:
		l.Info("Message: user_id=%d, text=%q", update.Message.Sender.ID, update.Message.Text)
	case update.Callback != nil:
		l.Info("CallbackQuery: user_id=%d, data=%q", update.Callback.Sender.ID, update.Callback.Data)
	case update.PreCheckoutQuery != nil:
		l.Info("PreCheckoutQuery: user_id=%d", update.PreCheckoutQuery.Sender.ID)
	}
}

// LogPayment логирует информацию о платеже
func (l *Logger) LogPayment(userID int64, payload, chargeID string, amount int) {
	l.Info("Payment: user_id=%d, payload=%s, amount=%d, charge_id=%s", userID, payload, amount, chargeID)
}

// LogDownload логирует информацию о скачивании
func (l *Logger) LogDownload(requestID, url string, userID int64, action string) {
	l.Info("Download [%s]: %s for URL: %s (user: %d)", action, requestID, url, userID)
}

// LogErrorWithContext логирует ошибку с контекстом
func (l *Logger) LogErrorWithContext(context string, err error, extraInfo ...string) {
	info := ""
	if len(extraInfo) > 0 {
		info = fmt.Sprintf(" [%s]", extraInfo[0])
	}
	l.Error("%s%s: %v", context, info, err)
}

// LogPerformance логирует производительность операции
func (l *Logger) LogPerformance(operation string, startTime time.Time) {
	duration := time.Since(startTime)
	l.Info("Performance: %s took %v", operation, duration)
}

// LogConfig логирует конфигурацию
func (l *Logger) LogConfig(config *BotConfig) {
	l.Info("Bot configuration: max_workers=%d, use_official_api=%t, api_url=%s",
		config.MaxWorkers, config.UseOfficialAPI, config.TelegramAPIURL)
}
