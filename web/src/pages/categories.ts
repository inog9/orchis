import { AppWindow, Bot, Box, Cloud, Cpu, Globe, Router, Server, Shapes, Smartphone, UserRound, type LucideIcon } from 'lucide-react';

export const CATEGORIES: { key: string; label: string; description: string; icon: LucideIcon }[] = [
  { key: 'host', label: 'Host', description: 'Physical or virtual machine, server or workstation.', icon: Server },
  { key: 'web_application', label: 'Web application', description: 'Website, web API or web service reachable over HTTP(S).', icon: Globe },
  { key: 'cloud_resource', label: 'Cloud resource', description: 'Compute, storage, database, network or serverless cloud resource.', icon: Cloud },
  { key: 'container', label: 'Container / Workload', description: 'Container, image, Kubernetes object or serverless function.', icon: Box },
  { key: 'network_device', label: 'Network device', description: 'Router, switch, firewall, load balancer or other network gear.', icon: Router },
  { key: 'mobile_device', label: 'Mobile device', description: 'Smartphone or tablet (iOS / Android).', icon: Smartphone },
  { key: 'iot_ot_device', label: 'IoT / OT device', description: 'IoT sensor, camera, PLC, SCADA or other operational technology.', icon: Cpu },
  { key: 'identity', label: 'Identity', description: 'User, service account, group or role.', icon: UserRound },
  { key: 'saas_application', label: 'SaaS application', description: 'Software-as-a-Service application or tenant.', icon: AppWindow },
  { key: 'ai_target', label: 'AI target', description: 'LLM endpoint, AI agent, MCP server or RAG pipeline to red-team.', icon: Bot },
  { key: 'generic', label: 'Generic asset', description: 'Anything that does not fit another category yet.', icon: Shapes },
];

export const categoryOf = (key: string) => CATEGORIES.find((c) => c.key === key) ?? CATEGORIES[CATEGORIES.length - 1]!;

export const PLATFORM_LABEL: Record<string, string> = {
  windows: 'Windows', linux: 'Linux', macos: 'macOS', ios: 'iOS', android: 'Android', other: 'Other',
};

// Menebak jenis alamat dari teks; server tetap memvalidasi dan menormalkan.
export function inferKind(v: string): string {
  const s = v.trim();
  if (/^https?:\/\//i.test(s)) return 'url';
  if (/^([0-9a-f]{2}[:-]){5}[0-9a-f]{2}$/i.test(s)) return 'mac';
  if (/^[0-9a-f:.]+\/\d{1,3}$/i.test(s)) return 'cidr';
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(s) || (s.includes(':') && /^[0-9a-f:.]+$/i.test(s))) return 'ip';
  return s.includes('.') ? 'domain' : 'hostname';
}
