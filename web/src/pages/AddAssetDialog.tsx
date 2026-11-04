import { t as textUI, tr as richUI, useLocale, fv as formatUI } from "../i18n/runtime";
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
import { OrganizationFilter } from '../components/OrganizationFilter';
import { CategoryFields, attributesOf, missingRequired } from './CategoryFields';
import { badAddress, kindsText, specOf } from './categorySchema';
export function AddAssetDialog({ onClose, organizationID }: {
    onClose: () => void;
    organizationID?: string;
}) {
    useLocale();
    const { data: me } = useMe();
    const create = useCreateAsset();
    const [category, setCategory] = useState<string | null>(null);
    const [orgChoice, setOrgChoice] = useState('');
    const [name, setName] = useState('');
    const [platform, setPlatform] = useState('');
    const [arch, setArch] = useState('');
    const [criticality, setCriticality] = useState('medium');
    const [addresses, setAddresses] = useState('');
    const [internet, setInternet] = useState(false);
    const [authorized, setAuthorized] = useState(false);
    const [tagIds, setTagIds] = useState<string[]>([]);
    const [values, setValues] = useState<Record<string, string>>({});
    const [clientErrors, setClientErrors] = useState<Record<string, string>>({});
    const err = create.error;
    // Galat server milik isian sebelumnya; hilangkan begitu pengguna mengubah sesuatu agar tidak menyesatkan.
    const touch = () => create.isError && create.reset();
    const fieldErr = (n: string) => (err instanceof ApiError ? err.fields.find((f) => f.name === n)?.message : undefined);
    const submit = (e: FormEvent) => {
        e.preventDefault();
        if (!category)
            return;
        const spec = specOf(category);
        const lines = addresses.split(/[\n,]+/).map((l) => l.trim()).filter(Boolean);
        const problems = missingRequired(spec, values);
        if (spec.address === 'required' && lines.length === 0)
            problems.addresses = textUI("m_cf_err_addr_required", [kindsText(spec.kinds)]);
        const bad = spec.address === 'hidden' ? null : badAddress(lines, spec.kinds, inferKind);
        if (bad)
            problems.addresses = textUI("m_cf_err_line", [bad, kindsText(spec.kinds)]);
        setClientErrors(problems);
        if (Object.keys(problems).length)
            return;
        create.mutate({
            category,
            ...((organizationID || orgChoice) ? { organization_id: organizationID || orgChoice } : {}),
            name,
            platform: spec.platform ? (platform || null) : null,
            arch: spec.arch ? (arch || null) : null,
            criticality,
            is_internet_facing: spec.internet && internet,
            testing_authorized: spec.testable && authorized,
            addresses: spec.address === 'hidden' ? [] : lines.map((value) => ({ kind: inferKind(value), value })),
            attributes: attributesOf(spec, values),
            ...((values.__notes ?? '').trim() ? { description: (values.__notes ?? '').trim() } : {}),
            tag_ids: tagIds,
        }, { onSuccess: onClose });
    };
    const cat = category ? categoryOf(category) : null;
    const spec = category ? specOf(category) : null;
    const setValue = (k: string, v: string) => { touch(); setClientErrors((c) => ({ ...c, [k]: '' })); setValues((x) => ({ ...x, [k]: v })); };
    return (<Modal labelledBy="add-asset-title" onClose={onClose}>
      <>
        <header className="dialog__head">
          <div>
            <h2 id="add-asset-title">{formatUI(cat ? textUI("m_483166d9eabb", [cat.label.toLowerCase()]) : textUI("m_06cb0d31b1b7"))}</h2>
            <ol className="steps" aria-label={textUI("m_4664827f8e89")}>
              <li className={category ? 'is-done' : 'is-current'}>{textUI("m_c2888b84cba7")}</li>
              <li className={category ? 'is-current' : ''}>{textUI("m_1a631d591dd7")}</li>
            </ol>
          </div>
          <button type="button" className="icon-btn" aria-label={textUI("m_7d9eb7acb13e")} onClick={onClose}>
            <X size={18}/>
          </button>
        </header>

        {formatUI(!cat ? (<div className="category-grid">
            {formatUI(CATEGORIES.map((c) => {
                const Icon = c.icon;
                return (<button key={c.key} type="button" className="category" onClick={() => setCategory(c.key)}>
                  <span className="category__icon">
                    <Icon size={18}/>
                  </span>
                  <span className="category__text">
                    <strong>{formatUI(c.label)}</strong>
                    <span>{formatUI(c.description)}</span>
                  </span>
                </button>);
            }))}
          </div>) : (<form className="dialog__form" onSubmit={submit} noValidate>
            {formatUI(err && !(err instanceof ApiError && err.status === 422) && (<div className="alert" role="alert">
                <strong>{textUI("m_a4b51e19d81d")}</strong>
                <span>
                  {formatUI(describeError(err).message)}
                  {formatUI(describeError(err).errorId ? textUI("m_488d61c8380b", [describeError(err).errorId]) : '')}
                </span>
              </div>))}
            {formatUI(!organizationID && (<div className="field"><span className="label">{textUI("m_asset_org_label")}</span><OrganizationFilter value={orgChoice} onChange={(v) => { touch(); setOrgChoice(v); }} required/><p className="hint">{textUI("m_asset_org_hint")}</p></div>))}
            <Field label={textUI("m_dcd1d5223f73")} value={name} onChange={(e) => { touch(); setName(e.target.value); }} error={fieldErr('name')} placeholder={textUI("m_368bcf1c9887")} autoFocus/>
            {formatUI(spec && (<>
            <div className="row">
              {formatUI(spec.platform && (<div className="select">
                <label htmlFor="asset-platform">{textUI("m_c78ffe195710")}</label>
                <select id="asset-platform" value={platform} onChange={(e) => { touch(); setPlatform(e.target.value); }}>
                  <option value="">{textUI("m_4895f73177ab")}</option>
                  <option value="windows">{textUI("m_d598026a9cbc")}</option>
                  <option value="linux">{textUI("m_4828e60247c1")}</option>
                  <option value="macos">{textUI("m_aed6b7aa2a05")}</option>
                  <option value="ios">{textUI("m_355f5e748485")}</option>
                  <option value="android">{textUI("m_6d612a86bee4")}</option>
                  <option value="other">{textUI("m_f97e9da0e3b8")}</option>
                </select>
              </div>))}
              {formatUI(spec.arch && (<div className="select">
                <label htmlFor="asset-arch">{textUI("m_cd74053c5481")}</label>
                <select id="asset-arch" value={arch} onChange={(e) => { touch(); setArch(e.target.value); }}>
                  <option value="">{textUI("m_4895f73177ab")}</option>
                  <option value="x86_64">{textUI("m_7520b5a1b312")}</option>
                  <option value="arm64">{textUI("m_f69162950f23")}</option>
                  <option value="other">{textUI("m_f97e9da0e3b8")}</option>
                </select>
              </div>))}
              <div className="select">
                <label htmlFor="asset-criticality">{textUI("m_afdc3ea3ef86")}</label>
                <select id="asset-criticality" value={criticality} onChange={(e) => { touch(); setCriticality(e.target.value); }}>
                  <option value="low">{textUI("m_f793de205ead")}</option>
                  <option value="medium">{textUI("m_8e588cd18774")}</option>
                  <option value="high">{textUI("m_c4ebc6d4a583")}</option>
                  <option value="critical">{textUI("m_427dd2969bd1")}</option>
                </select>
              </div>
            </div>
            <CategoryFields spec={spec} values={values} onChange={setValue} errors={clientErrors}/>
            {formatUI(spec.address !== 'hidden' && (<div className="textarea">
              <label htmlFor="asset-addresses">{formatUI(spec.addressLabel)}</label>
              <textarea id="asset-addresses" rows={3} value={addresses} onChange={(e) => { touch(); setClientErrors((c) => ({ ...c, addresses: '' })); setAddresses(e.target.value); }} placeholder={spec.addressPlaceholder} aria-invalid={!!(clientErrors.addresses || fieldErr('addresses'))} aria-describedby="asset-addresses-hint"/>
              <small id="asset-addresses-hint" className={(clientErrors.addresses || fieldErr('addresses')) ? 'is-error' : ''} role={(clientErrors.addresses || fieldErr('addresses')) ? 'alert' : undefined}>
                {formatUI(clientErrors.addresses || fieldErr('addresses') || spec.addressHint)}
              </small>
            </div>))}
            {formatUI((spec.address === 'hidden' || (spec.address === 'optional' && !addresses.trim())) && <p className="notice" role="note">{textUI("m_cf_inventory")}</p>)}
            <TagPicker value={tagIds} onChange={(v) => { touch(); setTagIds(v); }} canCreate/>
            {formatUI(spec.internet && (<label className="check">{richUI("m_7a59eaa2b03d", [<input type="checkbox" checked={internet} onChange={(e) => setInternet(e.target.checked)}/>,
                <span className="check__box" aria-hidden="true"/>])}</label>))}
            {formatUI(spec.testable && can(me, 'asset.authorize') && (<label className="check">
                <input type="checkbox" checked={authorized} onChange={(e) => setAuthorized(e.target.checked)}/>
                <span className="check__box" aria-hidden="true"/>
                <span>{richUI("m_dd8bf02dedb9", [<small className="check__sub">{textUI("m_cc2513acdabd")}</small>])}</span>
              </label>))}
            </>))}
            <footer className="dialog__foot">
              <button type="button" className="btn btn--secondary btn--auto" onClick={() => setCategory(null)}>{textUI("m_76900f1bfd16")}</button>
              <button type="submit" className="btn btn--primary btn--auto" disabled={create.isPending}>
                {formatUI(create.isPending ? textUI("m_c79ed9492e3c") : textUI("m_c863672b457b"))}
              </button>
            </footer>
          </form>))}
      </>
    </Modal>);
}
