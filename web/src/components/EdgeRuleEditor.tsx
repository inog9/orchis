import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useState } from 'react';
import type { ChainEdge } from '../api/types';
export function EdgeRuleEditor({ edge, disabled, onChange }: {
    edge: ChainEdge;
    disabled: boolean;
    onChange: (edge: ChainEdge) => void;
}) {
    useLocale();
    const [text, setText] = useState(edge.condition ? JSON.stringify(edge.condition, null, 2) : '');
    const [error, setError] = useState('');
    const apply = () => { try {
        const value = text.trim() ? JSON.parse(text) : undefined;
        if (value && (typeof value !== 'object' || Array.isArray(value)))
            throw new Error(textUI("m_dcce60b94f77"));
        onChange({ ...edge, condition: value });
        setError('');
    }
    catch {
        setError(textUI("m_923cb5f5feb4"));
    } };
    return <details className="org-card"><summary>{richUI("m_522e664fcf41", [formatUI(edge.parent), formatUI(edge.child)])}</summary><p className="muted small">{textUI("m_f309163c3664")}</p><div className="select"><label htmlFor={`shortcut-${edge.parent}-${edge.child}`}>{textUI("m_67d036be53e3")}</label><select id={`shortcut-${edge.parent}-${edge.child}`} disabled={disabled} value="" onChange={e => { if (!e.target.value)
        return; const condition = e.target.value === 'always' ? undefined : { op: e.target.value === 'found' ? 'GT' : 'EQ', field: 'finding_count', value: 0 }; setText(condition ? JSON.stringify(condition, null, 2) : ''); setError(''); onChange({ ...edge, condition }); }}><option value="">{textUI("m_14c9d7632221")}</option><option value="always">{textUI("m_44120c249ee8")}</option><option value="found">{textUI("m_af550a28e051")}</option><option value="empty">{textUI("m_60a7be766012")}</option></select></div><p className="muted small">{textUI("m_6f846bb1889b")}</p><p className="muted small">{textUI("m_5057e30c7066")}</p><label htmlFor={`condition-${edge.parent}-${edge.child}`}>{textUI("m_bb75e2dec670")}</label><textarea id={`condition-${edge.parent}-${edge.child}`} rows={5} disabled={disabled} value={text} placeholder={'{"op":"GT","field":"finding_count","value":0}'} onChange={e => setText(e.target.value)}/><button type="button" className="btn btn--secondary btn--auto" disabled={disabled} onClick={apply}>{textUI("m_eb482ea86670")}</button>{formatUI(error && <p role="alert">{formatUI(error)}</p>)}<div className="select"><label htmlFor={`branch-${edge.parent}-${edge.child}`}>{textUI("m_52656e8104ee")}</label><select id={`branch-${edge.parent}-${edge.child}`} disabled={disabled} value={edge.branch ?? 'match'} onChange={e => onChange({ ...edge, branch: e.target.value as ChainEdge['branch'] })}><option value="match">{textUI("m_c6925e414144")}</option><option value="else">{textUI("m_4d1ef57c0ac0")}</option><option value="failure">{textUI("m_c70fd09f0103")}</option></select></div><div className="select"><label htmlFor={`mapper-${edge.parent}-${edge.child}`}>{textUI("m_18cc6b971dcb")}</label><select id={`mapper-${edge.parent}-${edge.child}`} disabled={disabled} value={edge.mapper?.destination ?? ''} onChange={e => onChange({ ...edge, mapper: e.target.value === 'ports' ? { source: 'open_ports', destination: 'ports' } : e.target.value === 'target' ? { source: 'web_urls', destination: 'target' } : e.target.value === 'tags' ? { source: 'installed_software', destination: 'tags' } : undefined })}><option value="">{textUI("m_8a19df557747")}</option><option value="ports">{textUI("m_99a15931cf2a")}</option><option value="target">{textUI("m_ccf515144453")}</option><option value="tags">{textUI("m_fa78602ff306")}</option></select></div></details>;
}
