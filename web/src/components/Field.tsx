import { useLocale, fv as formatUI } from "../i18n/runtime";
import { useId, type InputHTMLAttributes, type ReactNode } from 'react';
type Props = InputHTMLAttributes<HTMLInputElement> & {
    label: string;
    hint?: string;
    error?: string;
    trailing?: ReactNode;
    leading?: ReactNode;
    labelRight?: ReactNode;
};
export function Field({ label, hint, error, trailing, leading, labelRight, className, ...input }: Props) {
    useLocale();
    const id = useId();
    const describedBy = error ? `${id}-err` : hint ? `${id}-hint` : undefined;
    return (<div className="field">
      <div className="field__label-row">
        <label htmlFor={id} className="field__label">
          {formatUI(label)}
        </label>
        {formatUI(labelRight)}
      </div>
      <div className={`field__input ${error ? 'is-error' : ''} ${className ?? ''}`}>
        {formatUI(leading && <span className="field__lead" aria-hidden="true">{formatUI(leading)}</span>)}
        <input id={id} aria-invalid={!!error} aria-describedby={describedBy} {...input}/>
        {formatUI(trailing)}
      </div>
      {formatUI(error ? (<p id={`${id}-err`} className="field__error" role="alert">
          {formatUI(error)}
        </p>) : hint ? (<p id={`${id}-hint`} className="field__hint">
          {formatUI(hint)}
        </p>) : null)}
    </div>);
}
