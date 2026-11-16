import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { ArrowRight, Eye, EyeOff, Lock, Mail, TriangleAlert } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { workspaceStart } from '../components/navigation';
import { ApiError } from '../api/client';
import { useForgot, useLogin } from '../api/hooks';
import { LoginHeroPanel } from '../components/login/LoginHeroPanel';
import { SupporterCredit } from '../components/SupporterCredit';
import { ThemeToggle } from '../components/ThemeToggle';
import './auth.css';
import { Field } from '../components/Field';
function Countdown({ seconds }: {
    seconds: number;
}) {
    useLocale();
    const [left, setLeft] = useState(seconds);
    useEffect(() => {
        const t = setInterval(() => setLeft((s) => Math.max(0, s - 1)), 1000);
        return () => clearInterval(t);
    }, []);
    const mm = String(Math.floor(left / 60)).padStart(2, '0');
    const ss = String(left % 60).padStart(2, '0');
    return (<div className="lock" role="timer" aria-live="off">
      <span className="lock__time">
        {formatUI(mm)}:{formatUI(ss)}
      </span>
      <span className="lock__unit">{textUI("m_2a9ecf780037")}</span>
    </div>);
}
export function AuthLayout({ children }: {
    children: React.ReactNode;
}) {
    useLocale();
    return (<div className="auth">
      <ThemeToggle className="auth__theme-toggle"/>
      <LoginHeroPanel />
      <main className="auth__form-panel">
        <div className="auth__help">
          <span className="muted">{textUI("m_0176cea22163")}</span>
          <strong>{textUI("m_a972cd2156be")}</strong>
        </div>
        <div className="auth__column">{formatUI(children)}<div className="auth__supporter"><SupporterCredit/></div></div>
        <footer className="auth__footer">
          <span>{textUI("m_c7163c58baae")}</span>
          <span className="auth__footer-links">
            <span>{textUI("m_54a57c3147c4")}</span>
            <span>{textUI("m_ede548996483")}</span>
            <span>{textUI("m_920e413c7d41")}</span>
          </span>
        </footer>
      </main>
    </div>);
}
export function LoginPage() {
    useLocale();
    const passwordWasReset = !!(useLocation().state as {
        reset?: boolean;
    } | null)?.reset;
    const navigate = useNavigate();
    const login = useLogin();
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [show, setShow] = useState(false);
    const [caps, setCaps] = useState(false);
    const err = login.error instanceof ApiError ? login.error : null;
    const locked = err?.status === 429;
    const submit = (e: FormEvent) => {
        e.preventDefault();
        login.mutate({ email, password, remember_me: false }, { onSuccess: user => navigate(workspaceStart(user), { replace: true }) });
    };
    if (locked) {
        return (<AuthLayout>
        <div className="heading">
          <p className="eyebrow">{textUI("m_84addceb0b64")}</p>
          <h1 className="title">{textUI("m_91434030e947")}</h1>
          <p className="subtitle">{textUI("m_c269514173cb")}</p>
        </div>
        <Countdown seconds={err?.retryAfter ?? 900}/>
        <button type="button" className="btn btn--secondary" onClick={() => login.reset()}>{textUI("m_71fe31ccd37c")}</button>
      </AuthLayout>);
    }
    return (<AuthLayout>
      <div className="heading">
        <p className="eyebrow">{textUI("m_d599547e30aa")}</p>
        <h1 className="title">{textUI("m_3d63b0352a07")}</h1>
        <p className="subtitle">{textUI("m_c851e95ed449")}</p>
      </div>
      <form className="form" onSubmit={submit} noValidate>
        {formatUI(passwordWasReset && !err && (<div className="alert alert--ok" role="status">
            <strong>{textUI("m_643e88355e6d")}</strong>
            <span>{textUI("m_7e4e2c31bb67")}</span>
          </div>))}
        {formatUI(err && (<div className="alert" role="alert">
            <strong>{formatUI(err.status === 401 ? textUI("m_22ed54d52adf") : textUI("m_7f0a399aeffe"))}</strong>
            <span>
              {formatUI(err.status === 401
                ? textUI("m_88c67ecae38f") : err.status >= 500
                ? textUI("m_ab3bd31a11e4", [err.errorId ? textUI("m_488d61c8380b", [err.errorId]) : '']) : textUI("m_3ea2033cd311"))}
            </span>
          </div>))}
        <Field label={textUI("m_969ccbd3cf63")} type="email" name="email" autoComplete="username" placeholder={textUI("m_35d973d92927")} value={email} onChange={(e) => setEmail(e.target.value)} error={err?.fields.find((f) => f.name === 'email') ? textUI("m_login_need_both") : undefined} leading={<Mail size={16}/>} autoFocus/>
        <Field label={textUI("m_e7cf3ef4f17c")} type={show ? 'text' : 'password'} name="password" autoComplete="current-password" placeholder={textUI("m_c06bf9670174")} value={password} onChange={(e) => setPassword(e.target.value)} leading={<Lock size={16}/>}
          onKeyDown={(e) => setCaps(e.getModifierState('CapsLock'))} onKeyUp={(e) => setCaps(e.getModifierState('CapsLock'))} onBlur={() => setCaps(false)}
          labelRight={<Link to="/forgot" className="link-subtle">
              {textUI("m_login_forgot")}
            </Link>} trailing={<button type="button" className="icon-btn" aria-label={show ? textUI("m_a60a56c584b3") : textUI("m_6aeaa6a53d09")} onClick={() => setShow((s) => !s)}>
              {show ? <EyeOff size={16}/> : <Eye size={16}/>}
            </button>}/>
        {formatUI(caps && <p className="auth__caps" role="status"><TriangleAlert size={14} aria-hidden="true"/>{textUI("m_login_caps")}</p>)}
        <button type="submit" className="btn btn--primary" disabled={login.isPending}>
          {formatUI(login.isPending && <span className="btn__spin" aria-hidden="true"/>)}
          {formatUI(login.isPending ? textUI("m_8b9d3de8ae00") : textUI("m_bfd402b2f6f3"))}
          {formatUI(!login.isPending && <ArrowRight size={18} aria-hidden="true"/>)}
        </button>
      </form>
    </AuthLayout>);
}
export function ForgotPage() {
    useLocale();
    const forgot = useForgot();
    const [email, setEmail] = useState('');
    const submit = (e: FormEvent) => {
        e.preventDefault();
        if (email.trim())
            forgot.mutate(email.trim());
    };
    if (forgot.isSuccess) {
        return (<AuthLayout>
        <div className="heading">
          <p className="eyebrow">{textUI("m_085e75abe3a2")}</p>
          <h1 className="title">{textUI("m_e29640055ae8")}</h1>
          <p className="subtitle">{textUI("m_e03958da8c30")}</p>
        </div>
        <Link to="/login" className="btn btn--secondary">{textUI("m_71fe31ccd37c")}</Link>
      </AuthLayout>);
    }
    return (<AuthLayout>
      <div className="heading">
        <p className="eyebrow">{textUI("m_085e75abe3a2")}</p>
        <h1 className="title">{textUI("m_59fa4379be1f")}</h1>
        <p className="subtitle">{textUI("m_1a610a167702")}</p>
      </div>
      <form className="form" onSubmit={submit} noValidate>
        {formatUI(forgot.isError && <div className="alert" role="alert"><strong>{textUI("m_119a896eb2f4")}</strong><span>{textUI("m_481859b689b0")}</span></div>)}
        <Field label={textUI("m_969ccbd3cf63")} type="email" name="email" autoComplete="username" placeholder={textUI("m_35d973d92927")} value={email} onChange={(e) => setEmail(e.target.value)} autoFocus/>
        <button type="submit" className="btn btn--primary" disabled={forgot.isPending || !email.trim()}>{formatUI(forgot.isPending ? textUI("m_b8ed5279e897") : textUI("m_569c7d75cb27"))}</button>
      </form>
      <Link to="/login" className="btn btn--secondary">{textUI("m_71fe31ccd37c")}</Link>
    </AuthLayout>);
}
