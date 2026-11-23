# Nexus402 — Autonomous Agent-to-Agent Micro-Service Mesh & Escrow on QDay Aevum Testnet

Nexus402 lets one agent pay another for a single unit of work without a human sitting on the approval button. A Go orchestrator hits HTTP 402, locks tUSD8 in `NexusEscrow`, and a Rust worker releases that escrow only after an Ethereum-signed proof of the result.

## Core problem

High-frequency agent swarms cannot wait on wallet popups. A standing ERC-20 allowance is worse: a prompt-injected agent can drain it. Payroll and streaming apps move money on a clock, not on a completed task.

Nexus402 binds each payment to one HTTP request.

1. The worker answers an unpaid `POST /api/v1/compute` with **402 Payment Required** and the `X-Payment-*` headers (protocol, tUSD8, amount, receiver, task hash, timeout).
2. The orchestrator, using an EIP-712 session key, locks that exact amount in `NexusEscrow` for the task hash.
3. The worker re-checks the pending task on Aevum, runs deterministic compute, and claims with `completeTaskWithProof`.
4. If the deadline block passes first, `refundExpiredTask` returns the full amount to the principal.

The human signs the session once. After that, settlement is machine to machine, capped, and reversible by the principal.

## Live deployment

| | |
| --- | --- |
| Network | QDay Aevum Testnet |
| Chain ID | `44005` |
| RPC | https://rpc-test.qday.info |
| Explorer | https://explorer-test.qday.info |
| Settlement token (tUSD8, 6 decimals) | [`0x456569838f1d43009A91ebA4263147Afd7233483`](https://explorer-test.qday.info/address/0x456569838f1d43009A91ebA4263147Afd7233483) |
| NexusEscrow | **Not broadcast from this workspace yet.** Set `PRIVATE_KEY` for a tQDAY-funded deployer and run the script in [Local setup](#local-setup). |

The frontend reads the escrow from `NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS`. Do not hardcode a key. After broadcast, paste the logged address into `.env` and `frontend/.env.local`.

Broadcast attempted in this environment failed before a transaction was sent: `PRIVATE_KEY` is not set, and there is no funded deployer key on disk. Explorer links for session registration, escrow lock, and worker claim are filled in only after that funded run. Do not treat a placeholder as a verified deployment.

## Architecture

```
                    +---------------------------+
                    |  Next.js command center   |
                    |  RainbowKit / EIP-712     |
                    |  Three.js mesh + terminals|
                    +-------------+-------------+
                                  |
                    sign session  |  emergencyRevokeSession
                    approve tUSD8 |  read getSession / TVL
                                  v
+------------------+   HTTP 402    +------------------+
| Go orchestrator  | ------------> | Rust worker      |
| session key      | <------------ | compute + proof  |
| :8081            |  X-Escrow-    | :8080            |
+--------+---------+  TaskId       +--------+---------+
         |                                   |
         | lockTaskEscrow                    | completeTaskWithProof
         | (agent pays gas)                  | (worker pays gas)
         v                                   v
+------------------------------------------------------+
| NexusEscrow (Solidity 0.8.24, Cancun)                |
| tUSD8 pulled from the principal, not the agent key   |
| hourly window + per-task cap + refund + revoke       |
+------------------------------------------------------+
                          |
                          v
              QDay Aevum Testnet  #44005
```

The orchestrator key is the transaction sender for locks. tUSD8 moves `principal -> escrow -> worker` (or back to the principal on refund). The command center never holds the session private key.

## Security model

| Threat | Mitigation |
| --- | --- |
| Stolen or over-broad agent key | EIP-712 `SessionAuthorization` names the agent, both caps, expiry, and a monotonic nonce. Domain is `Nexus402 Protocol` / `1` / chain `44005` / the escrow address. |
| Spend burst inside a valid session | `perTaskLimit` caps one lock. `hourlyLimit` caps the sum inside `block.timestamp / 3600`. The window resets when the hour id advances. |
| Compromised agent still signing | Principal calls `emergencyRevokeSession(agentKey)`. Later locks revert with `SessionInactive`. |
| Worker never delivers | `deadline` is `block.number + durationBlocks`. After that block, `refundExpiredTask` returns the full escrow to the principal. |
| Forged completion | The claim signature is the Ethereum signed message over `keccak256(abi.encodePacked(taskId, resultHash))`. Recovery must equal `task.worker`. |
| Replayed session signature | `nonces[principal]` is part of the typed struct and increments on each successful registration. |
| Reentrancy on payout | Token moves use `SafeERC20` behind `ReentrancyGuard`. |

Amounts are tUSD8 base units (`1e6` = 1 tUSD8). The worker price is `250000` (0.25 tUSD8).

## Local setup

### Docker Compose

```bash
cp .env.example .env
# fill NEXUS_ESCROW_ADDRESS and the three testnet keys, then:
docker compose up --build
```

| Service | Port |
| --- | --- |
| `agent-worker` | 8080 |
| `agent-orchestrator` | 8081 |
| `frontend` | 3000 |

`WORKER_URL` inside Compose is `http://agent-worker:8080`. The browser still calls `NEXT_PUBLIC_API_URL=http://localhost:8081`.

### Manual

**Contracts** (Foundry, Solidity 0.8.24, Cancun, OpenZeppelin Contracts v5.0.2):

```bash
cd contracts
forge test -vvv
```

Deploy (legacy transactions, chain id 44005). The shell must export `PRIVATE_KEY` for an account that holds tQDAY. `TUSD8_ADDRESS` is optional and defaults to the address above.

```bash
cd contracts
forge script script/DeployAevum.s.sol:DeployAevum \
  --rpc-url https://rpc-test.qday.info \
  --broadcast \
  --legacy \
  -vvvv
```

Verify on the Blockscout-compatible explorer API:

```bash
forge verify-contract <DEPLOYED_NEXUS_ESCROW_ADDRESS> src/NexusEscrow.sol:NexusEscrow \
  --chain-id 44005 \
  --verifier-url https://explorer-test.qday.info/api \
  --verifier blockscout \
  --constructor-args $(cast abi-encode "constructor(address)" 0x456569838f1d43009A91ebA4263147Afd7233483)
```

**Worker** (Rust, Axum 0.7, Alloy 0.1):

```bash
cd agent-worker
cp .env.example .env
cargo run --release
```

**Orchestrator** (Go 1.22, go-ethereum v1.13.14, Gin on `:8081`):

```bash
cd agent-orchestrator
cp .env.example .env
go run ./cmd
```

**Command center** (Next.js 14, Wagmi v2, RainbowKit):

```bash
cd frontend
cp .env.example .env.local
npm install
npm run dev
```

Before the first paid task, from the connected principal wallet:

1. `approve` the escrow for at least the hourly tUSD8 budget.
2. Sign `SessionAuthorization` and submit `registerSessionWithSig` for the orchestrator agent address.
3. Keep the session key and the worker key funded with tQDAY. They pay gas. They do not custody tUSD8.

Then `POST http://127.0.0.1:8081/api/jobs` with `{"prompt":"...","task_type":"hash"}`, or use **Dispatch task** on the dashboard. `scripts/verify-e2e.sh` checks the unpaid 402, dispatches a job, and prints the lock and claim hashes once the job is `SETTLED`. `scripts/mesh-bench.sh` repeats that path for five tasks, then sends `emergencyRevokeSession` and expects task 6 to revert with `SessionInactive`.

## E2E testnet transactions

These three receipts are the submission evidence. They are produced by a funded run against the deployed escrow. This workspace does not have them yet.

| Step | Call | Explorer |
| --- | --- | --- |
| Session registration | `registerSessionWithSig` | `https://explorer-test.qday.info/tx/<SESSION_TX>` |
| Task escrow deposit | `lockTaskEscrow` (`TaskEscrowLocked`) | `https://explorer-test.qday.info/tx/<LOCK_TX>` |
| Proof and release | `completeTaskWithProof` (`TaskCompleted`) | `https://explorer-test.qday.info/tx/<CLAIM_TX>` |

## Submission checklist

- [x] Public GitHub repository: https://github.com/mahitss/Aavum_web3_chain
- [ ] Frontend deployed on Vercel and pointed at Aevum. Local build is `npm run build` in `frontend/`.
- [x] Contracts compile and `forge test` is green. Source verification on the explorer waits on the broadcast above.
- [x] This README, [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md), [docs/SUBMISSION.md](docs/SUBMISSION.md), and [docs/DEMO_SCRIPT.md](docs/DEMO_SCRIPT.md).
- [ ] 3-minute demo recorded from `docs/DEMO_SCRIPT.md` and linked on DoraHacks.
- [x] Tracked files ignore `.env`. No private keys are committed. `contracts/broadcast/` is gitignored.

The form text is [docs/SUBMISSION.md](docs/SUBMISSION.md). The demo script is [docs/DEMO_SCRIPT.md](docs/DEMO_SCRIPT.md).
