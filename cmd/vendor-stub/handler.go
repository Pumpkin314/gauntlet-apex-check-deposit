package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// accountIDToCode maps well-known seed account UUIDs to their account codes.
// These correspond to the fixed test accounts defined in db/seed.sql.
var accountIDToCode = map[string]string{
	"a0000000-0000-0000-0000-000000000001": "ALPHA-001",
	"a0000000-0000-0000-0000-000000000002": "ALPHA-002",
	"a0000000-0000-0000-0000-000000000003": "ALPHA-003",
	"a0000000-0000-0000-0000-000000000004": "ALPHA-004",
	"a0000000-0000-0000-0000-000000000005": "ALPHA-005",
	"a0000000-0000-0000-0000-000000000006": "ALPHA-IRA",
	"a0000000-0000-0000-0000-000000000007": "BETA-001",
	"a0000000-0000-0000-0000-000000000008": "BETA-002",
	"a0000000-0000-0000-0000-000000000009": "BETA-IRA",
}

// ---- YAML types ----

type micrData struct {
	Routing     string `yaml:"routing"      json:"routing"`
	Account     string `yaml:"account"      json:"account"`
	CheckNumber string `yaml:"check_number" json:"check_number"`
}

type scenarioResponse struct {
	IQAStatus             string    `yaml:"iqa_status"`
	IQAErrorType          *string   `yaml:"iqa_error_type"`
	MICRData              *micrData `yaml:"micr_data"`
	OCRAmountOverride     *float64  `yaml:"ocr_amount_override"` // nil = echo submitted amount
	ConfidenceScore       float64   `yaml:"confidence_score"`
	DuplicateFlag         bool      `yaml:"duplicate_flag"`
	DuplicateOriginalTxID *string   `yaml:"duplicate_original_tx_id"`
}

type scenario struct {
	Name           string           `yaml:"name"`
	TriggerAccount string           `yaml:"trigger_account"`
	Response       scenarioResponse `yaml:"response"`
}

type scenarioFile struct {
	Scenarios []scenario `yaml:"scenarios"`
}

// ---- Handler ----

type handler struct {
	byCode map[string]*scenario // keyed by account code (ALPHA-001, etc.)
	byName map[string]*scenario // keyed by scenario name (clean_pass, etc.)
	seq    atomic.Int64
}

func newHandler(scenariosPath string) (*handler, error) {
	data, err := os.ReadFile(scenariosPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", scenariosPath, err)
	}

	var f scenarioFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", scenariosPath, err)
	}

	h := &handler{
		byCode: make(map[string]*scenario, len(f.Scenarios)),
		byName: make(map[string]*scenario, len(f.Scenarios)),
	}
	for i := range f.Scenarios {
		s := &f.Scenarios[i]
		h.byCode[s.TriggerAccount] = s
		h.byName[s.Name] = s
	}

	return h, nil
}

// ---- Request / Response wire types ----

type validateRequest struct {
	AccountID  string  `json:"account_id"`
	Amount     float64 `json:"amount"`
	FrontImage string  `json:"front_image"`
	BackImage  string  `json:"back_image"`
}

type validateResponse struct {
	IQAStatus             string    `json:"iqa_status"`
	IQAErrorType          *string   `json:"iqa_error_type"`
	MICRData              *micrData `json:"micr_data"`
	OCRAmount             float64   `json:"ocr_amount"`
	ConfidenceScore       float64   `json:"confidence_score"`
	DuplicateFlag         bool      `json:"duplicate_flag"`
	DuplicateOriginalTxID *string   `json:"duplicate_original_tx_id"`
	TransactionID         string    `json:"transaction_id"`
	ScenarioUsed          string    `json:"scenario_used"`
}

// validate handles POST /validate.
// Routing priority:
//  1. X-Scenario header (override for ad-hoc testing)
//  2. account_id UUID → account code → scenario name lookup
func (h *handler) validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var s *scenario

	if name := r.Header.Get("X-Scenario"); name != "" {
		s = h.byName[name]
		if s == nil {
			http.Error(w, fmt.Sprintf("unknown scenario: %s", name), http.StatusNotFound)
			return
		}
	} else {
		code, ok := accountIDToCode[req.AccountID]
		if !ok {
			http.Error(w, fmt.Sprintf("unknown account_id: %s", req.AccountID), http.StatusNotFound)
			return
		}
		s = h.byCode[code]
		if s == nil {
			http.Error(w, fmt.Sprintf("no scenario for account code: %s", code), http.StatusNotFound)
			return
		}
	}

	ocrAmount := req.Amount
	if s.Response.OCRAmountOverride != nil {
		ocrAmount = *s.Response.OCRAmountOverride
	}

	resp := validateResponse{
		IQAStatus:             s.Response.IQAStatus,
		IQAErrorType:          s.Response.IQAErrorType,
		MICRData:              s.Response.MICRData,
		OCRAmount:             ocrAmount,
		ConfidenceScore:       s.Response.ConfidenceScore,
		DuplicateFlag:         s.Response.DuplicateFlag,
		DuplicateOriginalTxID: s.Response.DuplicateOriginalTxID,
		TransactionID:         fmt.Sprintf("vss-%d-%d", time.Now().UnixMilli(), h.seq.Add(1)),
		ScenarioUsed:          s.Name,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
