#!/bin/bash
# Local CI; isolated broker vhost. No cloud CI or provider AI calls.
set -euo pipefail
cd "$(dirname "$0")/../.."
TASK_VHOST="ci-$(date +%s)-$RANDOM"
MQ_CTL_ARGS='+S 1:1 +SDcpu 1 +SDio 1 +A 1'
mqctl(){ docker compose -f deploy/compose.yml exec -T -e "RABBITMQ_CTL_ERL_ARGS=$MQ_CTL_ARGS" mq rabbitmqctl "$@" >/dev/null; }
cleanup(){ mqctl delete_vhost "$TASK_VHOST" 2>/dev/null || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mqctl add_vhost "$TASK_VHOST"
mqctl set_permissions -p "$TASK_VHOST" orchis '.*' '.*' '.*'
export ORCHIS_TEST_AMQP_URL="amqp://orchis:orchis@127.0.0.1:15672/$TASK_VHOST"
export E2E_AMQP_URL="$ORCHIS_TEST_AMQP_URL"
# Audit all lockfiles including build/test tools. CI fails on affected call paths.
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
npm --prefix web audit
npm --prefix e2e audit
npm --prefix deploy/report audit
make check
python3 -m unittest discover -s deploy/hackbot -p "*_test.py"
python3 -m unittest discover -s deploy/prod -p "*_test.py"
# Rules are executable configuration; test their firing conditions locally.
promtool(){ docker run --rm --network none --read-only --tmpfs /tmp:size=32m,mode=1777 --cap-drop ALL --security-opt no-new-privileges:true --memory 128m --pids-limit 32 -v "$PWD/deploy/prod/monitoring:/rules:ro" -w /rules --entrypoint /bin/promtool prom/prometheus:v3.15.1@sha256:fa3302279710be2f21a8301ec17a8b4f2301fd829bd113839b3b6b02b6f191a2 "$@"; }
promtool check rules alerts.yml
promtool test rules alerts.test.yml
node e2e/flow12.mjs
node deploy/report/render.test.mjs
python3 deploy/ci/check-bundles.py
