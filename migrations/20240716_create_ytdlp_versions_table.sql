-- +goose Up
-- Создание таблицы для хранения информации о версиях yt-dlp
CREATE TABLE IF NOT EXISTS ytdlp_versions (
    id SERIAL PRIMARY KEY,
    version TEXT NOT NULL UNIQUE,
    last_checked TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_updated TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Создание индекса для ускорения поиска по версии
CREATE INDEX IF NOT EXISTS idx_ytdlp_versions_version ON ytdlp_versions(version);

-- Создание индекса для ускорения поиска по дате последней проверки
CREATE INDEX IF NOT EXISTS idx_ytdlp_versions_last_checked ON ytdlp_versions(last_checked);

-- +goose Down
DROP INDEX IF EXISTS idx_ytdlp_versions_last_checked;
DROP INDEX IF EXISTS idx_ytdlp_versions_version;
DROP TABLE IF EXISTS ytdlp_versions;