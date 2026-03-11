# Rubric Audit + Human Verification + Execution Strategy

---

## Part 1: Line-by-Line Rubric Audit

Every requirement from the spec, mapped to the PR that delivers it and the verification criterion that proves it. Items marked ⚠️ are gaps I found.

---

### Category 1: System Design and Architecture (20 pts)

> "Clear service boundaries, data flow, state machine design, and trade-off rationale"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| Clear service boundaries | PR 6.2 (architecture.md) | Mermaid diagram with all 7 services labeled | ✅ |
| Data flow documented | PR 6.2 (architecture.md) | End-to-end flow diagram: investor → API → VSS → FS → Ledger → Settlement | ✅ |
| State machine design | PR 1.1 (orchestrator) | 8 states, all valid transitions, hand-rolled table | ✅ |
| Trade-off rationale | PR 6.2 (decision_log.md) | 12 decisions with alternatives from PRD Section 15 | ✅ |
| Clean separation of concerns (7 listed) | Repo structure PR 0.1 | Separate packages: vendorclient, vendor-stub, funding, ledger, orchestrator, settlement, returns | ✅ |

**Gap found: None.** This category is well-covered. The architecture doc and decision log are the main deliverables — both in PR 6.2.

---

### Category 2: Core Correctness (25 pts)

> "Happy path works end-to-end; business rules enforced; state transitions correct; ledger postings accurate"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| Happy path: submit → validate → rules → posting → review → settle → completed | TB1 (PR 1.5) + TB4 (PR 4.1) | Playwright: full lifecycle reaches Completed | ✅ |
| Business rule: deposit limit >$5,000 rejected | PR 1.3 | Unit test: $5001 → REJECT | ✅ |
| Business rule: contribution type defaults for retirement | PR 1.3 | Unit test: IRA → contribution_type=INDIVIDUAL | ✅ |
| Business rule: duplicate deposit detection (beyond VSS) | PR 3.2 | Unit test: FS duplicate → REJECT | ✅ |
| Business rule: account eligibility | PR 1.3 | Unit test: IRA at Beta → FS_ACCOUNT_INELIGIBLE | ✅ |
| Business rule: validate investor session | PR 3.3 | Auth middleware rejects invalid token | ✅ |
| Business rule: resolve account identifiers to internal account/routing | PR 1.3 | Unit test: omnibus resolution per correspondent | ✅ |
| State transitions correct (all 8 states) | PR 1.1 | Unit tests for all 9 valid transitions + invalid transition rejection | ✅ |
| Ledger postings accurate | PR 1.4 | Unit test: balanced pair, reconcile=0 | ✅ |
| Ledger has correct attributes (Type, Memo, SubType, etc.) | PR 1.5 | ⚠️ **SEE BELOW** | ⚠️ |
| Idempotency (no duplicate ledger postings) | PR 1.5 | Unit test: duplicate Idempotency-Key → no new transfer | ✅ |

**⚠️ Gap found: Spec's ledger attributes not explicitly verified.**

The spec says the ledger posting must have: To AccountId, From AccountId, Type: MOVEMENT, Memo: FREE, SubType: DEPOSIT, Transfer Type: CHECK, Currency: USD, Amount, SourceApplicationId: TransferID. These live on the `transfers` table per our design. But no verification criterion in the sprint plan explicitly checks that these fields are populated with the correct values.

**Fix:** Add to PR 1.5 verification:
```
□ Transfer record has: type='MOVEMENT', memo='FREE', sub_type='DEPOSIT', 
  transfer_type='CHECK', currency='USD', from_account_id=OMNIBUS, 
  account_id=investor account
□ GET /deposits/:id response includes all spec-required ledger attributes
```

And add a Go unit test:
```
TestTransferRecord_SpecAttributes: verify created transfer has all required fields
```

---

### Category 3: Vendor Service Stub Quality (15 pts)

