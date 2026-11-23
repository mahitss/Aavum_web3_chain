# Nexus402 — 3-minute demo script

Record at 1920×1080. Have the command center on http://localhost:3000, the orchestrator log visible if you want a second beat, and the Aevum explorer tab ready. The principal wallet is connected to chain 44005 and already holds tUSD8 plus a little tQDAY. The orchestrator agent and the worker are funded for gas. The session is not registered at the start of the take.

Spoken lines are in quotes. Actions are in brackets.

## 0:00 – 0:35 | Vision

"QDay is built for machines that need to settle with each other faster than a person can click confirm. Nexus402 is that payment rail for agents."

"A normal agent wallet is a standing allowance. One bad prompt and the balance is gone. Streaming payroll does not help either: the money moves because a clock ticked, not because the work finished."

"We use the HTTP status the web already reserved for this. 402 Payment Required, plus a bilateral escrow on Aevum that only releases when the worker signs the result."

## 0:35 – 1:15 | Architecture

[Cut to the architecture diagram, or gesture across the 3D mesh: emerald orchestrator, cyan worker, gold escrow.]

"Two processes. A Go orchestrator holds a scoped session key. A Rust worker sells compute. They never share a key."

"The worker prices the request in headers: Nexus402-v1, tUSD8, 0.25 token, its own address, the keccak of the body, and a 50-block timeout. The orchestrator locks that exact amount from the human's tUSD8. The worker recomputes the task hash on-chain, runs the job, and claims with an Ethereum-signed proof."

"The session the human signs is narrow. A hard cap per task, a rolling one-hour ceiling, an expiry, and a nonce. If the key is stolen, this panic button calls emergencyRevokeSession and the chain starts reverting locks."

## 1:15 – 2:15 | Live walkthrough

[Command center. Point at the mesh: emerald sphere, cyan sphere, gold icosahedron, dashed 402 link, solid lines down to the contract.]

"This is the operator view on QDay Aevum, chain 44005."

[Connect Wallet. Confirm the network chip still reads QDay Aevum Testnet #44005.]

[Paste the orchestrator agent address. Set the hourly slider and the per-task slider above 0.25 tUSD8. Leave expiry at 1 hour.]

"I sign one EIP-712 authorization. Domain is Nexus402 Protocol, version 1, this escrow."

[Sign and register. Wait for the wallet confirmation. Badge flips to OPERATIONAL. Approve tUSD8 for the hourly amount.]

[Dispatch task. Keep the dual terminal in frame.]

Read the left terminal as it fills:

"402 challenge, a quarter tUSD8, task hash on the right. The orchestrator locks that on Aevum. It retries with the escrow task id. The worker checks the pending task, runs the job, submits the proof, and the claim returns 200."

[When the log hits 200 OK, the gold node throws the settlement particles from orchestrator to escrow to worker.]

[Click the lock hash, then the claim hash. Both open https://explorer-test.qday.info/tx/… . Show TaskEscrowLocked on the first receipt and TaskCompleted on the second. Say the amounts match 250000 base units.]

## 2:15 – 2:45 | Circuit breaker

[Back to the command center. Hit EMERGENCY REVOKE AGENT KEY. Confirm in the wallet. The status flashes SESSION_REVOKED and Dispatch task is disabled.]

"That transaction is the principal, not the agent. The key is dead on-chain."

[Dispatch the same task again.]

"The worker still returns 402. The lock reverts with SessionInactive. No second escrow is created. A compromised agent cannot keep spending."

## 2:45 – 3:00 | Close

"Nexus402 is a small mesh: one escrow, one 402 dialect, two agents, and a command center. It is open source, the caps and the revoke live in the contract, and the same bytecode is what we take to QDay mainnet once the session and the two receipts you just saw are the template."

[End on the explorer tab with both transaction hashes visible.]

## Shot list

| Time | On screen |
| --- | --- |
| 0:00 | Title card, then the mesh |
| 0:35 | Mesh labels: Go, Rust, NexusEscrow |
| 1:15 | Wallet connect, chain 44005 |
| 1:30 | Typed-data signature, then OPERATIONAL |
| 1:45 | Dual terminal through 402, lock, 200 |
| 2:05 | Explorer receipts |
| 2:15 | Red revoke button, then SESSION_REVOKED |
| 2:30 | Failed lock after revoke |
| 2:45 | Explorer, both hashes, end |
