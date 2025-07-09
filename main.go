package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен в переменных окружения")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Авторизация прошла успешно: %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			log.Printf("Получено сообщение от %s: %s", update.Message.From.UserName, update.Message.Text)

			// Проверяем, есть ли ссылка на YouTube
			ytRegex := regexp.MustCompile(`(https?://)?(www\.)?(youtube\.com|youtu\.be)/[\w\-?=&#./]+`)
			url := ytRegex.FindString(update.Message.Text)
			if url != "" {
				// Проверяем, что это ссылка на конкретное видео
				isVideo := false
				if strings.Contains(url, "watch?v=") {
					isVideo = true
				}
				if strings.Contains(url, "/shorts/") {
					// Проверяем, что после /shorts/ есть id
					parts := strings.Split(url, "/shorts/")
					if len(parts) > 1 && len(parts[1]) >= 5 { // id обычно не короче 5 символов
						isVideo = true
					}
				}
				if !isVideo {
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, отправьте ссылку на конкретное видео YouTube или Shorts."))
					continue
				}
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Скачиваю видео, пожалуйста, подождите...")
				bot.Send(msg)

				// Скачиваем видео
				filename, err := downloadYouTubeVideo(url)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при скачивании видео: "+err.Error()))
					continue
				}

				// Отправляем видео пользователю
				videoFile := tgbotapi.NewVideo(update.Message.Chat.ID, tgbotapi.FilePath(filename))
				videoFile.Caption = "Ваше видео!"
				_, err = bot.Send(videoFile)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при отправке видео: "+err.Error()))
				}

				// Удаляем файл после отправки
				os.Remove(filename)
			}
		}
	}
}

// downloadYouTubeVideo скачивает видео с помощью yt-dlp и возвращает путь к mp4-файлу
func downloadYouTubeVideo(url string) (string, error) {
	filename := filepath.Join(os.TempDir(), "ytvideo_"+randomString(8)+".mp4")
	cmd := exec.Command(".\\yt-dlp.exe", "-f", "mp4", "-o", filename, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New("yt-dlp error: " + err.Error() + ", details: " + string(output))
	}
	if _, err := os.Stat(filename); err != nil {
		return "", errors.New("файл не был создан: " + err.Error())
	}
	return filename, nil
}

// randomString генерирует случайную строку для имени файла
func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[int64(i+os.Getpid()+n)%int64(len(letters))]
	}
	return string(b)
}
