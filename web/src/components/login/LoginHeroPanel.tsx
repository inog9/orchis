import { ShieldCheck } from 'lucide-react';
import { t, useLocale } from '../../i18n/runtime';
import { SupporterCredit } from '../SupporterCredit';
import { SecurityJourney } from '../SecurityJourney';
import '../../pages/landing.css';
import './login-hero.css';

export function LoginHeroPanel() {
    useLocale();
    return <aside className="auth__brand public-login">
        <div className="public-brand"><ShieldCheck size={24} strokeWidth={1.4} aria-hidden="true"/>ORCHIS</div>
        <div className="public-login__story"><p className="public-eyebrow">{t('public_eyebrow')}</p><h2>{t('public_headline')}<span>{t('public_headline_accent')}</span></h2><p>{t('public_intro')}</p></div>
        <SecurityJourney/>
        <div className="public-login__foot"><SupporterCredit/><p>{t('public_footer')}</p></div>
    </aside>;
}
