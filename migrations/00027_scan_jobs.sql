-- +goose Up
-- Assessment tetap penyimpanan kelompok hasil; Scan tidak memerlukan scenario pengguna.
ALTER TABLE assessments ADD COLUMN is_scan boolean NOT NULL DEFAULT false,
 ADD COLUMN runner text NOT NULL DEFAULT 'injector' CHECK(runner='injector'),
 ADD COLUMN scan_options jsonb NOT NULL DEFAULT '{}';
CREATE INDEX idx_assessments_scan ON assessments(tenant_id,is_scan,created_at DESC,id DESC);

CREATE TABLE network_range_authorizations (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id),
 organization_id uuid NOT NULL, agent_id uuid NOT NULL, control_asset_id uuid NOT NULL,
 cidr text NOT NULL, authorized boolean NOT NULL DEFAULT false, version integer NOT NULL DEFAULT 1 CHECK(version>0),
 consent_revision bigint NOT NULL, authorized_by uuid NOT NULL, authorized_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,agent_id,cidr),
 FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id),
 FOREIGN KEY(tenant_id,agent_id) REFERENCES agents(tenant_id,id),
 FOREIGN KEY(tenant_id,control_asset_id) REFERENCES assets(tenant_id,id),
 FOREIGN KEY(tenant_id,authorized_by) REFERENCES users(tenant_id,id)
);
CREATE INDEX idx_network_ranges_org ON network_range_authorizations(tenant_id,organization_id);
CREATE INDEX idx_network_ranges_asset ON network_range_authorizations(tenant_id,control_asset_id);
CREATE INDEX idx_network_ranges_actor ON network_range_authorizations(tenant_id,authorized_by);
SELECT enable_tenant_rls('network_range_authorizations');

CREATE TABLE scan_jobs (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), assessment_id uuid NOT NULL,
 control_asset_id uuid NOT NULL, asset_id uuid, range_id uuid, range_version integer,
 name text NOT NULL, target text NOT NULL, addresses jsonb NOT NULL,
 tool text NOT NULL CHECK(tool IN ('nmap','nuclei')), contract_id uuid NOT NULL, contract_version integer NOT NULL,
 params jsonb NOT NULL, scope jsonb NOT NULL, simulation_id uuid,
 status text NOT NULL DEFAULT 'waiting' CHECK(status IN ('waiting','running','finished','failed','canceled')),
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), ended_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,simulation_id),
 FOREIGN KEY(tenant_id,assessment_id) REFERENCES assessments(tenant_id,id),
 FOREIGN KEY(tenant_id,control_asset_id) REFERENCES assets(tenant_id,id),
 FOREIGN KEY(tenant_id,asset_id) REFERENCES assets(tenant_id,id),
 FOREIGN KEY(tenant_id,range_id) REFERENCES network_range_authorizations(tenant_id,id),
 FOREIGN KEY(tenant_id,contract_id) REFERENCES contracts(tenant_id,id),
 FOREIGN KEY(tenant_id,simulation_id) REFERENCES simulations(tenant_id,id),
 CHECK((range_id IS NULL AND asset_id IS NOT NULL AND range_version IS NULL) OR (range_id IS NOT NULL AND range_version IS NOT NULL))
);
CREATE INDEX idx_scan_jobs_assessment ON scan_jobs(tenant_id,assessment_id,id);
CREATE INDEX idx_scan_jobs_control_asset ON scan_jobs(tenant_id,control_asset_id);
CREATE INDEX idx_scan_jobs_asset ON scan_jobs(tenant_id,asset_id);
CREATE INDEX idx_scan_jobs_range ON scan_jobs(tenant_id,range_id);
CREATE INDEX idx_scan_jobs_contract ON scan_jobs(tenant_id,contract_id);
SELECT enable_tenant_rls('scan_jobs');

ALTER TABLE findings ADD COLUMN organization_id uuid, ADD COLUMN subject_address text,
 ADD FOREIGN KEY(tenant_id,organization_id) REFERENCES organizations(tenant_id,id);
CREATE INDEX idx_findings_organization ON findings(tenant_id,organization_id);
CREATE UNIQUE INDEX uk_findings_network_subject ON findings(tenant_id,organization_id,subject_address,type,value_hash)
 WHERE asset_id IS NULL AND organization_id IS NOT NULL;

-- +goose Down
-- Downgrade bukan jalur operasional; data scan baru harus diarsipkan sebelum rollback lewat restore.
ALTER TABLE findings DROP COLUMN subject_address, DROP COLUMN organization_id;
DROP TABLE scan_jobs;
DROP TABLE network_range_authorizations;
ALTER TABLE assessments DROP COLUMN scan_options, DROP COLUMN runner, DROP COLUMN is_scan;
