import { Building2, FileText, Flag, Server, Settings, ShieldAlert } from 'lucide-react';
import { can, type User } from '../api/types';

// Menu utama tetap enam area; fitur lanjutan hanya tersedia melalui palet saat diaktifkan server.
export const navigation = [
  { id: 'organizations', label: 'Organisations', hint: 'Bring company assets together and review their scans and reports.', to: '/organizations', group: 'Workspace', icon: Building2, keys: 'o', perm: 'organization.read' },
  { id: 'simulations', label: 'Simulations', hint: 'Start a scan and follow live target and tool results.', to: '/simulations', group: 'Workspace', icon: Flag, keys: 'm', perm: 'simulation.read' },
  { id: 'findings', label: 'Findings', hint: 'Review vulnerabilities and open ports by organisation.', to: '/findings', group: 'Workspace', icon: ShieldAlert, keys: 'f', perm: 'finding.read' },
  { id: 'assets', label: 'Assets', hint: 'Add machines, set target addresses and authorize testing.', to: '/assets', group: 'Workspace', icon: Server, keys: 'a', perm: 'asset.read' },
  { id: 'reports', label: 'Reports', hint: 'Read frozen organisation results and export PDF or JSON.', to: '/reports', group: 'Workspace', icon: FileText, keys: 'r', perm: 'report.read' },
  { id: 'settings', label: 'Settings', hint: 'Manage access, integrations, scan workers and platform settings.', to: '/settings', group: 'Manage', icon: Settings, perm: '' },
];
export function advancedEnabled(me: User | null | undefined) { return me?.features?.advanced === true; }
export function navigationFor(me: User | null | undefined) { return navigation.filter(n => !n.perm || can(me, n.perm)); }
