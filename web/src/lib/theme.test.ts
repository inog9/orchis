import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, currentTheme, setTheme, THEME_KEY, toggleTheme } from './theme';

beforeEach(() => { localStorage.clear(); document.documentElement.removeAttribute('data-theme'); });
afterEach(() => vi.unstubAllGlobals());

describe('theme', () => {
    it('tanpa pilihan mengikuti sistem; dengan pilihan pengguna menang', () => {
        vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener() {} }));
        expect(currentTheme()).toBe('light');
        localStorage.setItem(THEME_KEY, 'dark');
        expect(currentTheme()).toBe('dark');
    });
    it('setTheme menyimpan dan memasang atribut; toggle membalik', () => {
        vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener() {} }));
        applyTheme();
        expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
        toggleTheme();
        expect(document.documentElement.getAttribute('data-theme')).toBe('light');
        expect(localStorage.getItem(THEME_KEY)).toBe('light');
        setTheme('dark');
        expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });
    it('penyimpanan yang melempar galat tidak merusak', () => {
        vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener() {} }));
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
        expect(() => setTheme('light')).not.toThrow();
        expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });
});
