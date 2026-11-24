# Disaster recovery

| Failure | What still holds | What to do |
|---|---|---|
| Orchestrator process dies | Escrows already mined | Restart. In-memory jobs are gone. Read the explorer for the lock hash if you saved it. |
| Worker dies after a lock | tUSD8 sits in escrow until the deadline | Restart the worker so it can claim, or call `refundExpiredTask` after the deadline. The UI button does that for the newest unsettled task id still in memory. |
| Wrong contract address shipped | Old escrow is unaffected | Point `.env` at the intended address. There is no upgrade. |
| Session key leaked | Caps and revoke | Principal sends `emergencyRevokeSession`. Move remaining tQDAY off that key. |
| RPC outage | No new locks or claims | Wait. Do not retry a claim that might already be mined without checking the receipt. The orchestrator polls receipts. A blind second lock for the same user intent can create a second escrow. |

RPO for job rows is zero: they are memory. RPO for tUSD8 is the last mined block. RTO is the time to `docker compose up` plus RPC recovery. No numeric SLO is promised.

There is no backup job, because there is no database.
