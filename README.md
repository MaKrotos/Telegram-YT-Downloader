# YoutubeDownloader Telegram Bot

Этот проект — Telegram-бот для скачивания видео с YouTube (включая Shorts) по ссылке. Видео скачивается с помощью yt-dlp и отправляется пользователю прямо в Telegram.

## Возможности

- Получает ссылку на YouTube-видео или Shorts
- Скачивает видео в формате mp4
- Отправляет скачанное видео пользователю
- Оплата через Telegram Stars (звёзды):
  - Разовое скачивание за 1 звезду
  - Подписка на месяц за 30 звёзд (премиум)
- Хранение пользователей и подписок в PostgreSQL

## Быстрый старт через Docker

1. Соберите Docker-образ:
   ```sh
   docker build -t youtube-downloader-bot .
   ```
2. Запустите контейнеры с ботом и PostgreSQL:
   ```sh
   docker-compose up -d
   ```

## Переменные окружения

- `TELEGRAM_BOT_TOKEN` — токен вашего Telegram-бота (обязателен)
- `ADMIN_ID` — Telegram user ID главного администратора (опционально)
- `DB_HOST` — адрес сервиса PostgreSQL (по умолчанию `db`)
- `DB_PORT` — порт PostgreSQL (по умолчанию `5432`)
- `DB_USER` — пользователь БД (по умолчанию `ytuser`)
- `DB_PASSWORD` — пароль пользователя БД (по умолчанию `ytpass`)
- `DB_NAME` — имя базы данных (по умолчанию `ytbot`)

## Миграции базы данных

Для управления схемой БД используется [goose](https://github.com/pressly/goose):

- Миграции хранятся в папке `migrations/`.
- Пример применения миграций:
  ```sh
  docker-compose exec bot goose -dir ./migrations postgres "host=db user=ytuser password=ytpass dbname=ytbot sslmode=disable" up
  ```

## Требования

- Docker
- Telegram-бот (создайте через @BotFather и получите токен)

## Структура проекта

- `main.go` — точка входа, запуск и инициализация бота
- `internal/bot/` — основная логика Telegram-бота (инициализация, обработка сообщений, оплата, скачивание, работа с БД)
- `internal/downloader/` — функции для скачивания видео с YouTube
- `internal/payment/` — логика транзакций и возвратов
- `migrations/` — миграции для PostgreSQL (таблицы пользователей и подписок)
- `yt-dlp.exe` — бинарник для скачивания видео
- `Dockerfile` — инструкция для сборки контейнера
- `docker-compose.yml` — запуск бота и БД в контейнерах

## Пример docker-compose.yml

```yaml
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
    build: .
    container_name: youtube_downloader_bot
    environment:
      - TELEGRAM_BOT_TOKEN=ваш_токен_бота
      - DB_HOST=db
      - DB_PORT=5432
      - DB_USER=ytuser
      - DB_PASSWORD=ytpass
      - DB_NAME=ytbot
    depends_on:
      - db
    restart: unless-stopped

volumes:
  pgdata:
```

## Примечания

- Видео скачиваются во временную папку и удаляются после отправки.
- Поддерживаются только прямые ссылки на видео или Shorts.
- Для работы вне Docker убедитесь, что yt-dlp.exe находится в той же папке, что и main.go.
- Для оплаты через Telegram Stars не требуется внешний provider_token — всё работает через встроенную платёжную систему Telegram.

---

**Не используйте для нарушения авторских прав!**
