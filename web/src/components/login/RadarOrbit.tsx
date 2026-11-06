import { useEffect, useId, useRef, useState } from 'react';
import { RavenMark } from '../RavenMark';

// Teks teknis hiasan (bukan data nyata dan tidak memanggil API apa pun).
const LABELS = ['scan://edge-01 · OK', 'TLS 1.3 · verified', 'port 443 · open', 'CVE check · clean'];

// Simpul sasaran yang mengorbit di cincin tengah: sudut awal (derajat), kecepatan (derajat/detik), ukuran (px pada orbit 440).
const NODES = [
    { phase: 38, speed: 14, size: 10, tone: 'gold' },
    { phase: 131, speed: -9, size: 6, tone: 'dim' },
    { phase: 214, speed: 22, size: 7, tone: 'gold' },
    { phase: 305, speed: -17, size: 6, tone: 'dim' },
] as const;

const SWEEP_DPS = 60; // 6 detik per putaran
const BURST_DPS = 240;
const RING_FRAC = 162 / 220; // jari-jari cincin tengah relatif terhadap orbit
const EYE = { x: 34, y: 10.4 }; // posisi mata burung pada kanvas gagak, dalam persen

type Label = { id: number; text: string; x: number; y: number; side: 'l' | 'r' };

const flash = (el: Element | null, frames: Keyframe[], ms: number) => {
    if (el && typeof (el as HTMLElement).animate === 'function') (el as HTMLElement).animate(frames, { duration: ms, easing: 'ease-out' });
};

