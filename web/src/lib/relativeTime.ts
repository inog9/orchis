import { locale } from '../i18n/runtime';

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [['year', 31_536_000], ['month', 2_592_000], ['day', 86_400], ['hour', 3600], ['minute', 60]];

// "2h ago" / "2 j lalu": waktu relatif singkat menurut bahasa aktif. Tanggal tak valid mengembalikan ''.
export function relativeTime(iso: string | null | undefined, now = Date.now()): string {
    if (!iso) return '';
    const t = Date.parse(iso);
    if (Number.isNaN(t)) return '';
    const seconds = Math.round((t - now) / 1000);
    const fmt = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto', style: 'narrow' });
    for (const [unit, size] of UNITS) {
        if (Math.abs(seconds) >= size) return fmt.format(Math.round(seconds / size), unit);
    }
    return fmt.format(0, 'second');
}
