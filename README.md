# Apex Mobile Check Deposit

Mobile check deposit system for brokerage accounts. Go backend, PostgreSQL, React + TypeScript frontend.

Investors submit checks via a mobile-responsive web app. The system validates images via a Vendor Service stub, enforces business rules via a Funding Service, posts double-entry ledger records, generates settlement files at EOD, and handles return/reversal scenarios with fee processing. An operator review workflow handles flagged deposits with full audit logging.

## Quick Start

```bash
# Prerequisites: Docker Desktop, Go 1.25+, Node 20+

# Start all services (Postgres, API, VSS stub, Settlement stub, React)
make dev

# Verify services are running
curl http://localhost:8080/health          # API → {"status":"ok"}
curl http://localhost:8081/health          # Vendor Service Stub
curl http://localhost:8082/health          # Settlement Bank Stub
open http://localhost:5173                 # React app

# Stop all services
make down

# Reset database (drop + re-migrate + seed)
make reset
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| API Server | 8080 | Go API: orchestration, state machine, ledger, settlement |
| Vendor Service Stub | 8081 | Check image validation simulation (IQA, MICR, duplicates) |
| Settlement Bank Stub | 8082 | Settlement file submission and return webhooks |
| React Frontend | 5173 | Mobile deposit, status page, operator queue, ledger dashboard |
| PostgreSQL | 5433 | Database (mapped from container port 5432) |

## Architecture

See [docs/architecture.md](docs/architecture.md) for diagrams and data flow.

**Key design choices:**
- Hand-rolled state machine (8 states, optimistic locking, audit events on every transition)
- True double-entry ledger (reconciliation invariant: SUM = 0)
- Interfaces at service boundaries (gRPC extraction seam for Milestone 2)
- SSE via `pg_notify` for real-time dashboard (no Redis/Kafka)

## API Endpoints

### Deposits
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/deposits` | Submit a check deposit (JSON or multipart with images) |
| `GET` | `/deposits/{id}` | Get transfer details |
| `GET` | `/deposits/{id}/events` | Get transfer audit trail |
| `GET` | `/deposits/{id}/images/{side}` | Get check image (front/back) |

### Operator Queue
| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/operator/queue` | Bearer token | List flagged transfers for review |
| `POST` | `/operator/actions` | Bearer token | Approve or reject a flagged transfer |

### Ledger
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/ledger/balances` | Account balances |
| `GET` | `/ledger/entries` | Ledger entries (filter by `?transfer_id=`) |
| `GET` | `/health/ledger` | Reconciliation check (`{ healthy: true, sum: "0.00" }`) |

### Settlement
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/settlement/trigger` | Manually trigger EOD settlement batch |
| `GET` | `/settlement/status` | Settlement batch summary |
| `GET` | `/settlement/batches` | List all settlement batches |
| `POST` | `/admin/simulate-return` | Simulate a settlement bank return |

### Returns
| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/returns` | Settlement token | Settlement bank return webhook |

### Other
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | API health check |
| `GET` | `/health/settlement` | Settlement monitoring (unbatched transfers) |
| `GET` | `/events/stream` | SSE stream for real-time state updates |
| `GET` | `/scenarios` | List available VSS test scenarios |
| `GET` | `/notifications` | Investor notifications (auth required) |

## Vendor Service Stub — Scenarios

The VSS stub routes requests based on account code (or `X-Scenario` header override):

| Account Code | Scenario | Result |
|-------------|----------|--------|
| `ALPHA-001` | Clean pass | IQA pass, MICR readable, confidence 0.97 → FundsPosted |
| `ALPHA-002` | IQA blur | IQA fail → Rejected (VSS_IQA_BLUR) |
| `ALPHA-003` | IQA glare | IQA fail → Rejected (VSS_IQA_GLARE) |
| `ALPHA-004` | MICR failure | MICR unreadable → Analyzing (flagged for operator review) |
| `ALPHA-005` | Amount mismatch | OCR amount differs → Analyzing (flagged for review) |
| `BETA-001` | Duplicate | Duplicate flag → Rejected (VSS_DUPLICATE_DETECTED) |
| `ALPHA-IRA` | IRA clean pass | Clean pass with IRA contribution type defaulting |

