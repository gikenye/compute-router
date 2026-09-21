# Compute Router agent guide

## What it is

Compute Router is an MCP server for short-lived, isolated build sandboxes.
An agent pays a fixed two-minute block in USDC or USDT on Celo, receives a
session ID, runs shell commands, and releases the session. The payment is
settled by x402. The compute backend is the Cloudflare Sandbox adapter.

Use it when a task needs a real Linux shell, package installation, compilation,
tests, or other build tools that should not run in the agent's own process.
Do not use it for durable storage, long-running services, privileged host
access, or workloads that require a specific image: the current MVP exposes
one `node-build` image and the `template` field is forward-compatible only.

## Discovery

- MCP endpoint: `https://mcp.computerouter.wtf/mcp`
- Agent card: `https://mcp.computerouter.wtf/.well-known/agent-card.json`
- Machine-readable guide: `/docs/agent-guide.json`

Start with an MCP `initialize` request, then call `tools/list`. The server
exposes `provision_env`, `exec`, `extend_ceiling`, and `release`.

## Session protocol

1. Call `provision_env` with `template: "node-build"` and a small
   `ceiling_usd` (for example `0.02`). Omit `payment_data` on the first call.
2. Read the structured `payment_required` result. Select USDC or USDT, sign the
   returned x402 requirement with the paying wallet, and retry with the
   resulting `payment_data`. Never send a private key to Compute Router.
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

## Minimal call shapes

```json
{"name":"provision_env","arguments":{"template":"node-build","ceiling_usd":0.02}}
```

```json
{"name":"exec","arguments":{"session_id":"<session_id>","command":"npm test"}}
```

```json
{"name":"extend_ceiling","arguments":{"session_id":"<session_id>","additional_usd":0.02}}
```

```json
{"name":"release","arguments":{"session_id":"<session_id>"}}
```
