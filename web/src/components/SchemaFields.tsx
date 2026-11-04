import {capitalLabel, displayLabel} from '../i18n/display';
import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useId } from 'react';
export type ParameterProperty = {
    type: 'string' | 'integer' | 'boolean' | 'array';
    label: string;
    help?: string;
    enum?: string[];
    enum_labels?: string[];
    items_enum?: string[];
    default?: unknown;
    show_when?: {
        field: string;
        value: string;
    };
    minimum?: number;
    maximum?: number;
    max_length?: number;
};
export type ParameterSchema = {
    type: 'object';
    properties: Record<string, ParameterProperty>;
    order: string[];
};
export function schemaDefaults(schema?: ParameterSchema): Record<string, unknown> { return Object.fromEntries(Object.entries(schema?.properties ?? {}).filter(([, p]) => p.default !== undefined).map(([key, p]) => [key, p.default])); }
export function SchemaFields({ schema, value, onChange, disabled = false, error = () => undefined }: {
    schema?: ParameterSchema;
    value: Record<string, unknown>;
    onChange: (v: Record<string, unknown>) => void;
    disabled?: boolean;
    error?: (key: string) => string | undefined;
}) {
    useLocale();
    const id = useId();
    if (!schema)
        return <p role="status" className="muted small">{textUI("m_27270e9fd34b")}</p>;
    if (!Object.keys(schema.properties).length)
        return <p className="muted small">{textUI("m_f8a69fcf3afe")}</p>;
    const values = { ...schemaDefaults(schema), ...value };
    const change = (key: string, v: unknown) => { const next = { ...values, [key]: v }; for (const [k, p] of Object.entries(schema.properties))
        if (p.show_when && next[p.show_when.field] !== p.show_when.value)
            delete next[k]; onChange(next); };
    return <>{formatUI(schema.order.map(key => {
            const p = schema.properties[key];
            if (!p || p.show_when && values[p.show_when.field] !== p.show_when.value)
                return null;
            const label = displayLabel(p.label);
            const fieldID = `${id}-${key}`;
            const err = error(`params.${key}`);
            const hintID = `${fieldID}-hint`;
            if (p.type === 'boolean')
                return <div key={key}><label className="check"><input type="checkbox" disabled={disabled} checked={!!values[key]} onChange={e => change(key, e.target.checked)}/><span className="check__box" aria-hidden="true"/><span>{formatUI(label)}{formatUI(p.help && <small className="check__sub">{formatUI(p.help && displayLabel(p.help))}</small>)}</span></label>{formatUI(err && <p role="alert">{formatUI(err)}</p>)}</div>;
            if (p.type === 'array' && p.items_enum) {
                const list = (values[key] as string[] | undefined) ?? [];
                return <fieldset className="fieldset" key={key}><legend>{formatUI(label)}</legend><div className="chips">{formatUI(p.items_enum.map(item => <button type="button" key={item} disabled={disabled} className={`chip ${list.includes(item) ? 'is-on' : ''}`} aria-pressed={list.includes(item)} onClick={() => change(key, list.includes(item) ? list.filter(s => s !== item) : [...list, item])}>{formatUI(capitalLabel(item))}</button>))}</div>{formatUI(err && <p className="field__error" role="alert">{formatUI(err)}</p>)}</fieldset>;
            }
            return <div className={p.enum ? 'select' : 'field'} key={key}><label htmlFor={fieldID}>{formatUI(label)}</label>{formatUI(p.enum ? <select id={fieldID} disabled={disabled} value={String(values[key] ?? '')} aria-describedby={hintID} onChange={e => change(key, e.target.value)}>{formatUI(p.enum.map((v, i) => <option key={v} value={v}>{formatUI(displayLabel(p.enum_labels?.[i] ?? v))}</option>))}</select> : <div className="field__input"><input id={fieldID} disabled={disabled} type={p.type === 'integer' ? 'number' : 'text'} min={p.minimum} max={p.maximum} maxLength={p.max_length} value={p.type === 'array' ? ((values[key] as string[] | undefined) ?? []).join(', ') : String(values[key] ?? '')} onChange={e => change(key, p.type === 'array' ? e.target.value.split(/[\s,]+/).filter(Boolean) : p.type === 'integer' ? Number(e.target.value) : e.target.value)} aria-invalid={!!err} aria-describedby={hintID}/></div>)}<small id={hintID} className={err ? 'field__error' : 'field__hint'} role={err ? 'alert' : undefined}>{formatUI(err || (p.help && displayLabel(p.help)))}</small></div>;
        }))}</>;
}
