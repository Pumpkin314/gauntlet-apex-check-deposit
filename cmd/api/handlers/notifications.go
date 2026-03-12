package handlers

import (
	"net/http"
	"time"

	"github.com/apex-checkout/check-deposit/cmd/api/middleware"
	"github.com/apex-checkout/check-deposit/internal/store"
)

// NotificationHandler handles GET /notifications and PATCH /notifications/{id}/read.
type NotificationHandler struct {
	Notifications *store.NotificationStore
}

type notifJSON struct {
	ID         string  `json:"id"`
	AccountID  string  `json:"account_id"`
	TransferID *string `json:"transfer_id,omitempty"`
	Type       string  `json:"type"`
	Message    string  `json:"message"`
	ReadAt     *string `json:"read_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// GetNotifications handles GET /notifications.
// Returns notifications for the authenticated investor's account.
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	accountID := middleware.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "investor auth required"})
		return
	}

	ns, err := h.Notifications.ListByAccountID(r.Context(), accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	result := make([]notifJSON, 0, len(ns))
	for _, n := range ns {
		nj := notifJSON{
			ID:        n.ID,
			AccountID: n.AccountID,
			TransferID: n.TransferID,
			Type:      n.Type,
			Message:   n.Message,
			CreatedAt: n.CreatedAt.Format(time.RFC3339),
		}
		if n.ReadAt != nil {
			s := n.ReadAt.Format(time.RFC3339)
			nj.ReadAt = &s
		}
		result = append(result, nj)
	}

	writeJSON(w, http.StatusOK, result)
}

// MarkRead handles PATCH /notifications/{id}/read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing notification id"})
		return
	}

	if err := h.Notifications.MarkRead(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "notification not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
