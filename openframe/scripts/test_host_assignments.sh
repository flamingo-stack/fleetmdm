#!/bin/bash
#
# Test script for policy/query host assignment endpoints (openframe mode).
# Requires: FLEET_URL, FLEET_TOKEN, and at least one policy + two hosts.
#
# Usage:
#   export FLEET_URL=https://localhost:8080
#   export FLEET_TOKEN=<api-token>
#   ./test_host_assignments.sh [--policy-id ID] [--query-id ID] [--host-ids "1,2,3"] [--no-cleanup] [--verbose]
#
set -euo pipefail

# ---------- defaults ----------
FLEET_URL="${FLEET_URL:-https://localhost:8080}"
FLEET_TOKEN="${FLEET_TOKEN:-}"
POLICY_ID=""
QUERY_ID=""
HOST_IDS=""
CURL_OPTS="-sk"
NO_CLEANUP=false
VERBOSE=false

# ---------- colors ----------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
DIM='\033[2m'
NC='\033[0m'

pass=0
fail=0

# ---------- parse args ----------
while [[ $# -gt 0 ]]; do
  case $1 in
    --policy-id)  POLICY_ID="$2"; shift 2;;
    --query-id)   QUERY_ID="$2";  shift 2;;
    --host-ids)   HOST_IDS="$2";  shift 2;;
    --url)        FLEET_URL="$2"; shift 2;;
    --token)      FLEET_TOKEN="$2"; shift 2;;
    --no-cleanup) NO_CLEANUP=true; shift;;
    --verbose)    VERBOSE=true; shift;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

if [[ -z "$FLEET_TOKEN" ]]; then
  echo "Error: FLEET_TOKEN is required (export or --token)"
  exit 1
fi

AUTH="Authorization: Bearer ${FLEET_TOKEN}"
CT="Content-Type: application/json"

# ---------- helpers ----------
log_request() {
  local method="$1" path="$2" data="${3:-}"
  if [[ "$VERBOSE" == true ]]; then
    echo -e "  ${CYAN}>>> ${method} ${path}${NC}" >&2
    if [[ -n "$data" ]]; then
      echo -e "  ${DIM}    Body: $(echo "$data" | jq -c . 2>/dev/null || echo "$data")${NC}" >&2
    fi
  fi
}

log_response() {
  local status="$1" body="$2"
  if [[ "$VERBOSE" == true ]]; then
    echo -e "  ${CYAN}<<< HTTP ${status}${NC}" >&2
    echo -e "  ${DIM}    Body: $(echo "$body" | jq -c . 2>/dev/null || echo "$body")${NC}" >&2
  fi
}

api() {
  local method="$1" path="$2"
  shift 2
  curl $CURL_OPTS -X "$method" \
    -H "$AUTH" -H "$CT" \
    "${FLEET_URL}${path}" "$@" 2>/dev/null
}

# api call with logging: returns "body\nstatus"
api_log() {
  local method="$1" path="$2" data="${3:-}"
  local curl_args=(-w '\n%{http_code}')
  if [[ -n "$data" ]]; then
    curl_args+=(-d "$data")
  fi
  log_request "$method" "$path" "$data"
  local raw
  raw=$(api "$method" "$path" "${curl_args[@]}")
  local status body
  status=$(echo "$raw" | tail -1)
  body=$(echo "$raw" | sed '$d')
  log_response "$status" "$body"
  echo "$body"
  echo "$status"
}

assert_status() {
  local expected="$1" actual="$2" label="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo -e "  ${GREEN}PASS${NC} $label (HTTP $actual)"
    ((pass++)) || true
  else
    echo -e "  ${RED}FAIL${NC} $label — expected HTTP $expected, got $actual"
    ((fail++)) || true
  fi
}

assert_json() {
  local jq_expr="$1" expected="$2" body="$3" label="$4"
  local actual
  actual=$(echo "$body" | jq -r "$jq_expr" 2>/dev/null || echo "PARSE_ERROR")
  if [[ "$actual" == "$expected" ]]; then
    echo -e "  ${GREEN}PASS${NC} $label ($jq_expr = $actual)"
    ((pass++)) || true
  else
    echo -e "  ${RED}FAIL${NC} $label — expected $jq_expr=$expected, got $actual"
    ((fail++)) || true
  fi
}

host_ids_json() {
  local ids="$1"
  echo "$ids" | tr ',' '\n' | jq -s '.'
}

# parse response: sets $status and $body from api_log output
parse_response() {
  local raw="$1"
  status=$(echo "$raw" | tail -1)
  body=$(echo "$raw" | sed '$d')
}

