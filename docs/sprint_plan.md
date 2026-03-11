# Apex Mobile Check Deposit — Sprint Plan

> **Companion to:** `prd_milestone1_mvp.md`
> **Framework:** Tracer bullets — each Epic threads a complete feature E2E
> **Parallelization:** PRs marked 🔀 can be worked in parallel on separate worktrees
> **Verification:** Every PR has explicit pass/fail criteria. No PR merges without green checks.

---

## Parallelization Strategy

Each Epic has PRs that touch different layers (Go backend, React frontend, stub services, tests, infra). Where PRs within an Epic are independent — meaning they don't modify the same files — they can be assigned to parallel subagents on separate git worktrees.

**Rules for parallel PR safety:**

1. PRs within an Epic that touch **different directories** (e.g., `internal/funding/` vs `web/src/`) can run in parallel.
2. PRs that both touch `internal/orchestrator/` or the same API route handler CANNOT run in parallel — they must be sequential.
3. The **schema migration** (Epic 0) must merge before ANY feature PR.
4. Each PR's verification criteria are runnable independently — a subagent can verify its own PR without needing another PR's code.
5. When parallel PRs merge, run the full verification suite for the Epic before starting the next Epic.

---

## Epic 0 — The Rail

> **Goal:** Every service starts. No business logic. Schema exists. `make dev` works.
> **Rubric points unlocked:** 0 (infrastructure only), but enables all subsequent points.
> **Estimated effort:** Foundation day.

### PR 0.1 — Repo skeleton + Docker Compose

**Commits:**
1. Initialize Go module (`go mod init`), create directory tree per PRD Section 13
2. `docker-compose.yml`: Postgres 16, Go API placeholder, VSS stub placeholder, Settlement stub placeholder, React dev server (Vite)
3. `Makefile` with targets: `dev` (docker compose up --build), `down`, `logs`, `test`, `migrate`
4. `.env.example` with all vars from PRD Section 14
5. `.gitignore` for Go binaries, node_modules, .env, reports/

**Verification criteria:**
```
□ `make dev` starts all 5 containers without errors
□ `docker compose ps` shows all containers healthy
□ Postgres accepts connections on :5432
□ Go API responds 200 on GET /health (returns {"status": "ok"})
□ VSS stub responds 200 on GET /health
□ Settlement stub responds 200 on GET /health
□ React dev server serves page on :5173
□ `make down` stops all containers cleanly
□ .env.example exists with all vars from PRD
```

---

### PR 0.2 — Database schema (all tables)

**Depends on:** PR 0.1 merged

**Commits:**
1. Install goose in API Dockerfile
2. Migration 001: `correspondents` table
3. Migration 002: `accounts` table with FK to correspondents
4. Migration 003: `transfers` table with all columns from PRD 4.3, CHECK constraints, unique constraint on `(account_id, vendor_transaction_id)`
5. Migration 004: `ledger_entries` table with movement_id, side CHECK, unique constraint per PRD 4.4
6. Migration 005: `transfer_events` table with index
7. Migration 006: `idempotency_keys` table
8. Migration 007: `settlement_batches` table
9. Migration 008: `notifications` table
10. Migration 009: RLS policies on all tables per PRD 4.9
11. Add `goose up` to `make dev` target (runs before API starts)

**Verification criteria:**
```
□ `make dev` runs all migrations without errors
□ \dt in psql shows all 8 tables
□ \d transfers shows all columns with correct types and constraints
□ \d ledger_entries shows movement_id, side CHECK, unique constraint
□ INSERT with invalid state value fails (CHECK constraint)
□ INSERT with amount <= 0 fails (CHECK constraint)
□ INSERT with duplicate (account_id, vendor_transaction_id) fails (UNIQUE)
□ RLS policies exist: SELECT * FROM pg_policies shows policies on all tables
□ goose down rolls back each migration cleanly (test down then up again)
```

---

### PR 0.3 — Seed data + scenarios.yaml

**Depends on:** PR 0.2 merged

**Commits:**
1. Create `test-scenarios/scenarios.yaml` with all 7 scenarios mapped to account codes
2. Seed SQL script (`db/seed.sql`): 2 correspondents (Alpha rules_config with deposit_limit:5000, no ineligible types; Beta rules_config with deposit_limit:5000, ineligible_account_types:["IRA"])
3. Seed: 12 accounts per PRD 4.2 table
4. Add `make seed` target that runs seed script. Add to `make dev` after migrations.
5. Add `make reset` target: drops DB, re-runs migrations + seed

**Verification criteria:**
```
□ `make dev` creates both correspondents with correct rules_config
□ SELECT count(*) FROM accounts = 12
□ ALPHA-IRA has type='IRA', correspondent=Alpha
□ BETA-IRA has type='IRA', correspondent=Beta
□ Beta's rules_config.ineligible_account_types contains 'IRA'
□ OMNIBUS-ALPHA and OMNIBUS-BETA have type='OMNIBUS'
□ FEE-APEX has type='FEE', correspondent_id IS NULL
□ scenarios.yaml parses correctly (validate YAML structure)
□ Each scenario maps to exactly one account code
□ `make reset` cleanly drops and recreates everything
```

---

### PR 0.4 — React app shell + route stubs

**Can parallel with:** PR 0.2, PR 0.3 (different directory: `web/`)  🔀

**Commits:**
1. `npm create vite` with React + TypeScript template in `web/`
2. Install dependencies: react-router-dom, tailwind (CDN classes only)
3. Route stubs: `/deposit`, `/status/:id`, `/admin/flow`, `/admin/queue`, `/admin/ledger`
4. App shell: header with role switcher dropdown (investor-alpha, operator-alpha, apex-admin, etc.), responsive sidebar for admin routes
5. Each route renders placeholder text: "Deposit Form — coming in TB1", etc.
6. API client stub (`web/src/api/client.ts`): base URL from env, fetch wrapper with auth header injection

**Verification criteria:**
```
□ `npm run dev` in web/ serves the app on :5173
□ Navigating to /deposit shows deposit placeholder
□ Navigating to /admin/flow shows admin placeholder
□ Role switcher dropdown appears in header with all 5 demo users
□ Switching roles updates the displayed role
□ Admin routes (/admin/*) show sidebar navigation
□ Mobile viewport (390px wide) shows stacked layout, no sidebar
□ Desktop viewport (1200px wide) shows sidebar layout
□ No console errors in browser
```

