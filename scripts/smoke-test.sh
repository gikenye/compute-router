#!/usr/bin/env bash
# Minimal local smoke test against the core service's MCP endpoint.
# Confirms the server boots and speaks JSON-RPC correctly — does NOT
# exercise real payment settlement (that needs testnet funds and a
# real signed payment_data, which this script doesn't have).
#
# Run: PORT=8080 bash scripts/smoke-test.sh
# (with `make dev-core` running in another terminal first)
set -euo pipefail
PORT="${PORT:-8080}"
BASE="http://localhost:${PORT}"

echo "--- 1. MCP initialize handshake ---"
curl -sS -X POST "$BASE/" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-06-18",
      "capabilities": {},
      "clientInfo": { "name": "smoke-test", "version": "0.0.1" }
    }
  }' | tee /tmp/mcp-init.json
echo
echo "If that returned a JSON-RPC result with serverInfo, the MCP server is up."

echo
echo "--- 2. tools/list ---"
curl -sS -X POST "$BASE/" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
echo
echo "Expect: provision_env, exec, extend_ceiling, release listed."

echo
echo "--- 3. tools/call provision_env WITHOUT payment_data ---"
echo "Expect: isError=true, payment_required content — this confirms"
echo "SPEC-100 §5.2's structured-payment-signal design actually works."
curl -sS -X POST "$BASE/" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "provision_env",
      "arguments": { "template": "node-build", "ceiling_usd": 0.5 }
    }
  }'
echo
echo "--- Smoke test done. Real payment flow needs testnet funds + a signed payment_data — not covered here. ---"
