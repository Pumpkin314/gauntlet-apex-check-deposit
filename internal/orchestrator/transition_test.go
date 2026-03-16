package orchestrator_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/apex-checkout/check-deposit/internal/orchestrator"
)

// ---- mock: TransferUpdater ----

type mockUpdater struct {
	mu        sync.Mutex
	callCount int
	failWith  error
}

func (m *mockUpdater) UpdateState(_ context.Context, _ string, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	return m.failWith
}

// onceUpdater allows exactly one successful UpdateState; subsequent calls return errLock.
type onceUpdater struct {
	mu   sync.Mutex
	used bool
}

var errLock = errors.New("optimistic lock conflict")

func (u *onceUpdater) UpdateState(_ context.Context, _ string, _, _ string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.used {
		return errLock
	}
	u.used = true
	return nil
}

// ---- mock: EventWriter ----

type capturedEvent struct {
	transferID string
	step       string
	actor      string
	data       map[string]interface{}
}

type mockEventWriter struct {
	mu     sync.Mutex
	events []capturedEvent
}

func (m *mockEventWriter) WriteEvent(_ context.Context, transferID, step, actor string, data map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, capturedEvent{transferID, step, actor, data})
	return nil
}

// ---- mock: Notifier ----

type mockNotifier struct {
	mu            sync.Mutex
	notifications []string
}

func (m *mockNotifier) Notify(_ context.Context, transferID string, _ map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, transferID)
	return nil
}

// ---- helpers ----

func doTransition(t *testing.T, updater orchestrator.TransferUpdater, events orchestrator.EventWriter, notifier orchestrator.Notifier, from, to orchestrator.TransferState) error {
	t.Helper()
	return orchestrator.Transition(context.Background(), updater, events, notifier, "transfer-test-1", from, to)
}

// ---- tests: valid transitions ----

func TestValidTransitions(t *testing.T) {
	type tc struct {
		from orchestrator.TransferState
		to   orchestrator.TransferState
	}
	// All 9 valid transitions from the spec
	cases := []tc{
		{orchestrator.Requested, orchestrator.Validating},
		{orchestrator.Validating, orchestrator.Analyzing},
		{orchestrator.Validating, orchestrator.Rejected},
		{orchestrator.Analyzing, orchestrator.Approved},
		{orchestrator.Analyzing, orchestrator.Rejected},
		{orchestrator.Analyzing, orchestrator.Validating},
		{orchestrator.Approved, orchestrator.FundsPosted},
		{orchestrator.FundsPosted, orchestrator.Completed},
		{orchestrator.FundsPosted, orchestrator.Returned},
		{orchestrator.Completed, orchestrator.Returned},
	}
	for _, c := range cases {
		t.Run(string(c.from)+"→"+string(c.to), func(t *testing.T) {
			updater := &mockUpdater{}
			events := &mockEventWriter{}
			err := doTransition(t, updater, events, nil, c.from, c.to)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if updater.callCount != 1 {
				t.Fatalf("expected UpdateState called once, got %d", updater.callCount)
			}
		})
	}
}

// ---- tests: invalid transitions ----

func TestInvalidTransition_RequestedToApproved(t *testing.T) {
	updater := &mockUpdater{}
	events := &mockEventWriter{}
	err := doTransition(t, updater, events, nil, orchestrator.Requested, orchestrator.Approved)
	assertInvalidTransitionError(t, err, orchestrator.Requested, orchestrator.Approved)
	if updater.callCount != 0 {
		t.Error("UpdateState should not be called for invalid transition")
	}
}

func TestInvalidTransition_CompletedToApproved(t *testing.T) {
	err := doTransition(t, &mockUpdater{}, &mockEventWriter{}, nil, orchestrator.Completed, orchestrator.Approved)
	assertInvalidTransitionError(t, err, orchestrator.Completed, orchestrator.Approved)
}

func TestInvalidTransition_RejectedIsTerminal(t *testing.T) {
	targets := []orchestrator.TransferState{
		orchestrator.Requested, orchestrator.Validating, orchestrator.Analyzing,
		orchestrator.Approved, orchestrator.FundsPosted, orchestrator.Completed,
		orchestrator.Returned,
	}
	for _, to := range targets {
		t.Run("Rejected→"+string(to), func(t *testing.T) {
			err := doTransition(t, &mockUpdater{}, &mockEventWriter{}, nil, orchestrator.Rejected, to)
			assertInvalidTransitionError(t, err, orchestrator.Rejected, to)
		})
	}
}

