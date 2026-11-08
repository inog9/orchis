import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import type { User } from '../api/types';
import { language, setLanguage, t, useLocale, type Language } from './runtime';

// Toggle bahasa minimal EN | ID untuk bilah atas (keputusan pemilik: ganti dropdown berat).
// Preferensinya sama (tersimpan per akun); pilihan tanpa sesi hanya berlaku di sesi ini.
export function LanguageToggle() {
    useLocale();
    const me = useMe();
    const qc = useQueryClient();
    const save = useMutation({
        mutationFn: (ui_language: Language) => api.patch<User>('/api/v1/auth/preferences', { ui_language }),
        onSuccess: user => {
            qc.setQueryData(['me'], user);
            setLanguage(user.ui_language === 'id' ? 'id' : 'en');
        },
    });
    const pick = (value: Language) => {
        if (value === language()) return;
        if (me.data) save.mutate(value); else setLanguage(value);
    };
    const item = (value: Language, label: string) => (
        <button type="button" className={`lang-toggle__item${language() === value ? ' is-on' : ''}`} aria-pressed={language() === value} disabled={save.isPending} onClick={() => pick(value)} lang={value}>{label}</button>
    );
    return (
        <div className="lang-toggle" role="group" aria-label={t('language_label')}>
            {item('en', 'EN')}
            <span className="lang-toggle__sep" aria-hidden="true">|</span>
            {item('id', 'ID')}
            {save.isError && <p role="alert" className="sr-only">{t('language_save_failed')}</p>}
            {save.isSuccess && <span className="sr-only" role="status">{t('language_saved')}</span>}
        </div>
    );
}
