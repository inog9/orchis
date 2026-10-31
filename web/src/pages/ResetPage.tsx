import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useCheckReset, useReset } from '../api/hooks';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { AuthLayout } from './LoginPage';

// Halaman publik: orang yang diberi link oleh admin memilih password-nya di sini.
export function ResetPage() {
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
  const submit = (e: FormEvent) => { e.preventDefault(); if (!mismatch) reset.mutate({ token, password: pw }, { onSuccess: () => navigate('/login', { replace: true, state: { reset: true } }) }); };

  const invalid = !token || check.isError;
  return (
    <AuthLayout>
      <div className="heading">
        <p className="eyebrow">ACCOUNT RECOVERY</p>
        <h1 className="title">{invalid ? 'This link can’t be used' : 'Choose a new password'}</h1>
        <p className="subtitle">{invalid ? 'It may have expired, already been used, or been replaced by a newer one. Ask your administrator for a new link.' : 'Pick a password you haven’t used before. The link works once.'}</p>
      </div>
      {check.isPending && !!token ? <div className="splash" role="status" aria-label="Checking link" /> : invalid ? (
        <Link to="/login" className="btn btn--secondary">Back to sign in</Link>
      ) : (
        <form className="form" onSubmit={submit} noValidate>
          <Field label="New password" type="password" autoComplete="new-password" value={pw} onChange={(e) => { reset.reset(); setPw(e.target.value); }} error={fieldErr} hint="At least 12 characters, with upper and lower case letters and a digit or symbol." autoFocus />
          <Field label="Repeat the password" type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} error={mismatch ? 'The passwords don’t match.' : undefined} />
          {err && !fieldErr && <div className="alert" role="alert"><strong>Couldn’t set the password</strong><span>{describeError(err).message}</span></div>}
          <button type="submit" className="btn btn--primary" disabled={reset.isPending || !pw || !again}>{reset.isPending ? 'Saving…' : 'Set password'}</button>
        </form>
      )}
    </AuthLayout>
  );
}
