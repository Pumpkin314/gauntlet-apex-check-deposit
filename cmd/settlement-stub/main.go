package main

import (
	"encoding/json"
	"fmt"
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

func handleReturn(w http.ResponseWriter, r *http.Request) {
	var req returnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.TransferID == "" || req.ReturnReasonCode == "" {
		http.Error(w, "transfer_id and return_reason_code are required", http.StatusBadRequest)
		return
	}

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	token := os.Getenv("SETTLEMENT_BANK_TOKEN")

	// Call back to the main API's /returns endpoint
	payload := fmt.Sprintf(`{"transfer_id":"%s","reason_code":"%s"}`, req.TransferID, req.ReturnReasonCode)

	httpReq, _ := http.NewRequest("POST", apiURL+"/returns", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	httpReq.Body = http.NoBody

	// For MVP, just acknowledge the return request
	log.Printf("Return requested: transfer=%s reason=%s (webhook callback to %s/returns)",
		req.TransferID, req.ReturnReasonCode, apiURL)
	_ = payload // Will be used when /returns endpoint is implemented in TB5

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "RETURN_INITIATED",
		"transfer_id": req.TransferID,
		"reason_code": req.ReturnReasonCode,
	})
}
