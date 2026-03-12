package handlers

import (
	"encoding/json"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

type scenarioEntry struct {
	Name           string `yaml:"name" json:"name"`
	Description    string `yaml:"description" json:"description"`
	TriggerAccount string `yaml:"trigger_account" json:"trigger_account"`
}

type scenarioFile struct {
	Scenarios []scenarioEntry `yaml:"scenarios"`
}

// ScenariosHandler serves GET /scenarios.
type ScenariosHandler struct {
	scenarios []scenarioEntry
}

// NewScenariosHandler loads scenarios.yaml at startup.
func NewScenariosHandler(path string) (*ScenariosHandler, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f scenarioFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &ScenariosHandler{scenarios: f.Scenarios}, nil
}

// List handles GET /scenarios.
func (h *ScenariosHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.scenarios)
}