# ---------- auto-discover or create IDs ----------
if [[ -z "$POLICY_ID" ]]; then
  echo -e "${YELLOW}Auto-discovering policy ID...${NC}"
  POLICY_ID=$(api GET "/api/latest/fleet/policies" | jq -r '.policies[0].id // empty')
  if [[ -z "$POLICY_ID" ]]; then
    echo -e "  ${YELLOW}No policies found, creating one...${NC}"
    POLICY_ID=$(api POST "/api/latest/fleet/policies" \
      -d '{"name":"test-host-assignment-policy","query":"SELECT 1;","description":"Auto-created by test script","resolution":"N/A","platform":""}' \
      | jq -r '.policy.id // empty')
    if [[ -z "$POLICY_ID" ]]; then
      echo "Failed to create policy"
      exit 1
    fi
    echo "  Created policy_id=$POLICY_ID"
  else
    echo "  Using policy_id=$POLICY_ID"
  fi
fi

if [[ -z "$QUERY_ID" ]]; then
  echo -e "${YELLOW}Auto-discovering query ID...${NC}"
  QUERY_ID=$(api GET "/api/latest/fleet/queries" | jq -r '.queries[0].id // empty')
  if [[ -z "$QUERY_ID" ]]; then
    echo -e "  ${YELLOW}No queries found, creating one...${NC}"
    QUERY_ID=$(api POST "/api/latest/fleet/queries" \
      -d '{"name":"test-host-assignment-query","query":"SELECT 1;","description":"Auto-created by test script"}' \
      | jq -r '.query.id // empty')
    if [[ -z "$QUERY_ID" ]]; then
      echo "Failed to create query"
      exit 1
    fi
    echo "  Created query_id=$QUERY_ID"
  else
    echo "  Using query_id=$QUERY_ID"
  fi
fi

if [[ -z "$HOST_IDS" ]]; then
  echo -e "${YELLOW}Auto-discovering host IDs (need at least 2)...${NC}"
  HOST_IDS=$(api GET "/api/latest/fleet/hosts?per_page=3" | jq -r '[.hosts[].id] | join(",")')
  if [[ -z "$HOST_IDS" || "$HOST_IDS" == "null" ]]; then
    echo "No hosts found. Enroll some first or pass --host-ids"
    exit 1
  fi
  echo "  Using host_ids=$HOST_IDS"
fi