All scenarios are defined in `test-scenarios/scenarios.yaml`.

## Demo Scripts

All scripts are self-contained and assert expected outcomes. Run after `make dev`:

```bash
# Full lifecycle: submit → validate → approve → post → settle → complete
scripts/demo-happy-path.sh

# Four rejection paths: blur, glare, duplicate, over-limit
scripts/demo-rejection.sh

# MICR failure → operator queue → approve/reject + audit trail
scripts/demo-manual-review.sh

# Completed → return → reversal entries + fee + negative balance + notification
scripts/demo-return.sh
```

**Expected output:** Each script prints PASS/FAIL for every assertion and exits 0 on success.

## Development Commands

```bash
make dev          # docker compose up --build -d
make down         # docker compose down
make logs         # docker compose logs -f
make reset        # Drop DB, re-run migrations + seed
make test         # go test ./... -v
make test-e2e     # cd web && npx playwright test
```

## Testing

```bash
# Run Go unit tests (94 tests across 11 packages)
make test

# Run Playwright E2E tests (29 tests across 5 suites)
make test-e2e

# Generate full test report
scripts/generate-report.sh
# Output: reports/TEST_REPORT.md, reports/unit-tests.txt, reports/playwright/
```

**Test coverage by area:** state machine (25), funding rules (15), VSS stub (14), ledger (11), returns (11), settlement (20), auth (5), PII redaction (11), E2E flows (29).

## Observability & Monitoring

The system provides full observability across the deposit lifecycle without external infrastructure (no Redis, Kafka, or OpenTelemetry in MVP).

### Health Endpoints

| Endpoint | What It Checks | Healthy Response |
|----------|---------------|------------------|
| `GET /health` | API server liveness | `{"status": "ok"}` |
| `GET /health/ledger` | Ledger reconciliation invariant (SUM = 0) | `{"healthy": true, "sum": "0.00"}` |
| `GET /health/settlement` | Unbatched transfers past cutoff | `{"healthy": true, "unbatched_count": 0, "ready_count": N}` |

### Per-Deposit Decision Trace

Every deposit generates a full event chain in the `transfer_events` table, queryable via `GET /deposits/{id}/events`:

```
submitted        → inputs (amount, account_code)
vss_called       → what was sent to Vendor Service
vss_result       → IQA status, confidence, MICR data, duplicate flag
state_changed    → every transition (with from/to states)
fs_evaluated     → funding decision + reason code
ledger_posted    → debit/credit accounts + amount
operator_action  → who approved/rejected + reason (if flagged)
settlement_completed → batch_id, acknowledged_at
return_received  → reason code (if bounced)
return_processed → reversal amount + fee amount
```

### Structured Logging

All Go services use `log/slog` with JSON output. Key fields always present:

- `transfer_id` and `correspondent_id` on every transfer-related log line
- PII redaction via `internal/logging/redact.go` — routing/account numbers masked to last 4 digits (`*****1234`)
- Error context propagated with `slog.ErrorContext`

Key log locations:
- Orchestrator flow: `internal/orchestrator/flow.go` (VSS calls, funding decisions, ledger posting)
- Return processing: `internal/returns/handler.go` (reversal, fee, collections flagging)
- Settlement engine: `internal/settlement/engine.go` (batch generation, file writes, acknowledgment)

### Real-Time Dashboard (SSE)

State changes propagate to connected clients in real-time:

```
PostgreSQL pg_notify('transfer_updates', payload)
    → Go SSE broadcaster (internal/events/sse.go)
        → HTTP stream at GET /events/stream
            → React FlowPage dashboard
```

No polling. No external message broker. 30-second keepalive pings detect disconnected clients.

### Ledger Reconciliation

The double-entry invariant (`SUM(CREDIT) - SUM(DEBIT) = 0`) is:
- Enforced at write time: `internal/ledger/service.go:PostDoubleEntry()` always writes exactly 2 entries
- Verified via health check: `GET /health/ledger` runs the reconciliation query
- Tested: `internal/ledger/service_test.go` (11 tests including reconciliation after returns)

### Settlement Monitoring

