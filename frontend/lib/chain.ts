import { defineChain, isAddress } from 'viem';

const rpcUrl = process.env.NEXT_PUBLIC_AEVUM_RPC_URL || 'https://rpc-test.qday.info';

export const aevum = defineChain({
  id: 44005,
  name: 'QDay Aevum Testnet',
  nativeCurrency: { name: 'tQDAY', symbol: 'tQDAY', decimals: 18 },
  rpcUrls: { default: { http: [rpcUrl] } },
  blockExplorers: {
    default: { name: 'Aevum Explorer', url: 'https://explorer-test.qday.info' },
  },
  testnet: true,
});

export const TUSD8 = '0x456569838f1d43009A91ebA4263147Afd7233483' as const;

const configured = process.env.NEXT_PUBLIC_ESCROW_ADDRESS ?? '';
export const escrowAddress = isAddress(configured) ? configured : undefined;

export const orchestratorUrl =
  process.env.NEXT_PUBLIC_ORCHESTRATOR_URL || 'http://127.0.0.1:8080';

export const walletConnectProjectId =
  process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID || '00000000000000000000000000000000';

export function explorerTx(hash: string) {
  return `${aevum.blockExplorers.default.url}/tx/${hash}`;
}
