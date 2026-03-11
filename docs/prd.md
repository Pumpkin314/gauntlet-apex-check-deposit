# Apex Mobile Check Deposit — Product Requirements Document

> **Status:** DRAFT — Milestone 1 (MVP)
> **Source of truth:** `Apex_Fintech_Services_-_Challenger_Projects.md`
> **Target score:** 100/100 on evaluation rubric

---

## 1. Project Summary

A minimal end-to-end mobile check deposit system. Investors submit checks via a mobile-responsive web app. The system validates images via a stubbed Vendor Service, enforces business rules via a Funding Service, posts double-entry ledger records, generates settlement files at EOD, and handles return/reversal scenarios with fee processing. An operator review workflow handles flagged deposits with full audit logging.

### Non-Goals (MVP)

- Real cloud deployment (Pulumi/GCP is Epic 1 but not required for the 100-point score)
- Real authentication (lightweight JWT with demo tokens; GCP Identity Platform is Milestone 2)
- Real encryption (synthetic data; pgcrypto hooks are Milestone 2)
- gRPC (REST everywhere; interfaces designed for gRPC extraction in Milestone 2)
- Redis (Postgres-only idempotency; Redis fast-path cache is Milestone 2)
- On-device IQA (scenario selector is the simulation)
- PWA offline mode / service workers
- Email notifications

---

## 2. Architecture Overview

### Service Boundaries

```
┌─────────────────────────────────────────────────────────────────────┐
│                        React PWA (Vite + TS)                        │
│  /deposit (mobile)  /status/:id  /admin/flow  /admin/queue  /admin/ledger │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ REST (JSON)
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         Go API Server                               │
│                                                                     │
│  ┌──────────────┐  ┌───────────────┐  ┌────────────────┐           │
│  │  Orchestrator │  │ Funding Svc   │  │ Ledger Svc     │           │
│  │  (state mach) │─▶│ (rule engine) │  │ (double-entry) │           │
│  └──────┬───────┘  └───────────────┘  └────────────────┘           │
│         │          ┌───────────────┐  ┌────────────────┐           │
│         │          │ Settlement    │  │ Return Handler  │           │
│         │          │ Engine        │  │                 │           │
│         │          └───────────────┘  └────────────────┘           │
│         │                                                           │
│  ┌──────▼──────────────────────────────────────────────────┐       │
│  │              Postgres (Cloud SQL or local)               │       │
│  │  schemas: core / ledger / audit                          │       │
│  └──────────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────┘
         │ HTTP                              │ HTTP
         ▼                                   ▼
┌─────────────────┐                ┌──────────────────────┐
│  Vendor Service  │                │  Settlement Bank     │
│  Stub (VSS)      │                │  Stub                │
│  :8081           │                │  :8082               │
│  scenarios.yaml  │                │  webhook callback    │
└─────────────────┘                └──────────────────────┘
```

### Tech Stack

| Layer | Choice | Justification |
|-------|--------|---------------|
| Language | Go | Goroutines, single binary, explicit error handling, Apex Ascend alignment |
| Database | Postgres | ACID, RLS, LISTEN/NOTIFY, production-appropriate |
| Frontend | React + TypeScript + Vite | Mobile-responsive, single app for investor + admin |
| Real-time | SSE (Server-Sent Events) | Server→client push for dashboard; simpler than WebSocket |
| Migrations | goose | Single-file SQL conventions, simpler than golang-migrate |
| Testing | Go `testing` + Playwright | Unit tests (fast, logic) + E2E tests (browser, integration) |
| Containers | Docker Compose | One-command local setup: `make dev` |
| IaC | Pulumi (Go) on GCP | Epic 1; two stacks (dev/prod) |

### Internal Service Design

All services are internal Go packages in MVP, called as function calls within the API process. However, each is defined behind a Go interface:

```go
type VendorServiceClient interface {
    Validate(ctx context.Context, req *ValidateRequest) (*ValidateResponse, error)
}

type FundingServiceClient interface {
    Evaluate(ctx context.Context, req *EvaluateRequest) (*FundingDecision, error)
}

type LedgerService interface {
    PostDoubleEntry(ctx context.Context, debit, credit AccountID, amount Amount, memo string, transferID uuid.UUID) error
    GetBalance(ctx context.Context, accountID AccountID) (Amount, error)
    GetEntries(ctx context.Context, transferID uuid.UUID) ([]LedgerEntry, error)
    Reconcile(ctx context.Context) (Amount, error)
}
```

These interfaces match what gRPC service definitions would produce. Milestone 2 extracts them to gRPC services with zero caller changes. Decision log entry: "REST for MVP, interfaces designed for gRPC extraction. Production: gRPC for internal service mesh, REST for external-facing APIs."

---

## 3. Transfer State Machine

### States (from spec)

| State | Description | Money Moved? |
|-------|-------------|:------------:|
| Requested | Deposit submitted by investor | No |
| Validating | Sent to Vendor Service for IQA/MICR/OCR | No |
| Analyzing | Business rules being applied by Funding Service | No |
| Approved | Passed all checks; awaiting ledger posting | No |
| FundsPosted | Provisional credit posted to investor account | **Yes** |
| Completed | Settlement confirmed by Settlement Bank | Yes |
| Rejected | Failed validation, business rules, or operator review | No |
| Returned | Check bounced after settlement; reversal posted | **Yes (reversal)** |

### Valid Transitions

```
Requested      → Validating                (Orchestrator starts VSS call)
Validating     → Analyzing                 (VSS returns success/flag → Orchestrator calls Funding Service)
Validating     → Rejected                  (VSS returns IQA fail or duplicate detected)
Analyzing      → Approved                  (Funding Service approves, or operator approves flagged)
Analyzing      → Rejected                  (Funding Service rejects, or operator rejects flagged)
Approved       → FundsPosted               (LedgerService posts debit/credit entries)
FundsPosted    → Completed                 (Settlement Bank acknowledges batch)
FundsPosted    → Returned                  (Return notification arrives pre-settlement)
Completed      → Returned                  (Return notification arrives post-settlement — KEY: Completed is NOT terminal)
```

