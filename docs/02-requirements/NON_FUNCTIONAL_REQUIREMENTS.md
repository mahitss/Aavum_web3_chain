# Non-functional requirements

| Topic | Target in this repo | Evidence |
|---|---|---|
| Chain | QDay Aevum, chain id 44005 | Constants in worker, orchestrator, and frontend |
| Token | tUSD8 `0x456569838f1d43009A91ebA4263147Afd7233483`, 6 decimals | Worker header and deploy script default |
| Price | 250000 units per hash task | `TASK_PRICE_UNITS` |
| Timeout | 50 blocks | `X-Task-Timeout-Blocks` |
| Orchestrator concurrency | 4 goroutines, queue capacity 100 | Go orchestrator |
| Local ports | Worker 8080, orchestrator 8081, UI 3000 | Compose file |
| Secrets | Keys only in `.env`, which is gitignored | `.gitignore` |
| Contract tests | The Foundry suite passes | 8 passed, 0 failed, 24 Nov 2026, this workspace |
| Live settlement latency | No SLO is claimed | **UNVERIFIED**. No mined lock or claim |
| Availability | No uptime SLO | Single local Compose stack |

A restart of the orchestrator drops in-memory jobs. That is accepted for the demo. See [ADR-001](../23-decisions/ADR-001-memory-queue.md).
