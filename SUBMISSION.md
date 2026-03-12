# Submission: Apex Mobile Check Deposit

## Candidate

Rajiv Nelakanti

## Project

Mobile check deposit system for brokerage accounts.

## Architecture Summary

Go backend with PostgreSQL, React + TypeScript frontend. Three services: API server (orchestration, state machine, ledger, settlement), Vendor Service stub (check image validation simulation), Settlement Bank stub (settlement file acceptance and return webhooks). All services run via Docker Compose (`make dev`).

**Key design choices:**
- Hand-rolled state machine with 8 states and optimistic locking for correctness
- True double-entry ledger with reconciliation invariant (SUM = 0)
- Interfaces at all service boundaries for future gRPC extraction
- SSE via pg_notify for real-time dashboard updates without Redis/Kafka
- Config-driven fees loaded from correspondent `rules_config`

## How to Run

```bash
# Prerequisites: Docker, Go 1.25+, Node 20+
make dev              # Start all services
# API: http://localhost:8080/health
# UI:  http://localhost:5173
make test             # Go unit tests (14+)
make test-e2e         # Playwright E2E tests (11+)
```

## Demo Scripts

```bash
scripts/demo-happy-path.sh      # Full lifecycle: submit → settle → complete
scripts/demo-rejection.sh       # Blur, glare, duplicate, over-limit rejections
scripts/demo-manual-review.sh   # MICR failure → operator queue → approve/reject
scripts/demo-return.sh          # Completed → return → reversal + fee + notification
```

## Deliverables

- [x] README.md
- [x] /docs/decision_log.md (12 decisions)
- [x] /docs/architecture.md (Mermaid diagrams)
- [x] /tests (unit + e2e)
- [x] /reports (test results)
- [x] .env.example
- [x] Vendor Service stub with 7 documented scenarios
- [x] Demo scripts (4 scripts, all paths)
- [x] Short write-up (see below)

## Short Write-Up

The system implements a complete mobile check deposit pipeline for brokerage accounts. The core design centers on three principles:

**State machine correctness.** The 8-state transition table is enforced at the database level with optimistic locking (`UPDATE ... WHERE state = $expected RETURNING id`). Every transition atomically writes an audit event and fires `pg_notify`, providing both an immutable decision trace and real-time UI updates without external infrastructure.

**Ledger integrity.** The double-entry model guarantees that every money movement produces exactly two entries (DEBIT + CREDIT) with the same `movement_id`. The reconciliation invariant (`SUM = 0`) is verified by `/health/ledger` and tested in unit and integration tests. Return processing always completes -- even if the investor balance goes negative, all 4 entries (reversal pair + fee pair) are posted and the account is flagged for collections.

**Stub design for realistic testing.** The Vendor Service stub uses a `scenarios.yaml` file to map account codes to deterministic VSS responses (clean pass, blur, glare, MICR failure, amount mismatch, duplicate, over-limit). An `X-Scenario` header override enables ad-hoc testing. This allows all 7 scenarios to be exercised from the UI or demo scripts without external dependencies.

**Risks and limitations.** No real encryption (synthetic data only; pgcrypto hooks are Milestone 2). No real authentication (demo JWT tokens). Settlement files are JSON, not X9 binary. Idempotency uses Postgres only (Redis cache is Milestone 2). The system is not production-hardened -- it demonstrates correctness, not operational readiness.