Operator manual rejection is valid from: Validating, Analyzing, Approved (all pre-money states). NOT from FundsPosted or Completed — post-money corrections go through the return/reversal path.

### Implementation

Hand-rolled transition table. No library (looplab/fsm adds complexity without benefit for 8 static states).

```go
var validTransitions = map[TransferState][]TransferState{
    Requested:   {Validating},
    Validating:  {Analyzing, Rejected},
    Analyzing:   {Approved, Rejected},
    Approved:    {FundsPosted},
    FundsPosted: {Completed, Returned},
    Completed:   {Returned},
}
```

Optimistic locking on every transition:

```sql
UPDATE transfers SET state = $2, updated_at = NOW()
WHERE id = $1 AND state = $3
RETURNING id;
-- Zero rows = concurrent transition already happened → abort with SYS_INVALID_TRANSITION
```

### Terminal States

`Rejected`, `Returned`.

### Milestone 2 Addition

`Completed_Finalized` — return window expires with no return. Background job checks `WHERE state = 'Completed' AND return_window_expires_at < NOW()`. True terminal happy-path state. Demonstrates clearing operations knowledge.

---

## 4. Data Model

### 4.1 `correspondents` table

```sql
CREATE TABLE correspondents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL,           -- 'ALPHA', 'BETA'
    name TEXT NOT NULL,                  -- 'Alpha Brokerage', 'Beta Wealth'
    rules_config JSONB NOT NULL,         -- per-correspondent business rules
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**`rules_config` JSONB structure:**

```json
{
  "deposit_limit": 5000,
  "ineligible_account_types": [],
  "contribution_cap": 7000,
  "fees": {
    "returned_check": 30.00,
    "currency": "USD"
  }
}
```

Note: `fees.returned_check` is configurable per correspondent. The spec's "$30 hard-coded for MVP" means the seed data sets it to 30, not that the code uses a magic number.

---

### 4.2 `accounts` table

```sql
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL,            -- 'ALPHA-001', 'OMNIBUS-ALPHA', 'FEE-APEX'
    correspondent_id UUID REFERENCES correspondents(id),  -- NULL for FEE account
    type TEXT NOT NULL,                   -- 'INDIVIDUAL', 'IRA', 'OMNIBUS', 'FEE'
    status TEXT NOT NULL DEFAULT 'ACTIVE', -- 'ACTIVE', 'SUSPENDED', 'COLLECTIONS'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Seed accounts:**

| Code | Correspondent | Type | Scenario Purpose |
|------|---------------|------|------------------|
| ALPHA-001 | Alpha | INDIVIDUAL | Happy path / Clean Pass |
| ALPHA-002 | Alpha | INDIVIDUAL | IQA Fail (Blur) |
| ALPHA-003 | Alpha | INDIVIDUAL | IQA Fail (Glare) |
| ALPHA-004 | Alpha | INDIVIDUAL | MICR Read Failure → operator review |
| ALPHA-005 | Alpha | INDIVIDUAL | Amount Mismatch → operator review |
| ALPHA-IRA | Alpha | IRA | Clean pass + contribution type defaulting |
| BETA-001 | Beta | INDIVIDUAL | Duplicate Detected |
| BETA-002 | Beta | INDIVIDUAL | Over $5,000 limit (FS rejection) |
| BETA-IRA | Beta | IRA | FS rejection (Beta disallows IRA check deposits) |
| OMNIBUS-ALPHA | Alpha | OMNIBUS | Alpha's pooled custodial account |
| OMNIBUS-BETA | Beta | OMNIBUS | Beta's pooled custodial account |
| FEE-APEX | — | FEE | Apex fee revenue destination |

---

### 4.3 `transfers` table

This carries the spec's required ledger attributes directly on the transfer record. The transfer is the business intent; ledger entries are the accounting execution.

```sql
CREATE TABLE transfers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Spec-required ledger attributes
    account_id UUID NOT NULL REFERENCES accounts(id),       -- investor account (spec: "To AccountId")
    from_account_id UUID NOT NULL REFERENCES accounts(id),  -- omnibus account (spec: "From AccountId")
    amount NUMERIC(12,2) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    type TEXT NOT NULL DEFAULT 'MOVEMENT',
    sub_type TEXT NOT NULL DEFAULT 'DEPOSIT',
    transfer_type TEXT NOT NULL DEFAULT 'CHECK',
    memo TEXT NOT NULL DEFAULT 'FREE',

    -- State machine
    state TEXT NOT NULL DEFAULT 'Requested',
    review_reason TEXT,                 -- NULL = not flagged; set by FS or VSS on flag
    error_code TEXT,                    -- NULL = no error; set on rejection or failure
    contribution_type TEXT,             -- NULL for non-retirement; 'INDIVIDUAL', 'ROLLOVER', 'EMPLOYER'
    contribution_type_override TEXT,    -- set by operator if overriding the default

    -- Vendor Service results
    vendor_transaction_id TEXT,         -- from VSS response; used for dedup constraint
    confidence_score NUMERIC(4,2),      -- from VSS response; displayed in operator queue
    micr_data JSONB,                    -- { routing, account, check_number } from VSS

    -- Images
    front_image_ref TEXT,               -- file path or GCS path
    back_image_ref TEXT,

    -- Settlement
    settlement_batch_id UUID,           -- set when Settlement Engine picks up the transfer
    settled_at TIMESTAMPTZ,             -- set when Settlement Bank acknowledges

    -- Correspondent (denormalized for RLS)
    correspondent_id UUID NOT NULL REFERENCES correspondents(id),

    -- Timestamps
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- when investor submitted (used for EOD cutoff)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT valid_state CHECK (state IN (
        'Requested', 'Validating', 'Analyzing', 'Approved',
        'FundsPosted', 'Completed', 'Rejected', 'Returned'
    )),
    CONSTRAINT valid_amount CHECK (amount > 0),
    CONSTRAINT unique_vendor_tx UNIQUE (account_id, vendor_transaction_id)
);
```

