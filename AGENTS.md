# AGENTS.md

Read this before touching any code. This file itself, `internal-docs/`,
and `tools/` are gitignored from the public repo — they exist on disk
for local agent use only. Full specs live in `internal-docs/ASD-STE-*.md`
— this file is the fast-orientation layer.

## 1. What this repo is

An MCP server that lets an AI agent provision a short-lived, pre-tooled
compute sandbox (shell + build tools), pay for it in USDC on Celo via
the x402 protocol, run commands in it, and release it — billed for
actual usage, not a flat rate. Registered as an ERC-8004 agent identity
on Celo. Built for the Celo "Agents at Work" hackathon (buy/x402 track).

**Active work queue: `internal-docs/ASD-STE-106.md`.** If you're a
coding agent picking this up with real network/deploy access, start
there — it's a sequential, gated checklist from "never compiled" through
mainnet deployment. Everything else in this file is background context;
that file is the actual next steps.

## 2. IP / repo-visibility policy — read this before creating any new file

- `services/` is the PUBLIC product — this is what ships in the public
  GitHub repo the hackathon requires.
- `internal-docs/`, `AGENTS.md`, and `tools/` are gitignored,
  permanently, by policy — treated as organizational IP, not a
  temporary hackathon shortcut. When adding a new doc or a new tool,
  put design docs under `internal-docs/ASD-STE-<next number>.md` and
  new tooling under `tools/<name>/` — both stay off the public repo
  automatically via `.gitignore`. Don't add public-facing detail to
  `README.md` beyond what's already there without checking whether it
  belongs in `internal-docs/` instead.
- Consequence worth knowing: the public repo's commit history will show
  only `services/` — the hackathon rules say judges look at commit
  history (rule 9, "code is written during the hackathon"), so this is
  a deliberate tradeoff (protecting internal IP over showing full
  process), not an oversight. Don't silently change it.

## 3. Non-negotiable design decisions (do not silently change these)

- **Two services, two languages, on purpose — see `internal-docs/ASD-STE-101.md`.**
  `services/core` (Go) holds all business logic: MCP server, x402
  payment, session/pricing, agent-card serving, optional safety check.
  `services/sandbox-adapter` (TypeScript, Cloudflare Worker) is a
  deliberately dumb wrapper around `@cloudflare/sandbox`.
- **Container image is fixed at Worker deploy time, not per-request.**
  Cloudflare Containers do not support runtime template switching — the
  MVP ships ONE image (`node-build`). The `template` field on
  `provision_env`/`/boot` is accepted for forward-compatibility but has
  no effect yet.
- **Compute backend: Cloudflare Sandbox SDK.** Not Cencori's Compute
  product (waitlisted, stubbed API — see ASD-STE-100 §2). Cencori's
  LLM gateway is used elsewhere for a different purpose (§5 below) —
  don't confuse the two.
- **Payment protocol: x402, vendor-neutral.** Don't hardcode assumptions
  about the buyer's wallet stack.
- **Facilitator: `api.x402.celo.org` (mainnet) / `api.x402.sepolia.celo.org`
  (testnet) by default.** MVP settlement is fixed-block "exact" scheme,
  not the metered "upto" scheme — see ASD-STE-100 §4.4.
- **Payment signal lives inside the JSON-RPC tool result, not a raw HTTP
  402.** See ASD-STE-100 §5.2 for why.
- **Testnet everywhere by default, mainnet required for anything that
  should count.** See ASD-STE-104 §1 — this is a hackathon-rules
  constraint, not just engineering discipline. `tools/txs` in
  particular must NEVER run against mainnet — see its own header
  comment and ASD-STE-104 §1 for why (its output structurally can't
  count toward the leaderboard regardless of chain).
- **This repo never holds a payment-receiving signing wallet** — only
  a plain `payTo` address. The ONE exception is
  `tools/agent-identity`'s `AGENT_OPERATOR_PRIVATE_KEY`, which owns the
  ERC-8004 NFT identity — a deliberately separate, minimally-scoped key,
  not the same as `tools/txs`'s `FUNDER_PRIVATE_KEY`. See ASD-STE-103 §5.
