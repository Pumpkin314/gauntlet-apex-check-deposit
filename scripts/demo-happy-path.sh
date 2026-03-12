#!/usr/bin/env bash
#
# Happy-path demo script for Apex Mobile Check Deposit.
# Submits a deposit, polls for FundsPosted, checks ledger balances and reconciliation.
#
# Usage:  ./scripts/demo-happy-path.sh [API_URL]
#   API_URL defaults to http://localhost:8080

set -euo pipefail

API_URL="${1:-http://localhost:8080}"
PASS=0
FAIL=0

pass() { ((PASS++)); printf "  \033[32mPASS\033[0m %s\n" "$1"; }
fail() { ((FAIL++)); printf "  \033[31mFAIL\033[0m %s\n" "$1"; }
check() { if [ "$1" = "$2" ]; then pass "$3"; else fail "$3 (expected=$2, got=$1)"; fi }

echo "=== Apex Check Deposit — Happy Path Demo ==="
echo "API: $API_URL"
echo ""

# 1. Health check
echo "1. Health check"
HEALTH=$(curl -sf "$API_URL/health" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
check "$HEALTH" "ok" "GET /health returns ok"

# 2. Reset state — use a unique idempotency key
IDEM_KEY="demo-happy-$(date +%s%N)"

# 3. Submit deposit (Clean Pass — ALPHA-001, $500)
echo ""
echo "2. Submit deposit (ALPHA-001, \$500)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-001","amount":500.00}')

TRANSFER_ID=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
STATE=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['state'])")
TYPE=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['type'])")
MEMO=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['memo'])")
SUB_TYPE=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['sub_type'])")
TRANSFER_TYPE=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['transfer_type'])")
CURRENCY=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['currency'])")

echo "  Transfer ID: $TRANSFER_ID"
check "$STATE" "FundsPosted" "Final state is FundsPosted"
check "$TYPE" "MOVEMENT" "type = MOVEMENT"
check "$MEMO" "FREE" "memo = FREE"
check "$SUB_TYPE" "DEPOSIT" "sub_type = DEPOSIT"
check "$TRANSFER_TYPE" "CHECK" "transfer_type = CHECK"
check "$CURRENCY" "USD" "currency = USD"

# 4. GET /deposits/:id
echo ""
echo "3. GET /deposits/:id"
GET_RESULT=$(curl -sf "$API_URL/deposits/$TRANSFER_ID")
GET_STATE=$(echo "$GET_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['state'])")
check "$GET_STATE" "FundsPosted" "GET returns FundsPosted"

