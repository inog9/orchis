import { Link, useLocation } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can } from '../api/types';

// Bantuan kontekstual juga berlaku pada tautan lama dan halaman detail.
export const guides = [
  {path:'/settings/users',purpose:'Manage the accounts that can sign in. Exercise participants are separate from platform users.',perm:'user.manage',next:'/people',label:'Exercise participants',nextPerm:'people.read'},
  {path:'/settings/roles',purpose:'Compare the fixed access rights of administrators, analysts and viewers.',perm:'user.manage',next:'/settings/users',label:'Manage accounts',nextPerm:'user.manage'},
  {path:'/settings/integrations',purpose:'Connect scan workers (injectors). Nmap and Nuclei run here, while agents collect information on endpoints.',perm:'integration.read',next:'/organizations',label:'Prepare an assessment',nextPerm:'organization.read'},
  {path:'/settings/tags',purpose:'Label assets so they are easier to find and group automatically.',perm:'asset.read',next:'/assets/groups',label:'Use labels in asset groups',nextPerm:'asset.read'},
  {path:'/assets/agents',purpose:'Manage endpoint agents. Installing an agent creates an asset; an administrator still sets its scan address and testing permission.',perm:'agent.read',next:'/organizations',label:'Install for an organisation',nextPerm:'organization.read'},
  {path:'/assets/groups',purpose:'Group assets using rules and labels; groups are separate from company organisations.',perm:'asset.read',next:'/organizations',label:'Organisation assets',nextPerm:'organization.read'},
  {path:'/organizations',purpose:'Organise a company’s assets, authorize checks, assess them together and receive one report.',perm:'organization.read',next:'/reports',label:'Organisation reports',nextPerm:'report.read'},
  {path:'/assessments',purpose:'Follow one organisation assessment. Each asset is checked separately; skipped or failed checks are explained below.',perm:'assessment.read',next:'/reports',label:'Read assessment reports',nextPerm:'report.read'},
  {path:'/reports',purpose:'Review a fixed snapshot of an organisation assessment. PDF generation can be retried without running scans again.',perm:'report.read',next:'/organizations',label:'Start from an organisation',nextPerm:'organization.read'},
  {path:'/assets',purpose:'Register test targets. Set an administrator-approved address and authorize testing before scanning.',perm:'asset.read',next:'/organizations',label:'Assign assets to an organisation',nextPerm:'organization.read'},
  {path:'/atomic',purpose:'Run one saved check against one asset and inspect each run’s findings. “Atomic” means one action at a time.',perm:'atomic.read',next:'/atomic',label:'Saved single checks',nextPerm:'atomic.read'},
  {path:'/chains',purpose:'Edit the action sequence of a scenario. Conditions choose branches; mapping passes validated results to the next action.',perm:'scenario.read',next:'/organizations',label:'Organisation scans',nextPerm:'organization.read'},
  {path:'/simulations',purpose:'Follow live scans, target and tool results, findings and reasons for failures or skipped checks.',perm:'simulation.read',next:'/organizations',label:'Choose an organisation',nextPerm:'organization.read'},
  {path:'/findings',purpose:'Review what scans observed on assets. A failed or skipped scan does not prove that an asset is safe.',perm:'finding.read',next:'/organizations',label:'Run checks for an organisation',nextPerm:'organization.read'},
  {path:'/people',purpose:'Manage exercise participants and teams. These records neither sign in nor authorize assets for testing.',perm:'people.read',next:'/organizations',label:'Company assets and assessments',nextPerm:'organization.read'},
  {path:'/mitre',purpose:'Explore the MITRE ATT&CK technique reference and see which techniques your checks cover.',perm:'scenario.read',next:'/organizations',label:'Organisation scans',nextPerm:'organization.read'},
  {path:'/notifications',purpose:'Review alerts about findings, failed scans and finished runs; open the related item to act.',perm:'alert.read',next:'/',label:'Return to your overview',nextPerm:''},
  {path:'/profile',purpose:'Update your account, password and personal API tokens. Tokens inherit your current role’s permissions.',perm:'',next:'/',label:'Return to your overview',nextPerm:''},
  {path:'/settings',purpose:'Check platform health and manage access, scan workers and labels. Start with Integrations when scans are waiting.',perm:'',next:'/settings/integrations',label:'Check scan workers',nextPerm:'integration.read'},
];
export function guideFor(path: string) { if(/^\/organizations\/[^/]+\/ai$/.test(path)||path.startsWith('/ai/jobs/')) return {path,purpose:'Review AI advice and proposed checks from a frozen report. External data sharing needs organisation consent; active checks need human approval.',perm:'ai.read',next:'/organizations',label:'Review organisation scope and consent',nextPerm:'organization.read'}; return guides.find(g=>path===g.path||path.startsWith(g.path+'/')); }
export function PageGuide() {
  const {pathname}=useLocation();const {data:me}=useMe();const g=guideFor(pathname);
  if(!g||g.perm&&!can(me,g.perm))return null;
  return <aside className="page-guide" aria-label="Page purpose"><p>{g.purpose}</p>{(!g.nextPerm||can(me,g.nextPerm))&&<Link to={g.next}>{g.label} →</Link>}
  </aside>;
}
export function PageAccess({children}:{children:React.ReactNode}) {
  const {pathname}=useLocation();const {data:me}=useMe();const g=guideFor(pathname);
  if(g?.perm&&!can(me,g.perm))return <section className="state"><h1 className="page__title">Access needed</h1><p>Your role cannot open this page. Ask an administrator for {g.perm} access, or return to your overview.</p><Link className="btn btn--secondary btn--auto" to="/">Go home</Link></section>;
  return children;
}
