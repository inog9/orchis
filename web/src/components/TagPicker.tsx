import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { ApiError } from '../api/client';
import { useCreateTag, useTags } from '../api/hooks';
// Pilih beberapa tag; tag baru bisa dibuat langsung (Enter).
export function TagPicker({ value, onChange, canCreate }: {
    value: string[];
    onChange: (ids: string[]) => void;
    canCreate: boolean;
}) {
    useLocale();
    const tags = useTags();
    const create = useCreateTag();
    const [name, setName] = useState('');
    const [error, setError] = useState('');
    const toggle = (id: string) => onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id]);
    const add = () => {
        const n = name.trim();
        if (!n)
            return;
        setError('');
        create.mutate({ name: n }, {
            onSuccess: (t) => {
                onChange([...value, t.id]);
                setName('');
            },
            onError: (e) => setError(e instanceof ApiError ? (e.fields[0]?.message ?? e.detail) : textUI("m_2e65fa33b980")),
        });
    };
    return (<fieldset className="tagpicker">
      <legend>{textUI("m_1331275bc537")}</legend>
      <div className="chips" role="group" aria-label={textUI("m_0bb840b90edb")}>
        {formatUI(tags.isPending && <span className="muted small">{textUI("m_23acfd6690fb")}</span>)}
        {formatUI(tags.data?.length === 0 && <span className="muted small">{textUI("m_431f90600235")}</span>)}
        {formatUI(tags.data?.map((t) => (<button key={t.id} type="button" className={`chip ${value.includes(t.id) ? 'is-on' : ''}`} aria-pressed={value.includes(t.id)} onClick={() => toggle(t.id)}>
            <span className="chip__dot" style={{ background: t.color }} aria-hidden="true"/>
            {formatUI(t.name)}
          </button>)))}
      </div>
      {formatUI(canCreate && (<div className="tagpicker__new">
          <input aria-label={textUI("m_4f8db0339aac")} placeholder={textUI("m_506fac01d499")} value={name} maxLength={50} onChange={(e) => {
                setName(e.target.value);
                setError('');
            }} onKeyDown={(e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    add();
                }
            }}/>
          <button type="button" className="btn btn--secondary btn--auto btn--sm" onClick={add} disabled={create.isPending || !name.trim()}>{textUI("m_3e10b08cc874")}</button>
        </div>))}
      {formatUI(error && (<p className="field__error" role="alert">
          {formatUI(error)}
        </p>))}
    </fieldset>);
}
export function TagChips({ tags }: {
    tags: {
        id: string;
        name: string;
        color: string;
    }[];
}) {
    useLocale();
    if (tags.length === 0)
        return null;
    return (<span className="tag-list">
      {formatUI(tags.map((t) => (<span key={t.id} className="tag">
          <span className="chip__dot" style={{ background: t.color }} aria-hidden="true"/>
          {formatUI(t.name)}
        </span>)))}
    </span>);
}
