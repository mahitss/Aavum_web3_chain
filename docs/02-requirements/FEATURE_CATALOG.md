# Feature catalog

| Feature | State | Notes |
|---|---|---|
| EIP-712 session registration | Implemented | Wallet signs. Contract checks the signer. |
| tUSD8 approval | Implemented | Button disabled when the escrow address is unset. |
| HTTP 402 challenge | Implemented | Six headers. Previously observed against a local worker. |
| Escrow lock | Implemented | Needs a funded session on Aevum to mine. **UNVERIFIED** live. |
| Proof claim | Implemented | Foundry test passes. Live claim **UNVERIFIED**. |
| Hourly and per-task caps | Implemented | Foundry tests pass. Live bench **UNVERIFIED**. |
| Emergency revoke | Implemented | UI shows `SESSION_REVOKED`. |
| Refund after deadline | Implemented | Command center calls `refundExpiredTask` for the newest unsettled task id. The chain still enforces the deadline. |
| 3D mesh and dual terminal | Implemented | Copy buttons write to the clipboard. |
| Docker Compose healthchecks | Implemented | Empty key fails the worker healthcheck. |
| `make deploy-aevum` | Implemented | Exits if no key. Not executed. |
| `verify-e2e.sh` | Implemented | Not executed to `SETTLED`. |
| `mesh-bench.sh` | Implemented | Not executed. |
| Vercel site | Not started | No project is linked. |
| Demo video | Not started | Script is [DEMO_SCRIPT.md](../DEMO_SCRIPT.md). |
