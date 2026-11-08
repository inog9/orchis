// Halaman Profil hasil redesain (mockup: /tmp/pen/profile.json dan /tmp/pen/profile.outline.txt).
// Sumber data nyata: GET /auth/me (useMe), GET /settings (useSettings) dan GET /me/tokens (useMyTokens).
// Endpoint diverifikasi langsung di backend sebelum dipakai:
//   - internal/settings/settings.go:118  GET  /api/v1/settings
//   - internal/users/http.go:151         GET    /api/v1/me/tokens
//   - internal/users/http.go:155         POST   /api/v1/me/tokens (buat token -> tombol "Buat token baru" boleh tampil)
//   - internal/users/http.go:166         DELETE /api/v1/me/tokens/{id} (revoke -> tombol cabut boleh tampil)
import { useState, type CSSProperties, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { X } from 'lucide-react';
import { language, t as textUI, useLocale, fv as formatUI } from '../i18n/runtime';
import { capitalLabel } from '../i18n/display';
import { ApiError } from '../api/client';
import { useChangePassword, useCreateToken, useLogout, useMe, useMyTokens, useRevokeToken, useSettings } from '../api/hooks';
import type { NewApiToken, PlatformSettings, User } from '../api/types';
import { CopyButton } from '../components/CopyButton';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { when } from '../components/RunBadges';
import { MetricCard, PanelCard, StateTag } from '../components/pen/primitives';
import { ConfirmDialog } from './ConfirmDialog';
import '../components/pen/pen.css';

// Batas chip izin sesuai spec: lebih dari 20 izin ditampilkan sebagai subset + "+N lainnya".
const MAX_PERMS_SHOWN = 20;

// Gaya tata letak lokal halaman ini; semua warna memakai token --v2-* (styles/ui2.css, dark + light).
const S = {
    stack: { display: 'flex', flexDirection: 'column', gap: 28 },
    head: { display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' },
    overline: { margin: 0, fontSize: 10, fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--v2-text-3)' },
    title: { margin: '5px 0 0', fontSize: 20, fontWeight: 500, letterSpacing: '-0.01em', color: 'var(--v2-text)' },
    desc: { margin: '4px 0 0', fontSize: 13, color: 'var(--v2-text-2)' },
    metrics: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))', gap: 12 },
    infoGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 16 },
    infoLabel: { margin: 0, fontSize: 11, color: 'var(--v2-text-3)' },
    infoValue: { margin: '2px 0 0', fontSize: 13, fontWeight: 500, color: 'var(--v2-text)', overflowWrap: 'anywhere' },
    secTitle: { margin: 0, fontSize: 15, fontWeight: 500, color: 'var(--v2-text)', display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' },
    secCount: { fontSize: 12, fontWeight: 400, color: 'var(--v2-text-3)' },
    secHead: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' },
    note: { margin: 0, fontSize: 13, color: 'var(--v2-text-3)' },
    noteError: { margin: 0, fontSize: 13, color: 'var(--v2-high)' },
    chips: { listStyle: 'none', display: 'flex', flexWrap: 'wrap', gap: 8, margin: 0, padding: 0 },
    chip: { display: 'inline-flex', alignItems: 'center', height: 26, padding: '0 10px', border: '1px solid var(--v2-border)', borderRadius: 6, fontSize: 12, color: 'var(--v2-text-2)' },
    more: { fontSize: 12, color: 'var(--v2-text-3)', alignSelf: 'center' },
    compsGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 },
    compCard: { border: '1px solid var(--v2-border)', borderRadius: 6, background: 'var(--v2-surface)', padding: '14px 16px', display: 'flex', flexDirection: 'column', gap: 8, minWidth: 0 },
    compTop: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 },
    compName: { margin: 0, fontSize: 13, fontWeight: 500, color: 'var(--v2-text)' },
    emptyBox: { border: '1px solid var(--v2-border)', borderRadius: 6, padding: '28px 16px', textAlign: 'center' },
    emptyText: { margin: 0, fontSize: 13, color: 'var(--v2-text-3)' },
    tokenList: { listStyle: 'none', margin: 0, padding: 0 },
    tokenMain: { minWidth: 0, display: 'flex', flexDirection: 'column', gap: 3 },
    tokenNameRow: { display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' },
    tokenName: { fontSize: 13, color: 'var(--v2-text)' },
    tokenPrefix: { fontSize: 12, color: 'var(--v2-text-3)' },
    tokenMeta: { fontSize: 12, color: 'var(--v2-text-3)' },
} as const satisfies Record<string, CSSProperties>;

