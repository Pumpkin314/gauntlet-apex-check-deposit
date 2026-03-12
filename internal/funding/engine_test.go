package funding_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/apex-checkout/check-deposit/internal/funding"
	"github.com/google/uuid"
)

// -- seed UUIDs matching db/seed.sql --

var (
	alphaCorrespondentID = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
	betaCorrespondentID  = uuid.MustParse("c0000000-0000-0000-0000-000000000002")

	omnibusAlphaID = uuid.MustParse("a0000000-0000-0000-0000-000000000010")
	omnibusBetaID  = uuid.MustParse("a0000000-0000-0000-0000-000000000011")

	alpha001ID = uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	alphaIRAID = uuid.MustParse("a0000000-0000-0000-0000-000000000006")
	betaIRAID  = uuid.MustParse("a0000000-0000-0000-0000-000000000009")
	beta002ID  = uuid.MustParse("a0000000-0000-0000-0000-000000000008")
)

// mockResolver implements funding.OmnibusResolver using a fixed map.
type mockResolver struct {
	m map[uuid.UUID]uuid.UUID
}

func (r *mockResolver) GetOmnibusForCorrespondent(_ context.Context, corrID uuid.UUID) (uuid.UUID, error) {
	if id, ok := r.m[corrID]; ok {
		return id, nil
	}
	return uuid.UUID{}, fmt.Errorf("mock: no omnibus for %s", corrID)
}

func newResolver() *mockResolver {
	return &mockResolver{m: map[uuid.UUID]uuid.UUID{
		alphaCorrespondentID: omnibusAlphaID,
		betaCorrespondentID:  omnibusBetaID,
	}}
}

// alphaRules is Alpha Brokerage's rules_config: deposit_limit=5000, no ineligible types.
func alphaRules() funding.RulesConfig {
	return funding.RulesConfig{
		DepositLimit:           5000,
		IneligibleAccountTypes: []string{},
		ContributionCap:        7000,
		Fees: funding.FeeConfig{ReturnedCheck: 30, Currency: "USD"},
	}
}

// betaRules is Beta Wealth's rules_config: deposit_limit=5000, IRA ineligible.
func betaRules() funding.RulesConfig {
	return funding.RulesConfig{
		DepositLimit:           5000,
		IneligibleAccountTypes: []string{"IRA"},
		ContributionCap:        7000,
		Fees: funding.FeeConfig{ReturnedCheck: 30, Currency: "USD"},
	}
}