---

### 4.4 `ledger_entries` table (true double-entry)

```sql
CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    movement_id UUID NOT NULL,          -- groups debit/credit pair
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    account_id UUID NOT NULL REFERENCES accounts(id),
    side TEXT NOT NULL,                  -- 'DEBIT' or 'CREDIT'
    amount NUMERIC(12,2) NOT NULL,
    entry_type TEXT NOT NULL,            -- 'PROVISIONAL_CREDIT', 'REVERSAL', 'FEE'
    memo TEXT NOT NULL,                  -- 'FREE', 'REVERSAL', 'RETURNED_CHECK_FEE'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_side CHECK (side IN ('DEBIT', 'CREDIT')),
    CONSTRAINT valid_amount CHECK (amount > 0),
    CONSTRAINT unique_entry_per_type UNIQUE (transfer_id, entry_type, account_id, side)
);
```

**Reconciliation query:**

```sql
SELECT SUM(CASE WHEN side = 'CREDIT' THEN amount ELSE -amount END) AS balance
FROM ledger_entries;
-- Must always return 0. Non-zero = compliance violation.
```

**Balance per account:**

```sql
SELECT SUM(CASE WHEN side = 'CREDIT' THEN amount ELSE -amount END) AS balance
FROM ledger_entries
WHERE account_id = $1;
```

**Deposit posting (Approved → FundsPosted):**

| movement_id | account_id | side | amount | entry_type | memo |
|-------------|-----------|------|--------|------------|------|
| mov-1 | OMNIBUS-ALPHA | DEBIT | 500.00 | PROVISIONAL_CREDIT | FREE |
| mov-1 | ALPHA-001 | CREDIT | 500.00 | PROVISIONAL_CREDIT | FREE |

**Return reversal (→ Returned):**

| movement_id | account_id | side | amount | entry_type | memo |
|-------------|-----------|------|--------|------------|------|
| mov-2 | ALPHA-001 | DEBIT | 500.00 | REVERSAL | REVERSAL |
| mov-2 | OMNIBUS-ALPHA | CREDIT | 500.00 | REVERSAL | REVERSAL |
| mov-3 | ALPHA-001 | DEBIT | 30.00 | FEE | RETURNED_CHECK_FEE |
| mov-3 | FEE-APEX | CREDIT | 30.00 | FEE | RETURNED_CHECK_FEE |

All 4 return entries are posted atomically in a single DB transaction.

---

### 4.5 `transfer_events` table (decision trace)

```sql
CREATE TABLE transfer_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    step TEXT NOT NULL,
    actor TEXT,                          -- 'system', 'vss', 'funding_service', operator ID
    data JSONB,                         -- step-specific payload
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_transfer_events_transfer ON transfer_events(transfer_id);
```

**Event types written throughout lifecycle:**

| Step | When | Data |
|------|------|------|
| `submitted` | Deposit created | `{ amount, account_code, idempotency_key }` |
| `vss_called` | Orchestrator calls VSS | `{ request_payload }` |
| `vss_result` | VSS responds | `{ iqa_status, confidence_score, micr_data, error_type }` |
| `fs_evaluated` | Funding Service returns | `{ decision, reason_code, rules_applied }` |
| `flagged` | Transfer flagged for review | `{ review_reason }` |
| `operator_action` | Operator acts | `{ operator_id, action, reason, overrides }` |
| `ledger_posted` | Entries created | `{ movement_id, debit_account, credit_account, amount }` |
| `settlement_batched` | Added to batch | `{ batch_id, cutoff_date }` |
| `settlement_confirmed` | Bank acknowledges | `{ acknowledged_at }` |
| `return_received` | Return notification | `{ reason_code, amount }` |
| `return_processed` | Reversal posted | `{ reversal_movement_id, fee_movement_id }` |
| `state_changed` | Every transition | `{ from_state, to_state, trigger }` |

This table satisfies the spec's "Per-deposit decision trace" requirement and subsumes operator audit logging. The `GET /transfers/:id/events` endpoint returns the full chain. The operator detail view in the queue displays it inline.

---

### 4.6 `idempotency_keys` table

```sql
CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    response_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Client sends `Idempotency-Key` header. API checks this table before creating a transfer. If key exists, returns the stored response. Permanent record (no TTL — Postgres is the authoritative store). Milestone 2 adds Redis as a fast-path TTL cache in front of this table.

---

### 4.7 `settlement_batches` table

```sql
CREATE TABLE settlement_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    correspondent_id UUID NOT NULL REFERENCES correspondents(id),
    cutoff_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',  -- 'PENDING', 'SUBMITTED', 'ACKNOWLEDGED'
    file_ref TEXT,                           -- path to generated settlement file
    record_count INT,
    total_amount NUMERIC(14,2),
    submitted_at TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_status CHECK (status IN ('PENDING', 'SUBMITTED', 'ACKNOWLEDGED'))
);
```

---

### 4.8 `notifications` table

```sql
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    transfer_id UUID REFERENCES transfers(id),
    type TEXT NOT NULL,                 -- 'DEPOSIT_CONFIRMED', 'DEPOSIT_REJECTED', 'RETURN_RECEIVED'
    message TEXT NOT NULL,              -- plain-English investor-facing message
    read_at TIMESTAMPTZ,               -- NULL = unread
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Delivered via SSE channel to the frontend. No email for MVP.

---

### 4.9 Row-Level Security

