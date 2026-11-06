import { t as textUI, useLocale, fv as formatUI } from '../../i18n/runtime';
import { ChevronLeft, ChevronRight } from 'lucide-react';

// Daftar nomor halaman dengan elipsis: 1 … 4 5 6 … 12
export function pageList(page: number, pages: number): (number | '…')[] {
    if (pages <= 7) return Array.from({ length: pages }, (_, i) => i + 1);
    const set = new Set([1, pages, page - 1, page, page + 1]);
    const nums = [...set].filter((n) => n >= 1 && n <= pages).sort((a, b) => a - b);
    const out: (number | '…')[] = [];
    nums.forEach((n, i) => { if (i && n - nums[i - 1]! > 1) out.push('…'); out.push(n); });
    return out;
}

// Disembunyikan bila semua item muat di satu halaman.
export function Pagination({ page, total, pageSize, onPage }: { page: number; total: number; pageSize: number; onPage: (p: number) => void }) {
    useLocale();
    const pages = Math.max(1, Math.ceil(total / pageSize));
    if (total <= pageSize) return null;
    const from = (page - 1) * pageSize + 1;
    const to = Math.min(total, page * pageSize);
    return (<nav className="pager" aria-label={textUI("m_orgs_pagination")}>
      <span className="pager__info mono">{textUI("m_orgs_showing", [from, to, total])}</span>
      <div className="pager__ctl">
        <button type="button" className="pager__btn" disabled={page <= 1} onClick={() => onPage(page - 1)}><ChevronLeft size={14} aria-hidden="true"/>{textUI("m_orgs_prev")}</button>
        {formatUI(pageList(page, pages).map((n, i) => n === '…'
            ? <span key={`e${i}`} className="pager__gap" aria-hidden="true">…</span>
            : <button key={n} type="button" className={`pager__btn pager__num${n === page ? ' is-current' : ''}`} aria-current={n === page ? 'page' : undefined} aria-label={textUI("m_orgs_page", [n])} onClick={() => onPage(n)}>{n}</button>))}
        <button type="button" className="pager__btn" disabled={page >= pages} onClick={() => onPage(page + 1)}>{textUI("m_orgs_next")}<ChevronRight size={14} aria-hidden="true"/></button>
      </div>
    </nav>);
}
