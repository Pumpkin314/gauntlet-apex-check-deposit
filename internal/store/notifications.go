package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// NotificationStore provides write access to the notifications table.
type NotificationStore struct {
	db *sql.DB
}

// NewNotificationStore creates a NotificationStore backed by db.
func NewNotificationStore(db *sql.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

// CreateNotification inserts a new notification row.
func (s *NotificationStore) CreateNotification(ctx context.Context, accountID, transferID uuid.UUID, notifType, message string) error {
	const q = `INSERT INTO notifications (account_id, transfer_id, type, message) VALUES ($1, $2, $3, $4)`
	_, err := s.db.ExecContext(ctx, q, accountID, transferID, notifType, message)
	return err
}
