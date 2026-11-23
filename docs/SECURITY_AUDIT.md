# Nexus402 security audit

This matrix covers the four threats the session-key escrow is built to stop. It describes the code in `contracts/src/NexusEscrow.sol` (Solidity 0.8.24, OpenZeppelin Contracts v5.0.2) and the checks in the Go orchestrator and Rust worker. It is a design review of this repository, not a third-party audit report.

Amounts are tUSD8 base units. The worker quote is `250000` (0.25 tUSD8).

## Vector 1 — Stolen or leaked agent private key

**Threat.** An attacker reads `ORCHESTRATOR_AGENT_PRIVATE_KEY` from the orchestrator environment and signs transactions as the ephemeral session key.

**What that key can do.** `lockTaskEscrow` authorizes the caller by `msg.sender`. The key can lock a new escrow only while the session is active, unexpired, and inside both caps. Each lock pulls tUSD8 from the principal's allowance, not from the agent. The contract has no withdraw, no `transfer`, and no function that sends the agent's balance anywhere. The agent is expected to hold tQDAY for gas and zero tUSD8.

**Caps that still apply after theft.**

| Check | Revert |
| --- | --- |
| `amount <= perTaskLimit` | `ExceedsPerTaskLimit` |
| `spentInWindow + amount <= hourlyLimit` inside `block.timestamp / 3600` | `ExceedsHourlyLimit` |
| `block.timestamp < expiresAt` | `SessionExpired` |
| `isActive` | `SessionInactive` |

**Circuit breaker.** `emergencyRevokeSession(agentKey)` is callable only by the principal (`msg.sender` is the session owner). It sets `isActive` to false and emits `SessionRevoked(principal, agentKey, "emergency")`. The next `lockTaskEscrow` from that key reverts with `SessionInactive`. The command center exposes this as **EMERGENCY REVOKE AGENT KEY**, then shows `SESSION_REVOKED` and disables local dispatch.

**Residual risk.** Locks that are already `PENDING` stay locked until the worker claims or someone calls `refundExpiredTask` after `deadline`. Revoke stops new locks. It does not claw back a task that is still inside its deadline. Gas already sitting on the stolen key can be spent by the attacker. That is native tQDAY, not the principal's tUSD8.

## Vector 2 — Prompt injection and runaway spend

**Threat.** A loop in the orchestrator, or a prompt that keeps calling paid endpoints, submits `lockTaskEscrow` without a person on each call.

**Mitigation.** The session stores `spentInWindow` for `currentWindow = block.timestamp / 3600`. Before the token move, the contract adds `amount` and reverts with `ExceedsHourlyLimit` when the sum would pass `hourlyLimit`. The counter resets to zero only when a later call observes a strictly greater window id. `perTaskLimit` is checked on every lock and does not reset hourly, so one call cannot spend the whole hour if the per-task ceiling is lower.

The orchestrator also rejects a 402 quote locally when it exceeds `PER_TASK_LIMIT` or the remaining `HOURLY_LIMIT`, and it drops that reservation if the lock transaction is never sent. Two orchestrators that share one session can each pass the local check. The contract counter is the one that stops the second lock.

Registration cannot be replayed to raise the cap. `nonces[principal]` is inside the EIP-712 struct and increments on success. The domain separator includes chain id `44005` and the escrow address when the contract is deployed on Aevum.

## Vector 3 — Offline or malicious worker

**Threat.** The worker returns 402, the agent locks tUSD8, and the worker never returns a body or never calls `completeTaskWithProof`.

**Mitigation.** `lockTaskEscrow` sets `deadline = block.number + durationBlocks`. The worker's challenge header `X-Task-Timeout-Blocks` is `50`, and the orchestrator passes that value through as `durationBlocks`. While `block.number <= deadline`, `refundExpiredTask` reverts with `DeadlineNotPassed`. After that block, any caller can refund. The contract requires `PENDING`, sets `status` to `REFUNDED`, then `safeTransfer`s the full `amount` back to `task.principal` and emits `TaskRefunded`.

The worker will not claim a task that fails `getTask` checks: status must be `PENDING` (enum value 1), `worker` must be the worker's own address, `amount` must be at least `250000`, and `taskHash` must equal `keccak256` of the same request body. A lock for a different body or a smaller amount does not get a proof.

**Residual risk.** Funds sit in the escrow until the deadline block and until someone sends `refundExpiredTask`. The principal pays the refund gas. A worker that delivers a worthless but correctly signed result still gets paid. The contract checks the signature and the task hash, not the meaning of the output.

## Vector 4 — Reentrancy on proof settlement

**Threat.** `task.worker` is a contract. `safeTransfer` calls it, and that contract calls `completeTaskWithProof` or `refundExpiredTask` again in the same transaction.

**Mitigation.** `lockTaskEscrow`, `completeTaskWithProof`, and `refundExpiredTask` use OpenZeppelin `ReentrancyGuard`. A re-entering call hits the guard before business logic.

Those three functions also follow checks-effects-interactions:

| Function | Effect written first | Interaction after |
| --- | --- | --- |
| `lockTaskEscrow` | `spentInWindow` updated, task stored as `PENDING` | `safeTransferFrom(principal, escrow, amount)` |
| `completeTaskWithProof` | `resultHash` set, `status = COMPLETED` | `safeTransfer(worker, amount)` |
| `refundExpiredTask` | `status = REFUNDED` | `safeTransfer(principal, amount)` |

A re-entrant claim sees `COMPLETED` and reverts with `TaskNotPending` even if the guard were absent. `SafeERC20` refuses a `false` return from tUSD8. `registerSessionWithSig` and `emergencyRevokeSession` do not move tokens. Revoke only flips `isActive`.

## Matrix

| Vector | Exploit | Control in this repo | Test |
| --- | --- | --- | --- |
| Stolen agent key | Drain via the session key | No withdraw path. `perTaskLimit`, `hourlyLimit`, expiry. Principal `emergencyRevokeSession`. | `test_EmergencyRevokeCircuitBreaker` |
| Runaway loop | Repeat `lockTaskEscrow` | Window `block.timestamp / 3600`. `ExceedsHourlyLimit`. Per-task ceiling stays in force. | `test_RollingHourlyVelocityCap`, `test_RevertWhen_ExceedingPerTaskLimit` |
| Silent worker | Lock and never claim | `deadline = block.number + durationBlocks`. `refundExpiredTask` returns the full amount. | `test_RefundExpiredTask` |
| Reentering worker | Claim twice from the token callback | `ReentrancyGuard` plus status write before `safeTransfer`. | `test_EndToEnd_LockAndCompleteProof`, `test_RevertWhen_UnauthorizedWorkerClaims` |
| Forged session | Register a session the principal did not sign | EIP-712 recover must equal `principal`. Nonce increments. | `test_RegisterSessionWithValidSig`, `test_RevertWhen_InvalidSignature` |
| Forged proof | Another key signs `keccak256(taskId, resultHash)` | Recovered signer must equal `task.worker`. | `test_RevertWhen_UnauthorizedWorkerClaims` |

`scripts/mesh-bench.sh` runs five live settlements and then a sixth dispatch after `emergencyRevokeSession`. The sixth lock is expected to fail with `SessionInactive`. That script needs a funded principal, a deployed escrow, and the two agent keys. It is not a substitute for `forge test`.
