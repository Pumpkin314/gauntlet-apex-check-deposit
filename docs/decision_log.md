# Decision Log

Architectural and technology decisions for Apex Mobile Check Deposit (Milestone 1 / MVP).

---

## 1. Language: Go

**Choice:** Go 1.25

**Alternatives considered:** Java (Spring Boot), TypeScript (Node)

**Rationale:** Goroutines for concurrent deposit processing, single-binary deployment to Cloud Run, strong stdlib (`net/http`, `log/slog`, `database/sql`). Aligns with Apex's Ascend platform. No framework overhead for an MVP.

---

## 2. Database: PostgreSQL 16

**Choice:** PostgreSQL 16 via Docker (local) and Cloud SQL (GCP)

**Alternatives considered:** SQLite (spec allows), CockroachDB

**Rationale:** ACID transactions for ledger integrity, `LISTEN/NOTIFY` for real-time SSE without external message brokers, Row-Level Security hooks for future multi-tenant isolation, and `FOR UPDATE` / optimistic locking for state machine transitions. SQLite lacks `LISTEN/NOTIFY` and concurrent write support needed for the settlement engine.

---

## 3. State Machine: Hand-Rolled Transition Table

**Choice:** Static `map[TransferState][]TransferState` with optimistic locking (`UPDATE ... WHERE state = $expected RETURNING id`)

**Alternatives considered:** `looplab/fsm`, `stateless` library

**Rationale:** Only 8 states and 7 valid transitions. A library adds abstraction without value at this scale. The hand-rolled table is auditable in one glance, enforces transitions at the DB level (not just in-memory), and writes audit events atomically. Every transition writes a `transfer_events` row and calls `pg_notify`.

---

## 4. Internal Communication: REST (MVP)

**Choice:** REST/JSON between all services

**Alternatives considered:** gRPC (Protocol Buffers)

**Rationale:** Spec requires REST for MVP. All service interfaces are designed as Go interfaces (`VendorServiceClient`, `FundingServiceClient`, `LedgerService`) so gRPC implementations can be swapped in Milestone 2 without changing business logic. The orchestrator calls services through interfaces -- this is the gRPC extraction seam.

---

## 5. Ledger Model: True Double-Entry

**Choice:** Every money movement = exactly 2 ledger entries (DEBIT + CREDIT, same `movement_id`, same amount)

**Alternatives considered:** Single-row movements, running balance column

**Rationale:** `SUM(CASE WHEN side='CREDIT' THEN amount ELSE -amount END)` must always equal zero. This reconciliation invariant is industry-standard for financial systems. Append-only (never UPDATE/DELETE). Returns create 4 entries: reversal pair + fee pair.

---

## 6. Real-Time Transport: Server-Sent Events (SSE)

**Choice:** SSE via `pg_notify` -> Go listener -> HTTP stream

**Alternatives considered:** WebSocket, polling

**Rationale:** Only server-to-client push is needed (state transition updates for the dashboard). SSE is simpler than WebSocket, requires no additional library, and works through proxies/load balancers without special configuration. `pg_notify` provides the event source without adding Redis or Kafka.

---

## 7. Idempotency: Postgres-Only

**Choice:** `Idempotency-Key` header -> Postgres table with unique constraint on key

**Alternatives considered:** Redis + Postgres, in-memory cache

**Rationale:** Correct at demo/MVP scale. Single source of truth (Postgres) avoids cache invalidation bugs. Redis fast-path cache is a Milestone 2 optimization that doesn't change the API contract.

---

## 8. Authentication: Demo JWT Tokens

**Choice:** Hardcoded JWT tokens per demo user, middleware extracts `role` and `correspondent_id`

**Alternatives considered:** GCP Identity Platform, Auth0

**Rationale:** Spec doesn't require real auth for MVP. Same `middleware.Auth()` interface. GCP Identity Platform now supported via `AUTH_MODE=gcp`. Firebase JWT custom claims (`role`, `correspondent_id`, `operator_id`, `account_id`) replace demo tokens. Demo mode (`AUTH_MODE=demo`, default) remains unchanged for testing and scripted demos.

---

## 9. Settlement File: JSON (X9-Equivalent)

