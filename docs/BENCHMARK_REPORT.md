# Nexus402 benchmark

No live Aevum measurement has been captured in this workspace.

The deployer key is unset, so NexusEscrow has not been broadcast and there is no confirmed `SessionAuthorized`, `TaskEscrowLocked`, `TaskCompleted`, `TaskRefunded`, or `SessionRevoked` transaction to link.

| Flow | HTTP 402 latency | Confirmation | Gas | Explorer |
|---|---|---|---|---|
| SessionAuthorized | Not recorded | Not recorded | Not recorded | Not recorded |
| TaskEscrowLocked and TaskCompleted | Not recorded | Not recorded | Not recorded | Not recorded |
| TaskRefunded | Not recorded | Not recorded | Not recorded | Not recorded |
| SessionRevoked | Not recorded | Not recorded | Not recorded | Not recorded |

`scripts/benchmark-report.sh` overwrites this file from a running orchestrator. It leaves these rows unchanged until those responses include real transaction hashes.
