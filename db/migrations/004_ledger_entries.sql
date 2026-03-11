-- +goose Up
CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    movement_id UUID NOT NULL,
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    account_id UUID NOT NULL REFERENCES accounts(id),
    side TEXT NOT NULL,
    amount NUMERIC(12,2) NOT NULL,
    entry_type TEXT NOT NULL,
    memo TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_side CHECK (side IN ('DEBIT', 'CREDIT')),
    CONSTRAINT valid_amount CHECK (amount > 0),
    CONSTRAINT unique_entry_per_type UNIQUE (transfer_id, entry_type, account_id, side)
);

-- +goose Down
DROP TABLE IF EXISTS ledger_entries;
