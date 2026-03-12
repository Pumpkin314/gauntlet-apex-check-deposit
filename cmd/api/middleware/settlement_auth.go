package middleware

import (
	"net/http"
	"os"
	"strings"
)

// SettlementAuth validates the bearer token on inbound settlement bank webhook
// calls (POST /returns). The expected token is read from the
// SETTLEMENT_BANK_TOKEN environment variable.
//
// Returns 401 Unauthorized if the token is missing or does not match.
func SettlementAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv("SETTLEMENT_BANK_TOKEN")
		if expected == "" {
			// No token configured: deny all requests rather than allow all.
			http.Error(w, `{"error":"settlement auth not configured"}`, http.StatusUnauthorized)
			return
		}

		token := extractBearerToken(r.Header.Get("Authorization"))
		if token != expected {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// extractBearerToken parses "Bearer <token>" and returns <token>.
// Returns empty string for any other format.
func extractBearerToken(header string) string {
	const prefix = "Bearer "
	if strings.HasPrefix(header, prefix) {
		return header[len(prefix):]
	}
	return ""
}
