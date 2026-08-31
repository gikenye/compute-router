# AGENTS.md

Read this before touching any code. Full spec lives in `docs/SPEC-100.md` —
this file is the fast-orientation layer.

## 1. What this repo is

An MCP server that lets an AI agent provision a short-lived, pre-tooled
compute sandbox (shell + build tools), pay for it in USDC on Celo via the
x402 protocol, run commands in it, and release it — billed for actual usage,
not a flat rate. Built for the Celo "Agents at Work" hackathon (buy/x402 track).

## 2. Non-negotiable design decisions (do not silently change these)

- **Two services, two languages, on purpose — see `docs/ADR-001-language-choice.md`.**
  `services/core` (Go) holds all business logic: MCP server, x402 payment,
  session/pricing. `services/sandbox-adapter` (TypeScript, Cloudflare
  Worker) is a deliberately dumb wrapper around `@cloudflare/sandbox` —
  the ONLY reason TypeScript exists in this repo is that Sandbox SDK is
  unavoidably Workers-runtime-only (confirmed: no raw REST API exists
  independent of it). Do not add business logic to the adapter, and do
  not collapse the two services into one language for convenience — that
  trade was made deliberately for a long-term-business reason, not a
  hackathon shortcut.
- **Compute backend: Cloudflare Sandbox SDK.** Not Cencori (waitlisted,
  stateless function-call API, not a persistent shell — see SPEC-100 §2).
  Not raw Fly Machines unless Sandbox SDK proves insufficient — Sandbox SDK's
  stable-ID reconnect gives us session persistence for free. Note: Sandbox
  SDK is Beta — this is exactly why it's isolated behind the adapter
  boundary rather than woven through core logic.
- **Payment protocol: x402, vendor-neutral.** Do not hardcode assumptions
  about the buyer's wallet stack (not thirdweb-specific, not any single
  client). Our server only needs to speak the protocol correctly.
- **Facilitator: `api.x402.celo.org` by default.** Swap to thirdweb's
  facilitator only if the `upto` metered-settlement scheme is required and
  confirmed working (SPEC-100 §4.3) — Celo's own facilitator hedges on
  supporting it.
- **Payment signal lives inside the JSON-RPC tool result, not a raw HTTP
  402.** MCP's Streamable HTTP transport does not guarantee a per-tool-call
  HTTP status the way a plain `fetch()` does — see SPEC-100 §5.2. Do not
  build client-catching-402-on-a-tool-call logic without re-verifying this
  against whatever MCP SDK you're using.
- **Container image is fixed at Worker deploy time, not per-request.**
  Cloudflare Containers do not support runtime template switching — the
  MVP ships ONE image (`node-build`). The `template` field on
  `provision_env`/`/boot` is accepted for forward-compatibility but has
  no effect yet. Do not build fake multi-template support.
- **This is real, wired code, not stubs** — see `services/core` and
  `services/sandbox-adapter` for actual implementations. It has NOT
  been run or compiled (this environment has no network access to
  install dependencies) — run `make deps` first, see
  `docs/RUNBOOK.md`, and expect to fix real compile/integration errors,
  especially around the items in §5 below.

## 3. Before you write code

1. Read `docs/SPEC-100.md` in full — it flags which parts are VERIFIED
   against primary docs vs. UNCONFIRMED / needs-testing.
2. Read `docs/OPS.md` for the actual run-and-debug sequence, including
   the synthetic traffic driver (`tools/txs`).
3. Anything marked UNCONFIRMED must be tested against real infra
   (Celo Sepolia/Alfajores testnet, not mainnet) before it's load-bearing
   in the demo.
4. Copy `.env.example` to `.env`, `services/sandbox-adapter/.dev.vars.example`
   to `.dev.vars`, and `tools/txs/.env.example` to `tools/txs/.env` —
   fill all three in. Nothing here has real secrets baked in.

## 4. Repo layout (actual, scaffolded)

