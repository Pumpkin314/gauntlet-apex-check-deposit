#!/usr/bin/env bash
#
# Generate test report for Apex Mobile Check Deposit.
# Runs Go unit tests and Playwright E2E tests, captures output,
# and generates /reports/TEST_REPORT.md summary.
#
# Usage: ./scripts/generate-report.sh
#
# Prerequisites: Docker Compose services running (make dev)

set -euo pipefail

REPORT_DIR="reports"
mkdir -p "$REPORT_DIR"

echo "=== Generating Test Report ==="
echo ""

# ---- 1. Go unit tests ----
echo "1. Running Go unit tests..."
if go test ./... -v > "$REPORT_DIR/unit-tests.txt" 2>&1; then
  UNIT_STATUS="PASS"
else
  UNIT_STATUS="FAIL"
fi
UNIT_TOTAL=$(grep -c "^--- " "$REPORT_DIR/unit-tests.txt" 2>/dev/null || echo "0")
UNIT_PASS=$(grep -c "^--- PASS" "$REPORT_DIR/unit-tests.txt" 2>/dev/null || echo "0")
UNIT_FAIL=$(grep -c "^--- FAIL" "$REPORT_DIR/unit-tests.txt" 2>/dev/null || echo "0")
echo "  Status: $UNIT_STATUS ($UNIT_PASS passed, $UNIT_FAIL failed, $UNIT_TOTAL total)"

# ---- 2. Playwright E2E tests ----
echo ""
echo "2. Running Playwright E2E tests..."
E2E_STATUS="SKIP"
E2E_PASS=0
E2E_FAIL=0
E2E_TOTAL=0

if [ -d "web" ] && [ -f "web/package.json" ]; then
  cd web
  if npx playwright test --reporter=html 2>"../$REPORT_DIR/e2e-stderr.txt" | tee "../$REPORT_DIR/e2e-output.txt"; then
    E2E_STATUS="PASS"
  else
    E2E_STATUS="FAIL"
  fi

  # Copy HTML report
  if [ -d "playwright-report" ]; then
    cp -r playwright-report "../$REPORT_DIR/playwright/"
  fi

  E2E_TOTAL=$(grep -cE "^\s+[0-9]+ (passed|failed)" "../$REPORT_DIR/e2e-output.txt" 2>/dev/null || echo "0")
  E2E_PASS=$(grep -oE "[0-9]+ passed" "../$REPORT_DIR/e2e-output.txt" 2>/dev/null | head -1 | grep -oE "[0-9]+" || echo "0")
  E2E_FAIL=$(grep -oE "[0-9]+ failed" "../$REPORT_DIR/e2e-output.txt" 2>/dev/null | head -1 | grep -oE "[0-9]+" || echo "0")
  cd ..
  echo "  Status: $E2E_STATUS ($E2E_PASS passed, $E2E_FAIL failed)"
else
  echo "  SKIPPED (web/ directory not found)"
fi

# ---- 3. Generate TEST_REPORT.md ----
echo ""
echo "3. Generating $REPORT_DIR/TEST_REPORT.md..."

cat > "$REPORT_DIR/TEST_REPORT.md" << EOF
# Test Report — Apex Mobile Check Deposit

Generated: $(date -u '+%Y-%m-%d %H:%M:%S UTC')

## Summary

| Suite | Status | Passed | Failed | Total |
|-------|--------|--------|--------|-------|
| Go Unit Tests | $UNIT_STATUS | $UNIT_PASS | $UNIT_FAIL | $UNIT_TOTAL |
| Playwright E2E | $E2E_STATUS | $E2E_PASS | $E2E_FAIL | $E2E_TOTAL |

## Go Unit Tests

Full output: [unit-tests.txt](unit-tests.txt)

### Test Packages

$(grep "^ok\|^FAIL" "$REPORT_DIR/unit-tests.txt" 2>/dev/null | sed 's/^/- /' || echo "- (no package results)")

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

- \`GET /health\` -> 200 OK
- \`GET /health/ledger\` -> reconciliation status
- \`GET /health/settlement\` -> unbatched transfer monitoring
EOF

echo "  Done: $REPORT_DIR/TEST_REPORT.md"
echo ""
echo "=== Report generation complete ==="
