package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/cmd/api/middleware"
	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/orchestrator"
	"github.com/apex-checkout/check-deposit/internal/store"
)

// OperatorHandler handles operator queue and action endpoints.
type OperatorHandler struct {
	Transfers        *store.TransferStore
	OrchestratorDeps orchestrator.Deps
	Log              *slog.Logger
}

// GetQueue handles GET /operator/queue.
// Returns flagged transfers (state=Analyzing, review_reason IS NOT NULL).
// Filters: min_amount, max_amount, account_id, sort_by.
// RLS: operator tokens only see their correspondent's transfers.
func (h *OperatorHandler) GetQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	f := store.QueueFilter{
		CorrespondentID: middleware.CorrespondentIDFromContext(ctx),
	}

	if v := r.URL.Query().Get("min_amount"); v != "" {
		if amt, err := strconv.ParseFloat(v, 64); err == nil {
			f.MinAmount = amt
		}
	}
	if v := r.URL.Query().Get("max_amount"); v != "" {
		if amt, err := strconv.ParseFloat(v, 64); err == nil {
			f.MaxAmount = amt
		}
	}
	if v := r.URL.Query().Get("account_id"); v != "" {
		f.AccountID = v
	}
	if v := r.URL.Query().Get("sort_by"); v != "" {
		f.SortBy = v
	}

	transfers, err := h.Transfers.ListQueue(ctx, f)
	if err != nil {
		h.Log.ErrorContext(ctx, "list queue failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	result := make([]map[string]interface{}, 0, len(transfers))
	for _, t := range transfers {
		result = append(result, transferToJSON(t))
	}
	writeJSON(w, http.StatusOK, result)
}

// actionRequest is the JSON body for POST /operator/actions.
type actionRequest struct {
	TransferID             string `json:"transfer_id"`
	Action                 string `json:"action"` // "APPROVE" or "REJECT"
	Reason                 string `json:"reason,omitempty"`
	ContributionTypeOverride string `json:"contribution_type_override,omitempty"`
}

// PostAction handles POST /operator/actions.
// Validates transfer is in a reviewable state, then executes the action.
func (h *OperatorHandler) PostAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	operatorID := middleware.OperatorIDFromContext(ctx)
	correspondentID := middleware.CorrespondentIDFromContext(ctx)

	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.TransferID == "" || (req.Action != "APPROVE" && req.Action != "REJECT" && req.Action != "REVALIDATE") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "transfer_id and action (APPROVE, REJECT, or REVALIDATE) are required"})
		return
	}

	// Fetch transfer
	transfer, err := h.Transfers.GetByID(ctx, req.TransferID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transfer not found"})
		return
	}

	// RLS: operator can only act on their correspondent's transfers
	if correspondentID != "" && transfer.CorrespondentID != correspondentID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transfer not found"})
		return
	}

	// Validate transfer is in a reviewable state
	if !isReviewable(transfer) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("transfer in state %s is not reviewable", transfer.State),
		})
		return
	}

	// Write operator_action event
	eventData := map[string]interface{}{
		"operator_id": operatorID,
		"action":      req.Action,
	}
	if req.Reason != "" {
		eventData["reason"] = req.Reason
	}
	if req.ContributionTypeOverride != "" {
		eventData["contribution_type_override"] = req.ContributionTypeOverride
	}
	_ = h.Transfers.WriteEvent(ctx, req.TransferID, "operator_action", operatorID, eventData)

	// Apply contribution type override if provided
	if req.ContributionTypeOverride != "" {
		_ = h.Transfers.SetContributionType(ctx, req.TransferID, req.ContributionTypeOverride)
	}

	d := h.OrchestratorDeps
	log := h.Log.With("transfer_id", req.TransferID, "operator_id", operatorID)

	switch req.Action {
	case "APPROVE":
		// Analyzing → Approved
		if err := orchestrator.Transition(ctx, d.Updater, d.Events, d.Notifier, req.TransferID,
			orchestrator.Analyzing, orchestrator.Approved); err != nil {
			log.ErrorContext(ctx, "transition Analyzing→Approved failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transition failed"})
			return
		}

		// Approved → FundsPosted (immediate ledger post)
		omnibusID := uuid.MustParse(transfer.FromAccountID)
		accountID := uuid.MustParse(transfer.AccountID)
		amountStr := ledger.Amount(fmt.Sprintf("%.2f", transfer.Amount))
		transferUUID := uuid.MustParse(transfer.ID)

		if err := d.Ledger.PostDoubleEntry(ctx, omnibusID, accountID, amountStr, "FREE", transferUUID); err != nil {
			log.ErrorContext(ctx, "ledger posting failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ledger post failed"})
			return
		}

		_ = d.Events.WriteEvent(ctx, req.TransferID, "ledger_posted", "system", map[string]interface{}{
			"debit_account":  omnibusID.String(),
			"credit_account": accountID.String(),
			"amount":         transfer.Amount,
		})

		if err := orchestrator.Transition(ctx, d.Updater, d.Events, d.Notifier, req.TransferID,
			orchestrator.Approved, orchestrator.FundsPosted); err != nil {
			log.ErrorContext(ctx, "transition Approved→FundsPosted failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transition failed"})
			return
		}

		log.InfoContext(ctx, "operator approved deposit", "final_state", "FundsPosted")

	case "REJECT":
		fromState := orchestrator.Analyzing
		// Also allow rejecting from Validating (stuck VSS transfers)
		if transfer.State == "Validating" {
			fromState = orchestrator.Validating
		}

		if req.Reason != "" {
			_ = h.Transfers.SetErrorCode(ctx, req.TransferID, req.Reason)
		}

		if err := orchestrator.Transition(ctx, d.Updater, d.Events, d.Notifier, req.TransferID,
			fromState, orchestrator.Rejected); err != nil {
			log.ErrorContext(ctx, "transition to Rejected failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transition failed"})
			return
		}

		log.InfoContext(ctx, "operator rejected deposit")

	case "REVALIDATE":
		if err := orchestrator.Transition(ctx, d.Updater, d.Events, d.Notifier, req.TransferID,
			orchestrator.Analyzing, orchestrator.Validating); err != nil {
			log.ErrorContext(ctx, "transition Analyzing→Validating failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transition failed"})
			return
		}
		_ = h.Transfers.ClearReviewReason(ctx, req.TransferID)
		log.InfoContext(ctx, "operator triggered re-validation")
	}

	// Fetch updated transfer
	updated, err := h.Transfers.GetByID(ctx, req.TransferID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, transferToJSON(updated))
}

// isReviewable returns true if the transfer can be acted on by an operator.
// Transfers must be in Analyzing with a review_reason set, or in Validating (stuck).
func isReviewable(t *store.Transfer) bool {
	if t.State == "Analyzing" && t.ReviewReason != nil {
		return true
	}
	// Allow rejecting stuck Validating transfers
	if t.State == "Validating" {
		return true
	}
	return false
}
