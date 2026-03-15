package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/apex-checkout/check-deposit/internal/funding"
	"github.com/apex-checkout/check-deposit/internal/orchestrator"
	"github.com/apex-checkout/check-deposit/internal/store"
)

// DepositHandler handles POST /deposits and GET /deposits/:id.
type DepositHandler struct {
	Transfers        *store.TransferStore
	Accounts         *store.AccountStore
	Correspondents   *store.CorrespondentStore
	OrchestratorDeps orchestrator.Deps
	Log              *slog.Logger
}

// depositRequest is the JSON body for POST /deposits.
type depositRequest struct {
	AccountCode string  `json:"account_code"`
	Amount      float64 `json:"amount"`
	Scenario    string  `json:"scenario"` // optional VSS scenario override
}

// CreateDeposit handles POST /deposits.
func (h *DepositHandler) CreateDeposit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var frontImageRef, backImageRef string
	var req depositRequest

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/") {
		// Parse multipart form (max 10MB)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
			return
		}
		req.AccountCode = r.FormValue("account_code")
		req.Scenario = r.FormValue("scenario")
		amt := r.FormValue("amount")
		if _, err := fmt.Sscanf(amt, "%f", &req.Amount); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid amount"})
			return
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
	}

	if req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "amount must be greater than 0"})
		return
	}
	if req.AccountCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account_code is required"})
		return
	}

	// Look up account
	acct, err := h.Accounts.GetByCode(ctx, req.AccountCode)
	if err != nil {
		h.Log.ErrorContext(ctx, "account lookup failed", "account_code", req.AccountCode, "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return
	}

	if acct.CorrespondentID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot deposit to this account type"})
		return
	}

	// Load correspondent
	corr, err := h.Correspondents.GetByID(ctx, *acct.CorrespondentID)
	if err != nil {
		h.Log.ErrorContext(ctx, "correspondent lookup failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Resolve omnibus for initial from_account_id
	omnibusID, err := h.Accounts.GetOmnibusForCorrespondent(ctx, *acct.CorrespondentID)
	if err != nil {
		h.Log.ErrorContext(ctx, "omnibus resolution failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Create transfer
	transfer, err := h.Transfers.Create(ctx, store.CreateTransferInput{
		AccountID:       acct.ID.String(),
		FromAccountID:   omnibusID.String(),
		CorrespondentID: acct.CorrespondentID.String(),
		Amount:          req.Amount,
	})
	if err != nil {
		h.Log.ErrorContext(ctx, "create transfer failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create transfer"})
		return
	}

	// Write submitted event
	_ = h.Transfers.WriteEvent(ctx, transfer.ID, "submitted", "system", map[string]interface{}{
		"amount":       req.Amount,
		"account_code": req.AccountCode,
	})

	// Save images if multipart
	if strings.HasPrefix(contentType, "multipart/") {
		imgDir := filepath.Join(".", "data", "images", transfer.ID)
		os.MkdirAll(imgDir, 0755)

		for _, field := range []string{"front_image", "back_image"} {
			file, _, err := r.FormFile(field)
			if err != nil {
				continue
			}
			defer file.Close()
			ext := "jpg"
			name := "front"
			if field == "back_image" {
				name = "back"
			}
			path := filepath.Join(imgDir, name+"."+ext)
			dst, err := os.Create(path)
			if err != nil {
				continue
			}
			io.Copy(dst, file)
			dst.Close()
			if field == "front_image" {
				frontImageRef = path
			} else {
				backImageRef = path
			}
		}
		if frontImageRef != "" || backImageRef != "" {
			_ = h.Transfers.SetImageRefs(ctx, transfer.ID, frontImageRef, backImageRef)
		}
	}

	// Build rules config
	rulesConfig := funding.RulesConfig{
		DepositLimit:           corr.RulesConfig.DepositLimit,
		IneligibleAccountTypes: corr.RulesConfig.IneligibleAccountTypes,
		ContributionCap:        corr.RulesConfig.ContributionCap,
		Fees: funding.FeeConfig{
			ReturnedCheck: corr.RulesConfig.Fees.ReturnedCheck,
			Currency:      corr.RulesConfig.Fees.Currency,
		},
	}

	// Resolve scenario: body field > X-Scenario header > empty (VSS uses account lookup)
	scenario := req.Scenario
	if scenario == "" {
		scenario = r.Header.Get("X-Scenario")
	}

	// Run orchestrator flow
	td := orchestrator.TransferDetail{
		TransferID:       transfer.ID,
		AccountID:        acct.ID,
		CorrespondentID:  *acct.CorrespondentID,
		Amount:           req.Amount,
		AccountType:      acct.Type,
		AccountStatus:    acct.Status,
		ContributionType: "",
		RulesConfig:      rulesConfig,
		FrontImageRef:    frontImageRef,
		BackImageRef:     backImageRef,
		Scenario:         scenario,
	}

	finalState, err := orchestrator.ProcessDeposit(ctx, h.OrchestratorDeps, td)
	if err != nil {
		h.Log.ErrorContext(ctx, "orchestrator flow error",
			"transfer_id", transfer.ID,
			"correspondent_id", acct.CorrespondentID.String(),
			"error", err,
		)
	}

	// Fetch updated transfer
	updated, err := h.Transfers.GetByID(ctx, transfer.ID)
	if err != nil {
		h.Log.ErrorContext(ctx, "fetch updated transfer failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	_ = finalState // used for logging above
	writeJSON(w, http.StatusCreated, transferToJSON(updated))
}

// ListDeposits handles GET /deposits — returns transfers.
// If ?account_id= is provided, filters by that account.
func (h *DepositHandler) ListDeposits(w http.ResponseWriter, r *http.Request) {
	var transfers []*store.Transfer
	var err error

	if accountID := r.URL.Query().Get("account_id"); accountID != "" {
		transfers, err = h.Transfers.ListByAccountID(r.Context(), accountID)
	} else {
		transfers, err = h.Transfers.ListRecent(r.Context(), 50)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	result := make([]map[string]interface{}, 0, len(transfers))
	for _, t := range transfers {
		result = append(result, transferToJSON(t))
	}
	writeJSON(w, http.StatusOK, result)
}

// ListAccounts handles GET /accounts — returns investor accounts for login dropdown.
func (h *DepositHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.Accounts.ListInvestorAccounts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	result := make([]map[string]interface{}, 0, len(accounts))
	for _, a := range accounts {
		entry := map[string]interface{}{
			"id":   a.ID,
			"code": a.Code,
			"type": a.Type,
		}
		if a.CorrespondentID != nil {
			entry["correspondent_id"] = *a.CorrespondentID
		}
		result = append(result, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

// GetAccount handles GET /accounts/{code} — returns account details + balance.
func (h *DepositHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing account code"})
		return
	}

	acct, err := h.Accounts.GetByCode(r.Context(), code)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return
	}

	result := map[string]interface{}{
		"id":     acct.ID,
		"code":   acct.Code,
		"type":   acct.Type,
		"status": acct.Status,
	}
	if acct.CorrespondentID != nil {
		result["correspondent_id"] = *acct.CorrespondentID
	}

	// Include balance from ledger
	balance, err := h.OrchestratorDeps.Ledger.GetBalance(r.Context(), acct.ID)
	if err == nil {
		result["balance"] = balance
	}

	writeJSON(w, http.StatusOK, result)
}

// GetDeposit handles GET /deposits/{id}.
func (h *DepositHandler) GetDeposit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing transfer id"})
		return
	}

	transfer, err := h.Transfers.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transfer not found"})
		return
	}

	writeJSON(w, http.StatusOK, transferToJSON(transfer))
}

// GetDepositEvents handles GET /deposits/{id}/events.
func (h *DepositHandler) GetDepositEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing transfer id"})
		return
	}

	events, err := h.Transfers.GetEvents(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, events)
}

// GetDepositImage handles GET /deposits/{id}/images/{side} (front or back).
func (h *DepositHandler) GetDepositImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	side := r.PathValue("side")

	if side != "front" && side != "back" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "side must be 'front' or 'back'"})
		return
	}

	path := filepath.Join(".", "data", "images", id, side+".jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "image not found"})
		return
	}

	http.ServeFile(w, r, path)
}

