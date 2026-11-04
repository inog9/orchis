import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { advancedEnabled, navigationFor } from './navigation';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can, type User } from '../api/types';
import { Modal } from './Modal';
type Entry = {
    id: string;
    label: string;
    hint: string;
    to: string;
    show: boolean;
    keys?: string;
    words?: string;
};
// Daftar tujuan yang boleh dibuka pengguna ini. 'keys' = pintasan "g lalu huruf".
export function entriesFor(me: User | null | undefined): Entry[] {
    const advanced = advancedEnabled(me);
    const all: Entry[] = [
        ...navigationFor(me).map(n => ({ id: n.id, label: n.label, hint: n.hint, to: n.to, show: true, keys: 'keys' in n ? n.keys : undefined })),
        { id: 'chains', label: textUI("m_79aaa1a60301"), hint: textUI("m_7f33e0c6943d"), to: '/chains', show: advanced && can(me, 'scenario.read'), keys: 'c', words: textUI("m_278c3dc570af") },
        { id: 'atomic', label: textUI("m_b8eacc2c624f"), hint: textUI("m_8b745e616dee"), to: '/atomic', show: advanced && can(me, 'atomic.read'), keys: 't', words: textUI("m_225a88dd30c0") },
        { id: 'groups', label: textUI("m_39dd12de3b02"), hint: textUI("m_2a00d7e79433"), to: '/assets/groups', show: can(me, 'asset.read') },
        { id: 'people', label: textUI("m_7db20897053b"), hint: textUI("m_c3fc4af91714"), to: '/people', show: can(me, 'people.read'), keys: 'p' },
        { id: 'teams', label: textUI("m_1e1a1c078af2"), hint: textUI("m_ea10b53c3d1c"), to: '/people/teams', show: can(me, 'people.read') },
        { id: 'mitre', label: 'MITRE ATT&CK', hint: textUI("m_1f74861b5654"), to: '/mitre', show: advanced && can(me, 'scenario.read'), keys: 'x', words: textUI("m_f09df30f682d") },
        { id: 'notifications', label: textUI("m_788011833a5a"), hint: textUI("m_f62e9267d196"), to: '/notifications', show: can(me, 'alert.read') || can(me, 'finding.read') || can(me, 'report.read'), keys: 'n' },
        { id: 'profile', label: textUI("m_d696a35bdd18"), hint: textUI("m_7960d307c9e9"), to: '/profile', show: true },
        { id: 'new-asset', label: textUI("m_48f15767b06d"), hint: textUI("m_4759498ac2a7"), to: '/assets?new=1', show: can(me, 'asset.write') },
        { id: 'new-scan', label: textUI("m_de2b814aeb96"), hint: textUI("m_f5307ff0d903"), to: '/simulations/new', show: can(me, 'simulation.run') },
        { id: 'new-chain', label: textUI("m_7b08c04b5ad8"), hint: textUI("m_9f088dbebd6c"), to: '/chains?new=1', show: advanced && can(me, 'scenario.write') },
        { id: 'overview', label: textUI("m_8de6bc129042"), hint: textUI("m_0c999db652cf"), to: '/overview', show: advanced && can(me, 'organization.read'), keys: 'h' },
    ];
    return all.filter((e) => e.show);
}
export function matches(e: Entry, q: string): boolean {
    const s = q.trim().toLowerCase();
    return s === '' || e.label.toLowerCase().includes(s) || e.hint.toLowerCase().includes(s) || (e.words ?? '').includes(s);
}
function typing(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null;
    return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable);
}
// ⌘K / Ctrl+K membuka palet; "g lalu huruf" berpindah halaman (hanya di luar kolom isian).
export function useCommandPalette(): [
    boolean,
    (v: boolean) => void
] {
    const [open, setOpen] = useState(false);
    const { data: me } = useMe();
    const navigate = useNavigate();
    const pending = useRef<number | null>(null);
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
                e.preventDefault();
                setOpen((o) => !o);
                return;
            }
            if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target) || document.querySelector('[role=dialog]'))
                return;
            if (pending.current !== null) {
                const hit = entriesFor(me).find((x) => x.keys === e.key.toLowerCase());
                window.clearTimeout(pending.current);
                pending.current = null;
                if (hit) {
                    e.preventDefault();
                    navigate(hit.to);
                }
                return;
            }
            if (e.key === 'g')
                pending.current = window.setTimeout(() => { pending.current = null; }, 1200);
        };
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [me, navigate]);
    return [open, setOpen];
}
export function CommandPalette({ onClose }: {
    onClose: () => void;
}) {
    useLocale();
    const { data: me } = useMe();
    const navigate = useNavigate();
    const [q, setQ] = useState('');
    const [sel, setSel] = useState(0);
    const list = useMemo(() => entriesFor(me).filter((e) => matches(e, q)), [me, q, locale()]);
    const go = (to: string) => { onClose(); navigate(to); };
    return (<Modal labelledBy="palette-title" onClose={onClose} width={560}>
      <>
        <h2 id="palette-title" className="sr-only">{textUI("m_4493680c6f79")}</h2>
        <div className="field__input palette__input">
          <input aria-label={textUI("m_4493680c6f79")} role="combobox" aria-expanded="true" aria-controls="palette-list" aria-activedescendant={list[sel] ? `pal-${list[sel]!.id}` : undefined} placeholder={textUI("m_22e6cbc402ef")} value={q} autoFocus onChange={(e) => { setQ(e.target.value); setSel(0); }} onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                setSel((s) => Math.min(list.length - 1, s + 1));
            }
            else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setSel((s) => Math.max(0, s - 1));
            }
            else if (e.key === 'Enter' && list[sel]) {
                e.preventDefault();
                go(list[sel]!.to);
            }
        }}/>
        </div>
        <ul id="palette-list" role="listbox" className="palette__list" aria-label={textUI("m_72eb63f032e4")}>
          {formatUI(list.length === 0 ? <li className="muted pad" role="option" aria-selected="false">{richUI("m_05861bda343b", [formatUI(q)])}</li> : list.map((e, i) => (<li key={e.id} id={`pal-${e.id}`} role="option" aria-selected={i === sel} className={i === sel ? 'is-on' : ''} onMouseEnter={() => setSel(i)} onClick={() => go(e.to)}>
              <strong>{formatUI(e.label)}</strong><span className="muted small">{formatUI(e.hint)}</span>{formatUI(e.keys && <kbd>g {e.keys}</kbd>)}
            </li>)))}
        </ul>
      </>
    </Modal>);
}
