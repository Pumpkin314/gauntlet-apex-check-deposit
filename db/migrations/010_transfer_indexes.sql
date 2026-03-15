-- +goose Up
-- Composite index covers: settlement queries (state + submitted_at),
-- per-investor history (account_id), and operator queue filters (state).
CREATE INDEX idx_transfers_state_submitted ON transfers(state, submitted_at);
CREATE INDEX idx_transfers_account_id ON transfers(account_id);

-- +goose Down
DROP INDEX IF EXISTS idx_transfers_account_id;
DROP INDEX IF EXISTS idx_transfers_state_submitted;
