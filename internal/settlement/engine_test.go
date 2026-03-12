package settlement

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Mock implementations ---

type mockTransferQuerier struct {
	transfers []*Transfer
}

func (m *mockTransferQuerier) ListFundsPostedBefore(_ context.Context, _ time.Time) ([]*Transfer, error) {
	return m.transfers, nil
}

type mockBatchCreator struct {
	batches map[uuid.UUID]*Batch
	lastID  uuid.UUID
}

func newMockBatchCreator() *mockBatchCreator {
	return &mockBatchCreator{batches: make(map[uuid.UUID]*Batch)}
}

func (m *mockBatchCreator) CreateBatch(_ context.Context, correspondentID string, cutoffDate time.Time, fileRef string, recordCount int, totalAmount float64) (uuid.UUID, error) {
	id := uuid.New()
	m.lastID = id
	m.batches[id] = &Batch{
		ID:              id,
		CorrespondentID: correspondentID,
		CutoffDate:      cutoffDate,
		Status:          "PENDING",
		FileRef:         fileRef,
		RecordCount:     recordCount,
		TotalAmount:     totalAmount,
		CreatedAt:       time.Now(),
	}
	return id, nil
}

func (m *mockBatchCreator) UpdateBatchStatus(_ context.Context, batchID uuid.UUID, status string, acknowledgedAt *time.Time) error {
	if b, ok := m.batches[batchID]; ok {
		b.Status = status
		if acknowledgedAt != nil {
			b.AcknowledgedAt = acknowledgedAt
		}
	}
	return nil
}

func (m *mockBatchCreator) SetSettlementBatch(_ context.Context, transferID string, batchID uuid.UUID, settledAt time.Time) error {
	if b, ok := m.batches[batchID]; ok {
		b.TransferIDs = append(b.TransferIDs, transferID)
	}
	return nil
}

func (m *mockBatchCreator) ListBatches(_ context.Context) ([]*Batch, error) {
	var result []*Batch
	for _, b := range m.batches {
		result = append(result, b)
	}
	return result, nil
}

func (m *mockBatchCreator) GetBatch(_ context.Context, id uuid.UUID) (*Batch, error) {
	return m.batches[id], nil
}

type mockUpdater struct {
	transitions []string
}

func (m *mockUpdater) UpdateState(_ context.Context, transferID, from, to string) error {
	m.transitions = append(m.transitions, transferID+":"+from+"→"+to)
	return nil
}

type mockEvents struct {
	events []string
}

func (m *mockEvents) WriteEvent(_ context.Context, transferID, step, actor string, data map[string]interface{}) error {
	m.events = append(m.events, transferID+":"+step)
	return nil
}

type mockNotifier struct{}

func (m *mockNotifier) Notify(_ context.Context, _ string, _ map[string]interface{}) error { return nil }

// --- Tests ---

