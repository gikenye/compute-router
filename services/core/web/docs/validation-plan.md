# Compute Router validation plan

This plan answers “does the deployed service do what it claims?” with
observable checks. Record the UTC timestamp, deployment version, request ID,
session ID (if any), latency, exit code, and Celo transaction hashes for every
paid scenario. Run the read-only checks first.

## Read-only production checks

```bash
BASE_URL=https://mcp.computerouter.wtf bash scripts/production-health.sh
```

This checks the landing page, documentation, agent card, MCP handshake,
`tools/list`, and the structured payment challenge. It never signs a payment,
boots a sandbox, or spends funds.

## Paid mainnet scenarios

Use a dedicated wallet with only the amount needed for the run. Start with a
`0.02` ceiling, verify the payment recipient and Celo chain before signing,
and stop if the observed price or recipient differs from the agent card and
deployment configuration.

| ID | Scenario | Procedure | Expected evidence |
|---|---|---|---|
| M1 | Happy path | Provision, run `printf hello`, inspect output, release. | One provision settlement, exit code 0, release response, sandbox gone. |
| M2 | Build workload | Provision and run `node --version`, `npm --version`, create a small package, run its test, release. | Toolchain is present; stdout/stderr and exit code are preserved. |
| M3 | State persistence | Run `mkdir -p /tmp/cr-test && printf ok > /tmp/cr-test/state`; execute `cat /tmp/cr-test/state` in a second call. | Second call sees `ok` in the same session. |
| M4 | Non-zero command | Execute `sh -c 'printf failure >&2; exit 7'`. | MCP call succeeds as a tool result; `exit_code` is 7 and stderr contains `failure`. |
| M5 | Extension | Provision with `0.02`, extend by `0.02`, run a command after the original expiry boundary. | Second settlement hash, increased ceiling, extended expiry, command still works. |
| M6 | USDT payment | Repeat M1 selecting USDT in the payment challenge. | Requirement uses the configured USDT token; settlement succeeds. |
| M7 | Explicit release | Provision, release, then call `exec` with the old ID. | Release returns final cost; subsequent exec is rejected. |
| M8 | Expiry | Provision, wait past the lease, then call `exec`. | Core rejects the expired session; verify separately whether adapter cleanup occurred. |

## Payment and failure edge cases

These should be run with a wallet and a facilitator test procedure that can
produce the stated failure without risking a large transfer:

| ID | Edge case | Expected result |
|---|---|---|
| E1 | First provision omits `payment_data` | Structured `payment_required` with both supported assets; no sandbox boot. |
| E2 | Wrong asset or wrong recipient | Verification fails; no sandbox and no settlement. |
| E3 | Wrong chain or expired payment | Verification fails explicitly; caller does not retry the same payload. |
| E4 | Adapter unavailable during boot | Provision fails before settlement; no active session remains. |
| E5 | Facilitator returns an ambiguous outcome | Sandbox is destroyed; refund review record contains the payment identity and transaction hash. |
| E6 | Duplicate retry after a successful settlement | Must not create an unintended second active session; investigate payment and session correlation. |
| E7 | Invalid session ID | `exec`, `extend_ceiling`, and `release` return clear errors without adapter calls. |
| E8 | Concurrent extension/release | No negative balance or resurrected lease; inspect ledger and session state. |

## Bottleneck measurements

For M1–M6 capture separately: MCP round-trip latency, payment verification
latency, settlement latency, sandbox boot latency, first-command latency,
command latency, and release latency. Repeat each scenario at least five times
and report median and p95. Also record cold versus warm adapter behavior,
command duration, output size, and concurrent session count.

The most important current hypotheses are payment/facilitator latency, cold
Cloudflare Sandbox boot time, fixed-block overpayment for short commands, and
in-memory session state during a core restart. A passing happy path does not
prove restart recovery, refund idempotency, or capacity under concurrency.

## Current claims that are not yet proven

- Billing is fixed-block, not actual per-second usage billing.
- Session state is held in core memory; a core restart can lose leases.
- `template` is not a runtime image selector.
- The public smoke test does not prove signed settlement or real sandbox
  execution. Mainnet evidence must come from the paid scenarios above.
