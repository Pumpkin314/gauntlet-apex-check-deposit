package returns_test

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

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/returns"
)

// ---- in-memory TransferRepository ------------------------------------------

type memTransferRepo struct {
	mu       sync.Mutex
	transfer *returns.Transfer
	events   []memEvent
	notifies []map[string]interface{}
}

type memEvent struct {
	step  string
	actor string
	data  map[string]interface{}
}

func (m *memTransferRepo) GetTransfer(_ context.Context, id string) (*returns.Transfer, error) {
	if m.transfer == nil || m.transfer.ID != id {
		return nil, returns.ErrNotFound("transfer " + id)
	}
	// Return a copy so the handler cannot mutate our state.
	cp := *m.transfer
	return &cp, nil
}

func (m *memTransferRepo) UpdateState(_ context.Context, id, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transfer.ID != id {
		return errors.New("transfer not found")
	}
	if m.transfer.State != from {
		return fmt.Errorf("optimistic lock: expected %s got %s", from, m.transfer.State)
	}
	m.transfer.State = to
	return nil
}

func (m *memTransferRepo) WriteEvent(_ context.Context, id, step, actor string, data map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, memEvent{step: step, actor: actor, data: data})
	return nil
}

func (m *memTransferRepo) Notify(_ context.Context, id string, payload map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifies = append(m.notifies, payload)
	return nil
}

// ---- in-memory CorrespondentRepository -------------------------------------

type memCorrespondentRepo struct {
	fee float64
}

func (m *memCorrespondentRepo) GetReturnedCheckFee(_ context.Context, _ uuid.UUID) (float64, error) {
	return m.fee, nil
}

// ---- in-memory AccountRepository -------------------------------------------

type memAccountRepo struct {
	mu         sync.Mutex
	feeAcctID  uuid.UUID
	statuses   map[uuid.UUID]string
}

func newMemAccountRepo(feeID uuid.UUID) *memAccountRepo {
	return &memAccountRepo{feeAcctID: feeID, statuses: make(map[uuid.UUID]string)}
}

func (m *memAccountRepo) GetFeeAccountID(_ context.Context) (uuid.UUID, error) {
	return m.feeAcctID, nil
}

func (m *memAccountRepo) SetAccountStatus(_ context.Context, id uuid.UUID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[id] = status
	return nil
}

// ---- in-memory NotificationRepository --------------------------------------

type memNotificationRepo struct {
	mu            sync.Mutex
	notifications []memNotification
}

type memNotification struct {
	accountID  uuid.UUID
	transferID uuid.UUID
	notifType  string
	message    string
}

func (m *memNotificationRepo) CreateNotification(_ context.Context, accountID, transferID uuid.UUID, notifType, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, memNotification{
		accountID:  accountID,
		transferID: transferID,
		notifType:  notifType,
		message:    message,
	})
	return nil
}

// ---- in-memory ledger.Store ------------------------------------------------

type memLedgerStore struct {
	mu      sync.Mutex
	entries []ledger.LedgerEntry
}

func (m *memLedgerStore) InsertEntries(_ context.Context, entries []ledger.LedgerEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entries...)
	return nil
}

func (m *memLedgerStore) GetBalance(_ context.Context, accountID ledger.AccountID) (ledger.Amount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cents int64
	for _, e := range m.entries {
		if e.AccountID != accountID {
			continue
		}
		v := parseCents(e.Amount)
		if e.Side == ledger.SideCredit {
			cents += v
		} else {
			cents -= v
		}
	}
	return formatCents(cents), nil
}