---

### Epic 0 Completion Verification
```
□ `make dev` from clean clone: all services start, migrations run, seed loads
□ All 4 PR verification checklists pass
□ Go API, VSS stub, Settlement stub all respond on /health
□ React app renders with routes and role switcher
□ Database has all 8 tables with correct schemas
□ 12 accounts seeded, 2 correspondents seeded
□ `make reset` works (clean slate)
□ README has basic setup instructions
```

---

## Epic 1 — Pulumi Bootstrap

> **Goal:** Deploy the skeleton to GCP. `make deploy-dev` works.
> **Can start in parallel with TB1** if different developer/agent. 🔀
> **Rubric points unlocked:** Contributes to Architecture (20 pts) — deployment topology documented.

### PR 1.1 — Pulumi program + dev stack

**Commits:**
1. Initialize Pulumi Go program in `infra/`
2. Stack config for `dev`: Cloud Run services (API, VSS, Settlement stub — scale to zero), Cloud SQL (Postgres 16, smallest tier), GCS bucket
3. Dockerfile for Go API (multi-stage build, final image <20MB)
4. Dockerfiles for VSS stub and Settlement stub
5. `make deploy-dev` target: builds images, pushes to Artifact Registry, `pulumi up --stack dev`
6. Pulumi outputs: API URL, Cloud SQL connection string

**Verification criteria:**
```
□ `pulumi up --stack dev` completes without errors
□ Cloud Run services appear in GCP console
□ Cloud SQL instance is running
□ API health check passes at the Cloud Run URL
□ VSS and Settlement stubs respond on their Cloud Run URLs
□ `make dev` (local) still works independently — no Pulumi dependency
□ Container images are <20MB each
```

---

### PR 1.2 — Prod stack + deploy target

**Depends on:** PR 1.1 merged

**Commits:**
1. Stack config for `prod`: same topology, min instances = 1, Cloud SQL backups enabled
2. `make deploy-prod` target
3. Secret Manager entries: JWT_SECRET, SETTLEMENT_BANK_TOKEN (Pulumi-provisioned)
4. Document stack differences in `infra/README.md`

**Verification criteria:**
```
□ `pulumi up --stack prod` completes without errors
□ Prod Cloud Run services have minInstances: 1
□ Cloud SQL has automated backups enabled
□ Secrets exist in Secret Manager
□ API health check passes at prod URL
□ Dev and prod stacks are independent (destroying one doesn't affect the other)
```

---

## TB1 — Happy Path E2E

> **Goal:** Submit a deposit, watch it flow through all happy-path states, see ledger entries, verify reconciliation.
> **Rubric points unlocked:** ~40-50 (partial core correctness, partial stub, partial DX, partial tests)
> **This is the critical path. Everything else builds on it.**

### PR 1.1 — Orchestrator + state machine core

**Depends on:** Epic 0 complete

**Commits:**
1. `internal/orchestrator/states.go`: State type, all 8 state constants, `validTransitions` map
2. `internal/orchestrator/transition.go`: `Transition(ctx, transferID, fromState, toState)` with optimistic lock SQL
3. `internal/store/transfers.go`: Create, GetByID, UpdateState (the optimistic lock query)
4. `internal/orchestrator/events.go`: write `transfer_events` on every transition (state_changed step)
5. Unit tests: valid transitions succeed, invalid transitions return typed error, optimistic lock prevents double-transition

**Verification criteria:**
```
□ TestValidTransition: Requested→Validating succeeds
□ TestValidTransition: Validating→Analyzing succeeds
□ TestValidTransition: all 9 valid transitions from PRD succeed
□ TestInvalidTransition: Requested→Approved returns SYS_INVALID_TRANSITION error
□ TestInvalidTransition: Completed→Approved returns error
□ TestInvalidTransition: Rejected→anything returns error (terminal)
□ TestOptimisticLock: concurrent transition on same transfer — one succeeds, one fails
□ TestEventLogging: every transition writes a state_changed event to transfer_events
□ All tests pass: `go test ./internal/orchestrator/... -v`
```

---

### PR 1.2 — Vendor Service client interface + VSS stub (Clean Pass only)

**Can parallel with:** PR 1.1 (different directories) 🔀

**Commits:**
1. `internal/vendorclient/interface.go`: `VendorServiceClient` interface with `Validate` method
2. `internal/vendorclient/http.go`: HTTP implementation that calls VSS stub URL from config
3. `cmd/vendor-stub/main.go`: HTTP server on :8081, loads scenarios.yaml
4. `cmd/vendor-stub/handler.go`: `POST /validate` — route by account code lookup, return scenario response
5. Implement Clean Pass scenario only (ALPHA-001): IQA pass, MICR populated, confidence 0.97, no flags
6. Unit test for scenario routing: ALPHA-001 → clean_pass response

**Verification criteria:**
```
□ VSS stub starts on :8081 and responds 200 on /health
□ POST /validate with account_id for ALPHA-001 → returns clean pass JSON
□ Response includes: iqa_status="pass", micr_data populated, confidence_score=0.97, duplicate_flag=false
□ POST /validate with unknown account_id → returns 404
□ X-Scenario header override works: any account + X-Scenario: clean_pass → clean pass response
□ scenarios.yaml is loaded at startup (not re-read per request)
□ `go test ./cmd/vendor-stub/... -v` passes
```

---

### PR 1.3 — Funding Service (approve path only)

**Can parallel with:** PR 1.1, PR 1.2 (different directory) 🔀

**Commits:**
1. `internal/funding/interface.go`: `FundingServiceClient` interface, `FundingDecision` type (APPROVE/REJECT/FLAG_FOR_REVIEW)
2. `internal/funding/engine.go`: `Evaluate()` implementation
3. `internal/funding/rules.go`: Rule interface, `AccountEligibilityRule`, `DepositLimitRule`, `OmnibusResolutionRule`, `ContributionTypeRule`
4. `internal/store/accounts.go`: GetByID, GetByCode, GetOmnibusForCorrespondent
5. `internal/store/correspondents.go`: GetByID, load rules_config
6. Unit tests: $500 → APPROVE, $5000 exact → APPROVE, $5001 → REJECT, IRA at Alpha → APPROVE with contribution_type=INDIVIDUAL, IRA at Beta → REJECT with FS_ACCOUNT_INELIGIBLE