// Dot status komponen: hijau hanya bila status "up" (sesuai spec), selain itu netral.
function dotStyle(up: boolean): CSSProperties {
    return { width: 7, height: 7, borderRadius: '50%', flex: 'none', background: up ? 'var(--v2-success)' : 'var(--v2-text-3)' };
}

// Baris daftar token: pemisah garis antar baris, tanpa garis pada baris terakhir.
function tokenRowStyle(last: boolean): CSSProperties {
    return { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '12px 0', borderBottom: last ? 'none' : '1px solid var(--v2-border)' };
}

// Bahasa UI efektif: preferensi akun bila ada, bila kosong pakai bahasa antarmuka aktif.
function langLabelOf(me: User): string {
    return (me.ui_language || language()).toUpperCase();
}

// Panel "Informasi akun": grid nama/email/role/bahasa UI/fitur lanjutan dari data useMe.
function AccountPanel({ me }: { me: User }) {
    useLocale();
    const rows = [
        { label: textUI('pen_prof_field_name'), value: me.display_name },
        { label: textUI('pen_prof_field_email'), value: me.email },
        { label: textUI('pen_prof_field_role'), value: me.role },
        { label: textUI('pen_prof_field_lang'), value: langLabelOf(me) },
        { label: textUI('pen_prof_field_advanced'), value: me.features?.advanced ? textUI('pen_prof_advanced_on') : textUI('pen_prof_advanced_off') },
    ];
    return (
        <PanelCard title={textUI('pen_prof_account_title')}>
            <div style={S.infoGrid}>
                {rows.map((r) => (
                    <div key={r.label}>
                        <p style={S.infoLabel}>{r.label}</p>
                        <p style={S.infoValue}>{r.value}</p>
                    </div>
                ))}
            </div>
        </PanelCard>
    );
}

// Panel ganti password: fitur akun yang tetap diperlukan meski tidak ada di mockup (dipulihkan dari versi sebelumnya).
function Password() {
    useLocale();
    const change = useChangePassword();
    const [cur, setCur] = useState('');
    const [next, setNext] = useState('');
    const err = change.error;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const submit = (e: FormEvent) => { e.preventDefault(); change.mutate({ current_password: cur, new_password: next }, { onSuccess: () => { setCur(''); setNext(''); } }); };
    return (<form className="panel" noValidate aria-label={textUI("m_3f9c991f63a9")} onSubmit={submit}>
      <h2 className="panel__heading">{textUI("m_e7cf3ef4f17c")}</h2>
      <Field label={textUI("m_72ed2bd767ce")} type="password" autoComplete="current-password" value={cur} onChange={(e) => { change.reset(); setCur(e.target.value); }} error={fieldErr('current_password')}/>
      <Field label={textUI("m_3dd9df4441fb")} type="password" autoComplete="new-password" value={next} onChange={(e) => { change.reset(); setNext(e.target.value); }} error={fieldErr('new_password')} hint={textUI("m_b8ec1f61313b")}/>
      {formatUI(err && !(err instanceof ApiError && err.status === 422) && <div className="alert" role="alert"><strong>{textUI("m_eed73dab8440")}</strong><span>{formatUI(describeError(err).message)}</span></div>)}
      {formatUI(change.isSuccess && <p className="muted small" role="status">{textUI("m_28043045d17a")}</p>)}
      <div><button type="submit" className="btn btn--primary btn--auto" disabled={change.isPending || !cur || !next}>{formatUI(change.isPending ? textUI("m_92a0c89d7c37") : textUI("m_3f9c991f63a9"))}</button></div>
    </form>);
}

