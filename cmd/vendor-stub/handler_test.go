package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const scenariosPath = "../../test-scenarios/scenarios.yaml"

func mustNewHandler(t *testing.T) *handler {
	t.Helper()
	h, err := newHandler(scenariosPath)
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	return h
}

func postValidate(h *handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.validate(w, req)
	return w
}

func TestValidate_CleanPass(t *testing.T) {
	h := mustNewHandler(t)

	w := postValidate(h, `{"account_id":"a0000000-0000-0000-0000-000000000001","amount":500.00}`, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp validateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.IQAStatus != "pass" {
		t.Errorf("iqa_status: want %q, got %q", "pass", resp.IQAStatus)
	}
	if resp.IQAErrorType != nil {
		t.Errorf("iqa_error_type: want nil, got %v", *resp.IQAErrorType)
	}
	if resp.MICRData == nil {
		t.Fatal("micr_data: want populated, got nil")
	}
	if resp.MICRData.Routing == "" {
		t.Error("micr_data.routing: want non-empty")
	}
	if resp.MICRData.Account == "" {
		t.Error("micr_data.account: want non-empty")
	}
	if resp.MICRData.CheckNumber == "" {
		t.Error("micr_data.check_number: want non-empty")
	}
	if resp.ConfidenceScore != 0.97 {
		t.Errorf("confidence_score: want 0.97, got %f", resp.ConfidenceScore)
	}
	if resp.DuplicateFlag {
		t.Error("duplicate_flag: want false")
	}
	if resp.ScenarioUsed != "clean_pass" {
		t.Errorf("scenario_used: want %q, got %q", "clean_pass", resp.ScenarioUsed)
	}
	if resp.TransactionID == "" {
		t.Error("transaction_id: want non-empty")
	}
}

func TestValidate_UnknownAccount_Returns404(t *testing.T) {
	h := mustNewHandler(t)

	w := postValidate(h, `{"account_id":"00000000-0000-0000-0000-000000000000","amount":500.00}`, nil)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestValidate_XScenarioOverride(t *testing.T) {
	h := mustNewHandler(t)

	// Unknown account_id, but X-Scenario header forces clean_pass.
	w := postValidate(h,
		`{"account_id":"00000000-0000-0000-0000-000000000000","amount":250.00}`,
		map[string]string{"X-Scenario": "clean_pass"},
	)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp validateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ScenarioUsed != "clean_pass" {
		t.Errorf("scenario_used: want %q, got %q", "clean_pass", resp.ScenarioUsed)
	}
	if resp.IQAStatus != "pass" {
		t.Errorf("iqa_status: want %q, got %q", "pass", resp.IQAStatus)
	}
}

func TestValidate_XScenarioUnknown_Returns404(t *testing.T) {
	h := mustNewHandler(t)

	w := postValidate(h,
		`{"account_id":"a0000000-0000-0000-0000-000000000001","amount":500.00}`,
		map[string]string{"X-Scenario": "nonexistent_scenario"},
	)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestValidate_OCRAmount_EchoesSubmitted(t *testing.T) {
	h := mustNewHandler(t)

	w := postValidate(h, `{"account_id":"a0000000-0000-0000-0000-000000000001","amount":1234.56}`, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp validateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.OCRAmount != 1234.56 {
		t.Errorf("ocr_amount: want 1234.56, got %f", resp.OCRAmount)
	}
}

func TestScenariosLoadedAtStartup(t *testing.T) {
	// Verify scenarios are pre-loaded (byName and byCode populated) rather than read per request.
	h := mustNewHandler(t)

	if len(h.byName) == 0 {
		t.Error("byName map is empty — scenarios not loaded at startup")
	}
	if len(h.byCode) == 0 {
		t.Error("byCode map is empty — scenarios not loaded at startup")
	}
	if _, ok := h.byName["clean_pass"]; !ok {
		t.Error("clean_pass scenario not found in byName")
	}
	if _, ok := h.byCode["ALPHA-001"]; !ok {
		t.Error("ALPHA-001 not found in byCode")
	}
}
