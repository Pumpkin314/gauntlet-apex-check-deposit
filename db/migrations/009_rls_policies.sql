-- +goose Up

-- Enable RLS on all tenant-scoped tables
ALTER TABLE transfers ENABLE ROW LEVEL SECURITY;
ALTER TABLE ledger_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE transfer_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE settlement_batches ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;

-- Transfers: correspondent isolation or apex_admin sees all
CREATE POLICY correspondent_isolation ON transfers
    USING (
        correspondent_id = current_setting('app.correspondent_id', true)::uuid
        OR current_setting('app.role', true) = 'apex_admin'
    );

-- Accounts: correspondent isolation or apex_admin, also allow NULL correspondent_id (FEE account)
CREATE POLICY correspondent_isolation ON accounts
    USING (
        correspondent_id = current_setting('app.correspondent_id', true)::uuid
        OR correspondent_id IS NULL
        OR current_setting('app.role', true) = 'apex_admin'
    );

-- Ledger entries: join through account's correspondent or apex_admin
CREATE POLICY correspondent_isolation ON ledger_entries
    USING (
        account_id IN (
            SELECT id FROM accounts
            WHERE correspondent_id = current_setting('app.correspondent_id', true)::uuid
               OR correspondent_id IS NULL
        )
        OR current_setting('app.role', true) = 'apex_admin'
    );

-- Transfer events: join through transfer's correspondent or apex_admin
CREATE POLICY correspondent_isolation ON transfer_events
    USING (
        transfer_id IN (
            SELECT id FROM transfers
            WHERE correspondent_id = current_setting('app.correspondent_id', true)::uuid
        )
        OR current_setting('app.role', true) = 'apex_admin'
    );

-- Settlement batches: correspondent isolation or apex_admin
CREATE POLICY correspondent_isolation ON settlement_batches
    USING (
        correspondent_id = current_setting('app.correspondent_id', true)::uuid
        OR current_setting('app.role', true) = 'apex_admin'
    );

-- Notifications: through account's correspondent or apex_admin
CREATE POLICY correspondent_isolation ON notifications
    USING (
        account_id IN (
            SELECT id FROM accounts
            WHERE correspondent_id = current_setting('app.correspondent_id', true)::uuid
               OR correspondent_id IS NULL
        )
        OR current_setting('app.role', true) = 'apex_admin'
    );

-- +goose Down
DROP POLICY IF EXISTS correspondent_isolation ON notifications;
DROP POLICY IF EXISTS correspondent_isolation ON settlement_batches;
DROP POLICY IF EXISTS correspondent_isolation ON transfer_events;
DROP POLICY IF EXISTS correspondent_isolation ON ledger_entries;
DROP POLICY IF EXISTS correspondent_isolation ON accounts;
DROP POLICY IF EXISTS correspondent_isolation ON transfers;

ALTER TABLE notifications DISABLE ROW LEVEL SECURITY;
ALTER TABLE settlement_batches DISABLE ROW LEVEL SECURITY;
ALTER TABLE transfer_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE ledger_entries DISABLE ROW LEVEL SECURITY;
ALTER TABLE accounts DISABLE ROW LEVEL SECURITY;
ALTER TABLE transfers DISABLE ROW LEVEL SECURITY;
