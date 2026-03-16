package middleware

import (
	"context"
	"net/http"
	"strings"

	fbauth "firebase.google.com/go/v4/auth"
)

type contextKey string

const (
	CtxRole            contextKey = "role"
	CtxCorrespondentID contextKey = "correspondent_id"
	CtxOperatorID      contextKey = "operator_id"
	CtxAccountID       contextKey = "account_id"
)

// Auth mode constants.
const (
	AuthModeDemo = "demo"
	AuthModeGCP  = "gcp"
)

// DemoToken represents a hardcoded token for the demo environment.
type DemoToken struct {
	OperatorID      string
	Role            string // "operator", "admin", or "investor"
	CorrespondentID string // empty for admin (can see all)
	AccountID       string // investor account UUID (seed data)
}

// demoTokens maps Bearer token values to their associated identity.
var demoTokens = map[string]DemoToken{
	"operator-alpha": {
		OperatorID:      "op-alpha-001",
		Role:            "operator",
		CorrespondentID: "c0000000-0000-0000-0000-000000000001",
	},
	"operator-beta": {
		OperatorID:      "op-beta-001",
		Role:            "operator",
		CorrespondentID: "c0000000-0000-0000-0000-000000000002",
	},
	"apex-admin": {
		OperatorID: "admin-001",
		Role:       "admin",
		// No CorrespondentID — can see all transfers.
	},
	// Investor tokens — keyed to seed account IDs from db/seed.sql.
	"investor-alpha": {
		Role:            "investor",
		CorrespondentID: "c0000000-0000-0000-0000-000000000001",
		AccountID:       "a0000000-0000-0000-0000-000000000001",
	},
	"investor-beta": {
		Role:            "investor",
		CorrespondentID: "c0000000-0000-0000-0000-000000000002",
		AccountID:       "a0000000-0000-0000-0000-000000000007",
	},
}

// Auth extracts a Bearer token from the Authorization header, validates it
// against the hardcoded demo tokens, and injects role + correspondent_id
// into the request context.
func Auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, `{"error":"invalid Authorization format, expected Bearer <token>"}`, http.StatusUnauthorized)
			return
		}

		dt, ok := demoTokens[token]
		if !ok {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, CtxRole, dt.Role)
		ctx = context.WithValue(ctx, CtxOperatorID, dt.OperatorID)
		if dt.CorrespondentID != "" {
			ctx = context.WithValue(ctx, CtxCorrespondentID, dt.CorrespondentID)
		}
		if dt.AccountID != "" {
			ctx = context.WithValue(ctx, CtxAccountID, dt.AccountID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// FirebaseAuth returns middleware that validates Firebase JWT tokens and
// extracts custom claims (role, correspondent_id, operator_id, account_id).
func FirebaseAuth(client *fbauth.Client) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			idToken := strings.TrimPrefix(authHeader, "Bearer ")
			if idToken == authHeader {
				http.Error(w, `{"error":"invalid Authorization format, expected Bearer <token>"}`, http.StatusUnauthorized)
				return
			}

			token, err := client.VerifyIDToken(r.Context(), idToken)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()

			// Extract custom claims
			if role, ok := token.Claims["role"].(string); ok {
				ctx = context.WithValue(ctx, CtxRole, role)
			}
			if corrID, ok := token.Claims["correspondent_id"].(string); ok {
				ctx = context.WithValue(ctx, CtxCorrespondentID, corrID)
			}
			if opID, ok := token.Claims["operator_id"].(string); ok {
				ctx = context.WithValue(ctx, CtxOperatorID, opID)
			}
			if acctID, ok := token.Claims["account_id"].(string); ok {
				ctx = context.WithValue(ctx, CtxAccountID, acctID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// NewAuthMiddleware returns the appropriate auth middleware based on mode.
// "gcp" mode uses Firebase JWT validation; "demo" (default) uses hardcoded tokens.
func NewAuthMiddleware(mode string, firebaseClient *fbauth.Client) func(http.HandlerFunc) http.HandlerFunc {
	if mode == AuthModeGCP && firebaseClient != nil {
		return FirebaseAuth(firebaseClient)
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return Auth(next)
	}
}

// RoleFromContext returns the role from the request context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxRole).(string)
	return v
}

// CorrespondentIDFromContext returns the correspondent_id from the request context.
// Returns empty string for admin users (who can see all correspondents).
func CorrespondentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxCorrespondentID).(string)
	return v
}

// OperatorIDFromContext returns the operator_id from the request context.
func OperatorIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxOperatorID).(string)
	return v
}

// AccountIDFromContext returns the investor account_id from the request context.
// Non-empty only for investor tokens.
func AccountIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxAccountID).(string)
	return v
}
