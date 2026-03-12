package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/returns"
	"github.com/apex-checkout/check-deposit/internal/store"
)

// ReturnHandler handles POST /returns (settlement bank webhook).
type ReturnHandler struct {
	Transfers   *store.TransferStore
	ReturnDeps  returns.Deps
	Log         *slog.Logger
}

// returnRequest is the JSON body for POST /returns.
type returnRequest struct {
	TransferID       string `json:"transfer_id"`
	ReturnReasonCode string `json:"return_reason_code"`
}

// ProcessReturn handles POST /returns.
//
// Bearer token is validated upstream by middleware.SettlementAuth.
// Returns 200 on success (including idempotent no-op).
// Returns 400 for invalid/unresolvable state, 404 for unknown transfer, 500 for internal errors.
func (h *ReturnHandler) ProcessReturn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req returnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.TransferID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "transfer_id is required"})
		return
	}
	if _, err := uuid.Parse(req.TransferID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "transfer_id must be a valid UUID"})
		return
	}

	// Pre-check: verify transfer exists to produce a proper 404.
	t, err := h.Transfers.GetByID(ctx, req.TransferID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transfer not found"})
		return
	}
	if err != nil {
		h.Log.ErrorContext(ctx, "get transfer for return",
			"transfer_id", req.TransferID,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Run return processing.
	err = returns.ProcessReturn(ctx, h.ReturnDeps, returns.Input{
		TransferID: req.TransferID,
		ReasonCode: req.ReturnReasonCode,
	})

	var stateErr *returns.ErrInvalidReturnState
	switch {
	case err == nil:
		// Success or idempotent no-op — fall through to fetch final state.
	case errors.As(err, &stateErr):
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("transfer in state %s cannot be returned (expected FundsPosted or Completed)", t.State),
		})
		return
	default:
		h.Log.ErrorContext(ctx, "return processing failed",
			"transfer_id", req.TransferID,
			"correspondent_id", t.CorrespondentID,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Fetch updated transfer for the response body.
	updated, err := h.Transfers.GetByID(ctx, req.TransferID)
	if err != nil {
		h.Log.ErrorContext(ctx, "fetch updated transfer after return",
			"transfer_id", req.TransferID,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, transferToJSON(updated))
}

// ---- adapter types: implement returns.* interfaces using concrete store types ----

// TransferReturnAdapter adapts *store.TransferStore to returns.TransferRepository.
type TransferReturnAdapter struct {
	S *store.TransferStore
}

func (a *TransferReturnAdapter) GetTransfer(ctx context.Context, id string) (*returns.Transfer, error) {
	t, err := a.S.GetByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, returns.ErrNotFound("transfer " + id)
	}
	if err != nil {
		return nil, err
	}
	accountID, _ := uuid.Parse(t.AccountID)
	omnibusID, _ := uuid.Parse(t.FromAccountID)
	corrID, _ := uuid.Parse(t.CorrespondentID)
	return &returns.Transfer{
		ID:              t.ID,
		AccountID:       accountID,
		OmnibusID:       omnibusID,
		CorrespondentID: corrID,
		Amount:          t.Amount,
		State:           t.State,
	}, nil
}

func (a *TransferReturnAdapter) UpdateState(ctx context.Context, id, from, to string) error {
	return a.S.UpdateState(ctx, id, from, to)
}

func (a *TransferReturnAdapter) WriteEvent(ctx context.Context, id, step, actor string, data map[string]interface{}) error {
	return a.S.WriteEvent(ctx, id, step, actor, data)
}

func (a *TransferReturnAdapter) Notify(ctx context.Context, id string, payload map[string]interface{}) error {
	return a.S.Notify(ctx, id, payload)
}

// CorrespondentReturnAdapter adapts *store.CorrespondentStore to returns.CorrespondentRepository.
type CorrespondentReturnAdapter struct {
	S *store.CorrespondentStore
}

func (a *CorrespondentReturnAdapter) GetReturnedCheckFee(ctx context.Context, corrID uuid.UUID) (float64, error) {
	c, err := a.S.GetByID(ctx, corrID)
	if err != nil {
		return 0, err
	}
	return c.RulesConfig.Fees.ReturnedCheck, nil
}

// AccountReturnAdapter adapts *store.AccountStore to returns.AccountRepository.
type AccountReturnAdapter struct {
	S *store.AccountStore
}

func (a *AccountReturnAdapter) GetFeeAccountID(ctx context.Context) (uuid.UUID, error) {
	acct, err := a.S.GetByCode(ctx, "FEE-APEX")
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("resolve FEE-APEX account: %w", err)
	}
	return acct.ID, nil
}

func (a *AccountReturnAdapter) SetAccountStatus(ctx context.Context, accountID uuid.UUID, status string) error {
	return a.S.SetStatus(ctx, accountID, status)
}

// NotificationReturnAdapter adapts *store.NotificationStore to returns.NotificationRepository.
type NotificationReturnAdapter struct {
	S *store.NotificationStore
}

func (a *NotificationReturnAdapter) CreateNotification(ctx context.Context, accountID, transferID uuid.UUID, notifType, message string) error {
	return a.S.CreateNotification(ctx, accountID, transferID, notifType, message)
}

// NewReturnHandler wires all store types into a ready-to-use ReturnHandler.
func NewReturnHandler(
	transferStore *store.TransferStore,
	accountStore *store.AccountStore,
	correspondentStore *store.CorrespondentStore,
	notificationStore *store.NotificationStore,
	ledgerSvc ledger.LedgerService,
	log *slog.Logger,
) *ReturnHandler {
	deps := returns.Deps{
		Transfers:      &TransferReturnAdapter{S: transferStore},
		Correspondents: &CorrespondentReturnAdapter{S: correspondentStore},
		Accounts:       &AccountReturnAdapter{S: accountStore},
		Notifications:  &NotificationReturnAdapter{S: notificationStore},
		Ledger:         ledgerSvc,
		Log:            log,
	}
	return &ReturnHandler{
		Transfers:  transferStore,
		ReturnDeps: deps,
		Log:        log,
	}
}
