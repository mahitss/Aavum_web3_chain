# Nexus402 pitch deck

Ten slides for a live pitch or a PDF export. Numbers below are copied from `docs/BENCHMARK_REPORT.md` and `contracts/.gas-snapshot`. This deck does not invent an Aevum transaction hash or a deployed escrow address.

## Slide 1 — Title

**Nexus402**

Autonomous agent-to-agent escrow on QDay Aevum Testnet.

- Chain id `44005`
- RPC: https://rpc-test.qday.info
- Explorer: https://explorer-test.qday.info
- tUSD8: `0x456569838f1d43009A91ebA4263147Afd7233483`
- Source: https://github.com/mahitss/Aavum_web3_chain
- Vercel dashboard and Render service URLs: not deployed in this workspace

## Slide 2 — The M2M payment dilemma

Agents can call APIs. Payment screens still wait for a person to confirm each transfer. Handing an agent an unrestricted key turns a bug, a prompt injection, or a retry loop into a drained balance. MetaMask on every call is too slow for a swarm. A raw key with no ceiling is too dangerous for a treasury.

## Slide 3 — Nexus402

Three controls on one payment:

1. **x402 V2 paywall.** `POST /api/v1/compute` without payment returns HTTP 402 and the `PAYMENT-REQUIRED` header.
2. **EIP-712 session key.** The principal signs `SessionAuthorization` for one agent, an hourly limit, a per-task limit, an expiry, and a nonce. Domain: `Nexus402 Protocol`, version `1`, chain id `44005`.
3. **Conditional escrow.** tUSD8 moves from the principal into `NexusEscrow`. The worker is paid only after a proof, a voucher batch, or an evaluator approval.

## Slide 4 — Four production layers

| Layer | Role |
|---|---|
| Solidity ^0.8.24, Foundry, OpenZeppelin Contracts v5.0.2 | Escrow, session keys, org vault, ERC-8004 passport, ERC-8183 evaluation |
| Rust worker (Axum, Alloy) | Prices the service, checks the paywall, signs the result |
| Go orchestrator (Gin, go-ethereum) | Locks escrow, runs the watchdog, signs receipts, exports the ledger |
| Next.js 14 and Three.js | Command center, 3D mesh, playground, judge tour |

The command center never stores the agent or worker private keys.

## Slide 5 — x402 V2 schemes

| Scheme | What the judge sees |
|---|---|
| `exact` | Lock the posted price, then settle the same amount |
| `upto` | Lock a ceiling and refund the unused change in the same transaction |
| `batch-settlement` | Ten off-chain vouchers, one on-chain `settleVoucherBatch` |
| `auth-capture` | Lock under an evaluator. Garbage output is rejected and the principal is refunded |

Public prices in base units: `web_brief` 150000, `ai_synthesis` 250000, `chain_audit` 500000, `deep_due_diligence` 600000, `micro_call` 50000. The `upto` ceiling used by the metered path is 500000.

## Slide 6 — Trustless safety stack

- Rolling hourly cap and per-task cap on each session key.
- Org vault: one `globalHourlyLimit` across every agent key, plus `emergencyPauseAllAgents`.
- Watchdog calls `refundExpiredTask` after the deadline and emits `TASK_REFUNDED`.
- ERC-8183 evaluator: approval pays the worker; rejection refunds the principal and lowers reliability.
- Per-session `emergencyRevokeSession` stops new locks with `SessionInactive`.

## Slide 7 — Standards and adoptability

- **ERC-8004.** `NexusIdentityRegistry` mints one agent passport per wallet.
- **`@nexus402/sdk`.** `fetch402` is the payment wrapper. Its runtime dependency is `viem`.
- **`@nexus402/mcp`.** `npx @nexus402/mcp` speaks MCP to the worker catalog for Claude and Cursor. With no agent key it stays in inspect mode and returns the HTTP 402 price, chain `44005`, and `taskHash`.

`v1.0.0-dorahacks` stays untagged until `scripts/preflight-check.sh` exits 0.

## Slide 8 — Benchmarks and gas

`docs/BENCHMARK_REPORT.md` records no live Aevum measurement in this workspace. The deployer key is unset, so there is no confirmed `SessionAuthorized`, `TaskEscrowLocked`, `TaskCompleted`, `TaskRefunded`, or `SessionRevoked` transaction to link. HTTP 402 latency, confirmation time, and Aevum gas for those four flows are **Not recorded**.

`contracts/.gas-snapshot` is Foundry gas for the whole test, including setup. It is not a measured Aevum transaction receipt.

| Foundry test | Gas in `contracts/.gas-snapshot` |
|---|---:|
| `NexusEscrowTest:test_EndToEnd_LockAndCompleteProof()` | 798408 |
| `NexusEscrowTest:test_RefundExpiredTask()` | 753330 |
| `NexusEscrowTest:test_RollingHourlyVelocityCap()` | 898861 |
| `NexusEscrowVoucherTest:test_TenOffChainVouchersSettleInOneTransaction()` | 765960 |
| `NexusEscrowMeteredTest:test_MeteredSplitReturnsChangeAndCreditsWindow()` | 630761 |
| `NexusEscrowEvaluatorTest:test_GarbageOutputRefundsPrincipalAndSlashesReliability()` | 618540 |
| `NexusEscrowOrgVaultTest:test_EmergencyPauseBlocksEveryActiveSession()` | 554003 |

The latest local `forge snapshot` run that wrote this file passed 31 tests and failed 0.

## Slide 9 — Verified on-chain proofs

`scripts/generate-submission-proofs.sh` writes `docs/VERIFIED_ONCHAIN_PROOFS.md`. A hash is linked only after the orchestrator returns it, and it is marked confirmed only when `https://explorer-test.qday.info/api/v2/transactions/{hash}` returns HTTP 200.

Until that script exits 0, every scenario row stays `not captured`. Do not read a local Foundry gas number as an Aevum explorer link.

Scenarios the script dispatches, in order: `happy_path` (`exact`), `metered_batch` (`upto`), `voucher_burst` (`batch-settlement`), `worker_timeout_refund` (`exact`), `evaluator_dispute_slash` (`auth-capture`), `multi_hop_swarm` (`exact`), `velocity_attack` (`exact`).

## Slide 10 — QDay mainnet roadmap

After the hackathon, on QDay mainnet:

1. **Cross-agent liquidity pools** so one treasury can fund many swarms without a fresh approval per agent.
2. **ZK proof-of-inference verifiers** so an evaluator can check a proof instead of re-reading the raw model output.
3. **Multi-stablecoin settlement** so the same session and escrow flow can settle more than tUSD8.

Mainnet work starts from a funded Aevum deployment whose addresses match `README.md`, `docs/SUBMISSION.md`, telemetry, and `docs/VERIFIED_ONCHAIN_PROOFS.md`.
