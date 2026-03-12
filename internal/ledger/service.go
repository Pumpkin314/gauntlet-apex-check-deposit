package ledger

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// Store is the persistence interface consumed by Service.
// internal/store/ledger.go provides the production implementation backed by
// database/sql. This interface is the only seam between the ledger domain and
// the storage layer; tests inject an in-memory implementation.
type Store interface {
	// InsertEntries writes all entries in a single atomic transaction.
	// If any insert fails the entire batch must be rolled back.
	InsertEntries(ctx context.Context, entries []LedgerEntry) error

	GetBalance(ctx context.Context, accountID AccountID) (Amount, error)
	GetEntriesByTransfer(ctx context.Context, transferID uuid.UUID) ([]LedgerEntry, error)
	Reconcile(ctx context.Context) (Amount, error)
}

// Service is the production implementation of LedgerService.
type Service struct {
	store Store
	log   *slog.Logger
}

// New constructs a Service backed by store. log must not be nil.
func New(store Store, log *slog.Logger) *Service {
	return &Service{store: store, log: log}
}

// PostDoubleEntry satisfies LedgerService.
//
// It generates a movement_id, constructs exactly one DEBIT entry for the debit
// account and one CREDIT entry for the credit account (same amount, same
// movement_id), and delegates to Store.InsertEntries which must commit both
// rows atomically.
func (s *Service) PostDoubleEntry(
	ctx context.Context,
	debit, credit AccountID,
	amount Amount,
	memo string,
	transferID uuid.UUID,
) error {
	entryType, ok := MemoToEntryType[memo]
	if !ok {
		return fmt.Errorf("ledger: unknown memo %q: must be FREE, REVERSAL, or RETURNED_CHECK_FEE", memo)
	}

	movementID := uuid.New()
	entries := []LedgerEntry{
		{
			ID:         uuid.New(),
			MovementID: movementID,
			TransferID: transferID,
			AccountID:  debit,
			Side:       SideDebit,
			Amount:     amount,
			EntryType:  entryType,
			Memo:       memo,
		},
		{
			ID:         uuid.New(),
			MovementID: movementID,
			TransferID: transferID,
			AccountID:  credit,
			Side:       SideCredit,
			Amount:     amount,
			EntryType:  entryType,
			Memo:       memo,
		},
	}

	if err := s.store.InsertEntries(ctx, entries); err != nil {
		return fmt.Errorf("ledger: post double entry: %w", err)
	}

	s.log.InfoContext(ctx, "ledger double entry posted",
		"transfer_id", transferID,
		"movement_id", movementID,
		"amount", amount,
		"memo", memo,
	)
	return nil
}

// GetBalance satisfies LedgerService.
func (s *Service) GetBalance(ctx context.Context, accountID AccountID) (Amount, error) {
	bal, err := s.store.GetBalance(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("ledger: get balance: %w", err)
	}
	return bal, nil
}

// GetEntries satisfies LedgerService.
func (s *Service) GetEntries(ctx context.Context, transferID uuid.UUID) ([]LedgerEntry, error) {
	entries, err := s.store.GetEntriesByTransfer(ctx, transferID)
	if err != nil {
		return nil, fmt.Errorf("ledger: get entries: %w", err)
	}
	return entries, nil
}

// Reconcile satisfies LedgerService.
func (s *Service) Reconcile(ctx context.Context) (Amount, error) {
	sum, err := s.store.Reconcile(ctx)
	if err != nil {
		return "", fmt.Errorf("ledger: reconcile: %w", err)
	}
	return sum, nil
}
