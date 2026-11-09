import { agentDisplayName } from '../lib/engines';
import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useHome, useMe } from '../api/hooks';
import { can, type Asset } from '../api/types';
export function TestPrerequisites({ targetID, needsAgent }: {
    targetID?: string | null;
    needsAgent: boolean;
}) {
    useLocale();
    const { data: me } = useMe();
    const h = useHome();
    const q = useQuery({ queryKey: ['asset', targetID], enabled: !!targetID && can(me, 'asset.read'), queryFn: () => api.get<Asset>(`/api/v1/assets/${targetID}`), refetchInterval: 5000 });
    const a = q.data;
    const offline = h.data?.injectors?.filter(i => i.status !== 'online') || [];
    return <div className="prerequisites"><p>{textUI("m_bfe6b48ed152")}</p>
 {formatUI(!targetID && <p>{richUI("m_c631e8590e52", [<Link to="/assets">{textUI("m_a629202d0105")}</Link>])}</p>)}
 {formatUI(a && <><Link to={`/assets/${a.id}`}>{richUI("m_8fc521826256", [formatUI(a.name)])}</Link>{formatUI(!a.testing_authorized && <p>{textUI("m_861e54f4b362")}</p>)}{formatUI(!needsAgent && !a.addresses?.length && <p>{textUI("m_60a89d2b2f85")}</p>)}{formatUI(needsAgent && a.agent_state !== 'online' && <p>{richUI("m_eec654726881", [<Link to="/assets/agents">{textUI("m_4a00b3b281ae")}</Link>])}</p>)}</>)}
 {formatUI(!needsAgent && offline.length > 0 && <p>{richUI("m_20fbc7e967ad", [formatUI(offline.map(i => agentDisplayName(i.name)).join(', ')), formatUI(can(me, 'integration.read') ? <Link to="/settings/integrations">{textUI("m_a95106196bce")}</Link> : textUI("m_07751b8877b9"))])}</p>)}
 </div>;
}
