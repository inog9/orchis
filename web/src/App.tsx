import { t as textUI, useLocale, fv as formatUI } from "./i18n/runtime";
import { advancedEnabled } from './components/navigation';
import { lazy, Suspense } from 'react';
import { SettingsPage } from './pages/SettingsPage';
import { NotificationsPage } from './pages/NotificationsPage';
import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { LandingPage } from './pages/LandingPage';
import { useMe } from './api/hooks';
import { can } from './api/types';
import { describeError } from './components/ErrorNote';
import { Shell } from './components/Shell';
const NewScanPage = lazy(() => import('./pages/ScansPage').then(m => ({ default: m.NewScanPage })));
const ScanDetailPage = lazy(() => import('./pages/ScansPage').then(m => ({ default: m.ScanDetailPage })));
const AuditPanel = lazy(() => import('./pages/AuditPanel').then(module => ({ default: module.AuditPanel })));
const AIPage = lazy(() => import('./pages/AIPage').then(module => ({ default: module.AIPage })));
const AIJobPage = lazy(() => import('./pages/AIPage').then(module => ({ default: module.AIJobPage })));
const ReportsPage = lazy(() => import('./pages/ReportsPage').then(module => ({ default: module.ReportsPage })));
const ReportDetailPage = lazy(() => import('./pages/ReportsPage').then(module => ({ default: module.ReportDetailPage })));
const OrganizationsPage = lazy(() => import('./pages/OrganizationsPage').then(module => ({ default: module.OrganizationsPage })));
const OrganizationDetailPage = lazy(() => import('./pages/OrganizationsPage').then(module => ({ default: module.OrganizationDetailPage })));
const AssessmentPage = lazy(() => import('./pages/OrganizationsPage').then(module => ({ default: module.AssessmentPage })));
const AgentsPage = lazy(() => import('./pages/AgentsPage').then(module => ({ default: module.AgentsPage })));
const ChainWorkspacePage = lazy(() => import('./pages/ChainWorkspacePage').then(module => ({ default: module.ChainWorkspacePage })));
const ChainsPage = lazy(() => import('./pages/ChainsPage').then(module => ({ default: module.ChainsPage })));
const ScenarioDetailPage = lazy(() => import('./pages/ScenarioDetailPage').then(module => ({ default: module.ScenarioDetailPage })));
const SimulationDetailPage = lazy(() => import('./pages/SimulationDetailPage').then(module => ({ default: module.SimulationDetailPage })));
const SimulationsPage = lazy(() => import('./pages/SimulationsPage').then(module => ({ default: module.SimulationsPage })));
const IntegrationsPanel = lazy(() => import('./pages/IntegrationsPanel').then(module => ({ default: module.IntegrationsPanel })));
const MitrePage = lazy(() => import('./pages/MitrePage').then(module => ({ default: module.MitrePage })));
const PeoplePage = lazy(() => import('./pages/PeoplePage').then(module => ({ default: module.PeoplePage })));
const PlatformPanel = lazy(() => import('./pages/PlatformPanel').then(module => ({ default: module.PlatformPanel })));
const ProfilePage = lazy(() => import('./pages/ProfilePage').then(module => ({ default: module.ProfilePage })));
const ResetPage = lazy(() => import('./pages/ResetPage').then(module => ({ default: module.ResetPage })));
const TagsPanel = lazy(() => import('./pages/TagsPanel').then(module => ({ default: module.TagsPanel })));
const RolesPanel = lazy(() => import('./pages/UsersPanel').then(module => ({ default: module.RolesPanel })));
const UsersPanel = lazy(() => import('./pages/UsersPanel').then(module => ({ default: module.UsersPanel })));
const AtomicTestDetailPage = lazy(() => import('./pages/AtomicTestDetailPage').then(module => ({ default: module.AtomicTestDetailPage })));
const AtomicTestsPage = lazy(() => import('./pages/AtomicTestsPage').then(module => ({ default: module.AtomicTestsPage })));
const FindingsPage = lazy(() => import('./pages/FindingsPage').then(module => ({ default: module.FindingsPage })));
const AssetDetailPage = lazy(() => import('./pages/AssetDetailPage').then(module => ({ default: module.AssetDetailPage })));
const AssetsPage = lazy(() => import('./pages/AssetsPage').then(module => ({ default: module.AssetsPage })));
const GroupDetailPage = lazy(() => import('./pages/GroupDetailPage').then(module => ({ default: module.GroupDetailPage })));
const GroupsPage = lazy(() => import('./pages/GroupsPage').then(module => ({ default: module.GroupsPage })));
const HomePage = lazy(() => import('./pages/HomePage').then(module => ({ default: module.HomePage })));
const NotFoundPage = lazy(() => import('./pages/HomePage').then(module => ({ default: module.NotFoundPage })));
const ForgotPage = lazy(() => import('./pages/LoginPage').then(module => ({ default: module.ForgotPage })));
const LoginPage = lazy(() => import('./pages/LoginPage').then(module => ({ default: module.LoginPage })));
function Guard({ children, guest = false }: {
    children: React.ReactNode;
    guest?: boolean;
}) {
    useLocale();
    const me = useMe();
    const loc = useLocation();
    if (me.isPending)
        return <div className="splash" role="status" aria-label={textUI("m_dc380888c4e2")}/>;
    if (me.isError) {
        return (<div className="fullpage" role="alert">
        <p className="eyebrow">500</p>
        <h1 className="title">{textUI("m_8128da536c8b")}</h1>
        <p className="subtitle">{formatUI(describeError(me.error).message)}</p>
        {formatUI(describeError(me.error).errorId && <code className="mono">{describeError(me.error).errorId}</code>)}
        <button type="button" className="btn btn--secondary btn--auto" onClick={() => window.location.reload()}>{textUI("ui_retry")}</button>
      </div>);
    }
    if (guest)
        return me.data ? <Navigate to="/" replace/> : <>{formatUI(children)}</>;
    return me.data ? <>{formatUI(children)}</> : <Navigate to="/login" replace state={{ from: loc.pathname }}/>;
}
// Flag presentasi saja: API tetap memeriksa permission, scope dan consent yang sama.
function Advanced({ children }: {
    children: React.ReactNode;
}) { useLocale(); const { data: me } = useMe(); return advancedEnabled(me) ? children : <Navigate to="/organizations" replace/>; }
export function App() {
    useLocale();
    return (<Suspense fallback={<div className="splash" role="status" aria-label={textUI("m_91bf928d3527")}/>}><Routes>
      <Route path="/login" element={<Guard guest><LoginPage /></Guard>}/>
      <Route path="/forgot" element={<Guard guest><ForgotPage /></Guard>}/>
      <Route path="/reset" element={<Guard guest><ResetPage /></Guard>}/>
      <Route path="/" element={<LandingPage />}/>
      <Route element={<Guard><Shell /></Guard>}>
        <Route path="overview" element={<HomePage />}/>
        <Route path="reports" element={<ReportsPage />}/>
        <Route path="reports/:id" element={<ReportDetailPage />}/>
        <Route path="organizations" element={<OrganizationsPage />}/>
        <Route path="organizations/:id/ai" element={<Advanced><AIPage /></Advanced>}/>
    <Route path="ai/jobs/:id" element={<Advanced><AIJobPage /></Advanced>}/>
    <Route path="organizations/:id" element={<OrganizationDetailPage />}/>
        <Route path="assessments/:id" element={<AssessmentPage />}/>
        <Route path="assets" element={<AssetsPage />}/>
        <Route path="assets/agents" element={<AgentsPage />}/>
        <Route path="assets/groups" element={<GroupsPage />}/>
        <Route path="assets/groups/:id" element={<GroupDetailPage />}/>
        <Route path="assets/:id" element={<AssetDetailPage />}/>
        <Route path="people" element={<PeoplePage />}/>
        <Route path="people/:tab" element={<PeoplePage />}/>
        <Route path="mitre" element={<Advanced><MitrePage /></Advanced>}/>
        <Route path="notifications" element={<NotificationsPage />}/>
        <Route path="profile" element={<ProfilePage />}/>
        <Route path="settings" element={<SettingsPage />}>
          <Route index element={<PlatformPanel />}/>
          <Route path="users" element={<UsersPanel />}/>
          <Route path="roles" element={<RolesPanel />}/>
          <Route path="integrations" element={<IntegrationsPanel />}/>
          <Route path="tags" element={<TagsPanel />}/>
          <Route path="audit" element={<AuditPanel />}/>
        </Route>
        <Route path="chains" element={<Advanced><ChainsPage /></Advanced>}/>
        <Route path="chains/:id" element={<Advanced><ChainWorkspacePage /></Advanced>}/>
        <Route path="chains/:id/setup" element={<Advanced><ScenarioDetailPage /></Advanced>}/>
        <Route path="simulations" element={<SimulationsPage />}/>
        <Route path="simulations/new" element={<NewScanPage />}/>
        <Route path="simulations/:id" element={<ScanDetailPage />}/>
        <Route path="simulations/legacy/:id" element={<SimulationDetailPage />}/>
        <Route path="atomic" element={<Advanced><AtomicTestsPage /></Advanced>}/>
        <Route path="atomic/:id" element={<Advanced><AtomicTestDetailPage /></Advanced>}/>
        <Route path="findings" element={<FindingsPage />}/>
        <Route path="*" element={<NotFoundPage />}/>
      </Route>
    </Routes></Suspense>);
}
