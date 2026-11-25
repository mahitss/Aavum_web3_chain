'use client';

import { useEffect, useState } from 'react';
import { API_URL } from '@/lib/contracts';

export function SystemWakeBanner() {
  const [waking, setWaking] = useState(false);

  useEffect(() => {
    let stop = false;
    const reveal = window.setTimeout(() => {
      if (!stop) setWaking(true);
    }, 3000);

    async function waitUntilHealthy() {
      while (!stop) {
        try {
          const response = await fetch(`${API_URL}/health`, { cache: 'no-store' });
          if (response.ok) {
            stop = true;
            window.clearTimeout(reveal);
            setWaking(false);
            return;
          }
        } catch {
          if (!stop) setWaking(true);
        }
        await new Promise((resolve) => window.setTimeout(resolve, 2000));
      }
    }

    void waitUntilHealthy();
    return () => {
      stop = true;
      window.clearTimeout(reveal);
    };
  }, []);

  if (!waking) return null;
  return (
    <p className="rounded-xl border border-cyan/40 bg-card px-4 py-3 text-sm" role="status">
      Waking Aevum Agent Mesh...
    </p>
  );
}
