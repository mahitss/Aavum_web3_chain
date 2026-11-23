'use client';

import dynamic from 'next/dynamic';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { isAddress, type Address, type Hex } from 'viem';
import { useAccount, useReadContract } from 'wagmi';
import { EmergencyPanic } from '@/components/controls/EmergencyPanic';
import { SessionDelegator } from '@/components/controls/SessionDelegator';
import { DualTerminal, type LedgerRow } from '@/components/terminal/DualTerminal';
import { API_URL, erc20Abi, NEXUS_ESCROW_ADDRESS, nexusEscrowAbi, TUSD8_ADDRESS } from '@/lib/contracts';
import { shortHash, tusd } from '@/lib/format';

const MeshCanvas = dynamic(() => import('@/components/3d/MeshCanvas').then((mod) => mod.MeshCanvas), {
  ssr: false,
});

type Telemetry = {
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

function formatTtl(seconds: number) {
  const clamped = Math.max(0, seconds);
  const hours = Math.floor(clamped / 3600);
  const minutes = Math.floor((clamped % 3600) / 60);
  const rest = clamped % 60;
  return [hours, minutes, rest].map((part) => String(part).padStart(2, '0')).join(':');
}

export default function HomePage() {
  const { address, isConnected } = useAccount();
  const [agentKey, setAgentKey] = useState('');
  const [killed, setKilled] = useState(false);
  const [prompt, setPrompt] = useState('Calculate compute hash');
  const [taskType, setTaskType] = useState('hash');
  const [notice, setNotice] = useState('');
  const [audit, setAudit] = useState<string[]>([]);
  const [telemetry, setTelemetry] = useState<Telemetry>({});
  const [jobs, setJobs] = useState<OrchestratorJob[]>([]);
  const [localLedger, setLocalLedger] = useState<LedgerRow[]>([]);
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));

  const agent = isAddress(agentKey) ? (agentKey as Address) : undefined;
  const principalBalance = useReadContract({
    address: TUSD8_ADDRESS,
    abi: erc20Abi,
    functionName: 'balanceOf',
    args: address ? [address] : undefined,
    query: { enabled: Boolean(address), refetchInterval: 8000 },
  });
  const session = useReadContract({
    address: NEXUS_ESCROW_ADDRESS,
    abi: nexusEscrowAbi,
    functionName: 'getSession',
    args: address && agent ? [address, agent] : undefined,
    query: { enabled: Boolean(address && agent && NEXUS_ESCROW_ADDRESS), refetchInterval: 8000 },
  });

  useEffect(() => {
    const timer = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000);
    return () => clearInterval(timer);
  }, []);

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

  const logs = useMemo(() => [...jobs.flatMap((job) => handshakeLines(job)), ...audit], [jobs, audit]);
  const ledger = useMemo<LedgerRow[]>(() => {
    const remote = jobs.flatMap((job) => {
      const amount = job.amount === undefined ? undefined : String(job.amount);
      const rows: LedgerRow[] = [];
      if (job.lock_tx_hash) rows.push({ kind: 'Escrow Lock', txHash: job.lock_tx_hash, amount, status: job.status || 'Locked' });
      if (job.claim_tx_hash) rows.push({ kind: 'Proof Claim', txHash: job.claim_tx_hash, amount, status: 'Confirmed' });
      return rows;
    });
    return [...localLedger, ...remote];
  }, [localLedger, jobs]);

  const settledCount = jobs.filter((job) => job.status === 'SETTLED').length;
  const expiresAt = session.data ? Number(session.data[3]) : 0;
  const ttl = expiresAt > now ? formatTtl(expiresAt - now) : '—';
  const spent = asUnits(telemetry.hourly_spent);
  const limit = asUnits(telemetry.hourly_limit);

  async function dispatch(event: FormEvent) {
    event.preventDefault();
    if (killed) {
      setNotice('SESSION_KILLED. New tasks are blocked until a new session is registered.');
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
    if (kind === 'Revocation') {
      setKilled(true);
      setAudit((lines) => [...lines, `[KILL SWITCH] emergencyRevokeSession ${hash}`]);
    }
    if (kind === 'Session Grant') setKilled(false);
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-6">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="font-mono text-xs tracking-[0.28em] text-emerald">NEXUS402 // AGENTIC ESCROW MESH</p>
          <h1 className="text-2xl font-medium">Command center</h1>
        </div>
        <div className="flex items-center gap-3">
          <span className="rounded-full border border-line px-3 py-1 font-mono text-xs text-slate-300">
            QDay Aevum Testnet (Chain ID: 44005){isConnected ? '' : ''}
          </span>
          <ConnectButton />
        </div>
      </header>

      <section className="grid gap-3 md:grid-cols-4">
        <Metric label="Available principal balance" value={`${tusd(principalBalance.data)} tUSD8`} />
        <Metric label="Rolling hourly spend" value={telemetry.connected ? `${tusd(spent)} / ${tusd(limit)}` : '— / —'} />
        <Metric label="Active session TTL" value={ttl} mono />
        <Metric label="Micro-tasks settled" value={String(settledCount)} />
      </section>

      <section className="panel relative h-[460px] overflow-hidden rounded-3xl">
        <MeshCanvas pulse={settledCount} />
        <div className="pointer-events-none absolute left-4 top-4 space-y-1 font-mono text-xs">
          <p className="text-emerald">Client Orchestrator (Go)</p>
          <p className="text-cyan">Compute Worker (Rust)</p>
          <p className="text-amber">Aevum Escrow (Solidity)</p>
          <p className="text-slate-400">{telemetry.connected ? 'Orchestrator linked' : 'Orchestrator idle'}</p>
          <p className="text-slate-500">{agent ? shortHash(agent) : 'No session key'}</p>
        </div>
      </section>

      <section className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-4">
          <SessionDelegator agentKey={agentKey} onAgentKey={setAgentKey} onSession={(hash) => remember('Session Grant', hash)} />
          <EmergencyPanic agentKey={agentKey} killed={killed} onKilled={(hash) => remember('Revocation', hash)} />
          <form onSubmit={dispatch} className="panel flex flex-wrap items-end gap-3 rounded-2xl p-4">
            <p className="w-full text-xs uppercase tracking-[0.16em] text-slate-500">Test dispatch</p>
            <label className="min-w-48 flex-1 text-sm">
              Prompt
              <input value={prompt} onChange={(event) => setPrompt(event.target.value)} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm" />
            </label>
            <label className="w-32 text-sm">
              Task
              <input value={taskType} onChange={(event) => setTaskType(event.target.value)} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm" />
            </label>
            <button disabled={killed} className="rounded-lg bg-white/10 px-4 py-2 text-sm disabled:opacity-40">
              Dispatch task
            </button>
            <p className="w-full font-mono text-xs text-slate-400">{notice}</p>
          </form>
        </div>
        <DualTerminal logs={logs} ledger={ledger} />
      </section>
    </main>
  );
}

