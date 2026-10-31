import { useState } from 'react';
import { ApiError } from '../api/client';
import { useCreateTag, useTags } from '../api/hooks';

// Pilih beberapa tag; tag baru bisa dibuat langsung (Enter).
export function TagPicker({ value, onChange, canCreate }: { value: string[]; onChange: (ids: string[]) => void; canCreate: boolean }) {
  const tags = useTags();
  const create = useCreateTag();
  const [name, setName] = useState('');
  const [error, setError] = useState('');

  const toggle = (id: string) => onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id]);
  const add = () => {
    const n = name.trim();
    if (!n) return;
    setError('');
    create.mutate(
      { name: n },
      {
        onSuccess: (t) => {
          onChange([...value, t.id]);
          setName('');
        },
        onError: (e) => setError(e instanceof ApiError ? (e.fields[0]?.message ?? e.detail) : 'Could not create the tag.'),
      },
    );
  };

  return (
    <fieldset className="tagpicker">
      <legend>Tags</legend>
      <div className="chips" role="group" aria-label="Available tags">
        {tags.isPending && <span className="muted small">Loading tags…</span>}
        {tags.data?.length === 0 && <span className="muted small">No tags yet.</span>}
        {tags.data?.map((t) => (
          <button key={t.id} type="button" className={`chip ${value.includes(t.id) ? 'is-on' : ''}`} aria-pressed={value.includes(t.id)} onClick={() => toggle(t.id)}>
            <span className="chip__dot" style={{ background: t.color }} aria-hidden="true" />
            {t.name}
          </button>
        ))}
      </div>
      {canCreate && (
        <div className="tagpicker__new">
          <input
            aria-label="New tag name"
            placeholder="New tag…"
            value={name}
            maxLength={50}
            onChange={(e) => {
              setName(e.target.value);
              setError('');
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                add();
              }
            }}
          />
          <button type="button" className="btn btn--secondary btn--auto btn--sm" onClick={add} disabled={create.isPending || !name.trim()}>
            Add tag
          </button>
        </div>
      )}
      {error && (
        <p className="field__error" role="alert">
          {error}
        </p>
      )}
    </fieldset>
  );
}

export function TagChips({ tags }: { tags: { id: string; name: string; color: string }[] }) {
  if (tags.length === 0) return null;
  return (
    <span className="tag-list">
      {tags.map((t) => (
        <span key={t.id} className="tag">
          <span className="chip__dot" style={{ background: t.color }} aria-hidden="true" />
          {t.name}
        </span>
      ))}
    </span>
  );
}
