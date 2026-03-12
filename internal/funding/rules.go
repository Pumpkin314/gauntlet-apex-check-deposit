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
