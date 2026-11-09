import logo from '../assets/jogja-cybersec-transparent.png';
import { t, useLocale } from '../i18n/runtime';
import './supporter-credit.css';

// Original artwork from https://github.com/jogjacybersec (profile avatar).
// Kept locally so the public page does not depend on a third-party image request.
export function SupporterCredit() {
    useLocale();
    return <div className="supporter-credit">
        <p className="supporter-credit__label">{t('public_supported_by')}</p>
        <a href="https://jogjacybersec.com/" target="_blank" rel="noreferrer" aria-label="Jogja Cyber Security" className="supporter-credit__identity">
            <img src={logo} alt="" width="88" height="88" decoding="async"/>
        </a>
    </div>;
}
