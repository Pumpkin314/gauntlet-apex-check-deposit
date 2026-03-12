package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth_MissingHeader_Returns401(t *testing.T) {
	handler := Auth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest("GET", "/operator/queue", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_InvalidToken_Returns401(t *testing.T) {
	handler := Auth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest("GET", "/operator/queue", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_BadFormat_Returns401(t *testing.T) {
	handler := Auth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest("GET", "/operator/queue", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_OperatorAlpha_SetsContext(t *testing.T) {
	var gotRole, gotCorr, gotOp string

	handler := Auth(func(w http.ResponseWriter, r *http.Request) {
		gotRole = RoleFromContext(r.Context())
		gotCorr = CorrespondentIDFromContext(r.Context())
		gotOp = OperatorIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/operator/queue", nil)
	req.Header.Set("Authorization", "Bearer operator-alpha")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotRole != "operator" {
		t.Errorf("expected role=operator, got %s", gotRole)
	}
	if gotCorr != "c0000000-0000-0000-0000-000000000001" {
		t.Errorf("expected correspondent_id for Alpha, got %s", gotCorr)
	}
	if gotOp != "op-alpha-001" {
		t.Errorf("expected operator_id=op-alpha-001, got %s", gotOp)
	}
}

func TestAuth_ApexAdmin_NoCorrespondentID(t *testing.T) {
	var gotRole, gotCorr string

	handler := Auth(func(w http.ResponseWriter, r *http.Request) {
		gotRole = RoleFromContext(r.Context())
		gotCorr = CorrespondentIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/operator/queue", nil)
	req.Header.Set("Authorization", "Bearer apex-admin")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotRole != "admin" {
		t.Errorf("expected role=admin, got %s", gotRole)
	}
	if gotCorr != "" {
		t.Errorf("expected empty correspondent_id for admin, got %s", gotCorr)
	}
}
