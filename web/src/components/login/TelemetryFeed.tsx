import { useEffect, useState } from 'react';

// Hanya hiasan: baris log tiruan dari larik lokal, tanpa panggilan API. Cap waktu memakai jam perangkat.
const FEED: ReadonlyArray<readonly [string, string]> = [
    ['agent.sync', 'ok'],
    ['recon.dns', '12 assets indexed'],
    ['report.gen', 'queued'],
    ['scan.tls', 'TLS 1.3 verified'],
    ['scan.ports', '443 open · 22 filtered'],
    ['intel.cve', '0 new advisories'],
    ['agent.sync', 'ok · 3 endpoints'],
    ['report.gen', 'delivered'],
];
const KEY_W = 13;
const pad = (n: number) => String(n).padStart(2, '0');
const stamp = (d: Date) => `[${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}]`;

type Line = { id: number; stamp: string; key: string; val: string; shown: number };
const total = (l: Line) => l.stamp.length + 1 + KEY_W + l.val.length;
const make = (id: number, [key, val]: readonly [string, string], d: Date, shown: number): Line => ({ id, stamp: stamp(d), key: key.padEnd(KEY_W), val, shown });

// Potong tiga bagian baris (cap waktu, kunci, nilai) menurut jumlah karakter yang sudah "diketik".
function Parts({ line }: { line: Line }) {
    const a = line.stamp.length + 1;
    const stampText = (line.stamp + ' ').slice(0, line.shown);
    const keyText = line.key.slice(0, Math.max(0, line.shown - a));
    const valText = line.val.slice(0, Math.max(0, line.shown - a - KEY_W));
    return (<>
      <span className="tele__t">{stampText}</span>
      <span className="tele__k">{keyText}</span>
      <span className={valText === 'ok' || valText.startsWith('ok ') ? 'tele__v tele__v--ok' : 'tele__v'}>{valText}</span>
    </>);
}

export function TelemetryFeed({ running, still }: { running: boolean; still: boolean }) {
    const [lines, setLines] = useState<Line[]>(() => {
        const now = new Date();
        return still ? FEED.slice(0, 3).map((f, i) => { const l = make(i, f, now, 0); return { ...l, shown: total(l) }; }) : [];
    });
    useEffect(() => {
        if (!running || still) return;
        let idx = 0, id = 0, iv = 0, wait = 0;
        const next = () => {
            const line = make(++id, FEED[idx++ % FEED.length]!, new Date(), 0);
            setLines((prev) => [...prev.slice(-2), line]);
            let n = 0;
            iv = window.setInterval(() => {
                n += 1;
                setLines((prev) => prev.map((l) => (l.id === line.id ? { ...l, shown: n } : l)));
                if (n >= total(line)) {
                    clearInterval(iv);
                    wait = window.setTimeout(next, 1500 + Math.random() * 800);
                }
            }, 30);
        };
        wait = window.setTimeout(next, 1500);
        return () => { clearTimeout(wait); clearInterval(iv); };
    }, [running, still]);
    return (<div className="tele" aria-hidden="true">
      <div className="tele__bar"><i/><i/><i/></div>
      {lines.map((l, i) => (<div key={l.id} className="tele__line">
          <Parts line={l}/>
          {i === lines.length - 1 && <span className="tele__cursor"/>}
        </div>))}
      {lines.length === 0 && <div className="tele__line"><span className="tele__cursor"/></div>}
    </div>);
}
