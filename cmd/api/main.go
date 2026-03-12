package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	_ "github.com/lib/pq"

	"github.com/apex-checkout/check-deposit/cmd/api/handlers"
	"github.com/apex-checkout/check-deposit/cmd/api/middleware"
	"github.com/apex-checkout/check-deposit/internal/events"
	"github.com/apex-checkout/check-deposit/internal/funding"
	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/orchestrator"
	"github.com/apex-checkout/check-deposit/internal/store"
	"github.com/apex-checkout/check-deposit/internal/vendorclient"
)

func main() {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Connect to database
	dbURL := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	// Create stores
	transferStore := store.NewTransferStore(db)
	accountStore := store.NewAccountStore(db)
	correspondentStore := store.NewCorrespondentStore(db)
	ledgerStore := store.NewLedgerStore(db)

	// Create services
	ledgerService := ledger.New(ledgerStore, logger)
	fundingEngine := funding.NewEngine(accountStore)

	vssURL := os.Getenv("VSS_URL")
	if vssURL == "" {
		vssURL = "http://localhost:8081"
	}
	vssClient := vendorclient.NewHTTPClient(vssURL)

	// Orchestrator deps
	orchDeps := orchestrator.Deps{
		Updater:  transferStore,
		Events:   transferStore,
		Notifier: transferStore,
		VSS:      vssClient,
		Funding:  fundingEngine,
		Ledger:   ledgerService,
		Log:      logger,
	}

	// Idempotency store
	idempStore := &middleware.IdempotencyStore{DB: db}

	// Handlers
	depositHandler := &handlers.DepositHandler{
		Transfers:        transferStore,
		Accounts:         accountStore,
		Correspondents:   correspondentStore,
		OrchestratorDeps: orchDeps,
		Log:              logger,
	}

	ledgerHandler := &handlers.LedgerHandler{
		Ledger: ledgerService,
	}

	// SSE broadcaster for real-time events
	broadcaster, err := events.NewBroadcaster(dbURL, "transfer_updates", logger)
	if err != nil {
		log.Fatalf("failed to start SSE broadcaster: %v", err)
	}

	eventsHandler := &handlers.EventsHandler{
		Broadcaster: broadcaster,
	}

	// Scenarios handler
	scenariosPath := os.Getenv("SCENARIOS_PATH")
	if scenariosPath == "" {
		scenariosPath = "test-scenarios/scenarios.yaml"
	}
	scenariosHandler, err := handlers.NewScenariosHandler(scenariosPath)
	if err != nil {
		logger.Warn("scenarios handler not loaded (file not found)", "error", err)
		scenariosHandler = nil
	}

	// Routes
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Deposits
	mux.HandleFunc("POST /deposits", middleware.Idempotency(idempStore, depositHandler.CreateDeposit))
	mux.HandleFunc("GET /deposits/{id}", depositHandler.GetDeposit)
	mux.HandleFunc("GET /deposits/{id}/events", depositHandler.GetDepositEvents)
	mux.HandleFunc("GET /deposits/{id}/images/{side}", depositHandler.GetDepositImage)

	// Ledger
	mux.HandleFunc("GET /ledger/balances", ledgerHandler.GetBalances)
	mux.HandleFunc("GET /ledger/entries", ledgerHandler.GetEntries)
	mux.HandleFunc("GET /health/ledger", ledgerHandler.HealthLedger)

	// SSE events stream
	mux.HandleFunc("GET /events/stream", eventsHandler.Stream)

	// Scenarios
	if scenariosHandler != nil {
		mux.HandleFunc("GET /scenarios", scenariosHandler.List)
	}

	// CORS middleware for frontend
	handler := corsMiddleware(mux)

	addr := fmt.Sprintf(":%s", port)
	logger.Info("API server starting", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
