#!/usr/bin/env bash
#
# Rejection-path demo script for Apex Mobile Check Deposit.
# Exercises blur, glare, duplicate, and over-limit rejection paths.
# Each sub-test asserts expected state (Rejected) and expected error code.
#
# Usage:  ./scripts/demo-rejection.sh [API_URL]
#   API_URL defaults to http://localhost:8080

set -euo pipefail

API_URL="${1:-http://localhost:8080}"
PASS=0
FAIL=0

pass() { ((PASS++)); printf "  \033[32mPASS\033[0m %s\n" "$1"; }
fail() { ((FAIL++)); printf "  \033[31mFAIL\033[0m %s\n" "$1"; }
check() { if [ "$1" = "$2" ]; then pass "$3"; else fail "$3 (expected=$2, got=$1)"; fi }

json_field() {
  echo "$1" | python3 -c "import sys,json; print(json.load(sys.stdin).get('$2',''))"
}

echo "=== Apex Check Deposit — Rejection Path Demo ==="
echo "API: $API_URL"
echo ""

# 0. Health check
echo "0. Health check"
HEALTH=$(json_field "$(curl -sf "$API_URL/health")" "status")
check "$HEALTH" "ok" "GET /health returns ok"

# ---- 1. IQA Blur (ALPHA-002) ----
echo ""
echo "1. IQA Blur rejection (ALPHA-002)"
IDEM_KEY="demo-blur-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-002","amount":500.00}')

STATE=$(json_field "$RESULT" "state")
ERROR_CODE=$(json_field "$RESULT" "error_code")
USER_MSG=$(json_field "$RESULT" "user_message")
TRANSFER_ID=$(json_field "$RESULT" "id")
echo "  Transfer ID: $TRANSFER_ID"
check "$STATE" "Rejected" "State = Rejected"
check "$ERROR_CODE" "VSS_IQA_BLUR" "error_code = VSS_IQA_BLUR"
if [ -n "$USER_MSG" ]; then pass "user_message is present"; else fail "user_message should be present"; fi

# Verify no ledger entries
EVENTS=$(curl -sf "$API_URL/deposits/$TRANSFER_ID/events")
HAS_LEDGER=$(echo "$EVENTS" | python3 -c "import sys,json; evts=json.load(sys.stdin); print(any(e['step']=='ledger_posted' for e in evts))")
check "$HAS_LEDGER" "False" "No ledger_posted event for blur rejection"

# ---- 2. IQA Glare (ALPHA-003) ----
echo ""
echo "2. IQA Glare rejection (ALPHA-003)"
IDEM_KEY="demo-glare-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-003","amount":500.00}')

STATE=$(json_field "$RESULT" "state")
ERROR_CODE=$(json_field "$RESULT" "error_code")
USER_MSG=$(json_field "$RESULT" "user_message")
TRANSFER_ID=$(json_field "$RESULT" "id")
echo "  Transfer ID: $TRANSFER_ID"
check "$STATE" "Rejected" "State = Rejected"
check "$ERROR_CODE" "VSS_IQA_GLARE" "error_code = VSS_IQA_GLARE"
if [ -n "$USER_MSG" ]; then pass "user_message is present"; else fail "user_message should be present"; fi

# ---- 3. Duplicate (BETA-001) ----
echo ""
echo "3. Duplicate rejection (BETA-001)"
IDEM_KEY="demo-dup-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"BETA-001","amount":500.00}')

STATE=$(json_field "$RESULT" "state")
ERROR_CODE=$(json_field "$RESULT" "error_code")
USER_MSG=$(json_field "$RESULT" "user_message")
TRANSFER_ID=$(json_field "$RESULT" "id")
echo "  Transfer ID: $TRANSFER_ID"
check "$STATE" "Rejected" "State = Rejected"
check "$ERROR_CODE" "VSS_DUPLICATE_DETECTED" "error_code = VSS_DUPLICATE_DETECTED"
if [ -n "$USER_MSG" ]; then pass "user_message is present"; else fail "user_message should be present"; fi

# ---- 4. Over Deposit Limit (any account, $5001) ----
echo ""
echo "4. Over deposit limit (\$5001)"
IDEM_KEY="demo-overlimit-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-001","amount":5001.00}')

STATE=$(json_field "$RESULT" "state")
ERROR_CODE=$(json_field "$RESULT" "error_code")
TRANSFER_ID=$(json_field "$RESULT" "id")
echo "  Transfer ID: $TRANSFER_ID"
check "$STATE" "Rejected" "State = Rejected"
check "$ERROR_CODE" "FS_OVER_DEPOSIT_LIMIT" "error_code = FS_OVER_DEPOSIT_LIMIT"

# ---- 5. Verify GET /deposits/:id includes error_code + user_message ----
echo ""
echo "5. GET /deposits/:id for rejected transfer"
GET_RESULT=$(curl -sf "$API_URL/deposits/$TRANSFER_ID")
GET_STATE=$(json_field "$GET_RESULT" "state")
GET_ERROR=$(json_field "$GET_RESULT" "error_code")
GET_USER_MSG=$(json_field "$GET_RESULT" "user_message")
check "$GET_STATE" "Rejected" "GET state = Rejected"
check "$GET_ERROR" "FS_OVER_DEPOSIT_LIMIT" "GET error_code = FS_OVER_DEPOSIT_LIMIT"
if [ -n "$GET_USER_MSG" ]; then pass "GET user_message is present"; else fail "GET user_message should be present"; fi

# ---- 6. Ledger health unchanged ----
echo ""
echo "6. Ledger health after rejections"
HEALTH_LEDGER=$(curl -sf "$API_URL/health/ledger")
HEALTHY=$(json_field "$HEALTH_LEDGER" "healthy")
SUM=$(json_field "$HEALTH_LEDGER" "sum")
check "$HEALTHY" "True" "Ledger is healthy"
check "$SUM" "0.00" "Reconciliation sum = 0.00"

# Summary
echo ""
echo "=== Results ==="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo ""
if [ "$FAIL" -eq 0 ]; then
  printf "\033[32mAll assertions passed!\033[0m\n"
  exit 0
else
  printf "\033[31m$FAIL assertion(s) failed!\033[0m\n"
  exit 1
fi
