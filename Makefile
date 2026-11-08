RELEASE_KEY ?= dist/release.key
RELEASE_DIR ?= dist/agent

.PHONY: osquery-packs mitre-fetch prod-mitre-config osquery-fetch mitre-update mitre-verify demo-seed demo-purge demo-status agent-release dev-up dev-up-full dev-down build test web-test e2e check injector-image

dev-up:      ## Jalankan PostgreSQL pengembangan
	docker compose -f deploy/compose.yml up -d pg

dev-up-full: ## Jalankan PostgreSQL + RabbitMQ + Elasticsearch + MinIO (untuk Fase 3)
	docker compose -f deploy/compose.yml --profile full up -d

injector-image: ## Bangun image injector (nmap + nuclei)
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/injector ./cmd/injector
	mkdir -p work/injector-image/bin
	cp bin/injector work/injector-image/bin/
	docker build -f deploy/injector/Dockerfile -t orchis-injector:dev work/injector-image

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
	node deploy/report/render.test.mjs


demo-seed:   ## Isi data dummy bertanda is_demo (DEMO_SEED=n, bawaan 1); butuh server dev berjalan
	go run ./cmd/server demo-seed

demo-purge:  ## Hapus HANYA data dummy (is_demo); batal bila data nyata bergantung padanya
	go run ./cmd/server demo-purge

demo-status: ## Tampilkan jumlah data dummy dan seed yang dipakai
	go run ./cmd/server demo-status

mitre-update: ## Bekukan bundel ATT&CK Enterprise: make mitre-update VERSION=20.0 (menampilkan diff)
	go run ./cmd/mitre-update -version $(VERSION)

osquery-fetch: ## Ambil artefak osquery yang dipatok ke deploy/osquery (diverifikasi SHA-256; dipakai e2e agent nyata)
	go run ./cmd/agenttool -fetch
	go run ./cmd/agenttool -fetch -os darwin -arch amd64

mitre-fetch: ## Ambil tepat pin manifest; tidak mengubah versi atau hash yang ditinjau
	go run ./cmd/mitre-update -fetch
osquery-packs: ## Bekukan ulang pack resmi osquery dari artefak yang dipatok (butuh OSQUERYD=/jalur/osqueryd untuk sampel nyata)
	@test -n "$(OSQUERYD)" || { echo "isi OSQUERYD=/jalur/ke/osqueryd (biner dari artefak yang sama)"; exit 1; }
	go run ./cmd/osquery-packs -tar deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz -osqueryd "$(OSQUERYD)"

mitre-verify: ## Periksa bundel beku di deploy/mitre terhadap manifest (hash, versi, hitungan)
	go run ./cmd/mitre-update -verify

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
prod-build: ## Validasi engine lalu bangun server/web, renderer, scanner dan Hackbot
	python3 deploy/prod/ops.py build
prod-mitre-config: ## Terapkan pin MITRE checkout pada konfigurasi project terpilih; restart eksplisit sesudah build
	python3 deploy/prod/ops.py mitre-config
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

.PHONY: hackbot-check hackbot-engines hackbot-image build-test
hackbot-check: ## Periksa checkout engine dan hash integrasi tanpa Docker
	python3 deploy/hackbot/build.py --check

hackbot-engines: ## Bangun kedua engine dari checkout yang dikunci
	python3 deploy/hackbot/build.py

hackbot-image: hackbot-engines ## Bangun worker Hackbot dari kode checkout ini
	docker build --build-arg VERSION=$(or $(VERSION),local) -f deploy/hackbot/Dockerfile -t orchis-hackbot-worker:$(or $(VERSION),local) .

build-test: ## Tes validasi source engine dan orkestrasi build, tanpa Docker
	python3 -m unittest discover -s deploy/hackbot -p '*_test.py'
	python3 -m unittest discover -s deploy/prod -p 'ops_test.py'
