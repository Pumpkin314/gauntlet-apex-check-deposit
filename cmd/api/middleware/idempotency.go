package middleware

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
)

// IdempotencyStore checks and stores idempotency keys.
type IdempotencyStore struct {
	DB *sql.DB
}

type storedResponse struct {
	Code int
	Body json.RawMessage
}

// Lookup checks if key exists. Returns (response, true) if found.
func (s *IdempotencyStore) Lookup(ctx context.Context, key string) (*storedResponse, bool) {
	var code int
	var body []byte
	err := s.DB.QueryRowContext(ctx,
		`SELECT response_code, response_body FROM idempotency_keys WHERE key = $1`, key,
	).Scan(&code, &body)
	if err != nil {
		return nil, false
	}
	return &storedResponse{Code: code, Body: body}, true
}

// Store saves an idempotency key with its response.
func (s *IdempotencyStore) Store(ctx context.Context, key, transferID string, code int, body []byte) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO idempotency_keys (key, transfer_id, response_code, response_body) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (key) DO NOTHING`,
		key, transferID, code, body)
	return err
}

// Idempotency is middleware that checks the Idempotency-Key header.
func Idempotency(store *IdempotencyStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			// No key provided — proceed without idempotency
			next(w, r)
			return
		}

		// Check for existing response
		if resp, found := store.Lookup(r.Context(), key); found {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Idempotent-Replay", "true")
			w.WriteHeader(resp.Code)
			w.Write(resp.Body)
			return
		}

		// Wrap the response writer to capture the response
		rw := &responseCapture{ResponseWriter: w, buf: &bytes.Buffer{}}
		next(rw, r)

		// Store the response for future replays
		// Extract transfer_id from the response body
		var respData map[string]interface{}
		if err := json.Unmarshal(rw.buf.Bytes(), &respData); err == nil {
			if tid, ok := respData["id"].(string); ok {
				_ = store.Store(r.Context(), key, tid, rw.statusCode, rw.buf.Bytes())
			}
		}
	}
}

type responseCapture struct {
	http.ResponseWriter
	buf        *bytes.Buffer
	statusCode int
}

func (rw *responseCapture) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseCapture) Write(b []byte) (int, error) {
	rw.buf.Write(b)
	return rw.ResponseWriter.Write(b)
}