```
/services
  /core                     Go — MCP server, x402 payment, session, pricing
    go.mod                  no go.sum yet — run `make deps` (needs network)
    Dockerfile              cloud deploy image
    fly.toml                one turnkey cloud option — not the only one
    cmd/server/main.go      wires MCP server + all 4 tools + HTTP transport
    internal/config/        env var loading, fails fast on missing required vars
    internal/mcp/           tool handlers: provision_env, exec, extend_ceiling, release
    internal/payment/       x402 verify/settle client — real HTTP calls
    internal/session/       in-memory lease store (NOT durable — MVP shortcut)
    internal/sandboxclient/ HTTP client calling sandbox-adapter — the ONLY
                             place core knows the adapter exists
  /sandbox-adapter          TypeScript, Cloudflare Worker — thin @cloudflare/sandbox wrapper
    package.json
    wrangler.jsonc           real Cloudflare Containers config shape
    Dockerfile               real Sandbox SDK base image + build-essential
    .dev.vars.example        copy to .dev.vars for local `wrangler dev`
    src/index.ts             POST /boot, /exec, /destroy — nothing else belongs here
/docs
  SPEC-100.md               Full design spec, numbered, VERIFIED/UNCONFIRMED tags
  ADR-001-language-choice.md  Why Go + TypeScript, not one or the other
  RUNBOOK.md                Actual local-run, debug, and deploy steps
/scripts
  smoke-test.sh             curl-based MCP handshake + payment-required check
/tools
  /txs                      Synthetic traffic generator — 10 CSV-driven wallets
                             running the full user journey on a schedule.
                             See docs/OPS.md §4. NOT part of the product;
                             a beta-testing/bug-finding harness.
Makefile                    make deps / dev-core / dev-adapter / smoke-test /
                             deploy-* / txs-fund / txs-journey-once / txs-driver
.env.example
.gitignore
```

Every implementation file has inline comments tagged VERIFIED or
UNCONFIRMED against a specific SPEC-100/ADR-001 section — start there
when debugging, not from scratch.

## 5. Known open items (do not treat as solved)

- Whether Celo's hosted facilitator supports the `upto` scheme at all —
  their own docs hedge. Untested as of this writing. MVP uses "exact"
  fixed-block settlement instead (SPEC-100 §4.4 fallback path).
- Exact `PaymentRequirements` JSON field names for `api.x402.celo.org`
  reconstructed from partial docs + one example call — verify against
  `GET /supported` before shipping. This is the single most likely
  thing to break the payment flow — check here first if `/verify` or
  `/settle` reject a payload.
- Whether your specific MCP SDK version lets a tool handler return a
  non-200 at the transport layer — the code as written does NOT rely on
  this (it uses the structured-content pattern per SPEC-100 §5.2
  regardless), so this is lower risk than it was before the code existed.
- The exact chained call shape for Sandbox SDK's `createSession()`
  pattern was not independently confirmed — `services/sandbox-adapter/src/index.ts`
  currently calls `.exec()` directly on the object `getSandbox()`
  returns, with a comment flagging the fallback if `createSession()`
  turns out to be required instead.
- No confirmed explicit "destroy" method on Sandbox SDK — the adapter's
  `/destroy` handler calls it defensively (checks the method exists
  first) and treats failure as non-fatal, relying on Sandbox SDK's own
  idle timeout as the real backstop.
- Whether `services/sandbox-adapter` can expose Cloudflare's own
  per-sandbox CPU-second usage data for metered settlement (SPEC-100
  §4.4), or whether core needs to track wall-clock time as a fallback —
  not needed for MVP (fixed-block billing), only for the stretch goal.
- `tools/txs`: Alfajores testnet USDC token + fee-adapter addresses are
  blank in `.env.example` — only mainnet addresses were confirmed
  during this build. Must be filled in before `make txs-fund` will work.
- `tools/txs`: EIP-3009 domain `name`/`version` default to Circle's
  standard USDC values as a fallback — if the real facilitator's
  `payment_requirements.price.extra` supplies different values, use
  those instead (the signer already prefers `extra` when present).
- `tools/txs`: seed-to-account derivation assumes standard Ethereum
  path (coin type 60) — UNCONFIRMED against Celo specifically, see
  `docs/OPS.md` §4d.
