# Nexus402: Hackathon Demo Video Script (3:30 Target Duration)

Record at 1920×1080. Command center on http://localhost:3000, wallet on QDay Aevum (chain id 44005), orchestrator on port 8081, worker on port 8080. The principal holds tUSD8 and a little tQDAY. The orchestrator agent and the worker hold tQDAY for gas. Start with no session registered.

The explorer click at 2:30 needs a real lock or claim hash from that take. Film it only after `NEXUS_ESCROW_ADDRESS` is the address `DeployAevum` logged.

### 0:00 - 0:40 | The Hook & Problem Statement

- **Visual:** Split screen showing an AI agent trapped by a MetaMask "Confirm Transaction" popup on one side, and the Nexus402 title card on the other.
- **Voiceover:** "AI agents are transitioning from conversational chatbots into autonomous economic actors. But today, Web3 payments are broken for machines. Current dApps require a human to manually click 'Confirm' on MetaMask, while giving an LLM full private-key access leads to catastrophic drain if it hits an infinite loop or prompt injection. Machine commerce needs two things: native HTTP integration and strict cryptographic guardrails. Introducing Nexus402."

### 0:40 - 1:20 | System Architecture & Platform Alignment

- **Visual:** Architecture slide showing the Go Orchestrator, Rust Worker, and NexusEscrow.sol on QDay Aevum Testnet.
- **Voiceover:** "Nexus402 is built on QDay Aevum Testnet, tapping into its high-throughput, sub-second settlement rails and native tUSD8 stablecoin. We built a bilateral machine-to-machine service mesh using systems-grade architecture:
1. A high-performance Worker Agent written in Rust using Axum and Alloy.
2. A concurrent Orchestrator Agent written in Go using goroutines and go-ethereum.
3. An on-chain conditional escrow contract deployed on Aevum Testnet.
Instead of custom web sockets or off-chain trust, we implement the web-native HTTP 402 Payment Required protocol."

### 1:20 - 2:30 | Live Walkthrough & On-Chain Settlement

- **Visual:** Switch to live desktop showing the Next.js 14 Command Center with the Three.js 3D Mesh Topology on top and Dual Terminal below.
- **Voiceover:**
  - "Here is our 3D Command Center. First, the human operator connects their wallet and delegates an ephemeral session key to our Go Orchestrator using an EIP-712 typed signature. We set a hard per-task cap of 0.50 tUSD8 and a rolling hourly velocity ceiling of 5 tUSD8."
  - *(Action: Click 'Sign & Authorize' in MetaMask; badge turns green.)*
  - "Now, the Go Orchestrator autonomously dispatches a compute job to our Rust worker. Watch the live terminals."
  - *(Action: Trigger task. Terminal streams logs.)*
  - "Step 1: The Rust Worker intercepts the unauthenticated request and immediately returns HTTP 402 Payment Required with cryptographic task commitments."
  - "Step 2: The Go Orchestrator parses the challenge, validates that 0.25 tUSD8 is within its session limits, and calls lockTaskEscrow on Aevum Testnet without any human approval."
  - *(Action: Highlight glowing particle burst traversing the 3D nodes to the contract.)*
  - "Step 3: The Rust Worker detects the on-chain lock via Alloy, runs the deterministic compute task, hashes the output, and signs the proof. It calls completeTaskWithProof on-chain to atomically release the funds."
  - *(Action: Click the transaction hash in the right terminal to open Aevum Block Explorer.)*
  - "Here is the verified transaction on the Aevum Explorer. Zero human intervention, sub-second execution, fully verifiable on-chain."

### 2:30 - 3:05 | Defense-in-Depth & The Emergency Circuit Breaker

- **Visual:** Focus on the Emergency Revoke button in the UI.
- **Voiceover:**
  - "What happens if an agent goes rogue or is exploited? Nexus402 implements defense-in-depth:
  First, rolling 1-hour windows reset spend caps deterministically, blocking runaway recursive loops.
  Second, if a worker fails to deliver, the principal can claim a 100% refund after the deadline block.
  And third, the human principal has an on-chain panic button."
  - *(Action: Click 'EMERGENCY REVOKE AGENT KEY'.)*
  - "We trigger emergencyRevokeSession on-chain. When the orchestrator attempts another micro-task, the contract instantly reverts with SessionInactive. The agent is neutralized on-chain."

On screen the status reads `SESSION_REVOKED` and Dispatch task is disabled. Say the revert when the next lock fails.

### 3:05 - 3:30 | Conclusion & QDay Mainnet Roadmap

- **Visual:** GitHub repository overview, passing Foundry test suites, and final summary slide.
- **Voiceover:** "Nexus402 is 100% open-source, fully tested with Foundry, Rust, and Go, and ready to deploy on QDay's upcoming mainnet in February. We are providing the fundamental payment primitive for the autonomous machine economy. Thank you."

Cut the Foundry frame from `forge test -vvv` in `contracts/` (8 tests). End on https://github.com/mahitss/Aavum_web3_chain.
