# DoraHacks Submission Payload: Nexus402

Copy sections 1 through 6 into the form. Three fields are still tokens because this checkout has no broadcast and no recording. Fill those from the asset check at the bottom. Do not invent an address or a transaction hash.

## 1. Project Information

- **Project Name:** Nexus402
- **Tagline:** Autonomous Agent-to-Agent Micro-Service Mesh & Conditional Escrow Protocol on QDay Aevum Testnet
- **Track:** Aevum Builder Challenge #1 (Stablecoin Payment Applications)
- **Primary Category:** Agentic Payments / Infrastructure / Smart Contracts

## 2. Project Description

Nexus402 is an autonomous machine-to-machine (M2M) payment network that enables AI agents to discover, negotiate, hire, and settle micro-services using `tUSD8` stablecoins on the QDay Aevum Testnet.

Existing blockchain payment interfaces are engineered for humans clicking "Approve" on browser wallets. When developers give autonomous agents raw wallet access, they expose users to runaway spend loops and prompt-injection theft. Nexus402 eliminates both problems by combining the web-native HTTP 402 Payment Required standard with scoped on-chain escrow contracts and EIP-712 session keys.

### Core Workflow:

1. **HTTP 402 Handshake:** A client agent queries an API endpoint. The worker service responds with standard HTTP 402 headers detailing the price in tUSD8, recipient wallet, and cryptographic task commitment hash.
2. **Autonomous Escrow Lock:** The client's ephemeral session key locks funds in `NexusEscrow.sol` on Aevum Testnet within strict hourly spend limits.
3. **Cryptographic Proof Settlement:** The worker executes the compute, produces a result hash, signs it with its ECDSA key, and submits it on-chain to atomically unlock payout.
4. **Emergency Circuit Breaker:** Human principals retain sovereignty with instant on-chain revocation and timelocked refund guarantees.

## 3. Technology Stack & Systems Architecture

- **Smart Contracts:** Solidity ^0.8.24, Foundry, OpenZeppelin v5.0 (EIP-712, ReentrancyGuard, SafeERC20).
- **Worker Compute Node:** Rust 1.78+, Axum, Tokio, Alloy-rs (v0.1), Tower-HTTP.
- **Orchestrator Node:** Go 1.22, go-ethereum (ethclient), Gin, Goroutine worker pools.
- **Command Center & UI:** Next.js 14 (App Router), Three.js / React Three Fiber, Wagmi v2, Viem, RainbowKit, Tailwind CSS.

## 4. Verifiable Testnet Artifacts

- **Target Network:** QDay Aevum Testnet
- **Chain ID:** 44005
- **RPC URL:** https://rpc-test.qday.info
- **Explorer:** https://explorer-test.qday.info
- **Settlement Stablecoin (tUSD8):** `0x456569838f1d43009A91ebA4263147Afd7233483`
- **Deployed NexusEscrow Contract:** `<INSERT_DEPLOYED_CONTRACT_ADDRESS>`
- **Sample Lock Transaction:** `<INSERT_EXPLORER_TX_LINK>`
- **Sample Claim Transaction:** `<INSERT_EXPLORER_TX_LINK>`

## 5. Security & Risk Controls

- **Least-Privilege Session Keys:** Ephemeral keys only have authority to deposit into escrow; they can never transfer or withdraw principal funds.
- **Velocity Spend Limits:** On-chain rolling 1-hour window tracking ensures an agent cannot exceed pre-authorized spend caps even if compromised.
- **Replay Protection:** Domain-separated EIP-712 signatures tied to Aevum's Chain ID (44005) with sequential per-principal nonces.
- **Timelocked Refunds:** 100% principal refund path if a worker node fails to submit execution proof before the block deadline.
- **Instant Revocation:** One-click on-chain circuit breaker immediately invalidates active session keys.

## 6. Open Source Deliverables

- **GitHub Repository:** https://github.com/mahitss/Aavum_web3_chain
- **Docker Compose:** Run the entire system locally via `docker compose up --build`.
- **Demo Video:** `<INSERT_YOUTUBE_OR_LOOM_LINK>`

## Asset check

Checked in this workspace before the form is pasted:

1. `contracts/broadcast/` has no deployment artifact. `DeployAevum` has not been broadcast, because `PRIVATE_KEY` is not set. Leave `<INSERT_DEPLOYED_CONTRACT_ADDRESS>` until the script logs an address, then paste that address here and into `NEXUS_ESCROW_ADDRESS` / `NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS`.
2. `scripts/mesh-bench.sh` has not produced lock or claim hashes. Leave both `<INSERT_EXPLORER_TX_LINK>` tokens until the bench prints `TaskEscrowLocked` and `TaskCompleted` explorer URLs.
3. The recording from `docs/DEMO_SCRIPT.md` is not uploaded. Leave `<INSERT_YOUTUBE_OR_LOOM_LINK>` until the 3:30 cut is public.
4. Source on `main`: https://github.com/mahitss/Aavum_web3_chain
