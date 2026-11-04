import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
type Range = {
    id: string;
    cidr: string;
    authorized: boolean;
    version: number;
    authorized_by: string;
    authorized_at: string;
};
type Networks = {
    reported: string[];
    approvals: Range[];
    max_hosts: number;
    runner: string;
    source_error?: string;
};
export function AgentNetworks({ org, agent, selected = [], onSelect }: {
    org: string;
    agent: string;
    selected?: string[];
    onSelect?: (ids: string[]) => void;
}) {
    useLocale();
    const { data: me } = useMe();
    const [cidr, setCIDR] = useState('');
    const path = `/api/v1/organizations/${org}/agents/${agent}/networks`;
    const q = useQuery({ queryKey: ['agent-networks', org, agent], queryFn: () => api.get<Networks>(path), retry: false });
    const approve = useMutation({ mutationFn: (input: {
            cidr: string;
            authorized: boolean;
            version: number;
        }) => api.put(path, input), onSuccess: () => q.refetch() });
    const admin = can(me, 'settings.manage') && can(me, 'asset.authorize') && can(me, 'organization.write');
    return <section className="org-card" aria-label={textUI("m_8c10602211d2")}><h3>{textUI("m_3592d7ce3aad")}</h3><p>{textUI("m_067d647177ee")}</p>{formatUI(q.isPending && <p role="status">{textUI("m_2a7b002a4866")}</p>)}{formatUI(q.error && <p className="alert" role="alert">{formatUI(describeError(q.error).message)}</p>)}{formatUI(q.data && <>{formatUI(q.data.source_error && <p className="notice">{formatUI(q.data.source_error)}</p>)}<p className="muted">{richUI("m_34a9e08b60b6", [formatUI(q.data.max_hosts)])}</p><p>{richUI("m_88b95cb45642", [formatUI(q.data.reported.join(', ') || textUI("m_32b8c979134c"))])}</p><ul className="stack-list">{formatUI(q.data.approvals.map(r => <li key={r.id}><label className="check"><input type="checkbox" disabled={!r.authorized || !onSelect || q.isError || !!q.data?.source_error} checked={selected.includes(r.id)} onChange={e => onSelect?.(e.target.checked ? [...selected, r.id] : selected.filter(id => id !== r.id))}/><span className="check__box" aria-hidden="true"/><span>{richUI("m_aa83e2890090", [formatUI(r.cidr), formatUI(r.authorized ? textUI("m_87b42e40c2a2") : textUI("m_f6f738d04392")), formatUI(r.version)])}</span></label><p className="muted small">{richUI("m_c83dbbf9ceec", [formatUI(r.authorized_by), formatUI(new Date(r.authorized_at).toLocaleString(locale()))])}</p>{formatUI(admin && r.authorized && <button className="btn btn--secondary btn--auto" type="button" disabled={approve.isPending} onClick={() => approve.mutate({ cidr: r.cidr, authorized: false, version: r.version })}>{richUI("m_02ff89306904", [formatUI(r.cidr)])}</button>)}</li>))}</ul>{formatUI(admin && <div className="field"><label htmlFor={`network-approval-${agent}`}>{textUI("m_cc1dc4571546")}</label><div className="field__input"><input id={`network-approval-${agent}`} value={cidr} onChange={e => setCIDR(e.target.value)} maxLength={64} placeholder={q.data.reported[0] ?? '192.168.1.0/24'}/></div><button className="btn btn--secondary btn--auto" type="button" disabled={!cidr.trim() || approve.isPending} onClick={() => approve.mutate({ cidr: cidr.trim(), authorized: true, version: q.data.approvals.find(r => r.cidr === cidr.trim())?.version ?? 0 })}>{textUI("m_dd45e0655174")}</button><p className="field__hint">{textUI("m_cf39b2c20a68")}</p></div>)}</>)}{formatUI(approve.error && <p className="alert" role="alert">{formatUI(describeError(approve.error).message)}</p>)}</section>;
}
