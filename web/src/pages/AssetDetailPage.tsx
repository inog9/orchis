// Detail aset hasil redesain (mockup: /tmp/pen/asset-detail.json).
// Data nyata: GET /assets/{id} (useAsset), GET /assets/{id}/endpoint-data (useAssetEndpoint),
// GET /findings?asset_id= (useFindings), GET /agents hanya untuk heartbeat (useAgents).
// Fungsionalitas lama tetap dipertahankan: ubah aset, hapus aset, checklist kesiapan
// (AssetReadiness), riwayat scan (AssetScanHistory), dan panel DefenseBot + hapus data
// endpoint (EndpointDataPanel) yang dirender apa adanya di bawah tabel ringkas.
import { Fragment, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { locale, t as textUI, tr as richUI, useLocale, fv as formatUI } from '../i18n/runtime';
import { capitalLabel } from '../i18n/display';
import { ApiError } from '../api/client';
import { useAsset, useDeleteAsset, useFindings, useMe } from '../api/hooks';
import { can, type Asset, type Finding, type User } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { TagChips } from '../components/TagPicker';
import { MetricCard, PanelCard, SeverityPill, StateTag } from '../components/pen/primitives';
import { relativeTime } from '../lib/relativeTime';
import { ConfirmDialog } from './ConfirmDialog';
import { EditAssetDialog } from './EditAssetDialog';
import { AssetReadiness } from '../components/AssetReadiness';
import { AssetScanHistory } from '../components/AssetScanHistory';
import { EndpointDataPanel } from '../components/EndpointDataPanel';
import { categoryOf, PLATFORM_LABEL } from './categories';
import { NOTES_LABEL, specOf } from './categorySchema';
import '../components/pen/pen.css';
// assets.css masih dibutuhkan untuk .ast-scans (riwayat scan) dan .status.
import './assets.css';
import './asset-detail.css';

const fmt = (iso: string) => new Date(iso).toLocaleString(locale(), { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });

// Urutan severity untuk tabel temuan: kritis dulu, info terakhir.
const SEV_RANK: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3, info: 4 };
// Batas baris temuan agar halaman tetap ringan; sisanya jadi counter + tautan (RULES).
const MAX_FINDING_ROWS = 10;

// Atribut string dari asset.attributes; nilai lain (angka/objek) diabaikan.
const attrStr = (a: Asset, key: string): string => {
    const v = a.attributes?.[key];
    return typeof v === 'string' && v.trim() !== '' ? v : '';
};

// Kolom keempat tabel temuan (query/bukti): hanya field details yang benar-benar ditulis
// backend, bukan tebakan. osquery menulis details.query (internal/endpoint/endpoint.go) dan
// details.rule (internal/defensebot/run.go), nuclei menulis template_id, Nmap menulis port.
function evidenceOf(f: Finding): string {
    const d = f.details ?? {};
    if (typeof d.query === 'string' && d.query !== '') return d.query;
    if (typeof d.rule === 'string' && d.rule !== '') return d.rule;
    if (typeof d.template_id === 'string' && d.template_id !== '') return d.template_id;
    if (d.port !== undefined) return [String(d.port), typeof d.protocol === 'string' ? d.protocol : ''].filter(Boolean).join('/');
    if (typeof d.package === 'string' && d.package !== '') return d.package;
    return f.type;
}

// Label status agent yang pendek untuk nilai metrik/grid.
const agentStateLabel = (state: Asset['agent_state']): string =>
    state === 'online' ? textUI('pen_ad_agent_online') : state === 'offline' ? textUI('pen_ad_agent_offline') : textUI('m_1f0d3f74b799');

// ---------- kepala halaman ----------

