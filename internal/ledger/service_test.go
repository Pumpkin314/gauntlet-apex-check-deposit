package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/google/uuid"
)

// ---- in-memory Store --------------------------------------------------------

// memStore is a thread-safe, in-memory implementation of ledger.Store for
// unit tests. It enforces the DB unique constraint and simulates atomicity.
type memStore struct {
	mu       sync.Mutex
	entries  []ledger.LedgerEntry
	failNext bool // if true, next InsertEntries call returns an error
}

func (m *memStore) InsertEntries(_ context.Context, entries []ledger.LedgerEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failNext {
		m.failNext = false
		return errors.New("simulated DB error")
	}

	// Enforce unique constraint: (transfer_id, entry_type, account_id, side).
	// Check ALL incoming entries against existing AND against each other before
	// appending any, so the whole batch is rejected atomically on conflict.
	combined := append(m.entries, entries...) //nolint:gocritic // intentional copy check
	seen := make(map[[4]string]bool, len(combined))
	for _, e := range combined {
		key := [4]string{
			e.TransferID.String(),
			string(e.EntryType),
			e.AccountID.String(),
			string(e.Side),
		}
		if seen[key] {
			return fmt.Errorf("unique constraint violation: (transfer_id=%v, entry_type=%v, account_id=%v, side=%v)",
				e.TransferID, e.EntryType, e.AccountID, e.Side)
		}
		seen[key] = true
	}

	m.entries = append(m.entries, entries...)
	return nil
}

func (m *memStore) GetBalance(_ context.Context, accountID ledger.AccountID) (ledger.Amount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cents int64
	for _, e := range m.entries {
		if e.AccountID == accountID {
			v := parseCents(e.Amount)
			if e.Side == ledger.SideCredit {
				cents += v
			} else {
				cents -= v
			}
		}
	}
	return formatCents(cents), nil
}

func (m *memStore) GetEntriesByTransfer(_ context.Context, transferID uuid.UUID) ([]ledger.LedgerEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []ledger.LedgerEntry
	for _, e := range m.entries {
		if e.TransferID == transferID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memStore) Reconcile(_ context.Context) (ledger.Amount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cents int64
	for _, e := range m.entries {
		v := parseCents(e.Amount)
		if e.Side == ledger.SideCredit {
			cents += v
		} else {
			cents -= v
		}
	}
	return formatCents(cents), nil
}

// ---- decimal helpers ---------------------------------------------------------

// parseCents converts an Amount string like "500.00" to integer cents (50000).
// Individual ledger amounts are always positive per schema constraint.
func parseCents(a ledger.Amount) int64 {
	s := string(a)
	parts := strings.SplitN(s, ".", 2)
	dollars, _ := strconv.ParseInt(parts[0], 10, 64)
	var frac int64
	if len(parts) == 2 {
		f := parts[1]
		if len(f) == 1 {
			f += "0"
		}
		frac, _ = strconv.ParseInt(f[:2], 10, 64)
	}
	return dollars*100 + frac
}

// formatCents converts integer cents to an Amount string like "500.00".
// Handles negative balances (e.g. "-500.00").
func formatCents(cents int64) ledger.Amount {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return ledger.Amount(fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100))
}

// ---- helper -----------------------------------------------------------------

