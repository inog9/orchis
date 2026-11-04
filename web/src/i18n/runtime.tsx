import { Fragment, type ReactNode, useSyncExternalStore } from 'react';
import { en } from './en';
import { id } from './id';

export type Language = 'en' | 'id';
export function browserLanguage(languages: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages): Language {
  for (const language of languages) {
    const base = language.toLowerCase().split('-')[0];
    if (base === 'en' || base === 'id') return base;
  }
  return 'en';
}
let selected: Language = browserLanguage();
const listeners = new Set<() => void>();
export function language(): Language { return selected; }
export function locale(): 'en-US' | 'id-ID' { return selected === 'id' ? 'id-ID' : 'en-US'; }
export function setLanguage(value: Language) {
  if (value !== 'en' && value !== 'id') throw new Error('Unsupported language');
  if (typeof document !== 'undefined') document.documentElement.lang = value;
  if (selected === value) return;
  selected = value;
  for (const listener of listeners) listener();
}
function subscribe(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener); }; }
export function useLocale(): Language { return useSyncExternalStore(subscribe, language, () => 'en'); }
function message(key: keyof typeof en): string { return selected === 'id' ? id[key] : en[key]; }
// Only catalogue keys are accepted. Object names, evidence, and server strings never become lookup keys.
export function t(key: keyof typeof en, values: readonly (string | number | boolean | null | undefined)[] = []): string {
  return message(key).replace(/\{(\d+)\}/g, (_, index: string) => {
    const value = values[Number(index)];
    return typeof value === 'number' ? new Intl.NumberFormat(locale()).format(value) : String(value ?? '');
  });
}
// Rich messages use React nodes, never innerHTML. User content remains escaped by React.
export function tr(key: keyof typeof en, values: readonly ReactNode[] = []): ReactNode {
  return message(key).split(/(\{\d+\})/).map((part, index) => <Fragment key={index}>{/^\{\d+\}$/.test(part) ? values[Number(part.slice(1, -1))] : part}</Fragment>);
}
export function fv(value: ReactNode): ReactNode { return typeof value === 'number' ? new Intl.NumberFormat(locale()).format(value) : value; }
export function relativeTime(value: number, unit: Intl.RelativeTimeFormatUnit): string { return new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' }).format(value, unit); }
