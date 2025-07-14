-- +goose Up
CREATE TABLE IF NOT EXISTS weekly_user_activity (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    activity_date DATE NOT NULL,
    CONSTRAINT unique_user_date UNIQUE (user_id, activity_date)
);

-- +goose Down
DROP TABLE IF EXISTS weekly_user_activity; 