func transferToJSON(t *store.Transfer) map[string]interface{} {
	result := map[string]interface{}{
		"id":               t.ID,
		"account_id":       t.AccountID,
		"from_account_id":  t.FromAccountID,
		"correspondent_id": t.CorrespondentID,
		"amount":           t.Amount,
		"currency":         t.Currency,
		"type":             t.Type,
		"sub_type":         t.SubType,
		"transfer_type":    t.TransferType,
		"memo":             t.Memo,
		"state":            t.State,
		"submitted_at":     t.SubmittedAt,
		"created_at":       t.CreatedAt,
		"updated_at":       t.UpdatedAt,
	}
	if t.ReviewReason != nil {
		result["review_reason"] = *t.ReviewReason
	}
	if t.ErrorCode != nil {
		result["error_code"] = *t.ErrorCode
		if userMsg := orchestrator.UserMessageForCode(*t.ErrorCode); userMsg != "" {
			result["user_message"] = userMsg
		}
	}
	if t.ContributionType != nil {
		result["contribution_type"] = *t.ContributionType
	}
	if t.VendorTransactionID != nil {
		result["vendor_transaction_id"] = *t.VendorTransactionID
	}
	if t.ConfidenceScore != nil {
		result["confidence_score"] = *t.ConfidenceScore
	}
	if t.MICRData != nil {
		result["micr_data"] = t.MICRData
	}
	if t.FrontImageRef != nil {
		result["front_image_ref"] = *t.FrontImageRef
	}
	if t.BackImageRef != nil {
		result["back_image_ref"] = *t.BackImageRef
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