> "Differentiated responses are configurable, deterministic, and cover all required scenarios"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| IQA Pass | PR 1.2 (ALPHA-IRA uses this) | VSS returns iqa_status=pass | ✅ |
| IQA Fail (Blur) | PR 2.1 | VSS returns iqa_status=fail, error=blur | ✅ |
| IQA Fail (Glare) | PR 2.1 | VSS returns iqa_status=fail, error=glare | ✅ |
| MICR Read Failure | PR 3.1 | VSS returns micr_data=null | ✅ |
| Duplicate Detected | PR 2.1 | VSS returns duplicate_flag=true | ✅ |
| Amount Mismatch | PR 3.1 | VSS returns ocr_amount ≠ input amount | ✅ |
| Clean Pass | PR 1.2 | VSS returns all fields populated, confidence 0.97 | ✅ |
| Configurable without code changes | PR 1.2 | scenarios.yaml loaded at startup | ✅ |
| Deterministic | PR 1.2 | Same account → same response every time | ✅ |
| Selectable via test account number | PR 1.2 | Account code → scenario mapping | ✅ |
| Selectable via request header | PR 1.2 | X-Scenario header override | ✅ |
| Selectable via configuration file | PR 1.2 | scenarios.yaml | ✅ |

**⚠️ Gap found: "Vendor API stub accepts image payloads"**

The spec says under Deposit Submission: "Vendor API stub accepts image payloads and returns structured validation results." Our VSS endpoint takes account_id and amount, but does it accept actual image data? The spec says the deposit submission includes "front image, back image" — and the VSS must accept them.

For MVP with synthetic data, the images are simulated. But the VSS `POST /validate` request should include `front_image` and `back_image` fields (even if the stub ignores the actual image content and routes by account code). This ensures the interface is correct for a real vendor swap.

**Fix:** Already in PR 1.2's request schema, but add verification:
```
□ POST /validate accepts front_image and back_image fields in request body
□ Stub ignores image content for routing (routes by account_id) but accepts the fields
□ API POST /deposits accepts front image and back image (file upload or base64)
```

And verify in PR 1.7 (deposit frontend):
```
□ /deposit form has front image and back image upload inputs (file picker or camera)
□ Submitting includes image references in the request
```

---

### Category 4: Operator Workflow and Observability (10 pts)

> "Review queue functions; audit trail complete; decision traces available"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| Review queue showing flagged deposits | PR 3.3, 3.4 | Playwright: flagged transfer visible in queue | ✅ |
| Check images (front and back) in queue | PR 3.4 | ⚠️ **SEE BELOW** | ⚠️ |
| MICR data and confidence scores | PR 3.4 | Detail panel shows MICR data + confidence | ✅ |
| Risk indicators and Vendor Service scores | PR 3.4 | ⚠️ **SEE BELOW** | ⚠️ |
| Recognized vs. entered amount comparison | PR 3.4 | Detail panel shows both amounts side by side | ✅ |
| Approve/reject controls | PR 3.3, 3.4 | Playwright: approve and reject work | ✅ |
| Mandatory action logging | PR 3.3 | operator_action event written with operator_id, action, reason | ✅ |
| Ability to override contribution type | PR 3.3, 3.4 | Contribution type dropdown for IRA accounts | ✅ |
| Search and filter by date, status, account, amount | PR 3.3 | Query params on GET /operator/queue | ✅ |
| Audit log of all operator actions (who, what, when) | PR 3.3 | transfer_events WHERE step='operator_action' | ✅ |
| Per-deposit decision trace | TB1 PR 1.5 (events endpoint) + PR 3.4 (UI) | GET /deposits/:id/events returns full chain | ✅ |
| Differentiate between deposit sources in logs | PR 6.1 | Structured logging with correspondent_id | ✅ |
| Monitor for missing/delayed settlement files | PR 4.2 | /health/settlement + admin indicator | ✅ |
| Redacted logs | PR 6.1 | redact() helper, grep verification | ✅ |

