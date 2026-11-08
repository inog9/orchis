import { useSyncExternalStore } from 'react';

export type Theme = 'light' | 'dark';
export const THEME_KEY = 'orchis.theme';
const listeners = new Set<() => void>();

function stored(): Theme | null {
    try { const v = localStorage.getItem(THEME_KEY); return v === 'light' || v === 'dark' ? v : null; } catch { return null; }
}
// Pilihan pengguna menang; tanpa pilihan tersimpan, bawaan selalu terang
// (keputusan pemilik: bukan preferensi OS, bukan gelap). Gelap tetap tersedia lewat toggle.
export function currentTheme(): Theme { return stored() ?? 'light'; }
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
}
function subscribe(l: () => void) { listeners.add(l); return () => { listeners.delete(l); }; }
export function useTheme(): Theme { return useSyncExternalStore(subscribe, currentTheme, () => 'light'); }
