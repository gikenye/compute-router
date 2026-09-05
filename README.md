# Compute Router

Agent-native, metered compute sandboxes — provisioned, executed, and
paid for entirely over MCP, settled in USDC on Celo via x402.

Built for the Celo "Agents at Work" Hackathon.

## What this is

An MCP server exposes four tools — `provision_env`, `exec`,
`extend_ceiling`, `release` — that let any MCP-compatible agent spin up
a short-lived shell with common build tools preinstalled, run commands
in it, and tear it down, paying only for what it actually uses. No
account, no API key, no pre-registration — the agent's wallet pays per
session over x402, and the session runs on Cloudflare's Sandbox SDK.

## Architecture

- `services/core` — the MCP server (`/mcp`), x402 payment logic, and a
  small reference/landing UI served at `/` (Go)
- `services/sandbox-adapter` — a thin Cloudflare Worker wrapping
  `@cloudflare/sandbox` for the actual compute (TypeScript)

Two languages by design, not convenience: the Cloudflare Sandbox SDK is
unavoidably tied to the Workers runtime, so it's isolated behind a
narrow adapter boundary rather than dictating the language of the rest
of the system.

## Agent identity

This service is registered as an ERC-8004 agent on Celo. Its
registration file — endpoints, supported trust models, payout wallet —
is served live at `/.well-known/agent-card.json` on its deployed URL.

## Running it

Requires Go 1.23+, Node 20+, and Docker. See each service's own
`Dockerfile` and `wrangler.jsonc` for deploy configuration.

```bash
cd services/core && go run ./cmd/server
```

```bash
cd services/sandbox-adapter && npx wrangler dev
```

## License

TBD.
