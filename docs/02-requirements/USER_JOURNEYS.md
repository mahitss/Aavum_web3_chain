# User journeys

## Authorize, then one paid task

1. Principal connects a wallet on chain 44005.
2. Principal approves tUSD8 and signs the session for the orchestrator address.
3. Principal dispatches a hash task from the command center, or an operator runs `./scripts/verify-e2e.sh`.
4. Terminals show 402, then a lock hash, then a claim hash.
5. Both hashes open on https://explorer-test.qday.info.

Step 4 and 5 are **UNVERIFIED** until a funded `.env` exists.

## Stop the agent

1. Principal clicks EMERGENCY REVOKE AGENT KEY.
2. The panel shows `SESSION_REVOKED`.
3. Dispatch is disabled in the UI.
4. A later lock from that agent reverts `SessionInactive` even if something else calls the API.

## Worker disappears

1. Lock is mined. Worker never claims.
2. After the deadline block, the principal (or anyone) calls `refundExpiredTask`.
3. The full amount returns to the principal. The command center button **Refund expired task** sends `refundExpiredTask` for the newest job that has a task id and is not `SETTLED`. Before the deadline block the transaction reverts `DeadlineNotPassed`, and the wallet error is shown under the form.
