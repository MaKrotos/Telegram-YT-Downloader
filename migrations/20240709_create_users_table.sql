-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    premium_until TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS users; 