import { t as textUI, tr as richUI, useLocale, locale, fv as formatUI } from "../i18n/runtime";
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { ScanComparison, type Comparison } from '../components/ScanComparison';
import { OrganizationFilter } from '../components/OrganizationFilter';
import { can } from '../api/types';
import { Search } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useFindings, useMe } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { SeverityBadge, when } from '../components/RunBadges';
import { OrganizationGroup } from '../components/OrganizationGroup';
import { groupByOrganization } from '../lib/groupByOrganization';
import type { Finding } from '../api/types';
const SEVERITY_ORDER: Finding['severity'][] = ['critical', 'high', 'medium', 'low', 'info'];
// Di dalam satu organisasi, yang paling parah di atas; urutan waktu dipertahankan untuk tingkat yang sama.
const bySeverity = (items: Finding[]) => [...items].sort((a, b) => SEVERITY_ORDER.indexOf(a.severity) - SEVERITY_ORDER.indexOf(b.severity));
function useDebounced<T>(value: T, ms: number) {
    const [v, setV] = useState(value);
    useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
    return v;
}
function FindingRows({ list, me }: { list: Finding[]; me: ReturnType<typeof useMe>['data'] }) {
    useLocale();
    return (<ul className="list finding-list" aria-label={textUI("m_e171c2ff25b5")} tabIndex={0}>
    {formatUI(list.map((f) => (<li key={f.id} className="finding-row">
        <SeverityBadge severity={f.severity}/>
        <div className="finding-row__main">
          <strong>{formatUI(f.title)}</strong>
          <span className="muted small">{formatUI(f.asset_id && can(me, 'asset.read') ? <Link to={`/assets/${f.asset_id}`}>{formatUI(f.asset_name)}</Link> : f.asset_name || textUI("m_bedb7e61a067"))} · <code className="mono">{f.value}</code></span>
        </div>
        <span className="muted small">{richUI("m_dcf7003b8549", [formatUI(f.occurrence_count), formatUI(when(f.last_seen_at))])}</span>
      </li>)))}
  </ul>);
}
export function FindingsPage({ organizationID, organizationName }: {
    organizationID?: string;
    organizationName?: string;
} = {}) {
    useLocale();
    const [params, setParams] = useSearchParams();
    const assetID = params.get('asset_id') ?? '';
    const { data: me } = useMe();
    const organization = organizationID ?? params.get('org') ?? params.get('organization_id') ?? '';
    const Heading = organizationID ? 'h2' : 'h1';
    const [text, setText] = useState('');
    const [type, setType] = useState('');
    const [severity, setSeverity] = useState('');
    const q = useDebounced(text.trim(), 250);
    const res = useFindings(q, type, severity, assetID, organization);
    const items = res.data?.pages.flatMap((p) => p.items) ?? [];
    const [comparisonMode, setComparisonMode] = useState(false);
    useEffect(() => setComparisonMode(false), [organization]);
    const reportList = useQuery({ queryKey: ['latest-scan-report', organization], enabled: !!organization && can(me, 'report.read'), queryFn: () => api.get<{
            items: {
                id: string;
            }[];
        }>(`/api/v1/reports?organization_id=${encodeURIComponent(organization)}&scan_only=true`), refetchInterval: 5000 });
    const latestID = reportList.data?.items[0]?.id;
    const comparison = useQuery({ queryKey: ['scan-comparison-report', latestID], enabled: !!latestID && comparisonMode, queryFn: () => api.get<{
            id: string;
            created_at: string;
            snapshot: {
                comparison_details?: Comparison;
            };
        }>(`/api/v1/reports/${latestID}`) });
    const grouped = !organization;
    const filtered = !!(q || type || severity || assetID || organization);
    return (<section className="page" aria-labelledby="findings-title">
      <div className="page__head">
        <div>
          <div className="page__title-row"><Heading id="findings-title" className="page__title">{textUI("m_e171c2ff25b5")}</Heading></div>
          <p className="page__sub">{textUI("m_215192db3223")}</p>
        </div>
      </div>
      {formatUI(assetID && <div className="prerequisites" aria-label={textUI("m_b2fdf7a52389")}><p>{richUI("m_d9b72f36aee8", [formatUI(items[0]?.asset_name || textUI("m_a04e2c43801b"))])}</p><div className="report-relations">{formatUI(can(me, 'asset.read') && <Link to={`/assets/${encodeURIComponent(assetID)}`}>{textUI("m_fdeb90cab174")}</Link>)}<button className="btn btn--secondary btn--auto" onClick={() => { const next = new URLSearchParams(params); next.delete('asset_id'); setParams(next); }}>{textUI("m_8bd8ec88b961")}</button></div></div>)}
      <div className="toolbar" role="search">{formatUI(can(me, 'organization.read') && <OrganizationFilter value={organization} locked={!!organizationID} name={organizationName} onChange={value => { const next = new URLSearchParams(params); next.delete('organization_id'); if (value)
        next.set('org', value);
    else
        next.delete('org'); setParams(next); }}/>)}
        <label className="search-input">
          <Search size={15} aria-hidden="true"/>
          <input type="search" placeholder={textUI("m_fc3f375fe446")} aria-label={textUI("m_fc3f375fe446")} value={text} onChange={(e) => setText(e.target.value)}/>
        </label>
        <div className="filter">
          <select aria-label={textUI("m_51035b5b67dc")} value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">{textUI("m_f10988e79e8d")}</option><option value="open_port">{textUI("m_5c9e0324afba")}</option><option value="service">{textUI("m_d677190e0a99")}</option><option value="vulnerability">{textUI("m_a123dd728a7a")}</option>
          </select>
        </div>
        <div className="filter">
          <select aria-label={textUI("m_d44b62308969")} value={severity} onChange={(e) => setSeverity(e.target.value)}>
            <option value="">{textUI("m_222b823b4b06")}</option><option value="critical">{textUI("m_427dd2969bd1")}</option><option value="high">{textUI("m_c4ebc6d4a583")}</option><option value="medium">{textUI("m_8e588cd18774")}</option><option value="low">{textUI("m_f793de205ead")}</option><option value="info">{textUI("m_170322a32f3c")}</option>
          </select>
        </div>
      </div>
      {formatUI(!!organization && can(me, 'report.read') && <div className="org-card"><label className="select">{richUI("m_cff8ed38d138", [<select aria-label={textUI("m_7e4ef287d309")} value={comparisonMode ? 'comparison' : 'all'} onChange={e => setComparisonMode(e.target.value === 'comparison')}><option value="all">{textUI("m_417102d18176")}</option><option value="comparison">{textUI("m_0e4edebd2891")}</option></select>])}</label>{formatUI(comparisonMode && <>{formatUI(reportList.error || comparison.error ? <p role="alert">{formatUI(describeError(reportList.error || comparison.error).message)}</p> : reportList.isPending || !!latestID && comparison.isPending ? <p role="status">{textUI("m_71bfd8846532")}</p> : !latestID ? <p>{textUI("m_5d13849e66cb")}</p> : <><p>{richUI("m_eb210a614bb5", [formatUI(new Date(comparison.data!.created_at).toLocaleString(locale())), <Link to={`/reports/${latestID}`}>{textUI("m_9f2bda9a060f")}</Link>])}</p><ScanComparison comparison={comparison.data?.snapshot.comparison_details} search={q} severity={severity} type={type} assetID={assetID}/></>)}</>)}</div>)}
      {formatUI(!comparisonMode && (res.isPending ? (<ul className="cards" aria-label={textUI("m_3b06d3881b0f")}>{formatUI([0, 1, 2].map((i) => <li key={i} className="card"><span className="skeleton"/></li>))}</ul>) : res.isError ? (<div className="state state--error" role="alert">
          <h2>{textUI("m_8891e771bd5c")}</h2><p>{richUI("m_19636dc199da", [formatUI(describeError(res.error).message)])}</p>
          <button type="button" className="btn btn--secondary btn--auto" onClick={() => res.refetch()}>{textUI("m_d8b8392e2c54")}</button>
        </div>) : items.length === 0 ? (<div className="state">
          <h2>{formatUI(filtered ? textUI("m_991467873550") : textUI("m_3076f1ee8a92"))}</h2>
          <p>{formatUI(filtered ? textUI("m_fff0463f16e2") : can(me, 'organization.read') ? textUI("m_af431b0a42ac") : textUI("m_dd629c0d9e7e"))}</p>
          {formatUI(!filtered && can(me, 'organization.read') && <Link className="btn btn--secondary btn--auto" to="/organizations">{textUI("m_d78c44ecc1be")}</Link>)}
        </div>) : (<>
          {formatUI(grouped ? groupByOrganization(items).map((g) => (<OrganizationGroup key={g.key} id={g.id} name={g.name} linkable={can(me, 'organization.read')} summary={<>
              <span>{richUI("m_grp_n_findings", [formatUI(g.items.length)])}</span>
              {formatUI(SEVERITY_ORDER.map((sev) => { const n = g.items.filter((f) => f.severity === sev).length; return n ? <span key={sev} className="sev-count"><SeverityBadge severity={sev}/> {formatUI(n)}</span> : null; }))}
            </>}>
              <FindingRows list={bySeverity(g.items)} me={me}/>
            </OrganizationGroup>)) : <FindingRows list={items} me={me}/>)}
          {formatUI(res.hasNextPage && <div className="more"><button type="button" className="btn btn--secondary btn--auto" onClick={() => res.fetchNextPage()} disabled={res.isFetchingNextPage}>{formatUI(res.isFetchingNextPage ? textUI("m_ba3bbbe10d8b") : textUI("m_ac8991ef0101"))}</button></div>)}
        </>)))}
    </section>);
}
