# Deploy Nexus402 for judging

This runbook deploys the existing escrow, worker, orchestrator, and command center. It does not create wallets or invent transaction hashes. Leave an address blank until the command that created it has printed it.

Network: QDay Aevum Testnet, chain id `44005`, RPC `https://rpc-test.qday.info`, explorer `https://explorer-test.qday.info`. Settlement token tUSD8 is `0x456569838f1d43009A91ebA4263147Afd7233483` (6 decimals). Gas is tQDAY (18 decimals).

## 1. Deploy and verify NexusEscrow

1. Put a funded deployer key in `.env` as `PRIVATE_KEY`. `scripts/deploy-aevum.sh` also accepts `PRINCIPAL_PRIVATE_KEY`.
2. From the repo root, run `make deploy-aevum` or `bash scripts/deploy-aevum.sh`.
3. Copy the logged `NexusEscrow` address into `.env` as `NEXUS_ESCROW_ADDRESS` and `NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS`.
4. Verify on Blockscout:

```bash
forge verify-contract <NEXUS_ESCROW_ADDRESS> src/NexusEscrow.sol:NexusEscrow \
  --chain 44005 \
  --verifier blockscout \
  --verifier-url https://explorer-test.qday.info/api \
  --constructor-args $(cast abi-encode "constructor(address)" 0x456569838f1d43009A91ebA4263147Afd7233483) \
  --watch
```

Run that from `contracts/`. The constructor argument is the tUSD8 address.

## 2. Fund three addresses

| Role | Needs | Why |
|---|---|---|
| Principal (`PRINCIPAL_ADDRESS`) | tUSD8 and a little tQDAY | Pays the escrow and sends the one-time session transaction. |
| Orchestrator agent (`ORCHESTRATOR_AGENT_ADDRESS`) | tQDAY | Gas for `lockTaskEscrow`. |
| Worker (`WORKER_ADDRESS`) | tQDAY | Gas for `completeTaskWithProof`. |

Do not commit the private keys. Render marks `ORCHESTRATOR_AGENT_PRIVATE_KEY`, `WORKER_PRIVATE_KEY`, and `LLM_API_KEY` as unsynced secrets.

## 3. Authorize the sandbox session

The public demo spends from the principal through `ORCHESTRATOR_AGENT_PRIVATE_KEY`. That agent must already be `getSession(...).isActive` on the deployed escrow.

1. Open the command center and connect the principal wallet on chain 44005.
2. Paste the orchestrator agent address.
3. Set the hourly cap high enough for the demo (five `ai_synthesis` tasks are 1.25 tUSD8) and the per-task cap to at least 0.50 tUSD8 if judges will run `chain_audit`.
4. Approve tUSD8 and sign the EIP-712 session.
5. Confirm `GET /api/sandbox/status` returns `"sessionActive": true` before sharing the URL.

`ENABLE_PUBLIC_SANDBOX=true` does not register a session by itself. It only lets anonymous visitors use the session that is already on-chain, and `SANDBOX_MAX_JOBS_PER_IP_HOUR` (default 5) stops one address from draining it. The velocity scenario is one of those requests, then it fires many locks, so keep the on-chain hourly cap small if judges will click **Simulate Runaway Loop Attack**.

## 4. Render and Vercel

`render.yaml` defines two free-tier Docker web services.

1. Push the repo and create a Render Blueprint from `render.yaml`.
2. Fill the unsynced values: `NEXUS_ESCROW_ADDRESS` on both services, `WORKER_PRIVATE_KEY`, `LLM_API_KEY` (optional), `PRINCIPAL_ADDRESS`, `ORCHESTRATOR_AGENT_PRIVATE_KEY`.
3. Set `ALLOWED_ORIGINS` on both services to the Vercel origin, for example `https://your-app.vercel.app`. Add `http://localhost:3000` as well if you still call the hosted API from a local page. Do not set `*`.
4. `WORKER_URL` is linked to the worker service's `RENDER_EXTERNAL_URL`.
5. Deploy `frontend/` on Vercel. `frontend/vercel.json` sets `nosniff`, `DENY` framing, and a strict referrer policy. Set `NEXT_PUBLIC_ORCHESTRATOR_API_URL` and `NEXT_PUBLIC_API_URL` to the orchestrator URL, and `NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS` to the verified contract.

Free instances sleep. The command center calls `GET /health` and, if that takes longer than three seconds, shows **Waking Aevum Agent Mesh...**. `GET /api/events` writes `: ping` every 15 seconds so the stream stays open while a judge is watching.

## 5. Confirm a live settlement

Point the harness at the hosted orchestrator and worker:

```bash
ORCHESTRATOR_URL=https://<orchestrator>.onrender.com \
WORKER_URL=https://<worker>.onrender.com \
./scripts/verify-e2e.sh
```

Record the lock and claim URLs from that run in `README.md` and `docs/SUBMISSION.md`. Until those commands have been executed with a funded session, the deployment is not live and no explorer link should be filled in.
