'use client';

import dynamic from 'next/dynamic';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { useAccount, useReadContract } from 'wagmi';
import { EmergencyPanic } from '@/components/controls/EmergencyPanic';
import { SessionDelegator } from '@/components/controls/SessionDelegator';
import { DualTerminal, type LedgerRow } from '@/components/terminal/DualTerminal';
import { API_URL, erc20Abi, NEXUS_ESCROW_ADDRESS, TUSD8_ADDRESS } from '@/lib/contracts';
import { tusd } from '@/lib/format';
import type { Hex } from 'viem';

const MeshCanvas = dynamic(() => import('@/components/3d/MeshCanvas').then((mod) => mod.MeshCanvas), {
  ssr: false,
});

type Telemetry = {
  spent_in_window?: string;
  hourly_limit?: string;
  agent?: string;
  connected?: boolean;
  logs?: string[];
  ledger?: { kind: string; tx_hash: string; amount?: string; status: string; block?: number }[];
};

export default function HomePage() {
  const { isConnected } = useAccount();
  const [agentKey, setAgentKey] = useState('');
  const [killed, setKilled] = useState(false);
  const [prompt, setPrompt] = useState('Calculate compute hash');
  const [taskType, setTaskType] = useState('hash');
  const [notice, setNotice] = useState('');
  const [telemetry, setTelemetry] = useState<Telemetry>({});
  const [localLedger, setLocalLedger] = useState<LedgerRow[]>([]);
  const [pulse, setPulse] = useState(0);

  const escrowBalance = useReadContract({
    address: TUSD8_ADDRESS,
    abi: erc20Abi,
    functionName: 'balanceOf',
    args: NEXUS_ESCROW_ADDRESS ? [NEXUS_ESCROW_ADDRESS] : undefined,
    query: { enabled: Boolean(NEXUS_ESCROW_ADDRESS), refetchInterval: 8000 },
  });

  useEffect(() => {
    let stop = false;
    async function poll() {
      try {
        const response = await fetch(`${API_URL}/api/telemetry`);
        if (!response.ok) return;
        const body = (await response.json()) as Telemetry;
        if (stop) return;
        setTelemetry(body);
        const settled = (body.logs || []).some((line) => line.includes('200 OK'));
        if (settled) setPulse((value) => (value === 0 ? 1 : value));
      } catch {
        if (!stop) setTelemetry((current) => ({ ...current, connected: false }));
      }
    }
    poll();
    const timer = setInterval(poll, 2000);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, []);

  const ledger = useMemo<LedgerRow[]>(() => {
    const remote = (telemetry.ledger || []).map((row) => ({
      kind: row.kind,
      txHash: row.tx_hash,
      amount: row.amount,
      status: row.status,
      block: row.block,
    }));
    return [...localLedger, ...remote];
  }, [localLedger, telemetry.ledger]);

  const settledCount = (telemetry.logs || []).filter((line) => line.includes('200 OK')).length;
  useEffect(() => {
    if (settledCount > 0) setPulse(settledCount);
  }, [settledCount]);

  async function dispatch(event: FormEvent) {
    event.preventDefault();
    setNotice('Dispatching to the orchestrator...');
    try {
      const response = await fetch(`${API_URL}/api/jobs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt, task_type: taskType }),
      });
      const body = await response.json();
      setNotice(response.ok ? `Queued ${body.id}` : body.error || 'Dispatch failed');
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Orchestrator unreachable');
    }
  }

  function remember(kind: string, hash: Hex) {
    setLocalLedger((rows) => [{ kind, txHash: hash, status: 'Submitted' }, ...rows]);
    if (kind === 'Session Revoke') setKilled(true);
    if (kind === 'Session Grant') setKilled(false);
  }

  const spent = telemetry.spent_in_window || '0';
  const limit = telemetry.hourly_limit || '0';
  const remaining = BigInt(limit || '0') - BigInt(spent || '0');

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-6">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="font-mono text-xs tracking-[0.35em] text-signal">NEXUS402</p>
          <h1 className="text-2xl font-medium">Command center</h1>
        </div>
        <div className="flex items-center gap-3">
          <span className="rounded-full border border-line px-3 py-1 font-mono text-xs text-slate-300">
            QDay Aevum Testnet #{isConnected ? '44005' : '44005'}
          </span>
          <ConnectButton />
        </div>
      </header>

      <section className="grid gap-3 md:grid-cols-3">
        <Metric label="Remaining hourly allowance" value={`${tusd(remaining > 0n ? remaining : 0n)} tUSD8`} />
        <Metric label="Active agent key" value={telemetry.agent || agentKey || '—'} mono />
        <Metric label="Escrow contract TVL" value={`${tusd(escrowBalance.data)} tUSD8`} />
      </section>

      <section className="panel relative h-[460px] overflow-hidden rounded-3xl">
        <MeshCanvas pulse={pulse} />
        <div className="pointer-events-none absolute left-4 top-4 space-y-1 text-xs">
          <p className="text-emerald-300">Orchestrator Agent (Go)</p>
          <p className="text-cyan-300">Worker Compute Node (Rust)</p>
          <p className="text-worker">Aevum Contract (NexusEscrow)</p>
          <p className="text-slate-400">{telemetry.connected ? 'RPC connected' : 'RPC idle'}</p>
        </div>
      </section>

      <section className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-4">
          <SessionDelegator
            agentKey={agentKey}
            onAgentKey={setAgentKey}
            onSession={(hash) => remember('Session Grant', hash)}
          />
          <EmergencyPanic
            agentKey={agentKey}
            killed={killed}
            onKilled={(hash) => remember('Session Revoke', hash)}
          />
          <form onSubmit={dispatch} className="panel flex flex-wrap items-end gap-3 rounded-2xl p-4">
            <label className="min-w-48 flex-1 text-sm">
              Prompt
              <input
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm"
              />
            </label>
            <label className="w-32 text-sm">
              Task
              <input
                value={taskType}
                onChange={(event) => setTaskType(event.target.value)}
                className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm"
              />
            </label>
            <button className="rounded-lg bg-white/10 px-4 py-2 text-sm">Dispatch task</button>
            <p className="w-full text-xs text-slate-400">{notice}</p>
          </form>
        </div>
        <DualTerminal logs={telemetry.logs || []} ledger={ledger} />
      </section>
    </main>
  );
}

function Metric({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <article className="panel rounded-2xl p-4">
      <p className="text-xs uppercase tracking-[0.16em] text-slate-500">{label}</p>
      <p className={mono ? 'mt-2 truncate font-mono text-sm text-signal' : 'mt-2 text-xl'}>{value}</p>
    </article>
  );
}