export function RadarOrbit({ running, still }: { running: boolean; still: boolean }) {
    const uid = useId().replace(/:/g, '');
    const root = useRef<HTMLDivElement>(null);
    const sweep = useRef<HTMLDivElement>(null);
    const nodeEls = useRef<(HTMLSpanElement | null)[]>([]);
    const coreEls = useRef<(HTMLSpanElement | null)[]>([]);
    const rippleEls = useRef<(HTMLSpanElement | null)[]>([]);
    const pos = useRef(NODES.map(() => ({ x: 0, y: 0 })));
    const hover = useRef(false);
    const burst = useRef(0);
    const [label, setLabel] = useState<Label | null>(null);

    // Satu loop rAF: sapuan radar, posisi simpul, dan deteksi saat tepi sapuan melewati simpul. Hanya transform yang ditulis.
    useEffect(() => {
        const el = root.current;
        if (!el) return;
        let size = el.clientWidth || 440;
        const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => { size = el.clientWidth || size; place(angles(), 0); });
        ro?.observe(el);
        let sweepAngle = 40;
        let speedFactor = 1;
        const nodeAngle: number[] = NODES.map((n) => n.phase);
        const prevRel = NODES.map(() => 0);
        const angles = () => nodeAngle;
        function place(a: number[], _t: number) {
            const r = (size / 2) * RING_FRAC;
            a.forEach((deg, i) => {
                const rad = (deg * Math.PI) / 180;
                const x = Math.cos(rad) * r;
                const y = Math.sin(rad) * r;
                pos.current[i] = { x, y };
                const n = nodeEls.current[i];
                if (n) n.style.transform = `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px)`;
            });
        }
        const paintSweep = () => { if (sweep.current) sweep.current.style.transform = `rotate(${sweepAngle.toFixed(1)}deg)`; };
        place(nodeAngle, 0);
        paintSweep();
        prevRel.forEach((_, i) => { prevRel[i] = (((sweepAngle - 90 - nodeAngle[i]!) % 360) + 360) % 360; });
        if (!running || still || typeof requestAnimationFrame !== 'function') return () => ro?.disconnect();
        let raf = 0, last = 0;
        const hit = (i: number) => {
            flash(coreEls.current[i] ?? null, [{ transform: 'scale(1)', boxShadow: '0 0 0 0 rgba(212,176,122,0)' }, { transform: 'scale(2.1)', boxShadow: '0 0 14px 3px rgba(212,176,122,.75)' }, { transform: 'scale(1)', boxShadow: '0 0 0 0 rgba(212,176,122,0)' }], 800);
            flash(rippleEls.current[i] ?? null, [{ transform: 'scale(1)', opacity: 0.6 }, { transform: 'scale(5.5)', opacity: 0 }], 1100);
        };
        const tick = (t: number) => {
            const dt = Math.min(0.05, (t - (last || t)) / 1000);
            last = t;
            speedFactor += ((hover.current ? 1.35 : 1) - speedFactor) * Math.min(1, dt * 4);
            const dps = burst.current > 0 ? BURST_DPS : SWEEP_DPS * speedFactor;
            const step = dps * dt;
            if (burst.current > 0) burst.current = Math.max(0, burst.current - step);
            sweepAngle = (sweepAngle + step) % 360;
            NODES.forEach((n, i) => {
                nodeAngle[i] = (nodeAngle[i]! + n.speed * dt + 360) % 360;
                // Tepi depan sapuan ada di sudut (sweepAngle - 90): rotate(0) CSS menunjuk ke atas, sudut simpul diukur dari arah kanan.
                const rel = (((sweepAngle - 90 - nodeAngle[i]!) % 360) + 360) % 360;
                // rel naik terus; melompat ke ~0 saat tepi sapuan menyusul simpul.
                if (rel < prevRel[i]! - 180) hit(i);
                prevRel[i] = rel;
            });
            place(nodeAngle, t);
            paintSweep();
            raf = requestAnimationFrame(tick);
        };
        raf = requestAnimationFrame(tick);
        return () => { cancelAnimationFrame(raf); ro?.disconnect(); };
    }, [running, still]);

    // Label teknis muncul sesekali di dekat sebuah simpul lalu memudar.
    useEffect(() => {
        if (!running || still) { setLabel(null); return; }
        let timer = 0, clear = 0, id = 0;
        const arm = (ms: number) => {
            timer = window.setTimeout(() => {
                const i = Math.floor(Math.random() * NODES.length);
                const p = pos.current[i]!;
                const side = p.x > 0 ? 'r' : 'l';
                setLabel({ id: ++id, text: LABELS[Math.floor(Math.random() * LABELS.length)]!, x: p.x + (side === 'r' ? 14 : -14), y: p.y - 16, side });
                clear = window.setTimeout(() => setLabel(null), 3400);
                arm(8000 + Math.random() * 4000);
            }, ms);
        };
        arm(3600);
        return () => { clearTimeout(timer); clearTimeout(clear); };
    }, [running, still]);

    const maskId = `radar-mask-${uid}`;
    return (<div ref={root} className={`radar${still ? ' is-still' : ''}`} aria-hidden="true">
      <div className="radar__rings">
        <svg className="radar__svg radar__ring-outer" viewBox="0 0 440 440" focusable="false">
          <defs>
            <mask id={maskId}><circle cx="220" cy="220" r="219.5" fill="none" stroke="#fff" strokeWidth="8" pathLength="1" className="radar__draw"/></mask>
          </defs>
          <circle cx="220" cy="220" r="219.5" fill="none" stroke="#8A7350" strokeOpacity=".5" strokeWidth="1" strokeDasharray="3 7" mask={`url(#${maskId})`}/>
        </svg>
        <svg className="radar__svg radar__ring-mid" viewBox="0 0 440 440" focusable="false">
          <circle cx="220" cy="220" r="162" fill="none" stroke="#3A3F48" strokeWidth="1" pathLength="1" className="radar__draw radar__draw--2"/>
        </svg>
        <svg className="radar__svg radar__ring-inner" viewBox="0 0 440 440" focusable="false">
          <circle cx="220" cy="220" r="106" fill="none" stroke="#4A4F58" strokeWidth="1" pathLength="1" className="radar__draw radar__draw--3"/>
        </svg>
        <div className="radar__sweep-wrap"><div ref={sweep} className="radar__sweep"/></div>
        <div className="radar__nodes">
          {NODES.map((n, i) => (<span key={i} ref={(e) => { nodeEls.current[i] = e; }} className="radar__node">
              <span ref={(e) => { rippleEls.current[i] = e; }} className="radar__ripple" style={{ width: n.size, height: n.size, left: -n.size / 2, top: -n.size / 2 }}/>
              <span ref={(e) => { coreEls.current[i] = e; }} className={`radar__core radar__core--${n.tone}`} style={{ width: n.size, height: n.size, left: -n.size / 2, top: -n.size / 2 }}/>
            </span>))}
          {label && (<span key={label.id} className={`radar__label radar__label--${label.side}`} style={{ transform: `translate(${label.x.toFixed(0)}px, ${label.y.toFixed(0)}px)` }}>{label.text}</span>)}
        </div>
      </div>
      <div className="radar__emblem" onPointerEnter={() => { hover.current = true; }} onPointerLeave={() => { hover.current = false; }} onClick={() => { if (running && !still && burst.current <= 0) burst.current = 360; }}>
        <div className="radar__glow"/>
        <div className="radar__float">
          <RavenMark className="radar__raven" size={260}/>
          <span className="radar__eye" style={{ left: `${EYE.x}%`, top: `${EYE.y}%` }}><i/></span>
        </div>
      </div>
    </div>);
}