**⚠️ Gap found: Check images in operator queue.**

The spec says the review queue shows "Check images (front and back)." Our system stores `front_image_ref` and `back_image_ref` on the transfer. But for MVP with synthetic data, what are these? Options:

- Placeholder image (a generic check image bundled with the app)
- The actual file the user uploaded via the deposit form's file picker
- A URL reference to a stored file

For MVP, the deposit form should accept a file upload (or camera capture). The Go API stores the file locally (`./data/images/{transfer_id}/front.jpg`). The operator detail view loads the image via `GET /deposits/:id/images/front`. This proves the image pipeline works even with synthetic check photos.

**Fix:** Add to PR 1.5:
```
□ POST /deposits accepts multipart form data with front_image and back_image files
□ Images stored at ./data/images/{transfer_id}/front.jpg and back.jpg
□ GET /deposits/:id returns front_image_ref and back_image_ref paths
```

Add to PR 1.7 (deposit frontend):
```
□ /deposit form has file picker inputs for front and back images
□ Camera capture works on mobile (input accept="image/*" capture="environment")
```

Add to PR 3.3:
```
□ GET /deposits/:id/images/front returns the stored image file
□ GET /deposits/:id/images/back returns the stored image file
```

Add to PR 3.4:
```
□ Operator detail view displays front and back check images
□ Images load from API (not hardcoded placeholders)
```

**⚠️ Gap found: "Risk indicators" are vague — need to define what we show.**

The spec says "Risk indicators and Vendor Service scores." Vendor Service scores = confidence_score (already handled). Risk indicators are not defined in the spec, so we define them. For MVP:

- Amount > $2,000 → yellow "Large deposit" badge
- Confidence score < 0.90 → yellow "Low confidence" badge  
- MICR failure → red "MICR unreadable" badge
- Amount mismatch → yellow "Amount discrepancy" badge

These are computed client-side from existing data. No new backend work.

**Fix:** Add to PR 3.4:
```
□ Operator queue shows risk indicator badges based on transfer data
□ Large deposit (>$2000): yellow badge
□ Low confidence (<0.90): yellow badge
□ MICR failure: red badge
□ Amount mismatch: yellow badge
□ Badges visible in list view, not just detail view
```

---

### Category 5: Return/Reversal Handling (10 pts)

> "Bounced checks reversed correctly with fee; state transitions correct"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| Accept return notifications (simulated for stub) | PR 5.1 | POST /returns processes notification | ✅ |
| Reversal postings: debit investor for original amount | PR 5.1 | Ledger entry: DEBIT investor $500 | ✅ |
| Deduct return fee ($30) | PR 5.1 | Ledger entry: DEBIT investor $30, CREDIT FEE-APEX $30 | ✅ |
| Transition to Returned state | PR 5.1 | Transfer state = Returned after return | ✅ |
| Notify investor of returned check and fee | PR 5.2 | Notification created + displayed in UI | ✅ |
| Return from Completed (not just FundsPosted) | PR 5.1 | Unit test: return from Completed works | ✅ |
| Fee from config (not magic number) | PR 5.1 | Reads rules_config.fees.returned_check | ✅ |

**⚠️ Gap found: Spec says "Debit the investor account for the original deposit amount" and separately "Deduct return fee ($30 hard-coded for MVP)."**

Our implementation posts 4 entries (2 reversal + 2 fee). The spec's language says "deduct return fee" — which could mean subtract from the reversal amount (return $470 net) OR charge separately (reverse $500 and charge $30 independently). Our design charges separately (4 entries), which is the correct financial pattern. But we should verify:

```
□ Reversal amount = original deposit amount (not deposit minus fee)
□ Fee is a SEPARATE ledger entry pair, not subtracted from reversal
□ After return: investor balance = -(fee amount), not 0
```

This is already in our design but the verification criterion should be explicit. Adding to PR 5.1.

---

