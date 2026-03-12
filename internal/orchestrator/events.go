package orchestrator

import "context"

// EventWriter persists a row to transfer_events.
// internal/store.TransferStore implements this interface.
type EventWriter interface {
	WriteEvent(ctx context.Context, transferID, step, actor string, data map[string]interface{}) error
}

// Notifier sends a pg_notify for the SSE dashboard.
// internal/store.TransferStore implements this interface.
type Notifier interface {
	Notify(ctx context.Context, transferID string, payload map[string]interface{}) error
}

// writeStateChangedEvent writes the mandatory state_changed event on every transition.
func writeStateChangedEvent(ctx context.Context, w EventWriter, transferID string, from, to TransferState) error {
	return w.WriteEvent(ctx, transferID, "state_changed", "system", map[string]interface{}{
		"from_state": string(from),
		"to_state":   string(to),
		"trigger":    "orchestrator",
	})
}
