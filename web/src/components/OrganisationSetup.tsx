import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
export type Readiness = {total:number;authorized:number;ready:number;items:{id:string;name:string;authorized:boolean;reason:string}[]};
export function useReadiness(id:string) {
 const {data:me}=useMe();
 return useQuery({queryKey:['organization-readiness',id],enabled:!!id&&can(me,'asset.read')&&can(me,'organization.read'),queryFn:()=>api.get<Readiness>(`/api/v1/organizations/${id}/readiness`),refetchInterval:30000});
}
export function setupSteps(v:{organization:boolean;assets:number;authorized:number;ready:number;assessments:number;reports:number}) {
 return [v.organization,v.assets>0,v.authorized>0&&v.ready>0,v.assessments>0,v.reports>0];
}
export function OrganisationSetup(){
 const {data:me}=useMe();const [selected,setSelected]=useState('');
 const orgs=useQuery({queryKey:['setup-organizations'],queryFn:()=>api.get<{items:{id:string;name:string}[];next_cursor:string|null}>('/api/v1/organizations?limit=100')});
 const id=selected||orgs.data?.items[0]?.id||'';const ready=useReadiness(id);
 const assessments=useQuery({queryKey:['setup-assessments',id],enabled:!!id&&can(me,'assessment.read'),queryFn:()=>api.get<{items:{id:string}[]}>(`/api/v1/organizations/${id}/assessments?limit=1`),refetchInterval:5000});
 const reports=useQuery({queryKey:['setup-reports',id],enabled:!!id&&can(me,'report.read')&&can(me,'finding.read')&&can(me,'asset.read')&&can(me,'assessment.read'),queryFn:()=>api.get<{items:{id:string;status:string}[]}>(`/api/v1/reports?organization_id=${id}&limit=1`),refetchInterval:5000});
 const v=ready.data;const latest=reports.data?.items[0];
 const done=setupSteps({organization:!!id,assets:v?.total??0,authorized:v?.authorized??0,ready:v?.ready??0,assessments:assessments.data?.items.length??0,reports:latest?.status==='ready'?1:0});
 const next=done.findIndex(d=>!d);const orgPath=id?`/organizations/${id}`:'/organizations';
 const steps=[
  {title:'Create organisation',text:'Keep one company’s assets and reports together.',to:orgPath,perm:'organization.write'},
  {title:'Install agents or add assets',text:'Agents register endpoints; you can also add targets manually.',to:id?orgPath:'/assets',perm:'asset.write'},
  {title:'Authorize testing and set addresses',text:'An administrator approves assets and enters reachable IPs or URLs. One ready target completes this step; others may still be skipped.',to:orgPath,perm:'asset.authorize'},
  {title:'Assess organisation',text:'Run Nmap and Nuclei together. Each skipped or failed asset is explained.',to:orgPath,perm:'assessment.run'},
  {title:'Read the report',text:latest?.status==='failed'?'PDF failed. Open the report to retry or export HTML, JSON or CSV.':'Review findings and download the organisation report.',to:latest?`/reports/${latest.id}`:'/reports',perm:'report.read'},
 ];
 const error=orgs.error||ready.error||assessments.error||reports.error;
 return <section className="panel" aria-labelledby="setup-title"><div className="setup-head"><div><p className="eyebrow">START HERE</p><h2 id="setup-title" className="panel__heading">Your organisation assessment in 5 steps</h2><p className="muted">Progress is based on the selected organisation. Open a completed step to review it.</p></div>{!!orgs.data?.items.length&&<div className="select"><label htmlFor="setup-org">Checklist organisation</label><select id="setup-org" value={id} onChange={e=>setSelected(e.target.value)}>{orgs.data.items.map(o=><option key={o.id} value={o.id}>{o.name}</option>)}</select></div>}</div>
 {orgs.isPending&&<p role="status">Loading organisation progress…</p>}{error&&<div className="alert" role="alert"><p>Progress could not be loaded: {describeError(error).message}</p><button className="link-quiet" onClick={()=>{void orgs.refetch();if(id){void ready.refetch();void assessments.refetch();void reports.refetch();}}}>Refresh progress</button></div>}
 <ol className="setup-list" aria-label="Organisation setup progress">{steps.map((s,i)=><li key={s.title} data-complete={done[i]} className={done[i]?'is-done':''} aria-current={i===next?'step':undefined}><span className="setup-number">{i+1} · {done[i]?'Complete':i===next?'Next step':'To do'}</span><Link to={s.to}>{s.title}</Link><p>{s.text}</p>{!done[i]&&!can(me,s.perm)&&<p>Ask an administrator{ i===3?' or analyst':''} to complete this step.</p>}</li>)}</ol>
 {orgs.data?.next_cursor&&<Link to="/organizations">More organisations are available in the organisation list.</Link>}
 <details className="setup-map"><summary>How the pieces fit together</summary><p>Organisation → assets → assessment → findings → one organisation report.</p><p>An agent registers a machine and runs local checks. A scan worker (injector) runs Nmap/Nuclei from a container and needs network access to the approved target address. Installing an agent does not grant testing permission.</p><p>For advanced workflows: a scenario is a saved plan; its chain connects actions and conditions. Launching it creates a simulation, a recorded run. A single test runs only one action on one asset.</p></details></section>;
}
