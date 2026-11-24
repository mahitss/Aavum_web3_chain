# Functional specification

## Session

The principal approves tUSD8 to the escrow for at least the hourly budget, then signs EIP-712 `SessionAuthorization` and the command center submits `registerSessionWithSig`.

The struct is `principal`, `agentKey`, `hourlyLimit`, `perTaskLimit`, `expiresAt`, `nonce`. The nonce is the current `nonces[principal]`. The domain is `Nexus402 Protocol`, version `1`, chain id `44005`, verifying contract equal to the deployed escrow.

## Hire

1. Orchestrator `POST`s `{prompt, task_type}` to `http://agent-worker:8080/api/v1/compute` in Compose, or `http://localhost:8080` when both run on the host.
2. Worker returns 402 and the headers in [ARCHITECTURE.md](../ARCHITECTURE.md).
3. Orchestrator checks the quote against `PER_TASK_LIMIT` and the local hourly remainder, then sends `lockTaskEscrow`.
4. On a mined `TaskEscrowLocked`, it retries with `X-Escrow-TaskId` and `X-Escrow-TxHash`.
5. Worker checks the pending task and calls `completeTaskWithProof`.
6. Job status becomes `SETTLED`, or `FAILED` with the error string.

## Refund and revoke

- Any account may call `refundExpiredTask` after `block.number > deadline`. Tokens return to the principal.
- Only the principal may call `emergencyRevokeSession(agentKey)`. New locks from that agent revert. Pending tasks are unchanged.

## Command center

The UI reads `GET /api/telemetry` and `GET /api/jobs`. Dispatch is `POST /api/jobs`. Kill switch calls `emergencyRevokeSession` from the connected wallet. **Refund expired task** calls `refundExpiredTask` for the newest job that has a task id and is not `SETTLED`. Copy buttons copy the terminal text. If `NEXUS_ESCROW_ADDRESS` is empty or the zero address, approve, revoke, and refund stay disabled.
