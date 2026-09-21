# Compute Router roadmap

## Now: MVP validation

**Status: deployed and under validation.** The live surface provides an MCP
Streamable HTTP endpoint, x402 exact-payment negotiation, Celo USDC/USDT
requirements, a Cloudflare Sandbox adapter, four MCP tools, an ERC-8004 agent
card, and a static discovery page.

The immediate gate is reproducible mainnet evidence for the scenarios in the
[validation plan](./validation-plan.md), including settlement hashes, latency
percentiles, expiry, extension, release, and failure behavior.

## Next: reliability

1. Persist leases and payment/session correlation outside process memory.
2. Make refund reservations durable and atomic before any refund transfer.
3. Add idempotency keys for provision, extension, release, and compensation.
4. Add structured metrics and request correlation for boot, payment, exec, and
   release latency.
5. Define output, command-duration, concurrency, and sandbox inactivity limits.

## Later: product breadth

1. Support deliberate runtime profiles or image versions instead of the
   current fixed `node-build` image.
2. Add a verified metered billing mode if the facilitator and accounting model
   support it.
3. Offer durable artifacts through an explicit storage service rather than
   pretending a sandbox is persistent.
4. Publish SDK examples for common MCP clients and wallet/payment adapters.

Roadmap status is intentionally evidence-driven: an item moves to complete
only when a scenario, production observation, and failure mode are documented.
