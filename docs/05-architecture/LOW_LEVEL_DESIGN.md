# Low-level design

## Contracts

`NexusEscrow` is not upgradeable. Constructor argument is the tUSD8 address. Inheritance is `INexusEscrow`, `ReentrancyGuard`, and `EIP712("Nexus402 Protocol", "1")`.

Task id:

`keccak256(abi.encodePacked(principal, msg.sender, worker, taskNonce, taskHash))`

The nonce is per `(principal, agent)` and increments inside the lock.

## Worker

Axum on `0.0.0.0:$WORKER_PORT`. Unpaid requests return 402 before any chain write. Paid requests read `getTask`. The claim builder is bound to a local before `send`. The signer uses chain id 44005.

## Orchestrator

Gin in release mode. `POST /api/jobs` returns 202 and the job. Four goroutines read a buffered channel of 100. Status strings are `PENDING`, `402_CHALLENGED`, `ESCROW_LOCKED`, `SETTLED`, `FAILED`. A failed broadcast releases the local hourly reservation so `hourly_spent` does not stick.

Lock gas limit is 300000. Signer is `types.NewCancunSigner`. Receipts are polled.

## Frontend

Next.js 14 App Router, `output: 'standalone'`. Escrow address is ignored when it is missing or the zero address. API base is `NEXT_PUBLIC_ORCHESTRATOR_API_URL`, then `NEXT_PUBLIC_API_URL`, then `http://localhost:8081`.
