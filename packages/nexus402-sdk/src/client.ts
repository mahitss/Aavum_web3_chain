import {
  createPublicClient,
  createWalletClient,
  decodeEventLog,
  defineChain,
  http,
  isAddress,
  type Address,
  type Hex,
} from 'viem';
import { privateKeyToAccount } from 'viem/accounts';

export const TUSD8_ADDRESS = '0x456569838f1d43009A91ebA4263147Afd7233483' as const;
const AEVUM_NETWORK = 'eip155:44005';

const aevum = defineChain({
  id: 44005,
  name: 'QDay Aevum Testnet',
  nativeCurrency: { name: 'tQDAY', symbol: 'tQDAY', decimals: 18 },
  rpcUrls: { default: { http: ['https://rpc-test.qday.info'] } },
});

const escrowAbi = [
  {
    type: 'function',
    name: 'lockTaskEscrow',
    stateMutability: 'nonpayable',
    inputs: [
      { name: 'principal', type: 'address' },
      { name: 'worker', type: 'address' },
      { name: 'amount', type: 'uint256' },
      { name: 'durationBlocks', type: 'uint256' },
      { name: 'taskHash', type: 'bytes32' },
    ],
    outputs: [{ name: 'taskId', type: 'bytes32' }],
  },
  {
    type: 'event',
    name: 'TaskEscrowLocked',
    inputs: [
      { name: 'taskId', type: 'bytes32', indexed: true },
      { name: 'principal', type: 'address', indexed: true },
      { name: 'worker', type: 'address', indexed: true },
      { name: 'amount', type: 'uint256', indexed: false },
      { name: 'deadline', type: 'uint256', indexed: false },
    ],
  },
] as const;

export type PaymentChallenge = {
  scheme: string;
  network: string;
  asset: Address;
  amount: bigint;
  payTo: Address;
  taskHash: Hex;
  timeoutBlocks: bigint;
};

export type LockResult = { taskId: Hex; lockTxHash: Hex };

export type NexusClientConfig = {
  rpcUrl: string;
  escrowAddress: Address;
  principalAddress: Address;
  agentPrivateKey: Hex;
  maxPerTaskAmount: bigint;
  lockEscrow?: (challenge: PaymentChallenge) => Promise<LockResult>;
};

export type Fetch402Result = {
  response: Response;
  payment: {
    taskId: Hex;
    lockTxHash: Hex;
    claimTxHash: Hex | null;
    explorerUrl: string;
  } | null;
};

export type NexusClient = {
  fetch402: (url: string, init?: RequestInit) => Promise<Fetch402Result>;
};

export function createNexusClient(config: NexusClientConfig): NexusClient {
  const lock = config.lockEscrow ?? ((challenge: PaymentChallenge) => lockOnChain(config, challenge));
  return {
    async fetch402(url, init) {
      const first = await fetch(url, init);
      if (first.status !== 402) return { response: first, payment: null };
      const challenge = decodePaymentRequired(first.headers.get('PAYMENT-REQUIRED'));
      assertChallenge(challenge, config.maxPerTaskAmount);
      const locked = await lock(challenge);
      const signature = encodeHeader({
        x402Version: 2,
        taskId: locked.taskId,
        lockTxHash: locked.lockTxHash,
        agentKey: accountAddress(config.agentPrivateKey),
        principal: config.principalAddress,
      });
      const retry = await fetch(url, withHeader(init, 'PAYMENT-SIGNATURE', signature));
      const paymentHeader = retry.headers.get('PAYMENT-RESPONSE');
      const decoded = paymentHeader ? decodePaymentResponse(paymentHeader) : null;
      return {
        response: retry,
        payment: {
          taskId: locked.taskId,
          lockTxHash: locked.lockTxHash,
          claimTxHash: decoded?.claimTxHash ?? null,
          explorerUrl: decoded?.explorerUrl || `https://explorer-test.qday.info/tx/${locked.lockTxHash}`,
        },
      };
    },
  };
}

