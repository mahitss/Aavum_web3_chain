'use client';

import dynamic from 'next/dynamic';
import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { isAddress, isHex, type Address, type Hex } from 'viem';
import { useAccount, useReadContract, useSwitchChain, useWriteContract } from 'wagmi';
import { aevumTestnet, explorerTx } from '@/config/chains';
import { MeshBoundary } from '@/components/3d/MeshBoundary';
import { EmergencyPanic } from '@/components/controls/EmergencyPanic';
import { SessionDelegator } from '@/components/controls/SessionDelegator';
import { DualTerminal, type LedgerRow } from '@/components/terminal/DualTerminal';
import { SystemWakeBanner } from '@/components/ui/SystemWakeBanner';
import { API_URL, erc20Abi, NEXUS_ESCROW_ADDRESS, nexusEscrowAbi, TUSD8_ADDRESS } from '@/lib/contracts';
import { shortHash, tusd } from '@/lib/format';

const MeshCanvas = dynamic(() => import('@/components/3d/MeshCanvas').then((mod) => mod.MeshCanvas), {
  ssr: false,
});

type SandboxStatus = {
  sandboxEnabled?: boolean;
  remainingIpQuota?: number;
  principalTusd8Balance?: string;
  agentGasBalance?: string;
  sessionActive?: boolean;
};

type Telemetry = {
  hourly_spent?: string | number;
  hourly_limit?: string | number;
  connected?: boolean;
};

type OrchestratorJob = {
  id: string;
  prompt?: string;
  status?: string;
  service?: string;
  amount?: string | number;
  task_hash?: string;
  result?: string;
  result_hash?: string;
  task_id?: string;
  lock_tx_hash?: string;
  claim_tx_hash?: string;
  refund_tx_hash?: string;
  blocks_remaining?: number;
  event?: string;
  error?: string;
};

