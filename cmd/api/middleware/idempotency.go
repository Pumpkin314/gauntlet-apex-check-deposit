package middleware

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// IdempotencyStore checks and stores idempotency keys.
type IdempotencyStore struct {
	DB          *sql.DB
	RedisClient *redis.Client
	Log         *slog.Logger
}

type storedResponse struct {
	Code int             `json:"code"`
	Body json.RawMessage `json:"body"`
}

const redisIdempTTL = 24 * time.Hour

// Lookup checks if key exists. Returns (response, true) if found.
// When Redis is configured, checks Redis first (read-through cache).
func (s *IdempotencyStore) Lookup(ctx context.Context, key string) (*storedResponse, bool) {
	// Try Redis first (best-effort)
	if s.RedisClient != nil {
		val, err := s.RedisClient.Get(ctx, "idemp:"+key).Bytes()
		if err == nil {
			var resp storedResponse
			if json.Unmarshal(val, &resp) == nil {
				return &resp, true
			}
		}
	}

	// Fall through to Postgres
	var code int
	var body []byte
	err := s.DB.QueryRowContext(ctx,
		`SELECT response_code, response_body FROM idempotency_keys WHERE key = $1`, key,
	).Scan(&code, &body)
	if err != nil {
		return nil, false
	}

	resp := &storedResponse{Code: code, Body: body}

	// Backfill Redis on Postgres hit (best-effort)
	if s.RedisClient != nil {
		if data, err := json.Marshal(resp); err == nil {
			if err := s.RedisClient.Set(ctx, "idemp:"+key, data, redisIdempTTL).Err(); err != nil && s.Log != nil {
				s.Log.Warn("redis backfill failed", "key", key, "error", err)
			}
		}
	}

	return resp, true
}

// Store saves an idempotency key with its response.
func (s *IdempotencyStore) Store(ctx context.Context, key, transferID string, code int, body []byte) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO idempotency_keys (key, transfer_id, response_code, response_body) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (key) DO NOTHING`,
		key, transferID, code, body)
	if err != nil {
		return err
	}

	// Write to Redis (best-effort)
	if s.RedisClient != nil {
		resp := storedResponse{Code: code, Body: body}
		if data, marshalErr := json.Marshal(resp); marshalErr == nil {
			if redisErr := s.RedisClient.Set(ctx, "idemp:"+key, data, redisIdempTTL).Err(); redisErr != nil && s.Log != nil {
				s.Log.Warn("redis store failed", "key", key, "error", redisErr)
			}
		}
	}

	return nil
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