### Category 6: Tests and Evaluation Rigor (10 pts)

> "Minimum 10 tests; all paths exercised; test report generated"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| Happy path end-to-end test | TB1 PR 1.8 | Playwright: submit → Completed | ✅ |
| Each Vendor Service stub response scenario tested | TB2 PR 2.4, TB3 PR 3.5 | Playwright: all 7 scenarios | ✅ |
| Business rule enforcement (deposit limits) | PR 1.3 | Unit test: $5000 boundary, $5001 reject | ✅ |
| Business rule enforcement (contribution defaults) | PR 1.3 | Unit test: IRA → INDIVIDUAL | ✅ |
| State machine transitions (valid and invalid) | PR 1.1 | Unit tests: all valid + invalid | ✅ |
| Reversal posting with fee calculation | PR 5.1 | Unit test: 4 entries, fee=$30 | ✅ |
| Settlement file contents validation | PR 4.1 | Unit test: file structure, no rejected transfers, totals correct | ✅ |
| Test report generated | PR 6.2 | /reports/TEST_REPORT.md + playwright HTML + unit test output | ✅ |
| Minimum 10 tests | All | 25 total (14 unit + 11 Playwright) | ✅ |
| All paths exercised | All TBs | 4 demo scripts + 11 Playwright tests cover every path | ✅ |

**Gap found: None.** Well-covered with margin.

---

### Category 7: Developer Experience (10 pts)

> "One-command setup; clear README; demo scripts; decision log"

| Requirement | PR | Verification | Status |
|-------------|-----|-------------|--------|
| One-command setup | PR 0.1 | `make dev` starts everything | ✅ |
| Clear README | PR 6.2 | Setup, architecture, demo instructions | ✅ |
| Demo scripts exercising all paths | TB1-TB5 (incrementally) | 4 shell scripts: happy, rejection, manual review, return | ✅ |
| Decision log | PR 6.2 | /docs/decision_log.md with 12 decisions | ✅ |
| .env.example | PR 0.1 | All env vars documented | ✅ |
| Risks/limitations note | PR 6.2 | No compliance claims, synthetic data noted | ✅ |
| Short write-up ≤1 page | PR 6.2 | Architecture, stub design, state machine rationale | ✅ |
| SUBMISSION.md | PR 6.2 | Per spec's Common Submission Format | ✅ |

**⚠️ Gap found: "Deterministic demo scripts"**

The spec says "Deterministic demo scripts that exercise all paths." Our 4 demo scripts cover: happy path, rejection (3 types), manual review, return. But we're missing one path explicitly: the **over-limit rejection path** ($5,001 deposit). It's tested in unit tests (PR 1.3) and Playwright (PR 3.5), but the demo-rejection.sh script should also exercise it via curl.

**Fix:** Already planned in PR 2.4 ("exercises blur, glare, duplicate, over-limit paths") but verification criterion should be explicit:
```
□ demo-rejection.sh exercises: IQA blur, IQA glare, duplicate, over-limit ($5001)
□ Each sub-test in the script asserts the expected state (Rejected) and error code
```

---

### Deliverables Checklist (from spec)

| Deliverable | PR | Status |
|-------------|-----|--------|
| README.md | PR 6.2 | ✅ |
| /docs/decision_log.md | PR 6.2 | ✅ |
| /docs/architecture.md | PR 6.2 | ✅ |
| /tests (unit + integration) | Incremental, finalized PR 6.3 | ✅ |
| /reports (test results + coverage) | PR 6.2 (generate-report.sh) | ✅ |
| .env.example | PR 0.1 | ✅ |
| Vendor Service stub with documented scenarios | PR 1.2 + README | ✅ |
| Demo scripts exercising all paths | TB1-TB5 | ✅ |
| Short write-up ≤1 page | PR 6.2 | ✅ |

**Gap found: None.**

---

### Performance Benchmarks

