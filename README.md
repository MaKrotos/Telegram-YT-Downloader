# YoutubeDownloader Telegram Bot

Telegram-бот для скачивания видео с YouTube, Shorts и TikTok по ссылке. Видео скачивается с помощью yt-dlp и отправляется пользователю прямо в Telegram. Поддерживает оплату через Telegram Stars, подписки, кэширование видео и хранение пользователей/статистики в PostgreSQL.

## Возможности

- Скачивание видео с YouTube, Shorts и TikTok по ссылке
- Кэширование скачанных видео (ускоряет повторные загрузки)
- Отправка видео пользователю в Telegram
- Оплата через Telegram Stars (разовое скачивание или подписка)
- Проверка подписки на канал для бесплатных загрузок
- Хранение пользователей, транзакций, статистики и кэша в PostgreSQL
- Админ-команды: статистика, управление кэшем, возвраты, тестовые платежи
- Локализация (русский, английский, испанский, французский)
- **Поддержка отправки больших файлов через локальный сервер Telegram Bot API**
- Очистка старого кэша и временных файлов

## Архитектура и структура internal/

- `internal/bot/` — основная логика Telegram-бота: обработка команд, сообщений, платежей, подписок, админ-функций, статистики, локализации, управления загрузками.
- `internal/downloader/` — скачивание видео с помощью yt-dlp, поддержка разных стратегий качества, очистка временных файлов, диагностика файловой системы.
- `internal/payment/` — работа с транзакциями: модели, сервисы, сохранение/чтение из БД, возвраты через Telegram Stars API.
- `internal/storage/` — кэширование скачанных видео (video_cache), работа с кэшем через БД, очистка старых записей, статистика кэша.
- `internal/i18n/` — локализация: менеджер переводов, поддержка нескольких языков, хранение переводов в JSON.
- `internal/utils/` — вспомогательные функции: генерация случайных строк, очистка временных файлов, диагностика файловой системы и др.
- `internal/config/` — конфигурация (расширяется при необходимости).

## Миграции и структура БД

Миграции находятся в папке `migrations/` и применяются через [goose](https://github.com/pressly/goose).

### Основные таблицы:
- **users** — пользователи, поддержка premium_until (премиум-подписка)
- **transactions** — все транзакции (user_id, amount, status, url, charge_id, payload, тип, причина, created_at, updated_at)
- **video_cache** — кэш скачанных видео (url, telegram_file_id, created_at)
- **total_stats** — агрегированная статистика (всего пользователей, загрузок, сообщений)
- **user_stats** — индивидуальная статистика по пользователям
- **weekly_user_activity** — недельная активность пользователей

Пример применения миграций:
```sh
# В контейнере
 docker-compose exec bot goose -dir ./migrations postgres "host=db user=ytuser password=ytpass dbname=ytbot sslmode=disable" up
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
- `USE_OFFICIAL_API` — использовать официальный Telegram Bot API (true/false, по умолчанию false)

## Быстрый старт через Docker Compose

1. Убедитесь, что установлен Docker и docker-compose.
2. Получите свои значения `api_id` и `api_hash` на https://my.telegram.org (раздел API development tools).
3. Создайте файл `docker-compose.yml` (пример ниже).
4. Запустите сервисы:
   ```sh
   docker-compose up -d
   ```

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

## Структура проекта

- `main.go` — точка входа
- `internal/bot/` — логика Telegram-бота (команды, платежи, подписки, статистика, локализация, загрузки)
- `internal/downloader/` — скачивание видео, очистка временных файлов
- `internal/payment/` — транзакции, возвраты, работа с БД
- `internal/storage/` — кэш видео, статистика кэша
- `internal/i18n/` — локализация и переводы
- `internal/utils/` — утилиты и вспомогательные функции
- `migrations/` — миграции PostgreSQL
- `Dockerfile` — инструкция для сборки
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
