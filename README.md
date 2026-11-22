# Nexus402

Agent-to-agent marketplace escrow for QDay Aevum Testnet (chain id `44005`). Settlement is tUSD8 (`0x456569838f1d43009A91ebA4263147Afd7233483`, 6 decimals).

A principal signs an EIP-712 session that lets an ephemeral orchestrator key lock tUSD8, up to a per-task cap and a rolling one-hour cap, until expiry. The worker serves `POST /api/v1/compute` behind HTTP 402. After the orchestrator locks escrow, the worker checks the task on Aevum, runs a deterministic keccak job, and claims with `completeTaskWithProof`. If the deadline block passes while the task is still pending, anyone can refund the principal. The principal can freeze the agent key with `emergencyRevokeSession`.

## Layout

- `contracts/` Foundry escrow (`Solidity 0.8.24`, Cancun, OpenZeppelin v5)
- `agent-worker/` Rust Axum worker
- `agent-orchestrator/` Go session-key client
- `frontend/` Next.js command center

## Contracts

```powershell
git submodule update --init --recursive
forge test
```

`contracts/lib/forge-std` is a submodule. OpenZeppelin Contracts v5.1.0 is vendored under `contracts/lib/openzeppelin-contracts`.

Deploy to Aevum (the script refuses any other chain id):

```powershell
$env:DEPLOYER_PRIVATE_KEY = "0x..."
forge script script/DeployAevum.s.sol --rpc-url aevum --broadcast
```

Copy the logged escrow address into each `.env`. The principal must `approve` the escrow for tUSD8 before the session key can lock funds. The session key and the worker key need tQDAY for gas.

Worker proof digest is `keccak256(abi.encodePacked(taskId, resultHash))`, recovered directly (no personal-sign prefix). Session authorization is EIP-712 typed data:

`SessionAuthorization(address principal,address agentKey,uint256 hourlyLimit,uint256 perTaskLimit,uint256 expiresAt,uint256 nonce)`

Domain name `Nexus402 Protocol`, version `1`, chain id `44005`, verifying contract = escrow.

Task deadlines are block numbers. `refundExpiredTask` succeeds only when `block.number > deadline`.

## Agents

```powershell
cd agent-worker
copy .env.example .env
cargo run

cd ..\agent-orchestrator
copy .env.example .env
go run .
```

`PER_TASK_LIMIT` and `HOURLY_LIMIT` must match the on-chain session. The worker advertises a fee of `250000` (0.25 tUSD8), so the per-task cap has to be at least that.

## Command center

```powershell
cd frontend
copy .env.example .env.local
npm install
npm run dev
```

Connect a wallet on Aevum, approve tUSD8, and sign the session for the orchestrator address printed at orchestrator startup. Dispatch posts the JSON body to the orchestrator, which performs the 402 handshake and the lock. The panic control calls `emergencyRevokeSession` for the agent address in the form. Confirm it with the second press.
