import { t as textUI } from "../i18n/runtime";
import { CalendarClock, Building2, FileText, Flag, LayoutDashboard, Server, Settings, ShieldAlert } from 'lucide-react';
import { can, type User } from '../api/types';
// Workspace navigation; advanced features remain available through the palette.
export const navigation = [
    { id: 'overview', get label() {
            return textUI("pen_overview_nav");
        }, get hint() {
            return textUI("pen_overview_nav_hint");
        }, to: '/overview', get group() {
            return textUI("m_87bb59ba2f92");
        }, icon: LayoutDashboard },
    { id: 'organizations', get label() {
            return textUI("m_7bd34abba2ae");
        }, get hint() {
            return textUI("m_20b6a4cbbb46");
        }, to: '/organizations', get group() {
            return textUI("m_87bb59ba2f92");
        }, icon: Building2, keys: 'o', perm: 'organization.read' },
    { id: 'assets', get label() {
            return textUI("m_bd12731d7bc9");
        }, get hint() {
            return textUI("m_a331b8f605c4");
        }, to: '/assets', get group() {
            return textUI("m_87bb59ba2f92");
        }, icon: Server, keys: 'a', perm: 'asset.read' },
    { id: 'simulations', get label() {
            return textUI("flow_nav_scans");
        }, get hint() {
            return textUI("m_8ea1b108f3a2");
        }, to: '/simulations', get group() {
            return textUI("m_87bb59ba2f92");
        }, icon: Flag, keys: 'm', perm: 'simulation.read' },
    { id: 'schedules', get label(){return textUI('sched_title');}, get hint(){return textUI('sched_subtitle');}, to:'/simulations/schedules', get group(){return textUI("m_87bb59ba2f92");}, icon:CalendarClock, perm:'assessment.read' },
    { id: 'findings', get label() {
            return textUI("m_e171c2ff25b5");
        }, get hint() {
            return textUI("m_b751373ae214");
        }, to: '/findings', get group() {
            return textUI("m_87bb59ba2f92");
        }, icon: ShieldAlert, keys: 'f', perm: 'finding.read' },
    { id: 'reports', get label() {
            return textUI("m_dacca3cba3f3");
        }, get hint() {
            return textUI("m_d62c54575e79");
        }, to: '/reports', get group() {
            return textUI("m_87bb59ba2f92");
        }, icon: FileText, keys: 'r', perm: 'report.read' },
    { id: 'settings', get label() {
            return textUI("m_74a883a037bc");
        }, get hint() {
            return textUI("m_d3f9e520ee1d");
        }, to: '/settings', get group() {
            return textUI("m_5a23444828db");
        }, icon: Settings, perm: 'settings.manage' },
];
export function advancedEnabled(me: User | null | undefined) { return me?.features?.advanced === true; }
export function navigationFor(me: User | null | undefined) { return navigation.filter(n => (!n.perm || can(me, n.perm)) && (n.id !== 'schedules' || ['organization.read','asset.read','simulation.read','finding.read','report.read'].every(p=>can(me,p)))); }
