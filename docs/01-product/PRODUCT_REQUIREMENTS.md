# Product requirements

## Vision

A human sets a spend cap once. After that, one agent can pay another agent for a single compute request in tUSD8 on QDay Aevum Testnet, and the human can stop new spend without handing the agent an unrestricted key.

## Users

| Actor | Job |
|---|---|
| Principal | Holds tUSD8 and tQDAY. Signs the session. Can revoke. |
| Orchestrator | Holds the session key. Pays gas for locks. Must not hold the principal's tUSD8. |
| Worker | Sells one hash task for 0.25 tUSD8. Signs the claim. Pays its own gas. |
| Judge or operator | Reads the command center and the explorer. |

## Success for this repository

1. Unpaid `POST /api/v1/compute` returns 402 with the six payment headers.
2. A registered session can lock `250000` tUSD8 units and the worker can claim with a proof the contract accepts.
3. A lock above the hourly cap reverts `ExceedsHourlyLimit`.
4. `emergencyRevokeSession` makes the next lock revert `SessionInactive`.
5. After the deadline block, `refundExpiredTask` returns the full amount to the principal.

Items 2 through 4 are implemented and covered by Foundry. A broadcast of those same calls on Aevum is **UNVERIFIED**.

## Not this product

A model marketplace, a mobile app, card payments, SSO, a data warehouse, or a multi-tenant SaaS.
