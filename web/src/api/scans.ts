import type {Comparison} from '../components/ScanComparison';
import { useQuery } from '@tanstack/react-query';
import { api } from './client';
export type ScanInput={language?:'en'|'id';organization_id:string;asset_ids:string[];range_ids?:string[];tools?:string[];preset?:'quick'|'standard'};
export type ScanFinding={id:string;type:string;value:string;title:string;severity:string;evidence:Record<string,unknown>};
export type ScanJob={id:string;asset_id:string|null;range_id:string|null;name:string;target:string;tool:string;status:string;error:string;summary?:string;phase?:string;disposition?:string;defense?:import('./defense').DefenseSummary;simulation_id:string|null;findings:ScanFinding[]};
export type Scan={report_id?:string;report_status?:string;summary?:string;limited_coverage?:boolean;results_expired_at?:string;comparison_details?:Comparison;id:string;organization_id:string;name:string;status:string;runner:'injector';options:ScanInput;created_at:string;ended_at:string|null;jobs?:ScanJob[]};
export function useScans(org='',cursor='',enabled=true){return useQuery({queryKey:['scans',org,cursor],enabled,queryFn:()=>api.get<{items:Scan[];next_cursor:string|null}>(`/api/v1/scans?org=${encodeURIComponent(org)}&cursor=${encodeURIComponent(cursor)}`),refetchInterval:3000});}
