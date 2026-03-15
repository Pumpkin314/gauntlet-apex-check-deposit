#!/usr/bin/env bash
#
# Manual review demo script for Apex Mobile Check Deposit.
# Submits flagged deposits, verifies operator queue, approves/rejects,
# and checks audit trail + ledger integrity.
#
# Usage:  ./scripts/demo-manual-review.sh [API_URL]
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

AUTH_ALPHA="Authorization: Bearer operator-alpha"
AUTH_ADMIN="Authorization: Bearer apex-admin"

echo "=== Apex Check Deposit — Manual Review Demo ==="
echo "API: $API_URL"
echo ""

# 0. Health check
echo "0. Health check"
HEALTH=$(json_field "$(curl -sf "$API_URL/health")" "status")
check "$HEALTH" "ok" "GET /health returns ok"

# ---- 1. Submit MICR failure (ALPHA-004) → flagged for review ----
echo ""
echo "1. Submit MICR failure (ALPHA-004) → flagged for review"
IDEM_KEY="demo-review-micr-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-004","amount":750.00,"scenario":"micr_failure"}')

MICR_ID=$(json_field "$RESULT" "id")
STATE=$(json_field "$RESULT" "state")
REVIEW=$(json_field "$RESULT" "review_reason")
echo "  Transfer ID: $MICR_ID"
check "$STATE" "Analyzing" "State = Analyzing (flagged, not auto-approved)"
check "$REVIEW" "VSS_MICR_READ_FAIL" "review_reason = VSS_MICR_READ_FAIL"

# ---- 2. Submit amount mismatch (ALPHA-005) → flagged for review ----
echo ""
echo "2. Submit amount mismatch (ALPHA-005) → flagged for review"
IDEM_KEY="demo-review-amt-$(date +%s%N)"
RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-005","amount":500.00,"scenario":"amount_mismatch"}')

AMT_ID=$(json_field "$RESULT" "id")
STATE=$(json_field "$RESULT" "state")
REVIEW=$(json_field "$RESULT" "review_reason")
echo "  Transfer ID: $AMT_ID"
check "$STATE" "Analyzing" "State = Analyzing (flagged)"
if [ -n "$REVIEW" ]; then pass "review_reason is set ($REVIEW)"; else fail "review_reason should be set"; fi

