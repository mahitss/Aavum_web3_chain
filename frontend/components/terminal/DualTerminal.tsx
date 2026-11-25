'use client';

import { Copy } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { explorerTx } from '@/config/chains';
import { API_URL } from '@/lib/contracts';
import { shortHash, tusd } from '@/lib/format';

export type LedgerRow = {
  kind: string;
  txHash: string;
  amount?: string;
  status: string;
  block?: number;
};

type ChainEvent = {
  event?: string;
  blockNumber?: number;
  txHash?: string;
  amount?: string;
  explorerUrl?: string;
};

export function DualTerminal({
  logs,
  ledger,
  live = false,
  onHistoryReady,
  receipt = null,
}: {
  logs: string[];
  ledger: LedgerRow[];
  live?: boolean;
  onHistoryReady?: () => void;
  receipt?: { ceiling?: string; actual_amount?: string; refunded_change?: string; claim_tx_hash?: string; breakdown?: string[] } | null;
}) {
  const stream = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState<'stream' | 'ledger' | null>(null);
  const [chain, setChain] = useState<LedgerRow[]>([]);
  const rows = useMemo(() => mergeRows(chain, ledger), [chain, ledger]);

  useEffect(() => {
    let cancelled = false;
    async function hydrate() {
      try {
        const response = await fetch(`${API_URL}/api/ledger/onchain`);
        if (response.ok && !cancelled) {
          const body = (await response.json()) as { events?: ChainEvent[] };
          setChain((body.events ?? []).map(chainRow).filter((row) => row.txHash));
        }
      } catch {
        if (!cancelled) setChain([]);
      } finally {
        if (!cancelled) onHistoryReady?.();
      }
    }
    hydrate();
    return () => {
      cancelled = true;
    };
  }, [onHistoryReady]);

  useEffect(() => {
    const node = stream.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
  }, [logs]);

  async function copy(target: 'stream' | 'ledger') {
    const text =
      target === 'stream'
        ? logs.join('\n')
        : rows.map((row) => [row.kind, row.block ?? '', row.txHash, row.amount ?? '', row.status].join('\t')).join('\n');
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
      {receipt?.actual_amount ? (
        <section className="term rounded-2xl border border-emerald/40 p-4 text-sm">
          <h2 className="text-lg font-medium">Metered Settlement Receipt</h2>
          <p className="mt-2">Authorized max ceiling {receipt.ceiling ? `${tusd(receipt.ceiling)} tUSD8` : '0.50 tUSD8'}</p>
          <p>Actual compute consumed {tusd(receipt.actual_amount)} tUSD8</p>
          <p>Instant on-chain change refunded {tusd(receipt.refunded_change)} tUSD8</p>
          <p className="mt-2 inline-block rounded-full border border-emerald/50 px-2 py-1 text-xs text-emerald">66% fewer locks than 3 separate escrows</p>
          {receipt.breakdown?.length ? <p className="mt-2 font-mono text-xs text-slate-400">{receipt.breakdown.join(' · ')}</p> : null}
          {receipt.claim_tx_hash ? (
            <a className="mt-2 block text-cyan underline-offset-2 hover:underline" href={explorerTx(receipt.claim_tx_hash)} target="_blank" rel="noreferrer">
              TaskCompletedMetered
            </a>
          ) : null}
        </section>
      ) : null}
      <section className="term rounded-2xl border border-line p-4">
        <header className="mb-3 flex items-center justify-between text-sm text-slate-300">
          <span>Verifiable Aevum Testnet Ledger</span>
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
              {rows.length === 0 ? (
                <tr>
                  <td className="py-3 text-slate-500" colSpan={5}>
                    On-chain locks, claims, and refunds appear here after the escrow history loads.
                  </td>
                </tr>
              ) : null}
              {rows.map((row) => (
                <tr key={`${row.kind}-${row.txHash}`} className="border-t border-white/5">
                  <td className="py-2 pr-2">{row.kind}</td>
                  <td>{row.block || '—'}</td>
                  <td>
                    <a className="text-cyan underline-offset-2 hover:underline" href={explorerTx(row.txHash)} target="_blank" rel="noreferrer">
                      {shortHash(row.txHash)}
                    </a>
                  </td>
                  <td>{row.amount ? tusd(row.amount) : '—'}</td>
                  <td className={row.status === 'REFUNDED' ? 'text-cyan' : ''}>{row.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function chainRow(event: ChainEvent): LedgerRow {
  return {
    kind: event.event || 'On-chain',
    txHash: event.txHash || '',
    amount: event.amount && event.amount !== '0' ? event.amount : undefined,
    block: event.blockNumber,
    status: eventStatus(event.event),
  };
}

function eventStatus(event: string | undefined) {
  if (event === 'TaskRefunded') return 'REFUNDED';
  if (event === 'TaskCompleted') return 'SETTLED';
  if (event === 'SessionRevoked') return 'Revoked';
  if (event === 'SessionAuthorized') return 'Authorized';
  if (event === 'TaskEscrowLocked') return 'Locked';
  return 'Confirmed';
}

function mergeRows(chain: LedgerRow[], live: LedgerRow[]) {
  const order: string[] = [];
  const rows = new Map<string, LedgerRow>();
  for (const row of [...chain, ...live]) {
    const key = row.txHash.toLowerCase();
    if (!key) continue;
    const existing = rows.get(key);
    if (!existing) {
      order.push(key);
      rows.set(key, row);
      continue;
    }
    rows.set(key, {
      ...existing,
      ...row,
      kind: row.kind || existing.kind,
      block: row.block || existing.block,
      amount: row.amount || existing.amount,
      txHash: existing.txHash,
    });
  }
  return order.map((key) => rows.get(key) as LedgerRow);
}
