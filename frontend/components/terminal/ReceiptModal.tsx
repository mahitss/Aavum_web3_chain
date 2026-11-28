'use client';

import { useEffect, useState } from 'react';
import { explorerTx } from '@/config/chains';
import { API_URL } from '@/lib/contracts';
import { canonicalJSON, sha256Hex } from '@/lib/jcs';

type ReceiptDocument = {
  taskId?: string;
  canonical?: string;
  digest?: string;
  receipt?: unknown;
  lockTx?: string;
  settleTx?: string;
  refundTx?: string;
  error?: string;
};

export function ReceiptModal({
  taskId,
  txHash,
  onClose,
}: {
  taskId?: string;
  txHash: string;
  onClose: () => void;
}) {
  const [document, setDocument] = useState<ReceiptDocument | null>(null);
  const [verdict, setVerdict] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    void load();
    // The row that opened the inspector is the receipt to fetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  async function load() {
    if (!taskId) {
      setError('This row has no task id, so the receipt chain cannot be loaded.');
      return;
    }
    setError('');
    const response = await fetch(`${API_URL}/api/receipts/${taskId}`);
    const body = (await response.json()) as ReceiptDocument;
    if (!response.ok) {
      setError(body.error || 'Receipt not found');
      setDocument(null);
      return;
    }
    setDocument(body);
    setVerdict('');
  }

  async function verify() {
    if (!document?.receipt || !document.digest) return;
    const again = canonicalJSON(document.receipt);
    const digest = await sha256Hex(again);
    setVerdict(digest === document.digest && again === document.canonical ? 'VERIFIED' : 'MISMATCH');
  }

  function download() {
    if (!document) return;
    const blob = new Blob([JSON.stringify(document, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = window.document.createElement('a');
    link.href = url;
    link.download = `${taskId || 'receipt'}.json`;
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" role="dialog" aria-modal="true" aria-label="Cryptographic Receipt Inspector">
      <section className="panel max-h-[80vh] w-full max-w-2xl overflow-auto rounded-2xl p-5">
        <header className="flex items-center justify-between gap-3">
          <h2 className="text-lg font-medium">Cryptographic Receipt Inspector</h2>
          <button type="button" onClick={onClose} className="text-sm text-slate-400">Close</button>
        </header>
        <p className="mt-2 text-xs text-slate-400">DelegationGrant → PaymentIntent → SettlementReceipt / RefundClaim</p>
        {!document && !error ? (
          <button type="button" onClick={load} className="mt-4 rounded-lg bg-emerald px-3 py-2 text-sm font-medium text-canvas">Load receipt</button>
        ) : null}
        {error ? <p className="mt-3 text-sm text-panic">{error}</p> : null}
        {document?.canonical ? (
          <>
            <pre className="mt-3 overflow-auto rounded-lg bg-black/40 p-3 font-mono text-xs text-slate-300">{document.canonical}</pre>
            <p className="mt-2 font-mono text-xs text-slate-400">sha256 {document.digest}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              <button type="button" onClick={verify} className="rounded-lg border border-emerald/50 px-3 py-2 text-sm">Verify Digest Locally</button>
              <button type="button" onClick={download} className="rounded-lg border border-cyan/50 px-3 py-2 text-sm">Download Receipt (.json)</button>
              {verdict === 'VERIFIED' ? <span className="rounded-full border border-emerald/50 px-2 py-1 text-xs text-emerald">VERIFIED</span> : null}
              {verdict === 'MISMATCH' ? <span className="rounded-full border border-panic/50 px-2 py-1 text-xs text-panic">MISMATCH</span> : null}
            </div>
            <div className="mt-3 space-y-1 text-sm">
              {document.lockTx ? <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(document.lockTx)} target="_blank" rel="noreferrer">open lock</a> : null}
              {document.settleTx ? <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(document.settleTx)} target="_blank" rel="noreferrer">settlement</a> : null}
              {document.refundTx ? <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(document.refundTx)} target="_blank" rel="noreferrer">refund</a> : null}
              {!document.lockTx && !document.settleTx && !document.refundTx ? (
                <a className="block text-cyan underline-offset-2 hover:underline" href={explorerTx(txHash)} target="_blank" rel="noreferrer">Aevum Explorer</a>
              ) : null}
            </div>
          </>
        ) : null}
      </section>
    </div>
  );
}
