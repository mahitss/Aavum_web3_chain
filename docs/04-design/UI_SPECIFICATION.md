# UI specification

Route: `/` only.

| Region | Behavior |
|---|---|
| Header | Title and RainbowKit connect. The chain label is QDay Aevum, chain id 44005. |
| Metrics | Principal tUSD8 balance, hourly spent/limit from telemetry, session TTL from `getSession`, count of jobs with status `SETTLED`. |
| Mesh | WebGL canvas. Particles move when `settledCount > 0`. Hidden from server render. |
| Session | Agent address field, hourly and per-task sliders, expiry 1/6/24 hours, approve, authorize. |
| Kill switch | `emergencyRevokeSession`. Disabled without an agent address or escrow address. |
| Dispatch | Prompt and task type. Disabled after a local kill. Posts `{prompt, task_type}` to `/api/jobs`. |
| Refund | Enabled when some job has a hex `task_id` and is not `SETTLED`. |
| Terminals | Handshake lines and a ledger of lock, claim, session, revoke, and refund hashes. Copy buttons call the clipboard API. |

Empty orchestrator: metrics show `— / —` and the mesh caption says `Orchestrator idle`.

Error: a failed fetch sets the notice to the error message. A reverted refund shows the wallet error under the form.

Wallet chain switch was not exercised in this workspace. That path is **UNVERIFIED**.
