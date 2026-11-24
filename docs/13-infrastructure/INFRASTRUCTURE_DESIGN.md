# Infrastructure

The supported runtime is one machine with Docker Compose.

| Piece | Image or tool | Port |
|---|---|---|
| Worker | Multi-stage `rust:1.90-slim-bookworm`, runtime `debian:bookworm-slim` | 8080 |
| Orchestrator | `golang:1.22-alpine`, runtime `alpine:3.19` | 8081 |
| Frontend | `node:20-alpine`, Next standalone `node server.js` | 3000 |
| Chain | QDay Aevum public RPC | 44005 |

Network: bridge `nexus402-network`. Healthchecks call `/health` every 5 seconds. The orchestrator waits until the worker is healthy. The frontend waits until the orchestrator is healthy.

No Kubernetes cluster, no paid cloud account, and no load balancer are part of this design. A public deployment would need a secret store and a firewall in front of ports 8080 and 8081. That work is not started.

Images built in this workspace on 24 Nov 2026: `aevumbuilder-agent-worker`, `aevumbuilder-agent-orchestrator`, `aevumbuilder-frontend`.
