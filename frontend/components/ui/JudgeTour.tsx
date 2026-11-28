'use client';

import { useEffect, useState } from 'react';

const STORAGE_KEY = 'nexus402.judgeTour.v1';

const STEPS = [
  {
    target: 'sandbox',
    title: 'Zero-wallet sandbox',
    body: 'Sandbox Mode can trigger real QDay Aevum Testnet tUSD8 settlements without MetaMask and without faucet tokens. Connect a wallet only when you want to sign your own session or use the kill switch.',
  },
  {
    target: 'mesh',
    title: '3D agent mesh',
    body: 'Four live nodes: the Go orchestrator, the Rust worker, Aevum escrow, and the evaluator or sub-worker. Green particles are a settlement, amber particles are metered change or a refund, and red particles are an evaluator slash or a revoke.',
  },
  {
    target: 'scenarios',
    title: 'One-click scenarios',
    body: 'The judge bar runs Happy Path, the runaway-loop cap, worker-crash auto-refund, the 10x voucher burst, evaluator slash, and multi-hop agent-to-agent settlement. Each button posts one live scenario.',
  },
  {
    target: 'proof',
    title: 'Cryptographic proof',
    body: 'The ledger and the JCS receipt inspector sit here. Open any settled or refunded row, then follow the transaction hash to https://explorer-test.qday.info.',
  },
] as const;

type Box = { top: number; left: number; width: number; height: number };

export function JudgeTour({ signal }: { signal: number }) {
  const [open, setOpen] = useState(false);
  const [index, setIndex] = useState(0);
  const [box, setBox] = useState<Box | null>(null);

  useEffect(() => {
    try {
      if (!window.localStorage.getItem(STORAGE_KEY)) setOpen(true);
    } catch {
      setOpen(true);
    }
  }, []);

  useEffect(() => {
    if (signal === 0) return;
    setIndex(0);
    setOpen(true);
  }, [signal]);

  useEffect(() => {
    if (!open) {
      setBox(null);
      return;
    }
    const step = STEPS[index];
    let frame = 0;
    const tick = () => {
      const node = document.querySelector(`[data-tour="${step.target}"]`);
      if (node instanceof HTMLElement) {
        const rect = node.getBoundingClientRect();
        setBox({ top: rect.top, left: rect.left, width: rect.width, height: rect.height });
      } else {
        setBox(null);
      }
      frame = window.requestAnimationFrame(tick);
    };
    frame = window.requestAnimationFrame(tick);
    document.querySelector(`[data-tour="${step.target}"]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    return () => window.cancelAnimationFrame(frame);
  }, [open, index]);

  function dismiss() {
    try {
      window.localStorage.setItem(STORAGE_KEY, 'done');
    } catch {
      /* storage can be blocked; the tour still closes */
    }
    setOpen(false);
  }

  if (!open) return null;
  const step = STEPS[index];

  return (
    <div className="pointer-events-none fixed inset-0 z-40">
      {box ? (
        <div
          className="pointer-events-none absolute rounded-2xl border-2 border-emerald shadow-[0_0_0_9999px_rgba(5,7,10,0.55)]"
          style={{ top: box.top - 6, left: box.left - 6, width: box.width + 12, height: box.height + 12 }}
        />
      ) : null}
      <section
        role="region"
        aria-label="Quick tour"
        className="pointer-events-auto absolute bottom-4 left-1/2 w-[min(32rem,calc(100%-2rem))] -translate-x-1/2 rounded-2xl border border-emerald/50 bg-card p-4 shadow-xl"
      >
        <p className="text-xs uppercase tracking-[0.16em] text-slate-500">
          Step {index + 1} of {STEPS.length}
        </p>
        <h2 className="mt-1 text-lg font-medium">{step.title}</h2>
        <p className="mt-2 text-sm leading-6 text-slate-300">{step.body}</p>
        <div className="mt-4 flex flex-wrap gap-2">
          <button type="button" onClick={dismiss} className="rounded-lg border border-line px-3 py-2 text-sm">
            Skip
          </button>
          {index < STEPS.length - 1 ? (
            <button type="button" onClick={() => setIndex((current) => current + 1)} className="rounded-lg bg-emerald px-3 py-2 text-sm font-medium text-canvas">
              Next
            </button>
          ) : (
            <button type="button" onClick={dismiss} className="rounded-lg bg-emerald px-3 py-2 text-sm font-medium text-canvas">
              Done
            </button>
          )}
        </div>
      </section>
    </div>
  );
}
