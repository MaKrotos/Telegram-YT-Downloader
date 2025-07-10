# YoutubeDownloader Telegram Bot

Telegram-бот для скачивания видео с YouTube, Shorts и TikTok по ссылке. Видео скачивается с помощью yt-dlp и отправляется пользователю прямо в Telegram. Поддерживает оплату через Telegram Stars и хранение пользователей в PostgreSQL.

## Возможности

- Скачивание видео с YouTube, Shorts и TikTok по ссылке
- Отправка видео пользователю в Telegram
- Оплата через Telegram Stars (разовое скачивание или подписка)
- Хранение пользователей и подписок в PostgreSQL

## Быстрый старт через Docker Hub

1. Убедитесь, что установлен Docker и docker-compose.
2. Создайте файл `docker-compose.yml` (пример ниже).
3. Запустите сервисы:
   ```sh
   docker-compose up -d
   ```

Контейнер бота будет скачан из Docker Hub: `makrotos/youtube_downloader_bot:latest`

## Пример docker-compose.yml

```yaml
version: "3.8"

services:
  db:
    image: postgres:16-alpine
    container_name: youtube_downloader_db
    restart: unless-stopped
    environment:
      - POSTGRES_DB=ytbot
      - POSTGRES_USER=ytuser
      - POSTGRES_PASSWORD=ytpass
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  bot:
    image: makrotos/youtube_downloader_bot:latest
    container_name: youtube_downloader_bot
    environment:
      - TELEGRAM_BOT_TOKEN=ваш_токен_бота
      - DB_HOST=db
      - DB_PORT=5432
      - DB_USER=ytuser
      - DB_PASSWORD=ytpass
      - DB_NAME=ytbot
      - CHANNEL_USERNAME=@your_channel_name
    depends_on:
      - db
    restart: unless-stopped

volumes:
  pgdata:
```

## Переменные окружения

- `TELEGRAM_BOT_TOKEN` — токен вашего Telegram-бота (**обязателен**)
- `DB_HOST` — адрес сервиса PostgreSQL (по умолчанию `db`)
- `DB_PORT` — порт PostgreSQL (по умолчанию `5432`)
- `DB_USER` — пользователь БД (по умолчанию `ytuser`)
- `DB_PASSWORD` — пароль пользователя БД (по умолчанию `ytpass`)
- `DB_NAME` — имя базы данных (по умолчанию `ytbot`)
- `CHANNEL_USERNAME` — username Telegram-канала для бесплатного доступа (опционально)

## Миграции базы данных

Для управления схемой БД используется [goose](https://github.com/pressly/goose):

- Миграции хранятся в папке `migrations/`.
- Пример применения миграций:
  ```sh
  docker-compose exec bot goose -dir ./migrations postgres "host=db user=ytuser password=ytpass dbname=ytbot sslmode=disable" up
  ```

## Структура проекта

- `main.go` — точка входа
- `internal/bot/` — логика Telegram-бота
- `internal/downloader/` — скачивание видео
- `internal/payment/` — транзакции и подписки
- `migrations/` — миграции PostgreSQL
- `Dockerfile` — инструкция для сборки (если нужно)
- `docker-compose.yml` — запуск бота и БД

---

**Не используйте для нарушения авторских прав!**
