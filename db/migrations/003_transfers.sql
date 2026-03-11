-- +goose Up
CREATE TABLE transfers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Spec-required ledger attributes
    account_id UUID NOT NULL REFERENCES accounts(id),
    from_account_id UUID NOT NULL REFERENCES accounts(id),
    amount NUMERIC(12,2) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    type TEXT NOT NULL DEFAULT 'MOVEMENT',
    sub_type TEXT NOT NULL DEFAULT 'DEPOSIT',
    transfer_type TEXT NOT NULL DEFAULT 'CHECK',
    memo TEXT NOT NULL DEFAULT 'FREE',

    -- State machine
    state TEXT NOT NULL DEFAULT 'Requested',
    review_reason TEXT,
    error_code TEXT,
    contribution_type TEXT,
    contribution_type_override TEXT,

    -- Vendor Service results
    vendor_transaction_id TEXT,
    confidence_score NUMERIC(4,2),
    micr_data JSONB,

    -- Images
    front_image_ref TEXT,
    back_image_ref TEXT,

    -- Settlement
    settlement_batch_id UUID,
    settled_at TIMESTAMPTZ,

    -- Correspondent (denormalized for RLS)
    correspondent_id UUID NOT NULL REFERENCES correspondents(id),

    -- Timestamps
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT valid_state CHECK (state IN (
        'Requested', 'Validating', 'Analyzing', 'Approved',
        'FundsPosted', 'Completed', 'Rejected', 'Returned'
    )),
    CONSTRAINT valid_amount CHECK (amount > 0),
    CONSTRAINT unique_vendor_tx UNIQUE (account_id, vendor_transaction_id)
);

-- +goose Down
DROP TABLE IF EXISTS transfers;
