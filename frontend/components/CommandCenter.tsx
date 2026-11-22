'use client';

import { ConnectButton } from '@rainbow-me/rainbowkit';
import { Ban, Radio, ShieldAlert } from 'lucide-react';
import dynamic from 'next/dynamic';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useAccount, useChainId, usePublicClient, useSwitchChain, useWatchContractEvent, useWriteContract } from 'wagmi';
import { isAddress, type Hex } from 'viem';
import { nexusEscrowAbi } from '@/lib/abi';
import { aevum, escrowAddress, orchestratorUrl, TUSD8 } from '@/lib/chain';
import { errorText } from '@/lib/format';
import { SessionPanel } from './SessionPanel';
import { Terminals, type LogLine } from './Terminals';

const Topology = dynamic(() => import('./Topology').then((mod) => mod.Topology), { ssr: false });

type Activity = { stream: 'http' | 'chain'; text: string; tx?: string };

export function CommandCenter() {
  const [agentKey, setAgentKey] = useState('');
  const [hourly, setHourly] = useState(10);
  const [perTask, setPerTask] = useState('1');
  const [hours, setHours] = useState(24);
  const [httpLogs, setHttpLogs] = useState<LogLine[]>([]);
  const [chainLogs, setChainLogs] = useState<LogLine[]>([]);
  const [pulse, setPulse] = useState(0);
  const [payload, setPayload] = useState('{"job":"keccak256","payload":"nexus402"}');
  const [busy, setBusy] = useState(false);
  const [armed, setArmed] = useState(false);
  const idRef = useRef(1);
  const seenRef = useRef(0);

  const push = useCallback((activity: Activity) => {
    const line: LogLine = {
      id: idRef.current++,
      time: new Date().toLocaleTimeString('en-GB', { hour12: false }),
      text: activity.text,
      tx: activity.tx,
    };
    const setter = activity.stream === 'chain' ? setChainLogs : setHttpLogs;
    setter((current) => [...current.slice(-160), line]);
  }, []);

  useEffect(() => {
    const timer = window.setInterval(async () => {
      try {
        const response = await fetch(`${orchestratorUrl}/api/v1/logs?since=${seenRef.current}`);
        if (!response.ok) return;
        const body = (await response.json()) as {
          lines: Array<{ id: number; stream: 'http' | 'chain'; text: string; tx?: string }>;
        };
        for (const line of body.lines) {
          seenRef.current = Math.max(seenRef.current, line.id);
          push({ stream: line.stream === 'chain' ? 'chain' : 'http', text: line.text, tx: line.tx || undefined });
          if (line.stream === 'chain' && line.tx) setPulse((value) => value + 1);
        }
      } catch {
        // Orchestrator is optional until the agent process is running.
      }
    }, 3000);
    return () => window.clearInterval(timer);
  }, [push]);

  async function dispatchTask() {
    setBusy(true);
    push({ stream: 'http', text: `POST ${orchestratorUrl}/api/v1/tasks` });
    try {
      const response = await fetch(`${orchestratorUrl}/api/v1/tasks`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: payload,
      });
      const text = await response.text();
      if (!response.ok) {
        push({ stream: 'http', text: `dispatch ${response.status}: ${text.slice(0, 280)}` });
        return;
      }
      push({ stream: 'http', text: 'worker returned the compute payload' });
      setPulse((value) => value + 1);
    } catch (error) {
      push({ stream: 'http', text: errorText(error) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-[1440px] flex-col gap-4 px-4 py-4 md:px-6">
      <header className="panel flex flex-wrap items-center justify-between gap-3 rounded-2xl px-4 py-3">
        <div className="flex items-center gap-3">
          <span className="grid h-10 w-10 place-items-center rounded-xl bg-signal/15 text-signal">
            <Radio size={18} />
          </span>
          <div>
            <p className="text-xs uppercase tracking-[0.22em] text-signal">Nexus402</p>
            <h1 className="text-xl font-medium leading-none">Command Center</h1>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3 text-xs text-slate-400">
          <span>Aevum {aevum.id}</span>
          <span className="font-mono">tUSD8 {TUSD8}</span>
          <ConnectButton />
        </div>
      </header>

      <div className="grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)_300px]">
        <SessionPanel
          agentKey={agentKey}
          setAgentKey={setAgentKey}
          hourly={hourly}
          setHourly={setHourly}
          perTask={perTask}
          setPerTask={setPerTask}
          hours={hours}
          setHours={setHours}
          onActivity={push}
          onPulse={() => setPulse((value) => value + 1)}
        />

        <section className="panel relative min-h-[420px] overflow-hidden rounded-2xl">
          <Topology pulse={pulse} />
          <div className="pointer-events-none absolute inset-x-0 top-4 flex justify-between px-6 text-xs uppercase tracking-[0.16em]">
            <span className="text-orch">Orchestrator</span>
            <span className="text-worker">Worker</span>
          </div>
          <div className="pointer-events-none absolute inset-x-0 bottom-4 text-center text-xs uppercase tracking-[0.16em] text-signal">
            Aevum escrow
          </div>
        </section>

        <aside className="flex flex-col gap-4">
          <PanicButton agentKey={agentKey} onActivity={push} onPulse={() => setPulse((value) => value + 1)} armed={armed} setArmed={setArmed} />
          <section className="panel flex flex-1 flex-col gap-3 rounded-2xl p-4">
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-worker">Dispatch</p>
              <h2 className="mt-1 text-lg font-medium">Send a paid compute task</h2>
            </div>
            <textarea
              className="min-h-28 flex-1 rounded-xl border border-line bg-ink p-3 font-mono text-xs outline-none focus:border-worker"
              value={payload}
              onChange={(event) => setPayload(event.target.value)}
            />
            <button
              type="button"
              disabled={busy}
              onClick={dispatchTask}
              className="rounded-xl bg-worker px-3 py-2 text-sm font-medium text-ink disabled:opacity-40"
            >
              {busy ? 'Negotiating…' : 'Dispatch to worker'}
            </button>
            <button
              type="button"
              onClick={() => {
                push({ stream: 'http', text: 'local visual replay — no settlement' });
                setPulse((value) => value + 1);
              }}
              className="rounded-xl border border-line px-3 py-2 text-sm"
            >
              Replay stream
            </button>
          </section>
        </aside>
      </div>

      {escrowAddress && <ChainWatcher onActivity={push} />}
      <Terminals httpLogs={httpLogs} chainLogs={chainLogs} />
    </main>
  );
}

function ChainWatcher({ onActivity }: { onActivity: (activity: Activity) => void }) {
  const onActivityRef = useRef(onActivity);
  onActivityRef.current = onActivity;
  useWatchContractEvent({
    address: escrowAddress,
    abi: nexusEscrowAbi,
    onLogs(logs) {
      for (const log of logs) {
        onActivityRef.current({
          stream: 'chain',
          text: `event ${log.eventName ?? 'log'}`,
          tx: log.transactionHash,
        });
      }
    },
  });
  return null;
}

function PanicButton({
  agentKey,
  onActivity,
  onPulse,
  armed,
  setArmed,
}: {
  agentKey: string;
  onActivity: (activity: Activity) => void;
  onPulse: () => void;
  armed: boolean;
  setArmed: (value: boolean) => void;
}) {
  const { isConnected } = useAccount();
  const chainId = useChainId();
  const { switchChainAsync } = useSwitchChain();
  const publicClient = usePublicClient();
  const { writeContractAsync, isPending } = useWriteContract();

  useEffect(() => {
    if (!armed) return;
    const timer = window.setTimeout(() => setArmed(false), 4000);
    return () => window.clearTimeout(timer);
  }, [armed, setArmed]);

  async function revoke() {
    if (!armed) {
      setArmed(true);
      onActivity({ stream: 'http', text: 'panic armed — press again to freeze the session key' });
      return;
    }
    setArmed(false);
    if (!escrowAddress || !isAddress(agentKey) || !publicClient) {
      onActivity({ stream: 'http', text: 'panic needs a connected wallet, escrow, and agent address' });
      return;
    }
    try {
      if (chainId !== aevum.id) await switchChainAsync({ chainId: aevum.id });
      onActivity({ stream: 'http', text: 'submitting emergencyRevokeSession' });
      const hash = await writeContractAsync({
        address: escrowAddress,
        abi: nexusEscrowAbi,
        functionName: 'emergencyRevokeSession',
        args: [agentKey as Hex],
        chainId: aevum.id,
      });
      onActivity({ stream: 'chain', text: 'emergency revoke submitted', tx: hash });
      await publicClient.waitForTransactionReceipt({ hash });
      onActivity({ stream: 'chain', text: 'agent key frozen', tx: hash });
      onPulse();
    } catch (error) {
      onActivity({ stream: 'http', text: errorText(error) });
    }
  }

  return (
    <button
      type="button"
      onClick={revoke}
      disabled={!isConnected || isPending}
      className={`flex items-center justify-center gap-2 rounded-2xl bg-danger px-4 py-5 text-left text-white disabled:opacity-40 ${armed ? 'armed' : ''}`}
    >
      {armed ? <ShieldAlert size={18} /> : <Ban size={18} />}
      <span>
        <span className="block text-xs uppercase tracking-[0.16em] text-white/80">Circuit breaker</span>
        <span className="text-base font-medium">{armed ? 'Confirm freeze' : 'Emergency revoke'}</span>
      </span>
    </button>
  );
}
