import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { RAVEN_PATHS, RAVEN_SIZE } from '../assets/raven-paths';
export function RavenMark({ size = 260, className }: {
    size?: number;
    className?: string;
}) {
    useLocale();
    return (<svg className={className} width={size} height={size} viewBox={`0 0 ${RAVEN_SIZE} ${RAVEN_SIZE}`} role="img" aria-label={textUI("m_f8dfae0a44c7")} xmlns="http://www.w3.org/2000/svg">
      {formatUI(RAVEN_PATHS.map((p, i) => (<path key={i} d={p.d} fill={p.fill} transform={`translate(${p.x} ${p.y})`}/>)))}
    </svg>);
}
