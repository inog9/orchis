import { Lock } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { t as textUI, tr as richUI, useLocale } from '../../i18n/runtime';
import { CursorSpotlight } from './CursorSpotlight';
import { usePageVisible, usePrefersReducedMotion, useWidePanel, type Pointer } from './hooks';
import { RadarOrbit } from './RadarOrbit';
import { StatusPill } from './StatusPill';
import { TelemetryFeed } from './TelemetryFeed';
import './login-hero.css';

function HeroWordmark() {
    useLocale();
    const word = textUI("m_f8dfae0a44c7");
    return (<div className="wordmark hero-wm" role="img" aria-label={word}>
      <span className="wordmark__dot" aria-hidden="true"/>
      <span className="wordmark__text" style={{ fontSize: 20 }} aria-hidden="true">
        {[...word].map((ch, i) => (<span key={i} className="hero-wm__ch" style={{ ['--i' as string]: i }}>{ch}</span>))}
      </span>
    </div>);
}

// Judul dipecah per kata agar muncul satu per satu; kata pertama tiap baris mendapat kilau emas.
function Tagline() {
    useLocale();
    const lines = textUI("m_52a079eafe32", ['\n']).split('\n');
    let n = 0;
    return (<h2 className="hero-headline">
      {lines.map((line, li) => (<span key={li} className="hero-line">
          {line.split(' ').filter(Boolean).map((word, wi) => (<span key={wi} className={`hero-word${wi === 0 ? ' hero-word--shine' : ''}`} style={{ ['--i' as string]: n++ }}>{word}{' '}</span>))}
        </span>))}
    </h2>);
}

const clamp = (v: number) => Math.max(-1, Math.min(1, v));

// Panel kiri login: instrumen keamanan yang "hidup". Murni hiasan; formulir di sisi kanan tidak disentuh.
export function LoginHeroPanel() {
    useLocale();
    const root = useRef<HTMLElement>(null);
    const pointer = useRef<Pointer>({ x: -9999, y: -9999, on: false });
    const reduced = usePrefersReducedMotion();
    const wide = useWidePanel();
    const visible = usePageVisible();
    const running = wide && visible && !reduced;

    // Pelacakan kursor: satu loop rAF yang menghaluskan kemiringan (ease seperti pegas) dan memindahkan sorotan.
    useEffect(() => {
        const el = root.current;
        if (!el || !running || typeof requestAnimationFrame !== 'function') return;
        const bg = el.querySelector<HTMLElement>('.hero-bg');
        const radar = el.querySelector<HTMLElement>('.radar');
        const cur = { x: 0, y: 0, spot: 0 };
        const tgt = { x: 0, y: 0, spot: 0 };
        let raf = 0;
        const frame = () => {
            raf = 0;
            cur.x += (tgt.x - cur.x) * 0.09;
            cur.y += (tgt.y - cur.y) * 0.09;
            cur.spot += (tgt.spot - cur.spot) * 0.1;
            radar?.style.setProperty('--tx', cur.x.toFixed(3));
            radar?.style.setProperty('--ty', cur.y.toFixed(3));
            bg?.style.setProperty('--spot', cur.spot.toFixed(3));
            if (pointer.current.on) {
                bg?.style.setProperty('--mx', `${pointer.current.x.toFixed(0)}px`);
                bg?.style.setProperty('--my', `${pointer.current.y.toFixed(0)}px`);
            }
            if (Math.abs(tgt.x - cur.x) > 0.002 || Math.abs(tgt.y - cur.y) > 0.002 || Math.abs(tgt.spot - cur.spot) > 0.01 || pointer.current.on) raf = requestAnimationFrame(frame);
        };
        const kick = () => { if (!raf) raf = requestAnimationFrame(frame); };
        const move = (e: PointerEvent) => {
            const r = el.getBoundingClientRect();
            pointer.current = { x: e.clientX - r.left, y: e.clientY - r.top, on: true };
            tgt.x = clamp((pointer.current.x / r.width) * 2 - 1);
            tgt.y = clamp((pointer.current.y / r.height) * 2 - 1);
            tgt.spot = 1;
            kick();
        };
        const leave = () => {
            pointer.current = { ...pointer.current, on: false };
            tgt.x = 0; tgt.y = 0; tgt.spot = 0;
            kick();
        };
        el.addEventListener('pointermove', move, { passive: true });
        el.addEventListener('pointerleave', leave);
        return () => {
            el.removeEventListener('pointermove', move);
            el.removeEventListener('pointerleave', leave);
            cancelAnimationFrame(raf);
            pointer.current = { x: -9999, y: -9999, on: false };
        };
    }, [running]);

    return (<aside ref={root} className={`auth__brand hero${running ? '' : ' is-still'}`} aria-hidden="false">
      <CursorSpotlight pointer={pointer} running={running} still={!running}/>
      <div className="hero-top">
        <HeroWordmark/>
        <StatusPill/>
      </div>
      <RadarOrbit running={running} still={!running}/>
      <TelemetryFeed running={running} still={!running}/>
      <div className="hero-copy">
        <Tagline/>
        <div className="hero-meta">{richUI("m_54f0c67845c1", [<><span className="hero-rule"/><Lock className="hero-lock" size={13} aria-hidden="true"/></>])}</div>
      </div>
    </aside>);
}