**Verification criteria:**
```
□ TestApprove_HappyPath: $500, INDIVIDUAL account, Alpha → APPROVE
□ TestApprove_ExactLimit: $5000 → APPROVE (boundary: ≤ not <)
□ TestReject_OverLimit: $5001 → REJECT with code FS_OVER_DEPOSIT_LIMIT
□ TestReject_IneligibleIRA: IRA at Beta → REJECT with code FS_ACCOUNT_INELIGIBLE
□ TestApprove_IRA_Alpha: IRA at Alpha → APPROVE, contribution_type = INDIVIDUAL
□ TestOmnibusResolution: Alpha account → resolves to OMNIBUS-ALPHA
□ TestOmnibusResolution: Beta account → resolves to OMNIBUS-BETA
□ All rules are independently testable (rule interface pattern)
□ `go test ./internal/funding/... -v` passes
```

---

### PR 1.4 — Ledger Service

**Can parallel with:** PR 1.1, PR 1.2, PR 1.3 (different directory) 🔀

**Commits:**
1. `internal/ledger/interface.go`: LedgerService interface per PRD
2. `internal/ledger/service.go`: Implementation — PostDoubleEntry (creates movement_id, writes 2 entries in single tx), GetBalance, GetEntries, Reconcile
3. `internal/store/ledger.go`: DB queries for ledger_entries
4. Unit tests: PostDoubleEntry creates balanced pair, GetBalance returns correct sum, Reconcile returns 0 after multiple postings, multiple movements for same account — balance accumulates correctly

**Verification criteria:**
```
□ TestPostDoubleEntry: creates exactly 2 rows with same movement_id
□ TestPostDoubleEntry: one DEBIT, one CREDIT, same amount
□ TestGetBalance: after posting $500 credit to ALPHA-001, balance = 500
□ TestGetBalance: after posting $500 credit then $500 debit, balance = 0
□ TestReconcile: after 10 random PostDoubleEntry calls, Reconcile() = 0
□ TestReconcile: empty ledger → Reconcile() = 0
□ TestAtomicity: if one entry fails, neither is written (simulated DB error)
□ TestUniqueConstraint: duplicate (transfer_id, entry_type, account_id, side) fails
□ `go test ./internal/ledger/... -v` passes
```

---

### PR 1.5 — Wire it together: deposit endpoint + orchestrator flow

**Depends on:** PR 1.1, PR 1.2, PR 1.3, PR 1.4 all merged

**Commits:**
1. `cmd/api/routes.go`: Register all route handlers
2. `cmd/api/handlers/deposit.go`: `POST /deposits` — validate input, check idempotency, create transfer, start orchestrator flow
3. `cmd/api/handlers/status.go`: `GET /deposits/:id` — return transfer with current state
4. `cmd/api/handlers/ledger.go`: `GET /ledger/balances`, `GET /ledger/entries`, `GET /health/ledger`
5. `internal/orchestrator/flow.go`: `ProcessDeposit()` — Requested → Validating (call VSS) → Analyzing (call FS) → Approved (post ledger) → FundsPosted. Writes transfer_events at each step.
6. `cmd/api/middleware/idempotency.go`: Check `Idempotency-Key` header against DB table, store response on first call
7. Integration test: POST /deposits with ALPHA-001 → verify transfer reaches FundsPosted, ledger has 2 entries, reconcile = 0

**Verification criteria:**
```
□ POST /deposits with valid payload → 201, returns transfer_id, state = "FundsPosted"
□ GET /deposits/:id → returns full transfer record with state, amount, account details
□ GET /ledger/balances → ALPHA-001 has balance 500, OMNIBUS-ALPHA has balance -500
□ GET /health/ledger → { sum: "0.00", healthy: true }
□ GET /deposits/:id/events → returns ordered event list (submitted, vss_called, vss_result, fs_evaluated, ledger_posted, state_changed × N)
□ POST /deposits with same Idempotency-Key → returns same response, no new transfer created
□ POST /deposits with amount 0 → 400 error
□ POST /deposits with invalid account_id → 404 error
□ POST /deposits accepts multipart form with front_image and back_image files
□ Images stored at ./data/images/{transfer_id}/front.jpg and back.jpg
□ GET /deposits/:id/images/front returns stored image (200 + binary)
□ Transfer record has: type='MOVEMENT', memo='FREE', sub_type='DEPOSIT', transfer_type='CHECK', currency='USD'
□ Transfer record has: from_account_id = OMNIBUS for correspondent, account_id = investor account
□ Transfer events table has entries for every step
□ `go test ./cmd/api/... -v` passes
```

---

### PR 1.6 — SSE stream + /admin/flow dashboard

**Can parallel with:** PR 1.5 (backend SSE can be built while frontend consumes mock data) 🔀 (partially — SSE endpoint needs orchestrator wired, but React can be built against mock EventSource)

**Commits:**
1. `internal/events/sse.go`: SSE handler — listens to pg_notify channel, writes to http.ResponseWriter with flushing
2. `internal/orchestrator/notify.go`: `pg_notify('transfer_updates', json)` called on every state transition
3. `cmd/api/handlers/events.go`: `GET /events/stream` — SSE endpoint
4. `web/src/pages/AdminFlow.tsx`: Real-time dashboard — connects to EventSource, renders transfer cards color-coded by state, live event stream log

**Verification criteria:**
```
□ GET /events/stream returns Content-Type: text/event-stream
□ Submitting a deposit causes SSE events to fire for each state transition
□ /admin/flow page connects to SSE and renders incoming events
□ Transfer cards update color based on state (blue=processing, yellow=awaiting, green=money moved, red=terminal)
□ Event stream log shows timestamp, transfer ID, from_state → to_state
□ Opening /admin/flow in two browser tabs — both receive events
□ Closing and reopening /admin/flow — reconnects SSE automatically
```

