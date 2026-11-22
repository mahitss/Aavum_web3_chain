import { defineChain } from 'viem';

export const aevumTestnet = defineChain({
  id: 44005,
  name: 'QDay Aevum Testnet',
  nativeCurrency: { name: 'tQDAY', symbol: 'tQDAY', decimals: 18 },
  rpcUrls: {
    default: { http: [process.env.NEXT_PUBLIC_AEVUM_RPC_URL || 'https://rpc-test.qday.info'] },
  },
  blockExplorers: {
    default: { name: 'Aevum Explorer', url: 'https://explorer-test.qday.info' },
  },
  testnet: true,
});

export const explorerUrl = aevumTestnet.blockExplorers.default.url;

export function explorerTx(hash: string) {
  return `${explorerUrl}/tx/${hash}`;
}
