'use client';

import { useMemo, useState } from 'react';
import { explorerTx } from '@/config/chains';
import { API_URL } from '@/lib/contracts';

const FALLBACK_SERVICES = ['web_brief', 'ai_synthesis', 'chain_audit', 'deep_due_diligence'];
const SCHEMES = ['exact', 'upto', 'batch-settlement', 'auth-capture'] as const;

type PlaygroundJob = {
  id?: string;
  prompt?: string;
  status?: string;
  result?: string;
  error?: string;
  task_id?: string;
  lock_tx_hash?: string;
  claim_tx_hash?: string;
  refund_tx_hash?: string;
  payment_required?: string;
  payment_signature?: string;
  payment_response?: string;
};

export function ProtocolPlayground({ jobs, services }: { jobs: PlaygroundJob[]; services: string[] }) {
  const options = services.length ? services : FALLBACK_SERVICES;
  const [service, setService] = useState(options[0] || 'ai_synthesis');
  const [scheme, setScheme] = useState<(typeof SCHEMES)[number]>('exact');
  const [prompt, setPrompt] = useState('Summarize why a capped session key is safer than an unlimited approval.');
  const [evaluator, setEvaluator] = useState(false);
  const [notice, setNotice] = useState('');
  const [activeId, setActiveId] = useState('');
  const [wire, setWire] = useState<'PAYMENT-REQUIRED' | 'PAYMENT-SIGNATURE' | 'PAYMENT-RESPONSE'>('PAYMENT-REQUIRED');

  const job = useMemo(() => jobs.find((item) => item.id === activeId) || null, [jobs, activeId]);

  async function run() {
    setNotice('Sending custom x402 request...');
    try {
      const response = await fetch(`${API_URL}/api/jobs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          prompt,
          service,
          task_type: service,
          scheme: evaluator ? 'auth-capture' : scheme,
          evaluator,
        }),
      });
      const body = await response.json();
      if (!response.ok) {
        setNotice(body.error || 'Request was not accepted');
        return;
      }
      setActiveId(body.id || '');
      setNotice(`Accepted ${body.id || 'job'}. Wire headers appear as the orchestrator reports them.`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Orchestrator unreachable');
    }
  }

  const encoded = job ? { 'PAYMENT-REQUIRED': job.payment_required, 'PAYMENT-SIGNATURE': job.payment_signature, 'PAYMENT-RESPONSE': job.payment_response }[wire] : '';

  return (
    <section className="panel space-y-3 rounded-2xl p-5" aria-label="Interactive x402 Playground">
      <div>
        <p className="text-xs uppercase tracking-[0.16em] text-slate-500">Interactive x402 Playground</p>
        <h2 className="text-lg font-medium">Compose a live request</h2>
      </div>
      <label className="block text-sm">
        Service
        <select value={service} onChange={(event) => setService(event.target.value)} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm">
          {options.map((name) => (
            <option key={name} value={name}>{name}</option>
          ))}
        </select>
      </label>
      <label className="block text-sm">
        Payment scheme
        <select value={scheme} onChange={(event) => setScheme(event.target.value as (typeof SCHEMES)[number])} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm">
          {SCHEMES.map((name) => (
            <option key={name} value={name}>{name}</option>
          ))}
        </select>
      </label>
      <label className="block text-sm">
        Prompt, target URL, or Aevum address
        <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={3} className="mt-1 w-full rounded-lg border border-line bg-black/30 px-3 py-2 text-sm" />
      </label>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={evaluator} onChange={(event) => setEvaluator(event.target.checked)} />
        Evaluator gate (ERC-8183)
      </label>
      <button type="button" onClick={run} className="rounded-lg bg-emerald px-3 py-2 text-sm font-medium text-canvas">Execute live request</button>
      <p className="text-sm text-slate-300">{notice}</p>
      <div>
        <div className="flex flex-wrap gap-2 text-xs">
          {(['PAYMENT-REQUIRED', 'PAYMENT-SIGNATURE', 'PAYMENT-RESPONSE'] as const).map((name) => (
            <button key={name} type="button" onClick={() => setWire(name)} className={`rounded-full border px-2 py-1 ${wire === name ? 'border-cyan text-cyan' : 'border-line text-slate-400'}`}>{name}</button>
          ))}
        </div>
        <pre className="mt-2 max-h-40 overflow-auto rounded-lg bg-black/40 p-3 font-mono text-xs text-slate-300">{encoded ? decodeWire(encoded) : 'No wire header yet.'}</pre>
      </div>
      {job?.result ? <p className="text-sm">{job.result}</p> : null}
      {job?.error ? <p className="text-sm text-panic">{job.error}</p> : null}
      <div className="space-y-1 text-sm">
        {job?.lock_tx_hash ? <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(job.lock_tx_hash)} target="_blank" rel="noreferrer">Lock transaction</a> : null}
        {job?.claim_tx_hash ? <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(job.claim_tx_hash)} target="_blank" rel="noreferrer">Settlement transaction</a> : null}
        {job?.refund_tx_hash ? <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(job.refund_tx_hash)} target="_blank" rel="noreferrer">Refund transaction</a> : null}
      </div>
    </section>
  );
}

function decodeWire(value: string) {
  try {
    const parsed = JSON.parse(atob(value));
    return JSON.stringify(parsed, null, 2);
  } catch {
    return value;
  }
}