---

### PR 1.7 — Frontend: /deposit form + /status page + /admin/ledger

**Can parallel with:** PR 1.6 (different React pages) 🔀

**Commits:**
1. `web/src/pages/Deposit.tsx`: Scenario dropdown (fetches from GET /scenarios), amount input, submit button. On success → redirect to /status/:id
2. `web/src/pages/Status.tsx`: Polls GET /deposits/:id every 2s. Shows state in plain English per PRD (Requested → "We received your check", etc.). Shows amount, account, timestamps.
3. `web/src/pages/AdminLedger.tsx`: Fetches GET /ledger/balances, renders balance table. Fetches GET /health/ledger, renders reconciliation indicator (green ✓ $0.00 or red ✗ with amount).
4. `cmd/api/handlers/scenarios.go`: `GET /scenarios` — reads scenarios.yaml, returns JSON
5. Mobile-responsive deposit form: touch targets ≥44px, stacked layout on narrow viewport

**Verification criteria:**
```
□ /deposit shows scenario dropdown with all scenario names from scenarios.yaml
□ Selecting "Clean Pass (ALPHA-001)" pre-fills account field
□ Submitting redirects to /status/:id
□ /status/:id shows current state in plain English
□ /status/:id updates on poll (submit a deposit, watch it progress)
□ /admin/ledger shows all account balances in a table
□ /admin/ledger shows green reconciliation indicator when healthy
□ Mobile viewport: deposit form is full-width, touch targets ≥44px
□ /deposit form has file picker inputs for front and back images
□ Mobile: file picker opens camera (accept="image/*" capture="environment")
□ Desktop viewport: admin ledger has data-dense table layout
```

---

### PR 1.8 — Happy path demo script + tests

**Depends on:** PR 1.5, PR 1.6, PR 1.7 merged

**Commits:**
1. `scripts/demo-happy-path.sh`: curl-based script that submits deposit, polls status, checks ledger
2. Playwright test: happy path E2E — select Clean Pass, enter $500, submit, verify status shows each state, check admin ledger
3. Run all Go unit tests, verify all pass
4. Generate initial test report in `/reports`

**Verification criteria:**
```
□ scripts/demo-happy-path.sh runs end-to-end with all assertions passing
□ Playwright test: submit → status page shows "Funds available" (FundsPosted)
□ Playwright test: /admin/ledger shows correct balances
□ Playwright test: /admin/flow shows transfer in live event stream
□ All Go unit tests pass: `go test ./... -v`
□ All Playwright tests pass: `npx playwright test`
□ /reports/unit-tests.txt exists with test output
□ /reports/playwright/ exists with HTML report
```

---

### TB1 Completion Verification
```
□ All 8 PR verification checklists pass
□ make dev from clean clone → submit deposit → FundsPosted in <5 seconds
□ Ledger reconciliation = 0 after happy path
□ SSE dashboard shows live state transitions
□ Demo script runs without manual intervention
□ Estimated rubric: ~45 pts (partial in 5 of 7 categories)
```

---

## TB2 — VSS Rejection Paths

> **Goal:** 5 more VSS scenarios. Rejection flows work. Typed error messages in UI.
> **Rubric points unlocked:** +15 (stub quality nearly complete), +5 (core correctness — rejection paths)

### PR 2.1 — VSS stub: 4 additional failure scenarios

**Commits:**
1. Add IQA Blur scenario (ALPHA-002): iqa_status=fail, iqa_error_type=blur
2. Add IQA Glare scenario (ALPHA-003): iqa_status=fail, iqa_error_type=glare
3. Add Duplicate Detected scenario (BETA-001): duplicate_flag=true, duplicate_original_tx_id=seeded UUID
4. Add IQA Pass scenario (ALPHA-IRA): clean pass (used for IRA flow in TB3)
5. Unit tests for each new scenario routing

**Verification criteria:**
```
□ POST /validate ALPHA-002 → iqa_status=fail, iqa_error_type=blur
□ POST /validate ALPHA-003 → iqa_status=fail, iqa_error_type=glare
□ POST /validate BETA-001 → duplicate_flag=true with original tx ID
□ POST /validate ALPHA-IRA → clean pass (same as ALPHA-001 but different account)
□ X-Scenario header still overrides account-based routing
□ scenarios.yaml has all 5 scenarios defined (clean_pass + 4 new)
□ `go test ./cmd/vendor-stub/... -v` passes with new tests
```

---

### PR 2.2 — Orchestrator: Validating → Rejected path + error taxonomy

**Depends on:** PR 2.1 merged

**Commits:**
1. `internal/orchestrator/errors.go`: DepositError type with Code, Message, UserMsg, Detail per PRD Section 10
2. Error code constants: VSS_IQA_BLUR, VSS_IQA_GLARE, VSS_DUPLICATE_DETECTED
3. Orchestrator flow: after VSS call, if iqa_status=fail or duplicate_flag=true → transition to Rejected with typed error
4. Store error_code on transfer record
5. Write rejection events to transfer_events
6. Unit tests: VSS blur → Rejected with VSS_IQA_BLUR, VSS duplicate → Rejected with VSS_DUPLICATE_DETECTED

**Verification criteria:**
```
□ POST /deposits with ALPHA-002 → transfer created, state = Rejected, error_code = VSS_IQA_BLUR
□ POST /deposits with ALPHA-003 → Rejected, error_code = VSS_IQA_GLARE
□ POST /deposits with BETA-001 → Rejected, error_code = VSS_DUPLICATE_DETECTED
□ GET /deposits/:id → includes error_code and user-facing error message
□ GET /deposits/:id/events → shows vss_result event with failure details, then state_changed to Rejected
□ Ledger has NO entries for rejected transfers (no money moved)
□ GET /health/ledger still returns 0 (no orphaned entries)
□ `go test ./internal/orchestrator/... -v` passes
```

---

### PR 2.3 — Frontend: error messages + rejection UI

**Can parallel with:** PR 2.2 (frontend work on different files, consumes error response shape) 🔀

