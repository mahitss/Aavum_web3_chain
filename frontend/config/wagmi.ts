import { getDefaultConfig } from '@rainbow-me/rainbowkit';
import { aevumTestnet } from './chains';

export const walletConnectProjectId =
  process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID || '00000000000000000000000000000000';

export const wagmiConfig = getDefaultConfig({
  appName: 'Nexus402 Command Center',
  projectId: walletConnectProjectId,
  chains: [aevumTestnet],
  ssr: true,
});
