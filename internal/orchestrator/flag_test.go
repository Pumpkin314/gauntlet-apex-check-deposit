package orchestrator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/apex-checkout/check-deposit/internal/orchestrator"
)

// ---- mock: ReviewFlagSetter ----

type mockFlagSetter struct {
	calledWith string
	err        error
}

func (m *mockFlagSetter) SetReviewReason(_ context.Context, transferID, _ string) error {
	m.calledWith = transferID
	return m.err
}

// TestFlagForReview_SetsReviewReason verifies that SetReviewReason is called with
// the correct transferID and that a "flagged" event is written.
func TestFlagForReview_SetsReviewReason(t *testing.T) {
	setter := &mockFlagSetter{}
	events := &mockEventWriter{}
	notifier := &mockNotifier{}

	err := orchestrator.FlagForReview(
		context.Background(), setter, events, notifier,
		"transfer-flag-1", "VSS_MICR_READ_FAIL",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setter.calledWith != "transfer-flag-1" {
		t.Errorf("SetReviewReason called with %q, want %q", setter.calledWith, "transfer-flag-1")
	}
}

// TestFlagForReview_WritesFlaggedEvent verifies that a "flagged" event is written
// with the review_reason in its data payload.
func TestFlagForReview_WritesFlaggedEvent(t *testing.T) {
	setter := &mockFlagSetter{}
	events := &mockEventWriter{}

	err := orchestrator.FlagForReview(
		context.Background(), setter, events, nil,
		"transfer-flag-2", "VSS_AMOUNT_MISMATCH",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events.mu.Lock()
	evts := events.events
	events.mu.Unlock()

	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	e := evts[0]
	if e.step != "flagged" {
		t.Errorf("expected step=flagged, got %q", e.step)
	}
	if e.actor != "system" {
		t.Errorf("expected actor=system, got %q", e.actor)
	}
	if e.data["review_reason"] != "VSS_AMOUNT_MISMATCH" {
		t.Errorf("expected review_reason=VSS_AMOUNT_MISMATCH in event data, got %v", e.data["review_reason"])
	}
}

// TestFlagForReview_SetterError propagates SetReviewReason errors.
func TestFlagForReview_SetterError(t *testing.T) {
	setter := &mockFlagSetter{err: errors.New("db error")}
	events := &mockEventWriter{}

	err := orchestrator.FlagForReview(
		context.Background(), setter, events, nil,
		"transfer-flag-3", "VSS_MICR_READ_FAIL",
	)
	if err == nil {
		t.Fatal("expected error from SetReviewReason, got nil")
	}

	events.mu.Lock()
	n := len(events.events)
	events.mu.Unlock()

	if n != 0 {
		t.Errorf("expected no events written on setter error, got %d", n)
	}
}

// TestFlagForReview_NotifierNil verifies FlagForReview works without a notifier.
func TestFlagForReview_NotifierNil(t *testing.T) {
	setter := &mockFlagSetter{}
	events := &mockEventWriter{}

	err := orchestrator.FlagForReview(
		context.Background(), setter, events, nil, // nil notifier
		"transfer-flag-4", "VSS_MICR_READ_FAIL",
	)
	if err != nil {
		t.Fatalf("unexpected error with nil notifier: %v", err)
	}
}
