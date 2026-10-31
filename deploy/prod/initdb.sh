#!/bin/sh
set -eu
# Executed only on a fresh PostgreSQL volume. psql variables quote passwords as
# SQL literals; input files contain generated base64url strings, never operator SQL.
OWNER_PASSWORD="$(cat /run/secrets/owner_password)"
APP_PASSWORD="$(cat /run/secrets/app_password)"
MAINT_PASSWORD="$(cat /run/secrets/maintenance_password)"
test -n "$OWNER_PASSWORD"
test -n "$APP_PASSWORD"
test -n "$MAINT_PASSWORD"
export OWNER_PASSWORD APP_PASSWORD MAINT_PASSWORD
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
\getenv owner_password OWNER_PASSWORD
\getenv app_password APP_PASSWORD
\getenv maintenance_password MAINT_PASSWORD
CREATE ROLE orchis LOGIN PASSWORD :'owner_password' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
CREATE ROLE orchis_app LOGIN PASSWORD :'app_password' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
CREATE ROLE orchis_maintenance LOGIN PASSWORD :'maintenance_password' NOSUPERUSER BYPASSRLS NOCREATEDB NOCREATEROLE;
ALTER DATABASE orchis OWNER TO orchis;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO orchis;
GRANT USAGE ON SCHEMA public TO orchis_app, orchis_maintenance;
ALTER DEFAULT PRIVILEGES FOR ROLE orchis IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO orchis_app, orchis_maintenance;
ALTER DEFAULT PRIVILEGES FOR ROLE orchis IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO orchis_app, orchis_maintenance;
SQL
unset OWNER_PASSWORD APP_PASSWORD MAINT_PASSWORD