| Benchmark | How Met | Verification |
|-----------|---------|-------------|
| Vendor stub responds <1 second | Stub is in-memory YAML lookup, no I/O | ⚠️ No explicit timing assertion in tests |
| Ledger posting within seconds of approval | Immediate in orchestrator flow | ✅ (tested by state reaching FundsPosted) |
| Settlement file within seconds of trigger | In-memory query + JSON marshal | ✅ (tested in PR 4.1) |
| Flagged items in queue immediately | Synchronous DB write, SSE push | ✅ (tested in PR 3.5 Playwright) |
| State changes queryable within 1 second | SSE push + DB write in same transaction | ✅ (tested by SSE verification in PR 1.6) |

**⚠️ Minor gap: No explicit latency assertions in tests.** Playwright tests inherently have timeouts, but we're not explicitly asserting "<1 second." For MVP this is fine — everything runs locally on localhost and will be well under 1 second. But add to PR 6.3 final verification:
```
□ demo-happy-path.sh completes full lifecycle in <30 seconds
□ No Playwright test times out (default 30s timeout)
```

---

## Summary of Gaps Found

| # | Gap | Severity | Fix PR | Fix Description |
|---|-----|----------|--------|-----------------|
| 1 | Spec's ledger attributes not verified on transfer record | Medium | PR 1.5 | Add unit test + API response verification for Type, Memo, SubType, etc. |
| 2 | Image upload pipeline not explicit enough | Medium | PR 1.5, 1.7, 3.3, 3.4 | Add multipart file upload, local storage, image serving endpoint, operator image display |
| 3 | Risk indicator badges in operator queue undefined | Low | PR 3.4 | Define 4 risk badges, compute client-side from existing data |
| 4 | Over-limit path not explicitly in demo script verification | Low | PR 2.4 | Add $5001 test to demo-rejection.sh with assertion |
| 5 | Reversal vs fee separation not explicitly verified | Low | PR 5.1 | Add verification: reversal amount = original, fee is separate |
| 6 | No explicit latency benchmarks in tests | Low | PR 6.3 | Add overall timing check to final verification |

None of these are architectural — they're all verification criteria additions and minor implementation details within existing PRs. After these fixes, I'm confident in 100/100.

---

## Part 2: Human Verification Points

These are things automated tests CANNOT verify. You (Rajiv) must eyeball them at each epic boundary.

### After Epic 0 — The Rail

```
HUMAN CHECK:
□ Run `make dev` yourself on a clean terminal — does it feel smooth?
□ Open the React app in a mobile viewport (Chrome DevTools, iPhone 14) — 
  does the shell look like an app, not a broken desktop site?
□ Open in desktop viewport — does the admin sidebar render?
□ Check the DB schema: open psql, run \d transfers — do the columns 
  match what you expect from the PRD?
□ Read .env.example — are the variable names intuitive? Would a stranger 
  know what to set?
```

### After Epic 1 — Pulumi

```
HUMAN CHECK:
□ Visit the deployed Cloud Run URL — does the health check respond?
□ Check GCP Console: are the services where you expect them?
□ Verify Cloud SQL is accessible from Cloud Run (not just localhost)
□ Check the cost: is it within free tier / acceptable range?
□ Can you run `make dev` locally AND `make deploy-dev` without conflict?
```

### After TB1 — Happy Path

```
HUMAN CHECK:
□ THE CRITICAL DEMO: On your phone, open /deposit, select Clean Pass, 
  enter $500, submit. On your laptop, open /admin/flow. Do you see the 
  deposit flowing through states in real time? This is the money demo 
  moment. If it doesn't feel good, stop and fix before proceeding.
□ Open /admin/ledger — do the balances make intuitive sense? 
  Does reconciliation show green?
□ Read the event trace for the deposit (GET /deposits/:id/events) — 
  is it a clear narrative? Would an interviewer reading it understand 
  what happened?
□ Run the demo script — does it work from a clean state (after make reset)?
□ Submit twice with the same idempotency key — does it correctly 
  return the same response?
□ NARRATIVE CHECK: Can you explain this system to someone in 2 minutes 
  using the live UI? Practice your demo pitch now. The demo will expose 
  UX issues that tests won't catch.
```

