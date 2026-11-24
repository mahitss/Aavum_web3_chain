# Threat model

This is the trust-boundary view. The control-by-control matrix is [SECURITY_AUDIT.md](../SECURITY_AUDIT.md). It is not an external audit and it is not a certification.

## Assets

- Principal tUSD8 allowance to the escrow
- tQDAY on the orchestrator key and the worker key
- Session private key and worker private key
- Task result and its hash

## Boundaries

| From | To | Data |
|---|---|---|
| Wallet | Escrow | EIP-712 signature, approve, revoke, refund |
| Browser | Orchestrator | Prompt, task type. No key. |
| Orchestrator | Worker | JSON body, then escrow headers |
| Orchestrator | Aevum RPC | Signed lock transaction |
| Worker | Aevum RPC | `getTask`, signed claim |
| Disk | Processes | `.env` keys |

## What an attacker can do

| Attacker | Reach | Stop |
|---|---|---|
| Stolen session key | Locks until cap, expiry, or revoke | `emergencyRevokeSession`, caps |
| Caller of the open orchestrator port | Same as the session key, by enqueueing jobs | Do not publish port 8081 |
| Stolen worker key | Claims tasks that name that worker. Cannot pull the principal's balance directly | Worker should not be funded with tUSD8 |
| Forged proof | Nothing, if recovery is not the worker | `InvalidProofSignature` |
| Reentering token callback | Second claim sees `COMPLETED` and the guard | `ReentrancyGuard`, effects before transfer |

## Out of model

Prompt secrecy, model safety, and KYC are not part of this system. The contract pays for a matching task hash and a valid worker signature. It does not judge whether the output was useful.
