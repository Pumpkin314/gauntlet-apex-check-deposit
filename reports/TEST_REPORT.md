# Test Report — Apex Mobile Check Deposit

Generated: 2026-03-12 08:34:10 UTC

## Summary

| Suite | Status | Passed | Failed | Total |
|-------|--------|--------|--------|-------|
| Go Unit Tests | PASS | 94 | 0
0 | 94 |
| Playwright E2E | PASS | 29 | 0 | 0
0 |

## Go Unit Tests

Full output: [unit-tests.txt](unit-tests.txt)

### Test Packages

- ok  	github.com/apex-checkout/check-deposit/cmd/api/middleware	(cached)
- ok  	github.com/apex-checkout/check-deposit/cmd/vendor-stub	(cached)
- ok  	github.com/apex-checkout/check-deposit/internal/funding	(cached)
- ok  	github.com/apex-checkout/check-deposit/internal/ledger	(cached)
- ok  	github.com/apex-checkout/check-deposit/internal/logging	(cached)
- ok  	github.com/apex-checkout/check-deposit/internal/orchestrator	(cached)
- ok  	github.com/apex-checkout/check-deposit/internal/returns	(cached)
- ok  	github.com/apex-checkout/check-deposit/internal/settlement	(cached)

## Playwright E2E Tests

Full output: [e2e-output.txt](e2e-output.txt)
HTML report: [playwright/index.html](playwright/index.html)

## Demo Scripts

| Script | Description |
|--------|-------------|
| demo-happy-path.sh | Full lifecycle: submit, validate, approve, post, settle, complete |
| demo-rejection.sh | Blur, glare, duplicate, over-limit rejection paths |
| demo-manual-review.sh | MICR failure -> operator queue -> approve/reject |
| demo-return.sh | Completed -> return -> reversal + fee + notification |

## Health Checks

- `GET /health` -> 200 OK
- `GET /health/ledger` -> reconciliation status
- `GET /health/settlement` -> unbatched transfer monitoring
