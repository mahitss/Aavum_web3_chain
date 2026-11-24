<div align="center">
  <h1>NEXUS402</h1>
  <p><strong>Autonomous Agent-to-Agent Micro-Service Mesh & Escrow Settlement on QDay Aevum Testnet</strong></p>
  <p>
    <a href="https://explorer-test.qday.info"><img src="https://img.shields.io/badge/Network-QDay%20Aevum%20Testnet-06b6d4?style=flat-square" alt="Network" /></a>
    <a href="https://soliditylang.org/"><img src="https://img.shields.io/badge/Solidity-0.8.24-363636?style=flat-square&logo=solidity" alt="Solidity" /></a>
    <a href="https://www.rust-lang.org/"><img src="https://img.shields.io/badge/Worker-Rust%20%7C%20Axum%20%7C%20Alloy-orange?style=flat-square&logo=rust" alt="Rust" /></a>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Orchestrator-Go%20%7C%20Geth%20%7C%20Gin-00ADD8?style=flat-square&logo=go" alt="Go" /></a>
    <a href="https://nextjs.org/"><img src="https://img.shields.io/badge/Frontend-Next.js%2014%20%7C%20Three.js-black?style=flat-square&logo=next.js" alt="Next.js" /></a>
  </p>
</div>

---

## Executive Summary

**Nexus402** is a machine-to-machine micro-commerce network. An autonomous client can hire a compute worker and settle in `tUSD8` without a human confirming every payment.

The worker answers an unpaid call with **HTTP 402 Payment Required**. The Go orchestrator, holding an **EIP-712 session key**, locks that price in **NexusEscrow** on the QDay Aevum Testnet. The Rust worker checks the pending escrow, runs the task, and claims with an Ethereum-signed proof. The human principal sets the hourly cap, the per-task cap, and can revoke the session.

---

## Live Testnet Deployments (QDay Aevum Testnet)