Applied to: `transfers`, `ledger_entries`, `transfer_events`, `accounts`, `settlement_batches`, `notifications`.

```sql
ALTER TABLE transfers ENABLE ROW LEVEL SECURITY;
CREATE POLICY correspondent_isolation ON transfers
    USING (
        correspondent_id = current_setting('app.correspondent_id')::uuid
        OR current_setting('app.role') = 'apex_admin'
    );
```

Session variable set by auth middleware:

```go
func setRLSContext(ctx context.Context, db *sql.DB, role, correspondentID string) error {
    _, err := db.ExecContext(ctx,
        "SELECT set_config('app.correspondent_id', $1, true), set_config('app.role', $2, true)",
        correspondentID, role)
    return err
}
```

Fee account (`FEE-APEX`) has `correspondent_id = NULL`. RLS policy condition handles this: visible only to `apex_admin` role.

---

## 5. API Endpoints

### Deposit Submission

| Method | Path | Description |
|--------|------|-------------|
| POST | `/deposits` | Submit a new deposit. Requires `Idempotency-Key` header. |
| GET | `/deposits/:id` | Get transfer status and details |
| GET | `/deposits/:id/events` | Get decision trace (transfer_events) |

### Operator Workflow

| Method | Path | Description |
|--------|------|-------------|
| GET | `/operator/queue` | Flagged transfers. Query params: `status`, `min_amount`, `max_amount`, `account_id`, `after`, `before`, `sort_by` |
| POST | `/operator/actions` | Submit operator decision: `{ transfer_id, action: APPROVE\|REJECT, reason, contribution_type_override? }` |

### Settlement

| Method | Path | Description |
|--------|------|-------------|
| POST | `/settlement/trigger` | Manual trigger for demo (calls same function as EOD cron) |
| GET | `/settlement/status` | Current batch window status + any unbatched transfers |
| GET | `/settlement/batches` | List of settlement batches with status |

### Returns (webhook — called by Settlement Bank stub)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/returns` | Accept return notification. Bearer token auth. `{ transfer_id, return_reason_code, amount }` |

### Ledger

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ledger/balances` | All account balances |
| GET | `/ledger/entries` | Filtered entries. Query params: `account_id`, `transfer_id`, `after`, `before` |
| GET | `/health/ledger` | Reconciliation check. Returns `{ sum: "0.00", healthy: true }` |

### Notifications

| Method | Path | Description |
|--------|------|-------------|
| GET | `/notifications` | Investor's notifications (filtered by auth context) |
| PATCH | `/notifications/:id/read` | Mark as read |

### Scenarios (demo support)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/scenarios` | Returns parsed scenarios.yaml as JSON for demo UI dropdown |

### SSE Stream

| Method | Path | Description |
|--------|------|-------------|
| GET | `/events/stream` | SSE stream of state transitions for the real-time admin dashboard |

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Basic health check |
| GET | `/health/ledger` | Reconciliation sum (see above) |
| GET | `/health/settlement` | Unbatched FundsPosted transfers past last cutoff |

---

## 6. Component Specifications

### 6.1 Vendor Service Stub (VSS)

Separate HTTP server on :8081. Reads `scenarios.yaml` on startup.

**Routing logic:** Request includes `account_id`. VSS looks up the account code, matches to scenario in `scenarios.yaml`. `X-Scenario` header overrides for ad-hoc testing.

**Endpoint:** `POST /validate`

**Request:**
```json
{
  "account_id": "uuid",
  "amount": 500.00,
  "front_image": "base64_or_ref",
  "back_image": "base64_or_ref"
}
```

**Response:**
```json
{
  "iqa_status": "pass",
  "iqa_error_type": null,
  "micr_data": { "routing": "021000021", "account": "123456789", "check_number": "1001" },
  "ocr_amount": 500.00,
  "confidence_score": 0.97,
  "duplicate_flag": false,
  "duplicate_original_tx_id": null,
  "transaction_id": "vss-uuid",
  "scenario_used": "clean_pass"
}
```

**7 required scenarios:**

| Scenario | Trigger Account | IQA | MICR | Duplicate | Amount Match | Routing |
|----------|----------------|-----|------|-----------|--------------|---------|
| Clean Pass | ALPHA-001 | pass | success | no | yes | → Analyzing (FS) |
| IQA Fail Blur | ALPHA-002 | fail (blur) | — | — | — | → Rejected |
| IQA Fail Glare | ALPHA-003 | fail (glare) | — | — | — | → Rejected |
| MICR Failure | ALPHA-004 | pass | fail | no | yes | → Analyzing (flagged) |
| Amount Mismatch | ALPHA-005 | pass | success | no | no | → Analyzing (flagged) |
| Duplicate | BETA-001 | pass | success | yes | yes | → Rejected |
| IQA Pass | ALPHA-IRA | pass | success | no | yes | → Analyzing (FS) |

### 6.2 Funding Service

Stateless rule engine. Does NOT write to DB.

**Input:** Transfer record, account record, correspondent's `rules_config`.

**Output:** `{ decision: APPROVE | REJECT | FLAG_FOR_REVIEW, reason_code, resolved_omnibus_id, contribution_type }`

**Rules evaluated (in order):**

1. Account active and eligible (`rules_config.ineligible_account_types`)
2. Amount ≤ `rules_config.deposit_limit` (default 5000)
3. Duplicate detection (query by account_id + amount + recent time window — independent of VSS check)
4. Contribution type defaulting for retirement accounts (set to `INDIVIDUAL` if IRA and not specified)
5. Omnibus account resolution (look up by correspondent_id + type=OMNIBUS)

**Error codes:** `FS_ACCOUNT_INELIGIBLE`, `FS_OVER_DEPOSIT_LIMIT`, `FS_DUPLICATE_DEPOSIT`, `FS_CONTRIBUTION_CAP`

### 6.3 Orchestrator

