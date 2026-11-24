# Implementation roadmap

## Done in this repository

- Escrow contract and 8 Foundry tests.
- Rust 402 worker and Go orchestrator.
- Command center, including revoke and refund submission.
- Compose files, Makefile, e2e script, mesh bench, submission docs.

## Blocked on a funded testnet key

1. Broadcast `NexusEscrow` and write the address into `.env`.
2. Approve tUSD8 and register the session.
3. Run `./scripts/verify-e2e.sh` and keep the two explorer URLs.
4. Replace the insert tokens in `docs/SUBMISSION.md` and the contract row in `README.md`.
5. Record `docs/DEMO_SCRIPT.md` only after those URLs open.

## Not scheduled

Mobile, a model gateway, Kubernetes, a database, card billing, SSO, and a compliance program. They are out of scope in [the index](../README.md).