- **MCP endpoint is `/mcp`, not `/`.** Root now serves the web UI
  (`services/core/web/`, static, no build step — ASD-STE-107). This
  changed after the UI was added; every existing caller in this repo
  was updated alongside it (`tools/txs`, `scripts/smoke-test.sh`). If
  you add a new tool that talks to the MCP server, point it at `/mcp`,
  not root.

## 4. Before you write code

1. Read `internal-docs/ASD-STE-100.md` (core spec) and `ASD-STE-101.md`
   (language split) in full.
2. Read `ASD-STE-103.md` (agent identity) and `ASD-STE-104.md`
   (hackathon compliance) before touching anything payment- or
   attribution-related — getting these wrong risks real leaderboard
   miscounting, not just a bug.
3. Read `ASD-STE-102.md` for the actual run-and-debug sequence.
4. Anything marked UNCONFIRMED must be tested against real infra
   (testnet first) before it's load-bearing in the demo.
5. Copy every `.env.example` in the repo to `.env` (root,
   `services/sandbox-adapter/.dev.vars.example` → `.dev.vars`,
   `tools/txs/.env.example`, `tools/agent-identity/.env.example`) and
   fill them in.

## 5. Repo layout (actual, scaffolded)

```text
/services                    PUBLIC — the product
  /core                       Go — MCP server (now at /mcp — see below),
                               x402, session, agent-card, optional
                               Cencori safety check, and the web UI
    /web                        Static HTML/CSS/JS scaffold (ASD-STE-107),
                                 served at root "/"
  /sandbox-adapter             TypeScript, Cloudflare Worker
/internal-docs                GITIGNORED — architecture specs, ADRs, ops
  ASD-STE-100.md               Core architecture spec
  ASD-STE-101.md               ADR: Go/TypeScript language split
  ASD-STE-102.md               Operations — run and debug everything
  ASD-STE-103.md               ERC-8004 agent identity design
  ASD-STE-104.md               Hackathon compliance & attribution
  ASD-STE-105.md               ADR: Chainstack + Cencori sponsor integrations
  ASD-STE-106.md               ACTIVE handoff — debug → run → Fly → mainnet
  ASD-STE-107.md               UI design brief & scaffold
  registration-record.md       Celo Builders skill registration status
/.celobuilders-credentials    GITIGNORED — Celo Builders skill's own auth,
                               not read by any of our services
/tools                        GITIGNORED — internal tooling
  /txs                          Synthetic traffic — beta-test harness ONLY,
                                 never counts toward the leaderboard, never
                                 run against mainnet (ASD-STE-104 §1)
  /agent-identity                ERC-8004 mint script (ASD-STE-103)
/scripts                      PUBLIC-safe utility scripts
  smoke-test.sh
Makefile
.env.example
.gitignore
```

## 6. Known open items (do not treat as solved)

- Whether Celo's hosted facilitator supports the `upto` scheme —
  untested. MVP uses fixed-block "exact" settlement instead.
- Exact `PaymentRequirements` JSON field names — reconstructed from
  docs, verify against `GET /supported` if `/verify` or `/settle` reject
  a payload.
- **Whether the x402 facilitator needs a separate attribution-tag
  parameter for Track 1 credit, beyond registering `PAYOUT_WALLET` as
  our payTo wallet — genuinely unconfirmed, see ASD-STE-104 §2. Resolve
  this early in the hackathon window, not at the end.**
- `@chaoschain/sdk`'s exact provider shape and return-value shape for
  the minted Agent ID — `register.ts` logs the raw result specifically
  so this is visible if wrong.
- Whether `@chaoschain/sdk`'s `register()` call can carry an ERC-8021
  attribution suffix at all — see ASD-STE-103 §6 / ASD-STE-104 §2 Case B.
- Cencori's gateway API endpoint/request shape for the safety check —
  `internal/safety/cencori.go` assumes an OpenAI-compatible shape,
  unconfirmed against Cencori's actual docs. Feature is OFF by default
  specifically because of this.
- The exact chained call shape for Sandbox SDK's `createSession()`
  pattern, and whether an explicit sandbox "destroy" method exists —
  both flagged inline in `services/sandbox-adapter/src/index.ts`.
- Seed-to-account derivation in `tools/txs` assumes standard Ethereum
  derivation path — unconfirmed against Celo specifically.
