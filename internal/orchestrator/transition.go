package orchestrator

import (
	"context"
	"fmt"
)

// TransferUpdater updates transfer state with optimistic locking.
// internal/store.TransferStore implements this interface.
//
// UpdateState must execute:
//
//	UPDATE transfers SET state=$to, updated_at=NOW()
//	WHERE id=$transferID AND state=$from
//	RETURNING id
//
// If no row is returned (transfer not in state `from`), it must return ErrOptimisticLock.
type TransferUpdater interface {
	UpdateState(ctx context.Context, transferID string, from, to string) error
}

// Transition performs a validated, optimistic-lock state transition:
//  1. Validates from→to against validTransitions (returns ErrInvalidTransition if not allowed).
//  2. Calls updater.UpdateState — the optimistic lock (returns error on conflict).
//  3. Writes a state_changed event to transfer_events.
//  4. Sends pg_notify for the SSE dashboard (best-effort; failure is logged, not fatal).
func Transition(
	ctx context.Context,
	updater TransferUpdater,
	events EventWriter,
	notifier Notifier,
	transferID string,
	from, to TransferState,
) error {
	if !isValidTransition(from, to) {
		return &ErrInvalidTransition{From: from, To: to}
	}

	if err := updater.UpdateState(ctx, transferID, string(from), string(to)); err != nil {
		return fmt.Errorf("UpdateState %s→%s: %w", from, to, err)
	}

	if err := writeStateChangedEvent(ctx, events, transferID, from, to); err != nil {
		return fmt.Errorf("WriteEvent state_changed %s→%s: %w", from, to, err)
	}

	if notifier != nil {
		// Best-effort: pg_notify failure must not roll back a committed state transition.
		_ = notifier.Notify(ctx, transferID, map[string]interface{}{
			"transfer_id": transferID,
			"from_state":  string(from),
			"to_state":    string(to),
		})
	}

	return nil
}
