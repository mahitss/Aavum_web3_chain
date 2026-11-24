# Data design

There is no database. Adding one would not make settlement safer.

| Fact | Source of truth | What a restart does |
|---|---|---|
| Session caps, spent window, active flag | `NexusEscrow` storage | Unchanged |
| Task status, amount, deadline, hashes | `NexusEscrow` storage | Unchanged |
| tUSD8 balances | tUSD8 contract | Unchanged |
| Job id, status string, error text | Orchestrator memory | Lost |
| UI notice and local ledger rows | Browser memory | Lost on refresh of the local rows that were not in the job list |

`GET /api/jobs` returns a JSON array. It is not durable. Operators who need a receipt use the explorer transaction, not the job row.

This choice is [ADR-001](../23-decisions/ADR-001-memory-queue.md).
