// Halaman Overview hasil redesain (mockup: /tmp/pen/overview.json).
// Sumber data nyata: GET /home (useHome), GET /simulations?status=running (useSimulations),
// GET /findings (useFindings + useSimulationFindings) dan GET /assets (useAllAssets) untuk
// tipe badge aset. Primitif visual wajib dari components/pen/primitives.
import { Fragment, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ChevronDown, ChevronRight, Crosshair, Globe, Plug, Radar, RefreshCw, Server, Shield, Zap } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { locale, t as textUI, tr as richUI, useLocale, fv as formatUI } from '../i18n/runtime';
import { capitalLabel, displayLabel } from '../i18n/display';
import { OrganisationSetup } from '../components/OrganisationSetup';
import { useAllAssets, useFindings, useHome, useMe, useSimulationFindings, useSimulations } from '../api/hooks';
import { can, type Asset, type Finding, type HomeSummary, type Simulation, type User } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { MetricCard, PanelCard, SeverityBar, SeverityPill, StateTag } from '../components/pen/primitives';
import { relativeTime } from '../lib/relativeTime';
import { defenseOf } from '../lib/findingsView';
import '../components/pen/pen.css';
import './overview.css';

// Batas jumlah node agar halaman tetap ringan; sisanya dirangkum sebagai counter (RULES).
const MAX_ASSET_NODES = 4;
const MAX_ENGINE_NODES = 4;
const MAX_FINDING_NODES = 12;
const MAX_LIVE_GRAPHS = 3;

// Ikon engine berdasarkan key injector bawaan (lihat internal/execution/service.go).
const ENGINE_ICONS: Record<string, LucideIcon> = { defensebot: Shield, hackbot: Crosshair, nmap: Radar, nuclei: Zap };

// Engine pemilik sebuah temuan dibaca dari field details yang memang ditulis backend,
// bukan ditebak dari judul: DefenseBot menulis "package" (internal/defensebot/run.go),
// jalur web pentest nuclei menulis "template_id" dan dilaporkan sebagai Hackbot,
// Nmap menulis "port" (internal/injector/parse.go). Selain itu: tidak dipetakan.
function engineKeyOf(finding: Finding): string {
    const d = finding.details ?? {};
    if (typeof d.package === 'string' && d.package !== '') return 'defensebot';
    if (typeof d.template_id === 'string' && d.template_id !== '') return 'hackbot';
    if (d.port !== undefined) return 'nmap';
    return '';
}

const SEV_RANK: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3, info: 4 };

// Urutkan temuan: severity tertinggi dulu, lalu yang terakhir terlihat.
function sortFindings(items: Finding[]): Finding[] {
    return [...items].sort((a, b) => {
        const rank = (SEV_RANK[a.severity] ?? 9) - (SEV_RANK[b.severity] ?? 9);
        if (rank !== 0) return rank;
        return Date.parse(b.last_seen_at || b.first_seen_at || '') - Date.parse(a.last_seen_at || a.first_seen_at || '');
    });
}

