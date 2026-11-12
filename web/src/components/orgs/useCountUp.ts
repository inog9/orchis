import { useEffect, useState } from 'react';
import { usePrefersReducedMotion } from '../../lib/usePrefersReducedMotion';

// Angka naik dari 0 ke target (ease-out). Gerak dikurangi: langsung ke nilai akhir.
export function useCountUp(target: number, ms = 700): number {
    const reduced = usePrefersReducedMotion();
    const [value, setValue] = useState(() => (reduced || typeof requestAnimationFrame !== 'function' ? target : 0));
    useEffect(() => {
        if (reduced || typeof requestAnimationFrame !== 'function') { setValue(target); return; }
        let raf = 0, start = 0;
        const tick = (t: number) => {
            start = start || t;
            const p = Math.min(1, (t - start) / ms);
            setValue(Math.round(target * (1 - Math.pow(1 - p, 3))));
            if (p < 1) raf = requestAnimationFrame(tick);
        };
        raf = requestAnimationFrame(tick);
        return () => cancelAnimationFrame(raf);
    }, [target, reduced, ms]);
    return value;
}
