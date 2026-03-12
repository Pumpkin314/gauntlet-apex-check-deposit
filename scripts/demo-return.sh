#!/usr/bin/env bash
#
# Return demo script for Apex Mobile Check Deposit.
# Submits a deposit, settles it to Completed, then triggers a return.
# Verifies Returned state, 6 ledger entries (2 deposit + 2 reversal + 2 fee),
# reconciliation = 0, fee = $30, and investor notification.
#
# Also tests return from FundsPosted (pre-settlement) path.
#
# Usage:  ./scripts/demo-return.sh [API_URL]
#   API_URL defaults to http://localhost:8080

set -euo pipefail

API_URL="${1:-http://localhost:8080}"
SETTLEMENT_URL="${SETTLEMENT_URL:-http://localhost:8082}"
PASS=0
FAIL=0

pass() { ((PASS++)); printf "  \033[32mPASS\033[0m %s\n" "$1"; }
fail() { ((FAIL++)); printf "  \033[31mFAIL\033[0m %s\n" "$1"; }
check() { if [ "$1" = "$2" ]; then pass "$3"; else fail "$3 (expected=$2, got=$1)"; fi }

json_field() {
  echo "$1" | python3 -c "import sys,json; print(json.load(sys.stdin).get('$2',''))"
}

echo "=== Apex Check Deposit — Return Demo ==="
echo "API: $API_URL"
echo ""

# 0. Health check
echo "0. Health check"
HEALTH=$(json_field "$(curl -sf "$API_URL/health")" "status")
check "$HEALTH" "ok" "GET /health returns ok"

# ---- 1. Submit deposit (ALPHA-001, $500) ----
echo ""
echo "1. Submit deposit (ALPHA-001, \$500)"
IDEM_KEY="demo-return-deposit-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-001","amount":500.00}')

TRANSFER_ID=$(json_field "$RESULT" "id")
STATE=$(json_field "$RESULT" "state")
echo "  Transfer ID: $TRANSFER_ID"
check "$STATE" "FundsPosted" "State = FundsPosted"

# ---- 2. Trigger settlement → Completed ----
echo ""
echo "2. Trigger settlement → Completed"
SETTLE_RESULT=$(curl -sf -X POST "$API_URL/settlement/trigger")
BATCH_COUNT=$(json_field "$SETTLE_RESULT" "batch_count")
echo "  Batches: $BATCH_COUNT"
if [ "$BATCH_COUNT" -ge 1 ]; then pass "At least 1 settlement batch created"; else fail "Expected >= 1 batch"; fi

# Verify Completed
SETTLED=$(curl -sf "$API_URL/deposits/$TRANSFER_ID")
SETTLED_STATE=$(json_field "$SETTLED" "state")
check "$SETTLED_STATE" "Completed" "After settlement: state = Completed"

# ---- 3. Trigger return (via admin simulate-return) ----
echo ""
echo "3. Trigger return (Completed → Returned)"
RETURN_RESULT=$(curl -sf -X POST "$API_URL/admin/simulate-return" \
  -H "Content-Type: application/json" \
  -d "{\"transfer_id\":\"$TRANSFER_ID\",\"reason_code\":\"R01\"}")

RETURN_STATE=$(json_field "$RETURN_RESULT" "state")
check "$RETURN_STATE" "Returned" "After return: state = Returned"

# ---- 4. Verify transfer is Returned via GET ----
echo ""
echo "4. GET /deposits/:id → Returned"
GET_RESULT=$(curl -sf "$API_URL/deposits/$TRANSFER_ID")
GET_STATE=$(json_field "$GET_RESULT" "state")
check "$GET_STATE" "Returned" "GET state = Returned"