type CatalogService = { name: string; price: string; priceUnits?: string; timeoutBlocks?: number };
type CatalogWorker = { url?: string; healthy?: boolean; services?: CatalogService[] };

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
  const [prompt, setPrompt] = useState('Explain why a capped session key is safer than an unlimited token approval.');
  const [service, setService] = useState('ai_synthesis');
  const [notice, setNotice] = useState('');
  const [audit, setAudit] = useState<string[]>([]);
  const [telemetry, setTelemetry] = useState<Telemetry>({});
  const [jobs, setJobs] = useState<OrchestratorJob[]>([]);
  const [burst, setBurst] = useState(0);
  const [live, setLive] = useState(false);
  const [sandbox, setSandbox] = useState<SandboxStatus>({});
  const [catalog, setCatalog] = useState<CatalogWorker[]>([]);
  const [refundBurst, setRefundBurst] = useState(0);
  const [historyReady, setHistoryReady] = useState(false);
  const markHistoryReady = useCallback(() => setHistoryReady(true), []);
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
    if (!historyReady) return;
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
        if (job.status === 'REFUNDED' || job.event === 'TASK_REFUNDED') {
          setRefundBurst((count) => count + 1);
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
  }, [historyReady]);

  useEffect(() => {
    let stop = false;
    async function poll() {
      try {
        const sandboxResponse = await fetch(`${API_URL}/api/sandbox/status`);
        if (sandboxResponse.ok && !stop) setSandbox((await sandboxResponse.json()) as SandboxStatus);
        const response = await fetch(`${API_URL}/api/telemetry`);
        if (!response.ok) return;
        const body = (await response.json()) as Telemetry;
        const servicesResponse = await fetch(`${API_URL}/api/services`);
        if (servicesResponse.ok && !stop) {
          const listed = (await servicesResponse.json()) as { workers?: CatalogWorker[] };
          setCatalog(Array.isArray(listed.workers) ? listed.workers : []);
        }
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
      if (job.refund_tx_hash) rows.push({ kind: 'Auto Refund', txHash: job.refund_tx_hash, amount, status: 'REFUNDED' });
      return rows;
    });
    return [...localLedger, ...remote];
  }, [localLedger, jobs]);

  const settledCount = jobs.filter((job) => job.status === 'SETTLED').length;
  const capEnforced = jobs.some((job) => job.status === 'BLOCKED_HOURLY_CAP');
  const latestOutput = [...jobs].reverse().find((job) => job.result) || null;
  const refundable = jobs.find((job) => job.task_id && isHex(job.task_id) && job.status !== 'SETTLED');
  const expiresAt = session.data ? Number(session.data[3]) : 0;
  const ttl = expiresAt > now ? formatTtl(expiresAt - now) : '—';
  const spent = asUnits(telemetry.hourly_spent);
  const limit = asUnits(telemetry.hourly_limit);
  const spendRatio = capEnforced ? 100 : limit > 0n ? Number((spent * 100n) / limit) : 0;
  const sessionActive = Boolean(session.data?.[6]);
  const nextStep = !isConnected
    ? 'Sandbox mode can run the four judge scenarios without a wallet. Connect MetaMask when you want to sign your own session and use the kill switch.'
    : !NEXUS_ESCROW_ADDRESS
      ? 'The escrow address is not set, so approve, register, revoke, and refund stay off.'
      : !agent
        ? 'Paste the orchestrator public key. That is the session key, not the wallet you just connected.'
        : killed
          ? 'SESSION_REVOKED. Register a new session before dispatching.'
          : !sessionActive
            ? 'Approve tUSD8 if asked, then sign the session. Dispatch stays a local request until that session exists on-chain.'
            : telemetry.connected
              ? 'Session is active. Pick a discovered service and send a task.'
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
        body: JSON.stringify({ prompt, task_type: service, service }),
      });
      const body = await response.json();
      setNotice(response.ok ? `Queued ${body.id}` : body.error || 'Dispatch failed');
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Orchestrator unreachable');
    }
  }

  async function runScenario(scenario: 'happy_path' | 'velocity_attack' | 'kill_switch_test' | 'worker_timeout_refund') {
    setNotice(`Starting ${scenario}...`);
    try {
      const response = await fetch(`${API_URL}/api/scenarios/run`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ scenario }),
      });
      const body = await response.json();
      setNotice(response.ok ? `Scenario ${scenario} accepted` : body.error || 'Scenario failed');
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

      <SystemWakeBanner />
      <LowBalanceBadge sandbox={sandbox} />

      {!isConnected ? (
        <section className="rounded-xl border border-emerald/40 bg-card px-4 py-3 text-sm leading-6">
          <p>Sandbox Mode Active — Trigger real QDay Aevum Testnet settlements immediately without a wallet.</p>
          <p className="mt-2 font-mono text-xs text-slate-400">
            {sandbox.sandboxEnabled ? 'Public sandbox enabled' : 'Public sandbox is off on this server'} · quota {sandbox.remainingIpQuota ?? '—'} · principal tUSD8 {tusd(sandbox.principalTusd8Balance)} · agent gas {sandbox.agentGasBalance ?? '—'} · session {sandbox.sessionActive ? 'active' : 'inactive'}
          </p>
        </section>
      ) : null}

      {isConnected && chainId !== aevumTestnet.id ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber/40 bg-card px-4 py-3" role="alert">
          <p className="text-sm">Switch to QDay Aevum Testnet #44005 before signing or paying.</p>
          <button
            type="button"
            className="rounded-lg bg-emerald px-3 py-2 text-sm font-medium text-black"
            disabled={switchingChain}
            onClick={() => switchChain({ chainId: aevumTestnet.id })}
          >
            {switchingChain ? 'Switching' : 'Switch to QDay Aevum Testnet #44005'}
          </button>
        </div>
      ) : null}

      <p className="rounded-xl border border-line bg-card px-4 py-3 text-sm leading-6">{nextStep}</p>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label="tUSD8 balance" value={isConnected ? `${tusd(principalBalance.data)}` : tusd(sandbox.principalTusd8Balance)} />
        <Metric label="Hourly spend" value={telemetry.connected ? `${tusd(spent)} / ${tusd(limit)}` : 'Orchestrator offline'} />
        <Metric label="Session time left" value={session.isLoading ? 'Loading session' : ttl} mono />
        <Metric label="Settled tasks" value={String(settledCount)} />
      </section>

      <section className="panel relative h-[420px] overflow-hidden rounded-3xl">
        <MeshBoundary>
          <MeshCanvas pulse={settledCount + burst} refundPulse={refundBurst} />
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
          {isConnected ? (
            <SessionDelegator agentKey={agentKey} onAgentKey={setAgentKey} onSession={(hash) => remember('Session Grant', hash)} />
          ) : (
            <section className="panel rounded-2xl p-5 text-sm text-slate-400">
              Connect a wallet for Full Operator Mode. That unlocks the EIP-712 session, the tUSD8 approval, and the emergency kill switch.
            </section>
          )}
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
              Service
              <select value={service} onChange={(event) => setService(event.target.value)} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm">
                <option value="web_brief">web_brief · 0.15 tUSD8</option>
                <option value="ai_synthesis">ai_synthesis · 0.25 tUSD8</option>
                <option value="chain_audit">chain_audit · 0.50 tUSD8</option>
              </select>
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
          {isConnected ? <EmergencyPanic agentKey={agentKey} killed={killed} onKilled={(hash) => remember('Revocation', hash)} /> : null}
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
          <section className="panel space-y-3 rounded-2xl p-5" aria-label="Live demo scenarios">
            <div>
              <p className="text-xs uppercase tracking-[0.16em] text-slate-500">Live demo scenarios</p>
              <h2 className="text-lg font-medium">Run a judge path</h2>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-black/40" aria-label="Hourly velocity cap">
              <div className="h-full bg-emerald" style={{ width: `${Math.min(100, Math.max(0, spendRatio))}%` }} />
            </div>
            <p className="text-xs text-slate-400">Hourly velocity {telemetry.connected ? `${tusd(spent)} / ${tusd(limit)}` : 'waiting for orchestrator'}</p>
            {capEnforced ? (
              <p className="rounded-lg border border-amber/50 bg-amber/10 px-3 py-2 text-sm text-amber" role="alert">
                CAP ENFORCED ON-CHAIN
              </p>
            ) : null}
            <div className="flex flex-col gap-2">
              <button type="button" onClick={() => runScenario('happy_path')} className="rounded-lg bg-emerald px-3 py-2 text-left text-sm font-medium text-canvas">
                Run Paid AI Task (0.25 tUSD8)
              </button>
              <button type="button" onClick={() => runScenario('velocity_attack')} className="rounded-lg border border-amber/50 px-3 py-2 text-left text-sm">
                Simulate Runaway Loop Attack
              </button>
              <button type="button" onClick={() => runScenario('kill_switch_test')} className="rounded-lg border border-panic/50 px-3 py-2 text-left text-sm">
                Test Kill Switch
              </button>
              <button type="button" onClick={() => runScenario('worker_timeout_refund')} className="rounded-lg border border-cyan/50 px-3 py-2 text-left text-sm">
                Simulate Worker Crash & Auto-Refund
              </button>
            </div>
            <p className="text-xs text-slate-500">Revoke the agent key first, then Test Kill Switch. The chain must answer SessionInactive.</p>
          </section>
          <section className="panel rounded-2xl p-5">
            <h2 className="text-lg font-medium">Discovered services</h2>
            {catalog.length === 0 ? (
              <p className="mt-2 text-sm text-slate-400">The orchestrator has not published a worker catalog yet.</p>
            ) : (
              <ul className="mt-3 space-y-2">
                {catalog.flatMap((worker) =>
                  (worker.services || []).map((item) => (
                    <li key={`${worker.url}-${item.name}`}>
                      <button type="button" onClick={() => setService(item.name)} className="flex w-full items-center justify-between rounded-lg border border-line px-3 py-2 text-left text-sm">
                        <span>{item.name}</span>
                        <span className="font-mono text-xs text-slate-400">{item.price} tUSD8 · {worker.healthy ? 'healthy' : 'offline'}</span>
                      </button>
                    </li>
                  )),
                )}
              </ul>
            )}
          </section>
          <OutputInspector job={latestOutput} />
          <DualTerminal logs={logs} ledger={ledger} live={live} onHistoryReady={markHistoryReady} />
        </div>
      </section>
    </main>
  );
}

