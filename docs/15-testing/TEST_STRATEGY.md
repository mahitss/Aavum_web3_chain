# Test strategy

| Requirement | Test | Last result in this workspace |
|---|---|---|
| Valid session signature registers | `test_RegisterSessionWithValidSig` | Pass, 24 Nov 2026 |
| Wrong signer reverts | `test_RevertWhen_InvalidSignature` | Pass |
| Per-task cap | `test_RevertWhen_ExceedingPerTaskLimit` | Pass |
| Hourly window | `test_RollingHourlyVelocityCap` | Pass |
| Lock and claim | `test_EndToEnd_LockAndCompleteProof` | Pass |
| Wrong worker cannot claim | `test_RevertWhen_UnauthorizedWorkerClaims` | Pass |
| Refund after deadline | `test_RefundExpiredTask` | Pass |
| Revoke blocks the next lock | `test_EmergencyRevokeCircuitBreaker` | Pass |
| Task hash is 32 bytes and stable | `cargo test` in `agent-worker` | Not re-run on 24 Nov 2026 |
| 402 header parse | `go test ./...` in `agent-orchestrator` | Pass, `pkg/x402` ok, 24 Nov 2026 |
| Unpaid HTTP 402 on a running worker | `scripts/verify-e2e.sh` step 2 | **UNVERIFIED** this session. Stack was not healthy. |
| Mined lock and claim | `scripts/verify-e2e.sh` steps 3 and 4 | **UNVERIFIED**. No funded key. |
| Ten parallel jobs, hourly cap, revoke | `scripts/mesh-bench.sh` | **UNVERIFIED** |
| Command center production build | `npm run build` in `frontend` | Exit 0, 24 Nov 2026. Route `/` static. Wallet click path **UNVERIFIED**. |

`forge test` is the release gate for the contract. A green Foundry run does not prove an Aevum transaction.

CI is `.github/workflows/ci.yml`. It runs Foundry, `go test`, `cargo test`, and `npm run build`. GitHub has not reported a result for that file yet, so the workflow itself is **UNVERIFIED**.
