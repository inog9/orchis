import { Moon, Sun } from 'lucide-react';
import { t as textUI } from '../i18n/runtime';
import { toggleTheme, useTheme } from '../lib/theme';

// Tombol ikon tema. Dipakai di bilah atas aplikasi dan di halaman masuk (kiri atas),
// dengan kunci penyimpanan dan perilaku yang sama (keputusan pemilik §1b-3).
export function ThemeToggle({ className = '' }: { className?: string }) {
    const theme = useTheme();
    const toLight = theme === 'dark';
    const label = toLight ? textUI("m_theme_to_light") : textUI("m_theme_to_dark");
    return (
        <button type="button" className={`theme-toggle${className ? ` ${className}` : ''}`} onClick={toggleTheme} aria-label={label} title={label}>
            {toLight ? <Moon size={18} aria-hidden="true"/> : <Sun size={18} aria-hidden="true"/>}
        </button>
    );
}
