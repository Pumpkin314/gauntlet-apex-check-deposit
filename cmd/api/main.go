package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/apex-checkout/check-deposit/cmd/api/handlers"
	"github.com/apex-checkout/check-deposit/cmd/api/middleware"
	"github.com/apex-checkout/check-deposit/internal/cloudauth"
	"github.com/apex-checkout/check-deposit/internal/events"
	"github.com/apex-checkout/check-deposit/internal/funding"
	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/orchestrator"
	"github.com/apex-checkout/check-deposit/internal/settlement"
	"github.com/apex-checkout/check-deposit/internal/store"
	"github.com/apex-checkout/check-deposit/internal/vendorclient"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("API_PORT")
	}
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
	// Retry db.Ping — Cloud SQL proxy socket may take a few seconds
	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		} else if i == 29 {
			log.Fatalf("failed to ping database after 30 retries: %v", err)
		} else {
			logger.Warn("waiting for database...", "attempt", i+1, "error", err)
			time.Sleep(2 * time.Second)
		}
	}

	// Create stores
	transferStore := store.NewTransferStore(db)
	accountStore := store.NewAccountStore(db)
	correspondentStore := store.NewCorrespondentStore(db)
	ledgerStore := store.NewLedgerStore(db)
	settlementStore := store.NewSettlementStore(db)

	// Create services
	ledgerService := ledger.New(ledgerStore, logger)
	fundingEngine := funding.NewEngine(accountStore)

	vssURL := os.Getenv("VSS_URL")
	if vssURL == "" {
		vssURL = "http://localhost:8081"
	}
	vssClient := vendorclient.NewHTTPClient(vssURL, cloudauth.NewClient(vssURL))

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

	// Notification store + handler
	notificationStore := store.NewNotificationStore(db)
	notificationHandler := &handlers.NotificationHandler{
		Notifications: notificationStore,
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

	returnHandler := handlers.NewReturnHandler(
		transferStore,
		accountStore,
		correspondentStore,
		notificationStore,
		ledgerService,
		logger,
	)

	// SSE broadcaster for real-time events
	broadcaster, err := events.NewBroadcaster(dbURL, "transfer_updates", logger)
	if err != nil {
		log.Fatalf("failed to start SSE broadcaster: %v", err)
	}

	eventsHandler := &handlers.EventsHandler{
		Broadcaster: broadcaster,
	}

	// Operator handler
	operatorHandler := &handlers.OperatorHandler{
		Transfers:        transferStore,
		OrchestratorDeps: orchDeps,
		Log:              logger,
	}

	// Settlement engine
	settlementBankURL := os.Getenv("SETTLEMENT_BANK_URL")
	if settlementBankURL == "" {
		settlementBankURL = "http://localhost:8082"
	}

	settlementEngine := &settlement.Engine{
		Transfers: settlementStore,
		Batches:   settlementStore,
		Updater:   transferStore,
		Events:    transferStore,
		Notifier:  transferStore,
		Log:       logger,
	}

	fullSettlementHandler := &handlers.SettlementHandler{
		Engine:        settlementEngine,
		Batches:       settlementStore,
		SettlementURL: settlementBankURL,
		HTTPClient:    cloudauth.NewClient(settlementBankURL),
		Log:           logger,
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
	mux.HandleFunc("GET /deposits", depositHandler.ListDeposits)
	mux.HandleFunc("POST /deposits", middleware.Idempotency(idempStore, depositHandler.CreateDeposit))
	mux.HandleFunc("GET /deposits/{id}", depositHandler.GetDeposit)
	mux.HandleFunc("GET /deposits/{id}/events", depositHandler.GetDepositEvents)
	mux.HandleFunc("GET /deposits/{id}/images/{side}", depositHandler.GetDepositImage)

	// Accounts
	mux.HandleFunc("GET /accounts", depositHandler.ListAccounts)
	mux.HandleFunc("GET /accounts/{code}", depositHandler.GetAccount)

	// Returns (settlement bank webhook — protected by bearer token)
	mux.HandleFunc("POST /returns", middleware.SettlementAuth(returnHandler.ProcessReturn))

	// Ledger
	mux.HandleFunc("GET /ledger/balances", ledgerHandler.GetBalances)
	mux.HandleFunc("GET /ledger/entries", ledgerHandler.GetEntries)
	mux.HandleFunc("GET /health/ledger", ledgerHandler.HealthLedger)

	// Settlement health
	settlementHealthHandler := &handlers.SettlementHealthHandler{
		Querier: &sqlSettlementQuerier{db: db},
	}
	mux.HandleFunc("GET /health/settlement", settlementHealthHandler.Health)
	mux.HandleFunc("POST /health/settlement/trigger", settlementHealthHandler.Trigger)

	// SSE events stream
	mux.HandleFunc("GET /events/stream", eventsHandler.Stream)

	// Operator (auth-gated)
	mux.HandleFunc("GET /operator/queue", middleware.Auth(operatorHandler.GetQueue))
	mux.HandleFunc("POST /operator/actions", middleware.Auth(operatorHandler.PostAction))

	// Settlement
	mux.HandleFunc("POST /settlement/trigger", fullSettlementHandler.Trigger)
	mux.HandleFunc("GET /settlement/status", fullSettlementHandler.Status)
	mux.HandleFunc("GET /settlement/batches", fullSettlementHandler.ListBatches)
	mux.HandleFunc("POST /admin/simulate-return", fullSettlementHandler.SimulateReturn)

	// Notifications (investor auth)
	mux.HandleFunc("GET /notifications", middleware.Auth(notificationHandler.GetNotifications))
	mux.HandleFunc("PATCH /notifications/{id}/read", middleware.Auth(notificationHandler.MarkRead))

	// Scenarios
	if scenariosHandler != nil {
		mux.HandleFunc("GET /scenarios", scenariosHandler.List)
	}

	// CORS middleware for frontend
	// Wrap with /api prefix stripping for Firebase Hosting rewrites.
	// /api/deposits → strips prefix → /deposits (Firebase path)
	// /deposits → routes directly (local dev, Cloud Run direct, tests)
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/", http.StripPrefix("/api", corsMiddleware(mux)))
	apiMux.Handle("/", corsMiddleware(mux))
	handler := http.Handler(apiMux)

	addr := fmt.Sprintf(":%s", port)
	logger.Info("API server starting", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// sqlSettlementQuerier implements handlers.SettlementQuerier using *sql.DB.
// It lives in main.go because cmd/api already owns the DB connection.
type sqlSettlementQuerier struct {
	db *sql.DB
}

func (s *sqlSettlementQuerier) CountUnbatched(ctx context.Context, cutoff time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfers
		WHERE state = 'FundsPosted'
		  AND settlement_batch_id IS NULL
		  AND submitted_at < $1`, cutoff).Scan(&count)
	return count, err
}

func (s *sqlSettlementQuerier) CountAllUnbatched(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfers
		WHERE state = 'FundsPosted'
		  AND settlement_batch_id IS NULL`).Scan(&count)
	return count, err
}

func (s *sqlSettlementQuerier) LastBatch(ctx context.Context) (*handlers.SettlementBatchInfo, error) {
	var b handlers.SettlementBatchInfo
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, created_at, COALESCE(record_count, 0), status
		FROM settlement_batches
		ORDER BY created_at DESC LIMIT 1`).Scan(&b.ID, &b.GeneratedAt, &b.Count, &b.Status)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *sqlSettlementQuerier) CreateBatch(ctx context.Context) (*handlers.SettlementBatchInfo, error) {
	// Find the first correspondent that has unbatched FundsPosted transfers.
	var corrID string
	var ready int
	err := s.db.QueryRowContext(ctx, `
		SELECT correspondent_id::text, COUNT(*)
		FROM transfers
		WHERE state = 'FundsPosted' AND settlement_batch_id IS NULL
		GROUP BY correspondent_id
		LIMIT 1`).Scan(&corrID, &ready)
	if err != nil {
		// No unbatched transfers — use first correspondent for an empty demo batch.
		if err2 := s.db.QueryRowContext(ctx, `SELECT id::text FROM correspondents LIMIT 1`).Scan(&corrID); err2 != nil {
			return nil, fmt.Errorf("no correspondents found: %w", err2)
		}
		ready = 0
	}

	var batchID string
	var createdAt time.Time
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO settlement_batches (correspondent_id, cutoff_date, status, record_count)
		VALUES ($1::uuid, CURRENT_DATE, 'ACKNOWLEDGED', $2)
		RETURNING id::text, created_at`, corrID, ready).Scan(&batchID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert settlement_batch: %w", err)
	}

	if ready > 0 {
		_, err = s.db.ExecContext(ctx, `
			UPDATE transfers
			SET settlement_batch_id = $1::uuid
			WHERE state = 'FundsPosted' AND settlement_batch_id IS NULL`, batchID)
		if err != nil {
			return nil, fmt.Errorf("link transfers to batch: %w", err)
		}
	}

	return &handlers.SettlementBatchInfo{
		ID:          batchID,
		GeneratedAt: createdAt,
		Count:       ready,
		Status:      "ACKNOWLEDGED",
	}, nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Scenario")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
