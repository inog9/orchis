import { t as textUI, useLocale } from "../i18n/runtime";
export function Wordmark({ size = 16, live = false }: {
    size?: number;
    live?: boolean;
}) {
    useLocale();
    return (<div className="wordmark">
      <span className={`wordmark__dot${live ? ' is-live' : ''}`} aria-hidden="true"/>
      <span className="wordmark__text" style={{ fontSize: size }}>{textUI("m_f8dfae0a44c7")}</span>
    </div>);
}
