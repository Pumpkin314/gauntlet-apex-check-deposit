// Package returns handles inbound return notifications from the settlement bank.
// It is responsible for:
//   - Validating the transfer is in a returnable state (FundsPosted or Completed)
//   - Posting exactly 4 ledger entries: reversal pair + fee pair
//   - Transitioning the transfer to Returned
//   - Writing audit events and creating an investor notification
//   - Flagging the account for COLLECTIONS if the investor balance goes negative
package returns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/ledger"
)

// State constants — mirrored from orchestrator to avoid a cross-package import.
const (
	StateFundsPosted = "FundsPosted"
	StateCompleted   = "Completed"
	StateReturned    = "Returned"
)

// ErrInvalidReturnState is returned when the transfer is not in a returnable state.
type ErrInvalidReturnState struct {
	TransferID string
	State      string
}

func (e *ErrInvalidReturnState) Error() string {
	return fmt.Sprintf("transfer %s in state %s cannot be returned (expected FundsPosted or Completed)", e.TransferID, e.State)
}

// Transfer is the return handler's view of a transfer record.
type Transfer struct {
	ID              string
	AccountID       uuid.UUID // investor account
	OmnibusID       uuid.UUID // from_account_id on the transfer
	CorrespondentID uuid.UUID
	Amount          float64
	State           string
}

// Input is the return notification payload from the settlement bank.
type Input struct {
	TransferID string
	ReasonCode string
}

// TransferRepository is how the return handler reads and writes transfers.
type TransferRepository interface {
	GetTransfer(ctx context.Context, id string) (*Transfer, error)
	UpdateState(ctx context.Context, id, from, to string) error
	WriteEvent(ctx context.Context, id, step, actor string, data map[string]interface{}) error
	Notify(ctx context.Context, id string, payload map[string]interface{}) error
}

// CorrespondentRepository loads correspondent fee configuration.
type CorrespondentRepository interface {
	GetReturnedCheckFee(ctx context.Context, correspondentID uuid.UUID) (float64, error)
}

// AccountRepository manages investor account state.
type AccountRepository interface {
	GetFeeAccountID(ctx context.Context) (uuid.UUID, error)
	SetAccountStatus(ctx context.Context, accountID uuid.UUID, status string) error
}

// NotificationRepository creates investor notifications.
type NotificationRepository interface {
	CreateNotification(ctx context.Context, accountID, transferID uuid.UUID, notifType, message string) error
}

// Deps bundles all return handler dependencies.
type Deps struct {
	Transfers      TransferRepository
	Correspondents CorrespondentRepository
	Accounts       AccountRepository
	Notifications  NotificationRepository
	Ledger         ledger.LedgerService
	Log            *slog.Logger
}

