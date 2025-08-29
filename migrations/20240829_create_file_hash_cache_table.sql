-- +goose Up
CREATE TABLE IF NOT EXISTS file_hash_cache (
    id SERIAL PRIMARY KEY,
    url TEXT NOT NULL UNIQUE, -- ссылка на видео
    file_hash TEXT NOT NULL, -- MD5 хеш файла
    created_at TIMESTAMP DEFAULT NOW()
);

-- Создаем индекс для быстрого поиска по хешу
CREATE INDEX IF NOT EXISTS idx_file_hash_cache_hash ON file_hash_cache(file_hash);

-- +goose Down
DROP TABLE IF EXISTS file_hash_cache;