**Commits:**
1. `web/src/components/DepositError.tsx`: renders typed error messages per PRD Section 10 (blur → "Hold steady on flat surface", glare → "Avoid direct light", duplicate → "This check has already been deposited")
2. Update /deposit: on submit error, show DepositError component with actionable guidance
3. Update /status/:id: Rejected state shows red indicator with error message
4. Re-submission support: after IQA error, "Try Again" button clears form for new attempt

**Verification criteria:**
```
□ Submit ALPHA-002 → deposit form shows blur-specific error message
□ Submit ALPHA-003 → deposit form shows glare-specific error message
□ Submit BETA-001 → deposit form shows duplicate error with original deposit reference
□ "Try Again" button appears on IQA errors (blur, glare) but NOT on duplicate
□ /status/:id for rejected transfer shows "Deposit not accepted" with reason
□ Error messages match PRD Section 10 exactly
□ No raw error codes shown to investor (only UserMsg)
```

---

### PR 2.4 — Rejection demo script + tests

**Depends on:** PR 2.2, PR 2.3 merged

**Commits:**
1. `scripts/demo-rejection.sh`: exercises blur, glare, duplicate, over-limit paths — each with explicit error code assertion
2. Playwright tests: submit each failure scenario, verify correct error message
3. Update test report

**Verification criteria:**
```
□ scripts/demo-rejection.sh runs with all assertions passing
□ demo-rejection.sh explicitly tests: IQA blur (ALPHA-002), IQA glare (ALPHA-003), duplicate (BETA-001), over-limit $5001 (BETA-002)
□ Each sub-test asserts expected state (Rejected) and expected error code
□ Playwright: ALPHA-002 → blur error message displayed
□ Playwright: ALPHA-003 → glare error message displayed
□ Playwright: BETA-001 → duplicate error displayed
□ All previous tests still pass (regression)
□ Test report updated in /reports
```

---

### TB2 Completion Verification
```
□ All 4 PR verification checklists pass
□ 5 of 7 VSS scenarios functional
□ Rejection paths produce typed errors with user-facing messages
□ No ledger entries created for rejected deposits
□ Estimated rubric: ~60 pts
```

---

## TB3 — Flagged Deposits + Operator Review

> **Goal:** Operator queue works. Flagged deposits reviewable. Full audit trail.
> **Rubric points unlocked:** +10 (operator workflow + observability)

### PR 3.1 — VSS stub: MICR Failure + Amount Mismatch scenarios

**Commits:**
1. Add MICR Read Failure scenario (ALPHA-004): iqa_status=pass, micr_data=null, confidence=0.0
2. Add Amount Mismatch scenario (ALPHA-005): iqa_status=pass, micr_data populated, ocr_amount differs from input amount

**Verification criteria:**
```
□ POST /validate ALPHA-004 → iqa_status=pass, micr_data=null (MICR unreadable)
□ POST /validate ALPHA-005 → iqa_status=pass, ocr_amount=250.00 when input was 500.00
□ All 7 VSS scenarios now implemented
□ Previous scenarios unaffected
```

---

### PR 3.2 — Funding Service: FLAG_FOR_REVIEW path + duplicate detection

**Can parallel with:** PR 3.1 (different directory) 🔀

**Commits:**
1. Funding Service: if VSS returned MICR failure or amount mismatch → FLAG_FOR_REVIEW with review_reason
2. Funding Service: FS-level duplicate detection (query recent transfers by account + amount + time window)
3. Orchestrator: on FLAG_FOR_REVIEW, set transfer.review_reason, stay in Analyzing, write flagged event
4. Unit tests: MICR failure → FLAG_FOR_REVIEW, amount mismatch → FLAG_FOR_REVIEW, FS duplicate → REJECT

**Verification criteria:**
```
□ TestFlagForReview_MICR: MICR failure input → FLAG_FOR_REVIEW, reason = VSS_MICR_READ_FAIL
□ TestFlagForReview_AmountMismatch: mismatched amounts → FLAG_FOR_REVIEW, reason = VSS_AMOUNT_MISMATCH
□ TestReject_FSDuplicate: same account + same amount within 5 min → REJECT with FS_DUPLICATE_DEPOSIT
□ POST /deposits with ALPHA-004 → state = Analyzing, review_reason = VSS_MICR_READ_FAIL
□ POST /deposits with ALPHA-005 → state = Analyzing, review_reason = VSS_AMOUNT_MISMATCH
□ Transfer stays in Analyzing (does NOT advance to Approved)
□ transfer_events includes flagged event with review_reason
```

---

### PR 3.3 — Operator API: queue + actions + auth middleware

**Depends on:** PR 3.2 merged

**Commits:**
1. `cmd/api/middleware/auth.go`: JWT extraction (hardcoded demo tokens), role + correspondent_id → context, RLS session variable setting
2. `cmd/api/handlers/operator.go`: `GET /operator/queue` with filter params (status, min_amount, max_amount, account_id, after, before, sort_by)
3. `cmd/api/handlers/operator.go`: `POST /operator/actions` — validate transfer is in reviewable state, execute action, write transfer_event (operator_action step), transition state
4. Operator approve: Analyzing → Approved → (immediate ledger post) → FundsPosted
5. Operator reject: Analyzing → Rejected, or Validating → Rejected (for stuck VSS transfers)
6. Contribution type override: if provided in action payload, store as contribution_type_override on transfer
7. `GET /deposits/:id/events` endpoint (already wired, verify event chain includes operator action)

**Verification criteria:**
```
□ GET /operator/queue → returns flagged transfers (state=Analyzing AND review_reason IS NOT NULL)
□ GET /operator/queue?min_amount=1000 → filters by amount
□ GET /operator/queue?sort_by=created_at → oldest first
□ POST /operator/actions {action: APPROVE} on flagged transfer → state becomes FundsPosted (Analyzing → Approved → FundsPosted)
□ POST /operator/actions {action: REJECT, reason: "Invalid check"} → state becomes Rejected
□ POST /operator/actions on non-flagged transfer → 400 error
□ POST /operator/actions on FundsPosted transfer → 400 error (can't reject after money moved)
□ GET /deposits/:id/events after approval → includes operator_action event with operator_id, action, reason
□ Contribution type override: POST action with contribution_type_override=ROLLOVER → stored on transfer
□ Auth: request without valid token → 401
□ Auth: operator-alpha token → can only see Alpha transfers (RLS)
□ Auth: apex-admin token → can see all transfers
□ `go test ./cmd/api/... -v` passes
```