`GET /health/settlement` detects operational issues:
- `unbatched_count`: transfers past midnight cutoff without a batch (should be 0)
- `ready_count`: FundsPosted transfers ready for next batch
- `last_batch`: most recent batch metadata (status, submitted_at, acknowledged_at)

## Documentation

- [Product Requirements](docs/prd.md) — Full PRD with spec details
- [Sprint Plan](docs/sprint_plan.md) — Execution plan with verification criteria
- [Architecture](docs/architecture.md) — System diagram, data flow, package structure
- [Decision Log](docs/decision_log.md) — 12 key architectural decisions with rationale
- [Submission](SUBMISSION.md) — Deliverables checklist and short write-up
- [Verification](VERIFICATION.md) — Spec requirement traceability (every requirement mapped to code + tests)
- [Rubric Audit](docs/rubric_audit.md) — Evaluation rubric gap analysis

## GCP Deployment

### Prerequisites

CLIs on PATH:
- `docker` (running)
- `pulumi`
- `gcloud`

### Auth Setup

```bash
# 1. Login to GCP
gcloud auth login

# 2. Configure Docker to push to Artifact Registry
gcloud auth configure-docker us-central1-docker.pkg.dev

# 3. Pulumi backend (if not already configured)
pulumi login          # Pulumi Cloud
# or: pulumi login --local
```

### Deploy

```bash
make deploy-dev GCP_PROJECT=apex-check-deposit
```

See [docs/architecture.md](docs/architecture.md#gcp-infrastructure-pulumi) for the full infrastructure diagram and resource inventory.

## Roadmap

### MVP (Milestone 1) — Current

Everything in this repo. Full deposit lifecycle, 7 VSS scenarios, operator workflow, settlement, returns, 123 tests.

### Milestone 2 — Hardening + Infra Signals

| Item | What Changes | MVP Seam |
|------|-------------|----------|
| gRPC extraction | Funding Service and/or Ledger as separate gRPC services | Interfaces already defined (`FundingServiceClient`, `LedgerService`) — swap implementation, zero caller changes |
| Redis idempotency cache | Fast-path TTL cache in front of Postgres `idempotency_keys` table | Same `Idempotency-Key` API contract; Redis is a read-through optimization |
| X9 binary settlement files | Replace JSON with real X9.37 via `moov-io/imagecashletter` | `SettlementFile` struct maps 1:1 to X9 concepts already |
| pgcrypto encryption | Column-level encrypt/decrypt via accessor functions | `internal/store/` is the only SQL touchpoint — add there |
| RLS policy refinement | Tighter row-level security per correspondent | `db/migrations/009_rls_policies.sql` already scaffolded |
| `CompletedFinalized` state | Background finalization job after settlement window closes | Add one state + one transition to `states.go` |

### Milestone 3 — Differentiation

| Item | What It Adds |
|------|-------------|
| Risk dashboard (`/admin/risk`) | Float exposure by correspondent, return rates, top investors by outstanding provisional credit |
| GCP Identity Platform | Real auth — Firebase JWT custom claims replace demo tokens via same `middleware.Auth()` interface |
| Decision trace search | Cross-transfer search on `transfer_events` with admin tab |
| QA/staging stack | Third Pulumi stack for pre-prod validation |

See `docs/prd.md` sections 12.2–12.3 for full details. Architecture decisions in `docs/decision_log.md` (#4, #7, #8, #9) document each MVP seam designed for these upgrades.

## Environment Variables

See [.env.example](.env.example) for all configuration options:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://apex:apex@localhost:5432/...` | PostgreSQL connection string |
| `API_PORT` | `8080` | API server port |
| `VSS_URL` | `http://localhost:8081` | Vendor Service stub URL |
| `SETTLEMENT_BANK_URL` | `http://localhost:8082` | Settlement Bank stub URL |
| `JWT_SECRET` | `dev-secret-change-in-production` | JWT signing secret |
| `SETTLEMENT_BANK_TOKEN` | `dev-settlement-token` | Settlement webhook auth token |
| `SCENARIOS_PATH` | `./test-scenarios/scenarios.yaml` | VSS scenario definitions |
