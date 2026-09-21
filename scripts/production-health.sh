#!/usr/bin/env bash
# Read-only production checks. This script never signs payments or provisions
# a sandbox, so it is safe to run against mainnet.
set -euo pipefail

BASE_URL="${BASE_URL:-https://mcp.computerouter.wtf}"
BASE_URL="${BASE_URL%/}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

get() {
  local path="$1"
  curl --fail --silent --show-error --location \
    -H "Accept: application/json, text/plain" \
    -o "$tmp_dir/response" -w "%{http_code}" "$BASE_URL$path"
}

expect_status() {
  local path="$1"
  local expected="$2"
  local actual
  actual="$(get "$path")"
  printf '%-34s HTTP %s\n' "GET $path" "$actual"
  test "$actual" = "$expected"
}

expect_status "/" 200
expect_status "/styles.css" 200
expect_status "/app.js" 200
expect_status "/docs/agent-guide.json" 200
expect_status "/.well-known/agent-card.json" 200

status="$(curl --fail --silent --show-error --location \
  -H "Accept: application/json, text/event-stream" \
  -o "$tmp_dir/mcp" -w "%{http_code}" "$BASE_URL/mcp")"
printf '%-34s HTTP %s\n' "GET /mcp" "$status"
test "$status" = "200"

grep -q '"endpoint"[[:space:]]*:[[:space:]]*"/mcp"' "$tmp_dir/mcp"
grep -q '"name"[[:space:]]*:[[:space:]]*"Compute Router"' "$tmp_dir/mcp"

curl --fail --silent --show-error --location \
  -X POST "$BASE_URL/mcp" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"production-health","version":"1"}}}' \
  -o "$tmp_dir/initialize"
grep -q 'serverInfo' "$tmp_dir/initialize"

curl --fail --silent --show-error --location \
  -X POST "$BASE_URL/mcp" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  -o "$tmp_dir/tools"
for tool in provision_env exec extend_ceiling release; do
  grep -q "\"name\"[[:space:]]*:[[:space:]]*\"$tool\"" "$tmp_dir/tools"
done

curl --fail --silent --show-error --location \
  -X POST "$BASE_URL/mcp" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"provision_env","arguments":{"template":"node-build","ceiling_usd":0.02}}}' \
  -o "$tmp_dir/payment"
grep -q 'payment_required' "$tmp_dir/payment"

echo "Production health checks passed; no payment was submitted."
