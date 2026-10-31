import { Check, Copy } from 'lucide-react';
import { useState } from 'react';

// Menyalin ke papan klip; bila peramban menolak (mis. bukan HTTPS), pengguna tetap bisa memilih teksnya sendiri.
export function CopyButton({ value, label = 'Copy' }: { value: string; label?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button type="button" className="btn btn--secondary btn--sm" onClick={async () => {
      try { await navigator.clipboard.writeText(value); setDone(true); setTimeout(() => setDone(false), 1800); } catch { /* biarkan pengguna menyalin manual */ }
    }}>
      {done ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />} {done ? 'Copied' : label}
    </button>
  );
}
