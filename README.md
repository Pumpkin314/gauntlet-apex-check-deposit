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
# Run Go unit tests (14+ tests)
make test

# Run Playwright E2E tests (11+ tests)
make test-e2e

# Generate full test report
scripts/generate-report.sh
# Output: reports/TEST_REPORT.md, reports/unit-tests.txt, reports/playwright/
```

## Documentation

- [Product Requirements](docs/prd.md) — Full PRD with spec details
- [Sprint Plan](docs/sprint_plan.md) — Execution plan with verification criteria
- [Architecture](docs/architecture.md) — System diagram, data flow, package structure
- [Decision Log](docs/decision_log.md) — 12 key architectural decisions with rationale
- [Submission](SUBMISSION.md) — Deliverables checklist and short write-up

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