func (m *memLedgerStore) GetEntriesByTransfer(_ context.Context, transferID uuid.UUID) ([]ledger.LedgerEntry, error) {
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

func (m *memLedgerStore) Reconcile(_ context.Context) (ledger.Amount, error) {
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

// ---- decimal helpers -------------------------------------------------------

func parseCents(a ledger.Amount) int64 {
	s := string(a)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
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
	total := dollars*100 + frac
	if neg {
		return -total
	}
	return total
}

func formatCents(cents int64) ledger.Amount {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return ledger.Amount(fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100))
}

// ---- test helpers ----------------------------------------------------------

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func buildDeps(transferRepo *memTransferRepo, corrRepo *memCorrespondentRepo, acctRepo *memAccountRepo, notifRepo *memNotificationRepo, ls *memLedgerStore) returns.Deps {
	svc := ledger.New(ls, newLogger())
	return returns.Deps{
		Transfers:      transferRepo,
		Correspondents: corrRepo,
		Accounts:       acctRepo,
		Notifications:  notifRepo,
		Ledger:         svc,
		Log:            newLogger(),
	}
}

// newTransfer creates a test transfer in the given state.
func newTransfer(state string) (investorID, omnibusID uuid.UUID, t *returns.Transfer) {
	investorID = uuid.New()
	omnibusID = uuid.New()
	corrID := uuid.New()
	t = &returns.Transfer{
		ID:              uuid.New().String(),
		AccountID:       investorID,
		OmnibusID:       omnibusID,
		CorrespondentID: corrID,
		Amount:          500.00,
		State:           state,
	}
	return
}

// ---- tests -----------------------------------------------------------------

// TestProcessReturn_FundsPosted: happy path from FundsPosted → Returned.
// Expects 4 ledger entries (reversal pair + fee pair).
func TestProcessReturn_FundsPosted(t *testing.T) {
	investorID, omnibusID, transfer := newTransfer("FundsPosted")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// State must be Returned
	if transferRepo.transfer.State != "Returned" {
		t.Errorf("expected state Returned, got %s", transferRepo.transfer.State)
	}

	// Exactly 4 ledger entries
	ls.mu.Lock()
	n := len(ls.entries)
	entries := make([]ledger.LedgerEntry, n)
	copy(entries, ls.entries)
	ls.mu.Unlock()

	if n != 4 {
		t.Fatalf("expected 4 ledger entries, got %d", n)
	}

	// Classify entries by EntryType
	var reversals, fees []ledger.LedgerEntry
	for _, e := range entries {
		switch e.EntryType {
		case ledger.EntryTypeReversal:
			reversals = append(reversals, e)
		case ledger.EntryTypeFee:
			fees = append(fees, e)
		}
	}

	if len(reversals) != 2 {
		t.Errorf("expected 2 REVERSAL entries, got %d", len(reversals))
	}
	if len(fees) != 2 {
		t.Errorf("expected 2 FEE entries, got %d", len(fees))
	}

	// Reversal: DEBIT investor, CREDIT omnibus
	for _, e := range reversals {
		if e.Side == ledger.SideDebit && e.AccountID != investorID {
			t.Errorf("reversal DEBIT should be investor, got %v", e.AccountID)
		}
		if e.Side == ledger.SideCredit && e.AccountID != omnibusID {
			t.Errorf("reversal CREDIT should be omnibus, got %v", e.AccountID)
		}
		if e.Amount != "500.00" {
			t.Errorf("reversal amount: expected 500.00, got %s", e.Amount)
		}
	}

	// Fee: DEBIT investor, CREDIT FEE-APEX
	for _, e := range fees {
		if e.Side == ledger.SideDebit && e.AccountID != investorID {
			t.Errorf("fee DEBIT should be investor, got %v", e.AccountID)
		}
		if e.Side == ledger.SideCredit && e.AccountID != feeAcctID {
			t.Errorf("fee CREDIT should be feeAcctID, got %v", e.AccountID)
		}
		if e.Amount != "30.00" {
			t.Errorf("fee amount: expected 30.00 from config, got %s", e.Amount)
		}
	}

	// Reversal pair shares a movement_id
	if reversals[0].MovementID != reversals[1].MovementID {
		t.Error("reversal entries must share movement_id")
	}
	// Fee pair shares a movement_id
	if fees[0].MovementID != fees[1].MovementID {
		t.Error("fee entries must share movement_id")
	}
	// But reversal and fee movement_ids are different
	if reversals[0].MovementID == fees[0].MovementID {
		t.Error("reversal and fee must have distinct movement_ids")
	}
}

// TestProcessReturn_Completed: return from Completed state also posts 4 entries.
func TestProcessReturn_Completed(t *testing.T) {
	_, _, transfer := newTransfer("Completed")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R02",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transferRepo.transfer.State != "Returned" {
		t.Errorf("expected Returned, got %s", transferRepo.transfer.State)
	}

	ls.mu.Lock()
	n := len(ls.entries)
	ls.mu.Unlock()
	if n != 4 {
		t.Errorf("expected 4 ledger entries, got %d", n)
	}
}

// TestProcessReturn_FeeFromConfig: fee amount must come from config, not hardcoded.
func TestProcessReturn_FeeFromConfig(t *testing.T) {
	_, _, transfer := newTransfer("FundsPosted")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 25.00} // non-default fee
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ls.mu.Lock()
	entries := make([]ledger.LedgerEntry, len(ls.entries))
	copy(entries, ls.entries)
	ls.mu.Unlock()

	for _, e := range entries {
		if e.EntryType == ledger.EntryTypeFee {
			if e.Amount != "25.00" {
				t.Errorf("fee amount: expected 25.00 from config, got %s", e.Amount)
			}
		}
	}
}

// TestProcessReturn_NegativeBalance_SetsCollections: if investor goes negative, flag COLLECTIONS.
func TestProcessReturn_NegativeBalance_SetsCollections(t *testing.T) {
	investorID, _, transfer := newTransfer("FundsPosted")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}
	// No deposit entries — balance starts at 0, goes negative after reversal+fee.

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	acctRepo.mu.Lock()
	status := acctRepo.statuses[investorID]
	acctRepo.mu.Unlock()

	if status != "COLLECTIONS" {
		t.Errorf("expected account status COLLECTIONS, got %q", status)
	}
}

