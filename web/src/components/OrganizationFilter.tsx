import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useEffect, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { useMe } from '../api/hooks';
import { can } from '../api/types';
import { describeError } from './ErrorNote';
type Choice = {
    id: string;
    name: string;
    slug?: string;
};
// Filter yang sama dipakai halaman global dan tab organisasi. Kegagalan opsi tidak menghapus scope URL.
export function OrganizationFilter({ value, onChange, locked = false, name, allowUnassigned = false, required = false }: {
    value: string;
    onChange: (value: string) => void;
    locked?: boolean;
    name?: string;
    allowUnassigned?: boolean;
    // required: tidak ada "Semua organisasi"; satu-satunya organisasi dipilih otomatis.
    required?: boolean;
}) {
    useLocale();
    const { data: me } = useMe();
    const [opened, setOpened] = useState(false);
    const q = useInfiniteQuery({ queryKey: ['organization-options'], enabled: !locked && can(me, 'organization.read') && (opened || !!value || required), initialPageParam: '', queryFn: ({ pageParam }) => api.get<{
            items: Choice[];
            next_cursor: string | null;
        }>(`/api/v1/organizations?limit=100&cursor=${encodeURIComponent(pageParam)}`), getNextPageParam: last => last.next_cursor ?? undefined, retry: false });
    const items = q.data?.pages.flatMap(p => p.items) ?? [];
    useEffect(() => {
        if (required && !locked && !value && items.length === 1 && !q.hasNextPage) onChange(items[0]!.id);
    }, [required, locked, value, items.length, q.hasNextPage]);
    const selectedKnown = items.some(o => o.id === value) || value === '' || value === 'unassigned';
    return <div className="organization-filter"><label className="filter"><span className="sr-only">{textUI("m_350be3643ce7")}</span><select aria-label={textUI("m_350be3643ce7")} value={value} disabled={locked} onFocus={() => setOpened(true)} onChange={e => onChange(e.target.value)}>
 {formatUI(!locked && <option value="">{textUI(required ? "m_scan_choose_org" : "m_d75795799ab8")}</option>)}
 {formatUI(!locked && allowUnassigned && <option value="unassigned">{textUI("m_14d33bd014e6")}</option>)}
 {formatUI((!selectedKnown || locked) && <option value={value}>{formatUI(name || textUI("m_6fcd5de74842"))}</option>)}
 {formatUI(!locked && items.map(o => <option key={o.id} value={o.id}>{formatUI(o.name)}{formatUI(o.slug ? ` · ${o.slug}` : '')}</option>))}
 </select></label>{formatUI(q.hasNextPage && <button type="button" className="btn btn--secondary btn--auto" disabled={q.isFetchingNextPage} onClick={() => q.fetchNextPage()}>{textUI("m_cc25cf56fca2")}</button>)}
 {formatUI(q.isError && <p role="alert" className="small">{richUI("m_496506682a31", [formatUI(describeError(q.error).message), <button type="button" className="link-quiet" onClick={() => q.refetch()}>{textUI("m_d8b8392e2c54")}</button>])}</p>)}
 </div>;
}
