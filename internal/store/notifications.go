package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Notification is a row from the notifications table.
type Notification struct {
	ID         string
	AccountID  string
	TransferID *string
	Type       string
	Message    string
	ReadAt     *time.Time
	CreatedAt  time.Time
}

// NotificationStore provides persistence for the notifications table.
// Only this package imports database/sql per CLAUDE.md architecture rules.
type NotificationStore struct {
	db *sql.DB
}

// NewNotificationStore creates a NotificationStore backed by db.
func NewNotificationStore(db *sql.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

// CreateNotification inserts a new notification row.
// Used by the Return Handler (PR 5.1) when processing returned checks.
func (s *NotificationStore) CreateNotification(ctx context.Context, accountID, transferID uuid.UUID, notifType, message string) error {
	const q = `INSERT INTO notifications (account_id, transfer_id, type, message) VALUES ($1, $2, $3, $4)`
	_, err := s.db.ExecContext(ctx, q, accountID, transferID, notifType, message)
	return err
}

// ListByAccountID returns all notifications for the account, newest first.
func (s *NotificationStore) ListByAccountID(ctx context.Context, accountID string) ([]*Notification, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, account_id, transfer_id, type, message, read_at, created_at
		 FROM notifications
		 WHERE account_id = $1
		 ORDER BY created_at DESC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ns []*Notification
	for rows.Next() {
		n := &Notification{}
		var transferID sql.NullString
		var readAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.AccountID, &transferID, &n.Type, &n.Message, &readAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		if transferID.Valid {
			n.TransferID = &transferID.String
		}
		if readAt.Valid {
			n.ReadAt = &readAt.Time
		}
		ns = append(ns, n)
	}
	return ns, rows.Err()
}

// MarkRead sets read_at = NOW() for the notification. Returns an error if not found.
func (s *NotificationStore) MarkRead(ctx context.Context, notifID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1`, notifID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("notification not found")
	}
	return nil
}
