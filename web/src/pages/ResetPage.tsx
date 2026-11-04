import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useCheckReset, useReset } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { AuthLayout } from './LoginPage';
// Halaman publik: orang yang diberi link oleh admin memilih password-nya di sini.
export function ResetPage() {
    useLocale();
    const [params] = useSearchParams();
    const token = params.get('token') ?? '';
    const navigate = useNavigate();
    const check = useCheckReset(token);
    const reset = useReset();
    const [pw, setPw] = useState('');
    const [again, setAgain] = useState('');
    const mismatch = again !== '' && pw !== again;
    const err = reset.error;
    const fieldErr = err instanceof ApiError ? err.fields.find((f) => f.name === 'password')?.message : undefined;
    const submit = (e: FormEvent) => { e.preventDefault(); if (!mismatch)
        reset.mutate({ token, password: pw }, { onSuccess: () => navigate('/login', { replace: true, state: { reset: true } }) }); };
    const invalid = !token || check.isError;
    return (<AuthLayout>
      <div className="heading">
        <p className="eyebrow">{textUI("m_085e75abe3a2")}</p>
        <h1 className="title">{formatUI(invalid ? textUI("m_4ebbeadf3e3e") : textUI("m_833b8512168f"))}</h1>
        <p className="subtitle">{formatUI(invalid ? textUI("m_b692adb70de3") : textUI("m_2a1373448966"))}</p>
      </div>
      {formatUI(check.isPending && !!token ? <div className="splash" role="status" aria-label={textUI("m_b87fcbc969c6")}/> : invalid ? (<Link to="/login" className="btn btn--secondary">{textUI("m_71fe31ccd37c")}</Link>) : (<form className="form" onSubmit={submit} noValidate>
          <Field label={textUI("m_3dd9df4441fb")} type="password" autoComplete="new-password" value={pw} onChange={(e) => { reset.reset(); setPw(e.target.value); }} error={fieldErr} hint={textUI("m_b8ec1f61313b")} autoFocus/>
          <Field label={textUI("m_10aa5818e928")} type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} error={mismatch ? 'The passwords don’t match.' : undefined}/>
          {formatUI(err && !fieldErr && <div className="alert" role="alert"><strong>{textUI("m_7d153a660375")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
          <button type="submit" className="btn btn--primary" disabled={reset.isPending || !pw || !again}>{formatUI(reset.isPending ? textUI("m_23e39291d613") : textUI("m_4e4116871bf1"))}</button>
        </form>))}
    </AuthLayout>);
}