Central coordinator. Drives the transfer lifecycle.

**Flow:**

```
1. POST /deposits received
2. Create transfer in Requested state
3. → Validating: call VSS
4. VSS result:
   - IQA fail or duplicate → Rejected (terminal)
   - Success/flag → Analyzing: call Funding Service
5. FS result:
   - REJECT → Rejected (terminal)
   - FLAG_FOR_REVIEW → stay in Analyzing, set review_reason, write event, appear in operator queue
   - APPROVE → Approved
6. Approved: immediately call LedgerService.PostDoubleEntry()
   - Success → FundsPosted
   - Failure → stay in Approved, log SYS_LEDGER_POST_FAIL, alert
7. FundsPosted: await settlement (Settlement Engine picks up at EOD)
8. Settlement confirmed → Completed
9. Return notification → Returned (from FundsPosted or Completed)
```

**Operator interaction point:** When transfer is in `Analyzing` with `review_reason` set, the operator queue shows it. Operator submits action via `POST /operator/actions`. Orchestrator validates the action, transitions state, writes event.

**Every state transition:**
- Writes a `state_changed` event to `transfer_events`
- Sends `pg_notify('transfer_updates', ...)` for SSE dashboard
- Updates `transfers.updated_at`

### 6.4 Settlement Engine

**Trigger:** `POST /settlement/trigger` (manual for demo) and/or cron job.

**Logic:**
1. Query: `SELECT * FROM transfers WHERE state = 'FundsPosted' AND submitted_at < [current cutoff timestamp]`
2. Group by correspondent
3. Generate JSON settlement file per correspondent (X9-equivalent structure):
   ```json
   {
     "file_header": { "sender": "APEX", "created_at": "..." },
     "cash_letter": {
       "correspondent_id": "...",
       "cutoff_date": "2025-01-15",
       "bundles": [{
         "checks": [{
           "transfer_id": "...",
           "micr": { "routing": "...", "account": "...", "check_number": "..." },
           "amount": 500.00,
           "front_image_ref": "...",
           "back_image_ref": "..."
         }]
       }],
       "total_amount": 500.00,
       "record_count": 1
     }
   }
   ```
4. Save file to disk (or GCS in Milestone 2)
5. Submit to Settlement Bank stub: `POST /settlement/submit`
6. Stub acknowledges → update batch status to `ACKNOWLEDGED`, update each transfer to `Completed`, set `settled_at`

**EOD cutoff:** 6:30 PM CT. All timestamps in UTC. Cutoff computed: `cutoff_time = today_in_CT.set(18, 30).to_utc()`. Deposits with `submitted_at < cutoff_time` are eligible for today's batch. Exactly 6:30:00.000 PM CT → next business day (strictly less than).

**Business days:** US federal banking holidays loaded from config. Static list for current + next year. Weekends excluded.

### 6.5 Return Handler

**Trigger:** `POST /returns` (called by Settlement Bank stub via webhook pattern).

**Auth:** Bearer token from env var (`SETTLEMENT_BANK_TOKEN`). Validated before any business logic.

**Logic:**
1. Validate token
2. Look up transfer by ID
3. Validate state is `FundsPosted` or `Completed` (otherwise reject with error)
4. Load fee amount from correspondent's `rules_config.fees.returned_check`
5. In a single DB transaction:
   - Post 4 ledger entries (2 reversal + 2 fee)
   - Transition transfer to `Returned`
   - Write `return_received` and `return_processed` events
   - Create investor notification
6. Send `pg_notify` for SSE dashboard

**Idempotency:** If transfer is already `Returned`, return 200 with current state (no-op).

**Negative balance:** If investor balance would go negative after reversal + fee, post all entries anyway. Set `account.status = 'COLLECTIONS'`. Do not block return processing.

### 6.6 Settlement Bank Stub

Separate HTTP server on :8082.

**Endpoints:**
- `POST /settlement/submit` — accepts JSON batch, returns `{ batch_id, status: "ACKNOWLEDGED", acknowledged_at, return_window_expires_at }`
- `POST /settlement/return` — triggered by admin UI "Simulate Return" button. Takes `{ transfer_id, return_reason_code }`. Calls `POST /returns` on the main API (webhook callback).

**Return reason codes:** `R01` (NSF), `R02` (closed account), `R08` (stop payment), `NCI` (non-conforming image).

---

## 7. Frontend Routes

| Route | Viewport | Role | Description |
|-------|----------|------|-------------|
| `/deposit` | Mobile | Investor | Deposit form: scenario dropdown (prebaked) or manual entry. Camera input. Submit button. |
| `/status/:id` | Mobile | Investor | Transfer status page. Plain-English state. Notification bell. |
| `/admin/flow` | Desktop | Admin/Operator | Real-time dashboard. Live state transitions (SSE). Settlement status indicator. |
| `/admin/queue` | Desktop | Operator | Review queue. Filters: date, status, account, amount. Detail view: images, MICR, confidence, amount comparison, decision trace. Approve/reject + contribution type override. |
| `/admin/ledger` | Desktop | Admin | Account balances. Ledger entries by transfer. Reconciliation indicator (green/red). |

All routes in one React app. CSS media queries handle responsive layout. Role gating via auth context (MVP: demo role switcher).

---

## 8. Auth (MVP)

Lightweight JWT middleware with hardcoded demo tokens. No external auth provider.

**Demo users:**

| User | Role | Correspondent | Token |
|------|------|---------------|-------|
| investor-alpha | investor | Alpha | hardcoded JWT |
| investor-beta | investor | Beta | hardcoded JWT |
| operator-alpha | operator | Alpha | hardcoded JWT |
| operator-beta | operator | Beta | hardcoded JWT |
| apex-admin | apex_admin | — | hardcoded JWT |

**Middleware extracts:**
- `role` → `context.Value(roleKey)`
- `correspondent_id` → `context.Value(correspondentKey)` → sets RLS session variable

