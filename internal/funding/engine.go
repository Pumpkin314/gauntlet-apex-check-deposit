package funding

import (
	"context"
	"fmt"
)

// Engine implements FundingServiceClient. It is stateless — no DB writes occur here.
type Engine struct {
	rules        []Rule
	ctRule       ContributionTypeRule
	omnibusRule  OmnibusResolutionRule
}

// NewEngine constructs a funding Engine. resolver provides omnibus account lookup;
// inject a mock in tests or the real store.AccountStore in production.
func NewEngine(resolver OmnibusResolver) *Engine {
	ctRule := ContributionTypeRule{}
	omnRule := OmnibusResolutionRule{Resolver: resolver}
	return &Engine{
		rules: []Rule{
			AccountEligibilityRule{},
			DepositLimitRule{},
			ctRule,
		},
		ctRule:      ctRule,
		omnibusRule: omnRule,
	}
}

// Evaluate runs all rules in order and returns a FundingDecision.
// The first rule that rejects short-circuits evaluation.
func (e *Engine) Evaluate(ctx context.Context, req *EvaluateRequest) (*FundingDecision, error) {
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
