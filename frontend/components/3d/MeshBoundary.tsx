'use client';

import { Component, type ReactNode } from 'react';

type State = { failed: boolean };

export class MeshBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  render() {
    if (this.state.failed) {
      return (
        <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center text-sm text-slate-400">
          <svg viewBox="0 0 360 140" className="h-28 w-full max-w-md" role="img" aria-label="Orchestrator, worker, and escrow">
            <line x1="70" y1="70" x2="180" y2="70" stroke="#243140" strokeDasharray="4 4" />
            <line x1="180" y1="70" x2="290" y2="70" stroke="#243140" />
            <circle cx="70" cy="70" r="28" fill="#0c1017" stroke="#10b981" />
            <circle cx="180" cy="70" r="28" fill="#0c1017" stroke="#06b6d4" />
            <circle cx="290" cy="70" r="28" fill="#0c1017" stroke="#f59e0b" />
            <text x="70" y="74" textAnchor="middle" fill="#10b981" fontSize="10">Orch</text>
            <text x="180" y="74" textAnchor="middle" fill="#06b6d4" fontSize="10">Worker</text>
            <text x="290" y="74" textAnchor="middle" fill="#f59e0b" fontSize="10">Escrow</text>
          </svg>
          <p>WebGL is unavailable. This 2D map stays in place while task status updates below.</p>
        </div>
      );
    }
    return this.props.children;
  }
}
