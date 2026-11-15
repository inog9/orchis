import { completionStatus } from '../lib/scanView';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowRight, CalendarClock, ChevronRight, RefreshCw, ShieldCheck } from 'lucide-react';
import { api } from '../api/client';
import { useHome, useMe } from '../api/hooks';
import { can, type Finding, type FindingPage, type HomeSummary, type User } from '../api/types';
import type { Scan } from '../api/scans';
import type { AuditSchedule } from '../api/schedules';
import { FindingDrawer } from '../components/findings/FindingDrawer';
import { SeverityPill, StateTag } from '../components/pen/primitives';
import { scanStatusLabel } from '../components/sims/StatusBadge';
import { agentDisplayName } from '../lib/engines';
import { defenseOf } from '../lib/findingsView';
import { relativeTime } from '../lib/relativeTime';
import { capitalLabel } from '../i18n/display';
import { t, locale, useLocale, fv } from '../i18n/runtime';
import { describeError } from '../components/ErrorNote';
import '../components/pen/pen.css';
import './findings.css';
import './overview.css';

const severities = ['critical','high','medium','low','info'] as const;
const canReadScans = (me: User|null|undefined) => ['organization.read','asset.read','assessment.read','simulation.read'].every(p=>can(me,p));
const canReadSchedules = (me: User|null|undefined) => canReadScans(me)&&can(me,'finding.read')&&can(me,'report.read');
const date = (iso: string, timezone?: string) => new Date(iso).toLocaleString(locale(),{dateStyle:'medium',timeStyle:'short',...(timezone?{timeZone:timezone}:{})});
function ErrorNote({error,retry}:{error:unknown;retry:()=>void}) {
 const e=describeError(error);
 return <div className="overview-error" role="alert"><p>{e.message}{e.errorId&&<code> · {e.errorId}</code>}</p><button className="link-quiet" onClick={retry}>{t('ui_retry')}</button></div>;
}
function Loading({label}:{label:string}){return <div className="overview-loading" role="status" aria-label={label}><span className="skeleton"/><span className="skeleton"/><span className="skeleton"/></div>;}

