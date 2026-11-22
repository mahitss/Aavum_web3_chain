'use client';

import { isAddress, type Address, type Hex } from 'viem';
import { useAccount, useWriteContract } from 'wagmi';
import { NEXUS_ESCROW_ADDRESS, nexusEscrowAbi } from '@/lib/contracts';
import { cn } from '@/lib/format';

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
    <section className={cn('panel rounded-2xl p-5', killed && 'armed border-danger')}>
      <p className="text-xs uppercase tracking-[0.2em] text-danger">Circuit breaker</p>
      <div className="mt-3 flex items-center justify-between gap-4">
        <div>
          <h2 className="text-lg">Emergency revoke</h2>
          <p className={cn('mt-1 font-mono text-sm', killed ? 'animate-pulse text-danger' : 'text-emerald-300')}>
            {killed ? 'SESSION_KILLED' : 'OPERATIONAL'}
          </p>
        </div>
        <button
          type="button"
          onClick={revoke}
          disabled={isPending || !agent || !NEXUS_ESCROW_ADDRESS}
          className="rounded-lg bg-danger px-4 py-3 text-sm font-semibold tracking-wide text-white disabled:opacity-40"
        >
          {isPending ? 'REVOKING' : 'EMERGENCY REVOKE AGENT KEY'}
        </button>
      </div>
      {error ? <p className="mt-3 text-xs text-danger">{error.message}</p> : null}
    </section>
  );
}
