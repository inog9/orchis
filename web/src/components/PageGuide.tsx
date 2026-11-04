import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { Link, useLocation } from 'react-router-dom';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
// Bantuan kontekstual juga berlaku pada tautan lama dan halaman detail.
export const guides = [
    { path: '/overview', get purpose() {
            return textUI("m_62c56a26be33");
        }, perm: 'organization.read', next: '/findings', get label() {
            return textUI("m_d3bfed0427b0");
        }, nextPerm: 'finding.read' },
    { path: '/settings/audit', get purpose() {
            return textUI("m_a2564cd75fb7");
        }, perm: 'audit.read', next: '/settings', get label() {
            return textUI("m_257cdfcd9bc0");
        }, nextPerm: 'settings.manage' },
    { path: '/settings/users', get purpose() {
            return textUI("m_7e06af6256dc");
        }, perm: 'user.manage', next: '/people', get label() {
            return textUI("m_c3fc4af91714");
        }, nextPerm: 'people.read' },
    { path: '/settings/roles', get purpose() {
            return textUI("m_38174b1e1250");
        }, perm: 'user.manage', next: '/settings/users', get label() {
            return textUI("m_daeaf38abf62");
        }, nextPerm: 'user.manage' },
    { path: '/settings/integrations', get purpose() {
            return textUI("m_328bbdd0a3da");
        }, perm: 'integration.read', next: '/organizations', get label() {
            return textUI("m_96c675840e6b");
        }, nextPerm: 'organization.read' },
    { path: '/settings/tags', get purpose() {
            return textUI("m_549a9f255478");
        }, perm: 'asset.read', next: '/assets/groups', get label() {
            return textUI("m_37f940335b1d");
        }, nextPerm: 'asset.read' },
    { path: '/assets/agents', get purpose() {
            return textUI("m_bd8d71a7d414");
        }, perm: 'agent.read', next: '/organizations', get label() {
            return textUI("m_3b14b94078ca");
        }, nextPerm: 'organization.read' },
    { path: '/assets/groups', get purpose() {
            return textUI("m_16d567a5d229");
        }, perm: 'asset.read', next: '/organizations', get label() {
            return textUI("m_80a5f49abcd1");
        }, nextPerm: 'organization.read' },
    { path: '/organizations', get purpose() {
            return textUI("m_a29ae67d5985");
        }, perm: 'organization.read', next: '/reports', get label() {
            return textUI("m_da8f8fed2930");
        }, nextPerm: 'report.read' },
    { path: '/assessments', get purpose() {
            return textUI("m_b4cd0b1671df");
        }, perm: 'assessment.read', next: '/reports', get label() {
            return textUI("m_b0a093a9adce");
        }, nextPerm: 'report.read' },
    { path: '/reports', get purpose() {
            return textUI("m_0026fe4565cb");
        }, perm: 'report.read', next: '/organizations', get label() {
            return textUI("m_20fea15eb935");
        }, nextPerm: 'organization.read' },
    { path: '/assets', get purpose() {
            return textUI("m_5463e4bded22");
        }, perm: 'asset.read', next: '/organizations', get label() {
            return textUI("m_c6e81b650b3e");
        }, nextPerm: 'organization.read' },
    { path: '/atomic', get purpose() {
            return textUI("m_6cd2fe61aff9");
        }, perm: 'atomic.read', next: '/atomic', get label() {
            return textUI("m_ccbb10848576");
        }, nextPerm: 'atomic.read' },
    { path: '/chains', get purpose() {
            return textUI("m_a5ef57e1418a");
        }, perm: 'scenario.read', next: '/organizations', get label() {
            return textUI("m_71869c5abd3f");
        }, nextPerm: 'organization.read' },
    { path: '/simulations', get purpose() {
            return textUI("m_0cb948bf9251");
        }, perm: 'simulation.read', next: '/organizations', get label() {
            return textUI("m_318de9854616");
        }, nextPerm: 'organization.read' },
    { path: '/findings', get purpose() {
            return textUI("m_3b4c8e4efc17");
        }, perm: 'finding.read', next: '/organizations', get label() {
            return textUI("m_3a7228f0e6af");
        }, nextPerm: 'organization.read' },
    { path: '/people', get purpose() {
            return textUI("m_df44129f76e3");
        }, perm: 'people.read', next: '/organizations', get label() {
            return textUI("m_80984d5011df");
        }, nextPerm: 'organization.read' },
    { path: '/mitre', get purpose() {
            return textUI("m_b6e6e19cf977");
        }, perm: 'scenario.read', next: '/organizations', get label() {
            return textUI("m_71869c5abd3f");
        }, nextPerm: 'organization.read' },
    { path: '/notifications', get purpose() {
            return textUI("m_ad4b2b9bad67");
        }, perm: 'alert.read', next: '/', get label() {
            return textUI("m_9a25a45a6df5");
        }, nextPerm: '' },
    { path: '/profile', get purpose() {
            return textUI("m_0eed5f91ab44");
        }, perm: '', next: '/', get label() {
            return textUI("m_9a25a45a6df5");
        }, nextPerm: '' },
    { path: '/settings', get purpose() {
            return textUI("m_2b0f82da592f");
        }, perm: 'settings.manage', next: '/settings/integrations', get label() {
            return textUI("m_ead504a77a76");
        }, nextPerm: 'integration.read' },
];
export function guideFor(path: string) { if (/^\/organizations\/[^/]+\/ai$/.test(path) || path.startsWith('/ai/jobs/'))
    return { path, purpose: textUI("m_c7f3ccedd57b"), perm: 'ai.read', next: '/organizations', label: textUI("m_dfc4813b10d2"), nextPerm: 'organization.read' }; return guides.find(g => path === g.path || path.startsWith(g.path + '/')); }
function guideAllowed(me: Parameters<typeof can>[0], g: ReturnType<typeof guideFor>) { return !g?.perm || can(me, g.perm) || g.path === '/notifications' && (can(me, 'finding.read') || can(me, 'report.read')); }
export function PageGuide() {
    useLocale();
    const { pathname } = useLocation();
    const { data: me } = useMe();
    const g = guideFor(pathname);
    if (!g || !guideAllowed(me, g))
        return null;
    return <aside className="page-guide" aria-label={textUI("m_b571ace9f5f5")}><p>{formatUI(g.purpose)}</p>{formatUI((!g.nextPerm || can(me, g.nextPerm)) && <Link to={g.next}>{formatUI(g.label)} →</Link>)}
  </aside>;
}
export function PageAccess({ children }: {
    children: React.ReactNode;
}) {
    useLocale();
    const { pathname } = useLocation();
    const { data: me } = useMe();
    const g = guideFor(pathname);
    if (!guideAllowed(me, g))
        return <section className="state"><h1 className="page__title">{textUI("m_f3e3d554f662")}</h1><p>{richUI("m_56e3ace5a57c", [formatUI(g?.perm)])}</p><Link className="btn btn--secondary btn--auto" to="/">{textUI("m_a0aac9145249")}</Link></section>;
    return children;
}