// Tanggal singkat sesuai bahasa aktif; string tidak valid mengembalikan ''.
function shortDate(iso: string | null | undefined): string {
    if (!iso) return '';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(locale(), { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
}

// Bukti pada node expanded: hanya field details yang benar-benar ada di data (tanpa mengarang).
function evidenceLines(finding: Finding): string[] {
    const d = finding.details ?? {};
    const lines: string[] = [];
    const def = defenseOf(finding);
    if (def) {
        // Temuan DefenseBot: paket terpasang dan versi perbaikan bila tersedia.
        const version = def.installed && def.fixed ? `${def.installed} → ${def.fixed}` : def.installed;
        const pkg = [def.pkg, version].filter(Boolean).join(' ');
        if (pkg) lines.push(textUI('pen_ov_pkg', [pkg]));
        if (def.status) lines.push(textUI('pen_ov_fix', [def.status]));
    } else if (typeof d.template_id === 'string' && d.template_id !== '') {
        // Temuan web pentest (nuclei): template dan lokasi kecocokan.
        lines.push(textUI('pen_ov_template', [d.template_id]));
        if (typeof d.matched_at === 'string' && d.matched_at !== '') lines.push(textUI('pen_ov_matched', [d.matched_at]));
    } else if (d.port !== undefined) {
        // Temuan Nmap: port/protokol dan layanan bila terdeteksi.
        lines.push(textUI('pen_ov_port_detail', [String(d.port), typeof d.protocol === 'string' ? d.protocol : '']));
        const product = [d.product, d.version].filter((v) => typeof v === 'string' && v !== '').join(' ');
        if (typeof d.service === 'string' && d.service !== '') lines.push(textUI('pen_ov_service_detail', [product ? `${d.service} ${product}` : d.service]));
    }
    return lines;
}

// Tingkat paparan diturunkan dari kerentanan yang ditemukan; bukan skor resmi, hanya ringkasan cepat.
export function exposureLevel(by: Record<string, number>, total: number): {
    label: string;
    tone: string;
} {
    if (total === 0)
        return { label: textUI("m_e2e18a767a2b"), tone: 'none' };
    if ((by.critical ?? 0) > 0)
        return { label: textUI("m_5e12b9290669"), tone: 'critical' };
    if ((by.high ?? 0) > 0)
        return { label: textUI("m_149a3ee3b92f"), tone: 'high' };
    if ((by.medium ?? 0) > 0)
        return { label: textUI("m_1894e0b14dfa"), tone: 'medium' };
    return { label: textUI("m_3d749c9d4d6b"), tone: 'low' };
}

// ---------- node diagram ----------

type AssetNodeData = { id: string; name: string; badge: string | null; icon: 'host' | 'web'; href: string };
type EngineNodeData = { key: string; name: string };

// Tipe badge aset dari data aset asli: HOST bila category="host", WEB bila aset punya
// URL web target (address kind "url") atau memang kategori web_application.
function assetKind(a: Asset | undefined): { badge: string | null; icon: 'host' | 'web' } {
    if (!a) return { badge: null, icon: 'host' };
    if (a.category === 'host') return { badge: 'HOST', icon: 'host' };
    const hasUrl = a.addresses.some((d) => d.kind === 'url' && /^https?:\/\//i.test(d.value));
    if (hasUrl || a.category === 'web_application') return { badge: 'WEB', icon: 'web' };
    return { badge: null, icon: 'host' };
}

function NodeBadge({ children }: { children: ReactNode }) {
    return <span className="pen-ov-node__badge">{children}</span>;
}

function AssetNodeView({ node }: { node: AssetNodeData }) {
    const Icon = node.icon === 'web' ? Globe : Server;
    return (
        <Link className="pen-ov-node pen-ov-node--asset" to={node.href}>
            <span className="pen-ov-node__top">
                <Icon aria-hidden="true" />
                <span className="pen-ov-node__name">{node.name}</span>
            </span>
            {node.badge && <NodeBadge>{node.badge}</NodeBadge>}
        </Link>
    );
}

function EngineNodeView({ engine }: { engine: EngineNodeData }) {
    const Icon = ENGINE_ICONS[engine.key] ?? Plug;
    return (
        <div className="pen-ov-node pen-ov-node--engine">
            <span className="pen-ov-node__top">
                <Icon aria-hidden="true" />
                <span className="pen-ov-node__name">{engine.name}</span>
            </span>
            <NodeBadge>{textUI('pen_ov_node_engine')}</NodeBadge>
        </div>
    );
}

// Node temuan individual: tombol expand/collapse, default tertutup, bisa diakses keyboard.
function FindingNodeView({ finding, open, onToggle }: { finding: Finding; open: boolean; onToggle: (id: string) => void }) {
    useLocale();
    const lines = open ? evidenceLines(finding) : [];
    const seen = shortDate(finding.last_seen_at || finding.first_seen_at);
    return (
        <button
            type="button"
            className={`pen-ov-node pen-ov-node--finding${open ? ' is-open' : ''}`}
            aria-expanded={open}
            onClick={() => onToggle(finding.id)}
        >
            <span className="pen-ov-node__top">
                <SeverityPill severity={finding.severity}>{capitalLabel(finding.severity)}</SeverityPill>
                {open ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}
            </span>
            <span className="pen-ov-node__title">{finding.title}</span>
            {open && (lines.length > 0 || seen) && (
                <span className="pen-ov-node__detail">
                    {lines.map((line, i) => <span key={i} className="pen-ov-node__line">{line}</span>)}
                    {seen && <span className="pen-ov-node__line">{textUI('pen_ov_seen', [seen])}</span>}
                </span>
            )}
        </button>
    );
}

// ---------- graph scan (aset → engine → temuan) ----------

// Satu graph scan. Tiap baris = satu engine online; temuan dipetakan ke engine lewat
// field details; kolom kiri menampilkan aset yang bersangkutan (pool most_exposed atau
// node simulasi untuk graph live). Temuan di luar batas dirangkum sebagai counter.
function ScanGraph({ assets, pinned, engines, findings, live, head, emptyText }: {
    assets: AssetNodeData[];
    pinned?: AssetNodeData;
    engines: EngineNodeData[];
    findings: Finding[];
    live: boolean;
    head?: ReactNode;
    emptyText: ReactNode;
}) {
    useLocale();
    const [openIds, setOpenIds] = useState<ReadonlySet<string>>(new Set());
    const toggle = (id: string) => setOpenIds((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id); else next.add(id);
        return next;
    });
    const shown = findings.slice(0, MAX_FINDING_NODES);
    const pool = assets.slice(0, MAX_ASSET_NODES);

    const nodeFor = (f: Finding): AssetNodeData | null => {
        if (!f.asset_id) return null;
        const known = pool.find((a) => a.id === f.asset_id);
        if (known) return known;
        // Aset dari temuan sendiri tetap node asli (data nyata) meski tidak ada di pool.
        return { id: f.asset_id, name: f.asset_name || f.asset_id, badge: null, icon: 'host', href: `/assets/${f.asset_id}` };
    };

    // Kelompokkan temuan yang dirender per engine; yang tak bisa dipetakan masuk counter.
    const grouped = new Map<string, Finding[]>();
    let unmapped = 0;
    for (const f of shown) {
        const key = engineKeyOf(f);
        if (!key || !engines.some((e) => e.key === key)) { unmapped += 1; continue; }
        const list = grouped.get(key) ?? [];
        list.push(f);
        grouped.set(key, list);
    }
    const activeEngines = engines.filter((e) => (grouped.get(e.key)?.length ?? 0) > 0).slice(0, MAX_ENGINE_NODES);
    const rows = activeEngines.length > 0 ? activeEngines : engines.slice(0, 1);
    const extra = findings.length - shown.length + unmapped;

    if (rows.length === 0) {
        return (
            <div className="pen-ov-graph">
                {head && <div className="pen-ov-graph__head">{head}</div>}
                <p className="pen-ov-empty">{emptyText}</p>
            </div>
        );
    }

    return (
        <div className={`pen-ov-graph${live ? ' pen-ov-graph--live' : ''}`}>
            {head && <div className="pen-ov-graph__head">{head}</div>}
            <div className="pen-ov-graph__grid" role="group" aria-label={textUI('pen_ov_diagram_label')}>
                <p className="pen-ov-graph__colhead">{textUI('pen_ov_col_assets')}</p>
                <span aria-hidden="true" />
                <p className="pen-ov-graph__colhead">{textUI('pen_ov_col_engines')}</p>
                <span aria-hidden="true" />
                <p className="pen-ov-graph__colhead">{textUI('pen_ov_col_findings')}</p>
                {rows.map((engine, rowIdx) => {
                    const rowFindings = grouped.get(engine.key) ?? [];
                    // Sel aset baris pertama memuat pool (snapshot) atau node simulasi (live);
                    // baris lain memuat aset yang muncul pada temuan baris tersebut.
                    const cell: AssetNodeData[] = [];
                    const push = (n: AssetNodeData | null) => {
                        if (n && !cell.some((c) => c.id === n.id) && cell.length < MAX_ASSET_NODES + (pinned ? 1 : 0)) cell.push(n);
                    };
                    if (rowIdx === 0 && pinned) push(pinned);
                    if (rowIdx === 0 && rowFindings.length === 0) for (const a of pool) push(a);
                    let noAsset = false;
                    for (const f of rowFindings) {
                        if (!f.asset_id) { noAsset = true; continue; }
                        push(nodeFor(f));
                    }
                    return (
                        <Fragment key={engine.key}>
                            <div className="pen-ov-cell">
                                {cell.map((a) => <AssetNodeView key={a.id} node={a} />)}
                                {noAsset && cell.length === 0 && <p className="pen-ov-empty">{textUI('m_8581bd739761')}</p>}
                                {rowIdx === 0 && !pinned && cell.length === 0 && !noAsset && <p className="pen-ov-empty">{textUI('pen_ov_no_assets')}</p>}
                            </div>
                            <span className="pen-ov-link" aria-hidden="true" />
                            <div className="pen-ov-cell pen-ov-cell--engine"><EngineNodeView engine={engine} /></div>
                            <span className="pen-ov-link" aria-hidden="true" />
                            <div className="pen-ov-cell">
                                {rowFindings.map((f) => (
                                    <FindingNodeView key={f.id} finding={f} open={openIds.has(f.id)} onToggle={toggle} />
                                ))}
                                {rowFindings.length === 0 && rowIdx === 0 && <p className="pen-ov-empty">{emptyText}</p>}
                            </div>
                        </Fragment>
                    );
                })}
            </div>
            {extra > 0 && <Link className="pen-ov-more" to="/findings">{textUI('pen_ov_more_findings', [extra])}</Link>}
        </div>
    );
}

// Graph terpisah untuk tiap simulasi yang sedang berjalan (state LIVE).
function LiveSimGraph({ sim, engines }: { sim: Simulation; engines: EngineNodeData[] }) {
    useLocale();
    const fq = useSimulationFindings(sim.id);
    const findings = useMemo(
        () => sortFindings(fq.data?.pages.flatMap((p) => p.items ?? []) ?? []),
        [fq.data],
    );
    const progress = textUI('pen_ov_progress', [sim.steps_done, sim.step_count]);
    const node: AssetNodeData = {
        id: sim.id,
        name: sim.name,
        badge: sim.target_count > 0 ? textUI('pen_ov_targets_badge', [sim.target_count]) : null,
        icon: 'web',
        href: `/simulations/${sim.id}`,
    };
    return (
        <ScanGraph
            assets={[node]}
            pinned={node}
            engines={engines}
            findings={findings}
            live
            head={<>
                <span className="pen-ov-graph__code">{sim.code}</span>
                <span className="pen-ov-graph__name">{sim.name}</span>
                <StateTag tone="accent">{displayLabel(sim.status)}</StateTag>
                <span className="pen-ov-graph__meta">{progress}</span>
            </>}
            emptyText={sim.step_count > 0 ? progress : textUI('pen_ov_no_findings')}
        />
    );
}

// ---------- bagian halaman ----------

// Notice ter atas: waktu pembaruan data, jumlah engine online, peringatan engine luring, tombol segarkan.
function NoticeBar({ h, onRefresh, busy }: { h: HomeSummary; onRefresh: () => void; busy: boolean }) {
    useLocale();
    const online = h.injectors.filter((i) => i.status === 'online');
    const offline = h.injectors.filter((i) => i.status !== 'online');
    const updated = shortDate(h.generated_at);
    return (
        <div className="pen-ov__notice" role="status">
            <span>
                {updated && richUI('pen_ov_notice_updated', [updated])}
                {updated && ' · '}
                {richUI(online.length === 1 ? 'pen_ov_engines_online_one' : 'pen_ov_engines_online', [online.length])}
            </span>
            {offline.length > 0 && (
                <span className="pen-ov__notice-warn">
                    {richUI('m_4845a5715da9', [
                        offline.map((i) => i.name).join(', '),
                        offline.length === 1 ? textUI('m_8a27f2fcccaa') : textUI('m_ba5bff2b4cdf'),
                    ])}
                </span>
            )}
            <span className="pen-ov__notice-spacer" aria-hidden="true" />
            <button type="button" className={`pen-ov__refresh${busy ? ' is-busy' : ''}`} onClick={onRefresh} disabled={busy}>
                <RefreshCw aria-hidden="true" />
                {textUI('pen_ov_refresh')}
            </button>
        </div>
    );
}

// Page heading: overline WORKSPACE, judul, deskripsi ringkas jumlah organisasi/aset.
function PageHead({ h, intro }: { h: HomeSummary; intro?: ReactNode }) {
    useLocale();
    const orgs = h.organizations?.length ?? 0;
    const assets = h.assets?.total ?? 0;
    return (
        <div className="pen-ov__head">
            <div>
                <p className="pen-ov__overline">{textUI('pen_ov_overline')}</p>
                <h1 className="pen-ov__title">{textUI('pen_ov_title')}</h1>
                <p className="pen-ov__desc">{intro ?? richUI('pen_ov_desc', [formatUI(orgs), formatUI(assets)])}</p>
            </div>
            <div className="pen-ov__actions">
                <Link className="btn btn--secondary btn--auto" to="/findings">{textUI('pen_ov_view_findings')}</Link>
                <Link className="btn btn--primary btn--auto" to="/simulations/new">{textUI('pen_ov_run_scan')}</Link>
            </div>
        </div>
    );
}

// Panel Live Scan: state IDLE menampilkan snapshot (most_exposed + temuan terbaru),
// state LIVE merender satu graph terpisah per simulasi yang sedang berjalan.
function LiveScanBox({ h, engines }: { h: HomeSummary; engines: EngineNodeData[] }) {
    useLocale();
    const live = (h.chains?.running ?? 0) > 0;
    const running = useSimulations('running', live);
    const latest = useFindings('', '', '', '', '', '');
    const allAssets = useAllAssets();
    const assetsById = useMemo(() => new Map((allAssets.data ?? []).map((a) => [a.id, a])), [allAssets.data]);

    // Pool aset dari most_exposed; badge tipe diambil dari data aset asli (bukan tebakan).
    const pool = useMemo(() => h.most_exposed.slice(0, MAX_ASSET_NODES).map((row) => {
        const kind = assetKind(assetsById.get(row.asset_id));
        return { id: row.asset_id, name: row.name, badge: kind.badge, icon: kind.icon, href: `/assets/${row.asset_id}` };
    }), [h.most_exposed, assetsById]);

    const idleFindings = useMemo(() => sortFindings(latest.data?.pages.flatMap((p) => p.items ?? []) ?? []), [latest.data]);

    const last = h.latest_simulations[0];
    const runningSims = running.data ?? [];
    const showLive = live && runningSims.length > 0;
    return (
        <section className="pen-ov-live" aria-labelledby="pen-ov-live-title">
            <div className="pen-ov-live__head">
                <h2 id="pen-ov-live-title" className="pen-ov-live__title">{textUI('pen_ov_livescan_title')}</h2>
                {showLive ? (
                    <span className="pen-ov-live__badge pen-ov-live__badge--live">
                        <span className="pen-ov-dot pen-ov-dot--live" aria-hidden="true" />
                        {textUI('pen_ov_badge_live')}
                    </span>
                ) : (
                    <span className="pen-ov-live__badge">
                        <span className="pen-ov-dot" aria-hidden="true" />
                        {textUI('pen_ov_badge_idle')}
                    </span>
                )}
                <span className="pen-ov-live__spacer" aria-hidden="true" />
                <span className="pen-ov-live__meta">
                    {last && <span>{textUI('pen_ov_last_run', [relativeTime(last.started_at ?? last.created_at)])}</span>}
                    <Link className="pen-ov-live__link" to="/simulations">{textUI('pen_ov_history_link')}</Link>
                </span>
            </div>
            {live && running.isPending ? (
                <div className="pen-ov-graph" aria-hidden="true">
                    <span className="pen-ov-skel pen-ov-skel--block" />
                </div>
            ) : showLive ? (
                <div className="pen-ov-graphs">
                    {runningSims.slice(0, MAX_LIVE_GRAPHS).map((sim) => <LiveSimGraph key={sim.id} sim={sim} engines={engines} />)}
                </div>
            ) : (
                <ScanGraph assets={pool} engines={engines} findings={idleFindings} live={false} emptyText={textUI('pen_ov_no_findings')} />
            )}
            <p className="pen-ov-live__foot">{textUI('pen_ov_diagram_hint')}</p>
        </section>
    );
}

// Strip 4 kartu metrik; bagian yang tidak dikirim server (null) tidak dirender.
function MetricStrip({ h }: { h: HomeSummary }) {
    useLocale();
    const f = h.findings;
    const a = h.assets;
    const at = h.atomic;
    const c = h.chains;
    const sev = f?.by_severity ?? {};
    return (
        <div className="pen-ov-metrics" role="group" aria-label={textUI('m_b6fead36007e')}>
            {f && (
                <MetricCard
                    label={textUI('m_64f74286b22d')}
                    value={formatUI(f.total)}
                    caption={textUI('pen_ov_cap_findings', [sev.critical ?? 0, sev.high ?? 0, sev.medium ?? 0, sev.low ?? 0, sev.info ?? 0])}
                    bar={{ bySeverity: sev }}
                />
            )}
            {a && <MetricCard label={textUI('pen_ov_metric_assets')} value={formatUI(a.total)} caption={textUI('pen_ov_cap_assets', [a.authorized, a.internet_facing])} />}
            {at && <MetricCard label={textUI('pen_ov_metric_atomic')} value={formatUI(at.tests)} caption={textUI('pen_ov_cap_atomic', [at.active, at.runs_7d])} />}
            {c && <MetricCard label={textUI('m_078a864ea0f1')} value={formatUI(c.simulations)} caption={textUI('pen_ov_cap_sims', [c.running, c.scenarios])} />}
        </div>
    );
}

// Kolom kiri: aset paling terekspos dengan severity bar per aset dan jumlah port terbuka.
function ExposedPanel({ h, level }: { h: HomeSummary; level: { label: string; tone: string } | null }) {
    useLocale();
    return (
        <PanelCard
            title={textUI('m_78463a7fd155')}
            actions={level && <span className={`level level--${level.tone}`}>{level.label}</span>}
        >
            {h.most_exposed.length === 0 ? (
                <>
                    <p className="pen-ov-empty">{textUI('m_5af7f5275308')}</p>
                    <p className="pen-ov-empty">{textUI('m_2bf1c9acabb0')}</p>
                </>
            ) : (
                <ul className="pen-ov-list">
                    {h.most_exposed.map((row) => {
                        const total = row.critical + row.high + row.medium + row.low;
                        const segs: ReactNode[] = [];
                        if (row.critical > 0) segs.push(richUI('m_1e3da69aa261', [formatUI(row.critical)]));
                        if (row.high > 0) segs.push(richUI('m_5b8ee36411fb', [formatUI(row.high)]));
                        if (row.medium > 0) segs.push(richUI('m_92f124e956e4', [formatUI(row.medium)]));
                        if (row.low > 0) segs.push(richUI('m_20edd6ad4f02', [row.low]));
                        return (
                            <li className="pen-ov-xrow" key={row.asset_id}>
                                <div className="pen-ov-xrow__top">
                                    <Link className="pen-ov-xrow__name" to={`/assets/${row.asset_id}`}>{row.name}</Link>
                                    <span className="pen-ov-xrow__ports">{richUI('m_47e1fe5e70b1', [formatUI(row.open_ports), row.open_ports === 1 ? 'port' : 'ports'])}</span>
                                </div>
                                <SeverityBar bySeverity={{ critical: row.critical, high: row.high, medium: row.medium, low: row.low }} total={total} />
                                <p className="pen-ov-xrow__cap">
                                    {segs.length > 0
                                        ? segs.map((s, i) => <Fragment key={i}>{i > 0 && ' · '}{s}</Fragment>)
                                        : textUI('m_ca952f95561c')}
                                </p>
                            </li>
                        );
                    })}
                </ul>
            )}
        </PanelCard>
    );
}

// Kolom kanan: simulasi terbaru dengan status tag dan waktu relatif.
function SimulationsPanel({ h }: { h: HomeSummary }) {
    useLocale();
    return (
        <PanelCard
            title={textUI('m_c90295345ee7')}
            actions={<Link className="pen-ov-panel-link" to="/simulations">{textUI('m_30a64216eaea')}</Link>}
        >
            {h.latest_simulations.length === 0 ? (
                <p className="pen-ov-empty">{textUI('m_b46f20e3b7c9')}</p>
            ) : (
                <ul className="pen-ov-list">
                    {h.latest_simulations.map((s) => (
                        <li className="pen-ov-srow" key={s.id}>
                            <span className="pen-ov-srow__code">{s.code}</span>
                            <Link className="pen-ov-srow__name" to={`/simulations/legacy/${s.id}`}>{s.name}</Link>
                            <StateTag tone={s.status === 'finished' ? 'success' : s.status === 'canceled' ? 'neutral' : 'accent'}>
                                {displayLabel(s.status)}
                            </StateTag>
                            <span className="pen-ov-srow__at">{relativeTime(s.started_at ?? s.created_at)}</span>
                        </li>
                    ))}
                </ul>
            )}
        </PanelCard>
    );
}

// Status engine: kartu per injector, dot hijau hanya bila status "online".
function EnginesSection({ h, me }: { h: HomeSummary; me: User | null | undefined }) {
    useLocale();
    if (h.injectors.length === 0) return null;
    const offline = h.injectors.filter((i) => i.status !== 'online');
    const statusText = (s: string) => (s === 'online' ? textUI('pen_ov_eng_online') : s === 'disabled' ? textUI('pen_ov_eng_disabled') : textUI('pen_ov_eng_offline'));
    return (
        <section aria-labelledby="pen-ov-engines-title">
            <h2 id="pen-ov-engines-title" className="pen-ov-sec-title">{textUI('pen_ov_engines_title')}</h2>
            <div className="pen-ov-engines" aria-label={textUI('m_0db61c756736')}>
                {h.injectors.map((inj) => {
                    const Icon = ENGINE_ICONS[inj.key] ?? Plug;
                    const online = inj.status === 'online';
                    return (
                        <div className="pen-ov-engine" key={inj.key}>
                            <span className="pen-ov-engine__icon" aria-hidden="true"><Icon /></span>
                            <div className="pen-ov-engine__text">
                                <p className="pen-ov-engine__name">{inj.name}</p>
                                <span className={`pen-ov-engine__status${online ? ' pen-ov-engine__status--online' : ''}`}>
                                    <span className={`pen-ov-dot${online ? ' pen-ov-dot--ok' : ''}`} aria-hidden="true" />
                                    {statusText(inj.status)}
                                </span>
                            </div>
                        </div>
                    );
                })}
            </div>
            {offline.length > 0 && (
                <p className="pen-ov-engines__note">
                    {richUI('m_3f8de742f797', [offline.length === 1 ? 'it' : 'them'])}{' '}
                    {can(me, 'integration.read')
                        ? <Link to="/settings/integrations">{textUI('m_8ac30ac6ae4d')}</Link>
                        : textUI('m_545a0ca386d8')}
                </p>
            )}
        </section>
    );
}

// Workspace baru yang benar-benar kosong: panduan langkah awal, bukan deretan nol.
function StartingState({ h, me }: { h: HomeSummary; me: User | null | undefined }) {
    useLocale();
    const online = h.injectors.filter((i) => i.status === 'online');
    const offline = h.injectors.filter((i) => i.status !== 'online');
    const noOrgs = (h.organizations?.length ?? 0) === 0;
    return (
        <>
            {can(me, 'organization.read') && <OrganisationSetup />}
            <section className="panel" aria-labelledby="first-results-title">
                <h2 id="first-results-title" className="panel__heading">{textUI('m_d822a6b52b11')}</h2>
                <p className="muted">{textUI('m_1de16038df84')}</p>
                <p className="muted">{richUI('m_ff02cdc06eab', [formatUI(online.length), formatUI(h.injectors.length)])}</p>
                {offline.length > 0 && (
                    <p>
                        {richUI('m_8a1c28fa6db9', [
                            offline.map((i) => i.name).join(' and '),
                            offline.length === 1 ? textUI('m_8a27f2fcccaa') : textUI('m_ba5bff2b4cdf'),
                        ])}
                    </p>
                )}
                {can(me, 'integration.read') && <Link to="/settings/integrations">{textUI('m_8ac30ac6ae4d')}</Link>}
            </section>
            {noOrgs && (
                <section className="panel" aria-labelledby="bring-assets-title">
                    <h2 id="bring-assets-title" className="panel__heading">{textUI('m_37ce5ee4967c')}</h2>
                    <p>{textUI('m_7fd913d97b30')}</p>
                    <Link to="/organizations">{can(me, 'organization.write') ? textUI('m_63230efa4040') : textUI('m_633c362aeef1')}</Link>
                </section>
            )}
        </>
    );
}

// Organisasi sudah ada tetapi belum ada hasil pindaian: ringkasan organisasi memandu
// assessment pertama memakai data nyata dari /home.
function OrgStrip({ h, me }: { h: HomeSummary; me: User | null | undefined }) {
    useLocale();
    const orgs = h.organizations ?? [];
    if (orgs.length === 0) return null;
    return (
        <section className="panel" aria-labelledby="org-strip-title">
            <div className="posture__head">
                <div>
                    <p className="eyebrow">{textUI('m_53000c389d4d')}</p>
                    <h2 id="org-strip-title" className="panel__heading">{textUI('m_0a154b9350d3')}</h2>
                    <p className="muted small">{textUI('m_14f942a625e7')}</p>
                </div>
                <Link className="btn btn--secondary btn--auto" to="/organizations">
                    {can(me, 'organization.write') ? textUI('m_07c7940a29a3') : textUI('m_17e36c43582d')}
                </Link>
            </div>
            <div className="org-grid">
                {orgs.map((o) => (
                    <article className="org-card" key={o.id}>
                        <p className="eyebrow">{o.latest_status.replace('_', ' ')}</p>
                        <h3><Link to={`/organizations/${o.id}`}>{o.name}</Link></h3>
                        <strong className="org-risk">{o.risk}</strong>
                        <p>{richUI('m_86370f6e7235', [formatUI(o.authorized), formatUI(o.assets)])}</p>
                        <div className="row">
                            <Link to={`/organizations/${o.id}`}>
                                {can(me, 'assessment.run') ? textUI('m_d5a89eaa6e23') : textUI('m_91020e1cae44')}
                            </Link>
                            {o.latest_assessment && <Link to={`/assessments/${o.latest_assessment}`}>{textUI('m_660449417ba0')}</Link>}
                        </div>
                    </article>
                ))}
            </div>
        </section>
    );
}

function HomeBody({ h, me, onRefresh, busy }: { h: HomeSummary; me: User | null | undefined; onRefresh: () => void; busy: boolean }) {
    const starting = (h.organizations?.length ?? 0) === 0
        && (h.assets?.total ?? 0) === 0
        && (h.findings?.total ?? 0) === 0
        && (h.atomic?.tests ?? 0) === 0
        && (h.chains?.scenarios ?? 0) === 0;
    const firstScan = !starting
        && (h.findings?.total ?? 0) === 0
        && (h.atomic?.tests ?? 0) === 0
        && (h.chains?.simulations ?? 0) === 0;
    const level = h.findings ? exposureLevel(h.findings.by_severity, h.findings.vulnerabilities) : null;
    const engines: EngineNodeData[] = h.injectors
        .filter((i) => i.status === 'online')
        .slice(0, MAX_ENGINE_NODES)
        .map((i) => ({ key: i.key, name: i.name }));
    return (
        <>
            <NoticeBar h={h} onRefresh={onRefresh} busy={busy} />
            <PageHead h={h} intro={starting ? richUI('m_584ab53ab844', [me ? `, ${me.display_name}` : '']) : undefined} />
            {starting ? (
                <StartingState h={h} me={me} />
            ) : (
                <>
                    {firstScan && <OrgStrip h={h} me={me} />}
                    <LiveScanBox h={h} engines={engines} />
                    <MetricStrip h={h} />
                    <div className="pen-ov-cols">
                        <ExposedPanel h={h} level={level} />
                        <SimulationsPanel h={h} />
                    </div>
                    <EnginesSection h={h} me={me} />
                </>
            )}
        </>
    );
}

function LoadingState() {
    useLocale();
    return (
        <div className="pen-ov-loading" role="status" aria-label={textUI('m_5ef130c39669')}>
            <span className="pen-ov-skel pen-ov-skel--bar" aria-hidden="true" />
            <span className="pen-ov-skel pen-ov-skel--bar" aria-hidden="true" />
            <span className="pen-ov-skel pen-ov-skel--block" aria-hidden="true" />
            <div className="pen-ov-metrics" aria-hidden="true">
                {[0, 1, 2, 3].map((i) => <span key={i} className="pen-ov-skel pen-ov-skel--card" />)}
            </div>
        </div>
    );
}

function ErrorState({ error, onRetry }: { error: unknown; onRetry: () => void }) {
    useLocale();
    return (
        <div className="state state--error" role="alert">
            <h2>{textUI('m_b96e0c63b366')}</h2>
            <p>{richUI('m_19636dc199da', [describeError(error).message])}</p>
            <button type="button" className="btn btn--secondary btn--auto" onClick={onRetry}>{textUI('m_d8b8392e2c54')}</button>
        </div>
    );
}

export function HomePage() {
    useLocale();
    const { data: me } = useMe();
    const q = useHome();
    return (
        <section className="page pen-ov" aria-label={textUI('m_3a78695388b3')}>
            {q.isPending
                ? <LoadingState />
                : q.isError || !q.data
                    ? <ErrorState error={q.error} onRetry={() => { void q.refetch(); }} />
                    : <HomeBody h={q.data} me={me} onRefresh={() => { void q.refetch(); }} busy={q.isFetching} />}
        </section>
    );
}

export function NotFoundPage() {
    useLocale();
    return (<div className="fullpage">
      <p className="eyebrow">404</p>
      <h1 className="title">{textUI("m_4427d9bbc643")}</h1>
      <p className="subtitle">{textUI("m_8052556251cd")}</p>
      <a className="btn btn--secondary btn--auto" href="/">{textUI("m_a0aac9145249")}</a>
    </div>);
}
