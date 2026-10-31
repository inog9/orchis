import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api} from '../api/client';
import {useMe} from '../api/hooks';
import {can} from '../api/types';
import {describeError} from '../components/ErrorNote';
import './AuditPanel.css';

type Entry = {id:string;at:string;actor_user_id:string|null;actor:string;action:string;resource_type:string;resource_id:string|null;ip:string;request_id:string;reason:string};
type Page = {items:Entry[];next_cursor:string|null};
const labels:Record<string,string>={'auth.login':'Signed in','auth.logout':'Signed out','auth.login_failed':'Sign-in failed'};
export function AuditPanel(){
 const me=useMe();const allowed=can(me.data,'audit.read');
 const [action,setAction]=useState('');const [cursors,setCursors]=useState<string[]>([]);
 const cursor=cursors.at(-1)??'';
 const query=useQuery({queryKey:['audit',action,cursor],queryFn:()=>api.get<Page>('/api/v1/audit?'+new URLSearchParams({limit:'25',action,cursor})),enabled:allowed});
 if(me.isPending)return <span className="skeleton" role="status" aria-label="Loading audit permissions"/>;
 if(!allowed)return <div className="state"><h2>Audit access required</h2><p>Your role needs permission to read the audit log.</p></div>;
 return <section className="stack" aria-labelledby="audit-title">
  <div><h2 id="audit-title" className="panel__heading">Audit log</h2><p className="muted">Who changed what, and when. Unknown sign-in attempts show no guessed email; credentials and raw request data are excluded.</p></div>
  <div className="field"><label htmlFor="audit-action" className="field__label">Event type</label><select id="audit-action" className="audit-filter" value={action} onChange={event=>{setAction(event.target.value);setCursors([])}}><option value="">All events</option><option value="auth.login">Signed in</option><option value="auth.logout">Signed out</option><option value="auth.login_failed">Sign-in failed</option></select></div>
  {query.isPending?<span className="skeleton" role="status" aria-label="Loading audit log"/>:query.isError?<div className="state state--error" role="alert"><h3>Couldn’t load audit log</h3><p>{describeError(query.error).message}</p><button className="btn btn--secondary btn--auto" onClick={()=>query.refetch()}>Try again</button></div>:!query.data?.items.length?<div className="state"><h3>No events match</h3><p>Events appear here after sign-in and changes to the workspace.</p></div>:<ul className="audit-list" aria-label="Audit events">{query.data.items.map(entry=><li key={entry.id+entry.at} className="audit-entry">
   <div className="audit-entry__head"><strong>{labels[entry.action]??entry.action.replaceAll('.',' · ')}</strong><time dateTime={entry.at}>{new Date(entry.at).toLocaleString()}</time></div>
   <dl className="audit-entry__meta"><div><dt>Actor</dt><dd>{entry.actor||(entry.actor_user_id?'Former user':entry.action.startsWith('auth.')?'Unauthenticated':'System')}</dd></div><div><dt>Client IP</dt><dd>{entry.ip||'Not recorded'}</dd></div>{entry.resource_type&&<div><dt>Resource</dt><dd>{entry.resource_type}{entry.resource_id&&<> · <span className="mono">{entry.resource_id}</span></>}</dd></div>}{entry.reason&&<div><dt>Outcome</dt><dd>{entry.reason.replaceAll('_',' ')}</dd></div>}{entry.request_id&&<div><dt>Request</dt><dd className="mono">{entry.request_id}</dd></div>}</dl>
  </li>)}</ul>}
  <nav className="audit-pagination" aria-label="Audit pagination"><button className="btn btn--secondary btn--auto" disabled={!cursors.length||query.isFetching} onClick={()=>setCursors(value=>value.slice(0,-1))}>Previous</button><span className="muted small">Page {cursors.length+1}</span><button className="btn btn--secondary btn--auto" disabled={!query.data?.next_cursor||query.isFetching} onClick={()=>{if(query.data?.next_cursor)setCursors(value=>[...value,query.data!.next_cursor!])}}>Next</button></nav>
 </section>;
}