**UI:** Role switcher dropdown in the app header. Switching roles swaps the token and reloads data.

**Milestone 2:** Swap to GCP Identity Platform. Same middleware interface — reads role and correspondent_id from Firebase JWT custom claims instead of hardcoded tokens.

---

## 9. Observability (Spec Requirements)

| Requirement | Implementation |
|-------------|---------------|
| Per-deposit decision trace | `transfer_events` table + `GET /deposits/:id/events` endpoint + operator detail view |
| Differentiate deposit sources in logs | Structured logging with `slog.With("correspondent_id", cID)` injected by middleware |
| Monitor for missing/delayed settlement files | `GET /health/settlement` endpoint + indicator on `/admin/flow` dashboard |
| Redacted logs | Helper function: `redact("021000021")` → `"*****0021"`. Applied to routing numbers, account numbers, MICR data in all log output |

---

## 10. Error Taxonomy

```go
type DepositError struct {
    Code    string                 // machine-readable, persisted to DB
    Message string                 // operator-visible, detailed
    UserMsg string                 // investor-visible, sanitized, actionable
    Detail  map[string]interface{} // structured metadata for logs
}
```

| Code | Source | Investor Message |
|------|--------|-----------------|
| VSS_IQA_BLUR | VSS | "Image too blurry — hold steady on a flat surface with good lighting." |
| VSS_IQA_GLARE | VSS | "Glare detected — avoid direct light on the check." |
| VSS_MICR_READ_FAIL | VSS | (not shown — goes to operator review) |
| VSS_DUPLICATE_DETECTED | VSS | "This check has already been deposited." |
| VSS_AMOUNT_MISMATCH | VSS | (not shown — goes to operator review) |
| FS_OVER_DEPOSIT_LIMIT | Funding | "Maximum single deposit is $5,000. Contact support for larger amounts." |
| FS_ACCOUNT_INELIGIBLE | Funding | "Check deposits are not available for this account type." |
| FS_DUPLICATE_DEPOSIT | Funding | "A similar deposit was recently submitted." |
| SYS_VENDOR_TIMEOUT | System | "We're experiencing delays. Please try again shortly." |
| SYS_LEDGER_POST_FAIL | System | (internal only — investor sees retry) |
| SYS_INVALID_TRANSITION | System | (internal only — concurrent transition race) |

---

## 11. Test Plan

### Go Unit Tests (fast, logic layer)

| # | Test | Category |
|---|------|----------|
| 1 | State machine: valid transitions succeed | State machine |
| 2 | State machine: invalid transitions return typed error | State machine |
| 3 | Funding Service: amount exactly $5,000 → APPROVE | Business rules |
| 4 | Funding Service: amount $5,001 → REJECT with FS_OVER_DEPOSIT_LIMIT | Business rules |
| 5 | Funding Service: IRA at Beta → REJECT with FS_ACCOUNT_INELIGIBLE | Business rules |
| 6 | Funding Service: IRA at Alpha → APPROVE with contribution_type = INDIVIDUAL | Business rules |
| 7 | Funding Service: duplicate detection → REJECT | Business rules |
| 8 | Ledger: PostDoubleEntry creates balanced debit/credit pair | Ledger |
| 9 | Ledger: Reconcile returns 0 after multiple postings | Ledger |
| 10 | Ledger: Return reversal creates 4 entries, fee = $30 from config | Reversal + fee |
| 11 | Ledger: Return reversal when balance insufficient — still posts, flags collections | Reversal |
| 12 | Settlement: file contains only FundsPosted transfers, no Rejected | Settlement |
| 13 | Settlement: deposit at exactly 6:30 PM CT → next business day | Settlement |
| 14 | Idempotency: duplicate key returns original response, no new transfer | Idempotency |

### Playwright E2E Tests (browser, integration layer)

| # | Test | Category |
|---|------|----------|
| 1 | Happy path: submit → status shows each state → Completed | Happy path E2E |
| 2 | IQA Blur: submit ALPHA-002 → Rejected, retake prompt shown | VSS scenario |
| 3 | IQA Glare: submit ALPHA-003 → Rejected, glare message shown | VSS scenario |
| 4 | MICR Failure: submit ALPHA-004 → appears in operator queue | VSS scenario |
| 5 | Amount Mismatch: submit ALPHA-005 → operator sees amount comparison | VSS scenario |
| 6 | Duplicate: submit BETA-001 → Rejected | VSS scenario |
| 7 | Over limit: submit $5,001 via BETA-002 → Rejected by FS | Business rule |
| 8 | Operator approve: open flagged transfer, approve, verify state advances | Operator workflow |
| 9 | Operator reject: open flagged transfer, reject with reason, verify audit | Operator workflow |
| 10 | Return/reversal: trigger return after Completed, verify Returned + ledger entries + fee | Return handling |
| 11 | Settlement: trigger settlement, verify file contents | Settlement |

**Total: 25 tests (14 unit + 11 E2E).** Exceeds the spec's minimum 10.

### Test Report

`npx playwright test --reporter=html` output saved to `/reports/playwright/`. Go test output saved to `/reports/unit-tests.txt`. Summary markdown generated in `/reports/TEST_REPORT.md`.

### Demo Scripts

Shell scripts in `/scripts/` that curl API endpoints in sequence. Deterministic. Documented in README.

| Script | Path |
|--------|------|
| scripts/demo-happy-path.sh | Full lifecycle: submit → validate → approve → post → settle → complete |
| scripts/demo-rejection.sh | IQA fail, over-limit, duplicate — three rejection paths |
| scripts/demo-manual-review.sh | MICR failure → operator queue → approve |
| scripts/demo-return.sh | Happy path through Completed → trigger return → verify Returned + fee |

---

## 12. Milestoning

### Epic 0 — Foundation

