import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useEffect, useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useMe, useMitreImport, useMitreMatrix, useSaveSettings, useSettings } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { when } from '../components/RunBadges';
const COMPONENT_LABEL = { get up() {
        return textUI("m_f4ccae29e1bb");
    }, get down() {
        return textUI("m_5ad99230df69");
    }, get not_configured() {
        return textUI("m_dd1841d29502");
    } } as const;
export function PlatformPanel() {
    useLocale();
    const { data: me } = useMe();
    const q = useSettings();
    const save = useSaveSettings();
    const mitre = useMitreMatrix();
    const importNow = useMitreImport();
    const [name, setName] = useState('');
    const admin = can(me, 'settings.manage');
    useEffect(() => {
        if (q.data)
            setName(q.data.platform_name);
    }, [q.data]);
    const err = save.error;
    const fieldErr = err instanceof ApiError ? err.fields.find((f) => f.name === 'platform_name')?.message : undefined;
    const submit = (e: FormEvent) => { e.preventDefault(); save.mutate(name); };
    const run = mitre.data?.last_run;
    if (q.isPending)
        return <span className="skeleton"/>;
    if (q.isError || !q.data)
        return <div className="state state--error" role="alert"><h2>{textUI("m_26e462978d2f")}</h2><p>{formatUI(describeError(q.error).message)}</p></div>;
    return (<div className="stack">
      <form className="panel" onSubmit={submit} noValidate aria-label={textUI("m_da1aaf08663b")}>
        <h2 className="panel__heading">{textUI("m_c78ffe195710")}</h2>
        <Field label={textUI("m_b36295f8a031")} value={name} onChange={(e) => { save.reset(); setName(e.target.value); }} disabled={!admin} error={fieldErr} hint={textUI("m_a7cd4b648e76")}/>
        <dl className="kv kv--2"><div><dt>{textUI("m_dd167905de0d")}</dt><dd className="mono">{formatUI(q.data.version)}</dd></div></dl>
        {formatUI(err && !fieldErr && <div className="alert" role="alert"><strong>{textUI("m_a018b7a8a188")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
        {formatUI(save.isSuccess && <p className="muted small" role="status">{textUI("m_fca046ccd586")}</p>)}
        {formatUI(admin && <div><button type="submit" className="btn btn--primary btn--auto" disabled={save.isPending || name.trim() === q.data.platform_name}>{formatUI(save.isPending ? textUI("m_23e39291d613") : textUI("m_1509f561f241"))}</button></div>)}
      </form>

      {formatUI(admin && (<section className="panel" aria-labelledby="components-title">
          <h2 id="components-title" className="panel__heading">{textUI("m_a150ce221602")}</h2>
          <ul className="mini-list">
            {formatUI(q.data.components.map((c) => (<li key={c.name}><strong>{formatUI(c.name)}</strong><span className={`status ${c.status === 'up' ? 'status--ok' : c.status === 'down' ? 'status--bad' : ''}`}>{formatUI(COMPONENT_LABEL[c.status])}</span></li>)))}
          </ul>
        </section>))}

      {formatUI(can(me, 'scenario.read') && (<section className="panel" aria-labelledby="attck-title">
          <div className="posture__head">
            <div><h2 id="attck-title" className="panel__heading">{textUI("m_b17732745c40")}</h2>
              <p className="muted small">{formatUI(run ? (run.status === 'succeeded' ? textUI("m_30807d629e28", [when(run.finished_at), run.source_version ? textUI("m_12e75f0ca6e5", [run.source_version]) : '', run.techniques]) : run.status === 'running' ? textUI("m_c01c4324f1fa") : textUI("m_b8c3b91f0f74", [when(run.finished_at)])) : textUI("m_deee0c3b41bc"))}</p></div>
            {formatUI(admin && <button type="button" className="btn btn--secondary btn--auto" disabled={importNow.isPending || run?.status === 'running'} onClick={() => importNow.mutate()}>{formatUI(run?.status === 'running' ? textUI("m_c01c4324f1fa") : textUI("m_6da9285b1e4f"))}</button>)}
          </div>
          {formatUI(run?.status === 'failed' && run.error && <div className="alert" role="alert"><strong>{textUI("m_0a26f41abd00")}</strong><span>{formatUI(run.error)}</span></div>)}
          {formatUI(importNow.isError && <div className="alert" role="alert"><strong>{textUI("m_cf496843a5e6")}</strong><span>{formatUI(describeError(importNow.error).message)}</span></div>)}
        </section>))}
    </div>);
}
