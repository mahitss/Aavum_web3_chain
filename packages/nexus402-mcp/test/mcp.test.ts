import { describe, expect, it, vi } from 'vitest';
import { CANONICAL_SERVICES, loadCatalog, toolName } from '../src/catalog';
import { executeServiceCall } from '../src/call';

const taskHash = `0x${'ab'.repeat(32)}`;

function challengeHeader(amount = '150000') {
  return Buffer.from(
    JSON.stringify({
      x402Version: 2,
      accepts: [
        {
          scheme: 'nexus-escrow-v1',
          network: 'eip155:44005',
          asset: '0x456569838f1d43009A91ebA4263147Afd7233483',
          amount,
          payTo: '0x3000000000000000000000000000000000000003',
          taskHash,
          timeoutBlocks: 50,
        },
      ],
    }),
  ).toString('base64');
}

describe('catalog', () => {
  it('registers the priced services from the worker manifest', async () => {
    const fetchImpl = vi.fn(async () =>
      new Response(
        JSON.stringify({
          services: CANONICAL_SERVICES,
        }),
        { status: 200 },
      ),
    );
    const services = await loadCatalog('http://worker.test', fetchImpl);
    expect(services.map((service) => toolName(service.name))).toEqual([
      'nexus_web_brief',
      'nexus_ai_synthesis',
      'nexus_chain_audit',
    ]);
    expect(services.map((service) => service.price)).toEqual(['0.15', '0.25', '0.50']);
  });
});

describe('executeServiceCall', () => {
  it('returns the 402 terms in inspect mode without paying', async () => {
    const pay = vi.fn();
    const result = await executeServiceCall({
      workerUrl: 'http://worker.test',
      service: CANONICAL_SERVICES[0]!,
      prompt: 'https://example.com',
      mode: 'inspect',
      pay,
      fetchImpl: vi.fn(async () => new Response('{}', { status: 402, headers: { 'PAYMENT-REQUIRED': challengeHeader() } })),
    });
    expect(result.content[0]?.text).toContain('0.15 tUSD8');
    expect(result.content[0]?.text).toContain('44005');
    expect(result.content[0]?.text).toContain(taskHash);
    expect(pay).not.toHaveBeenCalled();
  });

  it('returns settlement metadata when auto-pay succeeds', async () => {
    const payment = {
      taskId: `0x${'11'.repeat(32)}`,
      lockTxHash: `0x${'22'.repeat(32)}`,
      claimTxHash: `0x${'33'.repeat(32)}`,
      explorerUrl: 'https://explorer-test.qday.info/tx/' + '33'.repeat(32),
    };
    const result = await executeServiceCall({
      workerUrl: 'http://worker.test',
      service: CANONICAL_SERVICES[1]!,
      prompt: 'Explain session keys',
      mode: 'autopay',
      pay: vi.fn(async () => ({ response: new Response('{"result":"ok"}', { status: 200 }), payment })),
    });
    expect(result._meta?.['x402/payment-response']).toEqual(payment);
    expect(result.content[0]?.text).toContain('ok');
  });
});