### After TB2 — Rejection Paths

```
HUMAN CHECK:
□ Submit each failure scenario on your phone — are the error messages 
  clear and actionable? Would a real investor understand what to do?
□ IQA errors show "Try Again" — IQA blur says "hold steady", glare 
  says "avoid direct light." Do these read naturally?
□ Duplicate error: does it mention the original deposit? Is the 
  reference useful?
□ Does the /status/:id page for rejected deposits feel final? Red 
  state, clear message, no confusion about whether to retry?
□ Run demo-rejection.sh after make reset — clean run?
```

### After TB3 — Operator Review

```
HUMAN CHECK:
□ THE OPERATOR DEMO: Submit ALPHA-004 (MICR failure) on your phone. 
  Switch to laptop, go to /admin/queue. Is the flagged deposit 
  visible? Open it — do you see MICR data (or "unreadable"), 
  confidence score, check images?
□ For ALPHA-005 (Amount Mismatch): is the recognized vs entered 
  amount comparison obvious? A reviewer should immediately see 
  "$250 recognized / $500 entered" without hunting for it.
□ Approve a flagged deposit — does /admin/flow show it advancing 
  through Approved → FundsPosted in the live dashboard?
□ Reject a deposit — type a reason. Go to the event trace. Is the 
  operator action logged with your reason? Would an auditor be 
  satisfied?
□ Contribution type override: submit ALPHA-IRA, open in queue, 
  change contribution type to ROLLOVER. Is this saved? Does the 
  audit trail show the override?
□ FILTER CHECK: with multiple flagged deposits in queue, do the 
  filters (amount range, date, status) actually narrow the results?
□ ROLE CHECK: switch to operator-beta — can you see only Beta's 
  deposits? Switch to apex-admin — can you see everything?
□ Run demo-manual-review.sh after make reset — clean run?
```

### After TB4 — Settlement

```
HUMAN CHECK:
□ Happy path all the way to Completed — does it feel like a complete 
  lifecycle? Time it. Is it snappy?
□ The settlement file: open it (it's JSON). Read it. Does it look 
  like something a bank could plausibly consume? MICR data, amounts, 
  image refs, totals — all present?
□ Settlement status on /admin/flow — is the indicator intuitive? 
  Before trigger: "3 deposits ready." After: "Batch generated." 
  Would a non-technical operator understand it?
□ Run demo-happy-path.sh (now includes settlement) — clean and fast?
```

### After TB5 — Returns

```
HUMAN CHECK:
□ THE RETURN DEMO: This is the second money moment. Happy path to 
  Completed, then click "Simulate Return" on the admin dashboard. 
  Watch the transfer go from Completed → Returned in real time. 
  On mobile, check the investor's status page — does it show the 
  return with a clear message and fee explanation?
□ Open /admin/ledger — the deposit should show 6 entries total 
  (2 provisional + 4 reversal). Read them. Do the memos make sense? 
  Is reconciliation still green?
□ Check the investor balance — should be -$30. Does this display 
  clearly?
□ Check the notification — does it explain what happened in plain 
  English? "Your check deposit of $500 was returned. Reason: 
  Insufficient funds. A $30 returned check fee has been applied."
□ Try triggering a return on an already-Returned transfer — 
  does it gracefully no-op?
□ Run demo-return.sh — clean?
```

### After TB6 — Final Polish

