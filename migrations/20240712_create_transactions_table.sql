-- +goose Up
CREATE TABLE transactions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    url TEXT NOT NULL,
    telegram_payment_charge_id TEXT,
    invoice_payload TEXT,
    type TEXT,
    reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE transactions; 