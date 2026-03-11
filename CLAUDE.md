# CLAUDE.md

## Project

Mobile check deposit system for brokerage accounts. Go backend, Postgres, React + TypeScript frontend.

Source of truth: `docs/prd.md` (requirements), `docs/sprint_plan.md` (execution).

## Commands

```bash
make dev          # docker compose up — Postgres, API, VSS stub, Settlement stub, React
make reset        # drop DB, re-run migrations + seed
make test         # go test ./... -v
make test-e2e     # npx playwright test
```

## Hard Rules

### State Machine — Do Not Modify

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

Do not add, remove, or rename states. These are the 8 states from the spec. Every transition uses optimistic locking (`UPDATE ... WHERE state = $expected RETURNING id`). Every transition writes a `transfer_events` row and calls `pg_notify`.

### Ledger Invariants

1. Every money movement = exactly 2 ledger entries (one DEBIT, one CREDIT, same amount, same `movement_id`). No exceptions.
2. `SUM(CASE WHEN side='CREDIT' THEN amount ELSE -amount END) FROM ledger_entries` must always equal zero.
3. Ledger entries are append-only. Never UPDATE or DELETE.
4. Return processing always completes — even if investor balance goes negative. Post all 4 entries, flag account for collections.
5. Fee amount comes from `correspondents.rules_config.fees.returned_check`, never a hardcoded number.

### Transfer Record — Required Fields

Every transfer must have these at creation (from spec):
- `type='MOVEMENT'`, `memo='FREE'`, `sub_type='DEPOSIT'`, `transfer_type='CHECK'`, `currency='USD'`
- `from_account_id` = correspondent's omnibus account
- `account_id` = investor account

### Architecture Boundaries

- `internal/store/` is the ONLY package that imports `database/sql`. Everything else uses interfaces.
- No cross-imports between peer packages (funding doesn't import ledger, etc.).
- The orchestrator calls services through interfaces — this is the gRPC extraction seam.

### Do Not Add to MVP

No Redis, Kafka, gRPC, pgcrypto, service workers, localStorage, or TensorFlow. These are Milestone 2+. Mention in the decision log, not in code.

### Logging

Always include `correspondent_id` and `transfer_id` in structured log fields. Never log raw routing numbers or account numbers — use last-4-digits masking (`*****1234`).

## Verification

Every PR in `docs/sprint_plan.md` has explicit pass/fail criteria. Do not move to the next PR until every box is checked.
