.PHONY: deps dev-core dev-adapter smoke-test build-core deploy-adapter deploy-core-fly txs-fund txs-journey-once txs-driver agent-register-testnet agent-register-mainnet

# Run this FIRST, locally (needs network — this repo was scaffolded
# without network access, so no go.sum/lockfile exists yet).
deps:
	cd services/core && go get github.com/modelcontextprotocol/go-sdk/mcp@latest && go mod tidy
	cd services/sandbox-adapter && npm install
	cd tools/txs && npm install
	cd tools/agent-identity && npm install

# Run the Go core service locally. Reads .env — copy .env.example first.
dev-core:
	cd services/core && set -a && . ../../.env && set +a && go run ./cmd/server

# Run the sandbox adapter locally via Wrangler. First run builds the
# Docker container image (2-3 minutes) — subsequent runs are fast.
# Requires Docker running locally and services/sandbox-adapter/.dev.vars
# (copy from .dev.vars.example).
dev-adapter:
	cd services/sandbox-adapter && npm run dev 2>/dev/null || npx wrangler dev

# Basic end-to-end smoke test against a locally-running core service.
smoke-test:
	bash scripts/smoke-test.sh

# Build a deployable core binary/image.
build-core:
	cd services/core && docker build -t compute-router-core .

# Deploy targets — pick what fits. Both are independent.
deploy-adapter:
	cd services/sandbox-adapter && npx wrangler deploy

deploy-core-fly:
	cd services/core && fly deploy

# --- Synthetic traffic (tools/txs) — see docs/OPS.md before running any of these ---

# One-time: fund the 10 party wallets with USDC. Requires
# tools/txs/.env filled in, including a funded FUNDER_PRIVATE_KEY.
txs-fund:
	cd tools/txs && npm run fund

# Single manual round trip for the first wallet in the CSV — run this
# before the full driver to debug the payment/exec flow in isolation.
txs-journey-once:
	cd tools/txs && npm run journey:once

# The full scheduler — runs indefinitely, spacing each wallet's
# journeys naturally. Ctrl-C to stop; state persists in
# tools/txs/data/driver-state.json so restarts don't replay everything.
txs-driver:
	cd tools/txs && npm run driver

# --- ERC-8004 agent identity (tools/agent-identity) — see internal-docs/ASD-STE-103.md ---

# Testnet dry run — ALWAYS do this before mainnet. Mints a real (if
# throwaway) public NFT on Celo Sepolia.
agent-register-testnet:
	cd tools/agent-identity && npm run register:testnet

# The real, credited registration. Costs real gas, mints a real public
# NFT that isn't casually undoable. Read internal-docs/ASD-STE-103.md first.
agent-register-mainnet:
	cd tools/agent-identity && npm run register:mainnet
