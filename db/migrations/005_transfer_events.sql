-- +goose Up
CREATE TABLE transfer_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    step TEXT NOT NULL,
    actor TEXT,
    data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_transfer_events_transfer ON transfer_events(transfer_id);

-- +goose Down
DROP INDEX IF EXISTS idx_transfer_events_transfer;
DROP TABLE IF EXISTS transfer_events;