**Choice:** JSON file with `file_header`, `cash_letter`, `bundles`, and `checks` structure

**Alternatives considered:** Real X9.37 binary (moov-io/imagecashletter)

**Rationale:** Spec allows "structured equivalent." JSON is human-readable for demos and debugging. The structure mirrors X9 concepts (file header, cash letter, bundles, checks). X9 binary format now available via `SETTLEMENT_FORMAT=x9`. JSON remains default. `moov-io/imagecashletter` used for encoding.

---

## 10. Migrations: goose

**Choice:** `pressly/goose` v3 with SQL migration files

**Alternatives considered:** `golang-migrate`, manual SQL scripts

**Rationale:** Simpler single-file convention (one `.sql` file per migration with `-- +goose Up` / `-- +goose Down` markers). Embeds in the API Docker image for automatic migration on startup. No external binary needed at runtime.

---

## 11. Infrastructure as Code: Pulumi (Go) on GCP

**Choice:** Pulumi with Go SDK, two stacks (dev, prod)

**Alternatives considered:** Terraform (HCL), Cloud Deploy, manual `gcloud` scripts

**Rationale:** Same language as the application (Go), type-safe infrastructure definitions, Ascend alignment. Two-stack parameterization (dev: scale-to-zero, no backups; prod: min instances, backups enabled). `make deploy-dev` and `make deploy-prod` wrap the full build-push-deploy cycle.

---

## 12. Fee Handling: Config-Driven

**Choice:** Fee amount loaded from `correspondents.rules_config.fees.returned_check` at return processing time

**Alternatives considered:** Hardcoded `$30` constant, environment variable

**Rationale:** Fee varies per correspondent (seed data: Alpha = $30, Beta = $25). Hardcoding violates the multi-correspondent requirement. Config-driven fees enable correspondent onboarding without code changes.

---

## 13. Risk Dashboard — PRD-First Approach

**Choice:** Write a mini-PRD (`docs/risk_dashboard_prd.md`) defining metrics, API contract, and SQL queries before writing code.

**Alternatives considered:** Jump straight to implementation.

**Rationale:** Metrics are defined by operational need (rejection rate, float exposure, return rate, top investors, processing time). PRD-first ensures alignment on what to measure before building the dashboard. All queries use existing `transfers` table — no new tables or migrations required.

---

## 14. Operator Re-validation — Analyzing → Validating

**Choice:** Add `Analyzing → Validating` transition as a Milestone 2 state machine extension. Operators can trigger re-validation when better images are submitted.

**Alternatives considered:** New state (ReValidating), manual VSS override.

**Rationale:** Re-using the existing Validating state keeps the state machine minimal. The REVALIDATE action clears `review_reason` and transitions back to Validating so the transfer can be re-processed through VSS. CLAUDE.md updated to document the extension.

---

## 15. Redis Idempotency Cache — Read-Through Optimization

**Choice:** Add Redis as an optional read-through cache for the idempotency store. Postgres remains the authoritative source.

**Alternatives considered:** Redis-only (no Postgres fallback), in-memory LRU cache.

**Rationale:** Redis provides sub-millisecond lookups for repeat requests. Best-effort: all Redis operations are wrapped in error handling — if Redis is down or not configured (`REDIS_URL` unset), the system falls back to Postgres-only behavior. 24-hour TTL matches the idempotency key lifecycle. No data consistency risk since Postgres is always written first.

---

## 16. pgcrypto — Symmetric Encryption for MICR Data

**Choice:** Use `pgp_sym_encrypt`/`pgp_sym_decrypt` from pgcrypto extension for MICR data at rest. Dual-write migration strategy.

**Alternatives considered:** Application-level encryption (AES-GCM), AWS KMS/GCP KMS.

**Rationale:** Phase 1 (current): write encrypted `micr_data_enc` alongside plaintext `micr_data`. Reads stay on plaintext to avoid disrupting API responses. Phase 2: switch reads to encrypted column. Phase 3: drop plaintext columns. This ensures zero-downtime migration with rollback safety at each phase. Encryption key stored as a Postgres configuration parameter (`app.encryption_key`).
