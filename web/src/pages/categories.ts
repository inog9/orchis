import { t as textUI } from "../i18n/runtime";
import { AppWindow, Bot, Box, Cloud, Cpu, Globe, Router, Server, Shapes, Smartphone, UserRound, type LucideIcon } from 'lucide-react';
export const CATEGORIES: {
    key: string;
    label: string;
    description: string;
    icon: LucideIcon;
}[] = [
    { key: 'host', get label() {
            return textUI("m_4a823118b9ba");
        }, get description() {
            return textUI("m_8e12cbf2af83");
        }, icon: Server },
    { key: 'web_application', get label() {
            return textUI("m_5d2a9a87a829");
        }, get description() {
            return textUI("m_e3f45eac1bbe");
        }, icon: Globe },
    { key: 'cloud_resource', get label() {
            return textUI("m_703c49c576b3");
        }, get description() {
            return textUI("m_a9b0a4ce5daa");
        }, icon: Cloud },
    { key: 'container', get label() {
            return textUI("m_9bc6ca0ec802");
        }, get description() {
            return textUI("m_70b7bd1d42d0");
        }, icon: Box },
    { key: 'network_device', get label() {
            return textUI("m_700f434963f1");
        }, get description() {
            return textUI("m_c3755f79364e");
        }, icon: Router },
    { key: 'mobile_device', get label() {
            return textUI("m_76826d41272b");
        }, get description() {
            return textUI("m_8ff192a0ea3f");
        }, icon: Smartphone },
    { key: 'iot_ot_device', get label() {
            return textUI("m_9e9515bb63fb");
        }, get description() {
            return textUI("m_6ecf59587772");
        }, icon: Cpu },
    { key: 'identity', get label() {
            return textUI("m_999f23fcd7be");
        }, get description() {
            return textUI("m_9b2ccd077fb0");
        }, icon: UserRound },
    { key: 'saas_application', get label() {
            return textUI("m_885e9ddfdea3");
        }, get description() {
            return textUI("m_38e9a1704212");
        }, icon: AppWindow },
    { key: 'ai_target', label: textUI("m_bd0776274d9b"), description: textUI("m_54aa04ec2480"), icon: Bot },
    { key: 'generic', get label() {
            return textUI("m_1aa33abfb77a");
        }, get description() {
            return textUI("m_139c7c342f79");
        }, icon: Shapes },
];
export const categoryOf = (key: string) => CATEGORIES.find((c) => c.key === key) ?? CATEGORIES[CATEGORIES.length - 1]!;
export const PLATFORM_LABEL: Record<string, string> = {
    get windows() {
        return textUI("m_d598026a9cbc");
    },
    get linux() {
        return textUI("m_4828e60247c1");
    },
    macos: 'macOS', ios: 'iOS',
    get android() {
        return textUI("m_6d612a86bee4");
    },
    get other() {
        return textUI("m_f97e9da0e3b8");
    },
};
// Menebak jenis alamat dari teks; server tetap memvalidasi dan menormalkan.
export function inferKind(v: string): string {
    const s = v.trim();
    if (/^https?:\/\//i.test(s))
        return 'url';
    if (/^([0-9a-f]{2}[:-]){5}[0-9a-f]{2}$/i.test(s))
        return 'mac';
    if (/^[0-9a-f:.]+\/\d{1,3}$/i.test(s))
        return 'cidr';
    if (/^\d{1,3}(\.\d{1,3}){3}$/.test(s) || (s.includes(':') && /^[0-9a-f:.]+$/i.test(s)))
        return 'ip';
    return s.includes('.') ? 'domain' : 'hostname';
}
