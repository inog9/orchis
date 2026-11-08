import { t as textUI, useLocale } from '../i18n/runtime';
import { Crosshair, FileCheck, Radar, Shield, ShieldCheck, Swords, Zap } from 'lucide-react';
import { Link } from 'react-router-dom';
import './landing.css';

// Halaman landing publik ("/"): 100% statis dari i18n, tanpa satu pun pemanggilan API
// (semua endpoint butuh auth). Satu-satunya tujuan interaksi adalah flow login (/login).

// Statistik pemasaran bernilai statis dari mockup pemilik (v2fixed.pen "Landing Page -
// Argus x Orchis"), bukan angka dari server: /home butuh auth sehingga tidak bisa dihitung.
const STATS = [
    { valueKey: 'pen_landing_stat1_value', labelKey: 'pen_landing_stat1_label' },
    { valueKey: 'pen_landing_stat2_value', labelKey: 'pen_landing_stat2_label' },
    { valueKey: 'pen_landing_stat3_value', labelKey: 'pen_landing_stat3_label' },
    { valueKey: 'pen_landing_stat4_value', labelKey: 'pen_landing_stat4_label' },
] as const;

// Empat kemampuan produk; ikon dipilih dari mockup pemilik karena relevan dengan isinya:
// radar (pemindaian permukaan), shield-check (agent pertahanan), swords (simulasi serangan),
// file-check (laporan yang terverifikasi).
const FEATURES = [
    { icon: Radar, titleKey: 'pen_landing_feat1_title', descKey: 'pen_landing_feat1_desc' },
    { icon: ShieldCheck, titleKey: 'pen_landing_feat2_title', descKey: 'pen_landing_feat2_desc' },
    { icon: Swords, titleKey: 'pen_landing_feat3_title', descKey: 'pen_landing_feat3_desc' },
    { icon: FileCheck, titleKey: 'pen_landing_feat4_title', descKey: 'pen_landing_feat4_desc' },
] as const;

// Langkah alur kerja; nomor 01-03 statis (tidak ikut diterjemahkan).
const STEPS = [
    { num: '01', titleKey: 'pen_landing_how1_title', descKey: 'pen_landing_how1_desc' },
    { num: '02', titleKey: 'pen_landing_how2_title', descKey: 'pen_landing_how2_desc' },
    { num: '03', titleKey: 'pen_landing_how3_title', descKey: 'pen_landing_how3_desc' },
] as const;

// Engine terintegrasi; ikon mengikuti mockup pemilik (shield/crosshair/radar/zap).
const ENGINES = [
    { icon: Shield, labelKey: 'pen_landing_engine_defensebot' },
    { icon: Crosshair, labelKey: 'pen_landing_engine_hackbot' },
    { icon: Radar, labelKey: 'pen_landing_engine_nmap' },
    { icon: Zap, labelKey: 'pen_landing_engine_nuclei' },
] as const;

