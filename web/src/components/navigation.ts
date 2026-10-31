import { BookOpen, Building2, FileText, Flag, House, Server, Settings, ShieldAlert, Workflow } from 'lucide-react';
import { can, type User } from '../api/types';

// Satu sumber label, tujuan, dan izin untuk navigasi serta palet perintah.
export const navigation = [
  { id: 'home', label: 'Home', hint: 'Follow your next step and review security exposure.', to: '/', group: 'Start', icon: House, keys: 'h', perm: '' },
  { id: 'organizations', label: 'Organisations', hint: 'Bring company assets together, run an assessment and open its report.', to: '/organizations', group: 'Start', icon: Building2, keys: 'o', perm: 'organization.read' },
  { id: 'assets', label: 'Assets', hint: 'Add machines, set target addresses and authorize testing; manage their agents.', to: '/assets', group: 'Start', icon: Server, keys: 'a', perm: 'asset.read' },
  { id: 'actions', label: 'Actions', hint: 'Choose one check and review your saved single-asset tests.', to: '/actions', group: 'Test', icon: BookOpen, keys: 'l', perm: 'atomic.read' },
  { id: 'scenarios', label: 'Scenarios', hint: 'Prepare a sequence of checks, choose targets and edit its chain.', to: '/scenarios', group: 'Test', icon: Workflow, keys: 's', perm: 'scenario.read' },
  { id: 'simulations', label: 'Simulations', hint: 'Follow launched scenarios and understand each step and decision.', to: '/simulations', group: 'Test', icon: Flag, keys: 'm', perm: 'simulation.read' },
  { id: 'findings', label: 'Findings', hint: 'Review vulnerabilities and open ports discovered on assets.', to: '/findings', group: 'Results', icon: ShieldAlert, keys: 'f', perm: 'finding.read' },
  { id: 'reports', label: 'Reports', hint: 'Read and export the fixed results of organisation assessments.', to: '/reports', group: 'Results', icon: FileText, keys: 'r', perm: 'report.read' },
  { id: 'settings', label: 'Settings', hint: 'Check scan workers, platform health, access and shared labels.', to: '/settings', group: 'Manage', icon: Settings, perm: '' },
];
export function navigationFor(me: User | null | undefined) { return navigation.filter(n => !n.perm || can(me, n.perm)); }
