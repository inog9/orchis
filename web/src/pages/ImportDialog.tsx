import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { FileUp, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { ApiError } from '../api/client';
import { useImportAssets, useImportPreview } from '../api/hooks';
import type { ImportPreview, ImportResult } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';
const TARGETS: {
    value: string;
    label: string;
}[] = [
    { value: 'host', get label() {
            return textUI("m_9ad1f173f02e");
        } },
    { value: 'name', get label() {
            return textUI("m_dcd1d5223f73");
        } },
    { value: 'address', get label() {
            return textUI("m_4c43f2ac4e87");
        } },
    { value: 'category', get label() {
            return textUI("m_292c06f0045a");
        } },
    { value: 'platform', get label() {
            return textUI("m_c78ffe195710");
        } },
    { value: 'arch', get label() {
            return textUI("m_cd74053c5481");
        } },
    { value: 'criticality', get label() {
            return textUI("m_afdc3ea3ef86");
        } },
    { value: 'tags', get label() {
            return textUI("m_1331275bc537");
        } },
    { value: 'ignore', get label() {
            return textUI("m_995c0a91e8e0");
        } },
];
export const TEMPLATE = 'hostname,platform,arch,criticality,tags\nsrv-app-01,linux,x86_64,high,web;prod\nsrv-db-01,windows,x86_64,critical,database\n';
const MAX_BYTES = 1 << 20;
function Issues({ title, items }: {
    title: string;
    items: {
        row: number;
        message: string;
    }[];
}) {
    useLocale();
    if (items.length === 0)
        return null;
    return (<div className="issues">
      <h3>{formatUI(title)}</h3>
      <ul>
        {formatUI(items.slice(0, 6).map((i) => <li key={i.row + i.message}><span className="mono">{richUI("m_481ed9c3e4e4", [formatUI(i.row)])}</span> {formatUI(i.message)}</li>))}
      </ul>
      {formatUI(items.length > 6 && <p className="muted small">{richUI("m_d5d455437e1a", [formatUI(items.length - 6)])}</p>)}
    </div>);
}
export function ImportDialog({ onClose }: {
    onClose: () => void;
}) {
    useLocale();
    const preview = useImportPreview();
    const run = useImportAssets();
    const [csv, setCsv] = useState('');
    const [fileName, setFileName] = useState('');
    const [fileError, setFileError] = useState('');
    const [mapping, setMapping] = useState<Record<string, string>>({});
    const [skip, setSkip] = useState(false);
    const [result, setResult] = useState<ImportResult | null>(null);
    const [pv, setPv] = useState<ImportPreview | null>(null);
    const onFile = async (f: File | undefined) => {
        setFileError('');
        setPv(null);
        setMapping({});
        if (!f)
            return;
        if (f.size > MAX_BYTES) {
            setFileError(textUI("m_fdb1e35eb874"));
            return;
        }
        const text = await f.text();
        setCsv(text);
        setFileName(f.name);
        preview.mutate({ csv: text }, { onSuccess: (r) => { setMapping(r.suggested_mapping); setPv(r); } });
    };
    // Validasi ulang setiap kali pemetaan berubah (server memutuskan; di sini hanya menampilkan).
    useEffect(() => {
        if (!csv || Object.keys(mapping).length === 0)
            return;
        const t = setTimeout(() => preview.mutate({ csv, mapping }, { onSuccess: setPv }), 250);
        return () => clearTimeout(t);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [csv, mapping]);
    const download = () => {
        const url = URL.createObjectURL(new Blob([TEMPLATE], { type: 'text/csv' }));
        const a = document.createElement('a');
        a.href = url;
        a.download = 'assets-template.csv';
        a.click();
        URL.revokeObjectURL(url);
    };
    const needsFix = pv?.validated ? pv.invalid.length : 0;
    const canImport = !!pv?.validated && pv.valid > 0 && (needsFix === 0 || skip) && !run.isPending;
    const err = preview.error ?? run.error;
    const errMsg = err instanceof ApiError && err.status === 422 ? (err.fields[0]?.message ?? err.detail) : err ? describeError(err).message : '';
    return (<Modal labelledBy="import-title" onClose={onClose} width={760}>
      <header className="dialog__head">
        <div><h2 id="import-title" className="dialog__title">{textUI("m_9b1685f5ca12")}</h2><p className="muted">{textUI("m_2f64576de343")}</p></div>
        <button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button>
      </header>

      {formatUI(result ? (<div className="import-done" role="status">
          <h3>{richUI("m_7a5cf85d4573", [formatUI(result.created), formatUI(result.created === 1 ? 'asset' : 'assets')])}</h3>
          <p className="muted">{richUI("m_fbbf83f158ab", [formatUI(result.skipped_duplicates > 0 && `${result.skipped_duplicates} duplicate(s) skipped. `), formatUI(result.skipped_invalid > 0 && `${result.skipped_invalid} row(s) that needed fixing were skipped. `), formatUI(result.tags_created > 0 && `${result.tags_created} new tag(s) created. `)])}</p>
          <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>{textUI("m_11a6767d5674")}</button></footer>
        </div>) : (<div className="dialog__form">
          <div className="filepick">
            <label className="btn btn--secondary btn--auto" htmlFor="csv-file"><FileUp size={15} aria-hidden="true"/> {formatUI(fileName ? textUI("m_68ec54a908b7") : textUI("m_5a89a6161172"))}</label>
            <input id="csv-file" className="sr-only" type="file" accept=".csv,text/csv" onChange={(e) => onFile(e.target.files?.[0])}/>
            <span className="muted small">{formatUI(fileName || textUI("m_97d335008016"))}</span>
            <button type="button" className="link-subtle" onClick={download}>{textUI("m_332b74013ffe")}</button>
          </div>
          {formatUI(fileError && <p className="field__error" role="alert">{formatUI(fileError)}</p>)}
          {formatUI(errMsg && <div className="alert" role="alert"><strong>{textUI("m_5cbaacd77db0")}</strong><span>{formatUI(errMsg)}</span></div>)}

          {formatUI(pv && (<>
              <h3 className="panel__title">{textUI("m_c428ba61f9b9")}</h3>
              <table className="map">
                <thead><tr><th>{textUI("m_3fdc9a587693")}</th><th>{textUI("m_d029f87e3d80")}</th><th>{textUI("m_ab3fa4e6af9d")}</th></tr></thead>
                <tbody>
                  {formatUI(pv.columns.map((c, i) => (<tr key={c.name}>
                      <td className="mono">{formatUI(c.name)}</td>
                      <td className="muted small">{formatUI(c.sample.filter(Boolean).slice(0, 2).join(', ') || '—')}</td>
                      <td>
                        <select aria-label={textUI("m_17ab12e8e7ca", [c.name])} value={mapping[c.name] ?? 'ignore'} onChange={(e) => setMapping((m) => ({ ...m, [c.name]: e.target.value }))} id={`map-${i}`}>
                          {formatUI(TARGETS.map((t) => <option key={t.value} value={t.value}>{formatUI(t.label)}</option>))}
                        </select>
                      </td>
                    </tr>)))}
                </tbody>
              </table>
              {formatUI(pv.validated && (<div className="summary" role="status" aria-live="polite">
                  <span className="status status--ok">{richUI("m_f1a2a29e8b29", [formatUI(pv.valid)])}</span>
                  <span className={`status ${needsFix ? 'status--bad' : ''}`}>{richUI("m_647d169fbbd1", [formatUI(needsFix)])}</span>
                  <span className="status">{richUI("m_87f004f2b90a", [formatUI(pv.duplicates.length), formatUI(pv.duplicates.length === 1 ? '' : 's')])}</span>
                </div>))}
              {formatUI(pv.validated && <Issues title={textUI("m_eb808f932575")} items={pv.invalid}/>)}
              {formatUI(pv.validated && <Issues title={textUI("m_180df228b9b0")} items={pv.duplicates}/>)}
              {formatUI(needsFix > 0 && (<label className="check">{richUI("m_7291d0ddb939", [<input type="checkbox" checked={skip} onChange={(e) => setSkip(e.target.checked)}/>, <span className="check__box" aria-hidden="true"/>])}</label>))}
            </>))}
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
            <button type="button" className="btn btn--primary btn--auto" disabled={!canImport} onClick={() => run.mutate({ csv, mapping, skip_invalid: skip }, { onSuccess: setResult })}>
              {formatUI(run.isPending ? textUI("m_c01c4324f1fa") : pv?.validated ? textUI("m_c76603c5d08b", [pv.valid, pv.valid === 1 ? 'asset' : 'assets']) : textUI("m_2cff9baabf56"))}
            </button>
          </footer>
        </div>))}
    </Modal>);
}
