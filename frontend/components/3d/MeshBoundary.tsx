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
        <div className="flex h-full items-center justify-center px-6 text-center text-sm text-slate-400">
          WebGL is unavailable in this browser. Task status still updates in the terminal.
        </div>
      );
    }
    return this.props.children;
  }
}