function LowBalanceBadge({ sandbox }: { sandbox: SandboxStatus }) {
  const notes: string[] = [];
  if (sandbox.principalTusd8Balance !== undefined) {
    try {
      if (BigInt(sandbox.principalTusd8Balance || '0') < 2_000_000n) notes.push('principal tUSD8 is below 2.00');
    } catch {
      notes.push('principal tUSD8 balance is unreadable');
    }
  }
  if (sandbox.agentGasBalance !== undefined) {
    try {
      if (BigInt(sandbox.agentGasBalance || '0') < 50_000_000_000_000_000n) notes.push('agent tQDAY is below 0.05');
    } catch {
      notes.push('agent gas balance is unreadable');
    }
  }
  if (notes.length === 0) return null;
  return (
    <p className="rounded-xl border border-amber/50 bg-amber/10 px-4 py-2 text-sm text-amber" role="status">
      Gas tank low. {notes.join(' and ')} before the next judge run.
    </p>
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
  if (job.status === 'AWAITING_REFUND' || job.status === 'REFUNDED') {
    lines.push('[WORKER FAULT] HTTP 504. completeTaskWithProof was not called. Escrow stays PENDING.');
  }
  if (typeof job.blocks_remaining === 'number' && job.status !== 'REFUNDED' && job.status !== 'SETTLED') {
    lines.push(`[WATCHDOG] ${job.blocks_remaining} blocks remain before refundExpiredTask`);
  }
  if (job.status === 'REFUNDED') lines.push('[TASK_REFUNDED] refundExpiredTask returned the locked tUSD8 to the principal.');
  if (job.status === 'FAILED') lines.push(`[FAIL] ${job.error || 'settlement failed'}`);
  return lines;
}

function OutputInspector({ job }: { job: OrchestratorJob | null }) {
  let pretty = 'No settled output yet. Run the paid AI task to fill this panel.';
  if (job?.result) {
    try {
      pretty = JSON.stringify(JSON.parse(job.result), null, 2);
    } catch {
      pretty = job.result;
    }
  }
  return (
    <section className="panel rounded-2xl p-5">
      <h2 className="text-lg font-medium">Latest verified compute output</h2>
      <pre className="mt-3 max-h-56 overflow-auto whitespace-pre-wrap font-mono text-xs text-slate-300">{pretty}</pre>
      {job?.result_hash ? <p className="mt-3 break-all font-mono text-xs text-slate-400">resultHash {job.result_hash}</p> : null}
      <div className="mt-3 flex flex-wrap gap-3 text-sm">
        {job?.lock_tx_hash ? (
          <a className="text-amber underline-offset-2 hover:underline" href={explorerTx(job.lock_tx_hash)} target="_blank" rel="noreferrer">
            TaskEscrowLocked
          </a>
        ) : null}
        {job?.claim_tx_hash ? (
          <a className="text-emerald underline-offset-2 hover:underline" href={explorerTx(job.claim_tx_hash)} target="_blank" rel="noreferrer">
            TaskCompleted
          </a>
        ) : null}
        {job?.refund_tx_hash ? (
          <a className="text-cyan underline-offset-2 hover:underline" href={explorerTx(job.refund_tx_hash)} target="_blank" rel="noreferrer">
            TaskRefunded
          </a>
        ) : null}
      </div>
    </section>
  );
}

function Metric({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <article className="panel rounded-2xl p-4">
      <p className="text-xs uppercase tracking-[0.16em] text-slate-500">{label}</p>
      <p className={mono ? 'mt-2 truncate font-mono text-sm text-cyan' : 'mt-2 text-xl'}>{value}</p>
    </article>
  );
}
