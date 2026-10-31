import type { Filter, Rule } from '../api/types';

export type FieldMeta = { label: string; operators: string[]; options?: { value: string; label: string }[]; boolean?: boolean };

const opt = (vals: string[], labels?: Record<string, string>) => vals.map((v) => ({ value: v, label: labels?.[v] ?? v }));
const ENUM_OPS = ['eq', 'neq', 'in', 'not_in', 'is_empty', 'is_not_empty'];

// Harus sejalan dengan server (internal/groups/rule.go); server tetap memvalidasi ulang.
export const FIELDS: Record<string, FieldMeta> = {
  platform: { label: 'Platform', operators: ENUM_OPS, options: opt(['windows', 'linux', 'macos', 'ios', 'android', 'other'], { windows: 'Windows', linux: 'Linux', macos: 'macOS', ios: 'iOS', android: 'Android', other: 'Other' }) },
  arch: { label: 'Architecture', operators: ENUM_OPS, options: opt(['x86_64', 'arm64', 'other']) },
  criticality: { label: 'Criticality', operators: ENUM_OPS, options: opt(['low', 'medium', 'high', 'critical'], { low: 'Low', medium: 'Medium', high: 'High', critical: 'Critical' }) },
  category: { label: 'Category', operators: ENUM_OPS, options: opt(['host', 'web_application', 'cloud_resource', 'container', 'network_device', 'mobile_device', 'iot_ot_device', 'identity', 'saas_application', 'ai_target', 'generic']) },
  tag: { label: 'Tag', operators: ENUM_OPS },
  hostname: { label: 'Hostname', operators: ['eq', 'contains', 'is_empty', 'is_not_empty'] },
  is_internet_facing: { label: 'Internet-facing', operators: ['eq', 'neq'], boolean: true },
};

export const OPERATOR_LABEL: Record<string, string> = {
  eq: 'is', neq: 'is not', in: 'is any of', not_in: 'is none of', contains: 'contains', is_empty: 'is empty', is_not_empty: 'is not empty',
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
  if (takesNoValue(operator)) return { ...f, operator, values: [] };
  const meta = FIELDS[f.field]!;
  const first = f.values[0] ?? (meta.options?.[0]?.value ?? (meta.boolean ? 'true' : ''));
  return { ...f, operator, values: takesList(operator) ? (f.values.length ? f.values : [first]) : [first] };
}

export function describeFilter(f: Filter): string {
  const meta = FIELDS[f.field];
  const label = meta?.label ?? f.field;
  const op = OPERATOR_LABEL[f.operator] ?? f.operator;
  if (takesNoValue(f.operator)) return `${label} ${op}`;
  const show = (v: string) => meta?.options?.find((o) => o.value === v)?.label ?? v;
  return `${label} ${op} ${f.values.map(show).join(', ')}`;
}

export function describeRule(r: Rule): string {
  if (r.filters.length === 0) return 'Every asset';
  return r.filters.map(describeFilter).join(r.mode === 'or' ? '  OR  ' : '  AND  ');
}

// Menyiapkan aturan untuk dikirim: buang spasi dan nilai kosong; mengembalikan pesan bila ada yang belum lengkap.
export function cleanRule(r: Rule): { rule: Rule; problem: string | null } {
  const filters: Filter[] = [];
  for (const f of r.filters) {
    const values = takesNoValue(f.operator) ? [] : f.values.map((v) => v.trim()).filter(Boolean);
    if (!takesNoValue(f.operator) && values.length === 0) return { rule: r, problem: `Enter a value for “${FIELDS[f.field]?.label ?? f.field}”.` };
    filters.push({ ...f, values });
  }
  return { rule: { mode: r.mode, filters }, problem: null };
}
