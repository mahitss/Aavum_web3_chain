# API specification

No OpenAPI file is generated. These are the routes the code serves.

## Worker `http://localhost:8080`

| Method | Path | Auth | Success | Errors |
|---|---|---|---|---|
| GET | `/health` | None | 200, `status: healthy`, worker address, network name | Process is down if the key or escrow address is missing or not hex |
| POST | `/api/v1/compute` | None | 200 after a valid escrow header | 402 without `X-Escrow-TaskId`. 400 if the task id is not 32 bytes. Claim failures return the worker error JSON |

Request body: `{"prompt": string, "task_type": string}`.

402 headers are listed in [ARCHITECTURE.md](../ARCHITECTURE.md).

## Orchestrator `http://localhost:8081`

| Method | Path | Auth | Success | Errors |
|---|---|---|---|---|
| GET | `/health` | None | 200, `status: healthy` | Process exits on startup if the session key, escrow, or principal is empty |
| POST | `/api/jobs` | None on the local demo | 202 and the job object, status `PENDING` | 4xx if the body is not the expected JSON |
| GET | `/api/jobs` | None | JSON array. Optional `?limit=` | |
| GET | `/api/telemetry` | None | Agent address, principal, escrow, counts, hourly spent and limits, chain id 44005 | |

The local API is not authenticated. Do not expose port 8081 on a public network. Anyone who can reach it can enqueue jobs that spend the session up to the on-chain caps.

CORS allows any origin and does not allow credentials.
