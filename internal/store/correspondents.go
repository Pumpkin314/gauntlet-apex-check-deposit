package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Correspondent is the domain model for the correspondents table.
type Correspondent struct {
	ID          uuid.UUID
	Code        string
	Name        string
	RulesConfig CorrespondentRulesConfig
}

// CorrespondentRulesConfig mirrors the rules_config JSONB column.
type CorrespondentRulesConfig struct {
	DepositLimit           float64  `json:"deposit_limit"`
	IneligibleAccountTypes []string `json:"ineligible_account_types"`
	ContributionCap        float64  `json:"contribution_cap"`
	Fees                   struct {
		ReturnedCheck float64 `json:"returned_check"`
		Currency      string  `json:"currency"`
	} `json:"fees"`
}

// CorrespondentStore provides read access to the correspondents table.
type CorrespondentStore struct {
	db *sql.DB
}

// NewCorrespondentStore creates a CorrespondentStore backed by db.
func NewCorrespondentStore(db *sql.DB) *CorrespondentStore {
	return &CorrespondentStore{db: db}
}

// GetByID fetches a correspondent by UUID, loading rules_config from JSONB.
func (s *CorrespondentStore) GetByID(ctx context.Context, id uuid.UUID) (*Correspondent, error) {
	var c Correspondent
	var rulesRaw []byte

	err := s.db.QueryRowContext(ctx,
		`SELECT id, code, name, rules_config FROM correspondents WHERE id = $1`, id,
	).Scan(&c.ID, &c.Code, &c.Name, &rulesRaw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("store: correspondent %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: GetByID correspondent: %w", err)
	}

	if err := json.Unmarshal(rulesRaw, &c.RulesConfig); err != nil {
		return nil, fmt.Errorf("store: unmarshal rules_config for correspondent %s: %w", id, err)
	}
	return &c, nil
}
