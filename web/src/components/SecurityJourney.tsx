import { ArrowDown, ScanLine, FileCheck2, ListChecks } from 'lucide-react';
import { t, useLocale } from '../i18n/runtime';

// A product explanation, never a preview of customer data or live system status.
export function SecurityJourney() {
    useLocale();
    const steps = [
        { Icon: ScanLine, title: t('public_journey_assets'), detail: t('public_journey_assets_detail') },
        { Icon: FileCheck2, title: t('public_journey_evidence'), detail: t('public_journey_evidence_detail') },
        { Icon: ListChecks, title: t('public_journey_action'), detail: t('public_journey_action_detail') },
    ];
    return <figure className="public-journey" aria-label={t('public_journey_label')}>
        <figcaption>{t('public_journey_label')}</figcaption>
        <ul>{steps.map(({Icon, title, detail}, index) => <li key={title}>
            <span className="public-journey__icon"><Icon size={25} strokeWidth={1.4} aria-hidden="true"/></span>
            <div><h3>{title}</h3><p>{detail}</p></div>
            {index < steps.length - 1 && <ArrowDown className="public-journey__arrow" size={16} strokeWidth={1.3} aria-hidden="true"/>}
        </li>)}</ul>
        <p className="public-journey__end">{t('public_journey_end')}</p>
    </figure>;
}