function handshakeLines(job: OrchestratorJob): string[] {
  const amount = job.amount === undefined ? '—' : `${tusd(job.amount)} tUSD8`;
  const hash = job.task_hash ? shortHash(job.task_hash) : '—';
  const lines = ['[OUTBOUND] POST /api/v1/compute -> Challenge required'];
  if (job.status !== 'PENDING') {
    lines.push(`[402 CHALLENGE] Intercepted: ${amount} | TaskHash: ${hash}`);
    lines.push(`[LIMIT CHECK] Validated against session caps: Hourly remaining: ${job.error?.includes('exceeds') ? 'BLOCKED' : 'OK'}`);
  }
  if (job.lock_tx_hash || job.status === 'ESCROW_LOCKED' || job.status === 'SETTLED') {
    lines.push('[ON-CHAIN LOCK] Submitting lockTaskEscrow to Aevum Testnet...');
  }
  if (job.task_id) lines.push(`[INBOUND RETRY] POST /api/v1/compute (X-Escrow-TaskId: ${shortHash(job.task_id)})`);
  if (job.status === 'SETTLED') lines.push('[PROOF VERIFIED] Worker submitted execution proof on-chain. 200 OK.');
  if (job.status === 'FAILED') lines.push(`[FAIL] ${job.error || 'settlement failed'}`);
  return lines;
}

function Metric({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <article className="panel rounded-2xl p-4">
      <p className="text-xs uppercase tracking-[0.16em] text-slate-500">{label}</p>
      <p className={mono ? 'mt-2 truncate font-mono text-sm text-cyan' : 'mt-2 text-xl'}>{value}</p>
    </article>
  );
}
