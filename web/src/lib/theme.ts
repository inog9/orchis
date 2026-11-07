import { useSyncExternalStore } from 'react';

export type Theme = 'light' | 'dark';
export const THEME_KEY = 'orchis.theme';
const listeners = new Set<() => void>();

function stored(): Theme | null {
    try { const v = localStorage.getItem(THEME_KEY); return v === 'light' || v === 'dark' ? v : null; } catch { return null; }
}
function systemTheme(): Theme {
    return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}
// Pilihan pengguna menang; tanpa pilihan, ikut preferensi sistem.
export function currentTheme(): Theme { return stored() ?? systemTheme(); }
export function applyTheme(theme: Theme = currentTheme()) {
    if (typeof document !== 'undefined') document.documentElement.setAttribute('data-theme', theme);
}
export function setTheme(theme: Theme) {
    try { localStorage.setItem(THEME_KEY, theme); } catch { /* penyimpanan diblokir: pilihan berlaku sampai muat ulang */ }
    applyTheme(theme);
    for (const l of listeners) l();
}
export function toggleTheme() { setTheme(currentTheme() === 'light' ? 'dark' : 'light'); }
export function initTheme() {
    applyTheme();
    if (typeof matchMedia === 'function') matchMedia('(prefers-color-scheme: light)').addEventListener?.('change', () => { if (!stored()) { applyTheme(); for (const l of listeners) l(); } });
}
function subscribe(l: () => void) { listeners.add(l); return () => { listeners.delete(l); }; }
export function useTheme(): Theme { return useSyncExternalStore(subscribe, currentTheme, () => 'dark'); }
