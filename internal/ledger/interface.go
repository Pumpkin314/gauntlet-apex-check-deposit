package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AccountID identifies a ledger account. Type alias of uuid.UUID so callers
// need no conversion — designed for zero-cost gRPC extraction in Milestone 2.
type AccountID = uuid.UUID

// Amount is a monetary value as a decimal string (e.g. "500.00").
// Using string avoids float64 rounding on financial values; Postgres NUMERIC
// serialises to string through database/sql.
type Amount string

// Side of a double-entry ledger record.
type Side string

const (
	SideDebit  Side = "DEBIT"
	SideCredit Side = "CREDIT"
)

// EntryType classifies the purpose of a ledger entry pair.
type EntryType string

const (
	EntryTypeProvisionalCredit EntryType = "PROVISIONAL_CREDIT"
	EntryTypeReversal          EntryType = "REVERSAL"
	EntryTypeFee               EntryType = "FEE"
)

// MemoToEntryType maps the transfer memo to the canonical entry_type.
// Memo values are defined by the spec; entry_type is derived, never passed
// directly by callers.
var MemoToEntryType = map[string]EntryType{
	"FREE":              EntryTypeProvisionalCredit,
	"REVERSAL":          EntryTypeReversal,
	"RETURNED_CHECK_FEE": EntryTypeFee,
}

// LedgerEntry is a single row in the ledger_entries table.
type LedgerEntry struct {
	ID         uuid.UUID
	MovementID uuid.UUID
	TransferID uuid.UUID
	AccountID  AccountID
	Side       Side
	Amount     Amount
	EntryType  EntryType
	Memo       string
	CreatedAt  time.Time
}

// LedgerService posts and queries double-entry ledger records.
//
// Invariants (enforced by the implementation, not by callers):
//   - PostDoubleEntry always writes exactly 2 rows in a single transaction.
//   - Every movement_id groups exactly one DEBIT and one CREDIT for the same amount.
//   - Reconcile() must always return "0.00"; non-zero is a compliance violation.
//
// Designed for gRPC extraction in Milestone 2 with zero caller changes.
type LedgerService interface {
	// PostDoubleEntry writes one DEBIT against debit and one CREDIT to credit
	// for the same amount, atomically. The movement_id is generated internally.
	// memo must be "FREE", "REVERSAL", or "RETURNED_CHECK_FEE".
	PostDoubleEntry(ctx context.Context, debit, credit AccountID, amount Amount, memo string, transferID uuid.UUID) error

	// GetBalance returns the net balance for accountID: SUM(CREDIT) - SUM(DEBIT).
	GetBalance(ctx context.Context, accountID AccountID) (Amount, error)

	// GetEntries returns all ledger entries for transferID.
	GetEntries(ctx context.Context, transferID uuid.UUID) ([]LedgerEntry, error)

	// Reconcile returns the global ledger sum across all accounts:
	//   SUM(CASE WHEN side='CREDIT' THEN amount ELSE -amount END)
	// Must always be "0.00".
	Reconcile(ctx context.Context) (Amount, error)
}
