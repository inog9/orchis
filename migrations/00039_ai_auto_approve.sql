-- +goose Up
-- Auto-approve: bila true, job AI yang perlu review (planning dengan aksi)
-- langsung disetujui saat worker selesai, tanpa langkah approve manual.
ALTER TABLE ai_policies ADD COLUMN auto_approve boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE ai_policies DROP COLUMN auto_approve;