// TestProcessReturn_Idempotent: calling ProcessReturn on an already-Returned transfer is a no-op.
func TestProcessReturn_Idempotent(t *testing.T) {
	_, _, transfer := newTransfer("Returned")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	}); err != nil {
		t.Fatalf("unexpected error on idempotent re-return: %v", err)
	}

	ls.mu.Lock()
	n := len(ls.entries)
	ls.mu.Unlock()

	if n != 0 {
		t.Errorf("idempotent re-return must post 0 new ledger entries, got %d", n)
	}

	if len(notifRepo.notifications) != 0 {
		t.Errorf("idempotent re-return must create 0 notifications, got %d", len(notifRepo.notifications))
	}
}

// TestProcessReturn_InvalidState_Rejected: Rejected transfer → ErrInvalidReturnState.
func TestProcessReturn_InvalidState_Rejected(t *testing.T) {
	_, _, transfer := newTransfer("Rejected")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	})

	var stateErr *returns.ErrInvalidReturnState
	if !errors.As(err, &stateErr) {
		t.Errorf("expected ErrInvalidReturnState, got %T: %v", err, err)
	}
}

// TestProcessReturn_Notification: return creates a RETURN_RECEIVED notification.
func TestProcessReturn_Notification(t *testing.T) {
	investorID, _, transfer := newTransfer("FundsPosted")
	transferID := uuid.MustParse(transfer.ID)
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	// Pre-seed deposit so balance is positive after return (avoids COLLECTIONS path).
	svc := ledger.New(ls, newLogger())
	_ = svc.PostDoubleEntry(context.Background(), transfer.OmnibusID, investorID, "500.00", "FREE", transferID)

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notifRepo.mu.Lock()
	notifs := notifRepo.notifications
	notifRepo.mu.Unlock()

	if len(notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifs))
	}
	if notifs[0].accountID != investorID {
		t.Errorf("notification account_id: got %v, want %v", notifs[0].accountID, investorID)
	}
	if notifs[0].notifType != "RETURN_RECEIVED" {
		t.Errorf("notification type: got %s, want RETURN_RECEIVED", notifs[0].notifType)
	}
	if notifs[0].message == "" {
		t.Error("notification message must not be empty")
	}
}

// TestProcessReturn_Events: return_received and return_processed events are written.
func TestProcessReturn_Events(t *testing.T) {
	_, _, transfer := newTransfer("FundsPosted")
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := make(map[string]bool)
	for _, e := range transferRepo.events {
		steps[e.step] = true
	}

	if !steps["return_received"] {
		t.Error("expected return_received event")
	}
	if !steps["return_processed"] {
		t.Error("expected return_processed event")
	}
	if !steps["state_changed"] {
		t.Error("expected state_changed event")
	}
}

// TestProcessReturn_LedgerReconciliation: global ledger sum = 0 after return.
func TestProcessReturn_LedgerReconciliation(t *testing.T) {
	investorID, omnibusID, transfer := newTransfer("FundsPosted")
	transferID := uuid.MustParse(transfer.ID)
	feeAcctID := uuid.New()
	ls := &memLedgerStore{}

	// Pre-seed deposit entries (DEBIT omnibus, CREDIT investor).
	svc := ledger.New(ls, newLogger())
	if err := svc.PostDoubleEntry(context.Background(), omnibusID, investorID, "500.00", "FREE", transferID); err != nil {
		t.Fatalf("seed deposit: %v", err)
	}

	transferRepo := &memTransferRepo{transfer: transfer}
	corrRepo := &memCorrespondentRepo{fee: 30.00}
	acctRepo := newMemAccountRepo(feeAcctID)
	notifRepo := &memNotificationRepo{}
	deps := buildDeps(transferRepo, corrRepo, acctRepo, notifRepo, ls)

	if err := returns.ProcessReturn(context.Background(), deps, returns.Input{
		TransferID: transfer.ID,
		ReasonCode: "R01",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After deposit (2 entries) + reversal (2 entries) + fee (2 entries) = 6 total.
	ls.mu.Lock()
	n := len(ls.entries)
	ls.mu.Unlock()
	if n != 6 {
		t.Errorf("expected 6 total entries (deposit + reversal + fee), got %d", n)
	}

	// Global reconciliation must still be 0.
	sum, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if sum != "0.00" {
		t.Errorf("reconciliation: expected 0.00, got %s", sum)
	}

	// Investor balance: 500 (deposit) - 500 (reversal) - 30 (fee) = -30.
	bal, err := svc.GetBalance(context.Background(), investorID)
	if err != nil {
		t.Fatalf("get balance error: %v", err)
	}
	if bal != "-30.00" {
		t.Errorf("investor balance: expected -30.00, got %s", bal)
	}
}