---

### PR 3.4 — Operator frontend: /admin/queue

**Can parallel with:** PR 3.3 (frontend can be built against API contract, merge after) 🔀

**Commits:**
1. `web/src/pages/AdminQueue.tsx`: list view with filters (date picker, status dropdown, amount range, account search)
2. `web/src/components/TransferDetail.tsx`: detail panel — check images (placeholder for MVP), MICR data display, confidence score, recognized vs entered amount comparison, decision trace (fetch /deposits/:id/events)
3. `web/src/components/OperatorActions.tsx`: Approve/Reject buttons with mandatory reason text input. Contribution type override dropdown (appears for IRA accounts).
4. Optimistic UI: disable buttons after click, show "already reviewed" if race condition

**Verification criteria:**
```
□ /admin/queue shows flagged transfers with review_reason visible
□ Filter by amount range works
□ Filter by date range works
□ Clicking a transfer opens detail panel
□ Detail panel shows MICR data (or "MICR unreadable" for failure)
□ Detail panel shows confidence score
□ Detail panel for ALPHA-005 shows: entered amount vs recognized amount side by side
□ Detail panel shows decision trace (event list)
□ Approve button → transfer disappears from queue, state updates
□ Reject button requires reason text → transfer disappears from queue
□ IRA transfer shows contribution type override dropdown
□ After approval, /admin/flow shows transfer advancing to FundsPosted
□ After rejection, /status/:id shows "Deposit not accepted" with operator's reason
□ Detail panel displays front and back check images loaded from API
□ Queue list view shows risk indicator badges: "Large deposit" (>$2000), "Low confidence" (<0.90), "MICR unreadable", "Amount discrepancy"
□ Risk badges visible in both list and detail views
```

---

### PR 3.5 — Manual review demo script + tests

**Depends on:** PR 3.3, PR 3.4 merged

**Commits:**
1. `scripts/demo-manual-review.sh`: submit MICR failure → verify in queue → approve → verify FundsPosted
2. Playwright tests: submit ALPHA-004 → appears in queue → operator approves → state advances. Submit ALPHA-005 → appears in queue → operator rejects → verify audit log.
3. Playwright test: IRA contribution type override flow
4. Update test report

**Verification criteria:**
```
□ scripts/demo-manual-review.sh runs end-to-end
□ Playwright: ALPHA-004 submission → visible in operator queue
□ Playwright: operator approves ALPHA-004 → transfer reaches FundsPosted
□ Playwright: operator rejects ALPHA-005 → transfer shows Rejected
□ Playwright: operator approval is logged in decision trace
□ All previous tests still pass (regression)
□ Over-limit test: POST /deposits with BETA-002, amount=$5001 → Rejected by FS (no operator involved)
□ IRA test: POST /deposits with BETA-IRA → Rejected by FS (Beta disallows IRA)
□ IRA test: POST /deposits with ALPHA-IRA → Approved with contribution_type=INDIVIDUAL
```

---

### TB3 Completion Verification
```
□ All 5 PR verification checklists pass
□ All 7 VSS scenarios functional
□ Operator queue: list, filter, detail view, approve, reject all work
□ Audit trail: every operator action logged in transfer_events
□ Contribution type override works for IRA accounts
□ Auth middleware: role gating functional, RLS filters data by correspondent
□ Estimated rubric: ~75 pts
```

---

## TB4 — Settlement

> **Goal:** Settlement file generated. FundsPosted → Completed. EOD cutoff works.
> **Rubric points unlocked:** +10 (settlement integrity, partial core correctness completion)

### PR 4.1 — Settlement Engine + Settlement Bank stub

**Commits:**
1. `internal/settlement/engine.go`: query FundsPosted transfers before cutoff, group by correspondent, generate JSON settlement file per PRD 6.4 structure
2. `internal/settlement/cutoff.go`: EOD cutoff logic — 6:30 PM CT in UTC, strictly less than, business day calendar from config (embedded holiday list)
3. `cmd/settlement-stub/main.go`: complete Settlement Bank stub — POST /settlement/submit returns acknowledgment with batch_id and return_window_expires_at
4. `cmd/api/handlers/settlement.go`: POST /settlement/trigger (manual), GET /settlement/status, GET /settlement/batches
5. Orchestrator: on settlement acknowledgment, transition FundsPosted → Completed, set settled_at
6. Settlement file saved to `./data/settlement/` (local filesystem for MVP)
7. Unit tests: cutoff edge case (exactly 6:30 PM → next day), file contains no Rejected transfers, totals are correct

**Verification criteria:**
```
□ POST /settlement/trigger → generates settlement file, returns batch summary
□ Settlement file JSON structure matches PRD 6.4 (file_header, cash_letter, bundles, checks, totals)
□ Settlement file contains ONLY FundsPosted transfers (no Rejected, no already-Completed)
□ Settlement file totals match sum of included transfer amounts
□ Settlement file record_count matches number of included transfers
□ POST /settlement/submit on stub → returns ACKNOWLEDGED with batch_id
□ After acknowledgment: transfers are in Completed state, settled_at is set
□ GET /settlement/status → shows current batch status and any unbatched transfers
□ GET /settlement/batches → lists batches with status
□ TestCutoff_Exactly630: deposit at 18:30:00 CT → next business day
□ TestCutoff_Before630: deposit at 18:29:59 CT → today's batch
□ TestCutoff_Weekend: Saturday deposit → Monday batch (skips Sunday)
□ TestCutoff_Holiday: deposit on federal holiday → next business day
□ Settlement batch record created in settlement_batches table with correct status progression
□ `go test ./internal/settlement/... -v` passes
```

---

### PR 4.2 — Settlement frontend + health monitoring

**Can parallel with:** PR 4.1 (different layer) 🔀

**Commits:**
1. Update `/admin/flow`: add settlement status indicator — "Today's batch: N deposits ready / batch generated at HH:MM / ⚠ N unbatched transfers from yesterday"
2. `cmd/api/handlers/health.go`: GET /health/settlement — checks for unbatched FundsPosted transfers past last cutoff
3. Settlement trigger button on /admin/flow for demo use

