import { useEffect, useState } from 'react';

// Hook kecil untuk panel kiri login. Semuanya aman tanpa matchMedia (jsdom/SSR): bawaannya "gerak boleh, layar lebar".
function useMedia(query: string, fallback: boolean): boolean {
    const [on, setOn] = useState(() => (typeof window !== 'undefined' && window.matchMedia ? window.matchMedia(query).matches : fallback));
    useEffect(() => {
        if (typeof window === 'undefined' || !window.matchMedia) return;
        const mq = window.matchMedia(query);
        const sync = () => setOn(mq.matches);
        sync();
        mq.addEventListener?.('change', sync);
        return () => mq.removeEventListener?.('change', sync);
    }, [query]);
    return on;
}

export const usePrefersReducedMotion = () => useMedia('(prefers-reduced-motion: reduce)', false);
// Panel kiri disembunyikan di bawah 1100px (lihat app.css); animasi tidak perlu berjalan saat itu.
export const useWidePanel = () => useMedia('(min-width: 1101px)', true);

export function usePageVisible(): boolean {
    const [visible, setVisible] = useState(() => typeof document === 'undefined' || document.visibilityState !== 'hidden');
    useEffect(() => {
        const sync = () => setVisible(document.visibilityState !== 'hidden');
        document.addEventListener('visibilitychange', sync);
        return () => document.removeEventListener('visibilitychange', sync);
    }, []);
    return visible;
}

export type Pointer = { x: number; y: number; on: boolean };
