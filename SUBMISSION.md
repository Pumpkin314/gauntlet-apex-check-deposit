# Submission: Apex Mobile Check Deposit

## Candidate

Rajiv Nelakanti

## Summary

Mobile check deposit system for brokerage accounts. Go backend with PostgreSQL, React + TypeScript frontend. Hand-rolled 8-state machine with optimistic locking, true double-entry ledger with reconciliation invariant, SSE real-time dashboard via pg_notify, and 7 configurable vendor service stub scenarios. Key trade-off: correctness over performance — every transition is DB-enforced with audit trail, every ledger movement is a balanced pair.

## How to Run

```bash
# Prerequisites: Docker, Go 1.25+, Node 20+
make dev              # Start all 6 services (Postgres, API, VSS, Settlement, Redis, React)
# API: http://localhost:8080/health
# UI:  http://localhost:5173

make test             # Go unit tests (156 test cases)
make test-e2e         # Playwright E2E tests (31 test cases)
```

## Demo Scripts

```bash
scripts/demo-happy-path.sh      # Full lifecycle: submit → settle → complete (25 assertions)
scripts/demo-rejection.sh       # Blur, glare, duplicate, over-limit rejections
scripts/demo-manual-review.sh   # MICR failure → operator queue → approve/reject
scripts/demo-return.sh          # Completed → return → reversal + fee + notification
```

## Test/Eval Results

- 156 Go unit tests + 31 Playwright E2E tests = **187 total** (minimum requirement: 10)
- 4 deterministic demo scripts with pass/fail assertions including timing
- Reports: `/reports/TEST_REPORT.md`, `/reports/unit-tests.txt`, `/reports/playwright/`

## Deliverables

- [x] README.md — setup, architecture, observability, API reference
- [x] /docs/decision_log.md (16 decisions with alternatives)
- [x] /docs/architecture.md (Mermaid diagrams, service boundaries, data flow)
- [x] /tests (156 unit + 31 E2E = 187 tests)
- [x] /reports (TEST_REPORT.md + Playwright HTML report)
- [x] .env.example (all environment variables documented)
- [x] Vendor Service stub with 7 documented scenarios (YAML-configurable)
- [x] Demo scripts (4 scripts covering all paths)
- [x] Short write-up (see below)

## Short Write-Up

The system implements a complete mobile check deposit pipeline for brokerage accounts. The core design centers on three principles:

**State machine correctness.** The 8-state transition table is enforced at the database level with optimistic locking (`UPDATE ... WHERE state = $expected RETURNING id`). Every transition atomically writes an audit event and fires `pg_notify`, providing both an immutable decision trace and real-time UI updates without external infrastructure.

**Ledger integrity.** The double-entry model guarantees that every money movement produces exactly two entries (DEBIT + CREDIT) with the same `movement_id`. The reconciliation invariant (`SUM = 0`) is verified by `/health/ledger` and tested in unit and integration tests. Return processing always completes — even if the investor balance goes negative, all 4 entries (reversal pair + fee pair) are posted and the account is flagged for collections.

**Stub design for realistic testing.** The Vendor Service stub uses a `scenarios.yaml` file to map account codes to deterministic VSS responses (clean pass, blur, glare, MICR failure, amount mismatch, duplicate, over-limit). An `X-Scenario` header override enables ad-hoc testing. This allows all 7 scenarios to be exercised from the UI or demo scripts without external dependencies.

## With One More Week, We Would

- Switch reads to encrypted MICR columns (Phase 2 of pgcrypto dual-write migration)
- Add gRPC implementations behind existing Go interfaces for VSS and Funding Service
- Add Playwright tests for the risk dashboard and operator revalidation flow
- Implement rate limiting and request throttling on deposit submission
- Add Grafana dashboards for operational monitoring (latency percentiles, error rates)

## Risks and Limitations

- Authentication is demo tokens for MVP; Firebase JWT mode is implemented but requires a GCP project to activate
- Settlement files default to JSON; X9 binary format is available via `SETTLEMENT_FORMAT=x9` but not tested against a real clearing house
- pgcrypto encryption is dual-write Phase 1 (writes encrypted + plaintext); reads still use plaintext columns
- No rate limiting or DDoS protection — system demonstrates correctness, not operational hardening
- Synthetic data only; no real PII, account numbers, or check images

## How Should ACME Evaluate Production Readiness?

1. **Run `make reset && make dev`** from a fresh clone — verify all services start and health checks pass
2. **Run all 4 demo scripts** — each should pass with zero failures
3. **Run `make test && make test-e2e`** — 187 tests, all green
4. **Submit a deposit through the UI** — verify real-time state transitions on the Flow Dashboard
5. **Check `/admin/ledger`** — reconciliation must show $0.00 after any sequence of operations
6. **Trigger a return** — verify 6 ledger entries (2 provisional + 2 reversal + 2 fee), investor notified
7. **Review the decision log** — 16 entries documenting every architectural choice with alternatives considered
