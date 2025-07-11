-- +goose Up
CREATE TABLE IF NOT EXISTS video_cache (
    id SERIAL PRIMARY KEY,
    url TEXT NOT NULL UNIQUE, -- ссылка на видео
    telegram_file_id TEXT NOT NULL, -- file_id, полученный от Telegram
    created_at TIMESTAMP DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS video_cache; 