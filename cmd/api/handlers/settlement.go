package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/settlement"
	"github.com/apex-checkout/check-deposit/internal/store"
)

// SettlementHandler handles settlement-related API endpoints.
type SettlementHandler struct {
	Engine        *settlement.Engine
	Batches       *store.SettlementStore
	SettlementURL string // Settlement Bank stub URL
	Log           *slog.Logger
}

// Trigger handles POST /settlement/trigger.
// Generates settlement batches for FundsPosted transfers before the cutoff.
func (h *SettlementHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := h.Engine.Trigger(ctx, time.Now())
	if err != nil {
		h.Log.ErrorContext(ctx, "settlement trigger failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "settlement trigger failed"})
		return
	}

	if result.BatchCount == 0 {
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Submit each batch to the Settlement Bank stub
	for i, bs := range result.Batches {
		batchID := uuid.MustParse(bs.BatchID)

		// Mark batch as SUBMITTED
		_ = h.Batches.UpdateBatchStatus(ctx, batchID, "SUBMITTED", nil)

		// Read the settlement file
		fileData, err := os.ReadFile(bs.FileRef)
		if err != nil {
			h.Log.ErrorContext(ctx, "read settlement file failed", "file", bs.FileRef, "error", err)
			continue
		}

		// Submit to Settlement Bank stub
		ack, err := h.submitToSettlementBank(bs.BatchID, fileData)
		if err != nil {
			h.Log.ErrorContext(ctx, "submit to settlement bank failed", "batch_id", bs.BatchID, "error", err)
			continue
		}

		// Process acknowledgment
		ackTime := time.Now()
		if ack.AcknowledgedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, ack.AcknowledgedAt); err == nil {
				ackTime = parsed
			}
		}

		if err := h.Engine.AcknowledgeBatch(ctx, batchID, ackTime); err != nil {
			h.Log.ErrorContext(ctx, "acknowledge batch failed", "batch_id", bs.BatchID, "error", err)
			continue
		}

		result.Batches[i].BatchID = ack.BatchID
	}

	writeJSON(w, http.StatusOK, result)
}

// Status handles GET /settlement/status.
// Returns current settlement status including unbatched transfer count.
func (h *SettlementHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	unbatched, err := h.Batches.UnbatchedFundsPostedCount(ctx)
	if err != nil {
		h.Log.ErrorContext(ctx, "unbatched count failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	batches, err := h.Batches.ListBatches(ctx)
	if err != nil {
		h.Log.ErrorContext(ctx, "list batches failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	var latestBatch map[string]interface{}
	if len(batches) > 0 {
		b := batches[0]
		latestBatch = batchToJSON(b)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"unbatched_count": unbatched,
		"latest_batch":    latestBatch,
		"total_batches":   len(batches),
	})
}

// Batches handles GET /settlement/batches.
// Returns list of all settlement batches.
func (h *SettlementHandler) ListBatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	batches, err := h.Batches.ListBatches(ctx)
	if err != nil {
		h.Log.ErrorContext(ctx, "list batches failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	result := make([]map[string]interface{}, 0, len(batches))
	for _, b := range batches {
		result = append(result, batchToJSON(b))
	}

	writeJSON(w, http.StatusOK, result)
}

// settlementAck is the acknowledgment from the Settlement Bank stub.
type settlementAck struct {
	BatchID               string `json:"batch_id"`
	Status                string `json:"status"`
	AcknowledgedAt        string `json:"acknowledged_at"`
	ReturnWindowExpiresAt string `json:"return_window_expires_at"`
}

func (h *SettlementHandler) submitToSettlementBank(batchID string, fileData []byte) (*settlementAck, error) {
	url := h.SettlementURL + "/settlement/submit"

	payload := map[string]interface{}{
		"batch_id": batchID,
		"file":     json.RawMessage(fileData),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	token := os.Getenv("SETTLEMENT_BANK_TOKEN")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit to settlement bank: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("settlement bank returned %d", resp.StatusCode)
	}

	var ack settlementAck
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return nil, fmt.Errorf("decode acknowledgment: %w", err)
	}

	return &ack, nil
}

func batchToJSON(b *settlement.Batch) map[string]interface{} {
	result := map[string]interface{}{
		"id":               b.ID.String(),
		"correspondent_id": b.CorrespondentID,
		"cutoff_date":      b.CutoffDate.Format("2006-01-02"),
		"status":           b.Status,
		"file_ref":         b.FileRef,
		"record_count":     b.RecordCount,
		"total_amount":     b.TotalAmount,
		"created_at":       b.CreatedAt,
		"transfer_ids":     b.TransferIDs,
	}
	if b.SubmittedAt != nil {
		result["submitted_at"] = *b.SubmittedAt
	}
	if b.AcknowledgedAt != nil {
		result["acknowledged_at"] = *b.AcknowledgedAt
	}
	return result
}
