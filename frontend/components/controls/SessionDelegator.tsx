'use client';

import { FormEvent, useMemo, useState } from 'react';
import { isAddress, parseUnits, type Address, type Hex } from 'viem';
import { useAccount, useReadContract, useSignTypedData, useWaitForTransactionReceipt, useWriteContract } from 'wagmi';
import { erc20Abi, NEXUS_ESCROW_ADDRESS, nexusEscrowAbi, TUSD8_ADDRESS } from '@/lib/contracts';
import { cn, tusd } from '@/lib/format';

const EXPIRY = [
  { label: '1 hour', seconds: 60 * 60 },
  { label: '6 hours', seconds: 6 * 60 * 60 },
  { label: '24 hours', seconds: 24 * 60 * 60 },
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
  const { signTypedDataAsync } = useSignTypedData();
  const { writeContract, data: hash, isPending } = useWriteContract();
  const receipt = useWaitForTransactionReceipt({ hash });

  const hourlyUnits = useMemo(() => parseUnits(hourly.toString(), 6), [hourly]);
  const perTaskUnits = useMemo(() => parseUnits(perTask.toString(), 6), [perTask]);
  const agent = isAddress(agentKey) ? (agentKey as Address) : undefined;

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
    query: { enabled: Boolean(address && agent && NEXUS_ESCROW_ADDRESS) },
  });

  const active = Boolean(session.data && session.data[6]);

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
          <p className="text-xs uppercase tracking-[0.2em] text-signal">Session delegation</p>
          <h2 className="text-lg font-medium">Authorize the orchestrator key</h2>
        </div>
        <span
          className={cn(
            'rounded-full px-3 py-1 text-xs',
            active ? 'bg-emerald-400/15 text-emerald-300' : 'bg-white/5 text-slate-400',
          )}
        >
          {active ? 'OPERATIONAL' : 'INACTIVE'}
        </span>
      </div>
      <label className="block text-sm text-slate-300">
        Agent public key
        <input
          value={agentKey}
          onChange={(event) => onAgentKey(event.target.value.trim())}
          placeholder="0x orchestrator agent address"
          className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 font-mono text-sm outline-none focus:border-signal"
        />
      </label>
      <label className="block text-sm">
        Hourly spend limit · {hourly} tUSD8
        <input
          type="range"
          min={1}
          max={50}
          value={hourly}
          onChange={(event) => setHourly(Number(event.target.value))}
          className="mt-2 w-full"
        />
      </label>
      <label className="block text-sm">
        Per-task limit · {perTask.toFixed(2)} tUSD8
        <input
          type="range"
          min={1}
          max={50}
          value={Math.round(perTask * 10)}
          onChange={(event) => setPerTask(Number(event.target.value) / 10)}
          className="mt-2 w-full"
        />
      </label>
      <div className="flex gap-2">
        {EXPIRY.map((option) => (
          <button
            key={option.seconds}
            type="button"
            onClick={() => setExpiry(option.seconds)}
            className={cn(
              'rounded-full border px-3 py-1 text-xs',
              expiry === option.seconds ? 'border-signal text-signal' : 'border-line text-slate-400',
            )}
          >
            {option.label}
          </button>
        ))}
      </div>
      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          disabled={isPending}
          className="rounded-lg bg-signal px-4 py-2 text-sm font-medium text-ink disabled:opacity-50"
        >
          {isPending ? 'Confirm in wallet' : 'Sign and register'}
        </button>
        <button
          type="button"
          onClick={approve}
          disabled={isPending || !NEXUS_ESCROW_ADDRESS}
          className="rounded-lg border border-line px-4 py-2 text-sm"
        >
          Approve tUSD8
        </button>
      </div>
      <p className="text-xs text-slate-400">
        {receipt.isSuccess ? `Confirmed ${hash}` : message}
        {!NEXUS_ESCROW_ADDRESS ? ' Set NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS to enable writes.' : ''}
      </p>
    </form>
  );
}
