import { getDefaultConfig } from '@rainbow-me/rainbowkit';
import { aevum, walletConnectProjectId } from './chain';

export const wagmiConfig = getDefaultConfig({
  appName: 'Nexus402 Command Center',
  projectId: walletConnectProjectId,
  chains: [aevum],
  ssr: true,
});