IFS=',' read -ra HOST_ARR <<< "$HOST_IDS"
if [[ ${#HOST_ARR[@]} -lt 2 ]]; then
  echo "Need at least 2 host IDs for thorough testing"
  exit 1
fi

HOST_A="${HOST_ARR[0]}"
HOST_B="${HOST_ARR[1]}"
ALL_JSON=$(host_ids_json "$HOST_IDS")
A_JSON=$(host_ids_json "$HOST_A")
B_JSON=$(host_ids_json "$HOST_B")

echo ""
echo "============================================"
echo " Testing host assignment endpoints"
echo " Fleet: $FLEET_URL"
echo " Policy: $POLICY_ID  Query: $QUERY_ID"
echo " Hosts: ${HOST_IDS}"
echo " No-cleanup: $NO_CLEANUP  Verbose: $VERBOSE"
echo "============================================"
echo ""

# =============================================
# POLICY TESTS
# =============================================
echo -e "${YELLOW}=== POLICY HOST ASSIGNMENTS ===${NC}"

# 1. PUT — replace with full list (clean slate)
echo ""
echo "1. PUT /policies/$POLICY_ID/hosts — replace with all hosts"
parse_response "$(api_log PUT "/api/latest/fleet/policies/${POLICY_ID}/hosts" "{\"host_ids\": $ALL_JSON}")"
assert_status "200" "$status" "PUT replace"

# 2. GET — list hosts
echo ""
echo "2. GET /policies/$POLICY_ID/hosts — list all assigned"
parse_response "$(api_log GET "/api/latest/fleet/policies/${POLICY_ID}/hosts?per_page=100")"
assert_status "200" "$status" "GET list"
assert_json '.hosts | length' "${#HOST_ARR[@]}" "$body" "host count matches"

# 3. GET — pagination
echo ""
echo "3. GET /policies/$POLICY_ID/hosts — pagination (per_page=1)"
parse_response "$(api_log GET "/api/latest/fleet/policies/${POLICY_ID}/hosts?per_page=1&page=0")"
assert_status "200" "$status" "GET page 0"
assert_json '.hosts | length' "1" "$body" "page size = 1"

# 4. DELETE — remove one host
echo ""
echo "4. DELETE /policies/$POLICY_ID/hosts — remove host $HOST_A"
parse_response "$(api_log DELETE "/api/latest/fleet/policies/${POLICY_ID}/hosts" "{\"host_ids\": $A_JSON}")"
assert_status "200" "$status" "DELETE one"
assert_json '.removed' "1" "$body" "removed count"

# 5. GET — verify removal
echo ""
echo "5. GET /policies/$POLICY_ID/hosts — verify removal"
parse_response "$(api_log GET "/api/latest/fleet/policies/${POLICY_ID}/hosts?per_page=100")"
assert_status "200" "$status" "GET after delete"
remaining=$((${#HOST_ARR[@]} - 1))
assert_json '.hosts | length' "$remaining" "$body" "count after remove"

# 6. POST — add host back
echo ""
echo "6. POST /policies/$POLICY_ID/hosts — add host $HOST_A back"
parse_response "$(api_log POST "/api/latest/fleet/policies/${POLICY_ID}/hosts" "{\"host_ids\": $A_JSON}")"
assert_status "200" "$status" "POST add"
assert_json '.added' "1" "$body" "added count"

# 7. POST — idempotent add (should add 0)
echo ""
echo "7. POST /policies/$POLICY_ID/hosts — idempotent add (already exists)"
parse_response "$(api_log POST "/api/latest/fleet/policies/${POLICY_ID}/hosts" "{\"host_ids\": $A_JSON}")"
assert_status "200" "$status" "POST idempotent"
assert_json '.added' "0" "$body" "added 0 (idempotent)"

# 8. DELETE — idempotent remove (host not assigned)
echo ""
echo "8. DELETE /policies/$POLICY_ID/hosts — remove non-assigned host 999999"
parse_response "$(api_log DELETE "/api/latest/fleet/policies/${POLICY_ID}/hosts" '{"host_ids": [999999]}')"
assert_status "200" "$status" "DELETE non-existent"
assert_json '.removed' "0" "$body" "removed 0"

if [[ "$NO_CLEANUP" == false ]]; then
  # 9. PUT — clear all
  echo ""
  echo "9. PUT /policies/$POLICY_ID/hosts — clear all (empty list)"
  parse_response "$(api_log PUT "/api/latest/fleet/policies/${POLICY_ID}/hosts" '{"host_ids": []}')"
  assert_status "200" "$status" "PUT clear"

  # 10. GET — verify empty
  echo ""
  echo "10. GET /policies/$POLICY_ID/hosts — verify empty"
  parse_response "$(api_log GET "/api/latest/fleet/policies/${POLICY_ID}/hosts?per_page=100")"
  assert_status "200" "$status" "GET empty"
  assert_json '.hosts | length' "0" "$body" "empty after clear"
else
  echo ""
  echo -e "${YELLOW}9-10. SKIPPED (--no-cleanup): policy hosts left assigned${NC}"
fi

# =============================================
# QUERY TESTS
# =============================================
echo ""
echo -e "${YELLOW}=== QUERY HOST ASSIGNMENTS ===${NC}"

# 11. PUT — replace
echo ""
echo "11. PUT /queries/$QUERY_ID/hosts — replace with all hosts"
parse_response "$(api_log PUT "/api/latest/fleet/queries/${QUERY_ID}/hosts" "{\"host_ids\": $ALL_JSON}")"
assert_status "200" "$status" "PUT replace"

# 12. GET — list
echo ""
echo "12. GET /queries/$QUERY_ID/hosts — list all"
parse_response "$(api_log GET "/api/latest/fleet/queries/${QUERY_ID}/hosts?per_page=100")"
assert_status "200" "$status" "GET list"
assert_json '.hosts | length' "${#HOST_ARR[@]}" "$body" "host count"

# 13. DELETE — remove one
echo ""
echo "13. DELETE /queries/$QUERY_ID/hosts — remove host $HOST_B"
parse_response "$(api_log DELETE "/api/latest/fleet/queries/${QUERY_ID}/hosts" "{\"host_ids\": $B_JSON}")"
assert_status "200" "$status" "DELETE one"
assert_json '.removed' "1" "$body" "removed 1"

# 14. POST — add back
echo ""
echo "14. POST /queries/$QUERY_ID/hosts — add host $HOST_B back"
parse_response "$(api_log POST "/api/latest/fleet/queries/${QUERY_ID}/hosts" "{\"host_ids\": $B_JSON}")"
assert_status "200" "$status" "POST add"
assert_json '.added' "1" "$body" "added 1"

if [[ "$NO_CLEANUP" == false ]]; then
  # 15. PUT — clear
  echo ""
  echo "15. PUT /queries/$QUERY_ID/hosts — clear all"
  parse_response "$(api_log PUT "/api/latest/fleet/queries/${QUERY_ID}/hosts" '{"host_ids": []}')"
  assert_status "200" "$status" "PUT clear"

  # 16. GET — verify empty
  echo ""
  echo "16. GET /queries/$QUERY_ID/hosts — verify empty"
  parse_response "$(api_log GET "/api/latest/fleet/queries/${QUERY_ID}/hosts?per_page=100")"
  assert_status "200" "$status" "GET empty"
  assert_json '.hosts | length' "0" "$body" "empty after clear"
else
  echo ""
  echo -e "${YELLOW}15-16. SKIPPED (--no-cleanup): query hosts left assigned${NC}"
fi

# =============================================
# SUMMARY
# =============================================
echo ""
echo "============================================"
total=$((pass + fail))
echo -e " Results: ${GREEN}${pass} passed${NC}, ${RED}${fail} failed${NC} / ${total} total"
if [[ "$NO_CLEANUP" == true ]]; then
  echo -e " ${YELLOW}No-cleanup mode: test data preserved${NC}"
fi
echo "============================================"

if [[ $fail -gt 0 ]]; then
  exit 1
fi

