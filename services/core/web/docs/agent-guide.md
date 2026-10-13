# Compute Router agent guide

## What it is

Compute Router is an MCP server for short-lived, isolated build sandboxes.
An agent pays a fixed two-minute block in USDC, USDT, wARS (Peso Argentino),
wBRL (Real Brasileiro), or wCOP (Peso Colombiano) on Celo, receives a session
ID, runs shell commands, and releases the session. The payment is settled by
x402. The compute backend is the Cloudflare Sandbox adapter.

USD-pegged assets (USDC, USDT) use EIP-3009 `transferWithAuthorization`.
LatAm assets (wARS, wBRL, wCOP) use Permit2 `permitTransferFrom` with an
18-decimal token at the FX rate advertised in the payment requirement.

Use it when a task needs a real Linux shell, package installation, compilation,
tests, or other build tools that should not run in the agent's own process.
Do not use it for durable storage, long-running services, or privileged host
access. The available templates are advertised in the agent card's `runtimes`
field; `node-build` is always present and additional runtimes (e.g.
`python-build`, `rust-build`) appear when the deployment has them configured.

## Discovery

- MCP endpoint: `https://mcp.computerouter.wtf/mcp`
- Agent card: `https://mcp.computerouter.wtf/.well-known/agent-card.json`
- Machine-readable guide: `/docs/agent-guide.json`

The agent card (`/.well-known/agent-card.json`) includes machine-readable
discovery fields so a caller agent can determine supported options before
connecting:

- `payment_protocol` — always `"x402"`.
- `payment_network` — CAIP-2 chain ID, e.g. `"eip155:42220"` for Celo mainnet.
- `accepted_assets` — array of asset objects, each with `symbol`, `name`,
  `decimals`, `assetTransferMethod` (`"eip3009"` or `"permit2"`), and
  `tokenAddress`.
- `runtimes` — array of supported template names, e.g. `["node-build"]`.
- `pricing` — `{ "price_per_block_usd": 0.02, "block_minutes": 2 }`.

Start with an MCP `initialize` request, then call `tools/list`. The server
exposes `provision_env`, `exec`, `extend_ceiling`, and `release`.

## Session protocol

1. Call `provision_env` with `template: "node-build"` and a small
   `ceiling_usd` (for example `0.02`). Omit `payment_data` on the first call.
2. Read the structured `payment_required` result. Choose an asset from the
   `accepts` array. For USDC/USDT sign an EIP-3009 payload; for wARS/wBRL/wCOP
   sign a Permit2 payload (the `extra.assetTransferMethod` field identifies
   which scheme to use). Retry with the resulting `payment_data`. Never send a
   private key to Compute Router.
3. Save `session_id` and `expires_at` from the successful result.
4. Call `exec` with that session ID. The response includes `stdout`, `stderr`,
   and `exit_code`; a non-zero exit code is a command result, not necessarily
   an MCP transport failure.
5. If more time is needed, call `extend_ceiling` with `additional_usd` and a
   new signed payment. It adds one priced block and extends the lease.
6. Call `release` in a `finally`-style cleanup path. If the lease has already
   expired, the session may already be gone and release returns an error.

The price is configured by the deployment. The public MVP currently advertises
$0.02 per two-minute block. Provisioning and extension are paid in fixed
blocks; this is not yet per-second metering.

## Safety and failure handling

- A missing payment returns a structured error; it is not an HTTP 402.
- Invalid, expired, wrong-network, wrong-asset, or wrong-recipient payment
  data must be treated as a failed call and retried only with a new valid
  payment.
- Sandbox boot happens before settlement. If boot fails, no settlement should
  be submitted.
- If settlement is ambiguous, the server destroys the sandbox and creates a
  refund review instead of guessing that funds were lost or captured.
- `exec` is session-scoped. A missing or expired session is an error.
- Commands can be rejected when the optional safety checker is enabled.

Keep command output bounded in your own workflow, avoid putting secrets in
commands, and release sessions promptly. The server does not provide a durable
workspace: treat the sandbox as disposable.

## Idempotency

Supply `idempotency_key` on `provision_env` to make retries safe. A duplicate
call with the same key returns the original `session_id` and `expires_at`
without charging again. The key is opaque to the server; use a UUID or any
globally-unique string that identifies the provisioning attempt.

## Exec output limits

`exec` caps `stdout` and `stderr` to 256 KiB (262 144 bytes) each by default.
Pass `max_output_bytes` to request a smaller cap; the server clamps any value
larger than 256 KiB back to 256 KiB, so you cannot raise the limit above that.
When a stream is truncated the response includes `"truncated": true`.

## Minimal call shapes

```json
{"name":"provision_env","arguments":{"template":"node-build","ceiling_usd":0.02,"idempotency_key":"<uuid>"}}
```

```json
{"name":"exec","arguments":{"session_id":"<session_id>","command":"npm test","max_output_bytes":65536}}
```

```json
{"name":"extend_ceiling","arguments":{"session_id":"<session_id>","additional_usd":0.02}}
```

```json
{"name":"release","arguments":{"session_id":"<session_id>"}}
```
