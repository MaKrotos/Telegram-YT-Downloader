package main

import (
	"log"
	"os"

	"YoutubeDownloader/internal/bot"
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен в переменных окружения")
	}
	adminID := os.Getenv("ADMIN_ID")
	starsProviderToken := os.Getenv("STARS_PROVIDER_TOKEN")

	tgBot, err := bot.NewBot(botToken, adminID, starsProviderToken)
	if err != nil {
		log.Fatal(err)
	}
	tgBot.Run()
}
