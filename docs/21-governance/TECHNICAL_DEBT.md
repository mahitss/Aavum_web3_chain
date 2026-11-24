# Technical debt

| Item | Why it remains |
|---|---|
| Orchestrator API has no auth | Demo on localhost. A public bind needs a credential. |
| Jobs are not durable | Chosen in ADR-001. |
| `X-Escrow-TxHash` is sent and not checked by the worker | The worker trusts `getTask`, not the hash header. |
| Local hourly counter can disagree with the chain if two orchestrators share a session | The contract is the backstop. |
| `indexedDB is not defined` during Next SSR | Wagmi and WalletConnect. Build still exits 0. Pages still render. |
| `npm` engine warnings for Metro packages inside the wallet stack | Transitive. Node 20 is the image. |
| Refund button uses the newest in-memory task, not a picker | Enough for one active demo task. |
| No dependency CVE scanner | Not configured. |

None of these are fixed by adding a database or a second framework.
