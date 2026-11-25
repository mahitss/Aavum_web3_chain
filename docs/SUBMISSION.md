# DoraHacks Submission Form: Nexus402

Copy the sections below into the form. The contract address, transaction hashes, Render URLs, Vercel URL, and demo video are still not recorded in this workspace. Do not submit a guessed address, a local signed hash, or a video link that 404s.

## Project Name

Nexus402

## Tagline

Autonomous Agent-to-Agent Micro-Service Mesh & Escrow Settlement on QDay Aevum Testnet

## Category / Track

Stablecoin Payment Applications / Agentic Infrastructure / Smart Contracts

## Short Pitch (100 Words)

Nexus402 provides native financial infrastructure for the machine economy. Built on the QDay Aevum Testnet, it enables autonomous AI agents to negotiate, hire, and settle micro-services using `tUSD8` without human involvement. Nexus402 combines native HTTP 402 Payment Required status headers with EIP-712 scoped session keys and conditional on-chain escrows. With rolling 1-hour velocity caps, cryptographic proof-of-execution settlement, and an on-chain emergency circuit breaker, Nexus402 balances machine autonomy with a human spend limit and a revoke path.

## Detailed Description

### The Problem

Agents can offer and consume APIs. Web3 payment screens still expect a person to confirm each transfer. Giving an agent an unrestricted key turns a bug, a prompt injection, or a retry loop into a drained balance.

### The Solution: Nexus402

1. **HTTP 402 paywall.** `POST /api/v1/compute` without an escrow id returns 402 with `X-Payment-Protocol`, `X-Payment-Token`, `X-Payment-Amount` (`250000`), `X-Payment-Receiver`, `X-Task-Hash`, and `X-Task-Timeout-Blocks` (`50`).
2. **EIP-712 session.** The principal signs `SessionAuthorization` for one agent, an hourly limit, a per-task limit, an expiry, and a nonce. Domain: `Nexus402 Protocol`, version `1`, chain id `44005`, verifying contract = the escrow.
3. **Escrow lock.** The agent calls `lockTaskEscrow`. tUSD8 moves from the principal to the contract. The agent cannot transfer that balance elsewhere.
4. **Proof claim.** The worker signs `keccak256(abi.encodePacked(taskId, resultHash))` with the Ethereum personal-sign prefix and calls `completeTaskWithProof`. OpenZeppelin 5.0.2 checks it with `MessageHashUtils.toEthSignedMessageHash` and `ECDSA.tryRecover`.
5. **Failure paths.** After `block.number > deadline`, `refundExpiredTask` returns the full amount to the principal. The orchestrator watchdog calls that function on its own and emits `TASK_REFUNDED`. `emergencyRevokeSession` stops new locks with `SessionInactive`. It does not rewrite a task that is already pending. The worker catalog is `GET /.well-known/nexus402.json`. `ENABLE_DEMO_FAULTS=true` is required before the worker-crash scenario returns HTTP 504.

## Tech Stack

- **Smart Contracts:** Solidity ^0.8.24, Foundry, OpenZeppelin Contracts v5.0.2 (`ReentrancyGuard`, `EIP712`, `SafeERC20`, `MessageHashUtils`, `ECDSA`).
- **Worker Node:** Rust 1.90 (image pin; edition 2024 crates), Axum 0.7, Tokio, Alloy 0.1.
- **Orchestrator Node:** Go 1.22, go-ethereum v1.13.14, Gin.
- **Frontend / Command Center:** Next.js 14.2 (App Router), React Three Fiber, Wagmi v2, Viem, RainbowKit, Tailwind CSS.

## Live Testnet Links

- **Chain:** QDay Aevum Testnet (chain id `44005`)
- **RPC:** https://rpc-test.qday.info
- **Explorer:** https://explorer-test.qday.info
- **Settlement token (tUSD8):** `0x456569838f1d43009A91ebA4263147Afd7233483`
- **Contract address:** Not recorded. `contracts/broadcast/` has no Aevum deployment.
- **Sample lock transaction:** Not recorded. See `docs/BENCHMARK_REPORT.md`.
- **Sample claim transaction:** Not recorded. See `docs/BENCHMARK_REPORT.md`.

## Deliverables and Links

- **GitHub repository:** https://github.com/mahitss/Aavum_web3_chain
- **Demo video (YouTube or Loom):** Not recorded.
- **Live demo site:** Not deployed. Render and Vercel URLs are unset. `scripts/preflight-check.sh` stays red until those URLs, the escrow bytecode, the funded wallets, and a 14-day session all check out.
- **Agent SDK:** `packages/nexus402-sdk` (`@nexus402/sdk`). `fetch402` is the three-line payment wrapper. Its only runtime dependency is `viem`.

## What was verified here

| Check | Result |
|---|---|
| `forge test` in `contracts/` on 25 Nov 2026 | 14 passed, 0 failed, including `contracts/test/NexusEscrow.fuzz.t.sol` |
| `go vet`, `go test`, `go build` in `agent-orchestrator/` | Passed |
| `cargo clippy -- -D warnings` in `agent-worker/` | Passed with 0 warnings |
| `npm run lint` and `npm run build` in `frontend/` | Passed |
| Docker images for worker, orchestrator, and frontend | Built |
| `docker compose up` with an empty `.env` | Worker exited `HexError(InvalidStringLength)`. Stack was stopped. |
| `DeployAevum` broadcast | Not run. `PRIVATE_KEY` is unset. `contracts/broadcast/` has no deployment. |
| `./scripts/verify-e2e.sh` against Aevum | Not run to `SETTLED` |
| Video upload and Vercel | Not done |

Record the missing links only from a funded Aevum run and from the uploaded recording. The script for the recording is [DEMO_SCRIPT.md](DEMO_SCRIPT.md). The benchmark table is [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md).
