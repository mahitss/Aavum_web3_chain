# Security architecture

Authorization for money movement is only in `NexusEscrow`. The frontend disables buttons. That is not the control.

- Session registration: recovered EIP-712 signer must be `principal`.
- Locks: `msg.sender` must be the active agent. Amount checks run before `safeTransferFrom`.
- Claims: recovered personal-sign signer must be `task.worker`.
- Revoke: `msg.sender` must be the principal who registered that agent.
- Refund: anyone may call it, and only after the deadline. Funds go to `task.principal`, not to the caller.

Keys are environment variables. `.env` is gitignored. The repository must not contain a funded key.

Transport to Aevum is HTTPS on the public RPC. Local HTTP between worker, orchestrator, and browser is for the demo host only.

Dependency review is the Foundry suite plus `go test` and `cargo test`. No dependency scanner is configured. No vulnerability program is claimed.

Full matrix: [SECURITY_AUDIT.md](../SECURITY_AUDIT.md).
