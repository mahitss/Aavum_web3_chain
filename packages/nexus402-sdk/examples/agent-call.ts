import { createNexusClient } from '@nexus402/sdk';

const rpcUrl = process.env.AEVUM_RPC_URL ?? 'https://rpc-test.qday.info';
const escrowAddress = process.env.NEXUS_ESCROW_ADDRESS as `0x${string}`;
const principalAddress = process.env.PRINCIPAL_ADDRESS as `0x${string}`;
const agentPrivateKey = process.env.ORCHESTRATOR_AGENT_PRIVATE_KEY as `0x${string}`;

const nexus = createNexusClient({ rpcUrl, escrowAddress, principalAddress, agentPrivateKey, maxPerTaskAmount: 1_000_000n });
const worker = process.env.WORKER_URL ?? 'http://127.0.0.1:8080';

const { response, payment } = await nexus.fetch402(`${worker}/api/v1/compute`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ prompt: 'Summarize https://example.com', service: 'web_brief' }),
});

console.log(response.status, payment?.taskId, payment?.explorerUrl);
