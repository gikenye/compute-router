# Compute Router

Compute Router is an agent-native compute layer for AI systems that need disposable, pre-tooled developer environments without manual VM setup. An AI agent can provision a sandbox, run shell commands, extend the session, and release it while paying in USDC on Celo via x402.

This is the product layer behind an MCP-first experience: the agent asks for compute, receives a payment challenge when needed, signs once, and gets a working shell. The product is designed for real-world automation, build and test flows, and short-lived compute tasks where the operator needs an interactive environment without standing up infrastructure by hand.

This system follows the ASD-STE 100 model for agent-native infrastructure: compute, settlement, and execution are separated cleanly to keep the product portable, composable, and agent-first.

## Why this product exists

Most AI agents are good at reasoning, but poor at operating real, stateful developer environments. They need:

- a disposable shell with toolchains already installed
- a session that survives multiple commands
- a clean payment flow that is explicit and machine-readable
- a path to release resources immediately when the task is done

Compute Router solves that by combining:

- an MCP-compatible control plane
- a resilient sandbox backend
- x402-based payment authorization on Celo
- a pay-for-actual-usage model rather than a flat subscription

## Product overview

Compute Router exposes four core actions through an MCP server:

- `provision_env` - create a sandbox instance and authorize a block of compute
- `exec` - run commands in the active shell
- `extend_ceiling` - extend the session by purchasing another priced block
- `release` - terminate the session and return its final cost

The platform is intentionally split into two services:

- `services/core` (Go) - pricing, session logic, MCP tooling, x402 verification and settlement
- `services/sandbox-adapter` (TypeScript, Cloudflare Worker) - thin gateway around `@cloudflare/sandbox`

This separation keeps core business logic portable while isolating the vendor-specific sandbox runtime behind a clean adapter boundary.

## Architecture

### 1. Core control plane

The Go service owns the user-facing workflow:

- validates startup configuration
- exposes MCP tools
- manages session lease state
- handles payment requirements and settlement
- calls the sandbox adapter for lifecycle actions

The implementation lives under:

- `services/core/cmd/server`
- `services/core/internal/mcp`
- `services/core/internal/payment`
- `services/core/internal/session`
- `services/core/internal/sandboxclient`

### 2. Sandbox adapter

The TypeScript worker wraps Cloudflare Sandbox and exposes only the internal operations the core service needs:

- `POST /boot`
- `POST /exec`
- `POST /destroy`

This adapter intentionally does not own pricing, billing, or business logic. It is a dumb runtime wrapper to keep the compute backend replaceable without rewriting the product model.

### 3. Payment model

Compute Router uses x402 as the payment protocol and settles in USDC on Celo.

The design supports:

- fixed-block billing as the MVP
- a metered and ceiling model as the stretch goal
- verification and settlement through a facilitator endpoint

The repo is aligned with the design in `docs/SPEC-100.md`, which documents the expected MCP payment flow, the container backend decision, and the long-term architecture constraints.

## Features

- Disposable shell creation with tooling preinstalled
- Agent-native workflow via MCP
- Celo-native x402 payment integration
- Session lease model with extension capability
- Immediate cleanup on release
- Cloudflare Sandbox backend with portable adapter boundary
- No hardcoded wallet-stack assumptions on the buyer side

## Stack

- Go 1.25+ for the MCP server and business logic
- TypeScript and Cloudflare Worker for the sandbox adapter
- Cloudflare Sandbox SDK for compute fulfillment
- x402 protocol for machine-readable payment negotiation
- Celo USDC for settlement

## Repository layout

```text
.
├── services/
│   ├── core/                      # Go MCP + payment + session logic
│   └── sandbox-adapter/          # TypeScript wrapper around Cloudflare Sandbox
├── docs/
│   ├── SPEC-100.md               # system design and protocol requirements
│   ├── ADR-001-language-choice.md
│   ├── OPS.md                    # operating guide
│   └── RUNBOOK.md                # local dev and deploy flow
├── tools/
│   └── txs/                      # synthetic traffic harness for beta testing
├── scripts/
│   └── smoke-test.sh             # local MCP smoke validation
├── AGENTS.md                     # agent handoff notes for maintainers
├── README.md                    # product overview
├── .env.example
├── Makefile
├── agent.json                   # public agent metadata draft
└── .well-known/
    └── agent.json               # public ERC-8004 agent metadata endpoint
```

## Local development

### Prerequisites

- Go installed
- Node.js 22+ for Wrangler and Cloudflare tooling
- Docker running locally for the sandbox adapter image build
- Celo testnet configuration for payment testing

### First-time setup

```bash
cp .env.example .env
cp services/sandbox-adapter/.dev.vars.example services/sandbox-adapter/.dev.vars
```

Then fill in the required runtime secrets and addresses in `.env` and `.dev.vars`.

### Install dependencies

```bash
cd services/core && go mod tidy
cd ../sandbox-adapter && npm install
cd ../../tools/txs && npm install
```

### Run the services

Terminal 1 - sandbox adapter:

```bash
cd services/sandbox-adapter
npx wrangler dev
```

Terminal 2 - core service:

```bash
cd services/core
go run ./cmd/server
```

Terminal 3 - smoke test:

```bash
bash scripts/smoke-test.sh
```

The smoke test validates the MCP handshake and confirms that a tool call without payment data returns a structured payment-required response rather than a silent failure.

## Deployment

The codebase is designed for a two-service deployment model:

### Sandbox adapter

Deploy the worker to Cloudflare using Wrangler:

```bash
cd services/sandbox-adapter
npx wrangler deploy
```

### Core service

The Go service can run on any container host, including Fly.io or a generic Docker-capable platform. The included Dockerfile and config are intended to be portable beyond any single backend.

## Security and operational notes

- Secrets are not hardcoded; required values must be configured via environment variables or Cloudflare secrets.
- The sandbox adapter and core service share a shared secret so the internal HTTP boundary is not open by default.
- Real payment flow should be tested on Celo testnet first.
- Do not run production payment workflows against mainnet until the payment verification and settlement path has been exercised in a testnet validation pass.

## Status

Compute Router is a real implementation aligned with the product direction described in the design docs, not a placeholder. It is designed to be a deployable, agent-native compute product with x402 payment flow and a sandbox runtime for short-lived build and test tasks.

The system is intended to be production-minded and near-launch ready from a product and architecture standpoint, with the remaining work focused on end-to-end environment validation, live facilitator compatibility, and deployment hardening.

## Design references

- `docs/SPEC-100.md` - product design and protocol requirements
- `docs/ADR-001-language-choice.md` - language split and architecture rationale
- `docs/OPS.md` - operational guidance
- `docs/RUNBOOK.md` - local run and deploy reference

If you are integrating with this system, start with the design docs and the operational guides before making changes to the runtime contracts.
