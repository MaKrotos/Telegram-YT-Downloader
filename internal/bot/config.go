package bot

import (
	"os"
	"strconv"
)

// NewBotConfig создает конфигурацию бота из переменных окружения
func NewBotConfig(token, adminID, providerToken string) *BotConfig {
	config := &BotConfig{
		Token:           token,
		AdminID:         adminID,
		ProviderToken:   providerToken,
		ChannelUsername: os.Getenv("CHANNEL_USERNAME"),
		MaxWorkers:      DefaultMaxWorkers,
		UseOfficialAPI:  os.Getenv("USE_OFFICIAL_API") == "true",
		HTTPTimeout:     DefaultHTTPTimeout,
		DownloadTimeout: DefaultDownloadTimeout,
	}

	// Настройка максимального количества воркеров
	if mwStr := os.Getenv("MAX_DOWNLOAD_WORKERS"); mwStr != "" {
		if mw, err := strconv.Atoi(mwStr); err == nil && mw > 0 {
			config.MaxWorkers = mw
		}
	}

	// Настройка URL для API
	if config.UseOfficialAPI {
		config.TelegramAPIURL = "https://api.telegram.org"
	} else {
		if url := os.Getenv("TELEGRAM_API_URL"); url != "" {
			config.TelegramAPIURL = url
		}
	}

	return config
}

// GetAPISettings возвращает настройки для Telegram API
func (c *BotConfig) GetAPISettings() map[string]interface{} {
	return map[string]interface{}{
		"token":    c.Token,
		"url":      c.TelegramAPIURL,
		"timeout":  c.HTTPTimeout,
		"poller":   DefaultPollerTimeout,
		"official": c.UseOfficialAPI,
	}
}
