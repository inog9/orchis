import { Globe } from 'lucide-react';
import { useEffect } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import type { User } from '../api/types';
import { browserLanguage, language, setLanguage, t, useLocale, type Language } from './runtime';

// The server preference is account-scoped. Logging out resets to this browser's default.
export function LanguageSession() {
  const me = useMe();
  useEffect(() => {
    if (me.isPending || me.isError) return;
    const saved = me.data?.ui_language;
    setLanguage(saved === 'en' || saved === 'id' ? saved : browserLanguage());
  }, [me.data?.id, me.data?.ui_language, me.isPending, me.isError]);
  return null;
}
export function LanguagePreference() {
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
  return <div className="language-preference">
    <Globe className="language-preference__icon" size={16} aria-hidden="true"/>
    <label htmlFor="language-preference">{t('language_label')}</label>
    <select id="language-preference" value={language()} disabled={save.isPending} onChange={event => {
      const value = event.target.value as Language;
      if (me.data) save.mutate(value); else setLanguage(value);
    }}>
      <option value="en" lang="en">English</option>
      <option value="id" lang="id">Indonesia</option>
    </select>
    {save.isError && <p role="alert">{t('language_save_failed')}</p>}
    {save.isSuccess && <span className="sr-only" role="status">{t('language_saved')}</span>}
  </div>;
}