export function LandingPage() {
    useLocale();
    return (
        <div className="pen-landing">
            <a className="ld-skip" href="#utama">{textUI('pen_landing_skip')}</a>
            <header className="ld-nav">
                <div className="ld-nav__inner">
                    <p className="ld-brand">
                        <span className="ld-brand__dot" aria-hidden="true"/>
                        <span className="ld-brand__mark">ORCHIS</span>
                    </p>
                    <nav className="ld-nav__actions">
                        {/* Belum ada endpoint signup: kedua tombol menuju flow login yang ada. */}
                        <Link to="/login" className="ld-btn ld-btn--ghost">{textUI('pen_landing_nav_login')}</Link>
                        <Link to="/login" className="ld-btn ld-btn--primary">{textUI('pen_landing_nav_start')}</Link>
                    </nav>
                </div>
            </header>
            <main id="utama">
                <section className="ld-hero">
                    <p className="ld-hero__badge">{textUI('pen_landing_hero_badge')}</p>
                    <h1 className="ld-hero__headline">{textUI('pen_landing_hero_headline')}</h1>
                    <p className="ld-hero__sub">{textUI('pen_landing_hero_sub')}</p>
                    <div className="ld-hero__cta">
                        <Link to="/login" className="ld-btn ld-btn--primary">{textUI('pen_landing_hero_cta_primary')}</Link>
                        {/* Tanpa halaman demo: CTA kedua menaut ke bagian kemampuan di halaman ini. */}
                        <a href="#kemampuan" className="ld-btn ld-btn--ghost">{textUI('pen_landing_hero_cta_secondary')}</a>
                    </div>
                    <p className="ld-hero__trust">{textUI('pen_landing_hero_trust')}</p>
                </section>

                <section className="ld-stats">
                    <dl className="ld-stats__grid">
                        {STATS.map(({ valueKey, labelKey }) => (
                            <div className="ld-stat" key={valueKey}>
                                <dt className="ld-stat__label">{textUI(labelKey)}</dt>
                                <dd className="ld-stat__value">{textUI(valueKey)}</dd>
                            </div>
                        ))}
                    </dl>
                </section>

                <section id="kemampuan" className="ld-features">
                    <div className="ld-head">
                        <p className="ld-head__overline">{textUI('pen_landing_feat_overline')}</p>
                        <h2 className="ld-head__title">{textUI('pen_landing_feat_title')}</h2>
                    </div>
                    <ul className="ld-features__grid">
                        {FEATURES.map(({ icon: Icon, titleKey, descKey }) => (
                            <li className="ld-card" key={titleKey}>
                                <span className="ld-card__icon" aria-hidden="true"><Icon size={18}/></span>
                                <h3 className="ld-card__title">{textUI(titleKey)}</h3>
                                <p className="ld-card__desc">{textUI(descKey)}</p>
                            </li>
                        ))}
                    </ul>
                </section>

                <section className="ld-how">
                    <div className="ld-head">
                        <p className="ld-head__overline">{textUI('pen_landing_how_overline')}</p>
                        <h2 className="ld-head__title">{textUI('pen_landing_how_title')}</h2>
                    </div>
                    <ol className="ld-how__steps">
                        {STEPS.map((step) => (
                            <li className="ld-step" key={step.num}>
                                <span className="ld-step__num" aria-hidden="true">{step.num}</span>
                                <h3 className="ld-step__title">{textUI(step.titleKey)}</h3>
                                <p className="ld-step__desc">{textUI(step.descKey)}</p>
                            </li>
                        ))}
                    </ol>
                </section>

                <section className="ld-engines">
                    <p className="ld-engines__label">{textUI('pen_landing_engine_label')}</p>
                    <ul className="ld-engines__row">
                        {ENGINES.map(({ icon: Icon, labelKey }) => (
                            <li className="ld-chip" key={labelKey}>
                                <Icon size={14} aria-hidden="true"/>
                                <span>{textUI(labelKey)}</span>
                                {/* Titik status dekoratif; halaman publik tidak menanyakan status engine. */}
                                <span className="ld-chip__dot" aria-hidden="true"/>
                            </li>
                        ))}
                    </ul>
                </section>

                <section className="ld-banner">
                    <div className="ld-banner__inner">
                        <h2 className="ld-banner__title">{textUI('pen_landing_cta_title')}</h2>
                        <p className="ld-banner__desc">{textUI('pen_landing_cta_desc')}</p>
                        <Link to="/login" className="ld-btn ld-btn--primary">{textUI('pen_landing_cta_button')}</Link>
                    </div>
                </section>
            </main>
            <footer className="ld-footer">
                <div className="ld-footer__inner">
                    <div className="ld-footer__top">
                        <p className="ld-brand">
                            <span className="ld-brand__dot" aria-hidden="true"/>
                            <span className="ld-brand__mark">ORCHIS</span>
                        </p>
                        <p className="ld-footer__tagline">{textUI('pen_landing_footer_tagline')}</p>
                    </div>
                    <div className="ld-footer__bottom">
                        <p className="ld-footer__copyright">{textUI('pen_landing_footer_copyright')}</p>
                        <Link to="/login" className="ld-footer__login">{textUI('pen_landing_nav_login')}</Link>
                    </div>
                </div>
            </footer>
        </div>
    );
}
