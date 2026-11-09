import { ArrowRight, ArrowUpRight, Check, ShieldCheck } from 'lucide-react';
import { Link } from 'react-router-dom';
import { language, setLanguage, t, useLocale } from '../i18n/runtime';
import { ThemeToggle } from '../components/ThemeToggle';
import { SupporterCredit } from '../components/SupporterCredit';
import { SecurityJourney } from '../components/SecurityJourney';
import './landing.css';

// Public, static product information. Assessment data belongs in the signed-in workspace.
export function LandingPage() {
    useLocale();
    const benefits = [
        { title: t('public_benefit_visibility'), description: t('public_benefit_visibility_detail') },
        { title: t('public_benefit_focus'), description: t('public_benefit_focus_detail') },
        { title: t('public_benefit_confidence'), description: t('public_benefit_confidence_detail') },
    ];
    return <div className="public-site">
        <a className="public-skip" href="#main">{t('pen_landing_skip')}</a>
        <header className="public-nav public-wrap">
            <Link className="public-brand" to="/" aria-label="Orchis"><ShieldCheck size={23} strokeWidth={1.4} aria-hidden="true"/>ORCHIS</Link>
            <nav aria-label={t('public_navigation')}>
                <a className="public-nav__about" href="#why-orchis">{t('public_why_link')}</a>
                <div className="public-language" role="group" aria-label={t('language_label')}>
                    <button type="button" aria-pressed={language() === 'en'} onClick={() => setLanguage('en')}>EN</button>
                    <button type="button" aria-pressed={language() === 'id'} onClick={() => setLanguage('id')}>ID</button>
                </div>
                <ThemeToggle/>
                <Link className="public-signin" to="/login">{t('pen_landing_nav_login')}<ArrowUpRight size={16} aria-hidden="true"/></Link>
            </nav>
        </header>
        <main id="main">
            <section className="public-hero public-wrap" aria-labelledby="public-title">
                <div className="public-hero__copy">
                    <p className="public-eyebrow">{t('public_eyebrow')}</p>
                    <h1 id="public-title">{t('public_headline')}<span>{t('public_headline_accent')}</span></h1>
                    <p className="public-intro">{t('public_intro')}</p>
                    <div className="public-hero__actions"><Link className="public-button" to="/login">{t('public_primary_action')}<ArrowRight size={18} aria-hidden="true"/></Link><a className="public-text-link" href="#why-orchis">{t('public_secondary_action')}</a></div>
                    <p className="public-assurance"><Check size={15} aria-hidden="true"/>{t('public_assurance')}</p>
                </div>
                <div className="public-hero__visual"><SecurityJourney/></div>
            </section>
            <div className="public-support public-wrap"><SupporterCredit/></div>
            <section id="why-orchis" className="public-value public-wrap" aria-labelledby="public-value-title">
                <div className="public-value__heading"><p className="public-eyebrow">{t('public_why_link')}</p><h2 id="public-value-title">{t('public_value_title')}</h2><p>{t('public_value_intro')}</p></div>
                <div className="public-benefits">{benefits.map(benefit => <article key={benefit.title}><h3>{benefit.title}</h3><p>{benefit.description}</p></article>)}</div>
            </section>
            <section className="public-outcome" aria-labelledby="public-outcome-title"><div className="public-wrap public-outcome__inner">
                <div><p className="public-eyebrow">{t('public_outcome_eyebrow')}</p><h2 id="public-outcome-title">{t('public_outcome_title')}</h2><p>{t('public_outcome_detail')}</p></div>
                <Link className="public-button" to="/login">{t('public_primary_action')}<ArrowRight size={18} aria-hidden="true"/></Link>
            </div></section>
        </main>
        <footer className="public-footer public-wrap"><span className="public-brand">ORCHIS</span><p>{t('public_footer')}</p><Link to="/login">{t('pen_landing_nav_login')}<ArrowUpRight size={14} aria-hidden="true"/></Link></footer>
    </div>;
}