```
HUMAN CHECK:
□ FRESH CLONE TEST: Clone the repo into a new directory. Follow 
  ONLY the README instructions. Does `make dev` work? Can you 
  run all 4 demo scripts? This is what the evaluator will do.
□ READ THE README: Is it clear? Is it concise? Does it have 
  architecture overview, how to demo, how to run tests? No broken 
  links?
□ READ THE DECISION LOG: Would a senior Apex engineer nod along? 
  Are the trade-offs real, not performative?
□ READ THE SHORT WRITE-UP: Is it ≤1 page? Does it answer: what, 
  why, and trade-offs?
□ READ SUBMISSION.md: Does it follow the spec's format exactly?
□ CHECK /reports: Do test reports exist? Are results passing?
□ GREP THE LOGS: Run the happy path, then grep the API logs for 
  any 9-digit numbers (routing), any account codes that should be 
  redacted. There should be NONE visible in clear text.
□ THE FULL DEMO DRY RUN: Run all 4 demo scripts in sequence after 
  make reset. Time the whole thing. Practice narrating what's 
  happening at each step. This is your interview.
□ CHECK EVERY DELIVERABLE against the spec's deliverables list. 
  Print it out and check the boxes physically.
```

---

## Part 3: Execution Strategy

### The Tradeoffs

**Option A: Single Claude Code session orchestrating subagents**

The main session holds the full context (PRD, sprint plan, spec) and spawns subagents for parallelizable PRs. The orchestrator merges, resolves conflicts, runs verification, and advances to the next PR.

Pro: Single source of truth. The orchestrator knows what every subagent is doing and can resolve conflicts. Verification happens centrally.

Con: The orchestrator's context window is the bottleneck. After TB3-TB4, the conversation history gets long. The orchestrator spends tokens re-reading its own prior output. Subagents spawned by Claude Code have limited context — they get a task description, not the full PRD.

**Option B: Multiple independent sessions on worktree branches**

You open separate Claude Code sessions, each on its own git worktree branch. Each session gets a handover prompt with its specific PRD section, file targets, and verification criteria. You manually merge branches and run verification.

Pro: Each session has a fresh, focused context. No window pollution from unrelated PRs. Agents can work truly in parallel across terminal windows. Each agent can be given the full PRD + its specific sprint plan section without competing for context space.

Con: You are the orchestrator. You manage branches, merges, conflicts, and cross-PR verification. More manual coordination work.

**Option C: Hybrid (recommended)**

One primary Claude Code session that acts as the orchestrator for sequential work and verification. Parallel worktree sessions for the identified parallelizable PRs, each with a handover prompt. You merge the parallel branches and the primary session verifies.

### Recommended: Option C — Hybrid

Here's the concrete workflow:

**Phase 1: Epic 0 (sequential — one session)**

This is foundational. One Claude Code session does all 4 PRs sequentially. The context window is fresh, the work is infrastructure-only, and there's nothing to parallelize (PR 0.4 can overlap but it's small enough to not bother).

Session 1 (primary): PRs 0.1 → 0.2 → 0.3 → 0.4

After Epic 0: human verification. Then commit to `main`.

**Phase 2: TB1 (parallel burst — 4 sessions + 1 orchestrator)**

This is the maximum parallelization moment. Four PRs (1.1, 1.2, 1.3, 1.4) touch completely different directories.

- Session 1 (primary/orchestrator): remains open, does PR 1.5 (wiring) after the parallel PRs merge, then PR 1.8 (tests)
- Session 2 (worktree: `tb1-orchestrator`): PR 1.1 — orchestrator + state machine
- Session 3 (worktree: `tb1-vss`): PR 1.2 — VSS stub
- Session 4 (worktree: `tb1-funding`): PR 1.3 — Funding Service
- Session 5 (worktree: `tb1-ledger`): PR 1.4 — Ledger Service

You merge all 4 branches into a `tb1-integration` branch. Session 1 picks up there, does PR 1.5 (wiring), PR 1.6 + 1.7 (frontend, can be one session sequentially), PR 1.8 (tests).

After TB1: human verification (THE CRITICAL DEMO). Merge to `main`.

**Phase 3: TB2 through TB5 (sequential with targeted parallelism)**

