import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useId } from 'react';
import { Field } from '../components/Field';
import { type CategorySpec, NOTES_LABEL, NOTES_PLACEHOLDER } from './categorySchema';
// Kolom khusus kategori: isi disimpan di `attributes` aset; catatan di `description`.
export function CategoryFields({ spec, values, onChange, errors, hints }: {
    spec: CategorySpec;
    values: Record<string, string>;
    onChange: (key: string, value: string) => void;
    errors: Record<string, string>;
    hints?: Record<string, string | undefined>;
}) {
    useLocale();
    const id = useId();
    return (<>
      {formatUI(spec.fields.length > 0 && (<div className="category-fields">
        {formatUI(spec.fields.map((f) => f.kind === 'select'
            ? (<div className="select" key={f.key}>
                <label htmlFor={`${id}-${f.key}`}>{formatUI(f.label)}{formatUI(f.required ? ' *' : '')}</label>
                <select id={`${id}-${f.key}`} value={values[f.key] ?? ''} aria-invalid={!!errors[f.key]} onChange={(e) => onChange(f.key, e.target.value)}>
                  <option value="">{textUI("m_4895f73177ab")}</option>
                  {formatUI(f.options.map((o) => <option key={o.value} value={o.value}>{formatUI(o.label)}</option>))}
                </select>
                {formatUI(errors[f.key] && <small className="is-error" role="alert">{formatUI(errors[f.key])}</small>)}
              </div>)
            : (<Field key={f.key} label={f.label + (f.required ? ' *' : '')} value={values[f.key] ?? ''} placeholder={f.placeholder} hint={hints?.[f.key]} error={errors[f.key]} onChange={(e) => onChange(f.key, e.target.value)}/>)))}
      </div>))}
      <div className="textarea">
        <label htmlFor={`${id}-notes`}>{formatUI(NOTES_LABEL())}</label>
        <textarea id={`${id}-notes`} rows={2} value={values.__notes ?? ''} placeholder={NOTES_PLACEHOLDER()} onChange={(e) => onChange('__notes', e.target.value)}/>
      </div>
    </>);
}
// Validasi sisi klien yang menyebut kolomnya; server tetap memvalidasi ulang.
export function missingRequired(spec: CategorySpec, values: Record<string, string>): Record<string, string> {
    const out: Record<string, string> = {};
    for (const f of spec.fields) {
        if (f.required && !(values[f.key] ?? '').trim()) out[f.key] = f.kind === 'select' ? textUI("m_cf_err_choose", [f.label]) : textUI("m_cf_err_required", [f.label]);
    }
    return out;
}
export function attributesOf(spec: CategorySpec, values: Record<string, string>): Record<string, string> {
    const out: Record<string, string> = {};
    for (const f of spec.fields) {
        const v = (values[f.key] ?? '').trim();
        if (v) out[f.key] = v;
    }
    return out;
}
export function initialValues(spec: CategorySpec, attributes: Record<string, unknown> | undefined, description: string | null | undefined): Record<string, string> {
    const out: Record<string, string> = { __notes: description ?? '' };
    for (const f of spec.fields) {
        const v = attributes?.[f.key];
        if (typeof v === 'string') out[f.key] = v;
    }
    return out;
}
