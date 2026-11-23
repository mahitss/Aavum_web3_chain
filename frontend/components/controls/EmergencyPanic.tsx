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
    <section className={cn('panel rounded-2xl border border-panic/40 p-5', dead && 'armed')}>
      <p className="text-xs uppercase tracking-[0.2em] text-panic">Protocol kill switch</p>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-lg">PROTOCOL KILL SWITCH</h2>
          <p className="mt-1 font-mono text-xs text-slate-400">{agent ? shortHash(agent) : 'No agent key'}</p>
          <p className={cn('mt-1 font-mono text-sm', dead ? 'animate-pulse text-panic' : onChainActive ? 'text-emerald' : 'text-slate-400')}>
            {dead ? 'SESSION_KILLED' : onChainActive ? 'OPERATIONAL' : 'INACTIVE'}
          </p>
        </div>
        <button
          type="button"
          onClick={revoke}
          disabled={isPending || !agent || !NEXUS_ESCROW_ADDRESS}
          className="rounded-lg bg-panic px-4 py-3 text-sm font-semibold tracking-wide text-white disabled:opacity-40"
        >
          {isPending ? 'REVOKING' : 'EMERGENCY REVOKE AGENT KEY'}
        </button>
      </div>
      {error ? <p className="mt-3 text-xs text-panic">{error.message}</p> : null}
    </section>
  );
}
