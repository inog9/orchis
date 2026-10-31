import { X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useMe, usePatchAsset } from '../api/hooks';
import { can, type Asset } from '../api/types';
import { describeError } from '../components/ErrorNote';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { TagPicker } from '../components/TagPicker';
import { inferKind } from './categories';

export function EditAssetDialog({ asset, onClose, onStale }: { asset: Asset; onClose: () => void; onStale: () => void }) {
  const { data: me } = useMe();
  const patch = usePatchAsset(asset.id);
  const [name, setName] = useState(asset.name);
  const [platform, setPlatform] = useState(asset.platform ?? '');
  const [arch, setArch] = useState(asset.arch ?? '');
  const [criticality, setCriticality] = useState<string>(asset.criticality);
  const [addresses, setAddresses] = useState(asset.addresses.map((a) => a.value).join('\n'));
  const [internet, setInternet] = useState(asset.is_internet_facing);
  const [authorized, setAuthorized] = useState(asset.testing_authorized);
  const [tagIds, setTagIds] = useState(asset.tags.map((t) => t.id));

  const err = patch.error;
  const stale = err instanceof ApiError && err.status === 409;
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
  const touch = () => patch.isError && patch.reset();

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const lines = addresses.split(/[\n,]+/).map((l) => l.trim()).filter(Boolean);
    patch.mutate(
      {
        version: asset.version,
        name,
        ...(platform ? { platform } : asset.platform ? { clear_platform: true } : {}),
        ...(arch ? { arch } : asset.arch ? { clear_arch: true } : {}),
        criticality,
        is_internet_facing: internet,
        ...(can(me, 'asset.authorize') ? { testing_authorized: authorized } : {}),
        addresses: lines.map((value) => ({ kind: inferKind(value), value })),
        tag_ids: tagIds,
      },
      { onSuccess: onClose },
    );
  };

  return (
    <Modal labelledBy="edit-asset-title" onClose={onClose}>
      <header className="dialog__head">
        <h2 id="edit-asset-title" className="dialog__title">Edit asset</h2>
        <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}><X size={18} /></button>
      </header>
      <form className="dialog__form" onSubmit={submit} noValidate>
        {err && !(err instanceof ApiError && err.status === 422) && (
          <div className="alert" role="alert">
            <strong>{stale ? 'This asset changed' : 'We couldn’t save the changes'}</strong>
            <span>{describeError(err).message}{describeError(err).errorId ? ` Error id: ${describeError(err).errorId}` : ''}</span>
            {stale && <button type="button" className="btn btn--secondary btn--auto btn--sm" onClick={onStale}>Reload the latest version</button>}
          </div>
        )}
        <Field label="Name" value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} />
        <div className="row">
          <div className="select"><label htmlFor="ed-platform">Platform</label>
            <select id="ed-platform" value={platform} onChange={(e) => { touch(); setPlatform(e.target.value); }}>
              <option value="">Not set</option><option value="windows">Windows</option><option value="linux">Linux</option>
              <option value="macos">macOS</option><option value="ios">iOS</option><option value="android">Android</option><option value="other">Other</option>
            </select></div>
          <div className="select"><label htmlFor="ed-arch">Architecture</label>
            <select id="ed-arch" value={arch} onChange={(e) => { touch(); setArch(e.target.value); }}>
              <option value="">Not set</option><option value="x86_64">x86_64</option><option value="arm64">arm64</option><option value="other">Other</option>
            </select></div>
          <div className="select"><label htmlFor="ed-crit">Criticality</label>
            <select id="ed-crit" value={criticality} onChange={(e) => { touch(); setCriticality(e.target.value); }}>
              <option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
            </select></div>
        </div>
        <div className="textarea">
          <label htmlFor="ed-addresses">Addresses</label>
          <textarea id="ed-addresses" rows={3} value={addresses} aria-invalid={!!fieldErr('addresses')} aria-describedby="ed-addresses-hint"
            onChange={(e) => { touch(); setAddresses(e.target.value); }} />
          <small id="ed-addresses-hint" className={fieldErr('addresses') ? 'is-error' : ''} role={fieldErr('addresses') ? 'alert' : undefined}>
            {fieldErr('addresses') ?? 'One per line: IP, CIDR, hostname, domain, URL or MAC.'}
          </small>
        </div>
        <TagPicker value={tagIds} onChange={(v) => { touch(); setTagIds(v); }} canCreate />
        <label className="check"><input type="checkbox" checked={internet} onChange={(e) => { touch(); setInternet(e.target.checked); }} /><span className="check__box" aria-hidden="true" />Reachable from the internet</label>
        {can(me, 'asset.authorize') && (
          <label className="check"><input type="checkbox" checked={authorized} onChange={(e) => { touch(); setAuthorized(e.target.checked); }} /><span className="check__box" aria-hidden="true" />
            <span>This asset may be tested<small className="check__sub">Only authorized assets can be targeted by scans and simulations.</small></span></label>
        )}
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn--primary btn--auto" disabled={patch.isPending}>{patch.isPending ? 'Saving…' : 'Save changes'}</button>
        </footer>
      </form>
    </Modal>
  );
}
