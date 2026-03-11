-- +goose Up
CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    response_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;
