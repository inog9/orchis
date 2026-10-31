import { Plus, X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useDeleteTag, useMe, useSaveTag, useTagList } from '../api/hooks';
import { can, type TagRow } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { ConfirmDialog } from './ConfirmDialog';

function TagDialog({ tag, onClose }: { tag: TagRow | null; onClose: () => void }) {
  const save = useSaveTag();
  const [name, setName] = useState(tag?.name ?? '');
  const [color, setColor] = useState(tag?.color ?? '#8C919A');
  const err = save.error;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate({ id: tag?.id, name, color }, { onSuccess: onClose }); };
  return (
    <Modal labelledBy="tag-title" onClose={onClose} width={440}>
      <>
        <header className="dialog__head"><h2 id="tag-title">{tag ? 'Edit tag' : 'New tag'}</h2><button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button></header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {err && !(err instanceof ApiError && (err.status === 422)) && <div className="alert" role="alert"><strong>We couldn’t save</strong><span>{describeError(err).message}</span></div>}
          <Field label="Name" value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} error={fieldErr('name')} autoFocus />
          <div className="field"><div className="field__label-row"><label htmlFor="tag-color" className="field__label">Color</label></div>
            <div className="field__input"><input id="tag-color" type="color" value={color} onChange={(e) => setColor(e.target.value.toUpperCase())} /></div></div>
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending}>{save.isPending ? 'Saving…' : 'Save'}</button></footer>
        </form>
      </>
    </Modal>
  );
}

export function TagsPanel() {
  const { data: me } = useMe();
  const q = useTagList();
  const del = useDeleteTag();
  const [editing, setEditing] = useState<TagRow | 'new' | null>(null);
  const [deleting, setDeleting] = useState<TagRow | null>(null);
  return (
    <div className="stack">
      <div className="posture__head"><p className="muted">Labels you can put on assets and use in asset group rules.</p>
        {can(me, 'asset.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}><Plus size={16} aria-hidden="true" /> New tag</button>}</div>
      {q.isPending ? <span className="skeleton" /> : q.isError ? <div className="state state--error" role="alert"><h2>Couldn’t load tags</h2><p>{describeError(q.error).message}</p></div>
        : q.data!.length === 0 ? <div className="state"><h2>No tags yet</h2><p>Tags help you find and group assets, such as “crown-jewel” or “pci”.</p></div> : (
        <ul className="list tag-list" aria-label="Tags" tabIndex={0}>
          {q.data!.map((t) => (
            <li key={t.id} className="tag-row">
              <span className="tag-chip" style={{ borderColor: t.color, color: t.color }}>{t.name}</span>
              <span className="muted small">{t.assets} {t.assets === 1 ? 'asset' : 'assets'}</span>
              <span className="row-actions">
                {can(me, 'asset.write') && <button type="button" className="btn btn--secondary btn--sm" aria-label={`Edit ${t.name}`} onClick={() => setEditing(t)}>Edit</button>}
                {can(me, 'asset.delete') && <button type="button" className="btn btn--danger btn--sm" aria-label={`Delete ${t.name}`} onClick={() => setDeleting(t)}>Delete</button>}
              </span>
            </li>
          ))}
        </ul>
      )}
      {editing && <TagDialog tag={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      {deleting && <ConfirmDialog title={`Delete “${deleting.name}”?`} body="It is removed from every asset that has it." confirmLabel="Delete tag" busy={del.isPending}
        error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(deleting.id, { onSuccess: () => setDeleting(null) })} onClose={() => { del.reset(); setDeleting(null); }} />}
    </div>
  );
}
