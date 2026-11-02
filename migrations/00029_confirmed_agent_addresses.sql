-- +goose Up
-- Nama host identitas agent bukan persetujuan alamat scan. Data tetap tersedia untuk ditinjau admin.
ALTER TABLE asset_addresses ADD COLUMN scan_approved boolean NOT NULL DEFAULT true;
UPDATE asset_addresses d SET scan_approved=false FROM assets a
 WHERE d.tenant_id=a.tenant_id AND d.asset_id=a.id AND a.source='agent'
 AND d.kind='hostname' AND d.created_at=a.created_at
 AND EXISTS(SELECT 1 FROM agents g WHERE g.tenant_id=a.tenant_id AND g.asset_id=a.id AND lower(g.hostname)=d.value);

-- Antrean lama tidak boleh memakai hostname otomatis setelah upgrade. Laporan yang beku tidak diubah.
INSERT INTO audit_log(id,tenant_id,action,resource_type,resource_id,detail)
 SELECT gen_random_uuid(),e.tenant_id,'execution.scope_revoked','execution',e.id,
 jsonb_build_object('reason','Agent-reported hostname requires administrator confirmation','migration',29)
 FROM executions e WHERE e.state IN ('ready','running') AND EXISTS(
 SELECT 1 FROM asset_addresses d WHERE d.tenant_id=e.tenant_id AND d.asset_id=e.target_asset_id AND NOT d.scan_approved
 AND d.value=lower(regexp_replace(CASE WHEN position('://' IN e.target_address)>0 THEN split_part(e.target_address,'/',3) ELSE e.target_address END,':[0-9]+$','')));
UPDATE executions e SET state='ended',outcome='error',error='Agent-reported hostname requires administrator confirmation.',ended_at=now(),lease_until=NULL,version=version+1,updated_at=now()
 WHERE e.state IN ('ready','running') AND EXISTS(
 SELECT 1 FROM asset_addresses d WHERE d.tenant_id=e.tenant_id AND d.asset_id=e.target_asset_id AND NOT d.scan_approved
 AND d.value=lower(regexp_replace(CASE WHEN position('://' IN e.target_address)>0 THEN split_part(e.target_address,'/',3) ELSE e.target_address END,':[0-9]+$','')));
-- +goose Down
ALTER TABLE asset_addresses DROP COLUMN scan_approved;
