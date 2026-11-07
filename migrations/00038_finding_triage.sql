-- +goose Up
-- Triage per finding: open, accepted_risk (dengan tanggal habis + alasan), false_positive, fixed_pending.
-- Siapa dan kapan disimpan di kolom dan di audit_log; accepted risk disembunyikan dari daftar
-- "perbaiki lebih dulu" sampai tanggalnya habis (aturan di kueri pelaporan).
ALTER TABLE findings
  ADD COLUMN triage_status   text NOT NULL DEFAULT 'open',
  ADD COLUMN triage_reason   text NOT NULL DEFAULT '',
  ADD COLUMN triage_expires_at timestamptz,
  ADD COLUMN triage_by       uuid,
  ADD COLUMN triage_at       timestamptz;
ALTER TABLE findings
  ADD CONSTRAINT ck_findings_triage_status CHECK (triage_status IN ('open','accepted_risk','false_positive','fixed_pending'));
-- Risiko diterima wajib punya tanggal habis dan alasan tertulis.
ALTER TABLE findings
  ADD CONSTRAINT ck_findings_triage_risk CHECK (triage_status <> 'accepted_risk' OR (triage_expires_at IS NOT NULL AND triage_reason <> ''));
ALTER TABLE findings
  ADD CONSTRAINT fk_findings_triage_by FOREIGN KEY (tenant_id, triage_by) REFERENCES users (tenant_id, id);
CREATE INDEX idx_findings_triage ON findings (tenant_id, triage_status, triage_expires_at);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Triage history requires restore-based rollback; restore a pre-upgrade backup with its matching release.';
END $$;
-- +goose StatementEnd
