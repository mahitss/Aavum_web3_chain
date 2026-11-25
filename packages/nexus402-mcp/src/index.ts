import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { createNexusClient, type Fetch402Result } from '@nexus402/sdk';
import { createServer } from 'node:http';
import { z } from 'zod';
import { executeServiceCall } from './call.js';
import { loadCatalog, toolName, type ServiceOffer } from './catalog.js';

const inputShape = { prompt: z.string().describe('Service input: a public URL, prompt, or 0x address') };

export function createNexusMcpServer(options: {
  services: ServiceOffer[];
  workerUrl: string;
  mode: 'inspect' | 'autopay';
  pay?: (url: string, init: RequestInit) => Promise<Fetch402Result>;
}) {
  const server = new McpServer({ name: 'nexus402', version: '1.0.0' });
  for (const service of options.services) {
    server.registerTool(
      toolName(service.name),
      {
        description: `${service.name} on QDay Aevum Testnet for ${service.price} tUSD8. Input: ${service.input}.`,
        inputSchema: inputShape,
      },
      async ({ prompt }) =>
        executeServiceCall({
          workerUrl: options.workerUrl,
          service,
          prompt,
          mode: options.mode,
          pay: options.pay,
        }),
    );
  }
  return server;
}

async function main() {
  const workerUrl = process.env.NEXUS_WORKER_URL ?? 'http://127.0.0.1:8080';
  const services = await loadCatalog(workerUrl);
  const key = process.env.ORCHESTRATOR_AGENT_PRIVATE_KEY ?? '';
  const principal = process.env.PRINCIPAL_ADDRESS ?? '';
  const escrow = process.env.NEXUS_ESCROW_ADDRESS ?? '';
  const mode = key && principal && escrow ? 'autopay' : 'inspect';
  const client =
    mode === 'autopay'
      ? createNexusClient({
          rpcUrl: process.env.AEVUM_RPC_URL ?? 'https://rpc-test.qday.info',
          escrowAddress: escrow as `0x${string}`,
          principalAddress: principal as `0x${string}`,
          agentPrivateKey: key as `0x${string}`,
          maxPerTaskAmount: tusdToUnits(process.env.MAX_PER_TASK_TUSD8 ?? '1'),
        })
      : undefined;
  const pay = client ? (url: string, init?: RequestInit) => client.fetch402(url, init) : undefined;
  const http = process.argv.includes('--http');
  if (!http) {
    const server = createNexusMcpServer({ services, workerUrl, mode, pay });
    await server.connect(new StdioServerTransport());
    return;
  }
  const port = Number(process.env.MCP_HTTP_PORT ?? '8787');
  const listener = createServer(async (req, res) => {
    if (req.url !== '/mcp') {
      res.statusCode = 404;
      res.end('not found');
      return;
    }
    const chunks: Uint8Array[] = [];
    for await (const chunk of req) chunks.push(typeof chunk === 'string' ? new TextEncoder().encode(chunk) : chunk);
    const raw = new TextDecoder().decode(concatBytes(chunks));
    const body = raw ? JSON.parse(raw) : undefined;
    const server = createNexusMcpServer({ services, workerUrl, mode, pay });
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });
    await server.connect(transport);
    await transport.handleRequest(req, res, body);
  });
  listener.listen(port, '127.0.0.1');
}

function concatBytes(chunks: Uint8Array[]) {
  const length = chunks.reduce((total, chunk) => total + chunk.length, 0);
  const out = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  return out;
}

function tusdToUnits(value: string) {
  const [whole, frac = ''] = value.trim().split('.');
  const padded = `${frac}000000`.slice(0, 6);
  return BigInt(whole || '0') * 1_000_000n + BigInt(padded || '0');
}

const entry = process.argv[1]?.replace(/\\/g, '/');
if (entry?.endsWith('/src/index.ts') || entry?.endsWith('/dist/index.js')) {
  main().catch((error) => {
    console.error(error);
    process.exit(1);
  });
}
