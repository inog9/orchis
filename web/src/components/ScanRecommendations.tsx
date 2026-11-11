import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { t, useLocale } from '../i18n/runtime';
import { describeError } from './ErrorNote';
import './recommendations.css';

type Finding = { id: string; title: string; severity: string };
type Review = {
 status: 'queued' | 'running' | 'ready' | 'failed' | 'not_needed';
 input: { coverage: string; omitted: number; findings: Finding[] };
 output?: { summary: string; items: { finding_id: string; priority: 'now' | 'next' | 'planned'; reason: string; action: string; validation: string }[] };
};

const statusLabels = {queued:'review_status_queued',running:'review_status_running',ready:'review_status_ready',failed:'review_status_failed',not_needed:'review_status_not_needed'} as const;
const priorityLabels = {now:'review_priority_now',next:'review_priority_next',planned:'review_priority_planned'} as const;

export function ScanRecommendations({ scanID }: { scanID: string }) {
 useLocale();
 const q = useQuery({queryKey:['scan-recommendations',scanID],queryFn:()=>api.get<{job:Review|null}>(`/api/v1/scans/${scanID}/recommendations`),refetchInterval:query=>{
  const job=query.state.data?.job;
  return job?.status==='queued'||job?.status==='running'?5000:false;
 }});
 const job=q.data?.job;
 return <section className="sims__panel scan-review" aria-labelledby="review-heading">
  <header className="scan-review__head"><div><p className="eyebrow">{t('review_eyebrow')}</p><h2 id="review-heading" className="sims__h">{t('review_title')}</h2></div>{job&&<span className="pen-tag">{t(statusLabels[job.status])}</span>}</header>
  {q.isPending?<p role="status">{t('review_loading')}</p>:q.isError?<p role="alert">{describeError(q.error).message} <button className="btn btn--secondary btn--auto" onClick={()=>q.refetch()}>{t('ui_retry')}</button></p>:!job?<p className="muted">{t('review_missing')}</p>:job.status==='queued'||job.status==='running'?<p role="status">{t(job.status==='queued'?'review_queued':'review_running')}</p>:job.status==='failed'?<p role="status">{t('review_failed')}</p>:job.status==='not_needed'?<p>{t('review_not_needed')}</p>:job.output&&<>
   <p className="scan-review__summary">{job.output.summary}</p>
   {job.input.coverage!=='finished'&&<p className="notice">{t('review_incomplete')}</p>}
   {job.input.omitted>0&&<p className="notice">{t('review_omitted',[job.input.findings.length,job.input.omitted])}</p>}
   <ol className="scan-review__items">{job.output.items.map(item=>{
    const finding=job.input.findings.find(f=>f.id===item.finding_id);
    return <li key={item.finding_id}>
     <div className="scan-review__finding"><span className={`scan-review__priority scan-review__priority--${item.priority}`}>{t(priorityLabels[item.priority])}</span><h3>{finding?.title}</h3></div>
     <p className="muted">{item.reason}</p><p>{item.action}</p>
     <details><summary>{t('review_validation')}</summary><p>{item.validation}</p></details>
    </li>;
   })}</ol>
  </>}
  <p className="scan-review__foot">{t('review_advisory')}</p>
 </section>;
}
