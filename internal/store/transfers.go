package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq" // Postgres driver
)

// Sentinel errors.
var (
	// ErrOptimisticLock is returned by UpdateState when the transfer is not in the expected state.
	// This indicates a concurrent transition already moved the transfer.
	ErrOptimisticLock = errors.New("optimistic lock conflict: transfer state has changed")

	// ErrNotFound is returned when no transfer matches the given ID.
	ErrNotFound = errors.New("transfer not found")
)

// Transfer is the in-memory representation of a transfers row.
type Transfer struct {
	ID              string
	AccountID       string
	FromAccountID   string
	CorrespondentID string
	Amount          float64
	Currency        string
	Type            string
	SubType         string
	TransferType    string
	Memo            string
	State           string
	ReviewReason    *string
	ErrorCode       *string
	ContributionType *string
	VendorTransactionID *string
	ConfidenceScore *float64
	MICRData        map[string]interface{}
	FrontImageRef   *string
	BackImageRef    *string
	SubmittedAt     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateTransferInput carries the required fields for creating a new transfer.
// State is always set to Requested; spec fields (type, sub_type, etc.) are hardcoded per CLAUDE.md.
type CreateTransferInput struct {
	AccountID       string
	FromAccountID   string
	CorrespondentID string
	Amount          float64
}

// TransferStore is the DB-backed implementation of transfer persistence.
// It is the only type that touches database/sql for transfers.
type TransferStore struct {
	db *sql.DB
}

// NewTransferStore constructs a TransferStore from an open *sql.DB.
func NewTransferStore(db *sql.DB) *TransferStore {
	return &TransferStore{db: db}
}

// Create inserts a new transfer in Requested state and returns the created record.
// All spec-required fields are set here: type=MOVEMENT, memo=FREE, sub_type=DEPOSIT,
// transfer_type=CHECK, currency=USD.
func (s *TransferStore) Create(ctx context.Context, in CreateTransferInput) (*Transfer, error) {
	const q = `
		INSERT INTO transfers (
			account_id, from_account_id, correspondent_id, amount,
			currency, type, sub_type, transfer_type, memo, state
		) VALUES ($1, $2, $3, $4, 'USD', 'MOVEMENT', 'DEPOSIT', 'CHECK', 'FREE', 'Requested')
		RETURNING
			id, account_id, from_account_id, correspondent_id,
			amount, currency, type, sub_type, transfer_type, memo, state,
			submitted_at, created_at, updated_at`

	row := s.db.QueryRowContext(ctx, q,
		in.AccountID, in.FromAccountID, in.CorrespondentID, in.Amount,
	)
	return scanTransfer(row)
}

// GetByID fetches a transfer by primary key. Returns ErrNotFound if not present.
func (s *TransferStore) GetByID(ctx context.Context, id string) (*Transfer, error) {
	const q = `
		SELECT
			id, account_id, from_account_id, correspondent_id,
			amount, currency, type, sub_type, transfer_type, memo, state,
			review_reason, error_code, contribution_type,
			vendor_transaction_id, confidence_score,
			micr_data,
			front_image_ref, back_image_ref,
			submitted_at, created_at, updated_at
		FROM transfers
		WHERE id = $1`

	row := s.db.QueryRowContext(ctx, q, id)
	t, err := scanTransferFull(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// UpdateState transitions the transfer from `from` to `to` using optimistic locking.
//
// SQL:
//
//	UPDATE transfers SET state=$to, updated_at=NOW()
//	WHERE id=$transferID AND state=$from
//	RETURNING id
//
// Returns ErrOptimisticLock if no row is updated (transfer not in expected state).
func (s *TransferStore) UpdateState(ctx context.Context, transferID, from, to string) error {
	const q = `
		UPDATE transfers
		SET state = $1, updated_at = NOW()
		WHERE id = $2 AND state = $3
		RETURNING id`

	var id string
	err := s.db.QueryRowContext(ctx, q, to, transferID, from).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOptimisticLock
	}
	return err
}

// WriteEvent inserts a row into transfer_events.
// Implements orchestrator.EventWriter.
func (s *TransferStore) WriteEvent(ctx context.Context, transferID, step, actor string, data map[string]interface{}) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}
	const q = `
		INSERT INTO transfer_events (transfer_id, step, actor, data)
		VALUES ($1, $2, $3, $4)`
	_, err = s.db.ExecContext(ctx, q, transferID, step, actor, dataJSON)
	return err
}

// Notify sends a pg_notify on the 'transfer_updates' channel for SSE delivery.
// Implements orchestrator.Notifier.
func (s *TransferStore) Notify(ctx context.Context, transferID string, payload map[string]interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notify payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `SELECT pg_notify('transfer_updates', $1)`, string(payloadJSON))
	return err
}

// scanTransfer scans the minimal columns returned by Create.
func scanTransfer(row *sql.Row) (*Transfer, error) {
	var t Transfer
	err := row.Scan(
		&t.ID, &t.AccountID, &t.FromAccountID, &t.CorrespondentID,
		&t.Amount, &t.Currency, &t.Type, &t.SubType, &t.TransferType,
		&t.Memo, &t.State,
		&t.SubmittedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// scanTransferFull scans the full column set returned by GetByID.
func scanTransferFull(row *sql.Row) (*Transfer, error) {
	var t Transfer
	var micrRaw []byte
	err := row.Scan(
		&t.ID, &t.AccountID, &t.FromAccountID, &t.CorrespondentID,
		&t.Amount, &t.Currency, &t.Type, &t.SubType, &t.TransferType,
		&t.Memo, &t.State,
		&t.ReviewReason, &t.ErrorCode, &t.ContributionType,
		&t.VendorTransactionID, &t.ConfidenceScore,
		&micrRaw,
		&t.FrontImageRef, &t.BackImageRef,
		&t.SubmittedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if micrRaw != nil {
		if err := json.Unmarshal(micrRaw, &t.MICRData); err != nil {
			return nil, fmt.Errorf("unmarshal micr_data: %w", err)
		}
	}
	return &t, nil
}