// Section "Izin akses": chip per permission; di atas batas hanya subset + ringkasan "+N lainnya".
function PermissionsSection({ perms }: { perms: string[] }) {
    useLocale();
    const shown = perms.slice(0, MAX_PERMS_SHOWN);
    const rest = perms.length - shown.length;
    return (
        <section aria-labelledby="pen-prof-perms-title">
            <h2 id="pen-prof-perms-title" style={S.secTitle}>
                {textUI('pen_prof_perms_title')}
                <span style={S.secCount}>{textUI('pen_prof_perms_count', [perms.length])}</span>
            </h2>
            {perms.length === 0 ? (
                <p style={S.note}>{textUI('pen_prof_perms_empty')}</p>
            ) : (
                <ul style={S.chips} aria-label={textUI('pen_prof_perms_title')}>
                    {shown.map((p) => <li key={p} style={S.chip}>{p}</li>)}
                    {rest > 0 && <li style={S.more}>{textUI('pen_prof_perms_more', [rest])}</li>}
                </ul>
            )}
        </section>
    );
}

// Kartu metrik "Status komponen": nilai "up/total" dari settings.components.
function ComponentsMetric({ comps }: { comps: PlatformSettings['components'] }) {
    useLocale();
    const up = comps.filter((c) => c.status === 'up').length;
    return (
        <MetricCard
            label={textUI('pen_prof_metric_components')}
            value={textUI('pen_prof_metric_components_value', [up, comps.length])}
            caption={up === comps.length ? textUI('pen_prof_cap_components_ok') : textUI('pen_prof_cap_components_issues', [comps.length - up])}
        />
    );
}

// Section "Status platform": kartu per komponen dengan nama + dot status (hijau bila "up").
function PlatformSection({ settings }: { settings: ReturnType<typeof useSettings> }) {
    useLocale();
    const statusLabel = (s: PlatformSettings['components'][number]['status']) =>
        s === 'up' ? textUI('pen_prof_status_up') : s === 'down' ? textUI('pen_prof_status_down') : textUI('pen_prof_status_not_configured');
    return (
        <section aria-labelledby="pen-prof-platform-title">
            <h2 id="pen-prof-platform-title" style={S.secTitle}>{textUI('pen_prof_platform_title')}</h2>
            {settings.isPending ? (
                <span className="skeleton" aria-hidden="true"/>
            ) : settings.isError ? (
                <p style={S.noteError} role="alert">{textUI('pen_prof_platform_error')}</p>
            ) : !settings.data || settings.data.components.length === 0 ? (
                <p style={S.note}>{textUI('pen_prof_platform_empty')}</p>
            ) : (
                <div style={S.compsGrid} aria-label={textUI('pen_prof_platform_title')}>
                    {settings.data.components.map((c, i) => (
                        <div style={S.compCard} key={`${c.name}-${i}`}>
                            <div style={S.compTop}>
                                <p style={S.compName}>{c.name}</p>
                                <span style={dotStyle(c.status === 'up')} aria-hidden="true"/>
                            </div>
                            <StateTag tone={c.status === 'up' ? 'success' : 'neutral'}>{statusLabel(c.status)}</StateTag>
                        </div>
                    ))}
                </div>
            )}
        </section>
    );
}

