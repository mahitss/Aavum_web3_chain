'use client';

import { FormEvent, useMemo, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { isAddress, parseUnits, type Address, type Hex } from 'viem';
import { useAccount, useReadContract, useSignTypedData, useWaitForTransactionReceipt, useWriteContract } from 'wagmi';
import { explorerTx } from '@/config/chains';
import { erc20Abi, NEXUS_ESCROW_ADDRESS, nexusEscrowAbi, TUSD8_ADDRESS } from '@/lib/contracts';
import { cn, tusd } from '@/lib/format';

function clampNumber(raw: string, min: number, max: number) {
  const value = Number(raw);
  if (!Number.isFinite(value)) return min;
  return Math.min(max, Math.max(min, value));
}

const EXPIRY = [
  { label: '1 Hour', seconds: 60 * 60 },
  { label: '6 Hours', seconds: 6 * 60 * 60 },
  { label: '24 Hours', seconds: 24 * 60 * 60 },
];

export function SessionDelegator({
  agentKey,
  onAgentKey,
  onSession,
}: {
  agentKey: string;
  onAgentKey: (value: string) => void;
  onSession: (hash: Hex) => void;
}) {
  const { address } = useAccount();
  const [hourly, setHourly] = useState(10);
  const [perTask, setPerTask] = useState(0.25);
  const [expiry, setExpiry] = useState(EXPIRY[2].seconds);
  const [message, setMessage] = useState('Sign a scoped session, then approve tUSD8 for the escrow.');
  const { signTypedDataAsync, isPending: signing } = useSignTypedData();
  const { writeContract, data: hash, isPending } = useWriteContract();
  const receipt = useWaitForTransactionReceipt({ hash });

  const hourlyUnits = useMemo(() => parseUnits(hourly.toString(), 6), [hourly]);
  const perTaskUnits = useMemo(() => parseUnits(perTask.toString(), 6), [perTask]);
  const agent = isAddress(agentKey) ? (agentKey as Address) : undefined;
  const busy = signing || isPending || receipt.isLoading;

  const nonce = useReadContract({
    address: NEXUS_ESCROW_ADDRESS,
    abi: nexusEscrowAbi,
    functionName: 'nonces',
    args: address ? [address] : undefined,
    query: { enabled: Boolean(address && NEXUS_ESCROW_ADDRESS) },
  });
  const session = useReadContract({
    address: NEXUS_ESCROW_ADDRESS,
    abi: nexusEscrowAbi,
    functionName: 'getSession',
    args: address && agent ? [address, agent] : undefined,
    query: { enabled: Boolean(address && agent && NEXUS_ESCROW_ADDRESS), refetchInterval: 8000 },
  });
  const allowance = useReadContract({
    address: TUSD8_ADDRESS,
    abi: erc20Abi,
    functionName: 'allowance',
    args: address && NEXUS_ESCROW_ADDRESS ? [address, NEXUS_ESCROW_ADDRESS] : undefined,
    query: { enabled: Boolean(address && NEXUS_ESCROW_ADDRESS), refetchInterval: 8000 },
  });

  const active = Boolean(session.data && session.data[6]);
  const needsApproval = allowance.data !== undefined && allowance.data < hourlyUnits;

  async function authorize(event: FormEvent) {
    event.preventDefault();
    if (!address || !agent || !NEXUS_ESCROW_ADDRESS || nonce.data === undefined) {
      setMessage('Connect a wallet on Aevum and enter the orchestrator agent address.');
      return;
    }
    const expiresAt = BigInt(Math.floor(Date.now() / 1000) + expiry);
    const signature = await signTypedDataAsync({
      domain: {
        name: 'Nexus402 Protocol',
        version: '1',
        chainId: 44005,
        verifyingContract: NEXUS_ESCROW_ADDRESS,
      },
      types: {
        SessionAuthorization: [
          { name: 'principal', type: 'address' },
          { name: 'agentKey', type: 'address' },
          { name: 'hourlyLimit', type: 'uint256' },
          { name: 'perTaskLimit', type: 'uint256' },
          { name: 'expiresAt', type: 'uint256' },
          { name: 'nonce', type: 'uint256' },
        ],
      },
      primaryType: 'SessionAuthorization',
      message: {
        principal: address,
        agentKey: agent,
        hourlyLimit: hourlyUnits,
        perTaskLimit: perTaskUnits,
        expiresAt,
        nonce: nonce.data,
      },
    });
    const r = `0x${signature.slice(2, 66)}` as Hex;
    const s = `0x${signature.slice(66, 130)}` as Hex;
    const v = Number.parseInt(signature.slice(130, 132), 16);
    writeContract(
      {
        address: NEXUS_ESCROW_ADDRESS,
        abi: nexusEscrowAbi,
        functionName: 'registerSessionWithSig',
        args: [address, agent, hourlyUnits, perTaskUnits, expiresAt, v, r, s],
      },
      {
        onSuccess: (tx) => {
          setMessage('Session authorization submitted.');
          onSession(tx);
        },
        onError: (error) => setMessage(error.message),
      },
    );
  }

  function approve() {
    if (!NEXUS_ESCROW_ADDRESS) return;
    writeContract(
      {
        address: TUSD8_ADDRESS,
        abi: erc20Abi,
        functionName: 'approve',
        args: [NEXUS_ESCROW_ADDRESS, hourlyUnits],
      },
      {
        onSuccess: (tx) => {
          setMessage(`tUSD8 approval submitted for ${tusd(hourlyUnits)} tokens.`);
          onSession(tx);
        },
        onError: (error) => setMessage(error.message),
      },
    );
  }

  return (
    <form onSubmit={authorize} className="panel space-y-4 rounded-2xl p-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm text-slate-400">Step 1</p>
          <h2 className="text-lg font-medium">Set the spend cap</h2>
        </div>
        <span className={cn('rounded-full px-3 py-1 font-mono text-xs', active ? 'bg-emerald/15 text-emerald' : 'bg-white/5 text-slate-400')}>
          {active ? 'OPERATIONAL' : 'INACTIVE'}
        </span>
      </div>
      <label className="block text-sm text-slate-300">
        Agent public key
        <input
          value={agentKey}
          onChange={(event) => onAgentKey(event.target.value.trim())}
          placeholder="0x orchestrator agent address"
          className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 font-mono text-sm outline-none focus:border-emerald"
        />
      </label>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="block text-sm">
          Hourly limit (tUSD8)
          <input
            type="number"
            min={1}
            max={100}
            step={1}
            value={hourly}
            onChange={(event) => setHourly(clampNumber(event.target.value, 1, 100))}
            className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm"
          />
          <input type="range" min={1} max={100} step={1} value={hourly} onChange={(event) => setHourly(Number(event.target.value))} className="mt-2 w-full" aria-label="Hourly spend limit slider" />
        </label>
        <label className="block text-sm">
          Per-task limit (tUSD8)
          <input
            type="number"
            min={0.1}
            max={10}
            step={0.05}
            value={perTask}
            onChange={(event) => setPerTask(clampNumber(event.target.value, 0.1, 10))}
            className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm"
          />
          <input type="range" min={0.1} max={10} step={0.05} value={perTask} onChange={(event) => setPerTask(Number(event.target.value))} className="mt-2 w-full" aria-label="Per-task cap slider" />
        </label>
      </div>
      <fieldset className="flex flex-wrap gap-2 text-sm">
        <legend className="mb-2 w-full text-sm text-slate-300">Session length</legend>
        {EXPIRY.map((option) => (
          <label key={option.seconds} className={cn('inline-flex min-h-10 items-center gap-2 rounded-lg border px-3', expiry === option.seconds ? 'border-emerald text-emerald' : 'border-line text-slate-300')}>
            <input type="radio" name="expiry" checked={expiry === option.seconds} onChange={() => setExpiry(option.seconds)} />
            {option.label}
          </label>
        ))}
      </fieldset>
      <div className="flex flex-wrap items-center gap-2">
        {needsApproval ? (
          <button type="button" onClick={approve} disabled={busy || !NEXUS_ESCROW_ADDRESS} className="rounded-lg border border-amber px-4 py-2 text-sm text-amber disabled:opacity-50">
            Approve tUSD8
          </button>
        ) : null}
        <button type="submit" disabled={busy} className="inline-flex items-center gap-2 rounded-lg bg-emerald px-4 py-2 text-sm font-medium text-canvas disabled:opacity-50">
          {busy ? <Loader2 className="animate-spin" size={14} /> : null}
          {busy ? 'Confirm in wallet' : 'Sign and register'}
        </button>
      </div>
      <p className="font-mono text-xs text-slate-400">
        {hash ? (
          <a className="text-cyan underline-offset-2 hover:underline" href={explorerTx(hash)} target="_blank" rel="noreferrer">
            {receipt.isSuccess ? 'Confirmed' : 'Submitted'} {hash}
          </a>
        ) : (
          message
        )}
        {!NEXUS_ESCROW_ADDRESS ? ' Set NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS to enable writes.' : ''}
      </p>
    </form>
  );
}
