import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, currentTheme, setTheme, THEME_KEY, toggleTheme } from './theme';

beforeEach(() => { localStorage.clear(); document.documentElement.removeAttribute('data-theme'); });
afterEach(() => vi.unstubAllGlobals());

describe('theme', () => {
    // Keputusan pemilik (PROMPT §1b-2): tanpa pilihan tersimpan, bawaan selalu terang -
    // bukan preferensi OS. Preferensi sistem tidak lagi mengubah tema.
    it('tanpa pilihan selalu terang; dengan pilihan pengguna menang', () => {
        vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener() {} }));
        expect(currentTheme()).toBe('light');
        localStorage.setItem(THEME_KEY, 'dark');
        expect(currentTheme()).toBe('dark');
        vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener() {} }));
        expect(currentTheme()).toBe('dark'); // pilihan tersimpan tetap menang
        localStorage.removeItem(THEME_KEY);
        expect(currentTheme()).toBe('light'); // preferensi OS gelap pun bawaan tetap terang
    });
    it('setTheme menyimpan dan memasang atribut; toggle membalik', () => {
        applyTheme();
        expect(document.documentElement.getAttribute('data-theme')).toBe('light');
        toggleTheme();
        expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
        expect(localStorage.getItem(THEME_KEY)).toBe('dark');
        setTheme('light');
        expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });
    it('penyimpanan yang melempar galat tidak merusak', () => {
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
        expect(() => setTheme('light')).not.toThrow();
        expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });
});
