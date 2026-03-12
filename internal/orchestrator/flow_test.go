package orchestrator_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/funding"
	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/orchestrator"
	"github.com/apex-checkout/check-deposit/internal/vendorclient"
)

// ---- mock: TransferFieldUpdater (extends mockUpdater) ----

type mockFieldUpdater struct {
	mockUpdater
	errorCode        string
	reviewReason     string
	contributionType string
	vssVendorTxID    string
	vssConfidence    float64
}

func (m *mockFieldUpdater) SetErrorCode(_ context.Context, _, code string) error {
	m.errorCode = code
	return nil
}

func (m *mockFieldUpdater) SetReviewReason(_ context.Context, _, reason string) error {
	m.reviewReason = reason
	return nil
}

func (m *mockFieldUpdater) SetContributionType(_ context.Context, _, ct string) error {
	m.contributionType = ct
	return nil
}

func (m *mockFieldUpdater) SetVSSResults(_ context.Context, _ string, vendorTxID string, confidence float64, _ map[string]interface{}) error {
	m.vssVendorTxID = vendorTxID
	m.vssConfidence = confidence
	return nil
}

// ---- mock: VendorServiceClient ----

type mockVSS struct {
	resp *vendorclient.ValidateResponse
	err  error
}

func (m *mockVSS) Validate(_ context.Context, _ vendorclient.ValidateRequest) (*vendorclient.ValidateResponse, error) {
	return m.resp, m.err
}

// ---- mock: FundingServiceClient ----

type mockFunding struct {
	decision *funding.FundingDecision
	err      error
}

func (m *mockFunding) Evaluate(_ context.Context, _ *funding.EvaluateRequest) (*funding.FundingDecision, error) {
	return m.decision, m.err
}

// ---- mock: LedgerService ----

type mockLedger struct {
	posted bool
}

func (m *mockLedger) PostDoubleEntry(_ context.Context, _, _ ledger.AccountID, _ ledger.Amount, _ string, _ uuid.UUID) error {
	m.posted = true
	return nil
}

func (m *mockLedger) GetBalance(_ context.Context, _ ledger.AccountID) (ledger.Amount, error) {
	return "0.00", nil
}

func (m *mockLedger) GetEntries(_ context.Context, _ uuid.UUID) ([]ledger.LedgerEntry, error) {
	return nil, nil
}

func (m *mockLedger) Reconcile(_ context.Context) (ledger.Amount, error) {
	return "0.00", nil
}

// ---- helpers ----

func makeDeps(vss *mockVSS, fs *mockFunding, lg *mockLedger) (orchestrator.Deps, *mockFieldUpdater, *mockEventWriter) {
	updater := &mockFieldUpdater{}
	events := &mockEventWriter{}
	notifier := &mockNotifier{}
	return orchestrator.Deps{
		Updater:  updater,
		Events:   events,
		Notifier: notifier,
		VSS:      vss,
		Funding:  fs,
		Ledger:   lg,
		Log:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}, updater, events
}

func makeTransferDetail() orchestrator.TransferDetail {
	return orchestrator.TransferDetail{
		TransferID:      uuid.New().String(),
		AccountID:       uuid.MustParse("a0000000-0000-0000-0000-000000000001"),
		CorrespondentID: uuid.MustParse("c0000000-0000-0000-0000-000000000001"),
		Amount:          500,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     funding.RulesConfig{DepositLimit: 5000},
	}
}

func strPtr(s string) *string { return &s }

// ---- VSS rejection tests ----

