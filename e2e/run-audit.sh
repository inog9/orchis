#!/usr/bin/env bash
# Wrapper flow26 untuk stack audit terisolasi (pg 15433, AMQP 15673) - varian dari e2e/run.sh.
# Database baru per putaran, server sendiri, dibersihkan setelahnya. Tidak menyentuh stack lain.
set -u
cd "$(dirname "$0")/.."
PORT="${E2E_PORT:-18094}"
PSQL="docker exec orchis-audit-pg-1 psql -U orchis -d postgres -q"
AMQP_URL="${E2E_AMQP_URL:-amqp://orchis:orchis@127.0.0.1:15673/}"
status=0

run_one() (
  local script="$1" db="orchis_e2e_$(date +%s)_$RANDOM" srv="" reldir=""
  $PSQL -c "CREATE DATABASE $db" || return 1
  cleanup() {
    [ -n "$srv" ] && kill "$srv" 2>/dev/null; wait "$srv" 2>/dev/null
    [ -n "$reldir" ] && rm -rf "$reldir"
    $PSQL -c "DROP DATABASE IF EXISTS $db WITH (FORCE)" >/dev/null 2>&1; }
  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  export ORCHIS_MIGRATE_URL="postgres://orchis:orchis@127.0.0.1:15433/$db"
  export ORCHIS_DATABASE_URL="postgres://orchis_app:orchis_app@127.0.0.1:15433/$db"
  ./bin/server migrate >/dev/null 2>&1 || { echo "migrasi gagal"; return 1; }
  # flow26: rilis agent sungguhan (linux/amd64) bertanda tangan, seperti run.sh.
  reldir="$(mktemp -d)"
  for v in 1.0.0 1.1.0; do
    go run ./cmd/agentrelease -version "$v" -key "$reldir/release.key" -init -out "$reldir" -targets linux/amd64 >/dev/null || return 1
    cp "$reldir/manifest.json" "$reldir/manifest-$v.json"
  done
  cp "$reldir/release.pub" "$reldir/release.pub.trusted"
  go run ./cmd/agentrelease -version 1.2.0 -key "$reldir/other.key" -init -out "$reldir" -targets linux/amd64 >/dev/null || return 1
  mv "$reldir/release.pub.trusted" "$reldir/release.pub"
  cp "$reldir/manifest.json" "$reldir/manifest-1.2.0-bad.json"
  cp "$reldir/manifest-1.0.0.json" "$reldir/manifest.json"
  export E2E_RELEASE_DIR="$reldir"
  env ORCHIS_HTTP_ADDR="127.0.0.1:$PORT" ORCHIS_WEB_DIR="$PWD/web/dist" ORCHIS_ADMIN_EMAIL=admin@orchis.local ORCHIS_ADMIN_PASSWORD='Orchis-Dev-Pass-1' \
    ORCHIS_MITRE_URL=off ORCHIS_ADVANCED_FEATURES=false ORCHIS_AGENT_RELEASE_DIR="$reldir" \
    ORCHIS_ALLOW_LOOPBACK_TARGETS=true ORCHIS_AMQP_URL="$AMQP_URL" \
    ./bin/server serve >/tmp/orchis-e2e-server-flow26.log 2>&1 &
  srv=$!
  for _ in $(seq 1 40); do curl -sf "http://127.0.0.1:$PORT/api/healthz" >/dev/null && break; sleep 0.3; done
  echo "=== $script (stack audit, db $db)"
  E2E_BASE="http://127.0.0.1:$PORT" node "e2e/$script"
)

run_one "${E2E_SCRIPT:-flow26.mjs}" || status=1
exit $status