**Verification criteria:**
```
□ /admin/flow shows settlement status indicator
□ Before settlement trigger: shows "N deposits ready"
□ After settlement trigger: shows "Batch generated at HH:MM, N deposits, ACKNOWLEDGED"
□ If unbatched transfers exist past cutoff: shows warning indicator
□ GET /health/settlement returns { healthy: true/false, unbatched_count: N }
□ Settlement trigger button works from admin UI
```

---

### PR 4.3 — Settlement demo script + tests

**Depends on:** PR 4.1, PR 4.2 merged

**Commits:**
1. Extend `scripts/demo-happy-path.sh`: after FundsPosted, trigger settlement, verify Completed
2. Playwright test: full happy path including settlement trigger → Completed state
3. Playwright test: verify settlement file contents via API
4. Update test report

**Verification criteria:**
```
□ demo-happy-path.sh now runs: submit → FundsPosted → trigger settlement → Completed
□ Playwright: full happy path reaches Completed state
□ Playwright: /admin/flow shows Completed with settlement details
□ Playwright: /admin/ledger still shows reconciliation = 0 after settlement
□ All previous tests still pass
```

---

### TB4 Completion Verification
```
□ All 3 PR verification checklists pass
□ Happy path reaches Completed: Requested → ... → FundsPosted → Completed
□ Settlement file is generated with correct structure and contents
□ EOD cutoff logic works (tested at boundary)
□ Settlement Bank stub acknowledges batches
□ Settlement monitoring on admin dashboard
□ Estimated rubric: ~85 pts
```

---

## TB5 — Returns + Reversals

> **Goal:** Return notifications process correctly. Reversal + fee ledger entries. Completed → Returned.
> **Rubric points unlocked:** +10 (return/reversal handling)

### PR 5.1 — Return Handler + Settlement Bank stub webhook

**Commits:**
1. `internal/returns/handler.go`: accept return notification, validate state (FundsPosted or Completed), load fee from correspondent rules_config, post 4 ledger entries atomically, transition to Returned, create notification, write events
2. `cmd/api/handlers/returns.go`: POST /returns with bearer token validation
3. `cmd/settlement-stub/handler.go`: POST /settlement/return — takes transfer_id and reason_code, calls POST /returns on main API (webhook callback pattern)
4. `cmd/api/middleware/settlement_auth.go`: bearer token validation for /returns endpoint
5. Negative balance handling: if investor would go negative, post entries anyway, set account.status = COLLECTIONS
6. Return idempotency: transfer already Returned → return 200, no-op
7. Unit tests: return from FundsPosted (4 entries), return from Completed (4 entries), fee = config value, negative balance handling, idempotent re-return

**Verification criteria:**
```
□ POST /returns with valid token + FundsPosted transfer → Returned, 4 ledger entries created
□ POST /returns with valid token + Completed transfer → Returned, 4 ledger entries created
□ Reversal entries: DEBIT investor (original amount), CREDIT omnibus (original amount)
□ Fee entries: DEBIT investor ($30 from config), CREDIT FEE-APEX ($30)
□ All 4 entries share appropriate movement_ids (2 for reversal, 2 for fee)
□ GET /health/ledger after return → still 0 (6 total entries for this transfer sum to 0)
□ Investor balance after $500 deposit + return: -30 (500 - 500 - 30)
□ Reversal amount = original deposit amount (NOT deposit minus fee)
□ Fee is a SEPARATE ledger entry pair, not subtracted from reversal
□ If balance goes negative: account.status = COLLECTIONS
□ POST /returns without valid token → 401
□ POST /returns for Rejected transfer → 400 error (no money to reverse)
□ POST /returns for already-Returned transfer → 200, no new entries (idempotent)
□ transfer_events includes return_received and return_processed events
□ Notification created with type=RETURN_RECEIVED and plain-English message
□ `go test ./internal/returns/... -v` passes
```

---

### PR 5.2 — Notifications API + frontend: return flow

**Can parallel with:** PR 5.1 (frontend layer) 🔀 (partially — needs return endpoint working for full E2E, but can build UI components against mock data)

**Commits:**
1. `cmd/api/handlers/notifications.go`: GET /notifications (filtered by auth context), PATCH /notifications/:id/read
2. Update `/status/:id`: Returned state shows red indicator with return reason in plain English, fee amount, and guidance
3. Update `/admin/flow`: "Simulate Return" button that calls Settlement Bank stub's POST /settlement/return
4. `web/src/components/NotificationBell.tsx`: unread count badge, notification list dropdown
5. SSE: return events pushed to dashboard

**Verification criteria:**
```
□ GET /notifications → returns notifications for authenticated user's account
□ After return: notification exists with type=RETURN_RECEIVED, message includes reason and fee
□ PATCH /notifications/:id/read → sets read_at
□ /status/:id for Returned transfer → shows "Deposit reversed", return reason, "$30 returned check fee"
□ /admin/flow "Simulate Return" button → triggers return flow, dashboard updates live
□ Notification bell shows unread count
□ Notification bell dropdown shows recent notifications
□ SSE stream includes return events
```

---

### PR 5.3 — Return demo script + tests

**Depends on:** PR 5.1, PR 5.2 merged

**Commits:**
1. `scripts/demo-return.sh`: happy path through Completed, then trigger return, verify Returned + ledger
2. Playwright test: full return flow — submit → Completed → trigger return → Returned, check ledger has 6 entries, reconciliation = 0, fee = $30
3. Playwright test: return from FundsPosted (pre-settlement return)
4. Update test report

**Verification criteria:**
```
□ scripts/demo-return.sh runs end-to-end
□ Playwright: submit → settle → return → /status/:id shows "Deposit reversed"
□ Playwright: /admin/ledger shows 6 entries for the transfer, reconciliation = 0
□ Playwright: /admin/flow shows Returned state in dashboard
□ Playwright: notification appears for investor
□ All previous tests still pass
□ Estimated rubric: ~95 pts (only DX polish remaining)
```

---

