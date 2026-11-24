# Nexus402 Technical Specifications & System Protocol

This file describes the code in this repository. It is not a claim that a particular address is live on Aevum.

## 1. HTTP 402 Handshake Specification

`POST /api/v1/compute` with no escrow header returns **402**. The body is JSON (`error`, `message`, `required_amount`, `token`, `task_hash`). These response headers are always set:

| Header Name | Type | Description |
|---|---|---|
| `X-Payment-Protocol` | string | `Nexus402-v1` |
| `X-Payment-Token` | hex address | tUSD8: `0x456569838f1d43009A91ebA4263147Afd7233483` |
| `X-Payment-Amount` | uint256 string | `250000` (0.25 tUSD8, 6 decimals) |
| `X-Payment-Receiver` | hex address | The worker key derived from `WORKER_PRIVATE_KEY` |
| `X-Task-Hash` | bytes32 hex | `keccak256` of the serde_json encoding of `{prompt, task_type}` |
| `X-Task-Timeout-Blocks` | uint256 string | `50` |

`X-Task-Hash` is not the hash of the raw HTTP bytes. Field order is the Rust struct order: `prompt`, then `task_type`. The orchestrator must hash the same object or the worker rejects the escrow.

After `lockTaskEscrow` mines, the orchestrator retries the same JSON body with:

| Header Name | Type | Description |
|---|---|---|
| `X-Escrow-TaskId` | bytes32 hex | `taskId` from `TaskEscrowLocked` (the worker also accepts `X-Escrow-TaskId` spelled `X-Escrow-Taskid` in the legacy header `x-escrow-taskid`) |
| `X-Escrow-TxHash` | bytes32 hex | Hash of the lock transaction |

The paid path checks on-chain status `PENDING` (1), `task.worker` equal to this worker, `amount >= 250000`, and `task.taskHash` equal to the hash above. It then computes, signs, and calls `completeTaskWithProof`.

`GET /health` on the worker returns JSON whose `status` field is `healthy`. The orchestrator `GET /health` does the same.

## 2. Smart Contract State Machine

Task status values: `NONE = 0`, `PENDING = 1`, `COMPLETED = 2`, `REFUNDED = 3`.

```
                    +------------------+
                    |       NONE       |
                    +--------+---------+
                             | lockTaskEscrow()
                             | requires active session
                             v
                    +------------------+
                    |     PENDING      |
                    +----+--------+----+
                         |        |
     completeTaskWithProof()     refundExpiredTask()
     (worker signature)          (block.number > deadline)
                         |        |
                         v        v
                  +----------+  +----------+
                  | COMPLETED|  | REFUNDED |
                  +----------+  +----------+

emergencyRevokeSession(agentKey) is not a task transition.
The principal sets that session isActive = false.
The next lockTaskEscrow from that agent reverts SessionInactive.
A task already PENDING stays PENDING until claim or refund.
```

`registerSessionWithSig` checks `expiresAt > block.timestamp`, recovers the EIP-712 signer, and requires it to equal `principal`. Typehash:

`SessionAuthorization(address principal,address agentKey,uint256 hourlyLimit,uint256 perTaskLimit,uint256 expiresAt,uint256 nonce)`

The nonce stored in the signature is `nonces[principal]`, then the counter increments. A new session sets `currentWindow = block.timestamp / 3600`, `spentInWindow = 0`, `isActive = true`.

`lockTaskEscrow(principal, worker, amount, durationBlocks, taskHash)`:

- Caller is the agent.
- Session active, not expired, `amount > 0`, `worker != 0`, `amount <= perTaskLimit`.
- If `block.timestamp / 3600 > currentWindow`, the window rolls and `spentInWindow` becomes 0.
- `spentInWindow + amount <= hourlyLimit`, or `ExceedsHourlyLimit`.
- `safeTransferFrom(principal, escrow, amount)`.
- `taskId = keccak256(abi.encodePacked(principal, msg.sender, worker, taskNonce++, taskHash))`.
- `deadline = block.number + durationBlocks`.

## 3. Cryptographic Proof & Settlement Mechanics

No oracle. The worker hashes the output string with keccak256, then signs the 32-byte `keccak256(abi.encodePacked(taskId, resultHash))` with Alloy `sign_message`, which applies the Ethereum personal-sign prefix. The recovery id is normalized to 27 or 28.

The contract, OpenZeppelin Contracts 5.0.2:

```solidity
bytes32 digest = MessageHashUtils.toEthSignedMessageHash(
    keccak256(abi.encodePacked(taskId, resultHash))
);
(address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, workerSignature);
if (err != ECDSA.RecoverError.NoError || signer != task.worker) revert InvalidProofSignature();
```

`ECDSA.toEthSignedMessageHash` is not on `ECDSA` in this OpenZeppelin version. A 65-byte signature must already use v = 27 or 28. OZ 5.0.2 does not add 27 when v is 0 or 1.

On success the contract writes `resultHash`, sets `COMPLETED`, and `safeTransfer`s the escrowed tUSD8 to `task.worker` in the same transaction.

## 4. Process boundaries

| Process | Listens | Holds |
|---|---|---|
| `agent-worker` | `:8080` | Worker private key. Signs claims. |
| `agent-orchestrator` | `:8081` | Orchestrator session key. Signs locks. In-memory job list. |
| `frontend` | `:3000` | No session private key. Reads telemetry and sends the EIP-712 signature from the connected wallet. |

Inside Compose, the orchestrator calls `http://agent-worker:8080`. The browser calls `http://localhost:8081`. Jobs are not written to a database. A restart drops the queue. The chain remains the record of locks and claims.

## 5. Live status stream

`GET /api/events` is a Server-Sent Events response. Each `data:` line is one task JSON object. The orchestrator publishes `402_CHALLENGED`, `ESCROW_LOCKED`, `SETTLED`, and `FAILED`. The command center opens that stream, reconnects after a drop, and increments the mesh burst when the status is `ESCROW_LOCKED` or `SETTLED`.

Lock transactions share one agent key. `PendingNonceAt` and `SendTransaction` run under `sendMu` so the four worker goroutines do not reuse a nonce. Receipt polling retries a non-`NotFound` RPC error three times with exponential backoff.
