import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
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
export function EditAssetDialog({ asset, onClose, onStale }: {
    asset: Asset;
    onClose: () => void;
    onStale: () => void;
}) {
    useLocale();
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
    const canEditAddresses = !asset.testing_authorized || can(me, 'asset.authorize');
    const err = patch.error;
    const stale = err instanceof ApiError && err.status === 409;
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const touch = () => patch.isError && patch.reset();
    const submit = (e: FormEvent) => {
        e.preventDefault();
        const lines = addresses.split(/[\n,]+/).map((l) => l.trim()).filter(Boolean);
        patch.mutate({
            version: asset.version,
            name,
            ...(platform ? { platform } : asset.platform ? { clear_platform: true } : {}),
            ...(arch ? { arch } : asset.arch ? { clear_arch: true } : {}),
            criticality,
            is_internet_facing: internet,
            ...(can(me, 'asset.authorize') ? { testing_authorized: authorized } : {}),
            ...(canEditAddresses ? { addresses: lines.map((value) => ({ kind: inferKind(value), value })) } : {}),
            tag_ids: tagIds,
        }, { onSuccess: onClose });
    };
    return (<Modal labelledBy="edit-asset-title" onClose={onClose}>
      <header className="dialog__head">
        <h2 id="edit-asset-title" className="dialog__title">{textUI("m_fb1a47df691d")}</h2>
        <button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}><X size={18}/></button>
      </header>
      <form className="dialog__form" onSubmit={submit} noValidate>
        {formatUI(err && !(err instanceof ApiError && err.status === 422) && (<div className="alert" role="alert">
            <strong>{formatUI(stale ? textUI("m_8328027f01fd") : textUI("m_a6e065bf64d9"))}</strong>
            <span>{formatUI(describeError(err).message)}{formatUI(describeError(err).errorId ? textUI("m_488d61c8380b", [describeError(err).errorId]) : '')}</span>
            {formatUI(stale && <button type="button" className="btn btn--secondary btn--auto btn--sm" onClick={onStale}>{textUI("m_a5c9bd568be6")}</button>)}
          </div>))}
        <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')}/>
        <div className="row">
          <div className="select"><label htmlFor="ed-platform">{textUI("m_c78ffe195710")}</label>
            <select id="ed-platform" value={platform} onChange={(e) => { touch(); setPlatform(e.target.value); }}>
              <option value="">{textUI("m_4895f73177ab")}</option><option value="windows">{textUI("m_d598026a9cbc")}</option><option value="linux">{textUI("m_4828e60247c1")}</option>
              <option value="macos">{textUI("m_aed6b7aa2a05")}</option><option value="ios">{textUI("m_355f5e748485")}</option><option value="android">{textUI("m_6d612a86bee4")}</option><option value="other">{textUI("m_f97e9da0e3b8")}</option>
            </select></div>
          <div className="select"><label htmlFor="ed-arch">{textUI("m_cd74053c5481")}</label>
            <select id="ed-arch" value={arch} onChange={(e) => { touch(); setArch(e.target.value); }}>
              <option value="">{textUI("m_4895f73177ab")}</option><option value="x86_64">{textUI("m_7520b5a1b312")}</option><option value="arm64">{textUI("m_f69162950f23")}</option><option value="other">{textUI("m_f97e9da0e3b8")}</option>
            </select></div>
          <div className="select"><label htmlFor="ed-crit">{textUI("m_afdc3ea3ef86")}</label>
            <select id="ed-crit" value={criticality} onChange={(e) => { touch(); setCriticality(e.target.value); }}>
              <option value="low">{textUI("m_f793de205ead")}</option><option value="medium">{textUI("m_8e588cd18774")}</option><option value="high">{textUI("m_c4ebc6d4a583")}</option><option value="critical">{textUI("m_427dd2969bd1")}</option>
            </select></div>
        </div>
        <div className="textarea">
          <label htmlFor="ed-addresses">{textUI("m_76179aafc43c")}</label>
          <textarea id="ed-addresses" disabled={!canEditAddresses} rows={3} value={addresses} aria-invalid={!!fieldErr('addresses')} aria-describedby="ed-addresses-hint" onChange={(e) => { touch(); setAddresses(e.target.value); }}/>
          <small id="ed-addresses-hint" className={fieldErr('addresses') ? 'is-error' : ''} role={fieldErr('addresses') ? 'alert' : undefined}>
            {formatUI(fieldErr('addresses') ?? (canEditAddresses ? textUI("m_d105d1da575a") : textUI("m_3dce808e1410")))}
          </small>
        </div>
        <TagPicker value={tagIds} onChange={(v) => { touch(); setTagIds(v); }} canCreate/>
        <label className="check">{richUI("m_7a59eaa2b03d", [<input type="checkbox" checked={internet} onChange={(e) => { touch(); setInternet(e.target.checked); }}/>, <span className="check__box" aria-hidden="true"/>])}</label>
        {formatUI(can(me, 'asset.authorize') && (<label className="check"><input type="checkbox" checked={authorized} onChange={(e) => { touch(); setAuthorized(e.target.checked); }}/><span className="check__box" aria-hidden="true"/>
            <span>{richUI("m_21b0282172a5", [<small className="check__sub">{textUI("m_cc2513acdabd")}</small>])}</span></label>))}
        <footer className="dialog__foot">
          <button type="button" className="btn btn--secondary btn--auto" onClick={onClose}>{textUI("m_19766ed6ccb2")}</button>
          <button type="submit" className="btn btn--primary btn--auto" disabled={patch.isPending}>{formatUI(patch.isPending ? textUI("m_23e39291d613") : textUI("m_dd0ae7a5cbcf"))}</button>
        </footer>
      </form>
    </Modal>);
}
