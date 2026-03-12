// Package store is the ONLY package that imports database/sql.
// All other packages interact with persistent storage through interfaces
// defined in their own packages (e.g. ledger.Store).
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/google/uuid"
)

// LedgerStore implements ledger.Store against a Postgres database.
// All writes go through InsertEntries which wraps inserts in a single
// transaction — ledger entries are append-only and never UPDATEd or DELETEd.
type LedgerStore struct {
	db *sql.DB
}

// NewLedgerStore constructs a LedgerStore. db must be a live *sql.DB.
func NewLedgerStore(db *sql.DB) *LedgerStore {
	return &LedgerStore{db: db}
}

// InsertEntries writes all entries atomically in a single transaction.
// If any row fails (e.g. unique constraint) the whole batch is rolled back.
// Ledger entries are append-only: this method must never UPDATE or DELETE rows.
func (s *LedgerStore) InsertEntries(ctx context.Context, entries []ledger.LedgerEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/ledger: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback on error path; commit below on success

	const q = `
		INSERT INTO ledger_entries
			(id, movement_id, transfer_id, account_id, side, amount, entry_type, memo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, q,
			e.ID,
			e.MovementID,
			e.TransferID,
			e.AccountID,
			string(e.Side),
			string(e.Amount),
			string(e.EntryType),
			e.Memo,
		); err != nil {
			return fmt.Errorf("store/ledger: insert entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/ledger: commit: %w", err)
	}
	return nil
}

// GetBalance returns the net balance for accountID:
//
//	SUM(CASE WHEN side = 'CREDIT' THEN amount ELSE -amount END)
//
// Returns "0.00" for accounts with no entries.
func (s *LedgerStore) GetBalance(ctx context.Context, accountID ledger.AccountID) (ledger.Amount, error) {
	const q = `
		SELECT COALESCE(
			SUM(CASE WHEN side = 'CREDIT' THEN amount ELSE -amount END),
			0
		)::TEXT
		FROM ledger_entries
		WHERE account_id = $1`

	var bal string
	if err := s.db.QueryRowContext(ctx, q, accountID).Scan(&bal); err != nil {
		return "", fmt.Errorf("store/ledger: get balance: %w", err)
	}
	return ledger.Amount(bal), nil
}

// GetEntriesByTransfer returns all ledger entries for a transfer, ordered by
// creation time. Returns nil (not an error) if no entries exist.
func (s *LedgerStore) GetEntriesByTransfer(ctx context.Context, transferID uuid.UUID) ([]ledger.LedgerEntry, error) {
	const q = `
		SELECT id, movement_id, transfer_id, account_id,
		       side, amount::TEXT, entry_type, memo, created_at
		FROM ledger_entries
		WHERE transfer_id = $1
		ORDER BY created_at`

	rows, err := s.db.QueryContext(ctx, q, transferID)
	if err != nil {
		return nil, fmt.Errorf("store/ledger: query entries: %w", err)
	}
	defer rows.Close()

	var entries []ledger.LedgerEntry
	for rows.Next() {
		var e ledger.LedgerEntry
		var amt string
		var side, entryType string
		if err := rows.Scan(
			&e.ID, &e.MovementID, &e.TransferID, &e.AccountID,
			&side, &amt, &entryType, &e.Memo, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store/ledger: scan entry: %w", err)
		}
		e.Side = ledger.Side(side)
		e.Amount = ledger.Amount(amt)
		e.EntryType = ledger.EntryType(entryType)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/ledger: rows: %w", err)
	}
	return entries, nil
}

// Reconcile returns the global ledger sum:
//
//	SUM(CASE WHEN side = 'CREDIT' THEN amount ELSE -amount END)
//
// Must always return "0.00". Non-zero is a compliance violation.
func (s *LedgerStore) Reconcile(ctx context.Context) (ledger.Amount, error) {
	const q = `
		SELECT COALESCE(
			SUM(CASE WHEN side = 'CREDIT' THEN amount ELSE -amount END),
			0
		)::TEXT
		FROM ledger_entries`

	var sum string
	if err := s.db.QueryRowContext(ctx, q).Scan(&sum); err != nil {
		return "", fmt.Errorf("store/ledger: reconcile: %w", err)
	}
	return ledger.Amount(sum), nil
}
