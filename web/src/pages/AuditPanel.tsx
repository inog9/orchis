import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from '../components/ErrorNote';
import './AuditPanel.css';
type Entry = {
    id: string;
    at: string;
    actor_user_id: string | null;
    actor: string;
    action: string;
    resource_type: string;
    resource_id: string | null;
    ip: string;
    request_id: string;
    reason: string;
};
type Page = {
    items: Entry[];
    next_cursor: string | null;
};
const labels: Record<string, string> = { get 'auth.login'() {
        return textUI("m_ca566c8968e7");
    }, get 'auth.logout'() {
        return textUI("m_80c6e7caffee");
    }, get 'auth.login_failed'() {
        return textUI("m_5ee01ca9e679");
    } };
export function AuditPanel() {
    useLocale();
    const me = useMe();
    const allowed = can(me.data, 'audit.read');
    const [action, setAction] = useState('');
    const [cursors, setCursors] = useState<string[]>([]);
    const cursor = cursors.at(-1) ?? '';
    const query = useQuery({ queryKey: ['audit', action, cursor], queryFn: () => api.get<Page>('/api/v1/audit?' + new URLSearchParams({ limit: '25', action, cursor })), enabled: allowed });
    if (me.isPending)
        return <span className="skeleton" role="status" aria-label={textUI("m_ff9b1147e2a9")}/>;
    if (!allowed)
        return <div className="state"><h2>{textUI("m_33ca59f915ea")}</h2><p>{textUI("m_c1de6e668915")}</p></div>;
    return <section className="stack" aria-labelledby="audit-title">
  <div><h2 id="audit-title" className="panel__heading">{textUI("m_e4d36f9a4e22")}</h2><p className="muted">{textUI("m_3c2d8c724a1b")}</p></div>
  <div className="field"><label htmlFor="audit-action" className="field__label">{textUI("m_70d817035981")}</label><select id="audit-action" className="audit-filter" value={action} onChange={event => { setAction(event.target.value); setCursors([]); }}><option value="">{textUI("m_20b8487b3804")}</option><option value="auth.login">{textUI("m_ca566c8968e7")}</option><option value="auth.logout">{textUI("m_80c6e7caffee")}</option><option value="auth.login_failed">{textUI("m_5ee01ca9e679")}</option></select></div>
  {formatUI(query.isPending ? <span className="skeleton" role="status" aria-label={textUI("m_be915eb1b307")}/> : query.isError ? <div className="state state--error" role="alert"><h3>{textUI("m_d59ad08f5cc4")}</h3><p>{formatUI(describeError(query.error).message)}</p><button className="btn btn--secondary btn--auto" onClick={() => query.refetch()}>{textUI("m_d8b8392e2c54")}</button></div> : !query.data?.items.length ? <div className="state"><h3>{textUI("m_f29f67a5a45f")}</h3><p>{textUI("m_4f3a2b20d515")}</p></div> : <ul className="audit-list" aria-label={textUI("m_cc672a8a17a0")}>{formatUI(query.data.items.map(entry => <li key={entry.id + entry.at} className="audit-entry">
   <div className="audit-entry__head"><strong>{formatUI(labels[entry.action] ?? entry.action.replaceAll('.', ' · '))}</strong><time dateTime={entry.at}>{formatUI(new Date(entry.at).toLocaleString(locale()))}</time></div>
   <dl className="audit-entry__meta"><div><dt>{textUI("m_449995c4fe8f")}</dt><dd>{formatUI(entry.actor || (entry.actor_user_id ? textUI("m_6f48f04fa43f") : entry.action.startsWith('auth.') ? textUI("m_76f4e015467e") : textUI("m_6725e7bbcd28")))}</dd></div><div><dt>{textUI("m_eb3f1fe56719")}</dt><dd>{formatUI(entry.ip || textUI("m_b37c7879f6c3"))}</dd></div>{formatUI(entry.resource_type && <div><dt>{textUI("m_eb7a842ff958")}</dt><dd>{formatUI(entry.resource_type)}{formatUI(entry.resource_id && <> · <span className="mono">{formatUI(entry.resource_id)}</span></>)}</dd></div>)}{formatUI(entry.reason && <div><dt>{textUI("m_4e80abb5b146")}</dt><dd>{formatUI(entry.reason.replaceAll('_', ' '))}</dd></div>)}{formatUI(entry.request_id && <div><dt>{textUI("m_59f03d642b41")}</dt><dd className="mono">{formatUI(entry.request_id)}</dd></div>)}</dl>
  </li>))}</ul>)}
  <nav className="audit-pagination" aria-label={textUI("m_cf7ab3591ef5")}><button className="btn btn--secondary btn--auto" disabled={!cursors.length || query.isFetching} onClick={() => setCursors(value => value.slice(0, -1))}>{textUI("m_a57b08a480b8")}</button><span className="muted small">{richUI("m_502a79508657", [formatUI(cursors.length + 1)])}</span><button className="btn btn--secondary btn--auto" disabled={!query.data?.next_cursor || query.isFetching} onClick={() => { if (query.data?.next_cursor)
        setCursors(value => [...value, query.data!.next_cursor!]); }}>{textUI("m_1ff57a29d7c9")}</button></nav>
 </section>;
}
