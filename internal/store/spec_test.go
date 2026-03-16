package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSpecConstants(t *testing.T) {
	cases := []struct {
		name     string
		got      string
		expected string
	}{
		{"Type", SpecType, "MOVEMENT"},
		{"Memo", SpecMemo, "FREE"},
		{"SubType", SpecSubType, "DEPOSIT"},
		{"TransferType", SpecTransferType, "CHECK"},
		{"Currency", SpecCurrency, "USD"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.expected {
				t.Errorf("Spec%s = %q, want %q", c.name, c.got, c.expected)
			}
		})
	}
}

func TestTransferJSON_SpecFields(t *testing.T) {
	tr := Transfer{
		ID:              "test-id",
		AccountID:       "acct-1",
		FromAccountID:   "omnibus-1",
		CorrespondentID: "corr-1",
		Amount:          500.00,
		Currency:        SpecCurrency,
		Type:            SpecType,
		SubType:         SpecSubType,
		TransferType:    SpecTransferType,
		Memo:            SpecMemo,
		State:           "Requested",
		SubmittedAt:     time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	checks := map[string]string{
		"Currency":     SpecCurrency,
		"Type":         SpecType,
		"SubType":      SpecSubType,
		"TransferType": SpecTransferType,
		"Memo":         SpecMemo,
	}
	for field, want := range checks {
		got, ok := m[field].(string)
		if !ok {
			t.Errorf("field %q not found or not a string in JSON", field)
			continue
		}
		if got != want {
			t.Errorf("JSON field %q = %q, want %q", field, got, want)
		}
	}
}
