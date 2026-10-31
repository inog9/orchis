import { useEffect, useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useMe, useMitreImport, useMitreMatrix, useSaveSettings, useSettings } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { when } from '../components/RunBadges';

const COMPONENT_LABEL = { up: 'Running', down: 'Not reachable', not_configured: 'Not configured' } as const;

export function PlatformPanel() {
  const { data: me } = useMe();
  const q = useSettings();
  const save = useSaveSettings();
  const mitre = useMitreMatrix();
  const importNow = useMitreImport();
  const [name, setName] = useState('');
  const admin = can(me, 'settings.manage');
  useEffect(() => { if (q.data) setName(q.data.platform_name); }, [q.data]);
  const err = save.error;
  const fieldErr = err instanceof ApiError ? err.fields.find((f) => f.name === 'platform_name')?.message : undefined;
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate(name); };
  const run = mitre.data?.last_run;

  if (q.isPending) return <span className="skeleton" />;
  if (q.isError || !q.data) return <div className="state state--error" role="alert"><h2>Couldn’t load settings</h2><p>{describeError(q.error).message}</p></div>;
  return (
    <div className="stack">
      <form className="panel" onSubmit={submit} noValidate aria-label="Platform parameters">
        <h2 className="panel__heading">Platform</h2>
        <Field label="Platform name" value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} disabled={!admin} error={fieldErr} hint="Shown at the top right of every page." />
        <dl className="kv kv--2"><div><dt>Version</dt><dd className="mono">{q.data.version}</dd></div></dl>
        {err && !fieldErr && <div className="alert" role="alert"><strong>Couldn’t save</strong><span>{describeError(err).message}</span></div>}
        {save.isSuccess && <p className="muted small" role="status">Saved.</p>}
        {admin && <div><button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending || name.trim() === q.data.platform_name}>{save.isPending ? 'Saving…' : 'Save'}</button></div>}
      </form>

      {admin && (
        <section className="panel" aria-labelledby="components-title">
          <h2 id="components-title" className="panel__heading">Components</h2>
          <ul className="mini-list">
            {q.data.components.map((c) => (
              <li key={c.name}><strong>{c.name}</strong><span className={`status ${c.status === 'up' ? 'status--ok' : c.status === 'down' ? 'status--bad' : ''}`}>{COMPONENT_LABEL[c.status]}</span></li>
            ))}
          </ul>
        </section>
      )}

      {can(me, 'scenario.read') && (
        <section className="panel" aria-labelledby="attck-title">
          <div className="posture__head">
            <div><h2 id="attck-title" className="panel__heading">MITRE ATT&amp;CK data</h2>
              <p className="muted small">{run ? (run.status === 'succeeded' ? `Last imported ${when(run.finished_at)}${run.source_version ? ` · version ${run.source_version}` : ''} · ${run.techniques} techniques` : run.status === 'running' ? 'Importing…' : `Last import failed ${when(run.finished_at)}.`) : 'Not imported yet. It downloads automatically; you can also import now.'}</p></div>
            {admin && <button type="button" className="btn btn--secondary btn--auto" disabled={importNow.isPending || run?.status === 'running'} onClick={() => importNow.mutate()}>{run?.status === 'running' ? 'Importing…' : 'Import now'}</button>}
          </div>
          {run?.status === 'failed' && run.error && <div className="alert" role="alert"><strong>Import failed</strong><span>{run.error}</span></div>}
          {importNow.isError && <div className="alert" role="alert"><strong>Couldn’t start the import</strong><span>{describeError(importNow.error).message}</span></div>}
        </section>
      )}
    </div>
  );
}