func newSvc(store *memStore) *ledger.Service {
	return ledger.New(store, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

// ---- tests ------------------------------------------------------------------

// TestPostDoubleEntry: creates exactly 2 rows with same movement_id
func TestPostDoubleEntry_CreatesTwoRows(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	if err := svc.PostDoubleEntry(ctx, uuid.New(), uuid.New(), "500.00", "FREE", uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store.mu.Lock()
	n := len(store.entries)
	store.mu.Unlock()

	if n != 2 {
		t.Errorf("expected 2 entries, got %d", n)
	}
}

// TestPostDoubleEntry: one DEBIT, one CREDIT, same amount, same movement_id
func TestPostDoubleEntry_OneDebitOneCreditSameMovementID(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	debitAcct := uuid.New()
	creditAcct := uuid.New()
	transferID := uuid.New()

	if err := svc.PostDoubleEntry(ctx, debitAcct, creditAcct, "500.00", "FREE", transferID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store.mu.Lock()
	entries := make([]ledger.LedgerEntry, len(store.entries))
	copy(entries, store.entries)
	store.mu.Unlock()

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].MovementID != entries[1].MovementID {
		t.Errorf("movement_id mismatch: %v != %v", entries[0].MovementID, entries[1].MovementID)
	}
	if entries[0].Amount != entries[1].Amount {
		t.Errorf("amount mismatch: %v != %v", entries[0].Amount, entries[1].Amount)
	}

	sides := map[ledger.Side]int{}
	for _, e := range entries {
		sides[e.Side]++
	}
	if sides[ledger.SideDebit] != 1 || sides[ledger.SideCredit] != 1 {
		t.Errorf("expected 1 DEBIT + 1 CREDIT, got %v", sides)
	}

	var got ledger.LedgerEntry
	for _, e := range entries {
		if e.Side == ledger.SideDebit {
			got = e
		}
	}
	if got.AccountID != debitAcct {
		t.Errorf("debit entry has wrong account: got %v, want %v", got.AccountID, debitAcct)
	}
}

// TestGetBalance: after posting $500 credit to investor, balance = 500
func TestGetBalance_AfterCredit(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	omnibus := uuid.New()
	investor := uuid.New()

	if err := svc.PostDoubleEntry(ctx, omnibus, investor, "500.00", "FREE", uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bal, err := svc.GetBalance(ctx, investor)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if bal != "500.00" {
		t.Errorf("expected 500.00, got %s", bal)
	}
}

// TestGetBalance: after posting $500 credit then $500 debit, balance = 0
func TestGetBalance_CreditThenDebit_IsZero(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	omnibus := uuid.New()
	investor := uuid.New()

	// Deposit: debit omnibus, credit investor
	if err := svc.PostDoubleEntry(ctx, omnibus, investor, "500.00", "FREE", uuid.New()); err != nil {
		t.Fatalf("deposit error: %v", err)
	}
	// Reversal: debit investor, credit omnibus
	if err := svc.PostDoubleEntry(ctx, investor, omnibus, "500.00", "REVERSAL", uuid.New()); err != nil {
		t.Fatalf("reversal error: %v", err)
	}

	bal, err := svc.GetBalance(ctx, investor)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if bal != "0.00" {
		t.Errorf("expected 0.00, got %s", bal)
	}
}

// TestReconcile: after 10 random PostDoubleEntry calls, Reconcile() = 0
func TestReconcile_AfterMultiplePostings_IsZero(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	memos := []string{"FREE", "REVERSAL", "RETURNED_CHECK_FEE"}
	for i := 0; i < 10; i++ {
		err := svc.PostDoubleEntry(ctx, uuid.New(), uuid.New(), "100.00", memos[i%3], uuid.New())
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}

	sum, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if sum != "0.00" {
		t.Errorf("expected 0.00, got %s", sum)
	}
}

// TestReconcile: empty ledger → Reconcile() = 0
func TestReconcile_EmptyLedger(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	sum, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if sum != "0.00" {
		t.Errorf("expected 0.00, got %s", sum)
	}
}

// TestReconcile: multiple movements for same account — balance accumulates correctly
func TestGetBalance_MultipleMovementsSameAccount(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	omnibus := uuid.New()
	investor := uuid.New()

	for i := 0; i < 3; i++ {
		if err := svc.PostDoubleEntry(ctx, omnibus, investor, "200.00", "FREE", uuid.New()); err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}

	bal, err := svc.GetBalance(ctx, investor)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if bal != "600.00" {
		t.Errorf("expected 600.00, got %s", bal)
	}

	// Invariant: global sum still 0
	sum, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if sum != "0.00" {
		t.Errorf("expected reconcile 0.00, got %s", sum)
	}
}

// TestAtomicity: if InsertEntries fails, neither entry is written
func TestAtomicity_DBError_NeitherEntryWritten(t *testing.T) {
	store := &memStore{failNext: true}
	svc := newSvc(store)
	ctx := context.Background()

	err := svc.PostDoubleEntry(ctx, uuid.New(), uuid.New(), "500.00", "FREE", uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	store.mu.Lock()
	n := len(store.entries)
	store.mu.Unlock()

	if n != 0 {
		t.Errorf("expected 0 entries after failed insert, got %d", n)
	}
}

// TestUniqueConstraint: duplicate (transfer_id, entry_type, account_id, side) fails
func TestUniqueConstraint_DuplicateFails(t *testing.T) {
	store := &memStore{}
	svc := newSvc(store)
	ctx := context.Background()

	debit := uuid.New()
	credit := uuid.New()
	transferID := uuid.New()

	if err := svc.PostDoubleEntry(ctx, debit, credit, "500.00", "FREE", transferID); err != nil {
		t.Fatalf("first post unexpected error: %v", err)
	}

	// Same transferID + same accounts + same memo → unique constraint violation.
	err := svc.PostDoubleEntry(ctx, debit, credit, "500.00", "FREE", transferID)
	if err == nil {
		t.Fatal("expected unique constraint error, got nil")
	}
}