| Parameter | Network Value |
|---|---|
| **Chain ID** | `44005` |
| **RPC Endpoint** | https://rpc-test.qday.info |
| **Explorer** | https://explorer-test.qday.info |
| **Native Gas** | `tQDAY` (18 decimals) |
| **Official Settlement Token (tUSD8)** | [`0x456569838f1d43009A91ebA4263147Afd7233483`](https://explorer-test.qday.info/address/0x456569838f1d43009A91ebA4263147Afd7233483) (6 decimals) |
| **NexusEscrow Contract** | **Not deployed from this repository.** `contracts/broadcast/` is empty. |

Do not paste a guessed address into the table. After `make deploy-aevum` logs `NexusEscrow`, put that address in `.env` as `NEXUS_ESCROW_ADDRESS` and `NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS`, then replace this row.

---

## Systems Architecture & Information Flow

```
+-----------------------------------------------------------------------------+
|                       HUMAN PRINCIPAL / OPERATOR                            |
|           (RainbowKit on chain 44005 / EIP-712 SessionAuthorization)        |
+----------------------------------+------------------------------------------+
                                   | 1. EIP-712 delegation
                                   |    hourlyLimit, perTaskLimit, expiresAt, nonce
                                   v
+-------------------------------+  2. POST /api/v1/compute  +-------------------------------+
|     CLIENT ORCHESTRATOR       +------------------------->|        COMPUTE WORKER         |
|             (Go :8081)        |<-------------------------+            (Rust :8080)       |
|  - Session key signs locks    |  3. HTTP 402              |  - 402 challenge headers      |
|  - Retries with task id       |     X-Payment-*           |  - Proof claim via Alloy      |
+---------------+---------------+  4. X-Escrow-TaskId       +---------------+---------------+
                |                    X-Escrow-TxHash                        |
                | 5. lockTaskEscrow()                                      | 6. completeTaskWithProof()
                |    tUSD8 principal -> escrow                             |    tUSD8 escrow -> worker
                v                                                          v
+-----------------------------------------------------------------------------+
|                    NEXUS ESCROW (QDay Aevum Testnet)                         |
|  - Spend window is block.timestamp / 3600                                   |
|  - Proof is eth-signed keccak256(abi.encodePacked(taskId, resultHash))      |
|  - Refund when block.number > deadline                                      |
|  - emergencyRevokeSession freezes new locks; it does not move a pending task|
+-----------------------------------------------------------------------------+
```

tUSD8 is pulled from the principal's allowance. The orchestrator key and the worker key pay gas in tQDAY. They do not hold the settlement balance. The command center never stores those private keys.

---

## Security Model & Threat Matrix

| Threat | What the code does |
|---|---|
| Stolen session key drains the wallet | `lockTaskEscrow` is the agent's only token path. It cannot `transfer`, `withdraw`, or set an allowance. The pull is `safeTransferFrom(principal, escrow, amount)`, capped by `perTaskLimit` and the hourly window. |
| Prompt-injection spend loop | After the hour id `block.timestamp / 3600` advances, `spentInWindow` resets. Inside the hour, `spentInWindow + amount > hourlyLimit` reverts with `ExceedsHourlyLimit`. |
| Worker takes the lock and goes offline | `deadline = block.number + durationBlocks` (the worker asks for 50 blocks). After that, `refundExpiredTask` returns the full amount to the principal. Anyone may call it. The principal receives the tokens. |
| Forged completion | `MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)))`. `ECDSA.tryRecover` must return `task.worker` or the call reverts `InvalidProofSignature`. |
| Replayed session signature | The EIP-712 struct includes `nonces[principal]`. Registration increments it. Domain is `Nexus402 Protocol` / `1` / chain `44005` / the escrow address. |
| Compromised agent still locking | The principal calls `emergencyRevokeSession(agentKey)`. Later locks revert `SessionInactive`. An already `PENDING` task stays pending until claim or refund. |
| Reentrancy on payout | `ReentrancyGuard` plus OpenZeppelin `SafeERC20`. |

Worker price is `250000` base units (0.25 tUSD8). Local orchestrator defaults are `PER_TASK_LIMIT=1000000` and `HOURLY_LIMIT=10000000`. Those local checks are not a substitute for the contract caps.

The longer write-up is [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md).

---

## Turnkey Quickstart

### Prerequisites

- Docker and Docker Compose
- Foundry: `curl -L https://foundry.paradigm.xyz | bash` then `foundryup`
- Go 1.22+
- Rust 1.90+ (the worker image is `rust:1.90-slim-bookworm`; current crates need edition 2024)

### Setup and run

```bash
git clone https://github.com/mahitss/Aavum_web3_chain.git
cd Aavum_web3_chain

cp .env.example .env
# Set PRINCIPAL_*, ORCHESTRATOR_AGENT_PRIVATE_KEY, WORKER_PRIVATE_KEY,
# and NEXUS_ESCROW_ADDRESS. Empty keys crash the worker on startup.

docker compose up --build -d
./scripts/verify-e2e.sh
```

Dashboard: http://localhost:3000. Worker: http://localhost:8080. Orchestrator: http://localhost:8081.

`docker compose up` on 24 Nov 2026 built all three images, then the worker exited with `HexError(InvalidStringLength)` because `WORKER_PRIVATE_KEY` was empty. The orchestrator and frontend wait until that healthcheck passes. Fill `.env` before expecting `verify-e2e.sh` to pass.

`make deploy-aevum` broadcasts `DeployAevum` and writes the logged address into `.env`. It exits if `PRIVATE_KEY` and `PRINCIPAL_PRIVATE_KEY` are both empty. No contract was broadcast in this workspace.

### Contract tests

Run from `contracts/` on 24 Nov 2026:

`forge test -vvv --gas-report` — **8 passed, 0 failed, 0 skipped**.

The suite is `test_RegisterSessionWithValidSig`, `test_RevertWhen_InvalidSignature`, `test_RevertWhen_ExceedingPerTaskLimit`, `test_RollingHourlyVelocityCap`, `test_EndToEnd_LockAndCompleteProof`, `test_RevertWhen_UnauthorizedWorkerClaims`, `test_RefundExpiredTask`, `test_EmergencyRevokeCircuitBreaker`.

`scripts/verify-e2e.sh` and `scripts/mesh-bench.sh` are **UNVERIFIED** against Aevum. They need a deployed escrow, a registered session, and tQDAY on both agent keys. `mesh-bench.sh` dispatches 10 parallel jobs, then looks for `ExceedsHourlyLimit`, then `emergencyRevokeSession` and `SessionInactive`.

---

## Submission status

| Item | Status |
|---|---|
| Source | https://github.com/mahitss/Aavum_web3_chain |
| Contract tests | 8 passed, this workspace, 24 Nov 2026 |
| NexusEscrow on Aevum | Not broadcast |
| Lock and claim transactions | Not mined |
| Vercel frontend | Not deployed |
| Demo video | Not recorded. Script: [docs/DEMO_SCRIPT.md](docs/DEMO_SCRIPT.md) |
| Form text | [docs/SUBMISSION.md](docs/SUBMISSION.md) |

Protocol detail: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
