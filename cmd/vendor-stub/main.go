package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("VSS_PORT")
	}
	if port == "" {
		port = "8081"
	}

	scenariosPath := os.Getenv("SCENARIOS_PATH")
	if scenariosPath == "" {
		scenariosPath = "test-scenarios/scenarios.yaml"
	}

	h, err := newHandler(scenariosPath)
	if err != nil {
		log.Fatalf("vendor-stub: failed to load scenarios from %s: %v", scenariosPath, err)
	}
	log.Printf("vendor-stub: loaded %d scenarios from %s", len(h.byName), scenariosPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /validate", h.validate)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("vendor-stub: Vendor Service Stub starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("vendor-stub: server failed: %v", err)
	}
}
