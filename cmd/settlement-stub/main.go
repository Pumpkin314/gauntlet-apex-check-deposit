package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

func main() {
	port := os.Getenv("SETTLEMENT_PORT")
	if port == "" {
		port = "8082"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /settlement/submit", handleSubmit)

	// POST /settlement/return — trigger a return notification back to the main API.
	mux.HandleFunc("POST /settlement/return", handleReturn)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Settlement Bank Stub starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

type submitRequest struct {
	BatchID string          `json:"batch_id"`
	File    json.RawMessage `json:"file"`
}

type submitResponse struct {
	BatchID               string `json:"batch_id"`
	Status                string `json:"status"`
	AcknowledgedAt        string `json:"acknowledged_at"`
	ReturnWindowExpiresAt string `json:"return_window_expires_at"`
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Use the incoming batch_id if present, otherwise generate one
	batchID := req.BatchID
	if batchID == "" {
		batchID = uuid.New().String()
	}

	now := time.Now().UTC()
	returnWindowExpires := now.Add(48 * time.Hour)

	resp := submitResponse{
		BatchID:               batchID,
		Status:                "ACKNOWLEDGED",
		AcknowledgedAt:        now.Format(time.RFC3339),
		ReturnWindowExpiresAt: returnWindowExpires.Format(time.RFC3339),
	}

	log.Printf("Settlement batch ACKNOWLEDGED: %s", batchID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type returnRequest struct {
	TransferID       string `json:"transfer_id"`
	ReturnReasonCode string `json:"return_reason_code"`
}

// handleReturn accepts a return trigger from the admin UI or test scripts and
// forwards it to the main API as a signed settlement bank webhook callback.
func handleReturn(w http.ResponseWriter, r *http.Request) {
	var req returnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.TransferID == "" || req.ReturnReasonCode == "" {
		http.Error(w, `{"error":"transfer_id and return_reason_code are required"}`, http.StatusBadRequest)
		return
	}

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	token := os.Getenv("SETTLEMENT_BANK_TOKEN")
	if token == "" {
		http.Error(w, `{"error":"SETTLEMENT_BANK_TOKEN not configured"}`, http.StatusInternalServerError)
		return
	}

	// Forward to the main API's POST /returns endpoint with bearer token.
	payload, _ := json.Marshal(map[string]string{
		"transfer_id":        req.TransferID,
		"return_reason_code": req.ReturnReasonCode,
	})

	apiReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, apiURL+"/returns", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"build request: %s"}`, err), http.StatusInternalServerError)
		return
	}
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(apiReq)
	if err != nil {
		log.Printf("settlement-stub: POST /returns callback failed: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"callback failed: %s"}`, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("settlement-stub: return processed transfer=%s reason=%s status=%d",
		req.TransferID, req.ReturnReasonCode, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody) //nolint:errcheck
}
