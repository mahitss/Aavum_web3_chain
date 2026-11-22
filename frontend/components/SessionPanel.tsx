'use client';

import { isAddress, type Hex } from 'viem';
import { useAccount, useChainId, usePublicClient, useSignTypedData, useSwitchChain, useWriteContract } from 'wagmi';
import { useReadContract } from 'wagmi';
import { erc20Abi, nexusEscrowAbi, sessionTypes } from '@/lib/abi';
import { aevum, escrowAddress, TUSD8 } from '@/lib/chain';
import { errorText, formatTusd, parseTusd } from '@/lib/format';

type Activity = { stream: 'http' | 'chain'; text: string; tx?: string };

export function SessionPanel({
  agentKey,
  setAgentKey,
  hourly,
  setHourly,
  perTask,
  setPerTask,
  hours,
  setHours,
  onActivity,
  onPulse,
}: {
  agentKey: string;
  setAgentKey: (value: string) => void;
  hourly: number;
  setHourly: (value: number) => void;
  perTask: string;
  setPerTask: (value: string) => void;
  hours: number;
  setHours: (value: number) => void;
  onActivity: (activity: Activity) => void;
  onPulse: () => void;
}) {
  const { address, isConnected } = useAccount();
  const chainId = useChainId();
  const { switchChainAsync } = useSwitchChain();
  const publicClient = usePublicClient();
  const { signTypedDataAsync } = useSignTypedData();
  const { writeContractAsync, isPending } = useWriteContract();

  const balance = useReadContract({
    address: TUSD8,
    abi: erc20Abi,
    functionName: 'balanceOf',
    args: address ? [address] : undefined,
    query: { enabled: Boolean(address) },
  });
  const allowance = useReadContract({
    address: TUSD8,
    abi: erc20Abi,
    functionName: 'allowance',
    args: address && escrowAddress ? [address, escrowAddress] : undefined,
    query: { enabled: Boolean(address && escrowAddress) },
  });

  const ready = Boolean(isConnected && escrowAddress && isAddress(agentKey));

  async function ensureChain() {
    if (chainId !== aevum.id) {
      onActivity({ stream: 'http', text: 'switching wallet to Aevum 44005' });
      await switchChainAsync({ chainId: aevum.id });
    }
  }

  async function approve() {
    if (!address || !escrowAddress || !publicClient) return;
    try {
      await ensureChain();
      const amount = parseTusd(String(hourly)) * 24n;
      onActivity({ stream: 'http', text: `approving ${formatTusd(amount)} tUSD8 to escrow` });
      const hash = await writeContractAsync({
        address: TUSD8,
        abi: erc20Abi,
        functionName: 'approve',
        args: [escrowAddress, amount],
        chainId: aevum.id,
      });
      onActivity({ stream: 'chain', text: 'tUSD8 approval submitted', tx: hash });
      await publicClient.waitForTransactionReceipt({ hash });
      onActivity({ stream: 'chain', text: 'tUSD8 approval confirmed', tx: hash });
      await allowance.refetch();
      onPulse();
    } catch (error) {
      onActivity({ stream: 'http', text: errorText(error) });
    }
  }

  async function authorize() {
    if (!address || !escrowAddress || !publicClient || !isAddress(agentKey)) return;
    try {
      await ensureChain();
      const hourlyLimit = parseTusd(String(hourly));
      const perTaskLimit = parseTusd(perTask);
      if (perTaskLimit > hourlyLimit) {
        throw new Error('Per-task cap cannot exceed the hourly cap');
      }
      const nonce = await publicClient.readContract({
        address: escrowAddress,
        abi: nexusEscrowAbi,
        functionName: 'nonces',
        args: [address],
      });
      const expiresAt = BigInt(Math.floor(Date.now() / 1000) + hours * 3600);
      onActivity({
        stream: 'http',
        text: `signing EIP-712 session for ${agentKey.slice(0, 8)}… nonce ${nonce.toString()}`,
      });
      const signature = await signTypedDataAsync({
        domain: {
          name: 'Nexus402 Protocol',
          version: '1',
          chainId: aevum.id,
          verifyingContract: escrowAddress,
        },
        types: sessionTypes,
        primaryType: 'SessionAuthorization',
        message: {
          principal: address,
          agentKey: agentKey as Hex,
          hourlyLimit,
          perTaskLimit,
          expiresAt,
          nonce,
        },
      });
      onActivity({ stream: 'http', text: `typed-data signature ${signature.slice(0, 18)}…` });
      const hash = await writeContractAsync({
        address: escrowAddress,
        abi: nexusEscrowAbi,
        functionName: 'registerSessionWithSig',
        args: [address, agentKey as Hex, hourlyLimit, perTaskLimit, expiresAt, nonce, signature],
        chainId: aevum.id,
      });
      onActivity({ stream: 'chain', text: 'registerSessionWithSig submitted', tx: hash });
      await publicClient.waitForTransactionReceipt({ hash });
      onActivity({ stream: 'chain', text: 'session authorized on Aevum', tx: hash });
      onPulse();
    } catch (error) {
      onActivity({ stream: 'http', text: errorText(error) });
    }
  }

  return (
    <section className="panel flex flex-col gap-4 rounded-2xl p-4">
      <div>
        <p className="text-xs uppercase tracking-[0.18em] text-signal">Session delegator</p>
        <h2 className="mt-1 text-lg font-medium">Grant an ephemeral agent key</h2>
      </div>
      <label className="block text-sm text-slate-300">
        Hourly cap · {hourly.toFixed(2)} tUSD8
        <input
          className="mt-2 w-full"
          type="range"
          min={0.25}
          max={100}
          step={0.25}
          value={hourly}
          onChange={(event) => setHourly(Number(event.target.value))}
        />
      </label>
      <label className="block text-sm text-slate-300">
        Per-task cap (tUSD8)
        <input
          className="mt-2 w-full rounded-lg border border-line bg-ink px-3 py-2 font-mono text-sm outline-none focus:border-signal"
          value={perTask}
          onChange={(event) => setPerTask(event.target.value)}
          inputMode="decimal"
        />
      </label>
      <label className="block text-sm text-slate-300">
        Expires in {hours}h
        <input
          className="mt-2 w-full"
          type="range"
          min={1}
          max={168}
          step={1}
          value={hours}
          onChange={(event) => setHours(Number(event.target.value))}
        />
      </label>
      <label className="block text-sm text-slate-300">
        Orchestrator session address
        <input
          className="mt-2 w-full rounded-lg border border-line bg-ink px-3 py-2 font-mono text-xs outline-none focus:border-signal"
          value={agentKey}
          placeholder="0x…"
          onChange={(event) => setAgentKey(event.target.value.trim())}
          spellCheck={false}
        />
      </label>
      <div className="rounded-xl border border-line bg-black/20 px-3 py-2 font-mono text-xs text-slate-400">
        <div>balance {balance.data !== undefined ? `${formatTusd(balance.data)} tUSD8` : '—'}</div>
        <div>
          allowance {allowance.data !== undefined ? `${formatTusd(allowance.data)} tUSD8` : '—'}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <button
          type="button"
          disabled={!ready || isPending}
          onClick={approve}
          className="rounded-xl border border-line px-3 py-2 text-sm disabled:opacity-40"
        >
          Approve 24h cap
        </button>
        <button
          type="button"
          disabled={!ready || isPending}
          onClick={authorize}
          className="rounded-xl bg-signal px-3 py-2 text-sm font-medium text-ink disabled:opacity-40"
        >
          Sign & authorize
        </button>
      </div>
      {!escrowAddress && (
        <p className="text-xs text-worker">Set NEXT_PUBLIC_ESCROW_ADDRESS after deployment.</p>
      )}
    </section>
  );
}
