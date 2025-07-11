# YoutubeDownloader Telegram Bot

Telegram-бот для скачивания видео с YouTube, Shorts и TikTok по ссылке. Видео скачивается с помощью yt-dlp и отправляется пользователю прямо в Telegram. Поддерживает оплату через Telegram Stars и хранение пользователей в PostgreSQL.

## Возможности

- Скачивание видео с YouTube, Shorts и TikTok по ссылке
- Отправка видео пользователю в Telegram
- Оплата через Telegram Stars (разовое скачивание или подписка)
- Хранение пользователей и подписок в PostgreSQL
- **Поддержка отправки больших файлов через локальный сервер Telegram Bot API**

## Требования

- Docker и docker-compose
- PostgreSQL (поднимается через compose)
- yt-dlp (уже включён в Docker-образ, для локального запуска на Windows используйте yt-dlp.exe)
- ffmpeg (автоматически устанавливается в Docker-образе)
- Полученные значения `api_id` и `api_hash` на https://my.telegram.org (раздел API development tools)

## Архитектура

Бот работает через локальный сервер Telegram Bot API (`aiogram/telegram-bot-api`), что позволяет отправлять большие файлы (до 2 ГБ и выше). Все запросы к Telegram идут через этот сервер, а не напрямую в облако Telegram. Это важно для обхода ограничений на размер файлов.

В docker-compose сервисы общаются по внутренним именам (например, `telegram-bot-api:8081`).

## Быстрый старт через Docker Compose

1. Убедитесь, что установлен Docker и docker-compose.
2. Получите свои значения `api_id` и `api_hash` на https://my.telegram.org (раздел API development tools).
3. Создайте файл `docker-compose.yml` (пример ниже).
4. Запустите сервисы:
   ```sh
   docker-compose up -d
   ```

Контейнер бота будет скачан из Docker Hub: `makrotos/youtube_downloader_bot:latest`

## Пример docker-compose.yml с поддержкой больших файлов

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

  telegram-bot-api:
    image: aiogram/telegram-bot-api:latest
    container_name: telegram_bot_api_server
    restart: unless-stopped
    environment:
      - TELEGRAM_API_ID=ваш_api_id
      - TELEGRAM_API_HASH=ваш_api_hash
    command: >
      --api-id=ваш_api_id
      --api-hash=ваш_api_hash
      --local
    ports:
      - "8081:8081"
    volumes:
      - telegram-bot-api-data:/var/lib/telegram-bot-api

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
      - TELEGRAM_API_URL=http://telegram-bot-api:8081
    depends_on:
      - db
      - telegram-bot-api
    restart: unless-stopped
    pull_policy: always

volumes:
  pgdata:
  telegram-bot-api-data:
```

## Переменные окружения

- `TELEGRAM_BOT_TOKEN` — токен вашего Telegram-бота (**обязателен**)
- `DB_HOST` — адрес сервиса PostgreSQL (по умолчанию `db`)
- `DB_PORT` — порт PostgreSQL (по умолчанию `5432`)
- `DB_USER` — пользователь БД (по умолчанию `ytuser`)
- `DB_PASSWORD` — пароль пользователя БД (по умолчанию `ytpass`)
- `DB_NAME` — имя базы данных (по умолчанию `ytbot`)
- `CHANNEL_USERNAME` — username Telegram-канала для бесплатного доступа (опционально)
- `MAX_DOWNLOAD_WORKERS` — максимальное количество одновременных загрузок видео (по умолчанию 3)
- `TELEGRAM_API_URL` — адрес локального сервера Telegram Bot API (например, `http://telegram-bot-api:8081`, **обязателен**)
- `TELEGRAM_API_ID` и `TELEGRAM_API_HASH` — для сервиса telegram-bot-api (получить на https://my.telegram.org)

## Сборка и запуск из исходников

1. Клонируйте репозиторий и перейдите в папку проекта.
2. Соберите Docker-образ:
   ```sh
   docker build -t youtube_downloader_bot .
   ```
3. Запустите через docker-compose, как указано выше.

**В Dockerfile уже предусмотрена установка ffmpeg, wget и копирование yt-dlp_linux.**

Для локального запуска на Windows используйте yt-dlp.exe (лежит в корне проекта).

## Используемые библиотеки

- [telebot.v3](https://github.com/tucnak/telebot) — поддержка кастомного API endpoint (через поле URL в настройках)
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) — скачивание видео
- [ffmpeg](https://ffmpeg.org/) — обработка и склейка аудио/видео
- [goose](https://github.com/pressly/goose) — миграции БД

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
- `docker-compose.yml` — запуск бота, БД и локального Bot API
- `yt-dlp_linux` — бинарник yt-dlp для Linux (используется в Docker)
- `yt-dlp.exe` — бинарник yt-dlp для Windows (опционально)

## Советы по устранению ошибок

- Если бот не отправляет видео или возникают ошибки с размером файла — проверьте, что все запросы идут через локальный сервер Telegram Bot API (`TELEGRAM_API_URL`), а не напрямую в Telegram.
- Если видео не скачивается или не склеивается — убедитесь, что ffmpeg установлен (в Docker-образе он уже есть).
- Для копирования файлов с сервера используйте `scp`, а не `cp`.
- Если бот не может подключиться к Telegram Bot API, проверьте переменную `TELEGRAM_API_URL` и что сервис `telegram-bot-api` запущен.

---

**Не используйте для нарушения авторских прав!**
