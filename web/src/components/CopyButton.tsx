import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Check, Copy } from 'lucide-react';
import { useState } from 'react';
// Menyalin ke papan klip; bila peramban menolak (mis. bukan HTTPS), pengguna tetap bisa memilih teksnya sendiri.
export function CopyButton({ value, label = textUI("m_e21f935f11d7") }: {
    value: string;
    label?: string;
}) {
    useLocale();
    const [done, setDone] = useState(false);
    return (<button type="button" className="btn btn--secondary btn--sm" onClick={async () => {
            try {
                await navigator.clipboard.writeText(value);
                setDone(true);
                setTimeout(() => setDone(false), 1800);
            }
            catch { /* biarkan pengguna menyalin manual */ }
        }}>
      {formatUI(done ? <Check size={14} aria-hidden="true"/> : <Copy size={14} aria-hidden="true"/>)} {formatUI(done ? textUI("m_8d525e5f158b") : label)}
    </button>);
}
