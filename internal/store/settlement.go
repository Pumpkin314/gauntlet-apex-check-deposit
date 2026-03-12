package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/settlement"
)

// SettlementStore handles settlement batch persistence.
// It is the only type that touches database/sql for settlement_batches.
type SettlementStore struct {
	db *sql.DB
}

// NewSettlementStore creates a SettlementStore backed by db.
func NewSettlementStore(db *sql.DB) *SettlementStore {
	return &SettlementStore{db: db}
}

// ListFundsPostedBefore returns all transfers in FundsPosted state
// with submitted_at strictly before cutoffUTC.
// Implements settlement.TransferQuerier.
func (s *SettlementStore) ListFundsPostedBefore(ctx context.Context, cutoffUTC time.Time) ([]*settlement.Transfer, error) {
	const q = `
		SELECT id, account_id, from_account_id, correspondent_id,
		       amount, state, micr_data, front_image_ref, back_image_ref, submitted_at
		FROM transfers
		WHERE state = 'FundsPosted'
		  AND submitted_at < $1
		ORDER BY submitted_at ASC`

	rows, err := s.db.QueryContext(ctx, q, cutoffUTC)
	if err != nil {
		return nil, fmt.Errorf("query FundsPosted transfers: %w", err)
	}
	defer rows.Close()

	var transfers []*settlement.Transfer
	for rows.Next() {
		var t settlement.Transfer
		var micrRaw []byte
		err := rows.Scan(
			&t.ID, &t.AccountID, &t.FromAccountID, &t.CorrespondentID,
			&t.Amount, &t.State, &micrRaw, &t.FrontImageRef, &t.BackImageRef, &t.SubmittedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan transfer: %w", err)
		}
		if micrRaw != nil {
			json.Unmarshal(micrRaw, &t.MICRData)
		}
		transfers = append(transfers, &t)
	}
	return transfers, rows.Err()
}

// CreateBatch inserts a new settlement batch and returns its ID.
func (s *SettlementStore) CreateBatch(ctx context.Context, correspondentID string, cutoffDate time.Time, fileRef string, recordCount int, totalAmount float64) (uuid.UUID, error) {
	const q = `
		INSERT INTO settlement_batches (correspondent_id, cutoff_date, status, file_ref, record_count, total_amount)
		VALUES ($1, $2, 'PENDING', $3, $4, $5)
		RETURNING id`

	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, q, correspondentID, cutoffDate.Format("2006-01-02"), fileRef, recordCount, totalAmount).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert settlement batch: %w", err)
	}
	return id, nil
}

// UpdateBatchStatus updates a batch's status and optionally sets acknowledged_at and submitted_at.
func (s *SettlementStore) UpdateBatchStatus(ctx context.Context, batchID uuid.UUID, status string, acknowledgedAt *time.Time) error {
	switch status {
	case "SUBMITTED":
		_, err := s.db.ExecContext(ctx,
			`UPDATE settlement_batches SET status = $1, submitted_at = NOW() WHERE id = $2`,
			status, batchID)
		return err
	case "ACKNOWLEDGED":
		_, err := s.db.ExecContext(ctx,
			`UPDATE settlement_batches SET status = $1, acknowledged_at = $2 WHERE id = $3`,
			status, acknowledgedAt, batchID)
		return err
	default:
		_, err := s.db.ExecContext(ctx,
			`UPDATE settlement_batches SET status = $1 WHERE id = $2`,
			status, batchID)
		return err
	}
}

