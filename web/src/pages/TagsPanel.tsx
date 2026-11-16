import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Plus, X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useDeleteTag, useMe, useSaveTag, useTagList } from '../api/hooks';
import { can, type TagRow } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { ConfirmDialog } from './ConfirmDialog';
function TagDialog({ tag, onClose }: {
    tag: TagRow | null;
    onClose: () => void;
}) {
    useLocale();
    const save = useSaveTag();
    const [name, setName] = useState(tag?.name ?? '');
    const [color, setColor] = useState(tag?.color ?? '#8C919A');
    const err = save.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const submit = (e: FormEvent) => { e.preventDefault(); save.mutate({ id: tag?.id, name, color }, { onSuccess: onClose }); };
    return (<Modal labelledBy="tag-title" onClose={onClose} width={440}>
      <>
        <header className="dialog__head"><h2 id="tag-title">{formatUI(tag ? textUI("m_8b8e9d1789c2") : textUI("m_609117d5b0f9"))}</h2><button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button></header>
        <form className="dialog__form" onSubmit={submit} noValidate>
          {formatUI(err && !(err instanceof ApiError && (err.status === 422)) && <div className="alert" role="alert"><strong>{textUI("m_e924a6d48f91")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} error={fieldErr('name')} autoFocus/>
          <div className="field"><div className="field__label-row"><label htmlFor="tag-color" className="field__label">{textUI("m_6b73191a0a4b")}</label></div>
            <div className="field__input"><input id="tag-color" type="color" value={color} onChange={(e) => setColor(e.target.value.toUpperCase())}/></div></div>
          <footer className="dialog__foot"><button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_1509f561f241"))}</button></footer>
        </form>
      </>
    </Modal>);
}
export function TagsPanel() {
    useLocale();
    const { data: me } = useMe();
    const q = useTagList();
    const del = useDeleteTag();
    const [editing, setEditing] = useState<TagRow | 'new' | null>(null);
    const [deleting, setDeleting] = useState<TagRow | null>(null);
    return (<div className="stack">
      <div className="posture__head"><p className="muted">{textUI("m_d93e8f2ed1bf")}</p>
        {formatUI(can(me, 'asset.write') && <button type="button" className="btn btn--primary btn--auto" onClick={() => setEditing('new')}>{richUI("m_120a75de103c", [<Plus size={16} aria-hidden="true"/>])}</button>)}</div>
      {formatUI(q.isPending ? <span className="skeleton"/> : q.isError ? <div className="state state--error" role="alert"><h2>{textUI("m_cb228d7b7f8e")}</h2><p>{formatUI(describeError(q.error).message)}</p></div>
            : q.data!.length === 0 ? <div className="state"><h2>{textUI("m_b57d852b14d2")}</h2><p>{textUI("m_428256c77597")}</p></div> : (<ul className="list tag-list" aria-label={textUI("m_1331275bc537")} tabIndex={0}>
          {formatUI(q.data!.map((t) => (<li key={t.id} className="tag-row">
              <span className="tag-chip" style={{ borderColor: t.color }}>{formatUI(t.name)}</span>
              <span className="muted small">{formatUI(t.assets)} {formatUI(t.assets === 1 ? 'asset' : 'assets')}</span>
              <span className="row-actions">
                {formatUI(can(me, 'asset.write') && <button type="button" className="btn btn--secondary btn--sm" aria-label={textUI("m_fdd9d5053ccc", [t.name])} onClick={() => setEditing(t)}>{textUI("m_464c4ffd019e")}</button>)}
                {formatUI(can(me, 'asset.delete') && <button type="button" className="btn btn--danger btn--sm" aria-label={textUI("m_7eab122c2b58", [t.name])} onClick={() => setDeleting(t)}>{textUI("m_e2d0a54968ea")}</button>)}
              </span>
            </li>)))}
        </ul>))}
      {formatUI(editing && <TagDialog tag={editing === 'new' ? null : editing} onClose={() => setEditing(null)}/>)}
      {formatUI(deleting && <ConfirmDialog title={textUI("m_4be2a965ff77", [deleting.name])} body="It is removed from every asset that has it." confirmLabel="Delete tag" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onConfirm={() => del.mutate(deleting.id, { onSuccess: () => setDeleting(null) })} onClose={() => { del.reset(); setDeleting(null); }}/>)}
    </div>);
}