// Overline "ASET · {kategori}", judul, deskripsi platform/arch/OS/agent/sumber, aksi header.
function PageHead({ a, me, onEdit }: { a: Asset; me: User | null | undefined; onEdit: () => void }) {
    useLocale();
    const cat = categoryOf(a.category);
    const os = attrStr(a, 'os_version') || attrStr(a, 'agent_os_version');
    const desc = [
        a.platform ? (PLATFORM_LABEL[a.platform] ?? a.platform) : '',
        a.arch ?? '',
        os,
        a.agent_state === 'online' ? textUI('m_2d59e6737ad8') : a.agent_state === 'offline' ? textUI('m_8853f5f45fe6') : textUI('m_1f0d3f74b799'),
        textUI('pen_ad_desc_source', [a.source]),
    ].filter(Boolean).join(' · ');
    // Aksi pindai memakai wizard scan yang sudah ada; org diisi bila aset punya organisasi
    // (NewScanPage membaca ?org=). Tanpa organisasi atau izin, tombol tidak dirender.
    const canRunScan = !!a.organization_id && can(me, 'assessment.run') && can(me, 'simulation.run');
    return (
        <div className="pen-ad__head">
            <div className="pen-ad__headtext">
                <nav className="crumbs" aria-label={textUI('m_2bd873d6c734')}>
                    <Link to="/assets">{textUI('m_bd12731d7bc9')}</Link>
                    <ChevronRight size={12} aria-hidden="true" />
                    <span className="crumbs__current">{formatUI(a.name)}</span>
                </nav>
                <p className="pen-ad__overline">{richUI('pen_ad_overline', [cat.label])}</p>
                <h1 id="asset-title" className="pen-ad__title">{formatUI(a.name)}</h1>
                <p className="pen-ad__desc">{formatUI(desc)}</p>
                <TagChips tags={a.tags} />
            </div>
            <div className="pen-ad__actions">
                {formatUI(a.organization_id && (
                    <Link className="btn btn--secondary btn--auto" to={`/organizations/${a.organization_id}`}>{textUI('pen_ad_view_org')}</Link>
                ))}
                {formatUI(can(me, 'asset.write') && (
                    <button type="button" className="btn btn--secondary btn--auto" onClick={onEdit}>
                        {richUI('m_12a64771a7ec', [<Pencil size={15} aria-hidden="true" />])}
                    </button>
                ))}
                {formatUI(canRunScan && (
                    <Link className="btn btn--primary btn--auto" to={`/simulations/new?org=${encodeURIComponent(a.organization_id!)}`}>{textUI('pen_ad_run_scan')}</Link>
                ))}
            </div>
        </div>
    );
}

// ---------- strip metrik ----------

// Empat kartu: kritikalitas, status agent (dengan heartbeat), internet-facing, platform+arch.
function MetricStrip({ a, me }: { a: Asset; me: User | null | undefined }) {
    useLocale();
    const os = attrStr(a, 'os_version') || attrStr(a, 'agent_os_version');
    const platformValue = [a.platform ? (PLATFORM_LABEL[a.platform] ?? a.platform) : '', a.arch ?? ''].filter(Boolean).join(' ');
    return (
        <div className="pen-ad-metrics" role="group" aria-label={textUI('m_b6fead36007e')}>
            <MetricCard
                label={textUI('m_afdc3ea3ef86')}
                value={<span className={`pen-ad-sev pen-ad-sev--${a.criticality}`}>{capitalLabel(a.criticality)}</span>}
                caption={textUI('pen_ad_cap_crit')}
            />

            <MetricCard
                label={textUI('m_8cc5257478f0')}
                value={formatUI(a.is_internet_facing ? textUI('m_85a39ab345d6') : textUI('m_1ea442a134b2'))}
                caption={textUI(a.testing_authorized ? 'pen_ad_cap_facing_yes' : 'pen_ad_cap_facing_no')}
            />
            <MetricCard
                label={textUI('pen_ad_field_platform')}
                value={platformValue !== '' ? formatUI(platformValue) : '—'}
                caption={os || undefined}
            />
        </div>
    );
}

// ---------- panel detail aset ----------