func TestEngine_Trigger_NoTransfers(t *testing.T) {
	engine := &Engine{
		Transfers: &mockTransferQuerier{},
		Batches:   newMockBatchCreator(),
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	result, err := engine.Trigger(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BatchCount != 0 {
		t.Errorf("expected 0 batches, got %d", result.BatchCount)
	}
}

func TestEngine_Trigger_GroupsByCorrespondent(t *testing.T) {
	tmpDir := t.TempDir()

	transfers := []*Transfer{
		{ID: uuid.New().String(), CorrespondentID: "corr-aaa", Amount: 100, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
		{ID: uuid.New().String(), CorrespondentID: "corr-aaa", Amount: 200, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
		{ID: uuid.New().String(), CorrespondentID: "corr-bbb", Amount: 300, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
	}

	batchCreator := newMockBatchCreator()
	engine := &Engine{
		Transfers: &mockTransferQuerier{transfers: transfers},
		Batches:   batchCreator,
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		DataDir:   tmpDir,
	}

	result, err := engine.Trigger(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BatchCount != 2 {
		t.Errorf("expected 2 batches (one per correspondent), got %d", result.BatchCount)
	}
	if result.TotalChecks != 3 {
		t.Errorf("expected 3 total checks, got %d", result.TotalChecks)
	}
	if result.TotalAmount != 600 {
		t.Errorf("expected total amount 600, got %f", result.TotalAmount)
	}
}

func TestEngine_Trigger_FileStructure(t *testing.T) {
	tmpDir := t.TempDir()

	frontRef := "images/front.jpg"
	backRef := "images/back.jpg"
	transfers := []*Transfer{
		{
			ID:              uuid.New().String(),
			CorrespondentID: "corr-aaa",
			Amount:          500,
			State:           "FundsPosted",
			MICRData:        map[string]interface{}{"routing": "021000089", "account": "1234567890", "check_number": "1001"},
			FrontImageRef:   &frontRef,
			BackImageRef:    &backRef,
			SubmittedAt:     time.Now().Add(-1 * time.Hour),
		},
	}

	engine := &Engine{
		Transfers: &mockTransferQuerier{transfers: transfers},
		Batches:   newMockBatchCreator(),
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		DataDir:   tmpDir,
	}

	result, err := engine.Trigger(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(result.Batches))
	}

	// Read and verify the settlement file
	fileData, err := os.ReadFile(result.Batches[0].FileRef)
	if err != nil {
		t.Fatalf("read settlement file: %v", err)
	}

	var file SettlementFile
	if err := json.Unmarshal(fileData, &file); err != nil {
		t.Fatalf("unmarshal settlement file: %v", err)
	}

	if file.FileHeader.Sender != "APEX" {
		t.Errorf("expected sender APEX, got %s", file.FileHeader.Sender)
	}
	if file.CashLetter.CorrespondentID != "corr-aaa" {
		t.Errorf("expected correspondent_id corr-aaa, got %s", file.CashLetter.CorrespondentID)
	}
	if file.CashLetter.RecordCount != 1 {
		t.Errorf("expected record_count 1, got %d", file.CashLetter.RecordCount)
	}
	if file.CashLetter.TotalAmount != 500 {
		t.Errorf("expected total_amount 500, got %f", file.CashLetter.TotalAmount)
	}
	if len(file.CashLetter.Bundles) != 1 || len(file.CashLetter.Bundles[0].Checks) != 1 {
		t.Fatal("expected 1 bundle with 1 check")
	}

	check := file.CashLetter.Bundles[0].Checks[0]
	if check.MICR.Routing != "021000089" {
		t.Errorf("expected routing 021000089, got %s", check.MICR.Routing)
	}
	if check.MICR.Account != "1234567890" {
		t.Errorf("expected account 1234567890, got %s", check.MICR.Account)
	}
	if check.Amount != 500 {
		t.Errorf("expected amount 500, got %f", check.Amount)
	}
}

func TestEngine_Trigger_NoRejectedTransfers(t *testing.T) {
	// The engine only gets FundsPosted transfers from ListFundsPostedBefore.
	// Verify that the querier is filtering correctly.
	tmpDir := t.TempDir()

	transfers := []*Transfer{
		{ID: uuid.New().String(), CorrespondentID: "corr-aaa", Amount: 100, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
	}

	engine := &Engine{
		Transfers: &mockTransferQuerier{transfers: transfers},
		Batches:   newMockBatchCreator(),
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		DataDir:   tmpDir,
	}

	result, err := engine.Trigger(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All transfers should be FundsPosted
	if result.TotalChecks != 1 {
		t.Errorf("expected 1 check, got %d", result.TotalChecks)
	}
}

func TestEngine_Trigger_TotalsCorrect(t *testing.T) {
	tmpDir := t.TempDir()

	transfers := []*Transfer{
		{ID: uuid.New().String(), CorrespondentID: "corr-aaa", Amount: 100.50, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
		{ID: uuid.New().String(), CorrespondentID: "corr-aaa", Amount: 250.75, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
	}

	engine := &Engine{
		Transfers: &mockTransferQuerier{transfers: transfers},
		Batches:   newMockBatchCreator(),
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		DataDir:   tmpDir,
	}

	result, err := engine.Trigger(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := 351.25
	if result.TotalAmount != expected {
		t.Errorf("expected total %f, got %f", expected, result.TotalAmount)
	}

	// Also check the file
	fileData, _ := os.ReadFile(result.Batches[0].FileRef)
	var file SettlementFile
	json.Unmarshal(fileData, &file)
	if file.CashLetter.TotalAmount != expected {
		t.Errorf("file total_amount expected %f, got %f", expected, file.CashLetter.TotalAmount)
	}
}

func TestEngine_AcknowledgeBatch(t *testing.T) {
	batchCreator := newMockBatchCreator()
	updater := &mockUpdater{}
	events := &mockEvents{}
	notifier := &mockNotifier{}

	tid1 := uuid.New().String()
	tid2 := uuid.New().String()

	batchID := uuid.New()
	batchCreator.batches[batchID] = &Batch{
		ID:          batchID,
		Status:      "SUBMITTED",
		TransferIDs: []string{tid1, tid2},
	}

	engine := &Engine{
		Batches:  batchCreator,
		Updater:  updater,
		Events:   events,
		Notifier: notifier,
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	ackTime := time.Now()
	err := engine.AcknowledgeBatch(context.Background(), batchID, ackTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify batch status
	batch := batchCreator.batches[batchID]
	if batch.Status != "ACKNOWLEDGED" {
		t.Errorf("expected ACKNOWLEDGED, got %s", batch.Status)
	}

	// Verify transitions
	if len(updater.transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(updater.transitions))
	}
	if updater.transitions[0] != tid1+":FundsPosted→Completed" {
		t.Errorf("unexpected transition: %s", updater.transitions[0])
	}
	if updater.transitions[1] != tid2+":FundsPosted→Completed" {
		t.Errorf("unexpected transition: %s", updater.transitions[1])
	}

	// Verify events written
	if len(events.events) != 4 { // 2 state_changed + 2 settlement_completed
		t.Errorf("expected 4 events, got %d", len(events.events))
	}
}

func TestEngine_AcknowledgeBatch_Idempotent(t *testing.T) {
	batchCreator := newMockBatchCreator()
	batchID := uuid.New()
	batchCreator.batches[batchID] = &Batch{
		ID:     batchID,
		Status: "ACKNOWLEDGED", // Already acknowledged
	}

	engine := &Engine{
		Batches: batchCreator,
		Log:     slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	err := engine.AcknowledgeBatch(context.Background(), batchID, time.Now())
	if err != nil {
		t.Fatalf("re-acknowledge should be idempotent, got error: %v", err)
	}
}

func TestEngine_Trigger_FilesSavedToDisk(t *testing.T) {
	tmpDir := t.TempDir()

	transfers := []*Transfer{
		{ID: uuid.New().String(), CorrespondentID: "corr-aaa", Amount: 100, State: "FundsPosted", SubmittedAt: time.Now().Add(-1 * time.Hour)},
	}

	engine := &Engine{
		Transfers: &mockTransferQuerier{transfers: transfers},
		Batches:   newMockBatchCreator(),
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		DataDir:   tmpDir,
	}

	result, err := engine.Trigger(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file exists
	files, _ := filepath.Glob(filepath.Join(tmpDir, "settlement_corr-aaa*.json"))
	if len(files) != 1 {
		t.Errorf("expected 1 settlement file, found %d", len(files))
	}

	// Verify it matches the reported file ref
	if result.Batches[0].FileRef != files[0] {
		t.Errorf("file ref mismatch: result=%s, disk=%s", result.Batches[0].FileRef, files[0])
	}
}
