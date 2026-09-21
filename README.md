# Compute Router

Agent-native, metered compute sandboxes; provisioned, executed, and
paid for entirely over MCP, settled in USDC on Celo via x402.

Built for the Celo "Agents at Work" Hackathon.

## What this is

An MCP server exposes four tools : `provision_env`, `exec`,
`extend_ceiling`, `release` : that let any MCP-compatible agent spin up
a short-lived shell with common build tools preinstalled, run commands
in it, and tear it down, paying only for what it actually uses. No
account, no API key, no pre-registration : the agent's wallet pays per
session over x402, and the session runs on Cloudflare's Sandbox SDK.

## Architecture

- `services/core` : the MCP server (`/mcp`), x402 payment logic, and a
  small reference/landing UI served at `/` (Go)
- `services/sandbox-adapter` : a thin Cloudflare Worker wrapping
  `@cloudflare/sandbox` for the actual compute (TypeScript)


## Agent identity

This service is registered as an ERC-8004 agent on Celo. Its
registration file : endpoints, supported trust models, payout wallet :
is served live at `/.well-known/agent-card.json` on its deployed URL.

## Running it

Requires Go 1.25+, Node 20+, and Docker. See each service's own
`Dockerfile` and `wrangler.jsonc` for deploy configuration.

```bash
cd services/core && go run ./cmd/server
```

```bash
cd services/sandbox-adapter && npx wrangler dev
```

## Documentation and validation

The human documentation is published as a static GitHub Pages site:

- [Documentation home](https://gikenye.github.io/compute-router/)
- [For agents](https://gikenye.github.io/compute-router/agents.html)
- [Architecture](https://gikenye.github.io/compute-router/architecture.html)
- [Operators](https://gikenye.github.io/compute-router/operators.html)
- [Validation](https://gikenye.github.io/compute-router/validation.html)
- [Roadmap](https://gikenye.github.io/compute-router/roadmap.html)

The deployment UI also serves a compact agent guide:

- [Agent guide](services/core/web/docs/agent-guide.md) — how to discover and
  call the MCP tools, handle x402 payment negotiation, and manage sessions.
- [Validation plan](services/core/web/docs/validation-plan.md) — safe health
  checks plus paid mainnet scenarios, expected results, edge cases, and the
  evidence to capture.
- [Roadmap](services/core/web/docs/roadmap.md) — what is live, what is
  experimental, and what must be built next.
- [Machine-readable guide](services/core/web/docs/agent-guide.json) — a
  compact manifest for agents that prefer JSON over Markdown.

Run the non-paying production contract checks with:

```bash
BASE_URL=https://mcp.computerouter.wtf bash scripts/production-health.sh
```

These checks do not spend funds or create a sandbox. The paid scenarios in the
validation plan require an explicitly funded test wallet and must be run with
small ceilings.

## License

MIT 
