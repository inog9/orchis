#!/usr/bin/env bash
# Menjalankan tes end-to-end. Setiap skrip memakai database BARU dan server sendiri, lalu dibersihkan.
# Pakai E2E_SCRIPT=flow2.mjs untuk satu skrip saja.
# Syarat: `make dev-up-full`, `make injector-image`, `go build -o bin/server ./cmd/server`, `npm --prefix web run build`.
# flow3.mjs memakai RabbitMQ, Elasticsearch, MinIO, dua container injector, dan sasaran HTTP lokal.
set -u
cd "$(dirname "$0")/.."
PORT="${E2E_PORT:-18090}"
PSQL="docker compose -f deploy/compose.yml exec -T pg psql -U orchis -d postgres -q"
SCRIPTS="${E2E_SCRIPT:-flow.mjs flow2.mjs flow3.mjs flow4.mjs flow5.mjs flow6.mjs flow7.mjs flow8.mjs flow9.mjs flow10.mjs flow11.mjs flow13.mjs flow14.mjs flow15.mjs flow16.mjs access-audit.mjs}"
# Gunakan vhost khusus bila injector instalasi lain memakai broker yang sama.
AMQP_URL="${E2E_AMQP_URL:-amqp://orchis:orchis@127.0.0.1:15672/}"
status=0

