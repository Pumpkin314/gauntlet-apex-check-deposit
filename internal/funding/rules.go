package funding

import "context"

// Rule is the interface every funding rule implements.
// Evaluate returns nil on pass, or a non-nil FundingDecision on reject/flag.
type Rule interface {
	Name() string
	Evaluate(ctx context.Context, req *EvaluateRequest) *FundingDecision
}

// AccountEligibilityRule rejects inactive accounts and account types the
// correspondent has marked ineligible (e.g. Beta disallows IRA deposits).
type AccountEligibilityRule struct{}

func (AccountEligibilityRule) Name() string { return "AccountEligibilityRule" }

func (AccountEligibilityRule) Evaluate(_ context.Context, req *EvaluateRequest) *FundingDecision {
	if req.AccountStatus != "ACTIVE" {
		return &FundingDecision{Decision: DecisionReject, ReasonCode: "FS_ACCOUNT_INELIGIBLE"}
	}
	for _, t := range req.RulesConfig.IneligibleAccountTypes {
		if t == req.AccountType {
			return &FundingDecision{Decision: DecisionReject, ReasonCode: "FS_ACCOUNT_INELIGIBLE"}
		}
	}
	return nil
}

// DepositLimitRule rejects deposits that exceed the correspondent's limit.
// Boundary: amount == deposit_limit is APPROVED (≤, not <).
type DepositLimitRule struct{}

func (DepositLimitRule) Name() string { return "DepositLimitRule" }

func (DepositLimitRule) Evaluate(_ context.Context, req *EvaluateRequest) *FundingDecision {
	if req.Amount > req.RulesConfig.DepositLimit {
		return &FundingDecision{Decision: DecisionReject, ReasonCode: "FS_OVER_DEPOSIT_LIMIT"}
	}
	return nil
}

// ContributionTypeRule defaults contribution_type to "INDIVIDUAL" for IRA
// accounts when the caller has not specified one. Always passes (no rejection).
type ContributionTypeRule struct{}

func (ContributionTypeRule) Name() string { return "ContributionTypeRule" }

func (ContributionTypeRule) Evaluate(_ context.Context, _ *EvaluateRequest) *FundingDecision {
	return nil // enrichment only — see DefaultContributionType
}

// DefaultContributionType returns the contribution type to stamp on the
// FundingDecision. IRA accounts default to "INDIVIDUAL" when unspecified.
func (ContributionTypeRule) DefaultContributionType(req *EvaluateRequest) string {
	if req.AccountType == "IRA" && req.ContributionType == "" {
		return "INDIVIDUAL"
	}
	return req.ContributionType
}

// OmnibusResolutionRule resolves the correspondent's omnibus account.
// Always passes; omnibus ID is carried out via Resolve().
type OmnibusResolutionRule struct {
	Resolver OmnibusResolver
}

func (OmnibusResolutionRule) Name() string { return "OmnibusResolutionRule" }

func (OmnibusResolutionRule) Evaluate(_ context.Context, _ *EvaluateRequest) *FundingDecision {
	return nil
}

// VSSMICRRule flags deposits where the Vendor Service could not read the MICR line
// (micr_data=null in the VSS response). No-op when VSSResult is nil.
type VSSMICRRule struct{}

func (VSSMICRRule) Name() string { return "VSSMICRRule" }

func (VSSMICRRule) Evaluate(_ context.Context, req *EvaluateRequest) *FundingDecision {
	if req.VSSResult == nil {
		return nil
	}
	if !req.VSSResult.MICRReadable {
		return &FundingDecision{Decision: DecisionFlagForReview, ReasonCode: "VSS_MICR_READ_FAIL"}
	}
	return nil
}

// VSSAmountMismatchRule flags deposits where the OCR-read amount differs from the
// submitted amount. No-op when VSSResult is nil or MICR was unreadable (VSSMICRRule
// fires first in that case).
type VSSAmountMismatchRule struct{}

func (VSSAmountMismatchRule) Name() string { return "VSSAmountMismatchRule" }

func (VSSAmountMismatchRule) Evaluate(_ context.Context, req *EvaluateRequest) *FundingDecision {
	if req.VSSResult == nil || !req.VSSResult.MICRReadable {
		return nil
	}
	if req.VSSResult.OCRAmount != req.Amount {
		return &FundingDecision{Decision: DecisionFlagForReview, ReasonCode: "VSS_AMOUNT_MISMATCH"}
	}
	return nil
}
