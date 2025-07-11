package main

import (
	"YoutubeDownloader/internal/bot"
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен в переменных окружения")
	}
	adminID := os.Getenv("ADMIN_ID")
	starsProviderToken := os.Getenv("STARS_PROVIDER_TOKEN")

	// Подключение к базе данных
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dsn := "host=" + dbHost + " port=" + dbPort + " user=" + dbUser + " password=" + dbPass + " dbname=" + dbName + " sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных:", err)
	}
	defer db.Close()

	tgBot, err := bot.NewBot(botToken, adminID, starsProviderToken, db)
	if err != nil {
		log.Fatal(err)
	}
	tgBot.Run()
}
