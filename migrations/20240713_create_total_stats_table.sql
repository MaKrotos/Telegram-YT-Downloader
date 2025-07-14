-- +goose Up
CREATE TABLE IF NOT EXISTS total_stats (
    id SERIAL PRIMARY KEY,
    total_users BIGINT DEFAULT 0,
    total_downloads BIGINT DEFAULT 0,
    total_messages BIGINT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS total_stats; 