// Dialog buat token: nama + masa berlaku; rahasia ditampilkan sekali setelah dibuat.
function NewTokenDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (t: NewApiToken) => void }) {
    useLocale();
    const create = useCreateToken();
    const [name, setName] = useState('');
    const [days, setDays] = useState('90');
    const err = create.error;
    const fieldErr = err instanceof ApiError ? err.fields.find((f) => f.name === 'name')?.message : undefined;
    return (
        <Modal labelledBy="ntok-title" onClose={onClose} width={480}>
            <header className="dialog__head">
                <h2 id="ntok-title">{textUI('m_dc58d8b7248f')}</h2>
                <button type="button" className="icon-btn" aria-label={textUI('m_7d9eb7acb13e')} onClick={onClose}><X size={18} aria-hidden="true"/></button>
            </header>
            <form className="dialog__form" noValidate onSubmit={(e: FormEvent) => { e.preventDefault(); create.mutate({ name, expires_in_days: Number(days) || 90 }, { onSuccess: (t) => { onClose(); onCreated(t); } }); }}>
                <Field label={textUI('m_dcd1d5223f73')} value={name} onChange={(e) => { create.reset(); setName(e.target.value); }} error={fieldErr} placeholder={textUI('m_79601827b68e')} autoFocus/>
                <div className="select">
                    <label htmlFor="pen-prof-tok-days">{textUI('m_a5e4b9f547aa')}</label>
                    <select id="pen-prof-tok-days" value={days} onChange={(e) => setDays(e.target.value)}>
                        <option value="30">{textUI('m_ffd7280513ce')}</option>
                        <option value="90">{textUI('m_38825b0f646b')}</option>
                        <option value="365">{textUI('m_91647badc37b')}</option>
                    </select>
                </div>
                <p className="muted small">{textUI('m_72c1b3e9bdb0')}</p>
                {err && !(err instanceof ApiError && err.status === 422) && (
                    <div className="alert" role="alert"><strong>{textUI('m_7fb00f9af686')}</strong><span>{describeError(err).message}</span></div>
                )}
                <footer className="dialog__foot">
                    <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI('m_19766ed6ccb2')}</button>
                    <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending || !name.trim()}>{create.isPending ? textUI('m_c79ed9492e3c') : textUI('m_5d8e8e30bc4f')}</button>
                </footer>
            </form>
        </Modal>
    );
}

// Dialog rahasia token: hanya ditampilkan sekali, bisa disalin (mekanisme lama dipertahankan).
function SecretDialog({ secret, onClose }: { secret: NewApiToken; onClose: () => void }) {
    useLocale();
    return (
        <Modal labelledBy="stok-title" onClose={onClose} width={560}>
            <header className="dialog__head"><h2 id="stok-title">{textUI('m_f4d15ff00226')}</h2></header>
            <p className="subtitle">{textUI('m_080ec17bf7fe')}</p>
            <div className="secret"><code className="mono" data-testid="api-token">{secret.secret}</code><CopyButton value={secret.secret}/></div>
            <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>{textUI('m_11a6767d5674')}</button></footer>
        </Modal>
    );
}

// Section "Token API": daftar nama + waktu buat + pemakaian terakhir, buat token, dan revoke.
function TokensSection() {
    useLocale();
    const q = useMyTokens();
    const revoke = useRevokeToken();
    const [creating, setCreating] = useState(false);
    const [secret, setSecret] = useState<NewApiToken | null>(null);
    const [revoking, setRevoking] = useState<{ id: string; name: string } | null>(null);
    return (
        <section aria-labelledby="pen-prof-tokens-title">
            <div style={S.secHead}>
                <h2 id="pen-prof-tokens-title" style={S.secTitle}>{textUI('m_fa884c0e7a4a')}</h2>
                {/* Endpoint POST /api/v1/me/tokens benar-benar ada (internal/users/http.go:155), jadi tombol ini tampil. */}
                <button type="button" className="btn btn--primary btn--auto" onClick={() => setCreating(true)}>{textUI('pen_prof_token_create')}</button>
            </div>
            {q.isPending ? (
                <span className="skeleton" aria-hidden="true"/>
            ) : q.isError ? (
                <p style={S.noteError} role="alert">{describeError(q.error).message}</p>
            ) : q.data.length === 0 ? (
                <div style={S.emptyBox}>
                    <p style={S.emptyText}>{textUI('m_ebb639f0445d')}</p>
                </div>
            ) : (
                <ul style={S.tokenList} aria-label={textUI('m_fa884c0e7a4a')}>
                    {q.data.map((tok, i) => (
                        <li key={tok.id} style={tokenRowStyle(i === q.data.length - 1)}>
                            <div style={S.tokenMain}>
                                <div style={S.tokenNameRow}>
                                    <strong style={S.tokenName}>{tok.name}</strong>
                                    {/* Hanya awalan yang tampil di daftar; rahasia tidak pernah dikirim ulang server. */}
                                    <code className="mono" style={S.tokenPrefix}>{tok.prefix}…</code>
                                </div>
                                <span style={S.tokenMeta}>
                                    {textUI('pen_prof_token_created_at', [when(tok.created_at)])}
                                    {' · '}
                                    {tok.last_used_at ? textUI('pen_prof_token_last_used', [when(tok.last_used_at)]) : textUI('m_dd57c2813b0b')}
                                </span>
                            </div>
                            {/* Token demo sintetis tidak bisa dicabut karena memang tidak bisa autentikasi. */}
                            {!tok.is_demo && (
                                <button type="button" className="btn btn--danger btn--sm" aria-label={textUI('m_02ff89306904', [tok.name])} onClick={() => setRevoking({ id: tok.id, name: tok.name })}>
                                    {textUI('m_87e6d00bbf53')}
                                </button>
                            )}
                        </li>
                    ))}
                </ul>
            )}
            {creating && <NewTokenDialog onClose={() => setCreating(false)} onCreated={setSecret}/>}
            {secret && <SecretDialog secret={secret} onClose={() => setSecret(null)}/>}
            {revoking && (
                <ConfirmDialog
                    title={textUI('m_fa6b2ee31f5b', [revoking.name])}
                    body={textUI('pen_prof_token_revoke_body')}
                    confirmLabel={textUI('pen_prof_token_revoke_action')}
                    busy={revoke.isPending}
                    error={revoke.isError ? describeError(revoke.error).message : undefined}
                    onConfirm={() => revoke.mutate(revoking.id, { onSuccess: () => setRevoking(null) })}
                    onClose={() => setRevoking(null)}
                />
            )}
        </section>
    );
}

