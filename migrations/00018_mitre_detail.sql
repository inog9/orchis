-- +goose Up
-- Detail teknik ATT&CK (tautan sumber, mitigasi resmi) dan pemetaan aksi->teknik yang beralasan.
ALTER TABLE mitre_techniques ADD COLUMN url text;
ALTER TABLE mitre_techniques ADD COLUMN mitigations jsonb NOT NULL DEFAULT '[]';

-- basis: 'direct' = aksi itu melakukan perilaku teknik; 'estimate' = perkiraan (tidak membuktikan perilaku itu terjadi).
ALTER TABLE contract_techniques ADD COLUMN reason text NOT NULL DEFAULT '';
ALTER TABLE contract_techniques ADD COLUMN basis text NOT NULL DEFAULT 'direct' CHECK (basis IN ('direct','estimate'));

ALTER TABLE mitre_import_runs ADD COLUMN source_sha256 text;
ALTER TABLE mitre_import_runs ADD COLUMN subtechniques integer;
ALTER TABLE mitre_import_runs ADD COLUMN deprecated integer;

-- +goose Down
ALTER TABLE mitre_import_runs DROP COLUMN deprecated;
ALTER TABLE mitre_import_runs DROP COLUMN subtechniques;
ALTER TABLE mitre_import_runs DROP COLUMN source_sha256;
ALTER TABLE contract_techniques DROP COLUMN basis;
ALTER TABLE contract_techniques DROP COLUMN reason;
ALTER TABLE mitre_techniques DROP COLUMN mitigations;
ALTER TABLE mitre_techniques DROP COLUMN url;
