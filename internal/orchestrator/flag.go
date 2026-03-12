package orchestrator

import (
	"context"
	"fmt"
)

// ReviewFlagSetter sets the review_reason on a transfer without changing its state.
// internal/store.TransferStore implements this interface.
type ReviewFlagSetter interface {
	SetReviewReason(ctx context.Context, transferID, reason string) error
}

// FlagForReview marks a transfer as requiring manual review. The transfer stays in
// Analyzing state — no state transition occurs. It:
//  1. Sets transfer.review_reason via setter.
//  2. Writes a "flagged" event to transfer_events.
//  3. Sends pg_notify (best-effort; failure does not roll back the flag).
func FlagForReview(
	ctx context.Context,
	setter ReviewFlagSetter,
	events EventWriter,
	notifier Notifier,
	transferID, reason string,
) error {
	if err := setter.SetReviewReason(ctx, transferID, reason); err != nil {
		return fmt.Errorf("FlagForReview SetReviewReason: %w", err)
	}

	if err := events.WriteEvent(ctx, transferID, "flagged", "system", map[string]interface{}{
		"review_reason": reason,
	}); err != nil {
		return fmt.Errorf("FlagForReview WriteEvent: %w", err)
	}

	if notifier != nil {
		// Best-effort: pg_notify failure must not undo a committed review flag.
		_ = notifier.Notify(ctx, transferID, map[string]interface{}{
			"transfer_id":   transferID,
			"review_reason": reason,
		})
	}

	return nil
}