// Isi halaman setelah data /auth/me tersedia; heading + aksi Keluar + metrik + semua section.
function ProfileBody({ me }: { me: User }) {
    useLocale();
    const navigate = useNavigate();
    const logout = useLogout();
    const settings = useSettings();
    const perms = me.permissions;
    return (
        <div style={S.stack}>
            <div style={S.head}>
                <div>
                    <p style={S.overline}>{textUI('pen_prof_overline')}</p>
                    <h1 id="profile-title" style={S.title}>{me.display_name}</h1>
                    <p style={S.desc}>{textUI('pen_prof_desc', [me.email, capitalLabel(me.role), langLabelOf(me)])}</p>
                </div>
                <div style={S.secHead}>
                    {/* Mekanisme logout sama seperti menu akun di Shell: POST /auth/logout lalu pindah ke /login. */}
                    <button
                        type="button"
                        className="btn btn--secondary btn--auto"
                        disabled={logout.isPending}
                        onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login', { replace: true }) })}
                    >
                        {textUI('pen_prof_logout')}
                    </button>
                </div>
            </div>
            <div style={S.metrics}>
                <MetricCard label={textUI('pen_prof_metric_role')} value={capitalLabel(me.role)} caption={textUI('pen_prof_cap_role')}/>
                <MetricCard label={textUI('pen_prof_metric_perms')} value={formatUI(perms.length)} caption={textUI('pen_prof_cap_perms')}/>
                {settings.data && settings.data.components.length > 0 && <ComponentsMetric comps={settings.data.components}/>}
                {settings.data && <MetricCard label={textUI('pen_prof_metric_version')} value={settings.data.version} caption={settings.data.platform_name}/>}
            </div>
            <AccountPanel me={me}/>
            <Password/>
            <PermissionsSection perms={perms}/>
            <PlatformSection settings={settings}/>
            <TokensSection/>
        </div>
    );
}

export function ProfilePage() {
    useLocale();
    const me = useMe();
    return (
        <section className="page" aria-label={textUI('pen_prof_overline')}>
            {me.isPending ? (
                <div role="status" aria-label={textUI('pen_prof_loading')} style={S.stack}>
                    <span className="skeleton" aria-hidden="true"/>
                    <span className="skeleton" aria-hidden="true"/>
                </div>
            ) : me.isError ? (
                <div className="state state--error" role="alert">
                    <h2>{textUI('pen_prof_load_error')}</h2>
                    <p>{textUI('m_19636dc199da', [describeError(me.error).message])}</p>
                    <button type="button" className="btn btn--secondary btn--auto" onClick={() => { void me.refetch(); }}>{textUI('m_d8b8392e2c54')}</button>
                </div>
            ) : me.data ? (
                <ProfileBody me={me.data}/>
            ) : (
                <p style={S.noteError} role="alert">{textUI('pen_prof_load_error')}</p>
            )}
        </section>
    );
}