After TB1, the codebase is established and most PRs touch the orchestrator or API handlers (shared files). Parallelism is more limited.

For each TB:
- Session 1 (primary): sequential PRs
- When a frontend PR can parallel a backend PR (marked 🔀), spin up a worktree session

Specifically:
- TB2: Session 1 does PR 2.1 → 2.2. Session 2 does PR 2.3 (frontend) in parallel with 2.2. Merge, then Session 1 does PR 2.4.
- TB3: Session 1 does PR 3.1, 3.2. Session 2 does PR 3.4 (frontend) in parallel with 3.3. Merge, then PR 3.5.
- TB4: Session 1 does PR 4.1. Session 2 does PR 4.2 (frontend). Merge, then PR 4.3.
- TB5: Session 1 does PR 5.1. Session 2 does PR 5.2 (frontend). Merge, then PR 5.3.
- TB6: Session 1 does PR 6.1. Session 2 does PR 6.2 (docs). Merge, then PR 6.3.

**Phase 4: Epic 1 — Pulumi (parallel track)**

Pulumi can run as a completely independent session at any point after Epic 0. It touches only `infra/` and `Makefile` (deploy targets). It doesn't conflict with any feature work.

Session P (worktree: `pulumi`): PR 1.1 → 1.2. Merge to `main` whenever ready. Subsequent deploys happen after each TB merges.

### Handover Prompt Template

For each parallel session, the agent needs:

```markdown
# Task: [PR Number] — [PR Title]

## Context
You are working on the Apex Mobile Check Deposit system. 
The full PRD is attached. You are implementing one specific PR.

## Your Branch
git worktree: `[branch-name]`
Base: `main` (after [last merged PR])

## Files You Own
[List of directories/files this PR creates or modifies]

## Files You Must NOT Modify
[Everything else — especially shared files like routes.go]

## What To Build
[Copy the PR description from the sprint plan]

## Interfaces You Depend On (but don't implement)
[Go interfaces or API contracts from other PRs that you call or implement]

## Verification Criteria
[Copy the exact checklist from the sprint plan]
Run these checks. Every box must pass before you mark the PR ready.

## Key Decisions (from PRD)
[Relevant subset of decisions — e.g., for Ledger PR, include the 
double-entry model, movement_id grouping, reconciliation query]
```

The primary session (orchestrator) doesn't need handover prompts — it holds the full context and works sequentially.

### Estimated Session Count

| Phase | Primary Session | Parallel Sessions | Your Manual Work |
|-------|----------------|-------------------|------------------|
| Epic 0 | 1 session, ~4 PRs | 0 | Human verification |
| TB1 | 1 session, ~4 PRs | 4 sessions (one-shot each) | Merge 4 branches, human verification |
| TB2-TB5 | 1 session, ~12 PRs | 4 sessions (one per TB frontend) | Merge frontend branches, human verification × 4 |
| TB6 | 1 session, ~3 PRs | 1 session (docs) | Merge, final human verification |
| Pulumi | 0 | 1 session (independent) | Merge when ready |

**Total: 1 primary session (persistent) + ~10 parallel sessions (short-lived) + 6 human verification gates.**

### Context Window Management

The primary session will accumulate context over Epic 0 → TB1 → TB2 → ... → TB6. By TB4-TB5, the conversation history will be long. Mitigation:

1. **Start a new primary session at TB3.** The handover is: "Here's the PRD, here's the sprint plan, here's the repo state. Epics 0, TB1, TB2 are done. Continue from TB3." The new session gets a fresh context with the full PRD and sprint plan but no stale conversation history.

2. **Alternatively, use Claude Code's `/compact` or context management** to prune earlier conversation turns while keeping the PRD and sprint plan pinned.

My recommendation: start fresh at TB3 and again at TB5 if needed. Two context resets over the project. Each handover is cheap because the PRD and sprint plan are self-contained documents — the new session doesn't need the conversation history, just the current state of the repo and the plan.