// SetSettlementBatch links a transfer to a settlement batch and optionally sets settled_at.
func (s *SettlementStore) SetSettlementBatch(ctx context.Context, transferID string, batchID uuid.UUID, settledAt time.Time) error {
	if settledAt.IsZero() {
		_, err := s.db.ExecContext(ctx,
			`UPDATE transfers SET settlement_batch_id = $1, updated_at = NOW() WHERE id = $2`,
			batchID, transferID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET settlement_batch_id = $1, settled_at = $2, updated_at = NOW() WHERE id = $3`,
		batchID, settledAt, transferID)
	return err
}

// ListBatches returns all settlement batches ordered by created_at desc.
func (s *SettlementStore) ListBatches(ctx context.Context) ([]*settlement.Batch, error) {
	const q = `
		SELECT id, correspondent_id, cutoff_date, status, file_ref,
		       record_count, total_amount, submitted_at, acknowledged_at, created_at
		FROM settlement_batches
		ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []*settlement.Batch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		// Load transfer IDs for this batch
		tids, err := s.transferIDsForBatch(ctx, b.ID)
		if err != nil {
			return nil, err
		}
		b.TransferIDs = tids
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

// GetBatch returns a single settlement batch by ID with transfer IDs.
func (s *SettlementStore) GetBatch(ctx context.Context, id uuid.UUID) (*settlement.Batch, error) {
	const q = `
		SELECT id, correspondent_id, cutoff_date, status, file_ref,
		       record_count, total_amount, submitted_at, acknowledged_at, created_at
		FROM settlement_batches
		WHERE id = $1`

	row := s.db.QueryRowContext(ctx, q, id)
	b, err := scanBatchRow(row)
	if err != nil {
		return nil, err
	}

	tids, err := s.transferIDsForBatch(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	b.TransferIDs = tids
	return b, nil
}

// UnbatchedFundsPostedCount returns the number of FundsPosted transfers not yet in a batch.
func (s *SettlementStore) UnbatchedFundsPostedCount(ctx context.Context) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM transfers
		WHERE state = 'FundsPosted'
		  AND settlement_batch_id IS NULL`
	var count int
	err := s.db.QueryRowContext(ctx, q).Scan(&count)
	return count, err
}

func (s *SettlementStore) transferIDsForBatch(ctx context.Context, batchID uuid.UUID) ([]string, error) {
	const q = `SELECT id FROM transfers WHERE settlement_batch_id = $1 ORDER BY submitted_at`
	rows, err := s.db.QueryContext(ctx, q, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanBatch(rows *sql.Rows) (*settlement.Batch, error) {
	var b settlement.Batch
	var fileRef sql.NullString
	var recordCount sql.NullInt32
	var totalAmount sql.NullFloat64
	var submittedAt sql.NullTime
	var acknowledgedAt sql.NullTime

	err := rows.Scan(
		&b.ID, &b.CorrespondentID, &b.CutoffDate, &b.Status,
		&fileRef, &recordCount, &totalAmount,
		&submittedAt, &acknowledgedAt, &b.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if fileRef.Valid {
		b.FileRef = fileRef.String
	}
	if recordCount.Valid {
		b.RecordCount = int(recordCount.Int32)
	}
	if totalAmount.Valid {
		b.TotalAmount = totalAmount.Float64
	}
	if submittedAt.Valid {
		b.SubmittedAt = &submittedAt.Time
	}
	if acknowledgedAt.Valid {
		b.AcknowledgedAt = &acknowledgedAt.Time
	}
	return &b, nil
}

func scanBatchRow(row *sql.Row) (*settlement.Batch, error) {
	var b settlement.Batch
	var fileRef sql.NullString
	var recordCount sql.NullInt32
	var totalAmount sql.NullFloat64
	var submittedAt sql.NullTime
	var acknowledgedAt sql.NullTime

	err := row.Scan(
		&b.ID, &b.CorrespondentID, &b.CutoffDate, &b.Status,
		&fileRef, &recordCount, &totalAmount,
		&submittedAt, &acknowledgedAt, &b.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if fileRef.Valid {
		b.FileRef = fileRef.String
	}
	if recordCount.Valid {
		b.RecordCount = int(recordCount.Int32)
	}
	if totalAmount.Valid {
		b.TotalAmount = totalAmount.Float64
	}
	if submittedAt.Valid {
		b.SubmittedAt = &submittedAt.Time
	}
	if acknowledgedAt.Valid {
		b.AcknowledgedAt = &acknowledgedAt.Time
	}
	return &b, nil
}
