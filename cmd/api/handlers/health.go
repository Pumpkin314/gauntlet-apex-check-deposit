package handlers

import (
	"context"
	"net/http"
	"time"
)

// SettlementBatchInfo holds displayable info about a settlement batch.
type SettlementBatchInfo struct {
	ID          string    `json:"id"`
	GeneratedAt time.Time `json:"generated_at"`
	Count       int       `json:"count"`
	Status      string    `json:"status"`
}

// SettlementQuerier provides the DB queries needed for settlement health checks.
// Implemented in main.go using *sql.DB so this package stays DB-agnostic.
type SettlementQuerier interface {
	// CountUnbatched returns the number of FundsPosted transfers with no
	// settlement_batch_id that were submitted before the given cutoff.
	CountUnbatched(ctx context.Context, cutoff time.Time) (int, error)
	// CountAllUnbatched returns all unbatched FundsPosted transfers regardless of cutoff.
	CountAllUnbatched(ctx context.Context) (int, error)
	// LastBatch returns the most recent settlement batch, or nil if none exist.
	LastBatch(ctx context.Context) (*SettlementBatchInfo, error)
	// CreateBatch creates a settlement batch covering all unbatched FundsPosted
	// transfers and returns the resulting batch info.
	CreateBatch(ctx context.Context) (*SettlementBatchInfo, error)
}

// SettlementHealthHandler handles /health/settlement endpoints.
type SettlementHealthHandler struct {
	Querier SettlementQuerier
}

// Health handles GET /health/settlement.
// Returns { healthy, unbatched_count, ready_count, last_batch }.
func (h *SettlementHealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Transfers from before midnight today are "past cutoff" and make us unhealthy.
	cutoff := time.Now().UTC().Truncate(24 * time.Hour)
	pastCutoff, err := h.Querier.CountUnbatched(ctx, cutoff)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}

	// All unbatched FundsPosted transfers (including today) = "ready" for batching.
	ready, err := h.Querier.CountAllUnbatched(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}

	batch, _ := h.Querier.LastBatch(ctx)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"healthy":         pastCutoff == 0,
		"unbatched_count": pastCutoff,
		"ready_count":     ready,
		"last_batch":      batch,
	})
}

// Trigger handles POST /health/settlement/trigger.
// For demo use: creates a settlement batch covering all unbatched FundsPosted transfers.
func (h *SettlementHealthHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batch, err := h.Querier.CreateBatch(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create batch"})
		return
	}
	writeJSON(w, http.StatusOK, batch)
}