- [x] PRD (this document)
- [ ] Repo structure
- [ ] Docker Compose: Postgres + Go API + VSS stub + Settlement Bank stub + React dev server
- [ ] `make dev` works — all services start, health checks pass
- [ ] goose migrations: all tables from Section 4
- [ ] Seed script: correspondents, accounts, scenarios.yaml
- [ ] `.env.example`

### Epic 1 — Pulumi Bootstrap

- [ ] Pulumi Go program: 2 stacks (dev, prod)
- [ ] Dev stack: Cloud Run (scale to zero) + Cloud SQL + GCS bucket
- [ ] Prod stack: Cloud Run (min instances 1) + Cloud SQL (backups) + GCS
- [ ] `make deploy-dev` and `make deploy-prod`
- [ ] Verify: `make dev` (local) still works independently of Pulumi

### Epic 2 — Core Flow (25 pts: Core Correctness)

- [ ] State machine implementation (hand-rolled transition table)
- [ ] Orchestrator: Requested → Validating → Analyzing → Approved → FundsPosted
- [ ] VSS client interface + HTTP implementation calling stub
- [ ] Funding Service: all 5 rules
- [ ] LedgerService: PostDoubleEntry, GetBalance, GetEntries, Reconcile
- [ ] Idempotency middleware (Postgres-only)
- [ ] Transfer events logging on every transition

### Epic 3 — Vendor Service Stub (15 pts)

- [ ] Standalone HTTP server (:8081)
- [ ] scenarios.yaml loader
- [ ] 7 differentiated response scenarios
- [ ] Account code routing + X-Scenario header override
- [ ] Documented in README

### Epic 4 — Settlement + Returns (10 pts: Returns + partial Settlement)

- [ ] Settlement Engine: batch FundsPosted transfers, generate JSON file, submit to stub
- [ ] EOD cutoff logic (6:30 PM CT, business days)
- [ ] Settlement Bank stub: /submit (acknowledge) + /return (webhook callback)
- [ ] Return Handler: reversal postings + fee + state transition + notification
- [ ] `POST /settlement/trigger` manual trigger
- [ ] Settlement Bank acknowledgment tracking

### Epic 5 — Operator Workflow (10 pts)

- [ ] `GET /operator/queue` with search/filter params
- [ ] `POST /operator/actions` with audit logging
- [ ] Operator detail view: images, MICR data, confidence score, recognized vs entered amount
- [ ] Contribution type override
- [ ] Operator approve → Approved; reject → Rejected
- [ ] Decision trace in detail view (transfer_events)

### Epic 6 — Frontend (spans 20 pts: Architecture + 10 pts: Operator)

- [ ] /deposit: scenario dropdown, manual entry mode, submit, status redirect
- [ ] /status/:id: plain-English state, notification bell
- [ ] /admin/flow: real-time dashboard (SSE), settlement status indicator
- [ ] /admin/queue: review queue with filters, detail view, approve/reject
- [ ] /admin/ledger: balances, entries, reconciliation indicator
- [ ] Role switcher (demo auth)
- [ ] Responsive layout (mobile deposit, desktop admin)

### Epic 7 — Observability

- [ ] Structured logging with correspondent_id
- [ ] Log redaction helpers
- [ ] `GET /health/ledger` — reconciliation
- [ ] `GET /health/settlement` — unbatched transfer monitoring
- [ ] Settlement status on `/admin/flow`

### Epic 8 — Tests + Docs (10 pts: Tests + 10 pts: DX)

- [ ] 14 Go unit tests
- [ ] 11 Playwright E2E tests
- [ ] Test report artifact in `/reports`
- [ ] 4 demo shell scripts in `/scripts`
- [ ] README: setup, architecture, flows, demo instructions
- [ ] `/docs/decision_log.md`
- [ ] `/docs/architecture.md` with system diagram
- [ ] Short write-up (≤1 page)
- [ ] Risks/limitations note

### Milestone 2 — Hardening + Infra Signals

- [ ] CompletedFinalized state + background finalization job
- [ ] Redis idempotency fast-path cache
- [ ] pgcrypto encryption hooks (accessor functions → encrypt/decrypt)
- [ ] RLS policy refinement
- [ ] X9 binary file generation (moov-io/imagecashletter)
- [ ] gRPC extraction for Funding Service and/or LedgerService

### Milestone 3 — Differentiation

- [ ] Risk dashboard (`/admin/risk`): float exposure by correspondent, return rates, top investors by outstanding provisional credit
- [ ] GCP Identity Platform (real auth)
- [ ] Pulumi third stack (QA/staging) if needed
- [ ] Decision trace admin tab with cross-transfer search

---

## 13. Repo Structure

```
/
├── cmd/
│   ├── api/                  # Main API server
│   ├── vendor-stub/          # VSS standalone HTTP server
│   └── settlement-stub/      # Settlement Bank stub
├── internal/
│   ├── orchestrator/         # State machine + transfer lifecycle
│   ├── funding/              # Funding Service rule engine
│   ├── ledger/               # LedgerService interface + implementation
│   ├── settlement/           # Settlement Engine (batch + file gen)
│   ├── returns/              # Return Handler
│   ├── notify/               # Notification creation
│   ├── auth/                 # JWT middleware + RLS context
│   ├── events/               # SSE stream + pg_notify listener
│   └── store/                # DB access layer
├── db/
│   └── migrations/           # goose SQL migration files
├── test-scenarios/
│   └── scenarios.yaml        # Single source of truth for test scenarios
├── web/                      # React PWA (Vite + TypeScript)
├── infra/                    # Pulumi Go program
├── tests/
│   ├── unit/                 # Go unit tests
│   └── e2e/                  # Playwright tests
├── scripts/
│   ├── demo-happy-path.sh
│   ├── demo-rejection.sh
│   ├── demo-manual-review.sh
│   └── demo-return.sh
├── docs/
│   ├── decision_log.md
│   ├── architecture.md
│   └── prd.md                # This document
├── reports/                  # Generated test reports
├── .env.example
├── Makefile
├── docker-compose.yml
└── README.md
```