func TestFlow_VSSBlur_RejectsWithErrorCode(t *testing.T) {
	vss := &mockVSS{resp: &vendorclient.ValidateResponse{
		IQAStatus:    "fail",
		IQAErrorType: strPtr("blur"),
		TransactionID: "vss-test-1",
	}}
	deps, updater, events := makeDeps(vss, nil, &mockLedger{})
	td := makeTransferDetail()

	state, err := orchestrator.ProcessDeposit(context.Background(), deps, td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "Rejected" {
		t.Errorf("expected Rejected, got %s", state)
	}
	if updater.errorCode != orchestrator.ErrCodeVSSIQABlur {
		t.Errorf("expected error_code %s, got %s", orchestrator.ErrCodeVSSIQABlur, updater.errorCode)
	}

	// Verify events include vss_result with failure details then state_changed to Rejected
	hasVSSResult := false
	hasRejection := false
	for _, ev := range events.events {
		if ev.step == "vss_result" {
			hasVSSResult = true
			if ev.data["iqa_status"] != "fail" {
				t.Errorf("vss_result event should have iqa_status=fail")
			}
		}
		if ev.step == "state_changed" {
			if to, ok := ev.data["to_state"].(string); ok && to == "Rejected" {
				hasRejection = true
			}
		}
	}
	if !hasVSSResult {
		t.Error("expected vss_result event")
	}
	if !hasRejection {
		t.Error("expected state_changed to Rejected event")
	}
}

func TestFlow_VSSGlare_RejectsWithErrorCode(t *testing.T) {
	vss := &mockVSS{resp: &vendorclient.ValidateResponse{
		IQAStatus:    "fail",
		IQAErrorType: strPtr("glare"),
		TransactionID: "vss-test-2",
	}}
	deps, updater, _ := makeDeps(vss, nil, &mockLedger{})
	td := makeTransferDetail()

	state, err := orchestrator.ProcessDeposit(context.Background(), deps, td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "Rejected" {
		t.Errorf("expected Rejected, got %s", state)
	}
	if updater.errorCode != orchestrator.ErrCodeVSSIQAGlare {
		t.Errorf("expected error_code %s, got %s", orchestrator.ErrCodeVSSIQAGlare, updater.errorCode)
	}
}

func TestFlow_VSSDuplicate_RejectsWithErrorCode(t *testing.T) {
	vss := &mockVSS{resp: &vendorclient.ValidateResponse{
		IQAStatus:             "pass",
		DuplicateFlag:         true,
		DuplicateOriginalTxID: strPtr("vss-original-001"),
		TransactionID:         "vss-test-3",
		MICRData: &vendorclient.MICRData{
			Routing: "021000021", Account: "987654321", CheckNumber: "2001",
		},
		OCRAmount:       500,
		ConfidenceScore: 0.97,
	}}
	deps, updater, _ := makeDeps(vss, nil, &mockLedger{})
	td := makeTransferDetail()

	state, err := orchestrator.ProcessDeposit(context.Background(), deps, td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "Rejected" {
		t.Errorf("expected Rejected, got %s", state)
	}
	if updater.errorCode != orchestrator.ErrCodeVSSDuplicateDetected {
		t.Errorf("expected error_code %s, got %s", orchestrator.ErrCodeVSSDuplicateDetected, updater.errorCode)
	}
}

func TestFlow_Rejected_NoLedgerEntries(t *testing.T) {
	vss := &mockVSS{resp: &vendorclient.ValidateResponse{
		IQAStatus:    "fail",
		IQAErrorType: strPtr("blur"),
		TransactionID: "vss-test-4",
	}}
	lg := &mockLedger{}
	deps, _, _ := makeDeps(vss, nil, lg)
	td := makeTransferDetail()

	state, _ := orchestrator.ProcessDeposit(context.Background(), deps, td)
	if state != "Rejected" {
		t.Errorf("expected Rejected, got %s", state)
	}
	if lg.posted {
		t.Error("ledger should NOT be posted for rejected transfers")
	}
}

func TestFlow_HappyPath_ReachesFundsPosted(t *testing.T) {
	omnibusID := uuid.MustParse("a0000000-0000-0000-0000-000000000010")
	vss := &mockVSS{resp: &vendorclient.ValidateResponse{
		IQAStatus:       "pass",
		DuplicateFlag:   false,
		TransactionID:   "vss-test-5",
		ConfidenceScore: 0.97,
		OCRAmount:       500,
		MICRData: &vendorclient.MICRData{
			Routing: "021000021", Account: "123456789", CheckNumber: "1001",
		},
	}}
	fs := &mockFunding{decision: &funding.FundingDecision{
		Decision:          funding.DecisionApprove,
		ResolvedOmnibusID: omnibusID,
	}}
	lg := &mockLedger{}
	deps, _, events := makeDeps(vss, fs, lg)
	td := makeTransferDetail()

	state, err := orchestrator.ProcessDeposit(context.Background(), deps, td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "FundsPosted" {
		t.Errorf("expected FundsPosted, got %s", state)
	}
	if !lg.posted {
		t.Error("expected ledger to be posted")
	}

	// Verify events include submitted steps
	steps := make(map[string]bool)
	for _, ev := range events.events {
		steps[ev.step] = true
	}
	for _, expected := range []string{"vss_called", "vss_result", "fs_evaluated", "ledger_posted", "state_changed"} {
		if !steps[expected] {
			t.Errorf("expected event step %q", expected)
		}
	}
}

func TestFlow_FSRejection_OverLimit(t *testing.T) {
	vss := &mockVSS{resp: &vendorclient.ValidateResponse{
		IQAStatus:       "pass",
		DuplicateFlag:   false,
		TransactionID:   "vss-test-6",
		ConfidenceScore: 0.97,
		OCRAmount:       5001,
		MICRData: &vendorclient.MICRData{
			Routing: "021000021", Account: "123456789", CheckNumber: "1001",
		},
	}}
	fs := &mockFunding{decision: &funding.FundingDecision{
		Decision:   funding.DecisionReject,
		ReasonCode: "FS_OVER_DEPOSIT_LIMIT",
	}}
	lg := &mockLedger{}
	deps, updater, _ := makeDeps(vss, fs, lg)
	td := makeTransferDetail()
	td.Amount = 5001

	state, err := orchestrator.ProcessDeposit(context.Background(), deps, td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "Rejected" {
		t.Errorf("expected Rejected, got %s", state)
	}
	if updater.errorCode != "FS_OVER_DEPOSIT_LIMIT" {
		t.Errorf("expected FS_OVER_DEPOSIT_LIMIT, got %s", updater.errorCode)
	}
	if lg.posted {
		t.Error("ledger should NOT be posted for FS-rejected transfers")
	}
}

// ---- Error taxonomy tests ----

func TestDepositError_UserMessages(t *testing.T) {
	tests := []struct {
		code    string
		wantMsg string
	}{
		{orchestrator.ErrCodeVSSIQABlur, "Image too blurry — hold steady on a flat surface with good lighting."},
		{orchestrator.ErrCodeVSSIQAGlare, "Glare detected — avoid direct light on the check."},
		{orchestrator.ErrCodeVSSDuplicateDetected, "This check has already been deposited."},
		{orchestrator.ErrCodeFSOverDepositLimit, "Maximum single deposit is $5,000. Contact support for larger amounts."},
		{orchestrator.ErrCodeFSAccountIneligible, "Check deposits are not available for this account type."},
		{orchestrator.ErrCodeVSSMICRReadFail, ""},       // operator-only, no user message
		{orchestrator.ErrCodeSysLedgerPostFail, ""},      // internal-only
		{orchestrator.ErrCodeSysInvalidTransition, ""},   // internal-only
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := orchestrator.UserMessageForCode(tt.code)
			if got != tt.wantMsg {
				t.Errorf("UserMessageForCode(%s) = %q, want %q", tt.code, got, tt.wantMsg)
			}
		})
	}
}

func TestNewDepositError_Structure(t *testing.T) {
	de := orchestrator.NewDepositError(orchestrator.ErrCodeVSSIQABlur, map[string]interface{}{"iqa_error_type": "blur"})
	if de.Code != orchestrator.ErrCodeVSSIQABlur {
		t.Errorf("Code = %s, want %s", de.Code, orchestrator.ErrCodeVSSIQABlur)
	}
	if de.UserMsg == "" {
		t.Error("UserMsg should not be empty for VSS_IQA_BLUR")
	}
	if de.Message == "" {
		t.Error("Message should not be empty")
	}
	if de.Detail["iqa_error_type"] != "blur" {
		t.Error("Detail should preserve passed metadata")
	}
	if de.Error() == "" {
		t.Error("Error() should return non-empty string")
	}
}
