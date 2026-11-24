'use client';

import { isAddress, zeroAddress, type Address, type Hex } from 'viem';
import { useAccount, useReadContract, useWriteContract } from 'wagmi';
import { NEXUS_ESCROW_ADDRESS, nexusEscrowAbi } from '@/lib/contracts';
import { cn, shortHash } from '@/lib/format';

export function EmergencyPanic({
  agentKey,
  killed,
  onKilled,
}: {
  agentKey: string;
  killed: boolean;
  onKilled: (hash: Hex) => void;
}) {
  const { address } = useAccount();
  const { writeContract, isPending, error } = useWriteContract();
  const agent = isAddress(agentKey) ? (agentKey as Address) : undefined;
  const session = useReadContract({
    address: NEXUS_ESCROW_ADDRESS,
    abi: nexusEscrowAbi,
    functionName: 'getSession',
    args: address && agent ? [address, agent] : undefined,
    query: { enabled: Boolean(address && agent && NEXUS_ESCROW_ADDRESS), refetchInterval: 8000 },
  });
  const onChainActive = Boolean(session.data?.[6]);
  const registered = Boolean(session.data && session.data[0].toLowerCase() !== zeroAddress);
  const dead = killed || (registered && !onChainActive);

  function revoke() {
    if (!address || !agent || !NEXUS_ESCROW_ADDRESS) return;
    writeContract(
      {
        address: NEXUS_ESCROW_ADDRESS,
        abi: nexusEscrowAbi,
        functionName: 'emergencyRevokeSession',
        args: [agent],
      },
      { onSuccess: onKilled },
    );
  }

  return (
    <section className={cn('panel rounded-2xl border border-panic/50 p-5', dead && 'armed')}>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-base font-medium">Stop new spend</h2>
          <p className="mt-1 text-sm text-slate-400">
            {agent ? `Agent ${shortHash(agent)}` : 'Enter an agent address above before this can be used.'}
          </p>
          <p className={cn('mt-1 font-mono text-sm', dead ? 'text-panic' : onChainActive ? 'text-emerald' : 'text-slate-400')}>
            {dead ? 'SESSION_REVOKED' : onChainActive ? 'OPERATIONAL' : 'INACTIVE'}
          </p>
        </div>
        <button
          type="button"
          onClick={revoke}
          disabled={isPending || !agent || !NEXUS_ESCROW_ADDRESS}
          aria-label="Emergency revoke agent key"
          className="rounded-lg bg-panic px-4 py-3 text-sm font-semibold tracking-wide text-white disabled:opacity-40"
        >
          {isPending ? 'REVOKING' : 'EMERGENCY REVOKE AGENT KEY'}
        </button>
      </div>
      {error ? (
        <p className="mt-3 text-xs text-panic" role="alert">
          {error.message}
        </p>
      ) : null}
    </section>
  );
}
