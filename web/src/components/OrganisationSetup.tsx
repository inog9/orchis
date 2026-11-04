import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
export type Readiness = {
    total: number;
    authorized: number;
    ready: number;
    items: {
        id: string;
        name: string;
        authorized: boolean;
        reason: string;
    }[];
};
export function useReadiness(id: string) {
    const { data: me } = useMe();
    return useQuery({ queryKey: ['organization-readiness', id], enabled: !!id && can(me, 'asset.read') && can(me, 'organization.read'), queryFn: () => api.get<Readiness>(`/api/v1/organizations/${id}/readiness`), refetchInterval: 30000 });
}
export function setupSteps(v: {
    organization: boolean;
    assets: number;
    authorized: number;
    ready: number;
    assessments: number;
    reports: number;
}) {
    return [v.organization, v.assets > 0, v.authorized > 0 && v.ready > 0, v.assessments > 0, v.reports > 0];
}
export function OrganisationSetup() {
    useLocale();
    const { data: me } = useMe();
    const [selected, setSelected] = useState('');
    const orgs = useQuery({ queryKey: ['setup-organizations'], queryFn: () => api.get<{
            items: {
                id: string;
                name: string;
            }[];
            next_cursor: string | null;
        }>('/api/v1/organizations?limit=100') });
    const id = selected || orgs.data?.items[0]?.id || '';
    const ready = useReadiness(id);
    const assessments = useQuery({ queryKey: ['setup-assessments', id], enabled: !!id && can(me, 'assessment.read'), queryFn: () => api.get<{
            items: {
                id: string;
            }[];
        }>(`/api/v1/organizations/${id}/assessments?limit=1`), refetchInterval: 5000 });
    const reports = useQuery({ queryKey: ['setup-reports', id], enabled: !!id && can(me, 'report.read') && can(me, 'finding.read') && can(me, 'asset.read') && can(me, 'assessment.read'), queryFn: () => api.get<{
            items: {
                id: string;
                status: string;
            }[];
        }>(`/api/v1/reports?organization_id=${id}&limit=1`), refetchInterval: 5000 });
    const v = ready.data;
    const latest = reports.data?.items[0];
    const done = setupSteps({ organization: !!id, assets: v?.total ?? 0, authorized: v?.authorized ?? 0, ready: v?.ready ?? 0, assessments: assessments.data?.items.length ?? 0, reports: latest?.status === 'ready' ? 1 : 0 });
    const next = done.findIndex(d => !d);
    const orgPath = id ? `/organizations/${id}` : '/organizations';
    const steps = [
        { title: textUI("m_1c3b4bf6ffd9"), text: textUI("m_bbe2847107fd"), to: orgPath, perm: 'organization.write' },
        { title: textUI("m_0f4b8d36735f"), text: textUI("m_cbf9beaa88e3"), to: id ? orgPath : '/assets', perm: 'asset.write' },
        { title: textUI("m_a2389492485c"), text: textUI("m_dcabc6a09a25"), to: orgPath, perm: 'asset.authorize' },
        { title: textUI("m_0cf4ea064b2f"), text: textUI("m_83d557ed7e23"), to: orgPath, perm: 'assessment.run' },
        { title: textUI("m_38f569a75743"), text: latest?.status === 'failed' ? textUI("m_6b6b1dc1cfbd") : textUI("m_1afd105ca2f0"), to: latest ? `/reports/${latest.id}` : '/reports', perm: 'report.read' },
    ];
    const error = orgs.error || ready.error || assessments.error || reports.error;
    return <section className="panel" aria-labelledby="setup-title"><div className="setup-head"><div><p className="eyebrow">{textUI("m_e44a4595fc68")}</p><h2 id="setup-title" className="panel__heading">{textUI("m_8ab2c8b7736c")}</h2><p className="muted">{textUI("m_ea6a6f04a6ab")}</p></div>{formatUI(!!orgs.data?.items.length && <div className="select"><label htmlFor="setup-org">{textUI("m_98c8da378fd2")}</label><select id="setup-org" value={id} onChange={e => setSelected(e.target.value)}>{formatUI(orgs.data.items.map(o => <option key={o.id} value={o.id}>{formatUI(o.name)}</option>))}</select></div>)}</div>
 {formatUI(orgs.isPending && <p role="status">{textUI("m_baa41bf44218")}</p>)}{formatUI(error && <div className="alert" role="alert"><p>{richUI("m_121ed1ecad98", [formatUI(describeError(error).message)])}</p><button className="link-quiet" onClick={() => {
                void orgs.refetch();
                if (id) {
                    void ready.refetch();
                    void assessments.refetch();
                    void reports.refetch();
                }
            }}>{textUI("m_c55e1d589a38")}</button></div>)}
 <ol className="setup-list" aria-label={textUI("m_1856e453f00a")}>{formatUI(steps.map((s, i) => <li key={s.title} data-complete={done[i]} className={done[i] ? 'is-done' : ''} aria-current={i === next ? 'step' : undefined}><span className="setup-number">{formatUI(i + 1)} · {formatUI(done[i] ? textUI("m_143b270a3260") : i === next ? textUI("m_298a9207a732") : textUI("m_100ec1bc78a4"))}</span><Link to={s.to}>{formatUI(s.title)}</Link><p>{formatUI(s.text)}</p>{formatUI(!done[i] && !can(me, s.perm) && <p>{richUI("m_30bf2b0988f3", [formatUI(i === 3 ? ' or analyst' : '')])}</p>)}</li>))}</ol>
 {formatUI(orgs.data?.next_cursor && <Link to="/organizations">{textUI("m_4dce4b6b294a")}</Link>)}
 <details className="setup-map"><summary>{textUI("m_0dbc8287dfc0")}</summary><p>{textUI("m_891ee738faa1")}</p><p>{textUI("m_715c686fd4a2")}</p><p>{textUI("m_3479e28b0877")}</p></details></section>;
}