// ProcessReturn handles a return notification end-to-end:
//  1. Validates state is FundsPosted or Completed (idempotent no-op if already Returned)
//  2. Writes return_received event
//  3. Loads fee from correspondent rules_config
//  4. Posts reversal: DEBIT investor, CREDIT omnibus (original amount)
//  5. Posts fee: DEBIT investor, CREDIT FEE-APEX (fee from config)
//  6. Transitions transfer to Returned
//  7. Writes events + investor notification
//  8. Flags account COLLECTIONS if balance goes negative
func ProcessReturn(ctx context.Context, d Deps, in Input) error {
	log := d.Log.With("transfer_id", in.TransferID)

	// 1. Load transfer
	t, err := d.Transfers.GetTransfer(ctx, in.TransferID)
	if err != nil {
		return fmt.Errorf("returns: get transfer: %w", err)
	}

	log = log.With("correspondent_id", t.CorrespondentID)

	// 2. Idempotency: already Returned → no-op
	if t.State == StateReturned {
		log.InfoContext(ctx, "return already processed — no-op")
		return nil
	}

	// 3. Validate state
	if t.State != StateFundsPosted && t.State != StateCompleted {
		return &ErrInvalidReturnState{TransferID: in.TransferID, State: t.State}
	}

	fromState := t.State

	// 4. Write return_received event (best-effort)
	_ = d.Transfers.WriteEvent(ctx, in.TransferID, "return_received", "settlement_bank", map[string]interface{}{
		"reason_code": in.ReasonCode,
	})

	// 5. Load fee from correspondent rules_config (never hardcode)
	fee, err := d.Correspondents.GetReturnedCheckFee(ctx, t.CorrespondentID)
	if err != nil {
		return fmt.Errorf("returns: get correspondent fee: %w", err)
	}

	// 6. Resolve FEE-APEX account
	feeAccountID, err := d.Accounts.GetFeeAccountID(ctx)
	if err != nil {
		return fmt.Errorf("returns: get fee account: %w", err)
	}

	transferUUID := uuid.MustParse(in.TransferID)
	originalAmount := ledger.Amount(fmt.Sprintf("%.2f", t.Amount))
	feeAmount := ledger.Amount(fmt.Sprintf("%.2f", fee))

	// 7. Post reversal pair: DEBIT investor, CREDIT omnibus
	// (reverses the original deposit: DEBIT omnibus, CREDIT investor)
	if err := d.Ledger.PostDoubleEntry(ctx,
		t.AccountID, t.OmnibusID,
		originalAmount, "REVERSAL", transferUUID,
	); err != nil {
		return fmt.Errorf("returns: post reversal ledger entry: %w", err)
	}

	// 8. Post fee pair: DEBIT investor, CREDIT FEE-APEX
	if err := d.Ledger.PostDoubleEntry(ctx,
		t.AccountID, feeAccountID,
		feeAmount, "RETURNED_CHECK_FEE", transferUUID,
	); err != nil {
		return fmt.Errorf("returns: post fee ledger entry: %w", err)
	}

	// 9. Transition to Returned (optimistic locking via UpdateState)
	if err := d.Transfers.UpdateState(ctx, in.TransferID, fromState, StateReturned); err != nil {
		return fmt.Errorf("returns: transition %s→Returned: %w", fromState, err)
	}

	// 10. Write state_changed event
	_ = d.Transfers.WriteEvent(ctx, in.TransferID, "state_changed", "system", map[string]interface{}{
		"from_state": fromState,
		"to_state":   StateReturned,
		"trigger":    "settlement_bank_return",
	})

	// 11. Write return_processed event
	_ = d.Transfers.WriteEvent(ctx, in.TransferID, "return_processed", "system", map[string]interface{}{
		"reason_code":     in.ReasonCode,
		"fee_amount":      fee,
		"reversal_amount": t.Amount,
	})

	// 12. pg_notify for SSE dashboard (best-effort)
	_ = d.Transfers.Notify(ctx, in.TransferID, map[string]interface{}{
		"transfer_id": in.TransferID,
		"from_state":  fromState,
		"to_state":    StateReturned,
	})

	// 13. Create investor notification
	reasonMsg := reasonCodeToMessage(in.ReasonCode)
	message := fmt.Sprintf(
		"Your deposit of $%.2f was returned. Reason: %s. A returned check fee of $%.2f has been charged.",
		t.Amount, reasonMsg, fee,
	)
	if err := d.Notifications.CreateNotification(ctx, t.AccountID, transferUUID, "RETURN_RECEIVED", message); err != nil {
		log.WarnContext(ctx, "failed to create return notification", "error", err)
	}

	// 14. Check investor balance — flag COLLECTIONS if negative
	// Return processing always completes even if balance goes negative (CLAUDE.md invariant #4).
	balStr, err := d.Ledger.GetBalance(ctx, t.AccountID)
	if err != nil {
		log.WarnContext(ctx, "could not check investor balance after return", "error", err)
	} else {
		bal, parseErr := strconv.ParseFloat(string(balStr), 64)
		if parseErr == nil && bal < 0 {
			log.WarnContext(ctx, "investor balance negative after return — flagging COLLECTIONS",
				"account_id", t.AccountID,
				"balance", balStr,
			)
			if setErr := d.Accounts.SetAccountStatus(ctx, t.AccountID, "COLLECTIONS"); setErr != nil {
				log.ErrorContext(ctx, "failed to set account COLLECTIONS", "error", setErr)
			}
		}
	}

	log.InfoContext(ctx, "return processed successfully",
		"from_state", fromState,
		"reason_code", in.ReasonCode,
		"reversal_amount", t.Amount,
		"fee_amount", fee,
	)

	return nil
}

// reasonCodeToMessage converts ACH return reason codes to plain English.
func reasonCodeToMessage(code string) string {
	switch code {
	case "R01":
		return "Insufficient funds"
	case "R02":
		return "Account closed"
	case "R08":
		return "Payment stopped"
	case "NCI":
		return "Non-cash item"
	default:
		if code == "" {
			return "Unspecified"
		}
		return fmt.Sprintf("Return reason code %s", code)
	}
}

// IsNotFound reports whether err represents a not-found condition from the store.
// Used by callers to map errors to appropriate HTTP status codes.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errNotFound)
}

// errNotFound is the sentinel checked by IsNotFound.
// The store adapter wraps store.ErrNotFound into this.
var errNotFound = errors.New("not found")

// ErrNotFound wraps a not-found error so IsNotFound returns true.
func ErrNotFound(msg string) error {
	return fmt.Errorf("%w: %s", errNotFound, msg)
}
