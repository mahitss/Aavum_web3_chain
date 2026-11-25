import { afterEach, describe, expect, it, vi } from 'vitest';
import { assertChallenge, createNexusClient, decodePaymentRequired, TUSD8_ADDRESS, type PaymentChallenge } from '../src/client';

const taskHash = `0x${'11'.repeat(32)}` as const;
const payTo = '0x3000000000000000000000000000000000000003' as const;

function challenge(amount = '150000'): PaymentChallenge {
  return decodePaymentRequired(
    Buffer.from(
      JSON.stringify({
        x402Version: 2,
        accepts: [
          {
            scheme: 'nexus-escrow-v1',
            network: 'eip155:44005',
            asset: TUSD8_ADDRESS,
            amount,
            payTo,
            taskHash,
            timeoutBlocks: 5,
          },
        ],
      }),
    ).toString('base64'),
  );
}

describe('fetch402', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('rejects a price above maxPerTaskAmount before locking', async () => {
    const lockEscrow = vi.fn();
    const client = createNexusClient({
      rpcUrl: 'https://rpc-test.qday.info',
      escrowAddress: '0x1000000000000000000000000000000000000001',
      principalAddress: '0x2000000000000000000000000000000000000002',
      agentPrivateKey: `0x${'ab'.repeat(32)}`,
      maxPerTaskAmount: 150_000n,
      lockEscrow,
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('{}', { status: 402, headers: { 'PAYMENT-REQUIRED': headerFor('250000') } })),
    );
    await expect(client.fetch402('https://worker.example/api/v1/compute', { method: 'POST' })).rejects.toThrow(
      /maxPerTaskAmount/,
    );
    expect(lockEscrow).not.toHaveBeenCalled();
  });

  it('locks after a 402 and retries with PAYMENT-SIGNATURE', async () => {
    const lockEscrow = vi.fn(async () => ({
      taskId: `0x${'22'.repeat(32)}` as const,
      lockTxHash: `0x${'33'.repeat(32)}` as const,
    }));
    const calls: Array<{ url: string; signature: string | null }> = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const headers = new Headers(init?.headers);
        calls.push({ url, signature: headers.get('PAYMENT-SIGNATURE') });
        if (calls.length === 1) {
          return new Response('{}', { status: 402, headers: { 'PAYMENT-REQUIRED': headerFor('150000') } });
        }
        return new Response('{"status":"settled"}', {
          status: 200,
          headers: {
            'PAYMENT-RESPONSE': Buffer.from(
              JSON.stringify({
                x402Version: 2,
                claimTxHash: `0x${'44'.repeat(32)}`,
                explorerUrl: 'https://explorer-test.qday.info/tx/0x' + '44'.repeat(32),
              }),
            ).toString('base64'),
          },
        });
      }),
    );
    const client = createNexusClient({
      rpcUrl: 'https://rpc-test.qday.info',
      escrowAddress: '0x1000000000000000000000000000000000000001',
      principalAddress: '0x2000000000000000000000000000000000000002',
      agentPrivateKey: `0x${'ab'.repeat(32)}`,
      maxPerTaskAmount: 1_000_000n,
      lockEscrow,
    });
    const paid = await client.fetch402('https://worker.example/api/v1/compute', {
      method: 'POST',
      body: JSON.stringify({ prompt: 'hello', service: 'web_brief' }),
    });
    expect(paid.response.status).toBe(200);
    expect(lockEscrow).toHaveBeenCalledOnce();
    expect(calls[1]?.signature).toBeTruthy();
    expect(paid.payment?.taskId).toBe(`0x${'22'.repeat(32)}`);
    expect(paid.payment?.claimTxHash).toBe(`0x${'44'.repeat(32)}`);
    const signature = JSON.parse(Buffer.from(calls[1]?.signature ?? '', 'base64').toString('utf8'));
    expect(signature.x402Version).toBe(2);
    expect(signature.taskId).toBe(paid.payment?.taskId);
  });

  it('returns a non-402 response without locking', async () => {
    const lockEscrow = vi.fn();
    vi.stubGlobal('fetch', vi.fn(async () => new Response('ok', { status: 200 })));
    const client = createNexusClient({
      rpcUrl: 'https://rpc-test.qday.info',
      escrowAddress: '0x1000000000000000000000000000000000000001',
      principalAddress: '0x2000000000000000000000000000000000000002',
      agentPrivateKey: `0x${'ab'.repeat(32)}`,
      maxPerTaskAmount: 1_000_000n,
      lockEscrow,
    });
    const result = await client.fetch402('https://worker.example/health');
    expect(result.response.status).toBe(200);
    expect(result.payment).toBeNull();
    expect(lockEscrow).not.toHaveBeenCalled();
  });
});

describe('assertChallenge', () => {
  it('accepts the priced tUSD8 challenge', () => {
    expect(() => assertChallenge(challenge(), 150_000n)).not.toThrow();
  });
});

function headerFor(amount: string) {
  return Buffer.from(
    JSON.stringify({
      x402Version: 2,
      accepts: [
        {
          scheme: 'nexus-escrow-v1',
          network: 'eip155:44005',
          asset: TUSD8_ADDRESS,
          amount,
          payTo,
          taskHash,
          timeoutBlocks: 5,
        },
      ],
    }),
  ).toString('base64');
}
