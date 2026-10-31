import { ArrowRight, Eye, EyeOff } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { ApiError } from '../api/client';
import { useForgot, useLogin } from '../api/hooks';
import { RavenMark } from '../components/RavenMark';
import { Field } from '../components/Field';
import { Wordmark } from '../components/Wordmark';

function BrandPanel() {
  return (
    <aside className="auth__brand" aria-hidden="false">
      <div className="auth__brand-top">
        <Wordmark size={20} />
        <div className="pill">
          <span className="pill__dot" />
          All systems operational
        </div>
      </div>
      <div className="orbit" aria-hidden="true">
        <div className="orbit__glow" />
        <div className="orbit__ring orbit__ring--1" />
        <div className="orbit__ring orbit__ring--2" />
        <div className="orbit__ring orbit__ring--3" />
        <span className="orbit__dot orbit__dot--a" />
        <span className="orbit__dot orbit__dot--b" />
        <RavenMark className="orbit__raven" size={260} />
      </div>
      <div className="auth__copy">
        <h2 className="auth__headline">
          Watchful by nature.
          <br />
          Precise by design.
        </h2>
        <div className="auth__meta">
          <span className="auth__rule" />
          Admin console · Secure access only
        </div>
      </div>
    </aside>
  );
}

function Countdown({ seconds }: { seconds: number }) {
  const [left, setLeft] = useState(seconds);
  useEffect(() => {
    const t = setInterval(() => setLeft((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  const mm = String(Math.floor(left / 60)).padStart(2, '0');
  const ss = String(left % 60).padStart(2, '0');
  return (
    <div className="lock" role="timer" aria-live="off">
      <span className="lock__time">
        {mm}:{ss}
      </span>
      <span className="lock__unit">minutes remaining</span>
    </div>
  );
}

export function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="auth">
      <BrandPanel />
      <main className="auth__form-panel">
        <div className="auth__help">
          <span className="muted">Need access?</span>
          <strong>Contact your admin</strong>
        </div>
        <div className="auth__column">{children}</div>
        <footer className="auth__footer">
          <span>© 2026 ORCHIS</span>
          <span className="auth__footer-links">
            <span>Privacy</span>
            <span>Terms</span>
            <span>Status</span>
          </span>
        </footer>
      </main>
    </div>
  );
}

export function LoginPage() {
  const passwordWasReset = !!(useLocation().state as { reset?: boolean } | null)?.reset;
  const navigate = useNavigate();
  const login = useLogin();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [remember, setRemember] = useState(true);
  const [show, setShow] = useState(false);

  const err = login.error instanceof ApiError ? login.error : null;
  const locked = err?.status === 429;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    login.mutate({ email, password, remember_me: remember }, { onSuccess: () => navigate('/', { replace: true }) });
  };

  if (locked) {
    return (
      <AuthLayout>
        <div className="heading">
          <p className="eyebrow">ACCOUNT LOCKED</p>
          <h1 className="title">Too many attempts</h1>
          <p className="subtitle">For your security, sign-in is paused after 5 failed tries. You can try again in:</p>
        </div>
        <Countdown seconds={err?.retryAfter ?? 900} />
        <button type="button" className="btn btn--secondary" onClick={() => login.reset()}>
          Back to sign in
        </button>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout>
      <div className="heading">
        <p className="eyebrow">WELCOME BACK</p>
        <h1 className="title">Sign in to ORCHIS</h1>
        <p className="subtitle">Sign in to run authorized security checks for organisations and review their reports.</p>
      </div>
      <form className="form" onSubmit={submit} noValidate>
        {passwordWasReset && !err && (
          <div className="alert alert--ok" role="status">
            <strong>Password set</strong>
            <span>Sign in with your new password.</span>
          </div>
        )}
        {err && (
          <div className="alert" role="alert">
            <strong>{err.status === 401 ? 'Email or password is incorrect' : 'We could not sign you in'}</strong>
            <span>
              {err.status === 401
                ? 'Check both and try again.'
                : err.status >= 500
                  ? `The platform hit an unexpected error.${err.errorId ? ` Error id: ${err.errorId}` : ''}`
                  : 'Check your details and try again.'}
            </span>
          </div>
        )}
        <Field
          label="Email"
          type="email"
          name="email"
          autoComplete="username"
          placeholder="name@company.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          error={err?.fields.find((f) => f.name === 'email') ? 'Enter your email and password.' : undefined}
          autoFocus
        />
        <Field
          label="Password"
          type={show ? 'text' : 'password'}
          name="password"
          autoComplete="current-password"
          placeholder="Enter your password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          labelRight={
            <Link to="/forgot" className="link-subtle">
              Forgot password?
            </Link>
          }
          trailing={
            <button type="button" className="icon-btn" aria-label={show ? 'Hide password' : 'Show password'} onClick={() => setShow((s) => !s)}>
              {show ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          }
        />
        <label className="check">
          <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
          <span className="check__box" aria-hidden="true" />
          Keep me signed in for 30 days
        </label>
        <button type="submit" className="btn btn--primary" disabled={login.isPending}>
          {login.isPending ? 'Signing in…' : 'Sign in'}
          {!login.isPending && <ArrowRight size={18} aria-hidden="true" />}
        </button>
      </form>
    </AuthLayout>
  );
}

export function ForgotPage() {
  const forgot = useForgot();
  const [email, setEmail] = useState('');
  const submit = (e: FormEvent) => { e.preventDefault(); if (email.trim()) forgot.mutate(email.trim()); };
  if (forgot.isSuccess) {
    return (
      <AuthLayout>
        <div className="heading">
          <p className="eyebrow">ACCOUNT RECOVERY</p>
          <h1 className="title">Ask your administrator</h1>
          <p className="subtitle">
            If that email belongs to an account, your administrators have been told. They will send you a one-time link to choose a new password. The link works once and expires in 30 minutes.
          </p>
        </div>
        <Link to="/login" className="btn btn--secondary">Back to sign in</Link>
      </AuthLayout>
    );
  }
  return (
    <AuthLayout>
      <div className="heading">
        <p className="eyebrow">ACCOUNT RECOVERY</p>
        <h1 className="title">Reset your password</h1>
        <p className="subtitle">Enter your email. Password resets are handled by your administrator, who will send you a one-time link.</p>
      </div>
      <form className="form" onSubmit={submit} noValidate>
        {forgot.isError && <div className="alert" role="alert"><strong>We couldn’t send the request</strong><span>Check your connection and try again.</span></div>}
        <Field label="Email" type="email" name="email" autoComplete="username" placeholder="name@company.com" value={email} onChange={(e) => setEmail(e.target.value)} autoFocus />
        <button type="submit" className="btn btn--primary" disabled={forgot.isPending || !email.trim()}>{forgot.isPending ? 'Sending…' : 'Ask for a reset link'}</button>
      </form>
      <Link to="/login" className="btn btn--secondary">Back to sign in</Link>
    </AuthLayout>
  );
}
