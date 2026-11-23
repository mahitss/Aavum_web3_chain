# DoraHacks submission — Nexus402

Copy the fields below into the DoraHacks form. The elevator pitch is the 100-word text. Do not replace the escrow row with a made-up address.

## Project name

Nexus402

## Elevator pitch

Nexus402 is an autonomous agent-to-agent micro-service mesh and conditional escrow protocol built on the QDay Aevum Testnet. Combining the native HTTP 402 Payment Required standard with EIP-712 scoped session keys, Nexus402 enables AI agents to discover, negotiate, hire, and settle micro-services using tUSD8 with zero human intervention. Featuring rolling hourly velocity caps, proof-of-compute settlement, and an on-chain emergency circuit breaker, Nexus402 provides the foundational financial rails for machine-to-machine commerce.

## Tracks and categories

- Agentic Payments
- Stablecoin Infrastructure
- EVM Smart Contracts

## Verifiable testnet deployments

| Field | Value |
| --- | --- |
| Network | QDay Aevum Testnet |
| Chain ID | `44005` |
| RPC | https://rpc-test.qday.info |
| Explorer | https://explorer-test.qday.info |
| tUSD8 | `0x456569838f1d43009A91ebA4263147Afd7233483` (ERC-20, 6 decimals) |
| NexusEscrow | Not broadcast from this workspace. `PRIVATE_KEY` is unset, so there is no deployed address and no Blockscout verification to link yet. |

After a tQDAY-funded deployer is exported as `PRIVATE_KEY`:

```bash
cd contracts
forge script script/DeployAevum.s.sol:DeployAevum \
  --rpc-url https://rpc-test.qday.info \
  --broadcast \
  --legacy \
  -vvvv
```

The script logs the escrow address and the EIP-712 domain separator. Paste that address into `.env` as `NEXUS_ESCROW_ADDRESS` and into `frontend/.env.local` as `NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS`. Verify with:

```bash
forge verify-contract <DEPLOYED_NEXUS_ESCROW_ADDRESS> src/NexusEscrow.sol:NexusEscrow \
  --chain-id 44005 \
  --verifier-url https://explorer-test.qday.info/api \
  --verifier blockscout \
  --constructor-args $(cast abi-encode "constructor(address)" 0x456569838f1d43009A91ebA4263147Afd7233483)
```

The three receipt links the form should cite, once that run exists, are `registerSessionWithSig`, `TaskEscrowLocked`, and `TaskCompleted` on `https://explorer-test.qday.info/tx/...`. `scripts/verify-e2e.sh` prints the lock and claim URLs. `scripts/mesh-bench.sh` prints the same pair for five tasks and then checks that task 6 reverts with `SessionInactive`.

Repository: https://github.com/mahitss/Aavum_web3_chain

## Key highlights against the rubric

**Agentic mechanics.** The Go orchestrator posts compute with no payment header. The Rust worker answers `402` with `X-Payment-Protocol: Nexus402-v1`, the tUSD8 address, amount `250000`, the worker address, `keccak256` of the body, and a 50-block timeout. The orchestrator locks that quote, retries with `X-Escrow-TaskId`, and the worker claims. No wallet popup sits on that loop.

**Systems-grade engineering.** The orchestrator is Go 1.22 (Gin, go-ethereum, four goroutines on one job channel). The worker is Rust (Axum, Tokio, Alloy). The escrow is Solidity 0.8.24 on Cancun, tested with Foundry against OpenZeppelin Contracts v5.0.2. `forge test -vvv` covers the eight session, cap, proof, refund, and revoke cases.

**Zero-trust financial safety.** `perTaskLimit` caps one lock. `hourlyLimit` caps `spentInWindow` inside `block.timestamp / 3600`, then the counter resets on the next window id. `deadline = block.number + durationBlocks` makes `refundExpiredTask` return the full amount to the principal. `emergencyRevokeSession` is the principal's transaction. It clears `isActive`, and the cockpit shows `SESSION_REVOKED` and stops new local dispatches.

**3D command center.** The Next.js 14 cockpit connects to Aevum with RainbowKit and Wagmi v2, signs `SessionAuthorization`, and renders the orchestrator, the worker, and the escrow as a Three.js scene. Settlement pulses travel the edges. The split terminal shows the 402 dialogue on the left and explorer-linked receipts on the right.

## Packet in this repo

| Doc | Use |
| --- | --- |
| [README.md](../README.md) | Setup, deployment command, honest address status |
| [ARCHITECTURE.md](ARCHITECTURE.md) | State machine, 402 sequence, EIP-712, velocity window, proof |
| [SECURITY_AUDIT.md](SECURITY_AUDIT.md) | Four threat vectors and the Foundry tests that cover them |
| [DEMO_SCRIPT.md](DEMO_SCRIPT.md) | Three-minute recording shot list |