// TestApprove_HappyPath: $500, INDIVIDUAL account, Alpha → APPROVE
func TestApprove_HappyPath(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionApprove {
		t.Fatalf("want APPROVE, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
}

// TestApprove_ExactLimit: $5000 → APPROVE (boundary: ≤ not <)
func TestApprove_ExactLimit(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          5000.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionApprove {
		t.Fatalf("want APPROVE at exact limit, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
}

// TestReject_OverLimit: $5001 → REJECT with FS_OVER_DEPOSIT_LIMIT
func TestReject_OverLimit(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       beta002ID,
		CorrespondentID: betaCorrespondentID,
		Amount:          5001.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     betaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionReject {
		t.Fatalf("want REJECT, got %s", d.Decision)
	}
	if d.ReasonCode != "FS_OVER_DEPOSIT_LIMIT" {
		t.Fatalf("want FS_OVER_DEPOSIT_LIMIT, got %s", d.ReasonCode)
	}
}

// TestReject_IneligibleIRA: IRA at Beta → REJECT with FS_ACCOUNT_INELIGIBLE
func TestReject_IneligibleIRA(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       betaIRAID,
		CorrespondentID: betaCorrespondentID,
		Amount:          500.00,
		AccountType:     "IRA",
		AccountStatus:   "ACTIVE",
		RulesConfig:     betaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionReject {
		t.Fatalf("want REJECT, got %s", d.Decision)
	}
	if d.ReasonCode != "FS_ACCOUNT_INELIGIBLE" {
		t.Fatalf("want FS_ACCOUNT_INELIGIBLE, got %s", d.ReasonCode)
	}
}

// TestApprove_IRA_Alpha: IRA at Alpha → APPROVE, contribution_type = INDIVIDUAL
func TestApprove_IRA_Alpha(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alphaIRAID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00,
		AccountType:     "IRA",
		AccountStatus:   "ACTIVE",
		ContributionType: "", // not specified — should be defaulted
		RulesConfig:     alphaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionApprove {
		t.Fatalf("want APPROVE for IRA at Alpha, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
	if d.ContributionType != "INDIVIDUAL" {
		t.Fatalf("want contribution_type=INDIVIDUAL, got %q", d.ContributionType)
	}
}

// TestOmnibusResolution_Alpha: Alpha account → resolves to OMNIBUS-ALPHA
func TestOmnibusResolution_Alpha(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionApprove {
		t.Fatalf("want APPROVE, got %s", d.Decision)
	}
	if d.ResolvedOmnibusID != omnibusAlphaID {
		t.Fatalf("want omnibus OMNIBUS-ALPHA (%s), got %s", omnibusAlphaID, d.ResolvedOmnibusID)
	}
}

// TestOmnibusResolution_Beta: Beta account → resolves to OMNIBUS-BETA
func TestOmnibusResolution_Beta(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       beta002ID,
		CorrespondentID: betaCorrespondentID,
		Amount:          500.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     betaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionApprove {
		t.Fatalf("want APPROVE, got %s", d.Decision)
	}
	if d.ResolvedOmnibusID != omnibusBetaID {
		t.Fatalf("want omnibus OMNIBUS-BETA (%s), got %s", omnibusBetaID, d.ResolvedOmnibusID)
	}
}

// -- mock: DuplicateChecker --

type mockDupChecker struct {
	found bool
	err   error
}

func (m *mockDupChecker) HasRecentTransfer(_ context.Context, _ uuid.UUID, _ float64, _ time.Duration) (bool, error) {
	return m.found, m.err
}

// TestFlagForReview_MICR: MICR failure → FLAG_FOR_REVIEW with VSS_MICR_READ_FAIL
func TestFlagForReview_MICR(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
		VSSResult:       &funding.VSSResult{MICRReadable: false, OCRAmount: 0},
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionFlagForReview {
		t.Fatalf("want FLAG_FOR_REVIEW, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
	if d.ReasonCode != "VSS_MICR_READ_FAIL" {
		t.Fatalf("want reason VSS_MICR_READ_FAIL, got %q", d.ReasonCode)
	}
}

// TestFlagForReview_AmountMismatch: OCR amount differs → FLAG_FOR_REVIEW with VSS_AMOUNT_MISMATCH
func TestFlagForReview_AmountMismatch(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00, // submitted amount
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
		VSSResult:       &funding.VSSResult{MICRReadable: true, OCRAmount: 250.00}, // OCR disagrees
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionFlagForReview {
		t.Fatalf("want FLAG_FOR_REVIEW, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
	if d.ReasonCode != "VSS_AMOUNT_MISMATCH" {
		t.Fatalf("want reason VSS_AMOUNT_MISMATCH, got %q", d.ReasonCode)
	}
}

// TestReject_FSDuplicate: same account + same amount within window → REJECT with FS_DUPLICATE_DEPOSIT
func TestReject_FSDuplicate(t *testing.T) {
	engine := funding.NewEngine(newResolver()).
		WithDuplicateChecker(&mockDupChecker{found: true}, 5*time.Minute)
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionReject {
		t.Fatalf("want REJECT, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
	if d.ReasonCode != "FS_DUPLICATE_DEPOSIT" {
		t.Fatalf("want reason FS_DUPLICATE_DEPOSIT, got %q", d.ReasonCode)
	}
}

// TestVSSRulesSkip_WhenNoVSSResult: existing tests unaffected — no VSSResult → rules skip
func TestVSSRulesSkip_WhenNoVSSResult(t *testing.T) {
	engine := funding.NewEngine(newResolver())
	req := &funding.EvaluateRequest{
		TransferID:      uuid.New(),
		AccountID:       alpha001ID,
		CorrespondentID: alphaCorrespondentID,
		Amount:          500.00,
		AccountType:     "INDIVIDUAL",
		AccountStatus:   "ACTIVE",
		RulesConfig:     alphaRules(),
		VSSResult:       nil, // not set — VSS rules must be no-ops
	}
	d, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Decision != funding.DecisionApprove {
		t.Fatalf("want APPROVE when VSSResult is nil, got %s (reason: %s)", d.Decision, d.ReasonCode)
	}
}

// TestRules_IndependentlyTestable verifies the Rule interface pattern —
// each rule can be constructed and called in isolation.
func TestRules_IndependentlyTestable(t *testing.T) {
	ctx := context.Background()

	t.Run("AccountEligibilityRule_InactiveAccount", func(t *testing.T) {
		r := funding.AccountEligibilityRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{
			AccountType:   "INDIVIDUAL",
			AccountStatus: "SUSPENDED",
			RulesConfig:   alphaRules(),
		})
		if d == nil || d.ReasonCode != "FS_ACCOUNT_INELIGIBLE" {
			t.Fatalf("expected FS_ACCOUNT_INELIGIBLE for suspended account")
		}
	})

	t.Run("AccountEligibilityRule_IneligibleType", func(t *testing.T) {
		r := funding.AccountEligibilityRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{
			AccountType:   "IRA",
			AccountStatus: "ACTIVE",
			RulesConfig:   betaRules(),
		})
		if d == nil || d.ReasonCode != "FS_ACCOUNT_INELIGIBLE" {
			t.Fatalf("expected FS_ACCOUNT_INELIGIBLE for IRA at Beta")
		}
	})

	t.Run("DepositLimitRule_AtLimit", func(t *testing.T) {
		r := funding.DepositLimitRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{Amount: 5000, RulesConfig: alphaRules()})
		if d != nil {
			t.Fatalf("expected pass at exact limit, got %s", d.ReasonCode)
		}
	})

	t.Run("DepositLimitRule_OverLimit", func(t *testing.T) {
		r := funding.DepositLimitRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{Amount: 5001, RulesConfig: alphaRules()})
		if d == nil || d.ReasonCode != "FS_OVER_DEPOSIT_LIMIT" {
			t.Fatalf("expected FS_OVER_DEPOSIT_LIMIT")
		}
	})

	t.Run("ContributionTypeRule_IRA_Default", func(t *testing.T) {
		r := funding.ContributionTypeRule{}
		ct := r.DefaultContributionType(&funding.EvaluateRequest{
			AccountType:     "IRA",
			ContributionType: "",
		})
		if ct != "INDIVIDUAL" {
			t.Fatalf("want INDIVIDUAL, got %q", ct)
		}
	})

	t.Run("ContributionTypeRule_NonIRA", func(t *testing.T) {
		r := funding.ContributionTypeRule{}
		ct := r.DefaultContributionType(&funding.EvaluateRequest{
			AccountType:      "INDIVIDUAL",
			ContributionType: "",
		})
		if ct != "" {
			t.Fatalf("want empty for non-IRA, got %q", ct)
		}
	})

	t.Run("VSSMICRRule_NoVSSResult_Skips", func(t *testing.T) {
		r := funding.VSSMICRRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{VSSResult: nil})
		if d != nil {
			t.Fatalf("expected nil (skip) when VSSResult is nil, got %+v", d)
		}
	})

	t.Run("VSSMICRRule_MICRUnreadable_Flags", func(t *testing.T) {
		r := funding.VSSMICRRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{
			VSSResult: &funding.VSSResult{MICRReadable: false},
		})
		if d == nil || d.ReasonCode != "VSS_MICR_READ_FAIL" {
			t.Fatalf("expected VSS_MICR_READ_FAIL, got %v", d)
		}
		if d.Decision != funding.DecisionFlagForReview {
			t.Fatalf("expected FLAG_FOR_REVIEW, got %s", d.Decision)
		}
	})

	t.Run("VSSMICRRule_MICRReadable_Passes", func(t *testing.T) {
		r := funding.VSSMICRRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{
			VSSResult: &funding.VSSResult{MICRReadable: true},
		})
		if d != nil {
			t.Fatalf("expected nil (pass) when MICR readable, got %+v", d)
		}
	})

	t.Run("VSSAmountMismatchRule_NoVSSResult_Skips", func(t *testing.T) {
		r := funding.VSSAmountMismatchRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{Amount: 500, VSSResult: nil})
		if d != nil {
			t.Fatalf("expected nil (skip) when VSSResult is nil, got %+v", d)
		}
	})

	t.Run("VSSAmountMismatchRule_AmountMismatch_Flags", func(t *testing.T) {
		r := funding.VSSAmountMismatchRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{
			Amount:    500.00,
			VSSResult: &funding.VSSResult{MICRReadable: true, OCRAmount: 250.00},
		})
		if d == nil || d.ReasonCode != "VSS_AMOUNT_MISMATCH" {
			t.Fatalf("expected VSS_AMOUNT_MISMATCH, got %v", d)
		}
	})

	t.Run("VSSAmountMismatchRule_AmountMatches_Passes", func(t *testing.T) {
		r := funding.VSSAmountMismatchRule{}
		d := r.Evaluate(ctx, &funding.EvaluateRequest{
			Amount:    500.00,
			VSSResult: &funding.VSSResult{MICRReadable: true, OCRAmount: 500.00},
		})
		if d != nil {
			t.Fatalf("expected nil (pass) when amounts match, got %+v", d)
		}
	})
}
