import { useEffect, useRef, type RefObject } from 'react';
import type { Pointer } from './hooks';

const GOLD = '184,149,95';
const GOLD_HI = '212,176,122';
const LINK = 130;
const NEAR = 190;

type Node = { x: number; y: number; vx: number; vy: number };

// Jaringan titik yang hanyut pelan; garis di dekat kursor menyala. Hanya transform/opacity di DOM, kanvas digambar ulang per frame.
function Constellation({ pointer, running, still }: { pointer: RefObject<Pointer>; running: boolean; still: boolean }) {
    const ref = useRef<HTMLCanvasElement>(null);
    useEffect(() => {
        const canvas = ref.current;
        const ctx = canvas?.getContext?.('2d');
        if (!canvas || !ctx) return;
        let w = 0, h = 0, raf = 0, last = 0;
        let nodes: Node[] = [];
        const seed = () => {
            const count = Math.max(10, Math.min(34, Math.round((w * h) / 24000)));
            nodes = Array.from({ length: count }, () => ({ x: Math.random() * w, y: Math.random() * h, vx: (Math.random() - 0.5) * 0.18, vy: (Math.random() - 0.5) * 0.18 }));
        };
        const resize = () => {
            const dpr = Math.min(window.devicePixelRatio || 1, 2);
            w = canvas.clientWidth;
            h = canvas.clientHeight;
            canvas.width = Math.max(1, Math.round(w * dpr));
            canvas.height = Math.max(1, Math.round(h * dpr));
            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
            seed();
            draw();
        };
        const draw = () => {
            ctx.clearRect(0, 0, w, h);
            const p = pointer.current;
            const cx = p && p.on && !still ? p.x : -9999;
            const cy = p && p.on && !still ? p.y : -9999;
            for (let i = 0; i < nodes.length; i++) {
                const a = nodes[i]!;
                for (let j = i + 1; j < nodes.length; j++) {
                    const b = nodes[j]!;
                    const d = Math.hypot(a.x - b.x, a.y - b.y);
                    if (d > LINK) continue;
                    const mid = Math.hypot((a.x + b.x) / 2 - cx, (a.y + b.y) / 2 - cy);
                    const boost = mid < NEAR ? (1 - mid / NEAR) : 0;
                    const alpha = (1 - d / LINK) * (0.07 + boost * 0.42);
                    ctx.strokeStyle = `rgba(${boost > 0.05 ? GOLD_HI : GOLD},${alpha.toFixed(3)})`;
                    ctx.lineWidth = 1;
                    ctx.beginPath();
                    ctx.moveTo(a.x, a.y);
                    ctx.lineTo(b.x, b.y);
                    ctx.stroke();
                }
            }
            for (const n of nodes) {
                const near = Math.hypot(n.x - cx, n.y - cy) < NEAR;
                ctx.fillStyle = `rgba(${near ? GOLD_HI : GOLD},${near ? 0.6 : 0.22})`;
                ctx.beginPath();
                ctx.arc(n.x, n.y, near ? 1.8 : 1.3, 0, Math.PI * 2);
                ctx.fill();
            }
        };
        const tick = (t: number) => {
            const dt = Math.min(48, t - (last || t));
            last = t;
            for (const n of nodes) {
                n.x += n.vx * dt * 0.06;
                n.y += n.vy * dt * 0.06;
                if (n.x < -20) n.x = w + 20; else if (n.x > w + 20) n.x = -20;
                if (n.y < -20) n.y = h + 20; else if (n.y > h + 20) n.y = -20;
            }
            draw();
            raf = requestAnimationFrame(tick);
        };
        resize();
        const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(resize);
        ro?.observe(canvas);
        if (running && !still && typeof requestAnimationFrame === 'function') raf = requestAnimationFrame(tick);
        return () => {
            cancelAnimationFrame?.(raf);
            ro?.disconnect();
        };
    }, [pointer, running, still]);
    return <canvas ref={ref} className="hero-net" aria-hidden="true"/>;
}

// Lapisan paling belakang: kisi titik samar, kisi yang lebih terang di sekitar kursor, cahaya emas, dan jaringan konstelasi.
// Posisi kursor datang dari variabel CSS --mx/--my yang diisi LoginHeroPanel (satu loop rAF).
export function CursorSpotlight({ pointer, running, still }: { pointer: RefObject<Pointer>; running: boolean; still: boolean }) {
    return (<div className="hero-bg" aria-hidden="true">
      <div className="hero-grid"/>
      <div className="hero-grid hero-grid--lit"/>
      <Constellation pointer={pointer} running={running} still={still}/>
      <div className="hero-spot"/>
    </div>);
}
