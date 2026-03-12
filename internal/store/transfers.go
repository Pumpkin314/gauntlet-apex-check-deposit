package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// HasRecentTransfer returns true if a non-terminal transfer for the same account
// and amount was submitted within the given time window.
// Non-terminal states: anything except Rejected and Returned.
// Implements funding.DuplicateChecker.
func (s *TransferStore) HasRecentTransfer(ctx context.Context, accountID uuid.UUID, amount float64, window time.Duration) (bool, error) {
	since := time.Now().Add(-window)
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM transfers
			WHERE account_id = $1
			  AND amount = $2
			  AND state NOT IN ('Rejected', 'Returned')
			  AND submitted_at > $3
		)`
	var exists bool
	err := s.db.QueryRowContext(ctx, q, accountID, amount, since).Scan(&exists)
	return exists, err
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

// SetErrorCode sets the error_code on a transfer.
func (s *TransferStore) SetErrorCode(ctx context.Context, transferID, errorCode string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET error_code = $1, updated_at = NOW() WHERE id = $2`,
		errorCode, transferID)
	return err
}

// SetReviewReason sets the review_reason on a transfer (flags it for operator queue).
func (s *TransferStore) SetReviewReason(ctx context.Context, transferID, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET review_reason = $1, updated_at = NOW() WHERE id = $2`,
		reason, transferID)
	return err
}

// SetContributionType sets the contribution_type on a transfer.
func (s *TransferStore) SetContributionType(ctx context.Context, transferID, ct string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET contribution_type = $1, updated_at = NOW() WHERE id = $2`,
		ct, transferID)
	return err
}

// SetVSSResults updates vendor service results on a transfer.
func (s *TransferStore) SetVSSResults(ctx context.Context, transferID string, vendorTxID string, confidence float64, micrData map[string]interface{}) error {
	var micrJSON []byte
	if micrData != nil {
		var err error
		micrJSON, err = json.Marshal(micrData)
		if err != nil {
			return fmt.Errorf("marshal micr_data: %w", err)
		}
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET vendor_transaction_id = $1, confidence_score = $2, micr_data = $3, updated_at = NOW() WHERE id = $4`,
		vendorTxID, confidence, micrJSON, transferID)
	return err
}

// SetFromAccountID updates from_account_id (omnibus) after funding resolution.
func (s *TransferStore) SetFromAccountID(ctx context.Context, transferID, fromAccountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET from_account_id = $1, updated_at = NOW() WHERE id = $2`,
		fromAccountID, transferID)
	return err
}

// SetImageRefs sets the front and back image references.
func (s *TransferStore) SetImageRefs(ctx context.Context, transferID, front, back string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET front_image_ref = $1, back_image_ref = $2, updated_at = NOW() WHERE id = $3`,
		front, back, transferID)
	return err
}

// QueueFilter holds optional filter parameters for the operator queue.
type QueueFilter struct {
	CorrespondentID string  // empty = all (admin)
	MinAmount       float64 // 0 = no filter
	MaxAmount       float64 // 0 = no filter
	AccountID       string  // empty = no filter
	SortBy          string  // "created_at" (default), "amount"
}

// ListQueue returns transfers in the operator review queue:
// state='Analyzing' AND review_reason IS NOT NULL, with optional filters.
func (s *TransferStore) ListQueue(ctx context.Context, f QueueFilter) ([]*Transfer, error) {
	q := `
		SELECT
			id, account_id, from_account_id, correspondent_id,
			amount, currency, type, sub_type, transfer_type, memo, state,
			review_reason, error_code, contribution_type,
			vendor_transaction_id, confidence_score,
			micr_data,
			front_image_ref, back_image_ref,
			submitted_at, created_at, updated_at
		FROM transfers
		WHERE state = 'Analyzing' AND review_reason IS NOT NULL`

	args := []interface{}{}
	argIdx := 1

	if f.CorrespondentID != "" {
		q += fmt.Sprintf(" AND correspondent_id = $%d", argIdx)
		args = append(args, f.CorrespondentID)
		argIdx++
	}
	if f.MinAmount > 0 {
		q += fmt.Sprintf(" AND amount >= $%d", argIdx)
		args = append(args, f.MinAmount)
		argIdx++
	}
	if f.MaxAmount > 0 {
		q += fmt.Sprintf(" AND amount <= $%d", argIdx)
		args = append(args, f.MaxAmount)
		argIdx++
	}
	if f.AccountID != "" {
		q += fmt.Sprintf(" AND account_id = $%d", argIdx)
		args = append(args, f.AccountID)
		argIdx++
	}

	switch f.SortBy {
	case "amount":
		q += " ORDER BY amount ASC"
	default:
		q += " ORDER BY created_at ASC"
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*Transfer
	for rows.Next() {
		var t Transfer
		var micrRaw []byte
		err := rows.Scan(
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
			json.Unmarshal(micrRaw, &t.MICRData)
		}
		transfers = append(transfers, &t)
	}
	return transfers, rows.Err()
}

// GetEvents returns all transfer_events for a transfer, ordered by created_at.
func (s *TransferStore) GetEvents(ctx context.Context, transferID string) ([]TransferEvent, error) {
	const q = `
		SELECT id, transfer_id, step, actor, data, created_at
		FROM transfer_events
		WHERE transfer_id = $1
		ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []TransferEvent
	for rows.Next() {
		var e TransferEvent
		var dataRaw []byte
		if err := rows.Scan(&e.ID, &e.TransferID, &e.Step, &e.Actor, &dataRaw, &e.CreatedAt); err != nil {
			return nil, err
		}
		if dataRaw != nil {
			json.Unmarshal(dataRaw, &e.Data)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// TransferEvent is a row from the transfer_events table.
type TransferEvent struct {
	ID         string                 `json:"id"`
	TransferID string                 `json:"transfer_id"`
	Step       string                 `json:"step"`
	Actor      *string                `json:"actor"`
	Data       map[string]interface{} `json:"data"`
	CreatedAt  time.Time              `json:"created_at"`
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
