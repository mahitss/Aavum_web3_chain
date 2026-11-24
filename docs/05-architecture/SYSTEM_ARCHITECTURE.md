# System architecture

Four processes, one chain. No extra service is justified.

```
Browser --EIP-712--> NexusEscrow
Browser --HTTP-----> Go orchestrator :8081
Orchestrator --HTTP 402--> Rust worker :8080
Orchestrator --lockTaskEscrow--> NexusEscrow
Worker --completeTaskWithProof--> NexusEscrow
NexusEscrow --tUSD8--> worker or back to principal
```

| Boundary | Trust |
|---|---|
| Browser | Sees public addresses and job JSON. Does not hold the session key or the worker key. |
| Orchestrator | Holds the session key. Can lock up to the on-chain caps. |
| Worker | Holds the worker key. Can claim only a task that names it. |
| Contract | Holds tUSD8. Enforces caps, deadline, signature, and revoke. |

Compose network `nexus402-network` is a bridge. The worker must be healthy before the orchestrator starts. The orchestrator must be healthy before the frontend starts.

Details of the handshake and the state machine: [ARCHITECTURE.md](../ARCHITECTURE.md).

Decision to stay at four processes: [ADR-002](../23-decisions/ADR-002-four-processes.md).