func TestInvalidTransition_ReturnedIsTerminal(t *testing.T) {
	targets := []orchestrator.TransferState{
		orchestrator.Requested, orchestrator.Validating, orchestrator.Analyzing,
		orchestrator.Approved, orchestrator.FundsPosted, orchestrator.Completed,
		orchestrator.Rejected,
	}
	for _, to := range targets {
		t.Run("Returned→"+string(to), func(t *testing.T) {
			err := doTransition(t, &mockUpdater{}, &mockEventWriter{}, nil, orchestrator.Returned, to)
			assertInvalidTransitionError(t, err, orchestrator.Returned, to)
		})
	}
}

func assertInvalidTransitionError(t *testing.T, err error, from, to orchestrator.TransferState) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected ErrInvalidTransition for %s→%s, got nil", from, to)
	}
	var e *orchestrator.ErrInvalidTransition
	if !errors.As(err, &e) {
		t.Fatalf("expected *ErrInvalidTransition, got %T: %v", err, err)
	}
	if e.Code() != "SYS_INVALID_TRANSITION" {
		t.Errorf("expected code SYS_INVALID_TRANSITION, got %s", e.Code())
	}
}

// ---- test: optimistic lock ----

func TestOptimisticLock(t *testing.T) {
	updater := &onceUpdater{}
	events := &mockEventWriter{}

	var wg sync.WaitGroup
	results := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = orchestrator.Transition(
				context.Background(), updater, events, nil,
				"transfer-concurrent", orchestrator.Requested, orchestrator.Validating,
			)
		}(i)
	}
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d successes and %d failures", successes, failures)
	}
}

// ---- test: event logging ----

func TestEventLogging_StateChangedWritten(t *testing.T) {
	updater := &mockUpdater{}
	events := &mockEventWriter{}
	notifier := &mockNotifier{}

	err := orchestrator.Transition(
		context.Background(), updater, events, notifier,
		"transfer-evt-1", orchestrator.Requested, orchestrator.Validating,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events.mu.Lock()
	evts := events.events
	events.mu.Unlock()

	if len(evts) != 1 {
		t.Fatalf("expected 1 event written, got %d", len(evts))
	}
	e := evts[0]
	if e.step != "state_changed" {
		t.Errorf("expected step=state_changed, got %q", e.step)
	}
	if e.actor != "system" {
		t.Errorf("expected actor=system, got %q", e.actor)
	}
	if e.data["from_state"] != "Requested" {
		t.Errorf("expected from_state=Requested, got %v", e.data["from_state"])
	}
	if e.data["to_state"] != "Validating" {
		t.Errorf("expected to_state=Validating, got %v", e.data["to_state"])
	}
}

func TestEventLogging_EveryTransitionWritesEvent(t *testing.T) {
	// Walk through the happy path and verify each transition writes exactly one event.
	happyPath := []struct {
		from orchestrator.TransferState
		to   orchestrator.TransferState
	}{
		{orchestrator.Requested, orchestrator.Validating},
		{orchestrator.Validating, orchestrator.Analyzing},
		{orchestrator.Analyzing, orchestrator.Approved},
		{orchestrator.Approved, orchestrator.FundsPosted},
		{orchestrator.FundsPosted, orchestrator.Completed},
	}

	events := &mockEventWriter{}
	updater := &mockUpdater{}

	for _, step := range happyPath {
		before := len(events.events)
		err := orchestrator.Transition(
			context.Background(), updater, events, nil,
			"transfer-happy", step.from, step.to,
		)
		if err != nil {
			t.Fatalf("unexpected error at %s→%s: %v", step.from, step.to, err)
		}
		after := len(events.events)
		if after-before != 1 {
			t.Errorf("%s→%s: expected 1 new event, got %d", step.from, step.to, after-before)
		}
	}
}

func TestNotifierCalledOnValidTransition(t *testing.T) {
	updater := &mockUpdater{}
	events := &mockEventWriter{}
	notifier := &mockNotifier{}

	err := orchestrator.Transition(
		context.Background(), updater, events, notifier,
		"transfer-notify-1", orchestrator.Requested, orchestrator.Validating,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notifier.mu.Lock()
	notifications := notifier.notifications
	notifier.mu.Unlock()

	if len(notifications) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifications))
	}
}

func TestNotifierNotCalledOnInvalidTransition(t *testing.T) {
	updater := &mockUpdater{}
	events := &mockEventWriter{}
	notifier := &mockNotifier{}

	_ = orchestrator.Transition(
		context.Background(), updater, events, notifier,
		"transfer-notify-2", orchestrator.Requested, orchestrator.Approved, // invalid
	)

	notifier.mu.Lock()
	notifications := notifier.notifications
	notifier.mu.Unlock()

	if len(notifications) != 0 {
		t.Errorf("expected 0 notifications for invalid transition, got %d", len(notifications))
	}
}