export function decodePaymentRequired(header: string | null): PaymentChallenge {
  if (!header) throw new Error('402 response is missing PAYMENT-REQUIRED');
  const envelope = JSON.parse(decodeBase64(header)) as {
    x402Version?: number;
    accepts?: Array<Record<string, string | number>>;
  };
  const accepted = envelope.accepts?.[0];
  if (!accepted || envelope.x402Version !== 2) throw new Error('unsupported x402 payment challenge');
  const asset = String(accepted.asset ?? '');
  const payTo = String(accepted.payTo ?? '');
  const taskHash = String(accepted.taskHash ?? '');
  if (!isAddress(asset) || !isAddress(payTo) || !/^0x[0-9a-fA-F]{64}$/.test(taskHash)) {
    throw new Error('malformed PAYMENT-REQUIRED challenge');
  }
  return {
    scheme: String(accepted.scheme ?? ''),
    network: String(accepted.network ?? ''),
    asset,
    amount: BigInt(accepted.amount ?? 0),
    payTo,
    taskHash: taskHash as Hex,
    timeoutBlocks: BigInt(accepted.timeoutBlocks ?? 50),
  };
}

export function assertChallenge(challenge: PaymentChallenge, maxPerTaskAmount: bigint) {
  if (challenge.network !== AEVUM_NETWORK) throw new Error(`unsupported network ${challenge.network}`);
  if (challenge.asset.toLowerCase() !== TUSD8_ADDRESS.toLowerCase()) throw new Error('unsupported settlement asset');
  if (challenge.scheme !== 'nexus-escrow-v1') throw new Error(`unsupported scheme ${challenge.scheme}`);
  if (challenge.amount > maxPerTaskAmount) {
    throw new Error(`amount ${challenge.amount} exceeds maxPerTaskAmount ${maxPerTaskAmount}`);
  }
}

function decodePaymentResponse(header: string) {
  const body = JSON.parse(decodeBase64(header)) as { claimTxHash?: string; explorerUrl?: string };
  const claim = body.claimTxHash && /^0x[0-9a-fA-F]{64}$/.test(body.claimTxHash) ? (body.claimTxHash as Hex) : null;
  return { claimTxHash: claim, explorerUrl: body.explorerUrl };
}

async function lockOnChain(config: NexusClientConfig, challenge: PaymentChallenge): Promise<LockResult> {
  const account = privateKeyToAccount(config.agentPrivateKey);
  const transport = http(config.rpcUrl);
  const publicClient = createPublicClient({ chain: aevum, transport });
  const wallet = createWalletClient({ account, chain: aevum, transport });
  const lockTxHash = await wallet.writeContract({
    address: config.escrowAddress,
    abi: escrowAbi,
    functionName: 'lockTaskEscrow',
    args: [config.principalAddress, challenge.payTo, challenge.amount, challenge.timeoutBlocks, challenge.taskHash],
  });
  const receipt = await publicClient.waitForTransactionReceipt({ hash: lockTxHash });
  for (const log of receipt.logs) {
    if (log.address.toLowerCase() !== config.escrowAddress.toLowerCase()) continue;
    try {
      const decoded = decodeEventLog({ abi: escrowAbi, data: log.data, topics: log.topics });
      if (decoded.eventName === 'TaskEscrowLocked') {
        return { taskId: decoded.args.taskId, lockTxHash };
      }
    } catch {
      continue;
    }
  }
  throw new Error('TaskEscrowLocked was not found in the lock receipt');
}

function accountAddress(privateKey: Hex): Address {
  return privateKeyToAccount(privateKey).address;
}

function withHeader(init: RequestInit | undefined, name: string, value: string): RequestInit {
  const headers = new Headers(init?.headers);
  headers.set(name, value);
  return { ...init, headers };
}

function encodeHeader(value: unknown) {
  const bytes = new TextEncoder().encode(JSON.stringify(value));
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function decodeBase64(value: string) {
  const binary = atob(value);
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}
