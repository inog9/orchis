import { useEffect, useState } from 'react';

export function usePrefersReducedMotion(): boolean {
    const [reduced, setReduced] = useState(() =>
        typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches);
    useEffect(() => {
        if (!window.matchMedia) return;
        const media = window.matchMedia('(prefers-reduced-motion: reduce)');
        const sync = () => setReduced(media.matches);
        sync();
        media.addEventListener?.('change', sync);
        return () => media.removeEventListener?.('change', sync);
    }, []);
    return reduced;
}