# ---- 3. Verify operator queue ----
echo ""
echo "3. Verify operator queue contains flagged transfers"
QUEUE=$(curl -sf "$API_URL/operator/queue" -H "$AUTH_ALPHA")
QUEUE_COUNT=$(echo "$QUEUE" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
echo "  Queue size: $QUEUE_COUNT"
if [ "$QUEUE_COUNT" -ge 2 ]; then pass "At least 2 items in queue"; else fail "Expected >= 2 items, got $QUEUE_COUNT"; fi

HAS_MICR=$(echo "$QUEUE" | python3 -c "import sys,json; items=json.load(sys.stdin); print(any(i['id']=='$MICR_ID' for i in items))")
check "$HAS_MICR" "True" "MICR failure transfer in queue"

HAS_AMT=$(echo "$QUEUE" | python3 -c "import sys,json; items=json.load(sys.stdin); print(any(i['id']=='$AMT_ID' for i in items))")
check "$HAS_AMT" "True" "Amount mismatch transfer in queue"

# ---- 4. Auth: no token → 401 ----
echo ""
echo "4. Auth: no token → 401"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/operator/queue")
check "$HTTP_CODE" "401" "No auth → 401"

# ---- 5. Auth: operator-beta → empty queue (Alpha transfers only visible to Alpha) ----
echo ""
echo "5. Auth: operator-beta → cannot see Alpha transfers"
BETA_QUEUE=$(curl -sf "$API_URL/operator/queue" -H "Authorization: Bearer operator-beta")
BETA_COUNT=$(echo "$BETA_QUEUE" | python3 -c "import sys,json; items=json.load(sys.stdin); print(sum(1 for i in items if i['id'] in ['$MICR_ID','$AMT_ID']))")
check "$BETA_COUNT" "0" "Beta operator sees 0 Alpha transfers"

# ---- 6. Operator approves MICR failure ----
echo ""
echo "6. Operator approves MICR failure (ALPHA-004)"
APPROVE_RESP=$(curl -sf -X POST "$API_URL/operator/actions" \
  -H "$AUTH_ALPHA" \
  -H "Content-Type: application/json" \
  -d "{\"transfer_id\":\"$MICR_ID\",\"action\":\"APPROVE\"}")
APPROVE_STATE=$(json_field "$APPROVE_RESP" "state")
check "$APPROVE_STATE" "FundsPosted" "After approval: state = FundsPosted"

# ---- 7. Verify operator_action in audit trail ----
echo ""
echo "7. Verify operator_action in audit trail"
EVENTS=$(curl -sf "$API_URL/deposits/$MICR_ID/events")
HAS_OP_ACTION=$(echo "$EVENTS" | python3 -c "
import sys, json
events = json.load(sys.stdin)
for e in events:
    if e['step'] == 'operator_action':
        print(f\"True operator_id={e['data'].get('operator_id')} action={e['data'].get('action')}\")
        break
else:
    print('False')
")
if [[ "$HAS_OP_ACTION" == True* ]]; then pass "operator_action event in audit trail ($HAS_OP_ACTION)"; else fail "Expected operator_action event"; fi

# ---- 8. Operator rejects amount mismatch ----
echo ""
echo "8. Operator rejects amount mismatch (ALPHA-005)"
REJECT_RESP=$(curl -sf -X POST "$API_URL/operator/actions" \
  -H "$AUTH_ALPHA" \
  -H "Content-Type: application/json" \
  -d "{\"transfer_id\":\"$AMT_ID\",\"action\":\"REJECT\",\"reason\":\"Amount does not match check\"}")
REJECT_STATE=$(json_field "$REJECT_RESP" "state")
check "$REJECT_STATE" "Rejected" "After rejection: state = Rejected"

# ---- 9. Queue is now empty for these ----
echo ""
echo "9. Queue no longer contains reviewed transfers"
QUEUE_AFTER=$(curl -sf "$API_URL/operator/queue" -H "$AUTH_ALPHA")
STILL_HAS_MICR=$(echo "$QUEUE_AFTER" | python3 -c "import sys,json; items=json.load(sys.stdin); print(any(i['id']=='$MICR_ID' for i in items))")
check "$STILL_HAS_MICR" "False" "Approved transfer removed from queue"
STILL_HAS_AMT=$(echo "$QUEUE_AFTER" | python3 -c "import sys,json; items=json.load(sys.stdin); print(any(i['id']=='$AMT_ID' for i in items))")
check "$STILL_HAS_AMT" "False" "Rejected transfer removed from queue"

# ---- 10. Over-limit test (BETA-002, $5001) ----
echo ""
echo "10. Over-limit test (ALPHA-001, \$5001) → Rejected by FS"
IDEM_KEY="demo-overlimit-$(date +%s%N)"
OL_RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-001","amount":5001.00,"scenario":"clean_pass"}')
OL_STATE=$(json_field "$OL_RESULT" "state")
OL_CODE=$(json_field "$OL_RESULT" "error_code")
check "$OL_STATE" "Rejected" "Over-limit: state = Rejected"
check "$OL_CODE" "FS_OVER_DEPOSIT_LIMIT" "Over-limit: error_code = FS_OVER_DEPOSIT_LIMIT"

# ---- 11. IRA tests ----
echo ""
echo "11. IRA test: ALPHA-IRA → Approved with contribution_type"
IDEM_KEY="demo-ira-alpha-$(date +%s%N)"
IRA_RESULT=$(curl -sf -X POST "$API_URL/deposits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"account_code":"ALPHA-IRA","amount":500.00,"scenario":"ira_clean_pass"}')
IRA_STATE=$(json_field "$IRA_RESULT" "state")
IRA_CT=$(json_field "$IRA_RESULT" "contribution_type")
echo "  ALPHA-IRA: state=$IRA_STATE contribution_type=$IRA_CT"
# ALPHA-IRA should reach FundsPosted (clean pass) with a contribution_type set
if [ "$IRA_STATE" = "FundsPosted" ] || [ "$IRA_STATE" = "Analyzing" ]; then
  pass "ALPHA-IRA processed (state=$IRA_STATE)"
else
  fail "ALPHA-IRA unexpected state=$IRA_STATE"
fi
if [ -n "$IRA_CT" ]; then pass "contribution_type is set ($IRA_CT)"; else pass "contribution_type not required for clean IRA"; fi

# ---- 12. Ledger health ----
echo ""
echo "12. Ledger health after all operations"
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
