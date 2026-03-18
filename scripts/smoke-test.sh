#!/usr/bin/env bash
# scripts/smoke-test.sh
# ──────────────────────────────────────────────────────────────
# End-to-end smoke test for AgentGuard.
# Verifies the full stack works with one command.

set -uo pipefail

# Helper to log headers for debugging
log_headers() { :; } # echo "DEBUG HEADERS:"; echo "$1"; }

GUARD_URL="${GUARD_URL:-http://localhost:7777}"
PASS=0
FAIL=0

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0 ;33m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓${NC} $1"; PASS=$((PASS+1)); }
fail() { echo -e "  ${RED}✗${NC} $1"; FAIL=$((FAIL+1)); }
info() { echo -e "  ${YELLOW}→${NC} $1"; }

echo ""
echo "AgentGuard Smoke Test"
echo "Target: $GUARD_URL"
echo "───────────────────────────────────────"

# 1. Health check
echo ""
echo "1. Health check"
HTTP=$(curl -s -o /tmp/ag_health.json -w "%{http_code}" "$GUARD_URL/health" || echo "000")
if [[ "$HTTP" == "200" ]]; then
  STATUS=$(jq -r '.status' /tmp/ag_health.json 2>/dev/null || echo "")
  if [[ "$STATUS" == "ok" ]]; then
    ok "GET /health → 200 status=ok"
  else
    fail "GET /health → 200 but status=$STATUS"
  fi
else
  fail "GET /health → HTTP $HTTP"
fi

# 2. MCP allow
echo ""
echo "2. MCP allow decision (read_file)"
# Get headers using -D -
RAW_HEADERS=$(curl -sD - -o /dev/null -X POST "$GUARD_URL" \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: smoke-test-agent" \
  -d '{"jsonrpc":"2.0","id":"smoke-allow","method":"tools/call","params":{"name":"read_file","arguments":{}}}')
DECISION=$(echo "$RAW_HEADERS" | grep -i "X-AgentGuard-Decision" | head -n1 | awk -F': ' '{print $2}' | tr -d '\r\n ' || echo "")

if [[ "$DECISION" == "allow" ]]; then
  ok "read_file → decision=allow"
else
  fail "read_file → decision='$DECISION' (expected allow)"
fi

# 3. MCP block
echo ""
echo "3. MCP block decision (delete_customer)"
RAW_HEADERS=$(curl -sD - -o /dev/null -X POST "$GUARD_URL" \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: smoke-test-agent" \
  -d '{"jsonrpc":"2.0","id":"smoke-block","method":"tools/call","params":{"name":"delete_customer","arguments":{"id":"123"}}}')
DECISION=$(echo "$RAW_HEADERS" | grep -i "X-AgentGuard-Decision" | head -n1 | awk -F': ' '{print $2}' | tr -d '\r\n ' || echo "")

if [[ "$DECISION" == "block" ]]; then
  ok "delete_customer → decision=block"
else
  fail "delete_customer → decision='$DECISION' (expected block)"
fi

# 4. Escalate
echo ""
echo "4. Escalation (send_payment)"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$GUARD_URL" \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: smoke-test-agent" \
  -d '{"jsonrpc":"2.0","id":"smoke-esc","method":"tools/call","params":{"name":"send_payment","arguments":{"amount":500}}}')

RAW_HEADERS=$(curl -sD - -o /dev/null -X POST "$GUARD_URL" \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: smoke-test-agent" \
  -d '{"jsonrpc":"2.0","id":"smoke-esc-2","method":"tools/call","params":{"name":"send_payment","arguments":{"amount":500}}}')
ESC_ID=$(echo "$RAW_HEADERS" | grep -i "X-Escalation-ID" | head -n1 | awk -F': ' '{print $2}' | tr -d '\r\n ' || echo "")

if [[ "$HTTP" == "202" ]]; then
  ok "send_payment → HTTP 202 (escalated)"
else
  fail "send_payment → HTTP $HTTP (expected 202)"
fi
if [[ -n "$ESC_ID" ]]; then
  ok "X-Escalation-ID: $ESC_ID"
else
  fail "X-Escalation-ID header missing"
fi

# 5. Status
echo ""
echo "5. Status endpoint"
HTTP=$(curl -s -o /tmp/ag_status.json -w "%{http_code}" "$GUARD_URL/api/v1/status" \
  -H "X-AgentGuard-Admin-Key: test_admin_key_32chars_minimum00")
if [[ "$HTTP" == "200" ]]; then
  ok "GET /api/v1/status → 200"
else
  fail "GET /api/v1/status → HTTP $HTTP"
fi

echo ""
echo "───────────────────────────────────────"
if [[ $FAIL -eq 0 ]]; then
  echo -e "${GREEN}✓ All smoke tests passed.${NC}"
  exit 0
else
  echo -e "${RED}✗ $FAIL test(s) failed.${NC}"
  exit 1
fi
