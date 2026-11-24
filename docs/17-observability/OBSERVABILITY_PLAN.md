# Observability

What exists:

- Worker and orchestrator write structured logs to stdout. Docker collects them with `docker compose logs`.
- `GET /health` is the Compose probe.
- `GET /api/telemetry` returns counts, hourly spent, caps, agent, principal, escrow, and chain id.
- Job errors are stored on the job and shown in the terminal.
- The UI does not send logs to a vendor.

What does not exist:

- OpenTelemetry, metrics scrape, alerting, or uptime checks.
- Log redaction beyond the fact that the code does not print the raw private key on the happy path. Treat container logs as sensitive anyway, because RPC errors can echo signed transactions.

No dashboard vendor is approved. Do not add a paid APM account for this demo.
