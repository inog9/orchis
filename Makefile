RELEASE_KEY ?= dist/release.key
RELEASE_DIR ?= dist/agent

.PHONY: agent-release dev-up dev-up-full dev-down build test web-test e2e check injector-image

dev-up:      ## Jalankan PostgreSQL pengembangan
	docker compose -f deploy/compose.yml up -d pg

dev-up-full: ## Jalankan PostgreSQL + RabbitMQ + Elasticsearch + MinIO (untuk Fase 3)
	docker compose -f deploy/compose.yml --profile full up -d

injector-image: ## Bangun image injector (nmap + nuclei)
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/injector ./cmd/injector
	docker build -f deploy/injector/Dockerfile -t orchis-injector:dev .

agent-release: ## Bangun dan tandatangani agent semua platform (VERSION=1.0.0)
	go run ./cmd/agentrelease -version $(or $(VERSION),0.1.0) -key "$(RELEASE_KEY)" -init -out "$(RELEASE_DIR)"

dev-down:
	docker compose -f deploy/compose.yml down

build:       ## Bangun frontend lalu biner server
	npm --prefix web run build
	go build -o bin/server ./cmd/server
	go build -o bin/backup-seal ./cmd/backup-seal

test:        ## Tes backend (butuh `make dev-up-full` dan nmap)
	go vet ./...
	go test -count=1 ./...

web-test:
	npm --prefix web run typecheck
	npm --prefix web test

e2e: build   ## Tes end-to-end di browser (database baru per putaran)
	python3 e2e/cleanup-test.py
	./e2e/run.sh
	node e2e/ai-egress.mjs

check: test web-test e2e

ai-image: ## Bangun worker/proxy AI non-root; tidak membutuhkan API key untuk build.
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/ai-worker ./cmd/ai-worker
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/ai-proxy ./cmd/ai-proxy
	mkdir -p work/ai-image/bin
	cp bin/ai-worker bin/ai-proxy work/ai-image/bin/
	docker build -f deploy/ai/Dockerfile -t orchis-ai:dev work/ai-image

.PHONY: prod-init prod-build prod-up prod-down prod-status
prod-init: ## Buat secret di luar checkout (root; ORCHIS_STATE opsional)
	python3 deploy/prod/ops.py init
prod-build: ## Bangun image server/web, renderer dan scanner
	python3 deploy/prod/ops.py build
prod-up: ## Mulai hanya project terpilih; data yang sudah ada dipertahankan
	python3 deploy/prod/ops.py up
prod-down: ## Hentikan project terpilih tanpa menghapus volume
	python3 deploy/prod/ops.py down
prod-status:
	python3 deploy/prod/ops.py status

prod-probe: ## Verifikasi HTTPS/login dan renderer; belum smoke assessment/restore lengkap
	python3 deploy/prod/ops.py probe

.PHONY: ci
ci: ## CI lokal lengkap pada vhost disposable (jalankan dev-up-full lebih dulu)
	bash deploy/ci/run.sh

.PHONY: prod-backup-key prod-backup prod-restore
prod-backup-key:
	python3 deploy/prod/ops.py backup-key
prod-backup:
	python3 deploy/prod/ops.py backup
prod-restore: ## ORCHIS_STATE baru + ORCHIS_BACKUP_FILE + ORCHIS_BACKUP_KEY_FILE
	python3 deploy/prod/ops.py restore

.PHONY: prod-smoke
prod-smoke: ## Stack baru lokal, scan nyata, PDF, backup terenkripsi, restore terisolasi
	python3 deploy/prod/ops.py smoke

.PHONY: volume-study
volume-study: ## Ukur sintetis 50 organisasi/5000 aset/200000 temuan pada DB/ES disposable
	python3 deploy/ci/volume.py
