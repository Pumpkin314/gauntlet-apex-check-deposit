package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// Account is the domain model for the accounts table.
type Account struct {
	ID              uuid.UUID
	Code            string
	CorrespondentID *uuid.UUID // nil for the FEE-APEX account
	Type            string     // 'INDIVIDUAL', 'IRA', 'OMNIBUS', 'FEE'
	Status          string     // 'ACTIVE', 'SUSPENDED', 'COLLECTIONS'
}

// AccountStore provides read access to the accounts table.
// It satisfies the funding.OmnibusResolver interface via GetOmnibusForCorrespondent.
type AccountStore struct {
	db *sql.DB
}

// NewAccountStore creates an AccountStore backed by db.
func NewAccountStore(db *sql.DB) *AccountStore {
	return &AccountStore{db: db}
}

// GetByID fetches a single account by its UUID primary key.
func (s *AccountStore) GetByID(ctx context.Context, id uuid.UUID) (*Account, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, code, correspondent_id, type, status FROM accounts WHERE id = $1`, id)
	return scanAccount(row)
}

// GetByCode fetches a single account by its unique code (e.g. 'ALPHA-001').
func (s *AccountStore) GetByCode(ctx context.Context, code string) (*Account, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, code, correspondent_id, type, status FROM accounts WHERE code = $1`, code)
	return scanAccount(row)
}

// GetOmnibusForCorrespondent returns the UUID of the OMNIBUS account belonging
// to correspondentID. Satisfies funding.OmnibusResolver.
func (s *AccountStore) GetOmnibusForCorrespondent(ctx context.Context, correspondentID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM accounts WHERE correspondent_id = $1 AND type = 'OMNIBUS' LIMIT 1`,
		correspondentID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return uuid.UUID{}, fmt.Errorf("store: no omnibus account for correspondent %s", correspondentID)
	}
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("store: GetOmnibusForCorrespondent: %w", err)
	}
	return id, nil
}

// scanAccount scans a single row into an Account. Handles nullable correspondent_id.
func scanAccount(row *sql.Row) (*Account, error) {
	a := &Account{}
	var corrID sql.NullString
	err := row.Scan(&a.ID, &a.Code, &corrID, &a.Type, &a.Status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("store: account not found")
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan account: %w", err)
	}
	if corrID.Valid {
		id, err := uuid.Parse(corrID.String)
		if err != nil {
			return nil, fmt.Errorf("store: parse correspondent_id: %w", err)
		}
		a.CorrespondentID = &id
	}
	return a, nil
}
