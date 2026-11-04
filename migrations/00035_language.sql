-- +goose Up
ALTER TABLE users ADD COLUMN ui_language text CHECK(ui_language IN ('en','id'));
ALTER TABLE assessments ADD COLUMN report_language text NOT NULL DEFAULT 'en' CHECK(report_language IN ('en','id'));
-- +goose Down
ALTER TABLE assessments DROP COLUMN report_language;
ALTER TABLE users DROP COLUMN ui_language;
