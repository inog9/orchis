import { X } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { ApiError } from '../api/client';
import { useCreateAsset, useMe } from '../api/hooks';
import { can } from '../api/types';
import { Field } from '../components/Field';
import { Modal } from '../components/Modal';
import { TagPicker } from '../components/TagPicker';
import { describeError } from '../components/ErrorNote';
import { CATEGORIES, categoryOf, inferKind } from './categories';

export function AddAssetDialog({ onClose, organizationID }: { onClose: () => void; organizationID?: string }) {
  const { data: me } = useMe();
  const create = useCreateAsset();
  const [category, setCategory] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [platform, setPlatform] = useState('');
  const [arch, setArch] = useState('');
  const [criticality, setCriticality] = useState('medium');
  const [addresses, setAddresses] = useState('');
  const [internet, setInternet] = useState(false);
  const [authorized, setAuthorized] = useState(false);
  const [tagIds, setTagIds] = useState<string[]>([]);

  const err = create.error;
  // Galat server milik isian sebelumnya; hilangkan begitu pengguna mengubah sesuatu agar tidak menyesatkan.
  const touch = () => create.isError && create.reset();
  const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!category) return;
    const lines = addresses.split(/[\n,]+/).map((l) => l.trim()).filter(Boolean);
    create.mutate(
      {
        category,
        ...(organizationID?{organization_id:organizationID}:{}),
        name,
        platform: platform || null,
        arch: arch || null,
        criticality,
        is_internet_facing: internet,
        testing_authorized: authorized,
        addresses: lines.map((value) => ({ kind: inferKind(value), value })),
        tag_ids: tagIds,
      },
      { onSuccess: onClose },
    );
  };

  const cat = category ? categoryOf(category) : null;
  return (
    <Modal labelledBy="add-asset-title" onClose={onClose}>
      <>
        <header className="dialog__head">
          <div>
            <h2 id="add-asset-title">{cat ? `New ${cat.label.toLowerCase()}` : 'Create a new asset'}</h2>
            <ol className="steps" aria-label="Progress">
              <li className={category ? 'is-done' : 'is-current'}>1 Category</li>
              <li className={category ? 'is-current' : ''}>2 Details</li>
            </ol>
          </div>
          <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
            <X size={18} />
          </button>
        </header>

        {!cat ? (
          <div className="category-grid">
            {CATEGORIES.map((c) => {
              const Icon = c.icon;
              return (
                <button key={c.key} type="button" className="category" onClick={() => setCategory(c.key)}>
                  <span className="category__icon">
                    <Icon size={18} />
                  </span>
                  <span className="category__text">
                    <strong>{c.label}</strong>
                    <span>{c.description}</span>
                  </span>
                </button>
              );
            })}
          </div>
        ) : (
          <form className="dialog__form" onSubmit={submit} noValidate>
            {err && !(err instanceof ApiError && err.status === 422) && (
              <div className="alert" role="alert">
                <strong>We couldn’t create the asset</strong>
                <span>
                  {describeError(err).message}
                  {describeError(err).errorId ? ` Error id: ${describeError(err).errorId}` : ''}
                </span>
              </div>
            )}
            <Field label="Name" value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} placeholder="srv-db-01" autoFocus />
            <div className="row">
              <div className="select">
                <label htmlFor="asset-platform">Platform</label>
                <select id="asset-platform" value={platform} onChange={(e) => { touch(); setPlatform(e.target.value); }}>
                  <option value="">Not set</option>
                  <option value="windows">Windows</option>
                  <option value="linux">Linux</option>
                  <option value="macos">macOS</option>
                  <option value="ios">iOS</option>
                  <option value="android">Android</option>
                  <option value="other">Other</option>
                </select>
              </div>
              <div className="select">
                <label htmlFor="asset-arch">Architecture</label>
                <select id="asset-arch" value={arch} onChange={(e) => { touch(); setArch(e.target.value); }}>
                  <option value="">Not set</option>
                  <option value="x86_64">x86_64</option>
                  <option value="arm64">arm64</option>
                  <option value="other">Other</option>
                </select>
              </div>
              <div className="select">
                <label htmlFor="asset-criticality">Criticality</label>
                <select id="asset-criticality" value={criticality} onChange={(e) => { touch(); setCriticality(e.target.value); }}>
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                  <option value="critical">Critical</option>
                </select>
              </div>
            </div>
            <div className="textarea">
              <label htmlFor="asset-addresses">Addresses</label>
              <textarea
                id="asset-addresses"
                rows={3}
                value={addresses}
                onChange={(e) => { touch(); setAddresses(e.target.value); }}
                placeholder={'10.0.0.5\nsrv-db-01.corp.local'}
                aria-invalid={!!fieldErr('addresses')}
                aria-describedby="asset-addresses-hint"
              />
              <small id="asset-addresses-hint" className={fieldErr('addresses') ? 'is-error' : ''} role={fieldErr('addresses') ? 'alert' : undefined}>
                {fieldErr('addresses') ?? 'One per line: IP, CIDR, hostname, domain, URL or MAC.'}
              </small>
            </div>
            <TagPicker value={tagIds} onChange={(v) => { touch(); setTagIds(v); }} canCreate />
            <label className="check">
              <input type="checkbox" checked={internet} onChange={(e) => setInternet(e.target.checked)} />
              <span className="check__box" aria-hidden="true" />
              Reachable from the internet
            </label>
            {can(me, 'asset.authorize') && (
              <label className="check">
                <input type="checkbox" checked={authorized} onChange={(e) => setAuthorized(e.target.checked)} />
                <span className="check__box" aria-hidden="true" />
                <span>
                  I confirm this asset may be tested
                  <small className="check__sub">Only authorized assets can be targeted by scans and simulations.</small>
                </span>
              </label>
            )}
            <footer className="dialog__foot">
              <button type="button" className="btn btn--secondary btn--auto" onClick={() => setCategory(null)}>
                Back
              </button>
              <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>
                {create.isPending ? 'Creating…' : 'Create asset'}
              </button>
            </footer>
          </form>
        )}
      </>
    </Modal>
  );
}
