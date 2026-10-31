-- Peran aplikasi: bukan pemilik tabel, tidak melewati RLS. Pemilik (orchis) hanya dipakai migrasi.
CREATE ROLE orchis_app LOGIN PASSWORD 'orchis_app' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
GRANT CONNECT ON DATABASE orchis TO orchis_app;
GRANT USAGE ON SCHEMA public TO orchis_app;
ALTER DEFAULT PRIVILEGES FOR ROLE orchis IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO orchis_app;
ALTER DEFAULT PRIVILEGES FOR ROLE orchis IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO orchis_app;
-- Peran pemeliharaan lintas tenant (job retensi dan sejenisnya); dipakai eksplisit dan tercatat.
CREATE ROLE orchis_maintenance LOGIN PASSWORD 'orchis_maintenance' NOSUPERUSER BYPASSRLS;
GRANT CONNECT ON DATABASE orchis TO orchis_maintenance;
GRANT USAGE ON SCHEMA public TO orchis_maintenance;
ALTER DEFAULT PRIVILEGES FOR ROLE orchis IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO orchis_maintenance;
ALTER DEFAULT PRIVILEGES FOR ROLE orchis IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO orchis_maintenance;
