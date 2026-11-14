-- +goose Up
-- Temuan jaringan yang sudah tidak teramati ditandai 'stale' otomatis (bukan dihapus): riwayatnya tetap ada.
ALTER TABLE findings DROP CONSTRAINT ck_findings_triage_status;
ALTER TABLE findings ADD CONSTRAINT ck_findings_triage_status CHECK (triage_status IN ('open','accepted_risk','false_positive','fixed_pending','stale'));
ALTER TABLE findings
  ADD COLUMN absent_count smallint NOT NULL DEFAULT 0,
  ADD COLUMN absent_collected_at timestamptz;

-- +goose Down
UPDATE findings SET triage_status='open' WHERE triage_status='stale';
ALTER TABLE findings DROP COLUMN absent_collected_at, DROP COLUMN absent_count;
ALTER TABLE findings DROP CONSTRAINT ck_findings_triage_status;
ALTER TABLE findings ADD CONSTRAINT ck_findings_triage_status CHECK (triage_status IN ('open','accepted_risk','false_positive','fixed_pending'));
