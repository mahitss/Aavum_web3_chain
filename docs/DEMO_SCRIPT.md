# Nexus402: 3-Minute Hackathon Demo Script

Record this only after `NexusEscrow` is deployed and one `./scripts/verify-e2e.sh` run has printed two real explorer URLs. Do not put a placeholder hash on screen. The on-screen revoke state in the current UI is `SESSION_REVOKED`.

Runtime target: 3:00.

### [0:00 - 0:30] Introduction and the M2M payment dilemma

- **Screen:** Title card. Nexus402. Tagline: agent-to-agent escrow on QDay Aevum Testnet. Chain id 44005.
- **Narrator:**
  "Autonomous agents can plan and call APIs. Paying each other is the gap. Wallet popups are built for a person. A raw private key on an agent is one loop away from an empty balance. Nexus402 is the payment path we built for that on the QDay Aevum Testnet. The human sets a cap once. After that, machines lock tUSD8, do the work, and claim it with a signature."

### [0:30 - 1:15] Architectural blueprint

- **Screen:** The architecture diagram in `README.md`.
- **Narrator:**
  "Three pieces. A Go orchestrator holds an ephemeral session key and is the only signer allowed to lock escrow. A Rust worker, Axum and Alloy, answers unpaid calls with HTTP 402 and the price, the token, and the task hash. Solidity on Aevum holds the tUSD8 until the worker submits an Ethereum-signed proof, or until the block deadline and a full refund. The session is EIP-712. It names the agent, the hourly limit, the per-task limit, the expiry, and a nonce. The domain is Nexus402 Protocol, version 1, chain 44005."

### [1:15 - 2:15] Live settlement

- **Screen:** Command center at http://localhost:3000. Mesh on the left, terminals on the right. Wallet already funded with tQDAY and tUSD8.
- **Actions:**
  1. Connect Wallet. Confirm the wallet is on chain 44005.
  2. Set the session: 0.50 tUSD8 per task, 5 tUSD8 per hour.
  3. Approve tUSD8 if the allowance is short, then Authorize Session and sign the EIP-712 prompt.
  4. Dispatch one hash task, or run `./scripts/verify-e2e.sh` in a terminal the camera can see.
- **Narrator:**
  "The operator signs the delegation. Then we send one task. The worker returns 402. The headers say Nexus402-v1, tUSD8, 250000 base units, the worker address, and the task hash. The orchestrator locks that amount with lockTaskEscrow. The worker sees the pending task, runs the hash, and calls completeTaskWithProof. The right-hand terminal should show two hashes. Open each one on explorer-test.qday.info. One is the lock. One is the claim. If either link 404s, stop. That run did not settle."

### [2:15 - 2:45] Emergency revoke

- **Screen:** The panic control. Title in the UI: PROTOCOL KILL SWITCH.
- **Actions:**
  1. Click the revoke control and confirm the principal's transaction.
  2. Wait until the panel reads `SESSION_REVOKED`.
  3. Dispatch another task. The job must fail. The error text must include `SessionInactive`.
- **Narrator:**
  "Revoke does not spend the escrow. It sets that session inactive. The next lock reverts on-chain with SessionInactive. A task that was already pending can still be claimed or refunded. New spend stops."

### [2:45 - 3:00] Close

- **Screen:** `forge test -vvv` in `contracts/`, showing 8 passed and 0 failed, then the GitHub repository https://github.com/mahitss/Aavum_web3_chain.
- **Narrator:**
  "The Foundry suite is eight tests, all passing in this repo: registration, caps, the hourly window, a full claim, a bad signature, a refund, and revoke. The source is public. Settlement on Aevum is the receipt you just opened. Thank you."

## Shot checklist

- [ ] Escrow address on screen matches `NEXUS_ESCROW_ADDRESS` in `.env`.
- [ ] Lock URL opens on https://explorer-test.qday.info and shows `TaskEscrowLocked` or the escrow call.
- [ ] Claim URL opens and shows `completeTaskWithProof` success.
- [ ] Revoke transaction is the principal, not the agent key.
- [ ] No `.env` contents, seed phrase, or private key in frame.