# 5. Check events
echo ""
echo "4. Check transfer events"
EVENTS=$(curl -sf "$API_URL/deposits/$TRANSFER_ID/events")
EVENT_COUNT=$(echo "$EVENTS" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
echo "  Event count: $EVENT_COUNT"
if [ "$EVENT_COUNT" -ge 8 ]; then pass "At least 8 events recorded"; else fail "Expected >= 8 events, got $EVENT_COUNT"; fi

STEPS=$(echo "$EVENTS" | python3 -c "import sys,json; steps=[e['step'] for e in json.load(sys.stdin)]; print(','.join(steps))")
echo "  Steps: $STEPS"

# 6. Check ledger balances
echo ""
echo "5. Check ledger balances"
BALANCES=$(curl -sf "$API_URL/ledger/balances")

ALPHA001_BAL=$(echo "$BALANCES" | python3 -c "
import sys,json
balances = json.load(sys.stdin)
for b in balances:
    if b['code'] == 'ALPHA-001':
        print(b['balance'])
        break
")
echo "  ALPHA-001 balance: $ALPHA001_BAL"
if python3 -c "assert float('$ALPHA001_BAL') > 0"; then pass "ALPHA-001 has positive balance"; else fail "ALPHA-001 balance should be positive"; fi

OMNIBUS_BAL=$(echo "$BALANCES" | python3 -c "
import sys,json
balances = json.load(sys.stdin)
for b in balances:
    if b['code'] == 'OMNIBUS-ALPHA':
        print(b['balance'])
        break
")
echo "  OMNIBUS-ALPHA balance: $OMNIBUS_BAL"
if python3 -c "assert float('$OMNIBUS_BAL') < 0"; then pass "OMNIBUS-ALPHA has negative balance"; else fail "OMNIBUS-ALPHA balance should be negative"; fi

# 7. Reconciliation
echo ""
echo "6. Ledger reconciliation"
HEALTH_LEDGER=$(curl -sf "$API_URL/health/ledger")
HEALTHY=$(echo "$HEALTH_LEDGER" | python3 -c "import sys,json; print(json.load(sys.stdin)['healthy'])")
SUM=$(echo "$HEALTH_LEDGER" | python3 -c "import sys,json; print(json.load(sys.stdin)['sum'])")
check "$HEALTHY" "True" "Ledger is healthy"
check "$SUM" "0.00" "Reconciliation sum = 0.00"

# 8. Idempotency test
echo ""
echo "7. Idempotency test"
IDEM_RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-001","amount":500.00}')
IDEM_ID=$(echo "$IDEM_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
check "$IDEM_ID" "$TRANSFER_ID" "Same Idempotency-Key returns same transfer ID"

# ---- 8. Trigger Settlement ----
echo ""
echo "8. Trigger settlement"
SETTLE_RESULT=$(curl -sf -X POST "$API_URL/settlement/trigger")
BATCH_COUNT=$(echo "$SETTLE_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('batch_count', 0))")
TOTAL_CHECKS=$(echo "$SETTLE_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_checks', 0))")
TOTAL_AMOUNT=$(echo "$SETTLE_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_amount', 0))")
echo "  Batches: $BATCH_COUNT, Checks: $TOTAL_CHECKS, Amount: $TOTAL_AMOUNT"
if [ "$BATCH_COUNT" -ge 1 ]; then pass "At least 1 settlement batch created"; else fail "Expected >= 1 batch, got $BATCH_COUNT"; fi
if [ "$TOTAL_CHECKS" -ge 1 ]; then pass "At least 1 check in settlement"; else fail "Expected >= 1 check, got $TOTAL_CHECKS"; fi

# ---- 9. Verify transfer is now Completed ----
echo ""
echo "9. Verify transfer reached Completed"
SETTLED=$(curl -sf "$API_URL/deposits/$TRANSFER_ID")
SETTLED_STATE=$(echo "$SETTLED" | python3 -c "import sys,json; print(json.load(sys.stdin)['state'])")
check "$SETTLED_STATE" "Completed" "After settlement: state = Completed"

# ---- 10. Verify settlement events in audit trail ----
echo ""
echo "10. Verify settlement events in audit trail"
EVENTS_POST=$(curl -sf "$API_URL/deposits/$TRANSFER_ID/events")
HAS_SETTLE=$(echo "$EVENTS_POST" | python3 -c "
import sys, json
events = json.load(sys.stdin)
for e in events:
    if e['step'] == 'settlement_completed':
        print('True')
        break
else:
    print('False')
")
check "$HAS_SETTLE" "True" "settlement_completed event in audit trail"

# ---- 11. GET /settlement/status ----
echo ""
echo "11. Settlement status"
SETTLE_STATUS=$(curl -sf "$API_URL/settlement/status")
TOTAL_BATCHES=$(echo "$SETTLE_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_batches', 0))")
echo "  Total batches: $TOTAL_BATCHES"
if [ "$TOTAL_BATCHES" -ge 1 ]; then pass "At least 1 batch in status"; else fail "Expected >= 1 batch in status"; fi

# ---- 12. GET /settlement/batches ----
echo ""
echo "12. Settlement batches list"
BATCHES=$(curl -sf "$API_URL/settlement/batches")
BATCH_LIST_COUNT=$(echo "$BATCHES" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
echo "  Batches listed: $BATCH_LIST_COUNT"
if [ "$BATCH_LIST_COUNT" -ge 1 ]; then pass "Batch list has entries"; else fail "Expected >= 1 batch in list"; fi

FIRST_STATUS=$(echo "$BATCHES" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['status'])")
check "$FIRST_STATUS" "ACKNOWLEDGED" "Batch status = ACKNOWLEDGED"

# ---- 13. Ledger still reconciles after settlement ----
echo ""
echo "13. Ledger reconciliation after settlement"
HEALTH_LEDGER2=$(curl -sf "$API_URL/health/ledger")
HEALTHY2=$(echo "$HEALTH_LEDGER2" | python3 -c "import sys,json; print(json.load(sys.stdin)['healthy'])")
SUM2=$(echo "$HEALTH_LEDGER2" | python3 -c "import sys,json; print(json.load(sys.stdin)['sum'])")
check "$HEALTHY2" "True" "Ledger still healthy after settlement"
check "$SUM2" "0.00" "Reconciliation still = 0.00"

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
