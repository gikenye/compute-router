# Agent-Native Metered Compute Sandbox

Celo "Agents at Work" hackathon build — buy/x402 track.

**If you are a coding agent picking this up: read `AGENTS.md` first, then
`docs/SPEC-100.md` and `docs/ADR-001-language-choice.md` before writing
code, then `docs/OPS.md` to actually run and debug it.** The spec tags
every design claim as VERIFIED (checked against primary docs) or
UNCONFIRMED (best-effort — re-test before relying on it).

Quick summary: an MCP server (`services/core`, Go) provisions a Cloudflare
Sandbox SDK shell (`services/sandbox-adapter`, TypeScript) with
preinstalled build tools, paid for in USDC on Celo via x402. Two
languages, one deliberate boundary — see ADR-001 for why.
`tools/txs` is a synthetic traffic generator — 10 CSV-driven wallets
running the real user journey on a natural schedule, funded and
gas-abstracted on Celo — used as a first beta test to find bugs before
real agents show up.

**Status: real, wired implementation — not yet run.** It was written in
an environment with no network access, so dependencies were never
installed and the code has never compiled. Run `make deps` first (see
OPS.md), then expect to fix real integration issues — the places most
likely to need a fix are flagged inline as UNCONFIRMED throughout the
code and listed in `AGENTS.md` §5.

## Layout

```
services/core/            Go — business logic (MCP, payment, sessions)
services/sandbox-adapter/ TypeScript — thin Cloudflare Worker wrapper
docs/                     SPEC-100 (design) + ADR-001 (language decision)
.env.example
```