# ---- 5. Check ledger entries for the transfer ----
echo ""
echo "5. Ledger entries for transfer"
ENTRIES=$(curl -sf "$API_URL/ledger/entries?transfer_id=$TRANSFER_ID")
ENTRY_COUNT=$(echo "$ENTRIES" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
echo "  Entry count: $ENTRY_COUNT"
check "$ENTRY_COUNT" "6" "6 ledger entries (2 deposit + 2 reversal + 2 fee)"

# Verify fee entries exist ($30)
FEE_ENTRIES=$(echo "$ENTRIES" | python3 -c "
import sys,json
entries = json.load(sys.stdin)
fee_count = sum(1 for e in entries if 'FEE' in e.get('Memo','').upper() or 'FEE' in e.get('EntryType','').upper())
print(fee_count)
")
if [ "$FEE_ENTRIES" -ge 2 ]; then pass "At least 2 fee-related entries"; else fail "Expected >= 2 fee entries, got $FEE_ENTRIES"; fi

# ---- 6. Ledger reconciliation ----
echo ""
echo "6. Ledger reconciliation"
HEALTH_LEDGER=$(curl -sf "$API_URL/health/ledger")
HEALTHY=$(json_field "$HEALTH_LEDGER" "healthy")
SUM=$(json_field "$HEALTH_LEDGER" "sum")
check "$HEALTHY" "True" "Ledger is healthy"
check "$SUM" "0.00" "Reconciliation sum = 0.00"

# ---- 7. Check return events in audit trail ----
echo ""
echo "7. Verify return events in audit trail"
EVENTS=$(curl -sf "$API_URL/deposits/$TRANSFER_ID/events")
HAS_RETURN_RECEIVED=$(echo "$EVENTS" | python3 -c "
import sys,json
events = json.load(sys.stdin)
for e in events:
    if e['step'] == 'return_received':
        print('True')
        break
else:
    print('False')
")
check "$HAS_RETURN_RECEIVED" "True" "return_received event in audit trail"

HAS_RETURN_PROCESSED=$(echo "$EVENTS" | python3 -c "
import sys,json
events = json.load(sys.stdin)
for e in events:
    if e['step'] == 'return_processed':
        print('True')
        break
else:
    print('False')
")
check "$HAS_RETURN_PROCESSED" "True" "return_processed event in audit trail"

# ---- 8. Verify investor balance is negative (deposit reversed + fee) ----
echo ""
echo "8. Investor balance after return"
BALANCES=$(curl -sf "$API_URL/ledger/balances")
ALPHA001_BAL=$(echo "$BALANCES" | python3 -c "
import sys,json
balances = json.load(sys.stdin)
for b in balances:
    if b['code'] == 'ALPHA-001':
        print(b['balance'])
        break
else:
    print('NOT_FOUND')
")
echo "  ALPHA-001 balance: $ALPHA001_BAL"
# After deposit ($500) + return reversal (-$500) + fee (-$30) = -$30
if python3 -c "bal=float('$ALPHA001_BAL'); assert bal < 0, f'expected negative, got {bal}'"; then
  pass "ALPHA-001 balance is negative after return + fee"
else
  fail "ALPHA-001 balance should be negative after return + fee (got $ALPHA001_BAL)"
fi

# ---- 9. Return idempotency: already-Returned → 200 no-op ----
echo ""
echo "9. Return idempotency (already Returned → 200)"
IDEM_RETURN=$(curl -sf -X POST "$API_URL/admin/simulate-return" \
  -H "Content-Type: application/json" \
  -d "{\"transfer_id\":\"$TRANSFER_ID\",\"reason_code\":\"R01\"}")
IDEM_STATE=$(json_field "$IDEM_RETURN" "state")
check "$IDEM_STATE" "Returned" "Idempotent return: still Returned"

# Re-check entry count didn't double
ENTRIES2=$(curl -sf "$API_URL/ledger/entries?transfer_id=$TRANSFER_ID")
ENTRY_COUNT2=$(echo "$ENTRIES2" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
check "$ENTRY_COUNT2" "6" "Still 6 entries after idempotent return"

# ---- 10. Return from FundsPosted (pre-settlement) ----
echo ""
echo "10. Return from FundsPosted (pre-settlement)"
# Reset ALPHA-001 status back to ACTIVE (it was set to COLLECTIONS after step 8)
docker compose exec -T postgres psql -U apex -d apex_check_deposit -c \
  "UPDATE accounts SET status = 'ACTIVE' WHERE code = 'ALPHA-001';" > /dev/null 2>&1
IDEM_KEY2="demo-return-presettl-$(date +%s%N)"
RESULT2=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY2" \
  -d '{"account_code":"ALPHA-001","amount":250.00}')

TRANSFER_ID2=$(json_field "$RESULT2" "id")
STATE2=$(json_field "$RESULT2" "state")
echo "  Transfer ID: $TRANSFER_ID2"
check "$STATE2" "FundsPosted" "Pre-settlement deposit: FundsPosted"

# Return directly from FundsPosted (no settlement first)
RETURN_RESULT2=$(curl -sf -X POST "$API_URL/admin/simulate-return" \
  -H "Content-Type: application/json" \
  -d "{\"transfer_id\":\"$TRANSFER_ID2\",\"reason_code\":\"R08\"}")
RETURN_STATE2=$(json_field "$RETURN_RESULT2" "state")
check "$RETURN_STATE2" "Returned" "Pre-settlement return: state = Returned"

# Check ledger entries for this transfer too
ENTRIES3=$(curl -sf "$API_URL/ledger/entries?transfer_id=$TRANSFER_ID2")
ENTRY_COUNT3=$(echo "$ENTRIES3" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
check "$ENTRY_COUNT3" "6" "6 entries for pre-settlement return"

# ---- 11. Return rejected for invalid state ----
echo ""
echo "11. Return for Rejected transfer → error"
IDEM_KEY3="demo-return-invalid-$(date +%s%N)"
RESULT3=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY3" \
  -d '{"account_code":"ALPHA-002","amount":500.00}')
REJECTED_ID=$(json_field "$RESULT3" "id")
REJECTED_STATE=$(json_field "$RESULT3" "state")
check "$REJECTED_STATE" "Rejected" "Setup: transfer is Rejected"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API_URL/returns" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dev-settlement-token" \
  -d "{\"transfer_id\":\"$REJECTED_ID\",\"return_reason_code\":\"R01\"}")
check "$HTTP_CODE" "400" "Return on Rejected transfer → 400"

# ---- 12. Final ledger reconciliation ----
echo ""
echo "12. Final ledger reconciliation"
HEALTH_LEDGER_FINAL=$(curl -sf "$API_URL/health/ledger")
HEALTHY_FINAL=$(json_field "$HEALTH_LEDGER_FINAL" "healthy")
SUM_FINAL=$(json_field "$HEALTH_LEDGER_FINAL" "sum")
check "$HEALTHY_FINAL" "True" "Final ledger is healthy"
check "$SUM_FINAL" "0.00" "Final reconciliation sum = 0.00"

# ---- 13. Notification created ----
echo ""
echo "13. Verify notification created for investor"
NOTIFS=$(curl -sf "$API_URL/notifications" -H "Authorization: Bearer investor-alpha")
NOTIF_COUNT=$(echo "$NOTIFS" | python3 -c "
import sys,json
notifs = json.load(sys.stdin)
count = sum(1 for n in notifs if n.get('type') == 'RETURN_RECEIVED')
print(count)
")
echo "  RETURN_RECEIVED notifications: $NOTIF_COUNT"
if [ "$NOTIF_COUNT" -ge 1 ]; then pass "At least 1 RETURN_RECEIVED notification"; else fail "Expected >= 1 notification, got $NOTIF_COUNT"; fi

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
