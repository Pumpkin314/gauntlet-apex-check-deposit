-- +goose Up
CREATE TABLE settlement_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    correspondent_id UUID NOT NULL REFERENCES correspondents(id),
    cutoff_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    file_ref TEXT,
    record_count INT,
    total_amount NUMERIC(14,2),
    submitted_at TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_status CHECK (status IN ('PENDING', 'SUBMITTED', 'ACKNOWLEDGED'))
);

-- Add FK from transfers to settlement_batches now that both tables exist
ALTER TABLE transfers
    ADD CONSTRAINT fk_settlement_batch
    FOREIGN KEY (settlement_batch_id) REFERENCES settlement_batches(id);

-- +goose Down
ALTER TABLE transfers DROP CONSTRAINT IF EXISTS fk_settlement_batch;
DROP TABLE IF EXISTS settlement_batches;
