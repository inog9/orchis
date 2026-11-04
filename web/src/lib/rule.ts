import { t as textUI } from "../i18n/runtime";
import type { Filter, Rule } from '../api/types';
export type FieldMeta = {
    label: string;
    operators: string[];
    options?: {
        value: string;
        label: string;
    }[];
    boolean?: boolean;
};
const opt = (vals: string[], labels?: Record<string, string>) => vals.map((v) => ({ value: v, label: labels?.[v] ?? v }));
const ENUM_OPS = ['eq', 'neq', 'in', 'not_in', 'is_empty', 'is_not_empty'];
// Harus sejalan dengan server (internal/groups/rule.go); server tetap memvalidasi ulang.
export const FIELDS: Record<string, FieldMeta> = {
    platform: { get label() {
            return textUI("m_c78ffe195710");
        }, operators: ENUM_OPS, options: opt(['windows', 'linux', 'macos', 'ios', 'android', 'other'], { get windows() {
                return textUI("m_d598026a9cbc");
            }, get linux() {
                return textUI("m_4828e60247c1");
            }, macos: 'macOS', ios: 'iOS', get android() {
                return textUI("m_6d612a86bee4");
            }, get other() {
                return textUI("m_f97e9da0e3b8");
            } }) },
    arch: { get label() {
            return textUI("m_cd74053c5481");
        }, operators: ENUM_OPS, options: opt(['x86_64', 'arm64', 'other']) },
    criticality: { get label() {
            return textUI("m_afdc3ea3ef86");
        }, operators: ENUM_OPS, options: opt(['low', 'medium', 'high', 'critical'], { get low() {
                return textUI("m_f793de205ead");
            }, get medium() {
                return textUI("m_8e588cd18774");
            }, get high() {
                return textUI("m_c4ebc6d4a583");
            }, get critical() {
                return textUI("m_427dd2969bd1");
            } }) },
    category: { get label() {
            return textUI("m_292c06f0045a");
        }, operators: ENUM_OPS, options: opt(['host', 'web_application', 'cloud_resource', 'container', 'network_device', 'mobile_device', 'iot_ot_device', 'identity', 'saas_application', 'ai_target', 'generic']) },
    tag: { get label() {
            return textUI("m_1503916a2ab2");
        }, operators: ENUM_OPS },
    hostname: { get label() {
            return textUI("m_2db533557011");
        }, operators: ['eq', 'contains', 'is_empty', 'is_not_empty'] },
    is_internet_facing: { get label() {
            return textUI("m_8cc5257478f0");
        }, operators: ['eq', 'neq'], boolean: true },
};
export const OPERATOR_LABEL: Record<string, string> = {
    eq: 'is',
    get neq() {
        return textUI("m_e1bf1560b1c2");
    },
    get in() {
        return textUI("m_2b39db5ba5ed");
    },
    get not_in() {
        return textUI("m_169b46113bf5");
    },
    contains: 'contains',
    get is_empty() {
        return textUI("m_86aaa00d0f04");
    },
    get is_not_empty() {
        return textUI("m_5a78c3a30610");
    },
};
export const takesNoValue = (op: string) => op === 'is_empty' || op === 'is_not_empty';
export const takesList = (op: string) => op === 'in' || op === 'not_in';
export function newFilter(field = 'platform'): Filter {
    const meta = FIELDS[field]!;
    const op = meta.operators[0]!;
    return { field, operator: op, values: meta.options ? [meta.options[0]!.value] : meta.boolean ? ['true'] : [''] };
}
// Mengganti kolom/operator mempertahankan apa yang masih berlaku dan membuang nilai yang tidak lagi sah.
export function changeField(f: Filter, field: string): Filter {
    return field === f.field ? f : newFilter(field);
}
export function changeOperator(f: Filter, operator: string): Filter {
    if (takesNoValue(operator))
        return { ...f, operator, values: [] };
    const meta = FIELDS[f.field]!;
    const first = f.values[0] ?? (meta.options?.[0]?.value ?? (meta.boolean ? 'true' : ''));
    return { ...f, operator, values: takesList(operator) ? (f.values.length ? f.values : [first]) : [first] };
}
export function describeFilter(f: Filter): string {
    const meta = FIELDS[f.field];
    const label = meta?.label ?? f.field;
    const op = OPERATOR_LABEL[f.operator] ?? f.operator;
    if (takesNoValue(f.operator))
        return `${label} ${op}`;
    const show = (v: string) => meta?.options?.find((o) => o.value === v)?.label ?? v;
    return `${label} ${op} ${f.values.map(show).join(', ')}`;
}
export function describeRule(r: Rule): string {
    if (r.filters.length === 0)
        return textUI("m_89adceb87c9c");
    return r.filters.map(describeFilter).join(r.mode === 'or' ? '  OR  ' : '  AND  ');
}
// Menyiapkan aturan untuk dikirim: buang spasi dan nilai kosong; mengembalikan pesan bila ada yang belum lengkap.
export function cleanRule(r: Rule): {
    rule: Rule;
    problem: string | null;
} {
    const filters: Filter[] = [];
    for (const f of r.filters) {
        const values = takesNoValue(f.operator) ? [] : f.values.map((v) => v.trim()).filter(Boolean);
        if (!takesNoValue(f.operator) && values.length === 0)
            return { rule: r, problem: textUI("m_d1bec567521e", [FIELDS[f.field]?.label ?? f.field]) };
        filters.push({ ...f, values });
    }
    return { rule: { mode: r.mode, filters }, problem: null };
}