// Satu field grid: label kecil di atas, nilai di bawah. mono untuk nilai teknis;
// catatan kecil (dari agent / belum diatur) tampil di bawah nilai bila ada.
function Field({ label, value, note, mono }: { label: string; value: ReactNode; note?: string; mono?: boolean }) {
    return (
        <div className="pen-ad-field">
            <p className="pen-ad-field__label">{label}</p>
            <p className={`pen-ad-field__value${mono ? ' mono' : ''}`}>{value}</p>
            {note !== undefined && note !== '' && <p className="pen-ad-field__note">{note}</p>}
        </div>
    );
}

// Panel "Detail aset": grid label/nilai + chip grup/tag + daftar alamat (mono).
function DetailPanel({ a }: { a: Asset }) {
    useLocale();
    const cat = categoryOf(a.category);
    const spec = specOf(a.category);
    // Field kategori (role, location, tech, provider, ...): perilaku lama dipertahankan,
    // nilai pilihan diterjemahkan lewat label opsinya; os_version punya fallback laporan
    // agent dengan keterangan "(from the agent)", yang kosong dijelaskan apinya (bukan diam).
    const specFields = spec.fields.map((f) => {
        const raw = a.attributes?.[f.key];
        const fromAgent = f.key === 'os_version' ? a.attributes?.agent_os_version : undefined;
        if (typeof raw === 'string' && raw !== '') {
            return { key: f.key, label: f.label, value: f.options.find((o) => o.value === raw)?.label ?? raw, note: '' };
        }
        if (typeof fromAgent === 'string' && fromAgent !== '') {
            return { key: f.key, label: f.label, value: fromAgent, note: textUI('m_attr_by_agent') };
        }
        return { key: f.key, label: f.label, value: '', note: textUI(f.key === 'os_version' && a.source === 'agent' ? 'm_attr_waiting_agent' : 'm_attr_not_set') };
    });
    return (
        <PanelCard title={textUI('pen_ad_detail_title')}>
            <div className="pen-ad-grid">
                <Field label={textUI('pen_ad_field_category')} value={formatUI(cat.label)} />
                {a.platform && <Field label={textUI('pen_ad_field_platform')} value={formatUI(PLATFORM_LABEL[a.platform] ?? a.platform)} />}
                {a.arch && <Field label={textUI('pen_ad_field_arch')} value={formatUI(a.arch)} mono />}
                <Field label={textUI('m_0e570ca6fabe')} value={formatUI(a.source)} mono />
                <Field label={textUI('pen_ad_field_agent')} value={formatUI(agentStateLabel(a.agent_state))} />
                <Field label={textUI('pen_ad_field_testing')} value={formatUI(a.testing_authorized ? textUI('m_85a39ab345d6') : textUI('m_1ea442a134b2'))} />
                <Field label={textUI('m_6b02e0d363a4')} value={formatUI(fmt(a.created_at))} />
                <Field label={textUI('m_3a5ecca188c0')} value={formatUI(fmt(a.updated_at))} />
                {specFields.map((f) => (f.value !== ''
                    ? <Field key={f.key} label={f.label} value={formatUI(f.value)} note={f.note} mono />
                    : <Field key={f.key} label={f.label} value={<span className="pen-ad-field__unset">{f.note}</span>} />))}
                {a.description && <Field label={NOTES_LABEL()} value={formatUI(a.description)} />}
            </div>
            {/* Catatan lama untuk aset agent: OS terisi otomatis, role/location diisi manual. */}
            {a.source === 'agent' && spec.fields.some((f) => f.key === 'os_version') && <p className="pen-ad-field__auto">{textUI('m_host_auto_note')}</p>}
            {(a.groups?.length ?? 0) > 0 && (
                <div className="pen-ad-row">
                    <span className="pen-ad-row__label">{textUI('m_39bbb719fa2b')}</span>
                    <span className="pen-ad-row__body">
                        {a.groups!.map((g) => (
                            <Fragment key={g.id}>
                                <Link className="pen-ad-chip" to={`/assets/groups/${g.id}`}>{formatUI(g.name)}</Link>
                                {g.is_system && <span className="pen-ad-chip__note">{textUI('m_5c73a5c73dc1')}</span>}
                            </Fragment>
                        ))}
                    </span>
                </div>
            )}
            {a.tags.length > 0 && (
                <div className="pen-ad-row">
                    <span className="pen-ad-row__label">{textUI('pen_ad_tags_label')}</span>
                    <span className="pen-ad-row__body"><TagChips tags={a.tags} /></span>
                </div>
            )}
            {a.addresses.length > 0 && (
                <div className="pen-ad-row pen-ad-row--top">
                    <span className="pen-ad-row__label">{textUI('m_76179aafc43c')}</span>
                    <ul className="pen-ad-addrs">
                        {a.addresses.map((ad) => (
                            <li key={ad.kind + ad.value}>
                                <span className="pen-ad-addrs__kind">{formatUI(ad.kind)}</span>
                                <span className="mono">{formatUI(ad.value)}</span>
                                {ad.is_primary && <span className="pen-ad-addrs__primary">{textUI('m_986a1b7135f4')}</span>}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
        </PanelCard>
    );
}

// ---------- section temuan ----------

// Tabel temuan dari GET /findings?asset_id=: severity, temuan, nilai, query/bukti.
// Hanya subset yang dirender; sisanya jadi counter dan tautan ke daftar lengkap.
function FindingsSection({ assetID }: { assetID: string }) {
    useLocale();
    const q = useFindings('', '', '', assetID);
    const items = useMemo(
        () => (q.data?.pages.flatMap((p) => p.items ?? []) ?? []).slice().sort((x, y) => (SEV_RANK[x.severity] ?? 9) - (SEV_RANK[y.severity] ?? 9)),
        [q.data],
    );
    const shown = items.slice(0, MAX_FINDING_ROWS);
    const extra = items.length - shown.length;
    return (
        <section className="pen-ad-sec" aria-labelledby="pen-ad-fnd-title">
            <h2 id="pen-ad-fnd-title" className="pen-ad-sec__title">{textUI('pen_ad_sec_findings')}</h2>
            <div className="pen-ad-tablewrap">
                {q.isPending ? (
                    <div className="pen-ad-skel pen-ad-skel--table" aria-hidden="true" />
                ) : q.isError ? (
                    <div className="pen-ad-sec__alert" role="alert">
                        <p>{formatUI(describeError(q.error).message)}</p>
                        <button type="button" className="btn btn--secondary btn--auto" onClick={() => void q.refetch()}>{textUI('m_d8b8392e2c54')}</button>
                    </div>
                ) : items.length === 0 ? (
                    <p className="pen-ad-sec__note">{textUI('m_cca407010ef1')}</p>
                ) : (
                    <>
                        <table className="pen-ad-table">
                            <thead>
                                <tr>
                                    <th scope="col">{textUI('pen_ad_col_severity')}</th>
                                    <th scope="col">{textUI('pen_ad_col_finding')}</th>
                                    <th scope="col">{textUI('pen_ad_col_value')}</th>
                                    <th scope="col">{textUI('pen_ad_col_query')}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {shown.map((f) => (
                                    <tr key={f.id}>
                                        <td><SeverityPill severity={f.severity}>{capitalLabel(f.severity)}</SeverityPill></td>
                                        <td><span className="pen-ad-table__title">{formatUI(f.title)}</span></td>
                                        <td className="mono pen-ad-table__mono">{formatUI(f.value)}</td>
                                        <td className="mono pen-ad-table__mono">{formatUI(evidenceOf(f))}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                        <p className="pen-ad-sec__meta">
                            {extra > 0 && <span>{textUI('pen_ad_fnd_more', [extra])}</span>}
                            <Link to={`/findings?asset_id=${encodeURIComponent(assetID)}`}>{textUI('m_9c6ec0c24bde')}</Link>
                        </p>
                    </>
                )}
            </div>
        </section>
    );
}

// ---------- halaman ----------

export function AssetDetailPage() {
    useLocale();
    const { id = '' } = useParams();
    const navigate = useNavigate();
    const { data: me } = useMe();
    const q = useAsset(id);
    const del = useDeleteAsset(id);
    const [editing, setEditing] = useState(false);
    const [confirming, setConfirming] = useState(false);
    if (q.isPending)
        return (
            <div className="page pen-ad" aria-busy="true">
                <p role="status" className="sr-only">{textUI('pen_ad_loading')}</p>
                <span className="pen-ad-skel pen-ad-skel--page" aria-hidden="true" />
                <div className="pen-ad-metrics" aria-hidden="true">
                    {[0, 1, 2, 3].map((i) => <span key={i} className="pen-ad-skel pen-ad-skel--card" />)}
                </div>
                <span className="pen-ad-skel pen-ad-skel--table" aria-hidden="true" />
            </div>
        );
    if (q.isError) {
        const notFound = q.error instanceof ApiError && q.error.status === 404;
        const d = describeError(q.error);
        return (<div className="state state--error" role="alert">
            <h2>{formatUI(notFound ? textUI("m_3965d1967e57") : textUI("m_cede667c6f14"))}</h2>
            <p>{formatUI(notFound ? textUI("m_50c2d179722c") : textUI("m_19636dc199da", [d.message]))}</p>
            {formatUI(d.errorId && !notFound && <code className="mono">{d.errorId}</code>)}
            <Link className="btn btn--secondary btn--auto" to="/assets">{textUI("m_089fb64fac74")}</Link>
        </div>);
    }
    const a = q.data;
    const canEndpoint = can(me, 'asset.read');
    return (<section className="page pen-ad" aria-labelledby="asset-title">
      <PageHead a={a} me={me} onEdit={() => setEditing(true)} />
      <MetricStrip a={a} me={me} />
      <DetailPanel a={a} />

      {/* Checklist kesiapan + tombol run per engine (aksi scan yang sudah ada di halaman ini). */}
      <AssetReadiness asset={a} />
      <AssetScanHistory assetID={a.id} organizationID={a.organization_id} allowed={can(me, 'simulation.read') && can(me, 'assessment.read')} />


      {/* Panel DefenseBot (riwayat run + hapus data) dipertahankan apa adanya. */}
      <EndpointDataPanel assetID={a.id} allowed={canEndpoint} canRun={false} authorized={a.testing_authorized} canDelete={can(me, 'asset.write') && can(me, 'settings.manage')} />

      {/* Tanpa izin baca temuan, section tetap tampil dengan catatan akses (bukan hilang). */}
      {can(me, 'finding.read')
        ? <FindingsSection assetID={a.id} />
        : (<section className="pen-ad-sec" aria-labelledby="pen-ad-fnd-title">
              <h2 id="pen-ad-fnd-title" className="pen-ad-sec__title">{textUI('pen_ad_sec_findings')}</h2>
              <p className="pen-ad-sec__note" role="status">{textUI('m_c06a30a9b145')}</p>
          </section>)}

      {formatUI(can(me, 'asset.delete') && (<section className="danger" aria-labelledby="danger-h">
          <div><h2 id="danger-h" className="panel__title">{textUI("m_fd8b8dae4421")}</h2><p className="muted">{textUI("m_128ede4e9e04")}</p></div>
          <button type="button" className="btn btn--danger btn--auto" onClick={() => setConfirming(true)}>{richUI("m_bb0aee048315", [<Trash2 size={15} aria-hidden="true" />])}</button>
        </section>))}

      {formatUI(editing && <EditAssetDialog asset={a} onClose={() => setEditing(false)} onStale={() => { setEditing(false); q.refetch(); }}/>)}
      {formatUI(confirming && (<ConfirmDialog title={textUI("m_9d64ce84e2c3", [a.name])} body="It will disappear from lists and groups. This can’t be undone from the console." confirmLabel="Remove asset" busy={del.isPending} error={del.isError ? describeError(del.error).message : undefined} onClose={() => setConfirming(false)} onConfirm={() => del.mutate(undefined, { onSuccess: () => navigate('/assets', { replace: true }) })}/>))}
    </section>);
}
