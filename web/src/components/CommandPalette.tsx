import { navigationFor } from './navigation';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can, type User } from '../api/types';
import { Modal } from './Modal';

type Entry = { id: string; label: string; hint: string; to: string; show: boolean; keys?: string; words?: string };

// Daftar tujuan yang boleh dibuka pengguna ini. 'keys' = pintasan "g lalu huruf".
export function entriesFor(me: User | null | undefined): Entry[] {
  const all: Entry[] = [
    ...navigationFor(me).map(n => ({ id:n.id, label:n.label, hint:n.hint, to:n.to, show:true, keys:'keys' in n ? n.keys : undefined })),
    { id: 'chains', label: 'Edit scenario chains', hint: 'Open the action editor for an existing scenario; this is the same scenario list.', to: '/chains', show: can(me, 'scenario.read'), keys: 'c', words:'attack chains' },
    { id: 'atomic', label: 'Saved single tests', hint: 'Single tests: run one saved check against one authorized asset.', to: '/atomic', show: can(me, 'atomic.read'), keys: 't', words:'atomic testing' },
    { id: 'groups', label: 'Asset groups', hint: 'Dynamic groups', to: '/assets/groups', show: can(me, 'asset.read') },
    { id: 'people', label: 'People', hint: 'Exercise participants', to: '/people', show: can(me, 'people.read'), keys: 'p' },
    { id: 'teams', label: 'Teams', hint: 'Groups of people', to: '/people/teams', show: can(me, 'people.read') },
    { id: 'mitre', label: 'MITRE ATT&CK', hint: 'Technique coverage', to: '/mitre', show: can(me, 'scenario.read'), keys: 'x', words: 'attack matrix techniques tactics' },
    { id: 'notifications', label: 'Notifications', hint: 'Alerts', to: '/notifications', show: can(me, 'alert.read'), keys: 'n' },
    { id: 'profile', label: 'Profile', hint: 'Password and API tokens', to: '/profile', show: true },
    { id: 'new-asset', label: 'Add asset', hint: 'Create', to: '/assets?new=1', show: can(me, 'asset.write') },
    { id: 'new-scenario', label: 'New scenario', hint: 'Create', to: '/scenarios?new=1', show: can(me, 'scenario.write') },
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
export function useCommandPalette(): [boolean, (v: boolean) => void] {
  const [open, setOpen] = useState(false);
  const { data: me } = useMe();
  const navigate = useNavigate();
  const pending = useRef<number | null>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setOpen((o) => !o); return; }
      if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target) || document.querySelector('[role=dialog]')) return;
      if (pending.current !== null) {
        const hit = entriesFor(me).find((x) => x.keys === e.key.toLowerCase());
        window.clearTimeout(pending.current);
        pending.current = null;
        if (hit) { e.preventDefault(); navigate(hit.to); }
        return;
      }
      if (e.key === 'g') pending.current = window.setTimeout(() => { pending.current = null; }, 1200);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [me, navigate]);
  return [open, setOpen];
}

export function CommandPalette({ onClose }: { onClose: () => void }) {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const [q, setQ] = useState('');
  const [sel, setSel] = useState(0);
  const list = useMemo(() => entriesFor(me).filter((e) => matches(e, q)), [me, q]);
  const go = (to: string) => { onClose(); navigate(to); };
  return (
    <Modal labelledBy="palette-title" onClose={onClose} width={560}>
      <>
        <h2 id="palette-title" className="sr-only">Search or jump to</h2>
        <div className="field__input palette__input">
          <input aria-label="Search or jump to" role="combobox" aria-expanded="true" aria-controls="palette-list" aria-activedescendant={list[sel] ? `pal-${list[sel]!.id}` : undefined}
            placeholder="Search or jump to…" value={q} autoFocus
            onChange={(e) => { setQ(e.target.value); setSel(0); }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(list.length - 1, s + 1)); }
              else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(0, s - 1)); }
              else if (e.key === 'Enter' && list[sel]) { e.preventDefault(); go(list[sel]!.to); }
            }} />
        </div>
        <ul id="palette-list" role="listbox" className="palette__list" aria-label="Destinations">
          {list.length === 0 ? <li className="muted pad" role="option" aria-selected="false">Nothing matches “{q}”.</li> : list.map((e, i) => (
            <li key={e.id} id={`pal-${e.id}`} role="option" aria-selected={i === sel} className={i === sel ? 'is-on' : ''} onMouseEnter={() => setSel(i)} onClick={() => go(e.to)}>
              <strong>{e.label}</strong><span className="muted small">{e.hint}</span>{e.keys && <kbd>g {e.keys}</kbd>}
            </li>
          ))}
        </ul>
      </>
    </Modal>
  );
}
