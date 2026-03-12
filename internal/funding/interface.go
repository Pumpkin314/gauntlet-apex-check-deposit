package funding

import (
	"context"

	"github.com/google/uuid"
)

// Decision is the outcome of a funding evaluation.
type Decision string

const (
	DecisionApprove       Decision = "APPROVE"
	DecisionReject        Decision = "REJECT"
	DecisionFlagForReview Decision = "FLAG_FOR_REVIEW"
)

// FeeConfig holds per-correspondent fee settings.
type FeeConfig struct {
	ReturnedCheck float64 `json:"returned_check"`
	Currency      string  `json:"currency"`
}

// RulesConfig is the correspondent's business-rule configuration (from rules_config JSONB).
type RulesConfig struct {
	DepositLimit           float64  `json:"deposit_limit"`
	IneligibleAccountTypes []string `json:"ineligible_account_types"`
	ContributionCap        float64  `json:"contribution_cap"`
	Fees                   FeeConfig `json:"fees"`
}

// EvaluateRequest is the input to the Funding Service. All data is pre-fetched
// by the caller (orchestrator); the Funding Service is stateless and does not
// query the DB directly (except omnibus resolution via OmnibusResolver).
type EvaluateRequest struct {
	TransferID      uuid.UUID
	AccountID       uuid.UUID
	CorrespondentID uuid.UUID
	Amount          float64 // in dollars, e.g. 500.00
	AccountType     string  // 'INDIVIDUAL', 'IRA', 'OMNIBUS', 'FEE'
	AccountStatus   string  // 'ACTIVE', 'SUSPENDED', 'COLLECTIONS'
	ContributionType string // may be empty; defaulted by ContributionTypeRule
	RulesConfig     RulesConfig
}

// FundingDecision is the output of a funding evaluation.
type FundingDecision struct {
	Decision          Decision
	ReasonCode        string    // set on REJECT or FLAG_FOR_REVIEW
	ResolvedOmnibusID uuid.UUID // set on APPROVE
	ContributionType  string    // set for IRA accounts on APPROVE
}

// FundingServiceClient is the interface the orchestrator uses to evaluate a deposit.
type FundingServiceClient interface {
	Evaluate(ctx context.Context, req *EvaluateRequest) (*FundingDecision, error)
}

// OmnibusResolver is the store interface the engine uses for omnibus account resolution.
// Defined here so funding never imports the store package.
type OmnibusResolver interface {
	GetOmnibusForCorrespondent(ctx context.Context, correspondentID uuid.UUID) (uuid.UUID, error)
}
