# Risk register

| ID | Risk | Status | Mitigation |
|---|---|---|---|
| R1 | No deployed escrow, so judges cannot open a contract page | Open | `make deploy-aevum` when a funded testnet key is supplied |
| R2 | Empty `.env` crash-loops the worker | Open, observed 24 Nov 2026 | Runbook says fill keys. Compose was stopped |
| R3 | Port 8081 has no authentication | Accepted for localhost | Do not publish the port |
| R4 | Job list dies with the process | Accepted | Chain is the receipt |
| R5 | Contract pays a signed hash even if the output is useless | Accepted | Documented in the audit |
| R6 | Pending escrow survives revoke until claim or refund | Accepted | Deadline plus refund |
| R7 | `npm ci` in Docker disagreed with the lockfile | Mitigated | Dockerfile uses `npm install` |
| R8 | Google Fonts blocked the image build | Mitigated | System font stacks |
| R9 | Submission video and Vercel URL missing | Open | Tokens stay in `docs/SUBMISSION.md` |
| R10 | CI workflow not yet observed on GitHub | Open | `.github/workflows/ci.yml` |

Do not close R1, R2, or R9 by typing an address or a URL.
