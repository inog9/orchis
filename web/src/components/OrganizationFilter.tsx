import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';

type Choice = { id: string; name: string; slug?: string };

// Filter yang sama dipakai halaman global dan tab organisasi. Kegagalan opsi tidak menghapus scope URL.
export function OrganizationFilter({value,onChange,locked=false,name,allowUnassigned=false}:{value:string;onChange:(value:string)=>void;locked?:boolean;name?:string;allowUnassigned?:boolean}) {
 const {data:me}=useMe();const [opened,setOpened]=useState(false);
 const q=useInfiniteQuery({queryKey:['organization-options'],enabled:!locked && can(me,'organization.read') && (opened || !!value),initialPageParam:'',queryFn:({pageParam})=>api.get<{items:Choice[];next_cursor:string|null}>(`/api/v1/organizations?limit=100&cursor=${encodeURIComponent(pageParam)}`),getNextPageParam:last=>last.next_cursor??undefined,retry:false});
 const items=q.data?.pages.flatMap(p=>p.items)??[];
 const selectedKnown=items.some(o=>o.id===value) || value==='' || value==='unassigned';
 return <div className="organization-filter"><label className="filter"><span className="sr-only">Organisation</span><select aria-label="Organisation" value={value} disabled={locked} onFocus={()=>setOpened(true)} onChange={e=>onChange(e.target.value)}>
 {!locked && <option value="">All organisations</option>}
 {!locked && allowUnassigned && <option value="unassigned">Unassigned</option>}
 {(!selectedKnown || locked) && <option value={value}>{name || 'Selected organisation'}</option>}
 {!locked && items.map(o=><option key={o.id} value={o.id}>{o.name}{o.slug?` · ${o.slug}`:''}</option>)}
 </select></label>{q.hasNextPage && <button type="button" className="btn btn--secondary btn--auto" disabled={q.isFetchingNextPage} onClick={()=>q.fetchNextPage()}>More organisations</button>}
 {q.isError && <p role="alert" className="small">Couldn’t load organisation options. {describeError(q.error).message} <button type="button" className="link-quiet" onClick={()=>q.refetch()}>Try again</button></p>}
 </div>;
}
