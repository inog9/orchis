import { useId, type InputHTMLAttributes, type ReactNode } from 'react';

type Props = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  hint?: string;
  error?: string;
  trailing?: ReactNode;
  labelRight?: ReactNode;
};

export function Field({ label, hint, error, trailing, labelRight, className, ...input }: Props) {
  const id = useId();
  const describedBy = error ? `${id}-err` : hint ? `${id}-hint` : undefined;
  return (
    <div className="field">
      <div className="field__label-row">
        <label htmlFor={id} className="field__label">
          {label}
        </label>
        {labelRight}
      </div>
      <div className={`field__input ${error ? 'is-error' : ''} ${className ?? ''}`}>
        <input id={id} aria-invalid={!!error} aria-describedby={describedBy} {...input} />
        {trailing}
      </div>
      {error ? (
        <p id={`${id}-err`} className="field__error" role="alert">
          {error}
        </p>
      ) : hint ? (
        <p id={`${id}-hint`} className="field__hint">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