function SeveritySummary({findings}:{findings:NonNullable<HomeSummary['findings']>}){
 const sum=severities.reduce((n,s)=>n+(findings.by_severity[s]??0),0);
 return <section className="overview-severity" aria-labelledby="ov-severity-title"><div className="overview-section-head"><h2 id="ov-severity-title">{t('ov_severity_title')}</h2><span>{t('ov_recorded_hint')}</span></div>
  {sum>0&&<><div className="overview-severity__bar" aria-hidden="true">{severities.map(s=>(findings.by_severity[s]??0)>0&&<span key={s} className={`overview-tone--${s}`} style={{flexGrow:findings.by_severity[s]}}/>)}</div>
  <dl className="overview-severity__legend">{severities.map(s=><div key={s}><dt><i className={`overview-tone--${s}`} aria-hidden="true"/>{capitalLabel(s)}</dt><dd>{fv(findings.by_severity[s]??0)}</dd></div>)}</dl></>}
  {sum===0&&<p className="overview-caption">{t('ov_no_vulnerabilities')}</p>}
 </section>;
}
// Area temuan critical di Overview: web/aplikasi/jaringan dari scan, plus deteksi endpoint (jaringan, aplikasi, akun, agent).
function areaOf(type:string):string{
 switch(type){
  case 'new_listening_port':case 'c2_suspect_connection':case 'open_port':return t('ov_area_network');
  case 'new_software_installed':return t('ov_area_apps');
  case 'new_local_account':return t('ov_area_account');
  case 'agent_tamper':case 'agent_unresponsive':return t('ov_area_agent');
  default:return t('ov_area_vuln');
 }
}
function ReviewQueue({me}:{me:User|null|undefined}){
 const [open,setOpen]=useState<Finding|null>(null);
 const q=useQuery({queryKey:['overview','review-queue'],enabled:can(me,'finding.read'),queryFn:async()=>{
  // The API sorts by last seen. Separate severity filters ensure an older
  // critical finding cannot be displaced by a more recent lower-severity row.
  const page=await api.get<FindingPage>(`/api/v1/findings?severity=critical&triage=open&limit=5`);
  return page.items.filter(f=>f.severity==='critical').slice(0,5);
 },refetchInterval:30000,retry:false});
 return <section className="overview-review" aria-labelledby="ov-review-title"><header className="overview-section-head"><div><p className="overview-kicker">{t('ov_attention')}</p><h2 id="ov-review-title">{t('ov_review_title')}</h2><p>{t('ov_review_hint')}</p></div><Link to="/findings">{t('ov_all_findings')} <ArrowRight size={14}/></Link></header>
  {q.isPending?<Loading label={t('ov_loading_findings')}/>:q.isError?<ErrorNote error={q.error} retry={()=>q.refetch()}/>:q.data?.length===0?<div className="overview-empty"><ShieldCheck size={24} aria-hidden="true"/><h3>{t('ov_queue_empty')}</h3><p>{t('ov_queue_empty_hint')}</p></div>:<ol className="overview-findings">{q.data?.map(f=>{const d=defenseOf(f);return <li key={f.id} data-finding-id={f.id}><button className="overview-finding" onClick={()=>setOpen(f)} aria-haspopup="dialog"><span className="overview-finding__severity"><SeverityPill severity={f.severity}>{capitalLabel(f.severity)}</SeverityPill></span><span className="overview-finding__text"><strong>{f.title}</strong><span><em>{areaOf(f.type)}</em>{f.asset_name||t('ov_unassigned')}{d?.kev&&<em>{t('ov_exploited')}</em>}{d?.exposure==='exposed'&&<em>{t('ov_exposed')}</em>}</span></span><time dateTime={f.last_seen_at} title={date(f.last_seen_at)}>{relativeTime(f.last_seen_at)}</time><ChevronRight size={16} aria-hidden="true"/></button></li>})}</ol>}
  {open&&<FindingDrawer key={open.id} finding={open} onClose={()=>{setOpen(null);void q.refetch();}} canAsset={can(me,'asset.read')}/>}
 </section>;
}
function RecentAudits(){
 const q=useQuery({queryKey:['overview','recent-audits'],queryFn:()=>api.get<{items:Scan[]}>('/api/v1/scans'),refetchInterval:10000,retry:false});
 return <section className="overview-recent" aria-labelledby="ov-recent-title"><header className="overview-section-head"><div><h2 id="ov-recent-title">{t('ov_recent')}</h2><p>{t('ov_recent_hint')}</p></div><Link to="/simulations">{t('ov_history')} <ArrowRight size={14}/></Link></header>
  {q.isPending?<Loading label={t('ov_loading_scans')}/>:q.isError?<ErrorNote error={q.error} retry={()=>q.refetch()}/>:q.data?.items.length===0?<div className="overview-simple-empty"><p>{t('ov_no_audits')}</p></div>:<ul className="overview-audits">{q.data?.items.slice(0,4).map(s=><li key={s.id}><Link to={`/simulations/${s.id}`}><span className="overview-audit__name">{agentDisplayName(s.name)}</span><StateTag tone={completionStatus(s)==='finished'?'success':['failed','incomplete'].includes(completionStatus(s))?'warn':s.status==='running'?'accent':'neutral'}>{scanStatusLabel(completionStatus(s))}</StateTag><time dateTime={s.created_at} title={date(s.created_at)}>{relativeTime(s.created_at)}</time><ChevronRight size={14} aria-hidden="true"/></Link></li>)}</ul>}
 </section>;
}
function NextAudit({canRun}:{canRun:boolean}){
 const q=useQuery({queryKey:['overview','next-audit'],queryFn:async()=>{
  let cursor='';const seen=new Set<string>();const items:AuditSchedule[]=[];
  do{const page=await api.get<{items:AuditSchedule[];next_cursor:string|null}>(`/api/v1/audit-schedules?cursor=${encodeURIComponent(cursor)}`);items.push(...page.items);cursor=page.next_cursor??'';if(seen.has(cursor))throw Error('Repeated schedule cursor');seen.add(cursor);}while(cursor);
  return items.filter(s=>!s.paused&&Number.isFinite(Date.parse(s.next_run_at))).sort((a,b)=>Date.parse(a.next_run_at)-Date.parse(b.next_run_at))[0]??null;
 },refetchInterval:30000,retry:false});
 const s=q.data;
 return <section className="overview-next" aria-labelledby="ov-next-title"><header><CalendarClock size={19} aria-hidden="true"/><h2 id="ov-next-title">{t('ov_next')}</h2></header>
  {q.isPending?<Loading label={t('ov_loading_schedule')}/>:q.isError?<ErrorNote error={q.error} retry={()=>q.refetch()}/>:s?<><p className="overview-next__time">{date(s.next_run_at,s.config.rule.timezone)}</p><p className="overview-caption">{s.config.rule.timezone}</p><Link className="overview-next__name" to={`/simulations/schedules/${s.id}`}>{s.name}<ArrowRight size={14}/></Link><p className="overview-caption">{s.organization}</p></>:<><p className="overview-next__empty">{t('ov_no_schedule')}</p><p className="overview-caption">{t('ov_no_schedule_hint')}</p><Link className="overview-text-link" to={canRun?'/simulations/schedules?new=1':'/simulations/schedules'}>{canRun?t('ov_schedule'):t('sched_title')} <ArrowRight size={14}/></Link></>}
 </section>;
}
function AssetScope({assets}:{assets:NonNullable<HomeSummary['assets']>}){
 return <section className="overview-scope" aria-labelledby="ov-scope-title"><div className="overview-section-head"><h2 id="ov-scope-title">{t('ov_scope')}</h2><Link to="/assets" aria-label={t('ov_view_assets')}><ArrowRight size={16}/></Link></div>
  {assets.total>0&&<><p className="overview-scope__value">{fv(assets.authorized)}<span> / {fv(assets.total)}</span></p><p className="overview-caption">{t('ov_scope_caption')}</p>
  <div className="overview-scope__bar" role="progressbar" aria-label={t('ov_scope')} aria-valuemin={0} aria-valuemax={Math.max(assets.total,1)} aria-valuenow={assets.authorized} aria-valuetext={t('ov_scope_ratio',[assets.authorized,assets.total])}><span style={{width:`${assets.total?Math.min(100,assets.authorized/assets.total*100):0}%`}}/></div></>}
  {assets.total===0?<Link className="overview-text-link" to="/assets?new=1">{t('ov_add_assets')} <ArrowRight size={14}/></Link>:assets.authorized<assets.total?<p className="overview-scope__note">{t('ov_scope_pending')}</p>:null}
  <p className="overview-caption overview-scope__caveat">{t('ov_scope_hint')}</p>
 </section>;
}
function OverviewBody({h,me,busy,refresh}:{h:HomeSummary;me:User|null|undefined;busy:boolean;refresh:()=>void}){
 const canRun=canReadScans(me)&&can(me,'assessment.run')&&can(me,'simulation.run');
 const offline=h.injectors.filter(i=>i.status!=='online');
 return <>
  <header className="overview-head"><div><p className="overview-kicker">{t('ov_workspace')}</p><h1>{t('pen_ov_title')}</h1><p>{t('ov_subtitle')}</p></div><div className="overview-head__actions"><button className={`overview-refresh${busy?' is-busy':''}`} disabled={busy} onClick={refresh} aria-label={t('pen_ov_refresh')} title={t('pen_ov_refresh')}><RefreshCw size={16}/></button>{canRun&&h.assets?.total!==0&&<Link className="btn btn--primary btn--auto" to="/simulations/new">{t('ov_run_audit')} <ArrowRight size={14}/></Link>}</div></header>
  {h.findings&&<SeveritySummary findings={h.findings}/>}
  <div className="overview-layout"><div className="overview-main">{can(me,'finding.read')&&<ReviewQueue me={me}/>} {canReadScans(me)&&<RecentAudits/>}{!h.findings&&!canReadScans(me)&&<p className="overview-simple-empty">{t('ov_limited')}</p>}</div><aside className="overview-aside" aria-label={t('ov_workspace_context')}>{canReadSchedules(me)&&<NextAudit canRun={canRun}/>} {h.assets&&can(me,'asset.read')&&<AssetScope assets={h.assets}/>}</aside></div>
  <footer className="overview-footer"><p>{t('ov_updated',[date(h.generated_at)])}</p>{offline.length>0&&<p className="overview-engine-warning"><span>{t('ov_engine_warning',[offline.map(i=>agentDisplayName(i.name)).join(', ')])}</span>{can(me,'integration.read')&&<Link to="/settings/integrations">{t('ov_check_engines')} <ArrowRight size={13}/></Link>}</p>}</footer>
 </>;
}
export function HomePage(){
 useLocale();const {data:me}=useMe();const q=useHome();const qc=useQueryClient();
 return <section className="page overview" aria-label={t('pen_ov_title')}>{q.isPending?<Loading label={t('ov_loading')}/>:q.isError||!q.data?<div className="state state--error" role="alert"><h1>{t('ov_load_error')}</h1><ErrorNote error={q.error} retry={()=>q.refetch()}/></div>:<OverviewBody h={q.data} me={me} busy={q.isFetching} refresh={()=>{void q.refetch();void qc.invalidateQueries({queryKey:["overview"]});}}/>}</section>;
}
export function NotFoundPage(){useLocale();return <div className="fullpage"><p className="eyebrow">404</p><h1 className="title">{t('m_4427d9bbc643')}</h1><p className="subtitle">{t('m_8052556251cd')}</p><a className="btn btn--secondary btn--auto" href="/">{t('m_a0aac9145249')}</a></div>;}
