'use client';

import { useEffect, useRef } from 'react';
import { explorerTx } from '@/lib/chain';

export type LogLine = { id: number; time: string; text: string; tx?: string };

export function Terminals({ httpLogs, chainLogs }: { httpLogs: LogLine[]; chainLogs: LogLine[] }) {
  return (
    <section className="grid gap-3 lg:grid-cols-2">
      <Pane title="HTTP 402 · agent log" lines={httpLogs} />
      <Pane title="Aevum settlement" lines={chainLogs} linked />
    </section>
  );
}

function Pane({ title, lines, linked = false }: { title: string; lines: LogLine[]; linked?: boolean }) {
  const endRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'nearest' });
  }, [lines]);

  return (
    <div className="term flex h-72 flex-col overflow-hidden rounded-2xl border border-line">
      <div className="border-b border-line px-3 py-2 text-xs uppercase tracking-[0.16em] text-slate-400">
        {title}
      </div>
      <div className="flex-1 space-y-1 overflow-y-auto px-3 py-2 font-mono text-xs leading-5">
        {lines.length === 0 && <p className="text-slate-600">waiting for traffic</p>}
        {lines.map((line) => (
          <p key={line.id} className="break-all text-slate-300">
            <span className="text-slate-500">{line.time}</span> {line.text}{' '}
            {linked && line.tx && (
              <a className="text-signal underline-offset-2 hover:underline" href={explorerTx(line.tx)} target="_blank" rel="noreferrer">
                {line.tx}
              </a>
            )}
          </p>
        ))}
        <div ref={endRef} />
      </div>
    </div>
  );
}
