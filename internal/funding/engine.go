package funding

import (
	"context"
	"fmt"
	"time"
)

// Engine implements FundingServiceClient. It is stateless — no DB writes occur here.
type Engine struct {
	rules       []Rule
	ctRule      ContributionTypeRule
	omnibusRule OmnibusResolutionRule
	dupChecker  DuplicateChecker // nil = FS duplicate detection disabled
	dupWindow   time.Duration
}

// DefaultDupWindow is the default duplicate detection window.
const DefaultDupWindow = 5 * time.Minute

// NewEngine constructs a funding Engine. resolver provides omnibus account lookup;
// inject a mock in tests or the real store.AccountStore in production.
func NewEngine(resolver OmnibusResolver) *Engine {
	return NewEngineWithDupWindow(resolver, DefaultDupWindow)
}

// NewEngineWithDupWindow constructs a funding Engine with a configurable duplicate
// detection window. Use NewEngine for the default (5 minutes).
func NewEngineWithDupWindow(resolver OmnibusResolver, dupWindow time.Duration) *Engine {
	ctRule := ContributionTypeRule{}
	omnRule := OmnibusResolutionRule{Resolver: resolver}
	return &Engine{
		rules: []Rule{
			VSSMICRRule{},
			VSSAmountMismatchRule{},
			AccountEligibilityRule{},
			DepositLimitRule{},
			ctRule,
		},
		ctRule:      ctRule,
		omnibusRule: omnRule,
		dupWindow:   dupWindow,
	}
}

// WithDuplicateChecker enables FS-level duplicate detection using checker with the
// given time window. Returns e for method chaining.
func (e *Engine) WithDuplicateChecker(checker DuplicateChecker, window time.Duration) *Engine {
	e.dupChecker = checker
	e.dupWindow = window
	return e
}

// Evaluate runs all rules in order and returns a FundingDecision.
// Duplicate detection runs first; then VSS rules; then eligibility and limit rules.
// The first rule that rejects or flags short-circuits evaluation.
func (e *Engine) Evaluate(ctx context.Context, req *EvaluateRequest) (*FundingDecision, error) {
	// FS-level duplicate detection — runs before all other rules.
	if e.dupChecker != nil {
		dup, err := e.dupChecker.HasRecentTransfer(ctx, req.AccountID, req.Amount, e.dupWindow)
		if err != nil {
			return nil, fmt.Errorf("funding: duplicate check for transfer %s: %w", req.TransferID, err)
		}
		if dup {
			return &FundingDecision{Decision: DecisionReject, ReasonCode: "FS_DUPLICATE_DEPOSIT"}, nil
		}
	}

	for _, rule := range e.rules {
		if d := rule.Evaluate(ctx, req); d != nil {
			return d, nil
		}
	}

	omnibusID, err := e.omnibusRule.Resolver.GetOmnibusForCorrespondent(ctx, req.CorrespondentID)
	if err != nil {
		return nil, fmt.Errorf("funding: omnibus resolution for correspondent %s: %w", req.CorrespondentID, err)
	}

	return &FundingDecision{
		Decision:          DecisionApprove,
		ResolvedOmnibusID: omnibusID,
		ContributionType:  e.ctRule.DefaultContributionType(req),
	}, nil
}
