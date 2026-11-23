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
  agent_address?: string;
  hourly_spent?: string | number;
  hourly_limit?: string | number;
  connected?: boolean;
};

type OrchestratorJob = {
  id: string;
  prompt?: string;
  status?: string;
  amount?: string | number;
  task_hash?: string;
  task_id?: string;
  lock_tx_hash?: string;
  claim_tx_hash?: string;
  error?: string;
};

function asUnits(value: unknown): bigint {
  if (typeof value === 'string' && value.length > 0) return BigInt(value);
  if (typeof value === 'number' && Number.isFinite(value)) return BigInt(Math.trunc(value));
  return 0n;
}

export default function HomePage() {
  const { isConnected } = useAccount();
  const [agentKey, setAgentKey] = useState('');
  const [killed, setKilled] = useState(false);
  const [prompt, setPrompt] = useState('Calculate compute hash');
  const [taskType, setTaskType] = useState('hash');
  const [notice, setNotice] = useState('');
  const [telemetry, setTelemetry] = useState<Telemetry>({});
  const [jobs, setJobs] = useState<OrchestratorJob[]>([]);
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
        const jobsResponse = await fetch(`${API_URL}/api/jobs`);
        const jobsBody = jobsResponse.ok ? ((await jobsResponse.json()) as OrchestratorJob[]) : [];
        if (stop) return;
        setTelemetry({ ...body, connected: true });
        setJobs(Array.isArray(jobsBody) ? jobsBody : []);
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

  const logs = useMemo(() => jobs.flatMap((job) => handshakeLines(job)), [jobs]);

  const ledger = useMemo<LedgerRow[]>(() => {
    const remote = jobs.flatMap((job) => {
      const amount = job.amount === undefined ? undefined : String(job.amount);
      const rows: LedgerRow[] = [];
      if (job.lock_tx_hash) {
        rows.push({ kind: 'Escrow Lock', txHash: job.lock_tx_hash, amount, status: job.status || 'Locked' });
      }
      if (job.claim_tx_hash) {
        rows.push({ kind: 'Proof Claim', txHash: job.claim_tx_hash, amount, status: 'Confirmed' });
      }
      return rows;
    });
    return [...localLedger, ...remote];
  }, [localLedger, jobs]);

  const settledCount = jobs.filter((job) => job.status === 'SETTLED').length;
  useEffect(() => {
    if (settledCount > 0) setPulse(settledCount);
  }, [settledCount]);

  async function dispatch(event: FormEvent) {
    event.preventDefault();
    if (killed) {
      setNotice('Session revoked. New tasks are blocked until a new session is registered.');
      return;
    }
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

  const remaining = asUnits(telemetry.hourly_limit) - asUnits(telemetry.hourly_spent);

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
        <Metric label="Active agent key" value={telemetry.agent_address || agentKey || '—'} mono />
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
            <button disabled={killed} className="rounded-lg bg-white/10 px-4 py-2 text-sm disabled:opacity-40">
              Dispatch task
            </button>
            <p className="w-full text-xs text-slate-400">{notice}</p>
          </form>
        </div>
        <DualTerminal logs={logs} ledger={ledger} />
      </section>
    </main>
  );
}

function handshakeLines(job: OrchestratorJob): string[] {
  const lines = [`[REQ] ${job.prompt || job.id}`];
  if (job.task_hash || job.status !== 'PENDING') {
    lines.push(`[402 CHALLENGE] amount=${job.amount ?? '—'} hash=${job.task_hash || '—'}`);
  }
  if (job.lock_tx_hash) lines.push(`[ESCROW LOCK] ${job.lock_tx_hash}`);
  if (job.status === 'SETTLED') lines.push(`[200 OK SETTLED] ${job.claim_tx_hash || job.task_id || ''}`);
  if (job.status === 'FAILED') lines.push(`[FAIL] ${job.error || 'failed'}`);
  return lines;
}

function Metric({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <article className="panel rounded-2xl p-4">
      <p className="text-xs uppercase tracking-[0.16em] text-slate-500">{label}</p>
      <p className={mono ? 'mt-2 truncate font-mono text-sm text-signal' : 'mt-2 text-xl'}>{value}</p>
    </article>
  );
}