run_one() (
  local script="$1" db="orchis_e2e_$(date +%s)_$RANDOM" srv="" tgt="" fix="" reldir="" render_fail="" store_proxy="" demo_failure="" demo_port_file="" scan_target_one="" scan_target_two="" tag="$$_$RANDOM" infra_env=()
  $PSQL -c "CREATE DATABASE $db" || return 1
  cleanup() {
    [ -n "$srv" ] && kill "$srv" 2>/dev/null; wait "$srv" 2>/dev/null
    [ -n "$tgt" ] && kill "$tgt" 2>/dev/null
    [ -n "$fix" ] && kill "$fix" 2>/dev/null
    [ -n "$store_proxy" ] && kill "$store_proxy" 2>/dev/null
    [ -n "$demo_failure" ] && rm -f "$demo_failure"
    [ -n "$demo_port_file" ] && rm -f "$demo_port_file"
    [ -n "$reldir" ] && rm -rf "$reldir"
    [ -n "$render_fail" ] && rm -f "$render_fail"
    docker rm -f "orchis-e2e-nmap-$tag" "orchis-e2e-nuclei-$tag" "orchis-e2e-fixture-one-$tag" "orchis-e2e-fixture-two-$tag" >/dev/null 2>&1
    $PSQL -c "DROP DATABASE IF EXISTS $db WITH (FORCE)" >/dev/null 2>&1; }
  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  export ORCHIS_MIGRATE_URL="postgres://orchis:orchis@127.0.0.1:15432/$db"
  export ORCHIS_DATABASE_URL="postgres://orchis_app:orchis_app@127.0.0.1:15432/$db"
  ./bin/server migrate >/dev/null 2>&1 || { echo "migrasi gagal"; return 1; }
  local advanced=true
  if [ "$script" = "flow16.mjs" ] || [ "$script" = "access-audit.mjs" ]; then advanced=false; fi
  local mitre_url=off
  if [ "$script" = "flow5.mjs" ] || [ "$script" = "flow7.mjs" ]; then
    # Fase 7/8: sumber ATT&CK lokal + loopback diizinkan; injector disimulasikan lewat API (tanpa container).
    local fport="${E2E_FIXTURE_PORT:-18082}"
    node e2e/fixture-server.mjs "$fport" >/tmp/orchis-e2e-fixture-$PORT.log 2>&1 &
    fix=$!
    mitre_url="http://127.0.0.1:$fport/bundle.json"
    infra_env=(ORCHIS_ALLOW_LOOPBACK_TARGETS=true)
  fi
  if [ "$script" = "flow14.mjs" ]; then
    # T3: bundel ATT&CK Enterprise penuh yang dibekukan (berkas lokal + hash dari manifest); wajib tersedia sebelum pemeriksaan penuh.
    local mfile msha
    mfile="$(python3 -c "import json;print(json.load(open('deploy/mitre/manifest.json'))['file'])")"
    msha="$(python3 -c "import json;print(json.load(open('deploy/mitre/manifest.json'))['sha256'])")"
    [ -f "deploy/mitre/$mfile" ] || { echo "FAIL flow14: pinned bundle required (make mitre-fetch)"; return 1; }
    mitre_url="$PWD/deploy/mitre/$mfile"
    infra_env=(ORCHIS_MITRE_SHA256="$msha" ORCHIS_AMQP_URL="$AMQP_URL" ORCHIS_ALLOW_LOOPBACK_TARGETS=true)
  fi
  if [ "$script" = "flow6.mjs" ] || [ "$script" = "flow8.mjs" ] || [ "$script" = "flow9.mjs" ] || [ "$script" = "flow10.mjs" ] || [ "$script" = "flow11.mjs" ] || [ "$script" = "flow15.mjs" ] || [ "$script" = "flow16.mjs" ]; then
    # Fase 4: rilis agent sungguhan (linux/amd64) bertanda tangan: 1.0.0, 1.1.0 (kunci sama), dan 1.2.0 yang ditandatangani kunci LAIN.
    reldir="$(mktemp -d)"
    for v in 1.0.0 1.1.0; do
      go run ./cmd/agentrelease -version "$v" -key "$reldir/release.key" -init -out "$reldir" -targets linux/amd64 >/dev/null || return 1
      cp "$reldir/manifest.json" "$reldir/manifest-$v.json"
    done
    go run ./cmd/agentrelease -version 1.2.0 -key "$reldir/other.key" -init -out "$reldir" -targets linux/amd64 >/dev/null || return 1
    cp "$reldir/manifest.json" "$reldir/manifest-1.2.0-bad.json"
    cp "$reldir/manifest-1.0.0.json" "$reldir/manifest.json"
    export E2E_RELEASE_DIR="$reldir"
    if [ "$script" = "flow15.mjs" ] || [ "$script" = "flow16.mjs" ]; then
      # T4: tandatangani artefak osquery yang dipatok dengan kunci rilis yang sama dengan agent (diambil lewat make osquery-fetch).
      local osq="$PWD/deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz"
      [ -f "$osq" ] || { echo "FAIL flow15: pinned osquery artifact required (make osquery-fetch)"; return 1; }
      go run ./cmd/agenttool -file "$osq" -key "$reldir/release.key" -out "$reldir" >/dev/null || return 1
    fi
    infra_env=(ORCHIS_AGENT_RELEASE_DIR="$reldir")
  fi
  if [ "$script" = "flow3.mjs" ] || [ "$script" = "flow4.mjs" ] || [ "$script" = "flow8.mjs" ] || [ "$script" = "flow9.mjs" ] || [ "$script" = "flow10.mjs" ] || [ "$script" = "flow11.mjs" ] || [ "$script" = "flow15.mjs" ] || [ "$script" = "flow16.mjs" ]; then
    docker image inspect orchis-injector:dev >/dev/null 2>&1 || { echo "image orchis-injector:dev belum ada (jalankan: make injector-image)"; return 1; }
    infra_env+=(ORCHIS_AMQP_URL="$AMQP_URL" ORCHIS_ES_URL=http://127.0.0.1:19200
      ORCHIS_S3_ENDPOINT=127.0.0.1:19000 ORCHIS_S3_ACCESS_KEY=orchis ORCHIS_S3_SECRET_KEY=orchis-dev-secret ORCHIS_S3_BUCKET=orchis-e2e
      ORCHIS_ALLOW_LOOPBACK_TARGETS=true)
  fi
  if [ "$script" = "flow13.mjs" ]; then
    # T2 artifacts are real PDFs and exact-key cleanup against local backends.
    # No scanner/AI worker is launched for this synthetic dataset.
    export ORCHIS_ES_URL=http://127.0.0.1:19200
    demo_failure="$(mktemp /tmp/orchis-e2e-demo-failure-XXXXXX)"; rm -f "$demo_failure"
    demo_port_file="$(mktemp /tmp/orchis-e2e-demo-port-XXXXXX)"
    node e2e/demo-storage-proxy.mjs "$demo_port_file" "$demo_failure" >/tmp/orchis-e2e-demo-proxy.log 2>&1 &
    store_proxy=$!
    for _ in $(seq 1 40); do [ -s "$demo_port_file" ] && break; sleep 0.1; done
    [ -s "$demo_port_file" ] || { echo "demo storage proxy did not start"; return 1; }
    export E2E_DEMO_STORAGE_FAILURE_FILE="$demo_failure"
    export ORCHIS_S3_ENDPOINT="127.0.0.1:$(cat "$demo_port_file")" ORCHIS_S3_ACCESS_KEY=orchis ORCHIS_S3_SECRET_KEY=orchis-dev-secret ORCHIS_S3_BUCKET=orchis-e2e
  fi
  if [ "$script" = "flow10.mjs" ]; then
    docker image inspect orchis-ai:dev >/dev/null 2>&1 || { echo "image orchis-ai:dev belum ada (jalankan make ai-image)"; return 1; }
    export ORCHIS_AI_ENCRYPTION_KEY=CQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQk=
    export ORCHIS_AI_WORKER_TOKEN=local-only-ai-worker-token-32-characters
    infra_env+=(ORCHIS_AI_ENCRYPTION_KEY="$ORCHIS_AI_ENCRYPTION_KEY" ORCHIS_AI_WORKER_TOKEN="$ORCHIS_AI_WORKER_TOKEN")
  fi
  if [ "$script" = "flow11.mjs" ]; then
    render_fail="$(mktemp /tmp/orchis-e2e-render-XXXXXX)"; rm -f "$render_fail"
    export E2E_RENDER_FAILURE_FILE="$render_fail"
    infra_env+=(ORCHIS_REPORT_RENDER_SCRIPT="$PWD/e2e/renderer-fixture.mjs" ORCHIS_E2E_RENDER_FAILURE_FILE="$render_fail")
  fi
  env ORCHIS_HTTP_ADDR="127.0.0.1:$PORT" ORCHIS_WEB_DIR="$PWD/web/dist" ORCHIS_ADMIN_EMAIL=admin@orchis.local ORCHIS_ADMIN_PASSWORD='Orchis-Dev-Pass-1' ORCHIS_MITRE_URL="$mitre_url" ORCHIS_ADVANCED_FEATURES="$advanced" "${infra_env[@]}" ./bin/server serve >/tmp/orchis-e2e-server.log 2>&1 &
  srv=$!
  for _ in $(seq 1 40); do curl -sf "http://127.0.0.1:$PORT/api/healthz" >/dev/null && break; sleep 0.3; done
  if [ "$script" = "flow3.mjs" ] || [ "$script" = "flow4.mjs" ] || [ "$script" = "flow8.mjs" ] || [ "$script" = "flow9.mjs" ] || [ "$script" = "flow10.mjs" ] || [ "$script" = "flow11.mjs" ] || [ "$script" = "flow14.mjs" ] || [ "$script" = "flow15.mjs" ] || [ "$script" = "flow16.mjs" ]; then
    local tport="${E2E_TARGET_PORT:-18081}"
    node e2e/target.mjs "$tport" >/tmp/orchis-e2e-target.log 2>&1 &
    tgt=$!
    for kind in nmap nuclei; do
      local tok; tok="$(env "${infra_env[@]}" ./bin/server injector-token "$kind")" || { echo "token $kind gagal"; return 1; }
      docker run -d --rm --network host --name "orchis-e2e-$kind-$tag" -e ORCHIS_SERVER_URL="http://127.0.0.1:$PORT" -e ORCHIS_INJECTOR_TOKEN="$tok" \
        -e ORCHIS_AMQP_URL="$AMQP_URL" -e ORCHIS_ALLOW_LOOPBACK_TARGETS=true \
        -v "$PWD/e2e/templates:/templates:ro" -e NUCLEI_TEMPLATES_DIR=/templates orchis-injector:dev --kind "$kind" >/dev/null || return 1
    done
    export E2E_NMAP_CONTAINER="orchis-e2e-nmap-$tag" E2E_TARGET_PORT="$tport"
  fi
  if [ "$script" = "flow16.mjs" ]; then
    for label in one two; do
      docker run -d --rm --read-only --cap-drop ALL --name "orchis-e2e-fixture-$label-$tag" -v "$PWD/e2e/target-container.mjs:/fixture.mjs:ro" node:24.22.0-alpine3.24 node /fixture.mjs >/dev/null || return 1
    done
    export E2E_SCAN_TARGET_ONE="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "orchis-e2e-fixture-one-$tag")"
    export E2E_SCAN_TARGET_TWO="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "orchis-e2e-fixture-two-$tag")"
  fi
  echo "=== $script"
  E2E_BASE="http://127.0.0.1:$PORT" node "e2e/$script"
)

for s in $SCRIPTS; do run_one "$s" || status=1; done
exit $status