---

## 14. Configuration

### .env.example

```env
# Database
DATABASE_URL=postgres://apex:apex@localhost:5432/apex_check_deposit?sslmode=disable

# Services
VSS_URL=http://localhost:8081
SETTLEMENT_BANK_URL=http://localhost:8082
API_PORT=8080

# Auth (MVP: demo tokens)
JWT_SECRET=dev-secret-change-in-production

# Settlement
SETTLEMENT_CUTOFF_TIME=18:30
SETTLEMENT_CUTOFF_TIMEZONE=America/Chicago
SETTLEMENT_RETURN_WINDOW_DAYS=2

# Settlement Bank webhook auth
SETTLEMENT_BANK_TOKEN=dev-settlement-token

# Scenarios
SCENARIOS_PATH=./test-scenarios/scenarios.yaml
```

---

## 15. Key Decisions (for decision_log.md)

| Decision | Choice | Alternative Considered | Rationale |
|----------|--------|----------------------|-----------|
| Language | Go | Java | Goroutines, single binary, Ascend alignment |
| Database | Postgres | SQLite (spec allows) | ACID, RLS, LISTEN/NOTIFY, production-appropriate |
| State machine | Hand-rolled table | looplab/fsm | 8 static states, auditable, no hidden abstractions |
| Internal comms | REST (MVP) | gRPC | Spec says REST; interfaces designed for gRPC extraction |
| Ledger model | True double-entry (debit/credit pairs) | Single-row movements | Reconciliation invariant (SUM=0), industry standard |
| Real-time transport | SSE | WebSocket | Server→client only needed; simpler, no library |
| Idempotency | Postgres-only | Redis + Postgres | Correct at demo scale; Redis is Milestone 2 optimization |
| Auth | Demo JWT tokens | GCP Identity Platform | Spec doesn't require auth; same middleware interface, swap later |
| Settlement file | JSON (X9-equivalent) | Real X9 binary | Spec allows "structured equivalent"; X9 upgrade in Milestone 2 |
| Migrations | goose | golang-migrate | Simpler single-file convention |
| IaC | Pulumi (Go) on GCP | Terraform / none | Same language as app, Ascend alignment, two-stack parameterization |
| Fee handling | Config-driven ($30 in seed) | Hardcoded constant | Configurable per correspondent via rules_config |

---

## 16. Addendum: Gap Fixes from Rubric Audit

These items were identified during a line-by-line audit of the spec against the sprint plan. Each is assigned to a specific PR.

### 16.1 Spec Ledger Attributes Must Be Verified on Transfer Record

The spec explicitly lists: To AccountId, From AccountId, Type: MOVEMENT, Memo: FREE, SubType: DEPOSIT, Transfer Type: CHECK, Currency: USD, Amount, SourceApplicationId: TransferID.

These live on the `transfers` table. Every PR that creates a transfer and every test that verifies a transfer must check these fields are populated with the correct values. Add to PR 1.5 verification:

- Transfer record has: `type='MOVEMENT'`, `memo='FREE'`, `sub_type='DEPOSIT'`, `transfer_type='CHECK'`, `currency='USD'`, `from_account_id` = correspondent omnibus, `account_id` = investor account.
- `GET /deposits/:id` response includes all spec-required attributes.
- Go unit test: `TestTransferRecord_SpecAttributes`.

### 16.2 Image Upload Pipeline

The spec says: "Endpoint or UI to simulate mobile check deposit submission (front image, back image, deposit amount, account identifier)" and the operator queue shows "Check images (front and back)."

Implementation:

- `POST /deposits` accepts multipart form data with `front_image` and `back_image` file fields.
- Images stored locally at `./data/images/{transfer_id}/front.jpg` and `back.jpg`.
- `GET /deposits/:id/images/{side}` serves the stored image.
- `/deposit` form has file picker inputs with camera capture on mobile (`accept="image/*" capture="environment"`).
- Operator detail view displays front and back images loaded from the API.

For demo with synthetic data: user can upload any image, or skip (the VSS stub ignores content). The pipeline must accept and store the files regardless.

### 16.3 Risk Indicators in Operator Queue

The spec says the queue shows "Risk indicators and Vendor Service scores."

Vendor Service scores = `confidence_score` from VSS (already on transfer record).

Risk indicators are computed client-side in the operator queue UI:

| Condition | Badge | Color |
|-----------|-------|-------|
| Amount > $2,000 | "Large deposit" | Yellow |
| Confidence score < 0.90 | "Low confidence" | Yellow |
| MICR failure (review_reason = VSS_MICR_READ_FAIL) | "MICR unreadable" | Red |
| Amount mismatch (review_reason = VSS_AMOUNT_MISMATCH) | "Amount discrepancy" | Yellow |

Badges visible in both list view and detail view.

### 16.4 Reversal Entry Separation

Verification must explicitly confirm:

- Reversal entries debit the investor for the **original deposit amount** (not deposit minus fee).
- Fee entries are a **separate** debit/credit pair.
- After return processing, investor balance = `-(fee amount)` (e.g., -$30 for a $500 deposit returned with $30 fee).

### 16.5 Demo Script Coverage

`demo-rejection.sh` must exercise all four rejection paths:

1. IQA Blur (ALPHA-002) → Rejected, VSS_IQA_BLUR
2. IQA Glare (ALPHA-003) → Rejected, VSS_IQA_GLARE
3. Duplicate (BETA-001) → Rejected, VSS_DUPLICATE_DETECTED
4. Over limit (BETA-002, $5,001) → Rejected, FS_OVER_DEPOSIT_LIMIT

Each sub-test asserts the expected state and error code.
