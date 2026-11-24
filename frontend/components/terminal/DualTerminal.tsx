'use client';

import { Copy } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { explorerTx } from '@/config/chains';
import { shortHash, tusd } from '@/lib/format';

export type LedgerRow = {
  kind: string;
  txHash: string;
  amount?: string;
  status: string;
  block?: number;
};

export function DualTerminal({ logs, ledger, live = false }: { logs: string[]; ledger: LedgerRow[]; live?: boolean }) {
  const stream = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState<'stream' | 'ledger' | null>(null);

  useEffect(() => {
    const node = stream.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
  }, [logs]);

  async function copy(target: 'stream' | 'ledger') {
    const text =
      target === 'stream'
        ? logs.join('\n')
        : ledger.map((row) => [row.kind, row.block ?? '', row.txHash, row.amount ?? '', row.status].join('\t')).join('\n');
    await navigator.clipboard.writeText(text);
    setCopied(target);
    window.setTimeout(() => setCopied(null), 1200);
  }

  return (
    <div className="grid gap-4">
      <section className="term rounded-2xl border border-line p-4">
        <header className="mb-3 flex items-center justify-between text-sm text-slate-300">
          <span>Activity {live ? '· live' : '· reconnecting'}</span>
          <button type="button" onClick={() => copy('stream')} className="inline-flex items-center gap-1 text-slate-400">
            <Copy size={12} /> {copied === 'stream' ? 'Copied' : 'Copy'}
          </button>
        </header>
        <pre ref={stream} aria-live="polite" className="h-40 overflow-auto font-mono text-xs leading-5 text-slate-300">
          {logs.length ? logs.join('\n') : 'No tasks yet. A dispatch shows the 402 challenge here.'}
        </pre>
      </section>
      <section className="term rounded-2xl border border-line p-4">
        <header className="mb-3 flex items-center justify-between text-sm text-slate-300">
          <span>Explorer links</span>
          <button type="button" onClick={() => copy('ledger')} className="inline-flex items-center gap-1 text-slate-400">
            <Copy size={12} /> {copied === 'ledger' ? 'Copied' : 'Copy'}
          </button>
        </header>
        <div className="h-40 overflow-auto">
          <table className="w-full text-left font-mono text-xs">
            <thead className="text-slate-500">
              <tr>
                <th className="pb-2">Action</th>
                <th>Block</th>
                <th>Tx</th>
                <th>Amount</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {ledger.length === 0 ? (
                <tr>
                  <td className="py-3 text-slate-500" colSpan={5}>
                    Lock and claim hashes show up here after a settled task.
                  </td>
                </tr>
              ) : null}
              {ledger.map((row) => (
                <tr key={`${row.kind}-${row.txHash}`} className="border-t border-white/5">
                  <td className="py-2 pr-2">{row.kind}</td>
                  <td>{row.block || '—'}</td>
                  <td>
                    <a className="text-cyan underline-offset-2 hover:underline" href={explorerTx(row.txHash)} target="_blank" rel="noreferrer">
                      {shortHash(row.txHash)}
                    </a>
                  </td>
                  <td>{row.amount ? tusd(row.amount) : '—'}</td>
                  <td>{row.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
