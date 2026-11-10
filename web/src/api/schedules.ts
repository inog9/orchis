import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api } from './client';
import type { EngineKind } from '../lib/engines';
export type ScheduleRule = {frequency: 'daily'|'weekly'|'monthly'; timezone: string; time: string; weekday: number; month_day: number};
export type ScheduleConfig = {rule: ScheduleRule; kind: EngineKind; all_assets: boolean; asset_ids: string[]; language: 'en'|'id'; max_runtime_minutes: number; notify_changes: boolean; notify_gaps: boolean; weekly_digest: boolean};
export type AuditRun = {id: string; scheduled_at: string; status: string; scan_id: string|null; reason: string; summary: Partial<Record<'new'|'reappeared'|'persistent'|'resolved'|'uncompared'|'completed'|'total'|'attention',number>>; started_at: string|null; ended_at: string|null};
export type AuditSchedule = {id: string; name: string; organization_id: string; organization: string; config: ScheduleConfig; paused: boolean; next_run_at: string; next_runs: string[]; run_as: string; version: number; last_run?: AuditRun};
export type ScheduleInput = {request_key?: string; name: string; organization_id: string; config: ScheduleConfig; paused: boolean; version?: number};
type Page<T> = {items: T[]; next_cursor: string|null};
export function useSchedules(org: string,enabled: boolean) {
 return useInfiniteQuery({queryKey:['audit-schedules',org],enabled,initialPageParam:'',queryFn:({pageParam})=>api.get<Page<AuditSchedule>>(`/api/v1/audit-schedules?org=${encodeURIComponent(org)}&cursor=${pageParam}`),getNextPageParam:p=>p.next_cursor??undefined,refetchInterval:30000});
}
export function useSchedule(id: string,enabled: boolean) {
 return useQuery({queryKey:['audit-schedule',id],enabled,queryFn:()=>api.get<AuditSchedule>(`/api/v1/audit-schedules/${id}`),refetchInterval:15000});
}
export function useScheduleRuns(id: string,enabled: boolean) {
 return useInfiniteQuery({queryKey:['audit-schedule-runs',id],enabled,initialPageParam:'',queryFn:({pageParam})=>api.get<Page<AuditRun>>(`/api/v1/audit-schedules/${id}/runs?cursor=${pageParam}`),getNextPageParam:p=>p.next_cursor??undefined,refetchInterval:15000});
}
