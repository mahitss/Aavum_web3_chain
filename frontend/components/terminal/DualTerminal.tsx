'use client';

import { explorerTx } from '@/config/chains';
import { shortHash, tusd } from '@/lib/format';

export type HandshakeLog = string;

export type LedgerRow = {
  kind: string;
  txHash: string;
  amount?: string;
  status: string;
  block?: number;
};

export function DualTerminal({
  logs,
  ledger,
}: {
  logs: HandshakeLog[];
  ledger: LedgerRow[];
}) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <section className="term rounded-2xl border border-line p-4">
        <header className="mb-3 text-xs uppercase tracking-[0.18em] text-signal">
          Autonomous HTTP 402 handshake stream
        </header>
        <pre className="h-72 overflow-auto font-mono text-xs leading-5 text-slate-300">
          {logs.length ? logs.join('\n') : 'Waiting for orchestrator telemetry...'}
        </pre>
      </section>
      <section className="term rounded-2xl border border-line p-4">
        <header className="mb-3 text-xs uppercase tracking-[0.18em] text-worker">
          Verifiable Aevum Testnet ledger
        </header>
        <div className="h-72 overflow-auto">
          <table className="w-full text-left font-mono text-xs">
            <thead className="text-slate-500">
              <tr>
                <th className="pb-2">Event</th>
                <th>Block</th>
                <th>Tx</th>
                <th>Amount</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {ledger.map((row) => (
                <tr key={`${row.kind}-${row.txHash}`} className="border-t border-white/5">
                  <td className="py-2 pr-2">{row.kind}</td>
                  <td>{row.block || '—'}</td>
                  <td>
                    <a className="text-signal underline-offset-2 hover:underline" href={explorerTx(row.txHash)} target="_blank" rel="noreferrer">
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
