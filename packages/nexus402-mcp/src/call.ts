import type { Fetch402Result } from '@nexus402/sdk';
import type { ServiceOffer } from './catalog.js';

export type ToolCallResult = {
  content: Array<{ type: 'text'; text: string }>;
  isError?: boolean;
  _meta?: Record<string, unknown>;
};

type Challenge = {
  amount: string;
  taskHash: string;
  network: string;
  asset: string;
};

export async function executeServiceCall(options: {
  workerUrl: string;
  service: ServiceOffer;
  prompt: string;
  mode: 'inspect' | 'autopay';
  pay?: (url: string, init: RequestInit) => Promise<Fetch402Result>;
  fetchImpl?: typeof fetch;
}): Promise<ToolCallResult> {
  const url = `${options.workerUrl.replace(/\/$/, '')}/api/v1/compute`;
  const init: RequestInit = {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ prompt: options.prompt, service: options.service.name }),
  };
  if (options.mode === 'inspect') {
    const response = await (options.fetchImpl ?? fetch)(url, init);
    if (response.status === 402) {
      const challenge = readChallenge(response.headers.get('PAYMENT-REQUIRED'));
      return {
        content: [
          {
            type: 'text',
            text: [
              `${options.service.name} requires payment before it will run.`,
              `Amount: ${options.service.price} tUSD8 (${challenge?.amount ?? options.service.priceUnits} base units).`,
              `Chain ID: 44005 (${challenge?.network ?? 'eip155:44005'}).`,
              `Asset: ${challenge?.asset ?? '0x456569838f1d43009A91ebA4263147Afd7233483'}.`,
              `taskHash: ${challenge?.taskHash ?? 'unavailable'}.`,
              'No funds were spent. Set ORCHESTRATOR_AGENT_PRIVATE_KEY and PRINCIPAL_ADDRESS to auto-pay.',
            ].join('\n'),
          },
        ],
      };
    }
    return { content: [{ type: 'text', text: await response.text() }] };
  }
  if (!options.pay) {
    return { isError: true, content: [{ type: 'text', text: 'Auto-pay mode has no payment client.' }] };
  }
  try {
    const paid = await options.pay(url, init);
    const text = await paid.response.text();
    return {
      content: [{ type: 'text', text }],
      isError: !paid.response.ok,
      _meta: paid.payment ? { 'x402/payment-response': paid.payment } : undefined,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'payment failed';
    return { isError: true, content: [{ type: 'text', text: message }] };
  }
}

function readChallenge(header: string | null): Challenge | null {
  if (!header) return null;
  try {
    const envelope = JSON.parse(decodeBase64(header)) as {
      accepts?: Array<Record<string, string | number>>;
    };
    const accepted = envelope.accepts?.[0];
    if (!accepted) return null;
    return {
      amount: String(accepted.amount ?? ''),
      taskHash: String(accepted.taskHash ?? ''),
      network: String(accepted.network ?? ''),
      asset: String(accepted.asset ?? ''),
    };
  } catch {
    return null;
  }
}

function decodeBase64(value: string) {
  const binary = atob(value);
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}
