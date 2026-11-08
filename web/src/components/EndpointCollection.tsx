import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { useEndpointCatalog, useOrgOsquery } from '../api/hooks';
import { describeError } from './ErrorNote';
import { catalogOS, collectionSupported } from '../lib/collectionSupport';
export function EndpointCollection({ org, platform, arch, canManage }: {
    org: string;
    platform: string;
    arch: string;
    canManage: boolean;
}) {
    useLocale();
    const cat = useEndpointCatalog();
    const settings = useOrgOsquery(org);
    const qc = useQueryClient();
    const supported = collectionSupported(platform, arch);
    const queries = cat.data?.queries.filter(q => q.os.includes(catalogOS(platform))) ?? [];
    const items = settings.data?.items ?? [];
    // Sebuah baris berlaku bila masih aktif di bawah dokumen persetujuan yang berlaku dan tercatat siapa penyetujunya.
    const current = items.filter(i => i.enabled && i.consent_version === cat.data?.consent.version && i.consent_sha256 === cat.data?.consent.sha256 && !!i.consented_by);
    const hasCurrent = current.length > 0;
    // Lingkup sekali-klik: pemeriksaan yang dipakai DefenseBot plus pack resmi yang bisa dijalankan.
    // Check kategori Endpoint/network (pengguna, jaringan, rentang) tidak dipakai DefenseBot dan dibiarkan seperti adanya.
    const scope = queries.filter(q => (q.category === 'DefenseBot' && !q.pack) || !!q.pack);
    // Yang belum aktif di bawah persetujuan berlaku; inilah alasan tombol muncul walau sebagian sudah aktif.
    const missing = scope.filter(q => !current.some(i => i.key === q.key));
    // Server menuntut versi untuk setiap kueri yang didukung platform ini, termasuk kueri pack; baris yang belum ada dianggap versi 0.
    const versions = Object.fromEntries(queries.map(q => [q.key, items.find(i => i.key === q.key)?.version ?? 0]));
    const save = useMutation({
        mutationFn: (keys: string[]) => api.put(`/api/v1/organizations/${org}/endpoint-collection`, { platform: catalogOS(platform), keys, versions, consent: { version: cat.data!.consent.version, sha256: cat.data!.consent.sha256 } }),
        onSuccess: async () => { await Promise.all([qc.invalidateQueries({ queryKey: ['osquery-org', org] }), qc.invalidateQueries({ queryKey: ['asset-endpoint'] })]); },
    });
    const interval = cat.data?.collection_interval_seconds;
    return (<section className="org-card" aria-label={textUI("m_6836bbf4c02c")}>
      <h3>{textUI("m_5237f7a69c42")}</h3>
      {formatUI(!supported ? <p className="notice">{richUI("m_e2626da97752", [formatUI(platform === 'macos' || platform === 'darwin' ? 'macOS' : platform), formatUI(arch)])}</p> : <>
        {formatUI(hasCurrent && <p className="muted">{typeof interval === 'number'
                ? richUI("m_ep_auto_status", [formatUI(current.length), formatUI(interval)])
                : `${formatUI(current.length)} ${formatUI(current.length === 1 ? textUI("m_55d808d06666") : textUI("m_9913ded4c50d"))}`}</p>)}
        {formatUI(hasCurrent && canManage && <p><button className="btn btn--secondary btn--auto" type="button" disabled={save.isPending} onClick={() => save.mutate([])}>{textUI("m_06f0e210b27d")}</button></p>)}
        {formatUI(missing.length > 0 && canManage && cat.data !== undefined && settings.data !== undefined && <p><button className="btn btn--primary btn--auto" disabled={save.isPending} onClick={() => save.mutate([...new Set([...current.map(i => i.key), ...scope.map(q => q.key)])])}>{formatUI(save.isPending ? textUI("m_ep_auto_enabling") : textUI("m_ep_auto_on"))}</button></p>)}
        {formatUI(!hasCurrent && missing.length === 0 && <p className="muted">{textUI("m_ep_auto_idle")}</p>)}
      </>)}
      {formatUI(save.error && <p className="alert" role="alert">{formatUI(describeError(save.error).message)}</p>)}
      {formatUI((cat.error || settings.error) && <p role="alert">{formatUI(describeError(cat.error || settings.error).message)}</p>)}
    </section>);
}
