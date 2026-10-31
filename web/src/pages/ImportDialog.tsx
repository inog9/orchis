import { FileUp, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { ApiError } from '../api/client';
import { useImportAssets, useImportPreview } from '../api/hooks';
import type { ImportPreview, ImportResult } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Modal } from '../components/Modal';

const TARGETS: { value: string; label: string }[] = [
  { value: 'host', label: 'Hostname or IP' },
  { value: 'name', label: 'Name' },
  { value: 'address', label: 'Extra address' },
  { value: 'category', label: 'Category' },
  { value: 'platform', label: 'Platform' },
  { value: 'arch', label: 'Architecture' },
  { value: 'criticality', label: 'Criticality' },
  { value: 'tags', label: 'Tags' },
  { value: 'ignore', label: 'Ignore this column' },
];

export const TEMPLATE = 'hostname,platform,arch,criticality,tags\nsrv-app-01,linux,x86_64,high,web;prod\nsrv-db-01,windows,x86_64,critical,database\n';
const MAX_BYTES = 1 << 20;

function Issues({ title, items }: { title: string; items: { row: number; message: string }[] }) {
  if (items.length === 0) return null;
  return (
    <div className="issues">
      <h3>{title}</h3>
      <ul>
        {items.slice(0, 6).map((i) => <li key={i.row + i.message}><span className="mono">Row {i.row}</span> {i.message}</li>)}
      </ul>
      {items.length > 6 && <p className="muted small">and {items.length - 6} more.</p>}
    </div>
  );
}

export function ImportDialog({ onClose }: { onClose: () => void }) {
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
    if (!f) return;
    if (f.size > MAX_BYTES) { setFileError('The file is too large (max 1 MB).'); return; }
    const text = await f.text();
    setCsv(text);
    setFileName(f.name);
    preview.mutate({ csv: text }, { onSuccess: (r) => { setMapping(r.suggested_mapping); setPv(r); } });
  };

  // Validasi ulang setiap kali pemetaan berubah (server memutuskan; di sini hanya menampilkan).
  useEffect(() => {
    if (!csv || Object.keys(mapping).length === 0) return;
    const t = setTimeout(() => preview.mutate({ csv, mapping }, { onSuccess: setPv }), 250);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [csv, mapping]);

  const download = () => {
    const url = URL.createObjectURL(new Blob([TEMPLATE], { type: 'text/csv' }));
    const a = document.createElement('a');
    a.href = url; a.download = 'assets-template.csv'; a.click();
    URL.revokeObjectURL(url);
  };

  const needsFix = pv?.validated ? pv.invalid.length : 0;
  const canImport = !!pv?.validated && pv.valid > 0 && (needsFix === 0 || skip) && !run.isPending;
  const err = preview.error ?? run.error;
  const errMsg = err instanceof ApiError && err.status === 422 ? (err.fields[0]?.message ?? err.detail) : err ? describeError(err).message : '';

  return (
    <Modal labelledBy="import-title" onClose={onClose} width={760}>
      <header className="dialog__head">
        <div><h2 id="import-title" className="dialog__title">Import from CSV</h2><p className="muted">Add many assets in one go.</p></div>
        <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button>
      </header>

      {result ? (
        <div className="import-done" role="status">
          <h3>Imported {result.created} {result.created === 1 ? 'asset' : 'assets'}</h3>
          <p className="muted">
            {result.skipped_duplicates > 0 && `${result.skipped_duplicates} duplicate(s) skipped. `}
            {result.skipped_invalid > 0 && `${result.skipped_invalid} row(s) that needed fixing were skipped. `}
            {result.tags_created > 0 && `${result.tags_created} new tag(s) created. `}
            Imported assets are not authorized for testing; authorize each one when you are ready.
          </p>
          <footer className="dialog__foot"><button type="button" className="btn btn--primary btn--auto" onClick={onClose}>Done</button></footer>
        </div>
      ) : (
        <div className="dialog__form">
          <div className="filepick">
            <label className="btn btn--secondary btn--auto" htmlFor="csv-file"><FileUp size={15} aria-hidden="true" /> {fileName ? 'Replace file' : 'Choose a CSV file'}</label>
            <input id="csv-file" className="sr-only" type="file" accept=".csv,text/csv" onChange={(e) => onFile(e.target.files?.[0])} />
            <span className="muted small">{fileName || 'Up to 1,000 rows and 1 MB.'}</span>
            <button type="button" className="link-subtle" onClick={download}>Download template</button>
          </div>
          {fileError && <p className="field__error" role="alert">{fileError}</p>}
          {errMsg && <div className="alert" role="alert"><strong>We couldn’t read that</strong><span>{errMsg}</span></div>}

          {pv && (
            <>
              <h3 className="panel__title">Match your columns</h3>
              <table className="map">
                <thead><tr><th>Column</th><th>Example</th><th>Import as</th></tr></thead>
                <tbody>
                  {pv.columns.map((c, i) => (
                    <tr key={c.name}>
                      <td className="mono">{c.name}</td>
                      <td className="muted small">{c.sample.filter(Boolean).slice(0, 2).join(', ') || '—'}</td>
                      <td>
                        <select aria-label={`Import column ${c.name} as`} value={mapping[c.name] ?? 'ignore'} onChange={(e) => setMapping((m) => ({ ...m, [c.name]: e.target.value }))} id={`map-${i}`}>
                          {TARGETS.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
                        </select>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {pv.validated && (
                <div className="summary" role="status" aria-live="polite">
                  <span className="status status--ok">{pv.valid} valid</span>
                  <span className={`status ${needsFix ? 'status--bad' : ''}`}>{needsFix} need fixing</span>
                  <span className="status">{pv.duplicates.length} duplicate{pv.duplicates.length === 1 ? '' : 's'}</span>
                </div>
              )}
              {pv.validated && <Issues title="Rows that need fixing" items={pv.invalid} />}
              {pv.validated && <Issues title="Duplicates (will be skipped)" items={pv.duplicates} />}
              {needsFix > 0 && (
                <label className="check"><input type="checkbox" checked={skip} onChange={(e) => setSkip(e.target.checked)} /><span className="check__box" aria-hidden="true" />Skip the rows that need fixing and import the rest</label>
              )}
            </>
          )}
          <footer className="dialog__foot">
            <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
            <button type="button" className="btn btn--primary btn--auto" disabled={!canImport}
              onClick={() => run.mutate({ csv, mapping, skip_invalid: skip }, { onSuccess: setResult })}>
              {run.isPending ? 'Importing…' : pv?.validated ? `Import ${pv.valid} ${pv.valid === 1 ? 'asset' : 'assets'}` : 'Import'}
            </button>
          </footer>
        </div>
      )}
    </Modal>
  );
}