### TB5 Completion Verification
```
□ All 3 PR verification checklists pass
□ Return from FundsPosted works
□ Return from Completed works (KEY demo moment — Completed is NOT terminal)
□ 4 reversal entries + correct fee from config
□ Negative balance handling works (doesn't block return)
□ Investor notification created and displayed
□ Ledger reconciliation still 0 after return
□ Estimated rubric: ~95 pts
```

---

## TB6 — Observability Polish + DX

> **Goal:** Structured logging, redaction, docs, test report. Ship-ready.
> **Rubric points unlocked:** +5 (remaining DX + observability polish → 100)

### PR 6.1 — Structured logging + redaction

**Commits:**
1. `internal/logging/logger.go`: slog-based structured logger, middleware that injects correspondent_id
2. `internal/logging/redact.go`: redact() helper — "021000021" → "*****0021", applied to routing, account, MICR in all log output
3. Audit all existing log calls — add correspondent context, redact sensitive fields
4. Unit test: redact function handles various input lengths correctly

**Verification criteria:**
```
□ Every API request log includes correspondent_id
□ No raw routing numbers in log output (grep for 9-digit strings)
□ No raw account numbers in log output
□ Redacted format: last 4 digits visible, rest masked
□ Log output is structured JSON (parseable by log aggregators)
□ `go test ./internal/logging/... -v` passes
```

---

### PR 6.2 — Documentation + test report generation

**Can parallel with:** PR 6.1 🔀

**Commits:**
1. `README.md`: setup (make dev), architecture overview, API endpoints, demo instructions (all 4 scripts), scenario list, how to run tests
2. `/docs/decision_log.md`: all decisions from PRD Section 15, expanded with alternatives and rationale
3. `/docs/architecture.md`: system diagram (Mermaid), data flow, service boundaries
4. Short write-up (≤1 page): architecture choices, stub design, state machine rationale, risks/limitations
5. `SUBMISSION.md` per spec format
6. `scripts/generate-report.sh`: runs all tests, captures output, generates `/reports/TEST_REPORT.md` summary
7. Risks/limitations note

**Verification criteria:**
```
□ README: a new developer can run `make dev` and submit a deposit by following instructions
□ README: all 4 demo scripts documented with expected output
□ /docs/decision_log.md covers all 12 decisions from PRD
□ /docs/architecture.md includes Mermaid diagram that renders
□ Short write-up is ≤1 page
□ SUBMISSION.md follows spec's Common Submission Format exactly
□ scripts/generate-report.sh produces /reports/TEST_REPORT.md
□ /reports includes: TEST_REPORT.md, unit-tests.txt, playwright/ HTML report
□ Risks/limitations note exists and makes no compliance claims
□ .env.example is complete and documented
```

---

### PR 6.3 — Final regression + polish

**Depends on:** PR 6.1, PR 6.2 merged

**Commits:**
1. Run full test suite — fix any regressions
2. Run all 4 demo scripts — verify all pass
3. Run `scripts/generate-report.sh` — final report
4. Verify `make dev` from clean clone works
5. Verify all deliverables per spec Section "Deliverables" checklist

**Verification criteria (FINAL — this is the submission gate):**
```
□ `make dev` from clean clone: all services start within 30 seconds
□ All Go unit tests pass (14+)
□ All Playwright E2E tests pass (11+)
□ No Playwright test exceeds 30 second timeout
□ demo-happy-path.sh completes full lifecycle in <30 seconds
□ scripts/demo-happy-path.sh passes
□ scripts/demo-rejection.sh passes
□ scripts/demo-manual-review.sh passes
□ scripts/demo-return.sh passes
□ /reports/TEST_REPORT.md exists with passing results
□ GET /health → 200
□ GET /health/ledger → { healthy: true }
□ GET /health/settlement → { healthy: true } (no unbatched transfers)
□ All 7 VSS scenarios exercisable from UI
□ Operator queue functional with filters
□ Ledger reconciliation = 0 after all demo scenarios
□ README is complete and accurate
□ /docs/decision_log.md exists
□ /docs/architecture.md exists with diagram
□ SUBMISSION.md exists
□ .env.example exists
□ No raw PII in logs (grep check)

DELIVERABLES CHECKLIST (from spec):
□ README.md
□ /docs/decision_log.md
□ /docs/architecture.md
□ /tests (unit + e2e)
□ /reports (test results + coverage)
□ .env.example
□ Vendor Service stub with documented scenarios
□ Demo scripts (4 scripts, all paths)
□ Short write-up ≤1 page
```

---

### TB6 Completion Verification = MILESTONE 1 COMPLETE
```
□ All verification criteria in PR 6.3 pass
□ Estimated rubric: 100/100
□ System is demo-ready
□ All spec deliverables present
□ Ready for Milestone 2 (hardening + infra signals)
```

---

## Summary: PR Count + Parallelization Map

| Epic | PRs | Sequential | Parallelizable |
|------|-----|-----------|----------------|
| Epic 0 | 4 | 0.1 → 0.2 → 0.3 | 0.4 🔀 with 0.2/0.3 |
| Epic 1 | 2 | 1.1 → 1.2 | Entire epic 🔀 with TB1 |
| TB1 | 8 | 1.5 depends on 1.1-1.4; 1.8 depends on 1.5-1.7 | 1.1, 1.2, 1.3, 1.4 🔀 all parallel; 1.6, 1.7 🔀 partial |
| TB2 | 4 | 2.2 depends on 2.1; 2.4 depends on 2.2+2.3 | 2.3 🔀 with 2.2 |
| TB3 | 5 | 3.3 depends on 3.2; 3.5 depends on 3.3+3.4 | 3.1, 3.2 🔀; 3.4 🔀 with 3.3 |
| TB4 | 3 | 4.3 depends on 4.1+4.2 | 4.1, 4.2 🔀 |
| TB5 | 3 | 5.3 depends on 5.1+5.2 | 5.1, 5.2 🔀 partial |
| TB6 | 3 | 6.3 depends on 6.1+6.2 | 6.1, 6.2 🔀 |

**Total: 32 PRs. Maximum parallelism: 4 concurrent agents in TB1 (PRs 1.1-1.4).**
