'use client';

import { useAccount, useReadContract, useWriteContract } from 'wagmi';
import { NEXUS_ESCROW_ADDRESS, nexusEscrowAbi } from '@/lib/contracts';
import { tusd } from '@/lib/format';

type Vault = {
  globalHourlyLimit: bigint;
  globalCurrentWindow: bigint;
  globalSpentInWindow: bigint;
  globalPaused: boolean;
};

export function OrgVaultPanel() {
  const { address, isConnected } = useAccount();
  const write = useWriteContract();
  const vault = useReadContract({
    address: NEXUS_ESCROW_ADDRESS,
    abi: nexusEscrowAbi,
    functionName: 'getOrgVault',
    args: address ? [address] : undefined,
    query: { enabled: Boolean(isConnected && address && NEXUS_ESCROW_ADDRESS), refetchInterval: 8000 },
  });
  const data = vault.data as Vault | undefined;
  const limit = data?.globalHourlyLimit ?? 0n;
  const spent = data?.globalSpentInWindow ?? 0n;
  const paused = Boolean(data?.globalPaused);
  const ratio = limit > 0n ? Number((spent * 100n) / limit) : 0;

  function toggle() {
    if (!NEXUS_ESCROW_ADDRESS) return;
    write.writeContract({
      address: NEXUS_ESCROW_ADDRESS,
      abi: nexusEscrowAbi,
      functionName: paused ? 'unpauseAllAgents' : 'emergencyPauseAllAgents',
    });
  }

  return (
    <section className="panel rounded-2xl border border-panic/40 p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-medium">Org vault</h2>
          <p className="mt-1 text-sm text-slate-400">
            {limit > 0n ? `${tusd(spent)} / ${tusd(limit)} tUSD8 this hour` : 'Global hourly cap is not set'}
          </p>
        </div>
        <button
          type="button"
          onClick={toggle}
          disabled={!isConnected || !NEXUS_ESCROW_ADDRESS || write.isPending}
          className="rounded-lg bg-panic px-4 py-3 text-sm font-semibold text-white disabled:opacity-40"
        >
          {write.isPending ? 'SUBMITTING' : paused ? 'UNPAUSE ALL AGENTS' : 'PAUSE ALL AGENTS'}
        </button>
      </div>
      <div className="mt-3 h-2 overflow-hidden rounded-full bg-black/40" aria-label="Global hourly spend">
        <div className="h-full bg-panic" style={{ width: `${Math.min(100, Math.max(0, ratio))}%` }} />
      </div>
      <p className="mt-2 text-xs text-slate-400">{paused ? 'ORG VAULT PAUSED' : 'All delegated agent keys share this ceiling.'}</p>
      {write.error ? <p className="mt-2 text-xs text-panic">{write.error.message}</p> : null}
    </section>
  );
}
