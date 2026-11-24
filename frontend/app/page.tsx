'use client';

import dynamic from 'next/dynamic';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { isAddress, isHex, type Address, type Hex } from 'viem';
import { useAccount, useReadContract, useSwitchChain, useWriteContract } from 'wagmi';
import { aevumTestnet } from '@/config/chains';
import { MeshBoundary } from '@/components/3d/MeshBoundary';
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
  const { address, isConnected, chainId } = useAccount();
  const { switchChain, isPending: switchingChain } = useSwitchChain();
  const refundWrite = useWriteContract();
  const [agentKey, setAgentKey] = useState('');
  const [killed, setKilled] = useState(false);
  const [prompt, setPrompt] = useState('Calculate compute hash');
  const [taskType, setTaskType] = useState('hash');
  const [notice, setNotice] = useState('');
  const [audit, setAudit] = useState<string[]>([]);
  const [telemetry, setTelemetry] = useState<Telemetry>({});
  const [jobs, setJobs] = useState<OrchestratorJob[]>([]);
  const [burst, setBurst] = useState(0);
  const [live, setLive] = useState(false);
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
    let source: EventSource | null = null;
    let timer: number | undefined;
    let stop = false;

    function connect() {
      source = new EventSource(`${API_URL}/api/events`);
      source.onopen = () => {
        if (!stop) setLive(true);
      };
      source.onmessage = (event) => {
        let job: OrchestratorJob;
        try {
          job = JSON.parse(event.data) as OrchestratorJob;
        } catch {
          return;
        }
        if (!job.id) return;
        setJobs((current) => {
          const index = current.findIndex((item) => item.id === job.id);
          if (index === -1) return [...current, job];
          const next = current.slice();
          next[index] = { ...current[index], ...job };
          return next;
        });
        if (job.status === 'ESCROW_LOCKED' || job.status === 'SETTLED') {
          setBurst((count) => count + 1);
        }
      };
      source.onerror = () => {
        source?.close();
        if (!stop) {
          setLive(false);
          timer = window.setTimeout(connect, 2000);
        }
      };
    }

    connect();
    return () => {
      stop = true;
      if (timer) window.clearTimeout(timer);
      source?.close();
    };
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
  const refundable = jobs.find((job) => job.task_id && isHex(job.task_id) && job.status !== 'SETTLED');
  const expiresAt = session.data ? Number(session.data[3]) : 0;
  const ttl = expiresAt > now ? formatTtl(expiresAt - now) : '—';
  const spent = asUnits(telemetry.hourly_spent);
  const limit = asUnits(telemetry.hourly_limit);
  const sessionActive = Boolean(session.data?.[6]);
  const nextStep = !isConnected
    ? 'Connect the principal wallet. It must be on QDay Aevum, chain 44005.'
    : !NEXUS_ESCROW_ADDRESS
      ? 'The escrow address is not set, so approve, register, revoke, and refund stay off.'
      : !agent
        ? 'Paste the orchestrator public key. That is the session key, not the wallet you just connected.'
        : killed
          ? 'SESSION_REVOKED. Register a new session before dispatching.'
          : !sessionActive
            ? 'Approve tUSD8 if asked, then sign the session. Dispatch stays a local request until that session exists on-chain.'
            : telemetry.connected
              ? 'Session is active. Send a task. The worker price is 0.25 tUSD8.'
              : `Session is active, but nothing is answering at ${API_URL}. Start the orchestrator before dispatch.`;

  async function dispatch(event: FormEvent) {
    event.preventDefault();
    if (killed) {
      setNotice('SESSION_REVOKED. New tasks are blocked until a new session is registered.');
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

  function refundEscrow() {
    if (!refundable?.task_id || !isHex(refundable.task_id) || !NEXUS_ESCROW_ADDRESS) return;
    refundWrite.writeContract(
      {
        address: NEXUS_ESCROW_ADDRESS,
        abi: nexusEscrowAbi,
        functionName: 'refundExpiredTask',
        args: [refundable.task_id],
      },
      {
        onSuccess: (hash) => {
          remember('Refund', hash);
          setNotice('Refund submitted. The contract reverts with DeadlineNotPassed until the deadline block.');
        },
      },
    );
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
    <main className="mx-auto flex min-h-screen max-w-6xl flex-col gap-5 px-4 py-6">
      <header className="flex flex-wrap items-end justify-between gap-4 border-b border-line pb-4">
        <div>
          <p className="text-sm text-slate-400">Nexus402</p>
          <h1 className="text-2xl font-medium">Pay a worker from a capped session</h1>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-sm text-slate-400">Aevum testnet · 44005</span>
          <ConnectButton />
        </div>
      </header>

      {isConnected && chainId !== aevumTestnet.id ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber/40 bg-card px-4 py-3" role="alert">
          <p className="text-sm">This wallet is not on QDay Aevum Testnet (44005).</p>
          <button
            type="button"
            className="rounded-lg bg-emerald px-3 py-2 text-sm font-medium text-black"
            disabled={switchingChain}
            onClick={() => switchChain({ chainId: aevumTestnet.id })}
          >
            {switchingChain ? 'Switching' : 'Switch to Aevum Testnet'}
          </button>
        </div>
      ) : null}

      <p className="rounded-xl border border-line bg-card px-4 py-3 text-sm leading-6">{nextStep}</p>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label="tUSD8 balance" value={isConnected ? `${tusd(principalBalance.data)}` : 'Connect wallet'} />
        <Metric label="Hourly spend" value={telemetry.connected ? `${tusd(spent)} / ${tusd(limit)}` : 'Orchestrator offline'} />
        <Metric label="Session time left" value={session.isLoading ? 'Loading session' : ttl} mono />
        <Metric label="Settled tasks" value={String(settledCount)} />
      </section>

      <section className="panel relative h-[420px] overflow-hidden rounded-3xl">
        <MeshBoundary>
          <MeshCanvas pulse={settledCount + burst} />
        </MeshBoundary>
        <div className="pointer-events-none absolute left-4 top-4 space-y-1 font-mono text-xs">
          <p className="text-emerald">Client Orchestrator (Go)</p>
          <p className="text-cyan">Compute Worker (Rust)</p>
          <p className="text-amber">Aevum Escrow (Solidity)</p>
          <p className="text-slate-400">{telemetry.connected ? 'Orchestrator linked' : 'Orchestrator offline'}</p>
        </div>
      </section>

      <section className="grid items-start gap-5 lg:grid-cols-2">
        <div className="space-y-4">
          <SessionDelegator agentKey={agentKey} onAgentKey={setAgentKey} onSession={(hash) => remember('Session Grant', hash)} />
          <form onSubmit={dispatch} className="panel space-y-3 rounded-2xl p-5">
            <div>
              <p className="text-sm text-slate-400">Step 2</p>
              <h2 className="text-lg font-medium">Send a task</h2>
            </div>
            <label className="block text-sm">
              Prompt
              <input value={prompt} onChange={(event) => setPrompt(event.target.value)} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm" />
            </label>
            <label className="block text-sm">
              Task type
              <input value={taskType} onChange={(event) => setTaskType(event.target.value)} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm" />
            </label>
            <div className="flex flex-wrap gap-2">
              <button disabled={killed} className="rounded-lg bg-emerald px-4 py-2 text-sm font-medium text-canvas disabled:opacity-40">
                Dispatch task
              </button>
              <button
                type="button"
                onClick={refundEscrow}
                disabled={!refundable || !NEXUS_ESCROW_ADDRESS || refundWrite.isPending}
                title={refundable ? 'Calls refundExpiredTask. The chain rejects this before the deadline.' : 'Available after a task is locked and not settled.'}
                className="rounded-lg border border-line px-4 py-2 text-sm disabled:opacity-40"
              >
                {refundWrite.isPending ? 'Refunding' : 'Refund expired task'}
              </button>
            </div>
            <p className="text-sm text-slate-300">{notice || 'Refund stays off until a task id exists and is not settled.'}</p>
            {refundWrite.error ? <p className="text-sm text-panic">{refundWrite.error.message}</p> : null}
          </form>
          <EmergencyPanic agentKey={agentKey} killed={killed} onKilled={(hash) => remember('Revocation', hash)} />
        </div>
        <div className="space-y-4">
          <section className="panel rounded-2xl p-5">
            <h2 className="text-lg font-medium">Tasks</h2>
            {jobs.length === 0 ? (
              <p className="mt-3 text-sm text-slate-400">Nothing queued. Tasks from the orchestrator appear here with their status.</p>
            ) : (
              <ul className="mt-3 space-y-2">
                {jobs.slice(0, 8).map((job) => (
                  <li key={job.id} className="rounded-lg border border-line px-3 py-2">
                    <div className="flex items-center justify-between gap-3">
                      <p className="truncate text-sm">{job.prompt || job.id}</p>
                      <span className="shrink-0 font-mono text-xs text-cyan">{job.status || 'UNKNOWN'}</span>
                    </div>
                    {job.error ? <p className="mt-1 text-xs text-panic">{job.error}</p> : null}
                  </li>
                ))}
              </ul>
            )}
          </section>
          <DualTerminal logs={logs} ledger={ledger} live={live} />
        </div>
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
