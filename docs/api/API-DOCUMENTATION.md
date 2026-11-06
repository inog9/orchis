# Dokumentasi API Orchis

Dokumentasi berbahasa Indonesia untuk integrasi HTTP Orchis. Dibuat pada **6 November 2026**, dari source main commit **84392fcb7b33aa2fc59be4188a2495bfb50b323b**. Cakupan: **167 kombinasi method/path** dalam inventaris kebijakan API, model JSON di source, dan handler implementasinya. Dokumen ini merupakan referensi source; tidak menguji ulang server yang sedang berjalan.

## Mulai di sini

Gunakan `index.html` untuk membaca dan mencari endpoint secara offline. `API-DOCUMENTATION.md` adalah salinan Markdown lengkap. `endpoints.csv` berisi daftar method/path/role; `openapi.json` menyimpan spesifikasi OpenAPI terstruktur. Tautan ke source repo menunjukkan berkas implementasi yang dirujuk untuk memeriksa validasi dan bentuk respons secara tepat.

Base URL contoh: `https://orchis.example.test`. Ganti dengan URL instalasi Anda. API utama menggunakan `/api/v1`; health check menggunakan `/api/healthz`. Semua nama domain, UUID, password dan token dalam contoh adalah placeholder. Contoh curl memakai shell POSIX.

## Autentikasi dan header

Ada tiga kelompok kredensial:

| Kelompok | Cara autentikasi | Pemakaian |
|---|---|---|
| Pengguna | Cookie `orchis_session` dari login, atau `Authorization: Bearer oat_…` | API konsol/integrasi pengguna |
| Agent | Bearer kredensial agent | `/api/v1/agent/*`; token enrollment terpisah untuk pendaftaran/unduhan |
| Injector / AI worker | Bearer token mesin yang sesuai | Endpoint injector dan AI worker |

`public` pada daftar endpoint berarti tidak memerlukan sesi pengguna. **Enrollment tetap memerlukan token pendaftaran yang valid**; `public` bukan berarti pendaftaran bebas. `machine` berarti token mesin, bukan API token pengguna. Bearer pengguna berawalan `oat_` diprioritaskan atas cookie; jika bearer itu salah, cookie tidak dipakai sebagai fallback.

Untuk body JSON, kirim `Content-Type: application/json`. Untuk permintaan pengguna yang mengubah data melalui cookie, sertakan `X-Requested-With: orchis-web`. Middleware mengecualikan bearer pengguna `oat_`, kanal agent, dan kanal injector dengan bearer dari pemeriksaan header CSRF tersebut. Mengirim header ini secara konsisten pada contoh integrasi mempermudah penggunaan cookie.

Decoder JSON umum membatasi body **1 MiB** dan menolak field yang tidak dikenal serta data tambahan setelah objek JSON. Endpoint unduhan, impor, dan protokol mesin dapat memiliki aturan body khusus; lihat handler endpoint terkait. Waktu menggunakan representasi JSON Go `time.Time` (RFC 3339), dan ID umumnya UUID.

### Login memakai cookie

```bash
export BASE_URL='https://orchis.example.test'
# Buat login.json secara lokal: {"email":"anda@example.test","password":"PASSWORD_ANDA","remember_me":false}
curl --fail-with-body --cookie-jar cookies.txt \
  -H 'Content-Type: application/json' -H 'X-Requested-With: orchis-web' \
  --data-binary @login.json "$BASE_URL/api/v1/auth/login"
curl --fail-with-body --cookie cookies.txt "$BASE_URL/api/v1/auth/me"
```

Login sukses mengembalikan profil pengguna: `id`, `email`, `display_name`, `role`, `permissions`, `ui_language`, dan `features`. Cookie bersifat HttpOnly dan SameSite=Lax; konfigurasi produksi mengaktifkan Secure. `remember_me: true` meminta sesi persisten sesuai TTL implementasi. `POST /api/v1/auth/logout` mengakhiri sesi dan mengembalikan 204.

Jika instalasi memakai CA internal, tambahkan `--cacert /path/ke/ca.pem` yang sudah diverifikasi ke setiap perintah curl. Gunakan identitas hostname yang cocok dengan sertifikat.

### API token pengguna

Kelola token melalui `GET/POST /api/v1/me/tokens` dan `DELETE /api/v1/me/tokens/{id}`. Token mengikuti izin pemilik dan scope token; role pada daftar bukan jaminan akses ke semua objek.

```bash
# Isi API_TOKEN melalui pengelola rahasia atau environment lokal.
curl --fail-with-body -H "Authorization: Bearer $API_TOKEN" \
  "$BASE_URL/api/v1/organizations?limit=25"
```

### Mengubah bahasa akun

```bash
curl --fail-with-body --cookie cookies.txt -X PATCH \
  -H 'Content-Type: application/json' -H 'X-Requested-With: orchis-web' \
  --data '{"ui_language":"id"}' "$BASE_URL/api/v1/auth/preferences"
```

Nilai bahasa akun adalah `en`, `id`, atau string kosong untuk menghapus preferensi. Bahasa laporan dibekukan saat peluncuran scan; perubahan bahasa akun tidak mengubah snapshot laporan lama. Kunci JSON tetap berbahasa Inggris.

## Izin dan batas organisasi

Admin mengelola pengaturan, otorisasi target, consent, pengguna, dan kontrol agent. Analyst menjalankan scan pada target yang sudah diotorisasi dan mengelola objek sesuai permission. Viewer membaca findings dan reports serta endpoint akun/inbox yang relevan. Inventaris role pada setiap endpoint berasal dari matriks pengujian API; **service tetap memeriksa permission tambahan, relasi objek, tenant, dan kondisi runtime**. Resource lintas tenant atau yang tidak terlihat dapat menghasilkan 404.

API lama seperti Scenarios/assessment masih ada untuk kompatibilitas dan engine bersama. Chains, Atomic, AI dan MITRE merupakan kemampuan lanjutan; keberadaan route tidak berarti fiturnya aktif dalam instalasi. AI nyata memerlukan konfigurasi dan persetujuan tersendiri.

## Paginasi, filter, dan konkurensi

Endpoint daftar tertentu mengembalikan `items` dan `next_cursor`; gunakan cursor yang diberikan tanpa menguraikan atau membuatnya sendiri. Jangan mengasumsikan semua daftar mendukung `limit` atau `cursor`: daftar agent, misalnya, memakai envelope `items`. Parameter yang terlihat dalam handler dicantumkan pada referensi endpoint; validasi service tetap berlaku.

Filter organisasi standar menerima `org=<uuid>` atau alias `organization_id=<uuid>` pada endpoint yang memanggil helper tersebut. Dua parameter dengan nilai berbeda ditolak. `org=unassigned` hanya diterima bila endpoint memang mengaktifkannya, misalnya daftar Assets. Reports juga memiliki `assessment_id` dan `scan_only=true`.

Banyak pembaruan memakai **optimistic concurrency**: ambil objek terbaru, sertakan `version` yang tepat, lalu gunakan versi baru dari respons. Konflik 409 meminta klien memuat ulang dan memutuskan perubahan; jangan menimpa perubahan pengguna lain secara otomatis. Kontrol agent memakai `control_version` dari objek agent sebagai `version` perintah.

`POST /api/v1/scans` dan peluncuran assessment menerima header **`Idempotency-Key` sepanjang 1–200 karakter**. Simpan key untuk satu permintaan logis; pengulangan key dengan payload yang sama mengembalikan objek yang sama. Payload berbeda dengan key yang sama menghasilkan konflik. Gunakan key baru untuk peluncuran baru.

## Format galat

Error memakai `Content-Type: application/problem+json`. Contoh bentuk, bukan respons dari data nyata:

```json
{
  "type": "about:blank",
  "title": "Invalid data",
  "status": 422,
  "code": "validation_failed",
  "detail": "Check the highlighted fields.",
  "error_id": "err_CONTOH",
  "request_id": "CONTOH_REQUEST_ID",
  "fields": [{"name":"name","code":"name_invalid","message":"Name is required (max 200 characters)."}]
}
```

| Status | Penanganan klien |
|---|---|
| 400 | Perbaiki JSON, parameter, atau bentuk request |
| 401 | Login ulang atau periksa kredensial |
| 403 | Periksa permission, scope token, dan header CSRF |
| 404 | Objek/format tidak ada atau tidak terlihat bagi kredensial ini |
| 409 | Muat ulang versi objek atau periksa kondisi operasi |
| 413 | Kurangi ukuran request bila endpoint mengembalikannya |
| 422 | Perbaiki field yang ditunjuk `fields` |
| 429 | Ikuti `Retry-After` / `retry_after_seconds` bila diberikan |
| 5xx | Catat `error_id`/`request_id`; retry hanya bila aman dan idempoten |

Gunakan `code` dan `fields[].code` untuk teks UI terjemahan. Detail dan message dapat berubah; jangan menjadikannya kontrak logika. Error tak terduga disajikan sebagai error internal generik dengan ID korelasi.

## Contoh alur organisasi → aset → scan → laporan

### 1. Buat organisasi

`POST /api/v1/organizations` mengembalikan 201. Slug unik dalam tenant, memakai huruf kecil/angka dengan pemisah tanda minus.

```bash
curl --fail-with-body --cookie cookies.txt \
  -H 'Content-Type: application/json' -H 'X-Requested-With: orchis-web' \
  --data '{"name":"Contoh Organisasi","slug":"contoh-organisasi","description":"Lingkungan yang diizinkan","industry":"Teknologi","contact":"admin@example.test"}' \
  "$BASE_URL/api/v1/organizations"
```

Simpan `id` sebagai `ORG_ID` dan `version` untuk pembaruan berikutnya. `PATCH /organizations/{id}` memakai model input organisasi lengkap, bukan JSON Patch RFC 6902.

### 2. Buat aset dan tetapkan otorisasi oleh Admin

```bash
curl --fail-with-body --cookie cookies.txt \
  -H 'Content-Type: application/json' -H 'X-Requested-With: orchis-web' \
  --data "{\"organization_id\":\"$ORG_ID\",\"category\":\"host\",\"name\":\"Target yang diizinkan\",\"testing_authorized\":true,\"addresses\":[{\"kind\":\"url\",\"value\":\"https://target.example.test\",\"scan_approved\":true,\"is_primary\":true}]}" \
  "$BASE_URL/api/v1/assets"
```

Ganti target dengan sistem yang memang diizinkan. Simpan ID aset sebagai `ASSET_ID`. Agent enrollment tidak otomatis memberi otorisasi pengujian. Alamat yang dilaporkan agent tidak otomatis menjadi target yang dipercaya. Alamat internet juga memerlukan aturan otorisasi internet-facing dan scope yang sesuai.

### 3. Jalankan scan manual

```bash
export SCAN_KEY='ganti-dengan-id-unik-satu-peluncuran'
curl --fail-with-body --cookie cookies.txt \
  -H 'Content-Type: application/json' -H 'X-Requested-With: orchis-web' \
  -H "Idempotency-Key: $SCAN_KEY" \
  --data "{\"organization_id\":\"$ORG_ID\",\"asset_ids\":[\"$ASSET_ID\"],\"range_ids\":[],\"tools\":[\"nmap\",\"nuclei\"],\"preset\":\"quick\",\"language\":\"id\"}" \
  "$BASE_URL/api/v1/scans"
```

Respons 201 adalah objek scan, bukan tanda bahwa scan selesai. Preset: `quick` atau `standard`; tools: `nmap` dan/atau `nuclei`. Poll `GET /api/v1/scans/{id}`. Objek scan memuat `options`, `status`, dan, pada detail, `jobs`. Hasil tiap job memiliki status dan findings sendiri. Pemeriksaan target gagal/dilewati/dihentikan tidak membuktikan host aman atau temuan teratasi.

Nmap/Nuclei berjalan pada injector. Jendela waktu, emergency hold, otorisasi, batas host dan scope tetap diperiksa. `POST /api/v1/scans/{id}/cancel` membatalkan scan dan merespons objek `{"canceled":true}` saat berhasil; HTTP sukses tidak membuktikan proses sudah berhenti pada saat yang sama.

### 4. Ambil temuan dan laporan

```bash
curl --fail-with-body --cookie cookies.txt "$BASE_URL/api/v1/findings?org=$ORG_ID"
curl --fail-with-body --cookie cookies.txt "$BASE_URL/api/v1/reports?assessment_id=$SCAN_ID"
# Ambil REPORT_ID dari daftar laporan:
curl --fail-with-body --cookie cookies.txt \
  "$BASE_URL/api/v1/reports/$REPORT_ID/export/json" -o report.json
curl --fail-with-body --cookie cookies.txt \
  "$BASE_URL/api/v1/reports/$REPORT_ID/export/pdf" -o report.pdf
```

ID scan dipakai pada filter `assessment_id` karena penyimpanan scan memakai assessment. Tunggu status laporan `ready` sebelum mengunduh PDF. Status scan dan render laporan terpisah. `POST /reports/{id}/retry` menjadwalkan ulang PDF yang gagal tanpa mengulang scan; respons diterima 202. Ekspor laporan saat ini hanya **PDF dan JSON**; `preview=1` pada ekspor PDF memakai disposition inline. HTML/CSV bukan format ekspor laporan yang didukung.

Snapshot laporan tetap. `comparison_details` memuat Baru/Masih ada/Teratasi/tidak dibandingkan. Teratasi membutuhkan cakupan selesai yang sebanding; kegagalan scan tidak boleh dianggap resolusi.

## Agent, consent, dan pengendalian scan

Pendaftaran: Admin membuat `POST /agents/enrollment-tokens` dengan `organization_id`, `os` (`linux`, `macos`, `windows`), `arch` (`x86_64`, `arm64`), dan `mode` (`service`, `user`). Gunakan perintah instalasi dari konsol atau respons token. Token sekali pakai mengikat organisasi di server. Endpoint `/agent/download` menggunakan token pendaftaran; `/agent/enroll` menerima payload enrollment, bukan sesi pengguna.

`GET /osquery/catalog` menjelaskan query dan consent. Persetujuan/konfigurasi berada pada endpoint organisasi `/osquery`, `/osquery/{key}`, `/osquery-packs/{pack}`, dan `/endpoint-collection`. Memasang agent saja tidak mengizinkan koleksi endpoint. Dukungan runtime bergantung OS/arsitektur dan artefak yang dipatok.

Untuk Admin, `POST /agents/{id}/control` menerima `action`, `version`, dan bila diperlukan `confirmation`. Action yang diizinkan: `pause`, `resume`, `update`, `uninstall`. Pakai `control_version` terbaru; uninstall memakai konfirmasi ID agent yang tepat dan kemampuan protokol yang sesuai. `POST /agents/{id}/collect` menerima pekerjaan koleksi (202), tetap tunduk pada consent/otorisasi/online/dukungan platform. Uninstall yang masih pending pada agent offline bukan bukti penghapusan fisik.

`GET/PUT /agents/rollout` memakai `mode` (`automatic`/`manual`) dan `version`. Kebijakan manual memungkinkan pengujian satu agent; update tetap memeriksa tanda tangan dan anti-downgrade.

Kontrol scan global: `GET /scan-control`, `POST /scan-control/stop`, dan `POST /scan-control/resume`. Versi per organisasi berada di `/organizations/{id}/scan-control`. Stop darurat sengaja mendahulukan penghentian dan tidak membutuhkan body versi; resume/perubahan window memakai versi terbaru.

Contoh window (body PUT endpoint organisasi):

```json
{"version":0,"window":{"timezone":"Asia/Jakarta","days":[1,2,3,4,5],"start":"09:00","end":"17:00"}}
```

Nomor hari ISO: Senin=1, Minggu=7. Akhir waktu eksklusif; window melintasi tengah malam mengikuti hari awal. Window kosong berarti tanpa pembatasan waktu. Hold tetap berlaku sampai Admin membuka kembali.

## Notifikasi dan retensi

`GET /alerts` memuat inbox sesuai hak atas objek; gunakan endpoint `read`, `read-all`, dan `dismiss` untuk tindakan per pengguna. Peran Viewer tidak mendapat akses semua notifikasi hanya karena endpoint inbox tersedia. `GET/PUT /alerts/policy` mengelola ambang offline oleh Admin.

`GET/PUT /retention` mengelola masa simpan. Default dalam source: findings 180 hari, scan 90, report 180, endpoint 30, audit 365; audit minimum 90 hari. Pakai `version` kebijakan terbaru saat menulis.

Penghapusan organisasi memakai dua tahap: `GET /organizations/{id}/deletion-preview`, lalu `DELETE /organizations/{id}` dengan `name`, `version`, dan objek `counts` persis dari preview terbaru. Simpan ID deletion untuk polling `GET /data-deletions/{id}`. Pembersihan objek eksternal dapat masih pending sesudah transaksi database; backup lama memiliki masa simpan tersendiri.

## Cara memakai referensi endpoint

Daftar berikut memisahkan role dasar, parameter yang terbaca di handler, tipe request, dan sumber implementasi. Query yang diteruskan utuh ke service dapat memiliki filter tambahan; baca tautan handler dan service bila perlu. Kode sukses yang diekstrak bersifat indikatif bila handler dipakai ulang oleh beberapa method.

Model Go di lampiran mempertahankan tag JSON asli. `omitempty` menjelaskan serialisasi, **bukan** aturan field wajib pada request. Pointer biasanya dapat merepresentasikan null/tidak diisi; validasi bisnis tetap menentukan nilai yang diterima. UUID diserialisasi sebagai string, `time.Time` sebagai tanggal/waktu, `json.RawMessage` sebagai JSON, dan tipe slice sebagai array. Internal model pada lampiran tidak otomatis merupakan payload yang boleh dikirim.

## Indeks seluruh endpoint

| Method | Path | Role dasar |
|---|---|---|
| DELETE | `/api/v1/asset-groups/{id}` | admin |
| DELETE | `/api/v1/assets/{id}` | admin |
| DELETE | `/api/v1/assets/{id}/endpoint-data` | admin |
| DELETE | `/api/v1/atomic-tests/{id}` | admin |
| DELETE | `/api/v1/me/tokens/{id}` | admin, analyst, viewer |
| DELETE | `/api/v1/people/{id}` | admin, analyst |
| DELETE | `/api/v1/scenarios/{id}` | admin |
| DELETE | `/api/v1/tags/{id}` | admin |
| DELETE | `/api/v1/teams/{id}` | admin, analyst |
| GET | `/api/healthz` | public |
| GET | `/api/v1/action-presets` | admin, analyst |
| GET | `/api/v1/agent/download` | machine |
| GET | `/api/v1/agent/tool` | machine |
| GET | `/api/v1/agents` | admin, analyst |
| GET | `/api/v1/agents/enrollment-tokens/{id}` | admin |
| GET | `/api/v1/ai/jobs/{id}` | admin, analyst |
| GET | `/api/v1/ai/jobs/{id}/export/{format}` | admin, analyst |
| GET | `/api/v1/ai/templates` | admin, analyst |
| GET | `/api/v1/alerts` | admin, analyst, viewer |
| GET | `/api/v1/assessments/{id}` | admin, analyst |
| GET | `/api/v1/asset-groups` | admin, analyst |
| GET | `/api/v1/asset-groups/{id}` | admin, analyst |
| GET | `/api/v1/assets` | admin, analyst |
| GET | `/api/v1/assets/{id}` | admin, analyst |
| GET | `/api/v1/assets/{id}/endpoint-data` | admin, analyst |
| GET | `/api/v1/atomic-tests` | admin, analyst |
| GET | `/api/v1/atomic-tests/{id}` | admin, analyst |
| GET | `/api/v1/audit` | admin |
| GET | `/api/v1/auth/me` | admin, analyst, viewer |
| GET | `/api/v1/contracts` | admin, analyst |
| GET | `/api/v1/demo/status` | admin, analyst |
| GET | `/api/v1/executions/{id}` | admin, analyst |
| GET | `/api/v1/findings` | admin, analyst, viewer |
| GET | `/api/v1/findings/{id}` | admin, analyst, viewer |
| GET | `/api/v1/home` | admin, analyst |
| GET | `/api/v1/integrations` | admin, analyst |
| GET | `/api/v1/me/tokens` | admin, analyst, viewer |
| GET | `/api/v1/mitre/matrix` | admin, analyst |
| GET | `/api/v1/mitre/techniques/{id}` | admin, analyst |
| GET | `/api/v1/organizations` | admin, analyst |
| GET | `/api/v1/organizations/{id}` | admin, analyst |
| GET | `/api/v1/organizations/{id}/ai` | admin, analyst |
| GET | `/api/v1/organizations/{id}/ai/jobs` | admin, analyst |
| GET | `/api/v1/organizations/{id}/ai/preview` | admin, analyst |
| GET | `/api/v1/organizations/{id}/assessments` | admin, analyst |
| GET | `/api/v1/organizations/{id}/osquery` | admin, analyst |
| GET | `/api/v1/organizations/{id}/readiness` | admin, analyst |
| GET | `/api/v1/organizations/{org}/agents/{agent}/networks` | admin, analyst |
| GET | `/api/v1/osquery/catalog` | admin, analyst |
| GET | `/api/v1/people` | admin, analyst |
| GET | `/api/v1/people/{id}` | admin, analyst |
| GET | `/api/v1/reports` | admin, analyst, viewer |
| GET | `/api/v1/reports/{id}` | admin, analyst, viewer |
| GET | `/api/v1/reports/{id}/export/{format}` | admin, analyst, viewer |
| GET | `/api/v1/roles` | admin |
| GET | `/api/v1/scans` | admin, analyst |
| GET | `/api/v1/scans/{id}` | admin, analyst |
| GET | `/api/v1/scenarios` | admin, analyst |
| GET | `/api/v1/scenarios/{id}` | admin, analyst |
| GET | `/api/v1/settings` | admin, analyst, viewer |
| GET | `/api/v1/simulations` | admin, analyst |
| GET | `/api/v1/simulations/{id}` | admin, analyst |
| GET | `/api/v1/simulations/{id}/attack-path` | admin, analyst |
| GET | `/api/v1/tags` | admin, analyst |
| GET | `/api/v1/teams` | admin, analyst |
| GET | `/api/v1/teams/{id}` | admin, analyst |
| GET | `/api/v1/users` | admin |
| PATCH | `/api/v1/assets/{id}` | admin, analyst |
| PATCH | `/api/v1/atomic-tests/{id}` | admin, analyst |
| PATCH | `/api/v1/integrations/{id}` | admin |
| PATCH | `/api/v1/me` | admin, analyst, viewer |
| PATCH | `/api/v1/organizations/{id}` | admin, analyst |
| PATCH | `/api/v1/people/{id}` | admin, analyst |
| PATCH | `/api/v1/scenarios/{id}` | admin, analyst |
| PATCH | `/api/v1/settings` | admin |
| PATCH | `/api/v1/tags/{id}` | admin, analyst |
| PATCH | `/api/v1/teams/{id}` | admin, analyst |
| PATCH | `/api/v1/users/{id}` | admin |
| POST | `/api/v1/action-presets` | admin, analyst |
| POST | `/api/v1/agent/enroll` | public |
| POST | `/api/v1/agent/executions/claim` | machine |
| POST | `/api/v1/agent/executions/{id}/heartbeat` | machine |
| POST | `/api/v1/agent/executions/{id}/result` | machine |
| POST | `/api/v1/agent/heartbeat` | machine |
| POST | `/api/v1/agents/enrollment-tokens` | admin |
| POST | `/api/v1/agents/{id}/revoke` | admin |
| POST | `/api/v1/ai-worker/claim` | machine |
| POST | `/api/v1/ai-worker/jobs/{id}/check` | machine |
| POST | `/api/v1/ai-worker/jobs/{id}/complete` | machine |
| POST | `/api/v1/ai/jobs/{id}/approve` | admin, analyst |
| POST | `/api/v1/ai/jobs/{id}/cancel` | admin, analyst |
| POST | `/api/v1/ai/templates` | admin |
| POST | `/api/v1/alerts/read-all` | admin, analyst, viewer |
| POST | `/api/v1/alerts/{id}/dismiss` | admin, analyst, viewer |
| POST | `/api/v1/alerts/{id}/read` | admin, analyst, viewer |
| POST | `/api/v1/assessments/{id}/cancel` | admin, analyst |
| POST | `/api/v1/asset-groups` | admin, analyst |
| POST | `/api/v1/asset-groups/preview` | admin, analyst |
| POST | `/api/v1/assets` | admin, analyst |
| POST | `/api/v1/assets/import` | admin, analyst |
| POST | `/api/v1/assets/import/preview` | admin, analyst |
| POST | `/api/v1/assets/{id}/endpoint-data/refresh` | admin, analyst |
| POST | `/api/v1/atomic-tests` | admin, analyst |
| POST | `/api/v1/atomic-tests/{id}/run` | admin, analyst |
| POST | `/api/v1/auth/forgot` | public |
| POST | `/api/v1/auth/login` | public |
| POST | `/api/v1/auth/logout` | admin, analyst, viewer |
| POST | `/api/v1/auth/reset` | public |
| POST | `/api/v1/auth/reset/check` | public |
| POST | `/api/v1/demo/purge` | admin |
| POST | `/api/v1/executions/{id}/cancel` | admin, analyst |
| POST | `/api/v1/injector/executions/{id}/claim` | machine |
| POST | `/api/v1/injector/executions/{id}/heartbeat` | machine |
| POST | `/api/v1/injector/executions/{id}/result` | machine |
| POST | `/api/v1/injector/heartbeat` | machine |
| POST | `/api/v1/integrations/{id}/token` | admin |
| POST | `/api/v1/me/password` | admin, analyst, viewer |
| POST | `/api/v1/me/tokens` | admin, analyst, viewer |
| POST | `/api/v1/mitre/import` | admin |
| POST | `/api/v1/organizations` | admin, analyst |
| POST | `/api/v1/organizations/{id}/ai/jobs` | admin, analyst |
| POST | `/api/v1/organizations/{id}/assessments` | admin, analyst |
| POST | `/api/v1/people` | admin, analyst |
| POST | `/api/v1/reports/{id}/retry` | admin, analyst |
| POST | `/api/v1/scan-jobs/{id}/asset` | admin, analyst |
| POST | `/api/v1/scans` | admin, analyst |
| POST | `/api/v1/scans/{id}/cancel` | admin, analyst |
| POST | `/api/v1/scenarios` | admin, analyst |
| POST | `/api/v1/scenarios/{id}/launch` | admin, analyst |
| POST | `/api/v1/simulations/{id}/cancel` | admin, analyst |
| POST | `/api/v1/simulations/{id}/dry-run` | admin, analyst |
| POST | `/api/v1/simulations/{id}/pause` | admin, analyst |
| POST | `/api/v1/simulations/{id}/resume` | admin, analyst |
| POST | `/api/v1/tags` | admin, analyst |
| POST | `/api/v1/teams` | admin, analyst |
| POST | `/api/v1/users` | admin |
| POST | `/api/v1/users/{id}/reset-link` | admin |
| PUT | `/api/v1/asset-groups/{id}` | admin, analyst |
| PUT | `/api/v1/organizations/{id}/ai` | admin |
| PUT | `/api/v1/organizations/{id}/assets` | admin, analyst |
| PUT | `/api/v1/organizations/{id}/endpoint-collection` | admin |
| PUT | `/api/v1/organizations/{id}/osquery-packs/{pack}` | admin |
| PUT | `/api/v1/organizations/{id}/osquery/{key}` | admin |
| PUT | `/api/v1/organizations/{org}/agents/{agent}/networks` | admin |
| PUT | `/api/v1/scenarios/{id}/graph` | admin, analyst |
| PUT | `/api/v1/scenarios/{id}/targets` | admin, analyst |
| PUT | `/api/v1/teams/{id}/members` | admin, analyst |
| GET | `/api/v1/scan-control` | admin |
| POST | `/api/v1/scan-control/stop` | admin |
| POST | `/api/v1/scan-control/resume` | admin |
| GET | `/api/v1/organizations/{id}/scan-control` | admin, analyst |
| PUT | `/api/v1/organizations/{id}/scan-control` | admin |
| POST | `/api/v1/organizations/{id}/scan-control/stop` | admin |
| POST | `/api/v1/organizations/{id}/scan-control/resume` | admin |
| GET | `/api/v1/retention` | admin |
| PUT | `/api/v1/retention` | admin |
| GET | `/api/v1/organizations/{id}/deletion-preview` | admin |
| DELETE | `/api/v1/organizations/{id}` | admin |
| GET | `/api/v1/data-deletions/{id}` | admin |
| GET | `/api/v1/agents/rollout` | admin |
| PUT | `/api/v1/agents/rollout` | admin |
| POST | `/api/v1/agents/{id}/control` | admin |
| POST | `/api/v1/agents/{id}/collect` | admin |
| POST | `/api/v1/agent/control/ack` | machine |
| GET | `/api/v1/alerts/policy` | admin |
| PUT | `/api/v1/alerts/policy` | admin |
| PATCH | `/api/v1/auth/preferences` | admin, analyst, viewer |

## Referensi endpoint

### DELETE /api/v1/asset-groups/{id}

Hapus asset-groups. Role dasar: **admin**.

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 470 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/asset-groups/{id}", auth.Require(authz.AssetDelete, h.Delete))
```

### DELETE /api/v1/assets/{id}

Hapus assets. Role dasar: **admin**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 108 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/assets/{id}", auth.Require(authz.AssetDelete, h.Delete))
```

### DELETE /api/v1/assets/{id}/endpoint-data

Hapus assets · endpoint-data. Role dasar: **admin**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 518 pada snapshot commit.

Parameter query yang dibaca handler: `version`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/assets/{id}/endpoint-data", auth.Require(authz.AssetWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if !authz.Has(p.Permissions, authz.SettingsManage) {
		httpx.Write(w, r, httpx.Forbidden(authz.SettingsManage))
		return
	}
	asset, err := id(r, "id", "Asset")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	version, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil || version < 1 {
		httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "version", Message: "Reload the asset before deleting collected data."}))
		return
	}
	if err = h.Svc.DeleteData(r.Context(), p, asset, version, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### DELETE /api/v1/atomic-tests/{id}

Hapus atomic-tests. Role dasar: **admin**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 77 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/atomic-tests/{id}", auth.Require(authz.AtomicDelete, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Atomic test")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.Svc.DeleteTest(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### DELETE /api/v1/me/tokens/{id}

Hapus me · tokens. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 166 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/me/tokens/{id}", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uid(r, "Token")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.Svc.RevokeToken(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### DELETE /api/v1/people/{id}

Hapus people. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 545 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/people/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Person")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	a, b := ip(r)
	if err := h.Svc.DeletePerson(r.Context(), p, id, a, b); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### DELETE /api/v1/scenarios/{id}

Hapus scenarios. Role dasar: **admin**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 68 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/scenarios/{id}", auth.Require(authz.ScenarioDelete, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	sid, err := id(r)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.Svc.Delete(r.Context(), p, sid, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### DELETE /api/v1/tags/{id}

Hapus tags. Role dasar: **admin**.

Sumber: [internal/tags/tags.go](../../internal/tags/tags.go), baris 201 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/tags/{id}", auth.Require(authz.AssetDelete, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Tag"))
		return
	}
	if err := h.Svc.Delete(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### DELETE /api/v1/teams/{id}

Hapus teams. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 594 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("DELETE /api/v1/teams/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Team")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	a, b := ip(r)
	if err := h.Svc.DeleteTeam(r.Context(), p, id, a, b); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### GET /api/healthz

Baca  · api · healthz. Role dasar: **public**.

Sumber: [internal/server/server.go](../../internal/server/server.go), baris 76 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := o.DB.Pool.Ping(ctx); err != nil {
		httpx.Write(w, r, httpx.NewProblem(503, "Database unavailable", ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
})
```

### GET /api/v1/action-presets

Baca action-presets. Role dasar: **admin, analyst**.

Sumber: [internal/execution/presets.go](../../internal/execution/presets.go), baris 41 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/action-presets", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	items := []Preset{}
	e := h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(r.Context(), `SELECT id,name,contract_key,contract_version,params,version FROM action_presets WHERE tenant_id=$1 ORDER BY name LIMIT 200`, p.TenantID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v Preset
			if e = rows.Scan(&v.ID, &v.Name, &v.ContractKey, &v.ContractVersion, &v.Params, &v.Version); e != nil {
				return e
			}
			items = append(items, v)
		}
		return rows.Err()
	})
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": items})
}))
```

### GET /api/v1/agent/download

Baca agent · download. Role dasar: **machine**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 151 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET "+AgentPrefix+"download", h.download)
```

### GET /api/v1/agent/tool

Baca agent · tool. Role dasar: **machine**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 152 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET "+AgentPrefix+"tool", h.guard(h.tool))
```

### GET /api/v1/agents

Baca agents. Role dasar: **admin, analyst**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 56 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/agents", auth.Require(authz.AgentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.List(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/agents/enrollment-tokens/{id}

Baca agents · enrollment-tokens. Role dasar: **admin**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 69 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/agents/enrollment-tokens/{id}", auth.Require(authz.AgentManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Enrollment token"))
		return
	}
	v, err := h.Svc.EnrollmentTokenStatus(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/ai/jobs/{id}

Baca ai · jobs. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 114 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/ai/jobs/{id}", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	v, e := s.Get(r.Context(), p, id)
	reply(w, r, v, e)
}))
```

### GET /api/v1/ai/jobs/{id}/export/{format}

Baca ai · jobs · export · {format}. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 206 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) export(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	switch r.PathValue("format") {
	case "json", "pdf":
	default:
		httpx.Write(w, r, httpx.NotFound("Export format"))
		return
	}
	j, e := h.Svc.Get(r.Context(), p, id)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	if j.Output == nil {
		httpx.Write(w, r, httpx.Conflict("AI output is not available."))
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ai-%s.%s"`, id, r.PathValue("format")))
	switch r.PathValue("format") {
	case "json":
		httpx.JSON(w, 200, j)
	case "pdf":
		var html bytes.Buffer
		if e := adviceTemplate.Execute(&html, j); e != nil {
			httpx.Write(w, r, e)
			return
		}
		renderer := h.Reports
		if renderer == nil {
			renderer = &reporting.Service{Script: os.Getenv("ORCHIS_REPORT_RENDER_SCRIPT")}
		}
		pdf, e := renderer.RenderPDF(r.Context(), html.Bytes())
		if e != nil {
			httpx.Write(w, r, httpx.NewProblem(503, "PDF unavailable", "The AI advice is saved. JSON remains available."))
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	default:
		httpx.Write(w, r, httpx.NotFound("Export format"))
	}
}
```

### GET /api/v1/ai/templates

Baca ai · templates. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 150 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/ai/templates", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	list := []map[string]any{}
	e := s.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(r.Context(), `SELECT id,kind,version,instructions,is_demo FROM ai_templates WHERE tenant_id=$1 ORDER BY kind,version DESC LIMIT 100`, p.TenantID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var kind, text string
			var v int
			var synthetic bool
			if e = rows.Scan(&id, &kind, &v, &text, &synthetic); e != nil {
				return e
			}
			list = append(list, map[string]any{"id": id, "kind": kind, "version": v, "instructions": text, "is_demo": synthetic})
		}
		return rows.Err()
	})
	reply(w, r, map[string]any{"items": list, "builtin_instructions": Instructions, "builtin_version": PromptVersion}, e)
}))
```

### GET /api/v1/alerts

Baca alerts. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/alerts/alerts.go](../../internal/alerts/alerts.go), baris 200 pada snapshot commit.

Parameter query yang dibaca handler: `limit`, `organization_id`, `unread`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/alerts", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var org *uuid.UUID
	if raw := r.URL.Query().Get("organization_id"); raw != "" {
		id, e := uuid.Parse(raw)
		if e != nil {
			httpx.Write(w, r, httpx.BadRequest("Invalid organization filter."))
			return
		}
		org = &id
	}
	l, err := h.Svc.ListOrganization(r.Context(), p, r.URL.Query().Get("unread") == "1", limit, org)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, l)
}))
```

### GET /api/v1/assessments/{id}

Baca assessments. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 135 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/assessments/{id}", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Assessment(r.Context(), p, id)
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/asset-groups

Baca asset-groups. Role dasar: **admin, analyst**.

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 465 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	list, err := h.Svc.List(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list, "next_cursor": nil})
}
```

### GET /api/v1/asset-groups/{id}

Baca asset-groups. Role dasar: **admin, analyst**.

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 467 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := h.Svc.Get(r.Context(), p, id)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}
```

### GET /api/v1/assets

Baca assets. Role dasar: **admin, analyst**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 102 pada snapshot commit.

Parameter query yang dibaca handler: `category`, `cursor`, `limit`, `org`, `organization_id`, `q`.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "limit", Message: "Must be a positive integer."}))
			return
		}
		limit = n
	}
	organization, err := httpx.OrganizationParam(q, true)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	page, err := h.Svc.List(r.Context(), p, ListFilter{Organization: organization, Category: q.Get("category"), Query: q.Get("q"), Limit: limit, Cursor: q.Get("cursor")})
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}
```

### GET /api/v1/assets/{id}

Baca assets. Role dasar: **admin, analyst**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 103 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Asset"))
		return
	}
	a, err := h.Svc.Get(r.Context(), p, id)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}
```

### GET /api/v1/assets/{id}/endpoint-data

Baca assets · endpoint-data. Role dasar: **admin, analyst**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 539 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/assets/{id}/endpoint-data", auth.Require(authz.AssetRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	asset, err := id(r, "id", "Asset")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	d, err := h.Svc.AssetData(r.Context(), p, asset)
	reply(w, r, 200, d, err)
}))
```

### GET /api/v1/atomic-tests

Baca atomic-tests. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 41 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/atomic-tests", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.ListTests(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/atomic-tests/{id}

Baca atomic-tests. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 54 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/atomic-tests/{id}", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Atomic test")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.GetTest(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/audit

Baca audit. Role dasar: **admin**.

Sumber: [internal/auditview/http.go](../../internal/auditview/http.go), baris 42 pada snapshot commit.

Parameter query yang dibaca handler: `action`, `cursor`, `limit`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handler) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "limit", Message: "Choose a page size from 1 to 100."}))
			return
		}
		limit = n
	}
	var after *cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var c cursor
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 1024 || e != nil || json.Unmarshal(b, &c) != nil || c.ID == uuid.Nil || c.At.IsZero() {
			httpx.Write(w, r, httpx.BadRequest("Invalid audit cursor."))
			return
		}
		after = &c
	}
	action := r.URL.Query().Get("action")
	if len(action) > 100 {
		httpx.Write(w, r, httpx.BadRequest("Invalid audit action filter."))
		return
	}
	page, e := h.list(r.Context(), p.TenantID, limit, after, action)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, page)
}
```

### GET /api/v1/auth/me

Baca auth · me. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/auth/http.go](../../internal/auth/http.go), baris 141 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request, p *Principal) {
	httpx.JSON(w, http.StatusOK, h.toDTO(p))
}
```

### GET /api/v1/contracts

Baca contracts. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 37 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/contracts", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.Contracts(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/demo/status

Baca demo · status. Role dasar: **admin, analyst**.

Sumber: [internal/demo/http.go](../../internal/demo/http.go), baris 16 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/demo/status", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	st, err := h.Svc.Status(r.Context(), p.TenantID)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}))
```

### GET /api/v1/executions/{id}

Baca executions. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 98 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/executions/{id}", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Execution")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.GetExecution(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/findings

Baca findings. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 116 pada snapshot commit.

Parameter query yang dibaca handler: `asset_id`, `cursor`, `limit`, `org`, `organization_id`, `q`, `severity`, `simulation_id`, `type`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/findings", auth.Require(authz.FindingRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	q := r.URL.Query()
	organization, err := httpx.OrganizationParam(q, false)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	f := FindingFilter{Type: q.Get("type"), Severity: q.Get("severity"), Query: q.Get("q"), Cursor: q.Get("cursor")}
	if organization != "" {
		id := uuid.MustParse(organization)
		f.OrganizationID = &id
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "limit", Message: "Must be a positive integer."}))
			return
		}
		f.Limit = n
	}
	if v := q.Get("asset_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "asset_id", Message: "Invalid asset id."}))
			return
		}
		f.AssetID = &id
	}
	if v := q.Get("simulation_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "simulation_id", Message: "Invalid simulation id."}))
			return
		}
		f.SimulationID = &id
	}
	v, err := h.Svc.ListFindings(r.Context(), p, f, h.Search)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/findings/{id}

Baca findings. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 155 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/findings/{id}", auth.Require(authz.FindingRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Finding")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.GetFinding(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/home

Baca home. Role dasar: **admin, analyst**.

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 284 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/home", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	s, err := h.Svc.Summary(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}))
```

### GET /api/v1/integrations

Baca integrations. Role dasar: **admin, analyst**.

Sumber: [internal/integrations/integrations.go](../../internal/integrations/integrations.go), baris 172 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/integrations", auth.Require(authz.IntegrationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.List(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": v})
}))
```

### GET /api/v1/me/tokens

Baca me · tokens. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 151 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/me/tokens", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.Tokens(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/mitre/matrix

Baca mitre · matrix. Role dasar: **admin, analyst**.

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 302 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/mitre/matrix", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	m, err := h.Svc.Matrix(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, m)
}))
```

### GET /api/v1/mitre/techniques/{id}

Baca mitre · techniques. Role dasar: **admin, analyst**.

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 310 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/mitre/techniques/{id}", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	d, err := h.Svc.Technique(r.Context(), p, r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, d)
}))
```

### GET /api/v1/organizations

Baca organizations. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 44 pada snapshot commit.

Parameter query yang dibaca handler: `cursor`, `limit`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/organizations", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	limit := 25
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 {
			httpx.Write(w, r, httpx.BadRequest("Invalid limit."))
			return
		}
		limit = n
	}
	v, e := h.Svc.List(r.Context(), p, limit, r.URL.Query().Get("cursor"))
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/organizations/{id}

Baca organizations. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 57 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/organizations/{id}", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Get(r.Context(), p, id)
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/organizations/{id}/ai

Baca organizations · ai. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 55 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/organizations/{id}/ai", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	v, e := s.Policy(r.Context(), p, org)
	reply(w, r, v, e)
}))
```

### GET /api/v1/organizations/{id}/ai/jobs

Baca organizations · ai · jobs. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 89 pada snapshot commit.

Parameter query yang dibaca handler: `cursor`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/organizations/{id}/ai/jobs", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	v, e := s.List(r.Context(), p, org, r.URL.Query().Get("cursor"))
	next := ""
	if len(v) == 50 {
		next = v[49].ID.String()
	}
	reply(w, r, map[string]any{"items": v, "next_cursor": next}, e)
}))
```

### GET /api/v1/organizations/{id}/ai/preview

Baca organizations · ai · preview. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 76 pada snapshot commit.

Parameter query yang dibaca handler: `report_id`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/organizations/{id}/ai/preview", access("ai.generate", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	report, e := uuid.Parse(r.URL.Query().Get("report_id"))
	if e != nil {
		httpx.Write(w, r, httpx.BadRequest("Choose a report."))
		return
	}
	v, _, e := s.Preview(r.Context(), p, org, report)
	reply(w, r, v, e)
}))
```

### GET /api/v1/organizations/{id}/assessments

Baca organizations · assessments. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 126 pada snapshot commit.

Parameter query yang dibaca handler: `cursor`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/organizations/{id}/assessments", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Assessments(r.Context(), p, id, r.URL.Query().Get("cursor"))
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/organizations/{id}/osquery

Baca organizations · osquery. Role dasar: **admin, analyst**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 457 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/organizations/{id}/osquery", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, err := id(r, "id", "Organization")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	items, err := h.Svc.OrgSettings(r.Context(), p, org)
	reply(w, r, 200, map[string]any{"items": items}, err)
}))
```

### GET /api/v1/organizations/{id}/readiness

Periksa kesiapan organisasi. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 35 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/organizations/{id}/readiness", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Readiness(r.Context(), p, id)
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/organizations/{org}/agents/{agent}/networks

Baca organizations · {org} · agents · {agent} · networks. Role dasar: **admin, analyst**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 79 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc(method+" /api/v1/organizations/{org}/agents/{agent}/networks", auth.Require(authz.AgentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, e := pathID(r, "org")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	agent, e := pathID(r, "agent")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	if r.Method == "GET" {
		v, e := h.Svc.Networks(r.Context(), p, org, agent)
		reply(w, r, 200, v, e)
		return
	}
	var in ApproveInput
	if e = httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.AuthorizeRange(r.Context(), p, org, agent, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/osquery/catalog

Baca osquery · catalog. Role dasar: **admin, analyst**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 450 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/osquery/catalog", auth.Require(authz.AssetRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	catalog := CatalogView()
	if h.Svc.CollectionInterval > 0 {
		catalog.CollectionIntervalSeconds = int(h.Svc.CollectionInterval.Seconds())
	}
	httpx.JSON(w, 200, catalog)
}))
```

### GET /api/v1/people

Baca people. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 509 pada snapshot commit.

Parameter query yang dibaca handler: `q`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/people", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.ListPeople(r.Context(), p, r.URL.Query().Get("q"))
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/people/{id}

Baca people. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 522 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/people/{id}", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Person")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.GetPerson(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/reports

Baca reports. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/reporting/http.go](../../internal/reporting/http.go), baris 59 pada snapshot commit.

Parameter query yang dibaca handler: `scan_only`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/reports", auth.Require(authz.ReportRead, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var org, assessment, cur *uuid.UUID
	for key, dst := range map[string]**uuid.UUID{"organization_id": &org, "assessment_id": &assessment, "cursor": &cur} {
		raw := r.URL.Query().Get(key)
		if raw == "" {
			continue
		}
		if key == "cursor" {
			b, e := base64.RawURLEncoding.DecodeString(raw)
			if e != nil {
				httpx.Write(w, r, httpx.BadRequest("Invalid cursor."))
				return
			}
			raw = string(b)
		}
		id, e := uuid.Parse(raw)
		if e != nil {
			httpx.Write(w, r, httpx.BadRequest("Invalid filter."))
			return
		}
		*dst = &id
	}
	items := []Report{}
	var next *string
	e := h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(r.Context(), `SELECT id,organization_id,assessment_id,version,status,error,created_at,snapshot-'assets'-'comparison_details' FROM reports WHERE tenant_id=$1 AND ($2::uuid IS NULL OR organization_id=$2) AND ($3::uuid IS NULL OR assessment_id=$3) AND ($4::uuid IS NULL OR id<$4) AND (NOT $5::boolean OR snapshot ? 'scan_id') ORDER BY id DESC LIMIT 26`, p.TenantID, org, assessment, cur, r.URL.Query().Get("scan_only") == "true")
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := scanReport(rows)
			if e != nil {
				return e
			}
			v.Snapshot.Assets = nil
			items = append(items, v)
		}
		return rows.Err()
	})
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	if len(items) > 25 {
		items = items[:25]
		v := base64.RawURLEncoding.EncodeToString([]byte(items[24].ID.String()))
		next = &v
	}
	httpx.JSON(w, 200, map[string]any{"items": items, "next_cursor": next})
})))
```

### GET /api/v1/reports/{id}

Baca reports. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/reporting/http.go](../../internal/reporting/http.go), baris 110 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/reports/{id}", auth.Require(authz.ReportRead, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		httpx.Write(w, r, httpx.NotFound("Report"))
		return
	}
	v, e := h.Svc.Get(r.Context(), p.TenantID, id)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, v)
})))
```

### GET /api/v1/reports/{id}/export/{format}

Baca reports · export · {format}. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/reporting/http.go](../../internal/reporting/http.go), baris 123 pada snapshot commit.

Parameter query yang dibaca handler: `preview`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/reports/{id}/export/{format}", auth.Require(authz.ReportRead, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		httpx.Write(w, r, httpx.NotFound("Report"))
		return
	}
	v, e := h.Svc.Get(r.Context(), p.TenantID, id)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	var data []byte
	format := r.PathValue("format")
	content := ""
	switch format {
	case "json":
		content = "application/json"
		data, e = json.MarshalIndent(v.Snapshot, "", "  ")
	case "pdf":
		content = "application/pdf"
		if v.Status != "ready" {
			httpx.Write(w, r, httpx.Conflict("The PDF is not ready. JSON is available."))
			return
		}
		store, ok := h.Svc.Store.(interface {
			Get(context.Context, string) ([]byte, error)
		})
		if !ok {
			httpx.Write(w, r, httpx.NewProblem(503, "Report storage unavailable", ""))
			return
		}
		data, e = store.Get(r.Context(), p.TenantID.String()+"/reports/"+id.String()+".pdf")
	default:
		httpx.Write(w, r, httpx.NotFound("Export format"))
		return
	}
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	w.Header().Set("Content-Type", content)
	disposition := "attachment"
	if format == "pdf" && r.URL.Query().Get("preview") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+"; filename=\"orchis-report-"+id.String()+"."+format+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
})))
```

### GET /api/v1/roles

Baca roles. Role dasar: **admin**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 46 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/roles", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.Roles(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/scans

Baca scans. Role dasar: **admin, analyst**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 37 pada snapshot commit.

Parameter query yang dibaca handler: `cursor`, `org`, `organization_id`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/scans", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	raw, e := httpx.OrganizationParam(r.URL.Query(), false)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	var org *uuid.UUID
	if raw != "" {
		v := uuid.MustParse(raw)
		org = &v
	}
	v, next, e := h.Svc.List(r.Context(), p, org, r.URL.Query().Get("cursor"))
	reply(w, r, 200, map[string]any{"items": v, "next_cursor": next}, e)
}))
```

### GET /api/v1/scans/{id}

Baca scans. Role dasar: **admin, analyst**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 51 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/scans/{id}", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := pathID(r, "id")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Get(r.Context(), p, id)
	reply(w, r, 200, v, e)
}))
```

### GET /api/v1/scenarios

Baca scenarios. Role dasar: **admin, analyst**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 32 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/scenarios", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.List(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/scenarios/{id}

Baca scenarios. Role dasar: **admin, analyst**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 45 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/scenarios/{id}", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	sid, err := id(r)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Get(r.Context(), p, sid)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/settings

Baca settings. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/settings/settings.go](../../internal/settings/settings.go), baris 118 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/settings", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.Get(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, v)
}))
```

### GET /api/v1/simulations

Baca simulations. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 49 pada snapshot commit.

Parameter query yang dibaca handler: `status`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/simulations", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	st := r.URL.Query().Get("status")
	if st != "" && !statuses[st] {
		httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "status", Message: "Unknown status."}))
		return
	}
	v, err := h.Svc.List(r.Context(), p, ListFilter{Status: st})
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/simulations/{id}

Baca simulations. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 58 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/simulations/{id}", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Simulation")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Get(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/simulations/{id}/attack-path

Baca simulations · attack-path. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 67 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/simulations/{id}/attack-path", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Simulation")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.AttackPath(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/tags

Baca tags. Role dasar: **admin, analyst**.

Sumber: [internal/tags/tags.go](../../internal/tags/tags.go), baris 178 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	list, err := h.Svc.List(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}
```

### GET /api/v1/teams

Baca teams. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 558 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/teams", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.ListTeams(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### GET /api/v1/teams/{id}

Baca teams. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 571 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/teams/{id}", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Team")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.GetTeam(r.Context(), p, id)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/users

Baca users. Role dasar: **admin**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 42 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/users", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, err := h.Svc.List(r.Context(), p)
	reply(w, r, 200, map[string]any{"items": v}, err)
}))
```

### PATCH /api/v1/assets/{id}

Perbarui assets. Role dasar: **admin, analyst**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 107 pada snapshot commit.

Model request: `assets.PatchInput` (lihat lampiran model).

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Asset"))
		return
	}
	var in PatchInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	a, err := h.Svc.Patch(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}
```

### PATCH /api/v1/atomic-tests/{id}

Perbarui atomic-tests. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 63 pada snapshot commit.

Model request: `execution.TestPatch` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/atomic-tests/{id}", auth.Require(authz.AtomicWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Atomic test")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in TestPatch
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.PatchTest(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### PATCH /api/v1/integrations/{id}

Perbarui integrations. Role dasar: **admin**.

Sumber: [internal/integrations/integrations.go](../../internal/integrations/integrations.go), baris 180 pada snapshot commit.

Field JSON pada handler: `enabled`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/integrations/{id}", auth.Require(authz.IntegrationManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Integration"))
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if in.Enabled == nil {
		httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "enabled", Message: "Send true or false."}))
		return
	}
	if err := h.Svc.SetEnabled(r.Context(), p, id, *in.Enabled, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### PATCH /api/v1/me

Perbarui me. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 124 pada snapshot commit.

Field JSON pada handler: `display_name`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/me", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	in, ok := decode[struct {
		DisplayName string `json:"display_name"`
	}](w, r)
	if !ok {
		return
	}
	if err := h.Svc.UpdateProfile(r.Context(), p, in.DisplayName); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### PATCH /api/v1/organizations/{id}

Perbarui organizations. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 72 pada snapshot commit.

Model request: `organizations.Input` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc(method+" "+path, auth.Require(authz.OrganizationWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Input
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	id := uuid.Nil
	if method == "PATCH" {
		var e error
		id, e = parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
	}
	v, e := h.Svc.Save(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	status := 200
	if method == "POST" {
		status = 201
	}
	reply(w, r, status, v, e)
}))
```

### PATCH /api/v1/people/{id}

Perbarui people. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 531 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/people/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Person")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	in, ok := body[PersonPatch](w, r)
	if !ok {
		return
	}
	a, b := ip(r)
	v, err := h.Svc.PatchPerson(r.Context(), p, id, in, a, b)
	reply(w, r, 200, v, err)
}))
```

### PATCH /api/v1/scenarios/{id}

Perbarui scenarios. Role dasar: **admin, analyst**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 54 pada snapshot commit.

Model request: `scenario.Patch` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/scenarios/{id}", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	sid, err := id(r)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in Patch
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Patch(r.Context(), p, sid, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### PATCH /api/v1/settings

Perbarui settings. Role dasar: **admin**.

Sumber: [internal/settings/settings.go](../../internal/settings/settings.go), baris 126 pada snapshot commit.

Field JSON pada handler: `platform_name`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/settings", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		PlatformName string `json:"platform_name"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.Svc.SetName(r.Context(), p, in.PlatformName, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### PATCH /api/v1/tags/{id}

Perbarui tags. Role dasar: **admin, analyst**.

Sumber: [internal/tags/tags.go](../../internal/tags/tags.go), baris 180 pada snapshot commit.

Field JSON pada handler: `color`, `name`.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/tags/{id}", auth.Require(authz.AssetWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Tag"))
		return
	}
	var in struct {
		Name	string	`json:"name"`
		Color	string	`json:"color"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	t, err := h.Svc.Update(r.Context(), p, id, in.Name, in.Color, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}))
```

### PATCH /api/v1/teams/{id}

Perbarui teams. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 580 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/teams/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Team")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	in, ok := body[TeamPatch](w, r)
	if !ok {
		return
	}
	a, b := ip(r)
	v, err := h.Svc.PatchTeam(r.Context(), p, id, in, a, b)
	reply(w, r, 200, v, err)
}))
```

### PATCH /api/v1/users/{id}

Perbarui users. Role dasar: **admin**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 58 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PATCH /api/v1/users/{id}", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uid(r, "User")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	in, ok := decode[PatchInput](w, r)
	if !ok {
		return
	}
	v, err := h.Svc.Patch(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### POST /api/v1/action-presets

Buat / jalankan action-presets. Role dasar: **admin, analyst**.

Sumber: [internal/execution/presets.go](../../internal/execution/presets.go), baris 64 pada snapshot commit.

Field JSON pada handler: `contract_key`, `name`, `params`.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/action-presets", auth.Require(authz.AtomicWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		Name	string		`json:"name"`
		Key	string		`json:"contract_key"`
		Params	json.RawMessage	`json:"params"`
	}
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	name, e := validateName(in.Name)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	def := contract.Find(in.Key)
	if def == nil {
		httpx.Write(w, r, httpx.BadRequest("Unknown action."))
		return
	}
	params, e := contract.Normalize(in.Key, in.Params)
	if e != nil {
		httpx.Write(w, r, paramProblem(e))
		return
	}
	v := Preset{ID: ids.New(), Name: name, ContractKey: in.Key, ContractVersion: def.Version, Params: params, Version: 1}
	e = h.Svc.DB.InTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		_, e := tx.Exec(r.Context(), `INSERT INTO action_presets(id,tenant_id,name,contract_key,contract_version,params) VALUES($1,$2,$3,$4,$5,$6)`, v.ID, p.TenantID, v.Name, v.ContractKey, v.ContractVersion, []byte(v.Params))
		if e != nil {
			var pe *pgconn.PgError
			if errors.As(e, &pe) && pe.Code == "23505" {
				return httpx.Conflict("A preset with this name already exists.")
			}
			return e
		}
		return audit.Write(r.Context(), tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "action_preset.create", ResourceType: "action_preset", ResourceID: &v.ID, IP: auth.ClientIP(r), RequestID: httpx.RequestIDFrom(r.Context())})
	})
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 201, v)
}))
```

### POST /api/v1/agent/enroll

Daftarkan agent. Role dasar: **public**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 92 pada snapshot commit.

Model request: `agents.EnrollInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+AgentPrefix+"enroll", func(w http.ResponseWriter, r *http.Request) {
	var in EnrollInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Enroll(r.Context(), in, auth.ClientIP(r))
	reply(w, r, 200, v, err)
})
```

### POST /api/v1/agent/executions/claim

Klaim pekerjaan mesin. Role dasar: **machine**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 110 pada snapshot commit.

Ekspresi status dalam handler: `200`, `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+AgentPrefix+"executions/claim", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
	spec, err := h.RT.ClaimNext(r.Context(), p.Worker())
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	if spec == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httpx.JSON(w, 200, spec)
}))
```

### POST /api/v1/agent/executions/{id}/heartbeat

Kirim heartbeat mesin. Role dasar: **machine**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 122 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+AgentPrefix+"executions/{id}/heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Execution"))
		return
	}
	cancel, err := h.RT.Heartbeat(r.Context(), p.Worker(), id)
	reply(w, r, 200, map[string]bool{"cancel": cancel}, err)
}))
```

### POST /api/v1/agent/executions/{id}/result

Kirim hasil eksekusi. Role dasar: **machine**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 131 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+AgentPrefix+"executions/{id}/result", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Execution"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	var res execution.Result
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res); err != nil {
		httpx.Write(w, r, httpx.BadRequest("The result is not valid JSON: "+err.Error()))
		return
	}
	if err := h.RT.Complete(context.WithoutCancel(r.Context()), p.Worker(), id, res); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/agent/heartbeat

Kirim heartbeat mesin. Role dasar: **machine**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 101 pada snapshot commit.

Model request: `agents.HeartbeatInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+AgentPrefix+"heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
	var in HeartbeatInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Heartbeat(r.Context(), p, in, auth.ClientIP(r))
	reply(w, r, 200, v, err)
}))
```

### POST /api/v1/agents/enrollment-tokens

Token pendaftaran agent. Role dasar: **admin**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 60 pada snapshot commit.

Model request: `agents.TokenInput` (lihat lampiran model).

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/agents/enrollment-tokens", auth.Require(authz.AgentManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in TokenInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.CreateEnrollmentToken(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/agents/{id}/revoke

Buat / jalankan agents · revoke. Role dasar: **admin**.

Sumber: [internal/agents/http.go](../../internal/agents/http.go), baris 78 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/agents/{id}/revoke", auth.Require(authz.AgentManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Agent"))
		return
	}
	if err := h.Svc.Revoke(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/ai-worker/claim

Klaim pekerjaan mesin. Role dasar: **machine**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 217 pada snapshot commit.

Ekspresi status dalam handler: `204`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/ai-worker/claim", worker(func(w http.ResponseWriter, r *http.Request) {
	v, e := s.Claim(r.Context())
	if e == nil && v == nil {
		w.WriteHeader(204)
		return
	}
	reply(w, r, v, e)
}))
```

### POST /api/v1/ai-worker/jobs/{id}/check

Buat / jalankan ai-worker · jobs · check. Role dasar: **machine**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 225 pada snapshot commit.

Field JSON pada handler: `token`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/ai-worker/jobs/{id}/check", worker(func(w http.ResponseWriter, r *http.Request) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := s.Alive(r.Context(), id, in.Token)
	reply(w, r, map[string]bool{"active": v}, e)
}))
```

### POST /api/v1/ai-worker/jobs/{id}/complete

Buat / jalankan ai-worker · jobs · complete. Role dasar: **machine**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 240 pada snapshot commit.

Model request: `ai.Completion` (lihat lampiran model).

Ekspresi status dalam handler: `204`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/ai-worker/jobs/{id}/complete", worker(func(w http.ResponseWriter, r *http.Request) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	var in Completion
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	if e := s.Complete(r.Context(), id, in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	w.WriteHeader(204)
}))
```

### POST /api/v1/ai/jobs/{id}/approve

Buat / jalankan ai · jobs · approve. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 128 pada snapshot commit.

Field JSON pada handler: `version`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/ai/jobs/{id}/"+op, access(perm, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Version int `json:"version"`
	}
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	var v Job
	var e error
	if op == "approve" {
		v, e = s.Approve(r.Context(), p, id, in.Version)
	} else {
		v, e = s.Cancel(r.Context(), p, id, in.Version)
	}
	reply(w, r, v, e)
}))
```

### POST /api/v1/ai/jobs/{id}/cancel

Batalkan pekerjaan. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 128 pada snapshot commit.

Field JSON pada handler: `version`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/ai/jobs/{id}/"+op, access(perm, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Version int `json:"version"`
	}
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	var v Job
	var e error
	if op == "approve" {
		v, e = s.Approve(r.Context(), p, id, in.Version)
	} else {
		v, e = s.Cancel(r.Context(), p, id, in.Version)
	}
	reply(w, r, v, e)
}))
```

### POST /api/v1/ai/templates

Buat / jalankan ai · templates. Role dasar: **admin**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 172 pada snapshot commit.

Field JSON pada handler: `instructions`, `kind`.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/ai/templates", access("ai.manage", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		Kind		string	`json:"kind"`
		Instructions	string	`json:"instructions"`
	}
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	if (in.Kind != "planning" && in.Kind != "remediation") || !validText(in.Instructions, 6000) {
		httpx.Write(w, r, httpx.BadRequest("Invalid template purpose or instructions."))
		return
	}
	id := ids.New()
	version := 0
	e := s.DB.InTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(r.Context(), tx, p.TenantID); err != nil {
			return err
		}

		if _, e := tx.Exec(r.Context(), `SELECT id FROM tenants WHERE id=$1 FOR UPDATE`, p.TenantID); e != nil {
			return e
		}
		if e := tx.QueryRow(r.Context(), `SELECT COALESCE(max(version),0)+1 FROM ai_templates WHERE tenant_id=$1 AND kind=$2`, p.TenantID, in.Kind).Scan(&version); e != nil {
			return e
		}
		_, e := tx.Exec(r.Context(), `INSERT INTO ai_templates(id,tenant_id,kind,version,instructions,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, p.TenantID, in.Kind, version, in.Instructions, p.UserID)
		if e != nil {
			return e
		}
		return note(r.Context(), tx, p, uuid.Nil, id, "ai.template", map[string]any{"kind": in.Kind, "version": version})
	})
	reply(w, r, map[string]any{"id": id, "version": version}, e)
}))
```

### POST /api/v1/alerts/read-all

Tandai semua notifikasi dibaca. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/alerts/alerts.go](../../internal/alerts/alerts.go), baris 234 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/alerts/read-all", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if err := h.Svc.ReadAll(r.Context(), p); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/alerts/{id}/dismiss

Sembunyikan notifikasi. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/alerts/alerts.go](../../internal/alerts/alerts.go), baris 219 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/alerts/{id}/"+name, auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Alert"))
		return
	}
	if err := f(r.Context(), p, id); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/alerts/{id}/read

Tandai notifikasi dibaca. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/alerts/alerts.go](../../internal/alerts/alerts.go), baris 219 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/alerts/{id}/"+name, auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Alert"))
		return
	}
	if err := f(r.Context(), p, id); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/assessments/{id}/cancel

Batalkan pekerjaan. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 144 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/assessments/{id}/cancel", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	e = h.Svc.Cancel(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, map[string]bool{"canceled": e == nil}, e)
}))
```

### POST /api/v1/asset-groups

Buat / jalankan asset-groups. Role dasar: **admin, analyst**.

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 468 pada snapshot commit.

Model request: `groups.Input` (lihat lampiran model).

Ekspresi status dalam handler: `Created`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	d, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/asset-groups/"+d.ID.String())
	httpx.JSON(w, http.StatusCreated, d)
}
```

### POST /api/v1/asset-groups/preview

Buat / jalankan asset-groups · preview. Role dasar: **admin, analyst**.

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 466 pada snapshot commit.

Field JSON pada handler: `rule`.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Preview(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		Rule Rule `json:"rule"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	pv, err := h.Svc.Preview(r.Context(), p, in.Rule)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pv)
}
```

### POST /api/v1/assets

Buat / jalankan assets. Role dasar: **admin, analyst**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 104 pada snapshot commit.

Model request: `assets.CreateInput` (lihat lampiran model).

Ekspresi status dalam handler: `Created`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in CreateInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	a, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/assets/"+a.ID.String())
	httpx.JSON(w, http.StatusCreated, a)
}
```

### POST /api/v1/assets/import

Buat / jalankan assets · import. Role dasar: **admin, analyst**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 106 pada snapshot commit.

Model request: `assets.ImportRequest` (lihat lampiran model).

Ekspresi status dalam handler: `Created`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in ImportRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+4096)
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	res, err := h.Svc.Import(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}
```

### POST /api/v1/assets/import/preview

Buat / jalankan assets · import · preview. Role dasar: **admin, analyst**.

Sumber: [internal/assets/http.go](../../internal/assets/http.go), baris 105 pada snapshot commit.

Model request: `assets.ImportRequest` (lihat lampiran model).

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) ImportPreview(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in ImportRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+4096)
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	pv, err := h.Svc.ImportPreview(r.Context(), p, in)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pv)
}
```

### POST /api/v1/assets/{id}/endpoint-data/refresh

Buat / jalankan assets · endpoint-data · refresh. Role dasar: **admin, analyst**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 548 pada snapshot commit.

Field JSON pada handler: `query_key`.

Ekspresi status dalam handler: `Accepted`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/assets/{id}/endpoint-data/refresh", auth.Require(authz.AtomicRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	asset, err := id(r, "id", "Asset")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in struct {
		Query string `json:"query_key"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	d, err := h.Svc.Refresh(r.Context(), p, asset, in.Query, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, http.StatusAccepted, d, err)
}))
```

### POST /api/v1/atomic-tests

Buat / jalankan atomic-tests. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 45 pada snapshot commit.

Model request: `execution.TestInput` (lihat lampiran model).

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/atomic-tests", auth.Require(authz.AtomicWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in TestInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.CreateTest(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/atomic-tests/{id}/run

Buat / jalankan atomic-tests · run. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 89 pada snapshot commit.

Ekspresi status dalam handler: `202`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/atomic-tests/{id}/run", auth.Require(authz.AtomicRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Atomic test")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Run(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 202, v, err)
}))
```

### POST /api/v1/auth/forgot

Buat / jalankan auth · forgot. Role dasar: **public**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 82 pada snapshot commit.

Field JSON pada handler: `email`.

Ekspresi status dalam handler: `Accepted`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/auth/forgot", func(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[struct {
		Email string `json:"email"`
	}](w, r)
	if !ok {
		return
	}
	if err := h.Svc.Forgot(r.Context(), in.Email); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
})
```

### POST /api/v1/auth/login

Login pengguna. Role dasar: **public**.

Sumber: [internal/auth/http.go](../../internal/auth/http.go), baris 139 pada snapshot commit.

Field JSON pada handler: `email`, `password`, `remember_me`.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email		string	`json:"email"`
		Password	string	`json:"password"`
		RememberMe	bool	`json:"remember_me"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	res, err := h.Svc.Login(r.Context(), h.Tenant, in.Email, in.Password, ClientIP(r), r.UserAgent(), in.RememberMe)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	h.setCookie(w, res.Token, res.Remember)
	httpx.JSON(w, http.StatusOK, h.toDTO(res.Principal))
}
```

### POST /api/v1/auth/logout

Logout pengguna. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/auth/http.go](../../internal/auth/http.go), baris 140 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/auth/logout", Require("", h.Logout))
```

### POST /api/v1/auth/reset

Buat / jalankan auth · reset. Role dasar: **public**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 108 pada snapshot commit.

Field JSON pada handler: `password`, `token`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/auth/reset", func(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[struct {
		Token		string	`json:"token"`
		Password	string	`json:"password"`
	}](w, r)
	if !ok {
		return
	}
	if err := h.Svc.UseLink(r.Context(), in.Token, in.Password, auth.ClientIP(r)); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
})
```

### POST /api/v1/auth/reset/check

Buat / jalankan auth · reset · check. Role dasar: **public**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 95 pada snapshot commit.

Field JSON pada handler: `token`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/auth/reset/check", func(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[struct {
		Token string `json:"token"`
	}](w, r)
	if !ok {
		return
	}
	if err := h.Svc.CheckLink(r.Context(), in.Token); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
})
```

### POST /api/v1/demo/purge

Buat / jalankan demo · purge. Role dasar: **admin**.

Sumber: [internal/demo/http.go](../../internal/demo/http.go), baris 24 pada snapshot commit.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/demo/purge", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	actor := p.UserID
	st, err := h.Svc.Purge(r.Context(), p.TenantID, &actor, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}))
```

### POST /api/v1/executions/{id}/cancel

Batalkan pekerjaan. Role dasar: **admin, analyst**.

Sumber: [internal/execution/http.go](../../internal/execution/http.go), baris 107 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/executions/{id}/cancel", auth.Require(authz.AtomicRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pathID(r, "Execution")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Cancel(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### POST /api/v1/injector/executions/{id}/claim

Klaim pekerjaan mesin. Role dasar: **machine**.

Sumber: [internal/execution/injector_http.go](../../internal/execution/injector_http.go), baris 52 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/claim", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Execution"))
		return
	}
	spec, err := h.RT.Claim(r.Context(), in, id)
	reply(w, r, 200, spec, err)
}))
```

### POST /api/v1/injector/executions/{id}/heartbeat

Kirim heartbeat mesin. Role dasar: **machine**.

Sumber: [internal/execution/injector_http.go](../../internal/execution/injector_http.go), baris 61 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Execution"))
		return
	}
	cancel, err := h.RT.Heartbeat(r.Context(), in, id)
	reply(w, r, 200, map[string]bool{"cancel": cancel}, err)
}))
```

### POST /api/v1/injector/executions/{id}/result

Kirim hasil eksekusi. Role dasar: **machine**.

Sumber: [internal/execution/injector_http.go](../../internal/execution/injector_http.go), baris 70 pada snapshot commit.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/result", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Execution"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	var res Result
	if err := decodeResult(r, &res); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.RT.Complete(context.WithoutCancel(r.Context()), in, id, res); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/injector/heartbeat

Kirim heartbeat mesin. Role dasar: **machine**.

Sumber: [internal/execution/injector_http.go](../../internal/execution/injector_http.go), baris 38 pada snapshot commit.

Field JSON pada handler: `version`.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+InjectorPrefix+"heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
	var body struct {
		Version string `json:"version"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.RT.Beat(r.Context(), in, body.Version); err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]string{"integration": in.Key})
}))
```

### POST /api/v1/integrations/{id}/token

Buat / jalankan integrations · token. Role dasar: **admin**.

Sumber: [internal/integrations/integrations.go](../../internal/integrations/integrations.go), baris 203 pada snapshot commit.

Ekspresi status dalam handler: `Created`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/integrations/{id}/token", auth.Require(authz.IntegrationManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Integration"))
		return
	}
	v, err := h.Svc.RotateToken(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}))
```

### POST /api/v1/me/password

Buat / jalankan me · password. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 137 pada snapshot commit.

Field JSON pada handler: `current_password`, `new_password`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/me/password", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	in, ok := decode[struct {
		Current	string	`json:"current_password"`
		New	string	`json:"new_password"`
	}](w, r)
	if !ok {
		return
	}
	if err := h.Svc.ChangePassword(r.Context(), p, in.Current, in.New, auth.ClientIP(r)); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### POST /api/v1/me/tokens

Buat / jalankan me · tokens. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 155 pada snapshot commit.

Field JSON pada handler: `expires_in_days`, `name`.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/me/tokens", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	in, ok := decode[struct {
		Name		string	`json:"name"`
		ExpiresInDays	int	`json:"expires_in_days"`
	}](w, r)
	if !ok {
		return
	}
	v, err := h.Svc.CreateToken(r.Context(), p, in.Name, in.ExpiresInDays, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/mitre/import

Buat / jalankan mitre · import. Role dasar: **admin**.

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 318 pada snapshot commit.

Ekspresi status dalam handler: `Accepted`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/mitre/import", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if h.Svc.Importer == nil {
		httpx.Write(w, r, httpx.Conflict("MITRE import is not configured."))
		return
	}
	last, err := h.Svc.lastRun(r.Context())
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	if last != nil && last.Status == "running" && time.Since(last.StartedAt) < staleAfter {
		httpx.Write(w, r, httpx.Conflict("An import is already running."))
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, err := h.Svc.Importer.Import(ctx); err != nil {
			slog.Warn("impor MITRE manual gagal", "err", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}))
```

### POST /api/v1/organizations

Buat / jalankan organizations. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 72 pada snapshot commit.

Model request: `organizations.Input` (lihat lampiran model).

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc(method+" "+path, auth.Require(authz.OrganizationWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Input
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	id := uuid.Nil
	if method == "PATCH" {
		var e error
		id, e = parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
	}
	v, e := h.Svc.Save(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	status := 200
	if method == "POST" {
		status = 201
	}
	reply(w, r, status, v, e)
}))
```

### POST /api/v1/organizations/{id}/ai/jobs

Buat / jalankan organizations · ai · jobs. Role dasar: **admin, analyst**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 101 pada snapshot commit.

Model request: `ai.Request` (lihat lampiran model).

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/organizations/{id}/ai/jobs", access("ai.generate", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	var in Request
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := s.Create(r.Context(), p, org, in, r.Header.Get("Idempotency-Key"))
	reply(w, r, v, e)
}))
```

### POST /api/v1/organizations/{id}/assessments

Buat / jalankan organizations · assessments. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 112 pada snapshot commit.

Model request: `organizations.AssessmentInput` (lihat lampiran model).

Ekspresi status dalam handler: `202`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/organizations/{id}/assessments", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	var in AssessmentInput
	if e = httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Launch(r.Context(), p, id, in, r.Header.Get("Idempotency-Key"), auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 202, v, e)
}))
```

### POST /api/v1/people

Buat / jalankan people. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 513 pada snapshot commit.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/people", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	in, ok := body[PersonInput](w, r)
	if !ok {
		return
	}
	a, b := ip(r)
	v, err := h.Svc.CreatePerson(r.Context(), p, in, a, b)
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/reports/{id}/retry

Coba ulang render PDF. Role dasar: **admin, analyst**.

Sumber: [internal/reporting/http.go](../../internal/reporting/http.go), baris 172 pada snapshot commit.

Ekspresi status dalam handler: `202`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/reports/{id}/retry", auth.Require(authz.ReportGenerate, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		httpx.Write(w, r, httpx.NotFound("Report"))
		return
	}
	e = h.Svc.DB.InTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		tag, e := tx.Exec(r.Context(), `UPDATE reports SET status='queued',attempts=0,render_generation=render_generation+1,error='',lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='failed'`, p.TenantID, id)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return httpx.Conflict("Only failed PDFs can be retried.")
		}
		return audit.Write(r.Context(), tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "report.retry", ResourceType: "report", ResourceID: &id, IP: auth.ClientIP(r), RequestID: httpx.RequestIDFrom(r.Context())})
	})
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 202, map[string]string{"status": "queued"})
})))
```

### POST /api/v1/scan-jobs/{id}/asset

Buat / jalankan scan-jobs · asset. Role dasar: **admin, analyst**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 69 pada snapshot commit.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/scan-jobs/{id}/asset", auth.Require(authz.AssetWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := pathID(r, "id")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.AddDiscoveredAsset(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, map[string]uuid.UUID{"id": v}, e)
}))
```

### POST /api/v1/scans

Buat / jalankan scans. Role dasar: **admin, analyst**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 28 pada snapshot commit.

Model request: `scans.Input` (lihat lampiran model).

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/scans", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, e := h.Svc.Launch(r.Context(), p, in, r.Header.Get("Idempotency-Key"), auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, e)
}))
```

### POST /api/v1/scans/{id}/cancel

Batalkan pekerjaan. Role dasar: **admin, analyst**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 60 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/scans/{id}/cancel", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := pathID(r, "id")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	e = h.Svc.Cancel(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, map[string]bool{"canceled": e == nil}, e)
}))
```

### POST /api/v1/scenarios

Buat / jalankan scenarios. Role dasar: **admin, analyst**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 36 pada snapshot commit.

Model request: `scenario.Input` (lihat lampiran model).

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/scenarios", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/scenarios/{id}/launch

Buat / jalankan scenarios · launch. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 35 pada snapshot commit.

Model request: `simulation.LaunchInput` (lihat lampiran model).

Ekspresi status dalam handler: `202`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/scenarios/{id}/launch", auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Scenario")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in LaunchInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Launch(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 202, v, err)
}))
```

### POST /api/v1/simulations/{id}/cancel

Batalkan pekerjaan. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 93 pada snapshot commit.

Model request: `simulation.CancelInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/simulations/{id}/cancel", auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Simulation")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in CancelInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.Cancel(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### POST /api/v1/simulations/{id}/dry-run

Buat / jalankan simulations · dry-run. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/advanced.go](../../internal/simulation/advanced.go), baris 87 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("POST /api/v1/simulations/{id}/dry-run", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if !authz.Has(p.Permissions, authz.FindingRead) || !authz.Has(p.Permissions, authz.AssetRead) {
		httpx.Write(w, r, httpx.Forbidden(authz.FindingRead))
		return
	}
	id, err := pid(r, "Simulation")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	items := []map[string]any{}
	err = h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM simulations WHERE tenant_id=$1 AND id=$2)`, p.TenantID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound("Simulation")
		}
		g, err := loadGraph(r.Context(), tx, p.TenantID, id)
		if err != nil {
			return err
		}
		targets, err := loadTargets(r.Context(), tx, p.TenantID, id)
		if err != nil {
			return err
		}
		execs, found, err := loadState(r.Context(), tx, p.TenantID, id)
		if err != nil {
			return err
		}
		recorded := map[string]string{}
		dr, err := tx.Query(r.Context(), `SELECT d.asset_id::text,s.key,d.reason FROM simulation_decisions d JOIN simulation_steps s ON s.tenant_id=d.tenant_id AND s.id=d.step_id WHERE d.tenant_id=$1 AND d.simulation_id=$2`, p.TenantID, id)
		if err != nil {
			return err
		}
		for dr.Next() {
			var asset, key, reason string
			if err = dr.Scan(&asset, &key, &reason); err != nil {
				dr.Close()
				return err
			}
			recorded[asset+"|"+key] = reason
		}
		dr.Close()
		if err = dr.Err(); err != nil {
			return err
		}
		for _, t := range targets {
			xs, fs := scoped(execs, found, t.ID.String())
			res := chain.Evaluate(g.chainSteps(), g.edges, xs, fs)
			for _, st := range g.steps {
				row := map[string]any{"asset_id": t.ID, "step": st.Key, "status": res.Status[st.Key], "reason": res.Reasons[st.Key], "recorded_reason": recorded[t.ID.String()+"|"+st.Key]}
				inputs, e := mappedInputs(st, t, g.edges, fs)
				if e != nil {
					row["mapping_error"] = e.Error()
				} else {
					preview := []map[string]any{}
					for _, in := range inputs {
						addr, e := mappedTarget(h.Svc.Engine, in.step, in.target)
						if e != nil {
							row["mapping_error"] = e.Error()
							break
						}
						preview = append(preview, map[string]any{"params": json.RawMessage(in.step.Params), "target": addr})
					}
					row["inputs"] = preview
				}
				items = append(items, row)
			}
		}
		return nil
	})
	reply(w, r, 200, map[string]any{"items": items, "executed": false}, err)
}))
```

### POST /api/v1/simulations/{id}/pause

Buat / jalankan simulations · pause. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 77 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/simulations/{id}/"+name, auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Simulation")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := f(r, p, id)
	reply(w, r, 200, v, err)
}))
```

### POST /api/v1/simulations/{id}/resume

Buat / jalankan simulations · resume. Role dasar: **admin, analyst**.

Sumber: [internal/simulation/http.go](../../internal/simulation/http.go), baris 77 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/simulations/{id}/"+name, auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Simulation")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := f(r, p, id)
	reply(w, r, 200, v, err)
}))
```

### POST /api/v1/tags

Buat / jalankan tags. Role dasar: **admin, analyst**.

Sumber: [internal/tags/tags.go](../../internal/tags/tags.go), baris 179 pada snapshot commit.

Field JSON pada handler: `color`, `name`.

Ekspresi status dalam handler: `Created`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		Name	string	`json:"name"`
		Color	string	`json:"color"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	t, err := h.Svc.Create(r.Context(), p, in.Name, in.Color, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}
```

### POST /api/v1/teams

Buat / jalankan teams. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 562 pada snapshot commit.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/teams", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	in, ok := body[TeamInput](w, r)
	if !ok {
		return
	}
	a, b := ip(r)
	v, err := h.Svc.CreateTeam(r.Context(), p, in, a, b)
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/users

Buat / jalankan users. Role dasar: **admin**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 50 pada snapshot commit.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/users", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	in, ok := decode[CreateInput](w, r)
	if !ok {
		return
	}
	v, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, err)
}))
```

### POST /api/v1/users/{id}/reset-link

Buat / jalankan users · reset-link. Role dasar: **admin**.

Sumber: [internal/users/http.go](../../internal/users/http.go), baris 71 pada snapshot commit.

Ekspresi status dalam handler: `201`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/users/{id}/reset-link", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uid(r, "User")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.CreateResetLink(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 201, v, err)
}))
```

### PUT /api/v1/asset-groups/{id}

Atur asset-groups. Role dasar: **admin, analyst**.

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 469 pada snapshot commit.

Model request: `groups.Input` (lihat lampiran model).

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	d, err := h.Svc.Update(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}
```

### PUT /api/v1/organizations/{id}/ai

Atur organizations · ai. Role dasar: **admin**.

Sumber: [internal/ai/http.go](../../internal/ai/http.go), baris 63 pada snapshot commit.

Model request: `ai.PolicyInput` (lihat lampiran model).

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/organizations/{id}/ai", access("ai.manage", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	var in PolicyInput
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := s.SavePolicy(r.Context(), p, org, in)
	reply(w, r, v, e)
}))
```

### PUT /api/v1/organizations/{id}/assets

Atur organizations · assets. Role dasar: **admin, analyst**.

Sumber: [internal/organizations/http.go](../../internal/organizations/http.go), baris 95 pada snapshot commit.

Model request: `organizations.MembershipInput` (lihat lampiran model).

Ekspresi status dalam handler: `204`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("PUT /api/v1/organizations/{id}/assets", auth.Require(authz.OrganizationWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := parseID(r)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	var in MembershipInput
	if e = httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	if e = h.Svc.Members(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); e != nil {
		httpx.Write(w, r, e)
		return
	}
	w.WriteHeader(204)
}))
```

### PUT /api/v1/organizations/{id}/endpoint-collection

Atur organizations · endpoint-collection. Role dasar: **admin**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 431 pada snapshot commit.

Model request: `endpoint.CollectionInput` (lihat lampiran model).

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/organizations/{id}/endpoint-collection", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, e := id(r, "id", "Organisation")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	var in CollectionInput
	if e = httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	e = h.Svc.SetCollection(r.Context(), p, org, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### PUT /api/v1/organizations/{id}/osquery-packs/{pack}

Atur organizations · osquery-packs · {pack}. Role dasar: **admin**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 492 pada snapshot commit.

Field JSON pada handler: `consent`, `enabled`, `versions`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/organizations/{id}/osquery-packs/{pack}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, err := id(r, "id", "Organization")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in struct {
		Versions	map[string]int64	`json:"versions"`
		Enabled		*bool			`json:"enabled"`
		Consent		osquery.ConsentRef	`json:"consent"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if in.Enabled == nil {
		httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "enabled", Message: "Choose on or off."}))
		return
	}
	if err := h.Svc.SetOrgPack(r.Context(), p, org, r.PathValue("pack"), *in.Enabled, in.Versions, in.Consent, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### PUT /api/v1/organizations/{id}/osquery/{key}

Atur organizations · osquery · {key}. Role dasar: **admin**.

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 466 pada snapshot commit.

Field JSON pada handler: `consent`, `enabled`, `version`.

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/organizations/{id}/osquery/{key}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, err := id(r, "id", "Organization")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in struct {
		Version	*int64			`json:"version"`
		Enabled	*bool			`json:"enabled"`
		Consent	osquery.ConsentRef	`json:"consent"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if in.Enabled == nil {
		httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "enabled", Message: "Choose on or off."}))
		return
	}
	err = h.Svc.SetOrg(r.Context(), p, org, r.PathValue("key"), *in.Enabled, in.Version, in.Consent, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### PUT /api/v1/organizations/{org}/agents/{agent}/networks

Atur organizations · {org} · agents · {agent} · networks. Role dasar: **admin**.

Sumber: [internal/scans/http.go](../../internal/scans/http.go), baris 79 pada snapshot commit.

Model request: `scans.ApproveInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc(method+" /api/v1/organizations/{org}/agents/{agent}/networks", auth.Require(authz.AgentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	org, e := pathID(r, "org")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	agent, e := pathID(r, "agent")
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	if r.Method == "GET" {
		v, e := h.Svc.Networks(r.Context(), p, org, agent)
		reply(w, r, 200, v, e)
		return
	}
	var in ApproveInput
	if e = httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.AuthorizeRange(r.Context(), p, org, agent, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, e)
}))
```

### PUT /api/v1/scenarios/{id}/graph

Atur scenarios · graph. Role dasar: **admin, analyst**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 94 pada snapshot commit.

Model request: `scenario.GraphInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/scenarios/{id}/graph", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	sid, err := id(r)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in GraphInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.SetGraph(r.Context(), p, sid, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### PUT /api/v1/scenarios/{id}/targets

Atur scenarios · targets. Role dasar: **admin, analyst**.

Sumber: [internal/scenario/http.go](../../internal/scenario/http.go), baris 80 pada snapshot commit.

Model request: `scenario.TargetsInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/scenarios/{id}/targets", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	sid, err := id(r)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	var in TargetsInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	v, err := h.Svc.SetTargets(r.Context(), p, sid, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, err)
}))
```

### PUT /api/v1/teams/{id}/members

Atur teams · members. Role dasar: **admin, analyst**.

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 607 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/teams/{id}/members", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := pid(r, "Team")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	in, ok := body[MembersInput](w, r)
	if !ok {
		return
	}
	a, b := ip(r)
	v, err := h.Svc.SetMembers(r.Context(), p, id, in, a, b)
	reply(w, r, 200, v, err)
}))
```

### GET /api/v1/scan-control

Baca scan-control. Role dasar: **admin**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 46 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### POST /api/v1/scan-control/stop

Buat / jalankan scan-control · stop. Role dasar: **admin**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 47 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### POST /api/v1/scan-control/resume

Buat / jalankan scan-control · resume. Role dasar: **admin**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 48 pada snapshot commit.

Model request: `scancontrol.Input` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### GET /api/v1/organizations/{id}/scan-control

Baca organizations · scan-control. Role dasar: **admin, analyst**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 49 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### PUT /api/v1/organizations/{id}/scan-control

Atur organizations · scan-control. Role dasar: **admin**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 50 pada snapshot commit.

Model request: `scancontrol.Input` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### POST /api/v1/organizations/{id}/scan-control/stop

Buat / jalankan organizations · scan-control · stop. Role dasar: **admin**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 51 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### POST /api/v1/organizations/{id}/scan-control/resume

Buat / jalankan organizations · scan-control · resume. Role dasar: **admin**.

Sumber: [internal/scancontrol/http.go](../../internal/scancontrol/http.go), baris 52 pada snapshot commit.

Model request: `scancontrol.Input` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
```

### GET /api/v1/retention

Baca retention. Role dasar: **admin**.

Sumber: [internal/retention/http.go](../../internal/retention/http.go), baris 29 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/retention", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, e := h.Svc.Get(r.Context(), p)
	respond(w, r, v, e)
}))
```

### PUT /api/v1/retention

Atur retention. Role dasar: **admin**.

Sumber: [internal/retention/http.go](../../internal/retention/http.go), baris 33 pada snapshot commit.

Model request: `retention.Policy` (lihat lampiran model).

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("PUT /api/v1/retention", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Policy
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Update(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	respond(w, r, v, e)
}))
```

### GET /api/v1/organizations/{id}/deletion-preview

Pratinjau penghapusan organisasi. Role dasar: **admin**.

Sumber: [internal/retention/http.go](../../internal/retention/http.go), baris 42 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/organizations/{id}/deletion-preview", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := objectID(w, r)
	if !ok {
		return
	}
	v, e := h.Svc.Preview(r.Context(), p, id)
	respond(w, r, v, e)
}))
```

### DELETE /api/v1/organizations/{id}

Hapus organizations. Role dasar: **admin**.

Sumber: [internal/retention/http.go](../../internal/retention/http.go), baris 50 pada snapshot commit.

Model request: `retention.DeleteInput` (lihat lampiran model).

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("DELETE /api/v1/organizations/{id}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := objectID(w, r)
	if !ok {
		return
	}
	var in DeleteInput
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.DeleteOrganization(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	respond(w, r, v, e)
}))
```

### GET /api/v1/data-deletions/{id}

Baca data-deletions. Role dasar: **admin**.

Sumber: [internal/retention/http.go](../../internal/retention/http.go), baris 63 pada snapshot commit.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/data-deletions/{id}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := objectID(w, r)
	if !ok {
		return
	}
	v, e := h.Svc.Deletion(r.Context(), p, id)
	respond(w, r, v, e)
}))
```

### GET /api/v1/agents/rollout

Baca agents · rollout. Role dasar: **admin**.

Sumber: [internal/agents/control_http.go](../../internal/agents/control_http.go), baris 13 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("GET /api/v1/agents/rollout", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	v, e := h.Svc.Rollout(r.Context(), p)
	reply(w, r, 200, v, e)
}))
```

### PUT /api/v1/agents/rollout

Atur agents · rollout. Role dasar: **admin**.

Sumber: [internal/agents/control_http.go](../../internal/agents/control_http.go), baris 17 pada snapshot commit.

Model request: `agents.RolloutPolicy` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("PUT /api/v1/agents/rollout", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in RolloutPolicy
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.SetRollout(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, e)
}))
```

### POST /api/v1/agents/{id}/control

Kirim kontrol agent. Role dasar: **admin**.

Sumber: [internal/agents/control_http.go](../../internal/agents/control_http.go), baris 26 pada snapshot commit.

Model request: `agents.ControlInput` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/agents/{id}/control", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		httpx.Write(w, r, httpx.NotFound("Agent"))
		return
	}
	var in ControlInput
	if e = httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	v, e := h.Svc.Control(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 200, v, e)
}))
```

### POST /api/v1/agents/{id}/collect

Minta koleksi endpoint. Role dasar: **admin**.

Sumber: [internal/agents/control_http.go](../../internal/agents/control_http.go), baris 40 pada snapshot commit.

Ekspresi status dalam handler: `202`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST /api/v1/agents/{id}/collect", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if e := adminControl(p); e != nil {
		httpx.Write(w, r, e)
		return
	}
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		httpx.Write(w, r, httpx.NotFound("Agent"))
		return
	}
	if h.Svc.CollectNow == nil {
		httpx.Write(w, r, httpx.Conflict("Endpoint collection is unavailable."))
		return
	}
	v, e := h.Svc.CollectNow(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	reply(w, r, 202, map[string]any{"execution_ids": v}, e)
}))
```

### POST /api/v1/agent/control/ack

Buat / jalankan agent · control · ack. Role dasar: **machine**.

Sumber: [internal/agents/control_http.go](../../internal/agents/control_http.go), baris 57 pada snapshot commit.

Model request: `agentproto.ControlAck` (lihat lampiran model).

Ekspresi status dalam handler: `NoContent`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
mux.HandleFunc("POST "+AgentPrefix+"control/ack", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
	var in agentproto.ControlAck
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	if e := h.Svc.Ack(r.Context(), p, in, auth.ClientIP(r)); e != nil {
		httpx.Write(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}))
```

### GET /api/v1/alerts/policy

Baca alerts · policy. Role dasar: **admin**.

Sumber: [internal/alerts/policy.go](../../internal/alerts/policy.go), baris 76 pada snapshot commit.

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("GET /api/v1/alerts/policy", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	out, e := h.Svc.GetPolicy(r.Context(), p)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, out)
}))
```

### PUT /api/v1/alerts/policy

Atur alerts · policy. Role dasar: **admin**.

Sumber: [internal/alerts/policy.go](../../internal/alerts/policy.go), baris 84 pada snapshot commit.

Model request: `alerts.Policy` (lihat lampiran model).

Ekspresi status dalam handler: `200`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
m.HandleFunc("PUT /api/v1/alerts/policy", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Policy
	if e := httpx.Decode(w, r, &in); e != nil {
		httpx.Write(w, r, e)
		return
	}
	out, e := h.Svc.SetPolicy(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, out)
}))
```

### PATCH /api/v1/auth/preferences

Preferensi bahasa akun. Role dasar: **admin, analyst, viewer**.

Sumber: [internal/auth/http.go](../../internal/auth/http.go), baris 142 pada snapshot commit.

Field JSON pada handler: `ui_language`.

Ekspresi status dalam handler: `OK`. Error memakai kontrak umum.

Implementasi handler berikut menunjukkan decoding request, pemanggilan service, dan pembentukan respons.

```go
func (h *Handlers) Preferences(w http.ResponseWriter, r *http.Request, p *Principal) {
	var in struct {
		Language string `json:"ui_language"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.Svc.SetLanguage(r.Context(), p, in.Language, ClientIP(r)); err != nil {
		httpx.Write(w, r, err)
		return
	}
	out := *p
	out.UILanguage = in.Language
	httpx.JSON(w, http.StatusOK, h.toDTO(&out))
}
```

## Lampiran model JSON

Definisi berikut diekstrak langsung dari deklarasi Go bertag JSON. Nama diawali package untuk membedakan `Input`/`Policy` yang berbeda. Field tanpa tag, embedded types, dan validasi service perlu dibaca pada sumber terkait; model ini bukan klaim skema OpenAPI yang telah divalidasi.

### agent.Config

Sumber: [internal/agent/agent.go](../../internal/agent/agent.go), baris 31.

```go
type Config struct {
	Name		string			`json:"name,omitempty"`
	ServerURL	string			`json:"server_url"`
	AgentID		string			`json:"agent_id"`
	Secret		string			`json:"secret"`
	Mode		string			`json:"mode"`
	Revoked		bool			`json:"revoked,omitempty"`
	Uninstall	*UninstallReceipt	`json:"uninstall,omitempty"`
	CAFile		string			`json:"ca_file,omitempty"`
}
```

### agent.UninstallReceipt

Sumber: [internal/agent/control.go](../../internal/agent/control.go), baris 16.

```go
type UninstallReceipt struct {
	ID	string	`json:"id"`
	Managed	bool	`json:"managed"`
}
```

### agent.heartbeatReply

Sumber: [internal/agent/agent.go](../../internal/agent/agent.go), baris 185.

```go
type heartbeatReply struct {
	LatestVersion	string				`json:"latest_version"`
	PollSeconds	int				`json:"poll_seconds"`
	Paused		bool				`json:"paused"`
	Command		*agentproto.ControlCommand	`json:"command,omitempty"`
}
```

### agentproto.ControlAck

Sumber: [internal/agentproto/control.go](../../internal/agentproto/control.go), baris 21.

```go
type ControlAck struct {
	ID	string	`json:"id"`
	Success	bool	`json:"success"`
	Code	string	`json:"code,omitempty"`
}
```

### agentproto.ControlCommand

Sumber: [internal/agentproto/control.go](../../internal/agentproto/control.go), baris 15.

```go
type ControlCommand struct {
	ID	string	`json:"id"`
	Kind	string	`json:"kind"`
	AgentID	string	`json:"agent_id"`
}
```

### agents.Agent

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 454.

```go
type Agent struct {
	OrganizationID		*uuid.UUID	`json:"organization_id"`
	InstanceName		*string		`json:"instance_name"`
	ID			uuid.UUID	`json:"id"`
	AssetID			uuid.UUID	`json:"asset_id"`
	AssetName		string		`json:"asset_name"`
	Hostname		*string		`json:"hostname"`
	OS			string		`json:"os"`
	Arch			string		`json:"arch"`
	Mode			string		`json:"mode"`
	Version			string		`json:"version"`
	Status			string		`json:"status"`
	LastHeartbeatAt		*time.Time	`json:"last_heartbeat_at"`
	EnrolledAt		time.Time	`json:"enrolled_at"`
	UpdateAvailable		bool		`json:"update_available"`
	ControlVersion		int		`json:"control_version"`
	Paused			bool		`json:"paused"`
	ControlProtocol		int		`json:"control_protocol"`
	OsqueryVersion		string		`json:"osquery_version"`
	DiagnosticCode		string		`json:"diagnostic_code"`
	DiagnosticAt		*time.Time	`json:"diagnostic_at"`
	RequestedUpdateVersion	string		`json:"requested_update_version"`
	UninstallStatus		string		`json:"uninstall_status"`
	UninstallRequestedAt	*time.Time	`json:"uninstall_requested_at"`
	UninstallCompletedAt	*time.Time	`json:"uninstall_completed_at"`
	UpdateStatus		string		`json:"update_status"`
}
```

### agents.ControlInput

Sumber: [internal/agents/control.go](../../internal/agents/control.go), baris 73.

```go
type ControlInput struct {
	Action		string	`json:"action"`
	Version		int	`json:"version"`
	Confirmation	string	`json:"confirmation,omitempty"`
}
```

### agents.EnrollInput

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 187.

```go
type EnrollInput struct {
	Name		string	`json:"name"`
	Token		string	`json:"token"`
	Hostname	string	`json:"hostname"`
	OS		string	`json:"os"`
	Arch		string	`json:"arch"`
	Version		string	`json:"version"`
	ProtocolVersion	int	`json:"protocol_version"`
}
```

### agents.Enrolled

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 197.

```go
type Enrolled struct {
	AgentID		uuid.UUID	`json:"agent_id"`
	Secret		string		`json:"secret"`
	Mode		string		`json:"mode"`
	PollSeconds	int		`json:"poll_seconds"`
}
```

### agents.EnrollmentToken

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 74.

```go
type EnrollmentToken struct {
	ID		uuid.UUID	`json:"id"`
	Token		string		`json:"token"`
	OS		string		`json:"os"`
	Arch		string		`json:"arch"`
	Mode		string		`json:"mode"`
	ExpiresAt	time.Time	`json:"expires_at"`
	Version		string		`json:"version"`
	SHA256		string		`json:"sha256"`
	FileName	string		`json:"file_name"`
	InstallTrust
}
```

### agents.HeartbeatInput

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 339.

```go
type HeartbeatInput struct {
	Version		string	`json:"version"`
	ProtocolVersion	int	`json:"protocol_version"`
	LastUpdateError	string	`json:"last_update_error"`
	ControlProtocol	int	`json:"control_protocol,omitempty"`
	OsqueryVersion	string	`json:"osquery_version,omitempty"`
	DiagnosticCode	string	`json:"diagnostic_code,omitempty"`

	OSVersion	string	`json:"os_version,omitempty"`
}
```

### agents.HeartbeatReply

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 361.

```go
type HeartbeatReply struct {
	LatestVersion	string				`json:"latest_version,omitempty"`
	PollSeconds	int				`json:"poll_seconds"`
	Paused		bool				`json:"paused"`
	Command		*agentproto.ControlCommand	`json:"command,omitempty"`
}
```

### agents.InstallTrust

Sumber: [internal/agents/installtrust.go](../../internal/agents/installtrust.go), baris 14.

```go
type InstallTrust struct {
	ReleaseKey	string	`json:"release_key,omitempty"`
	CAFingerprint	string	`json:"ca_fingerprint,omitempty"`
	TLSPin		string	`json:"tls_pin,omitempty"`
}
```

### agents.Manifest

Sumber: [internal/agents/release.go](../../internal/agents/release.go), baris 23.

```go
type Manifest struct {
	Version	string			`json:"version"`
	Targets	map[string]Target	`json:"targets"`
}
```

### agents.RolloutPolicy

Sumber: [internal/agents/control.go](../../internal/agents/control.go), baris 17.

```go
type RolloutPolicy struct {
	Mode	string	`json:"mode"`
	Version	int	`json:"version"`
}
```

### agents.Target

Sumber: [internal/agents/release.go](../../internal/agents/release.go), baris 17.

```go
type Target struct {
	File		string	`json:"file"`
	SHA256		string	`json:"sha256"`
	Signature	string	`json:"signature"`
}
```

### agents.TokenInput

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 87.

```go
type TokenInput struct {
	OrganizationID	*uuid.UUID	`json:"organization_id"`
	OS		string		`json:"os"`
	Arch		string		`json:"arch"`
	Mode		string		`json:"mode"`
}
```

### agents.TokenStatus

Sumber: [internal/agents/agents.go](../../internal/agents/agents.go), baris 151.

```go
type TokenStatus struct {
	Used	bool		`json:"used"`
	Expired	bool		`json:"expired"`
	AgentID	*uuid.UUID	`json:"agent_id"`
	Agent	*Agent		`json:"agent"`
}
```

### agents.ToolEntry

Sumber: [internal/agents/tools.go](../../internal/agents/tools.go), baris 24.

```go
type ToolEntry struct {
	Version	string			`json:"version"`
	Targets	map[string]Target	`json:"targets"`
}
```

### agents.ToolManifest

Sumber: [internal/agents/tools.go](../../internal/agents/tools.go), baris 20.

```go
type ToolManifest struct {
	Tools map[string]ToolEntry `json:"tools"`
}
```

### ai.Action

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 97.

```go
type Action struct {
	AssetID		string		`json:"asset_id"`
	ContractKey	string		`json:"contract_key"`
	Version		int		`json:"version"`
	Params		json.RawMessage	`json:"params"`
	Reason		string		`json:"reason"`
}
```

### ai.Advice

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 92.

```go
type Advice struct {
	FindingID	string	`json:"finding_id"`
	Recommendation	string	`json:"recommendation"`
	Validation	string	`json:"validation"`
}
```

### ai.Claim

Sumber: [internal/ai/worker.go](../../internal/ai/worker.go), baris 17.

```go
type Claim struct {
	Job		Job	`json:"job"`
	Token		string	`json:"token"`
	APIKey		string	`json:"api_key"`
	MaxOutputTokens	int	`json:"max_output_tokens"`
}
```

### ai.Completion

Sumber: [internal/ai/worker.go](../../internal/ai/worker.go), baris 118.

```go
type Completion struct {
	Token		string		`json:"token"`
	Output		json.RawMessage	`json:"output"`
	Failure		string		`json:"failure"`
	UsageTokens	int		`json:"usage_tokens"`
}
```

### ai.Input

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 35.

```go
type Input struct {
	Assets	[]Target	`json:"assets"`
	Catalog	[]contract.Def	`json:"catalog"`
}
```

### ai.Job

Sumber: [internal/ai/service.go](../../internal/ai/service.go), baris 204.

```go
type Job struct {
	IsDemo			bool		`json:"is_demo"`
	ID			uuid.UUID	`json:"id"`
	OrganizationID		uuid.UUID	`json:"organization_id"`
	ReportID		uuid.UUID	`json:"report_id"`
	Kind			string		`json:"kind"`
	Provider		string		`json:"provider"`
	Model			string		`json:"model"`
	Status			string		`json:"status"`
	Version			int		`json:"version"`
	PolicyVersion		int		`json:"policy_version"`
	PromptVersion		int		`json:"prompt_version"`
	Instructions		string		`json:"instructions"`
	Input			Input		`json:"input"`
	Output			*Output		`json:"output"`
	Error			string		`json:"error"`
	ReservedTokens		int		`json:"reserved_tokens"`
	ReservedCostMicros	int64		`json:"reserved_cost_micros"`
	CreatedAt		time.Time	`json:"created_at"`
	InputBytes		int		`json:"input_bytes"`
	References		Mapping		`json:"references"`
}
```

### ai.Mapping

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 39.

```go
type Mapping struct {
	Assets		map[string]string	`json:"assets"`
	Findings	map[string]string	`json:"findings"`
	AssetLabels	map[string]string	`json:"asset_labels"`
	FindingLabels	map[string]string	`json:"finding_labels"`
	FindingTargets	map[string]string	`json:"finding_targets"`
}
```

### ai.Observation

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 25.

```go
type Observation struct {
	ID		string		`json:"id"`
	Type		string		`json:"type"`
	Severity	string		`json:"severity"`
	CVEs		[]string	`json:"cves"`
}
```

### ai.Output

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 104.

```go
type Output struct {
	Summary		string		`json:"summary"`
	Remediations	[]Advice	`json:"remediations"`
	Actions		[]Action	`json:"actions"`
}
```

### ai.Policy

Sumber: [internal/ai/service.go](../../internal/ai/service.go), baris 33.

```go
type Policy struct {
	IsDemo			bool	`json:"is_demo"`
	Version			int	`json:"version"`
	Enabled			bool	`json:"enabled"`
	Planning		bool	`json:"planning"`
	Remediation		bool	`json:"remediation"`
	PlanningModel		string	`json:"planning_model"`
	RemediationModel	string	`json:"remediation_model"`
	MaxInputBytes		int	`json:"max_input_bytes"`
	MaxOutputTokens		int	`json:"max_output_tokens"`
	DailyTokens		int	`json:"daily_tokens"`
	DailyCalls		int	`json:"daily_calls"`
	DailyCostMicros		int64	`json:"daily_cost_micros"`
	InputRateMicros		int64	`json:"input_rate_micros"`
	OutputRateMicros	int64	`json:"output_rate_micros"`
	HasKeys			bool	`json:"has_keys"`
	Configured		bool	`json:"configured"`
}
```

### ai.PolicyInput

Sumber: [internal/ai/service.go](../../internal/ai/service.go), baris 51.

```go
type PolicyInput struct {
	Policy
	DeepSeekKey	string	`json:"deepseek_key"`
	OpenAIKey	string	`json:"openai_key"`
	Consent		bool	`json:"consent"`
}
```

### ai.Request

Sumber: [internal/ai/service.go](../../internal/ai/service.go), baris 294.

```go
type Request struct {
	ReportID	uuid.UUID	`json:"report_id"`
	Kind		string		`json:"kind"`
	TemplateID	*uuid.UUID	`json:"template_id"`
}
```

### ai.Target

Sumber: [internal/ai/contract.go](../../internal/ai/contract.go), baris 31.

```go
type Target struct {
	ID		string		`json:"id"`
	Findings	[]Observation	`json:"findings"`
}
```

### ai.keys

Sumber: [internal/ai/service.go](../../internal/ai/service.go), baris 57.

```go
type keys struct {
	DeepSeek	string	`json:"deepseek"`
	OpenAI		string	`json:"openai"`
}
```

### alerts.Alert

Sumber: [internal/alerts/alerts.go](../../internal/alerts/alerts.go), baris 73.

```go
type Alert struct {
	ID		uuid.UUID	`json:"id"`
	Kind		string		`json:"kind"`
	Severity	string		`json:"severity"`
	Title		string		`json:"title"`
	Body		*string		`json:"body"`
	ResourceType	*string		`json:"resource_type"`
	ResourceID	*uuid.UUID	`json:"resource_id"`
	OrganizationID	*uuid.UUID	`json:"organization_id"`
	ResourceLabel	*string		`json:"resource_label"`
	CreatedAt	time.Time	`json:"created_at"`
	Read		bool		`json:"read"`
}
```

### alerts.List

Sumber: [internal/alerts/alerts.go](../../internal/alerts/alerts.go), baris 87.

```go
type List struct {
	Items	[]Alert	`json:"items"`
	Unread	int	`json:"unread"`
}
```

### alerts.Policy

Sumber: [internal/alerts/policy.go](../../internal/alerts/policy.go), baris 16.

```go
type Policy struct {
	OfflineMinutes	int	`json:"offline_minutes"`
	Version		int	`json:"version"`
}
```

### assets.Address

Sumber: [internal/assets/model.go](../../internal/assets/model.go), baris 34.

```go
type Address struct {
	ScanApproved	bool	`json:"scan_approved"`
	Kind		string	`json:"kind"`
	Value		string	`json:"value"`
	IsPrimary	bool	`json:"is_primary"`
}
```

### assets.Asset

Sumber: [internal/assets/model.go](../../internal/assets/model.go), baris 68.

```go
type Asset struct {
	OrganizationID		*uuid.UUID	`json:"organization_id"`
	OrganizationName	*string		`json:"organization_name"`
	ID			uuid.UUID	`json:"id"`
	Category		string		`json:"category"`
	Name			string		`json:"name"`
	Description		*string		`json:"description"`
	Platform		*string		`json:"platform"`
	Arch			*string		`json:"arch"`
	Criticality		string		`json:"criticality"`
	IsInternetFacing	bool		`json:"is_internet_facing"`
	TestingAuthorized	bool		`json:"testing_authorized"`
	Source			string		`json:"source"`
	AgentState		string		`json:"agent_state"`
	Attributes		json.RawMessage	`json:"attributes"`
	Addresses		[]Address	`json:"addresses"`
	Tags			[]TagRef	`json:"tags"`
	Groups			[]GroupRef	`json:"groups,omitempty"`
	CreatedAt		time.Time	`json:"created_at"`
	UpdatedAt		time.Time	`json:"updated_at"`
	Version			int		`json:"version"`
}
```

### assets.Column

Sumber: [internal/assets/importer.go](../../internal/assets/importer.go), baris 40.

```go
type Column struct {
	Name	string		`json:"name"`
	Sample	[]string	`json:"sample"`
}
```

### assets.CreateInput

Sumber: [internal/assets/model.go](../../internal/assets/model.go), baris 53.

```go
type CreateInput struct {
	OrganizationID		*uuid.UUID	`json:"organization_id"`
	Category		string		`json:"category"`
	Name			string		`json:"name"`
	Description		*string		`json:"description"`
	Platform		*string		`json:"platform"`
	Arch			*string		`json:"arch"`
	Criticality		string		`json:"criticality"`
	IsInternetFacing	bool		`json:"is_internet_facing"`
	TestingAuthorized	bool		`json:"testing_authorized"`
	Attributes		json.RawMessage	`json:"attributes"`
	Addresses		[]Address	`json:"addresses"`
	TagIDs			[]uuid.UUID	`json:"tag_ids"`
}
```

### assets.GroupRef

Sumber: [internal/assets/model.go](../../internal/assets/model.go), baris 47.

```go
type GroupRef struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	IsSystem	bool		`json:"is_system"`
}
```

### assets.ImportPreview

Sumber: [internal/assets/importer.go](../../internal/assets/importer.go), baris 50.

```go
type ImportPreview struct {
	Columns			[]Column		`json:"columns"`
	SuggestedMapping	map[string]string	`json:"suggested_mapping"`
	Rows			int			`json:"rows"`
	Validated		bool			`json:"validated"`
	Valid			int			`json:"valid"`
	Invalid			[]RowIssue		`json:"invalid"`
	Duplicates		[]RowIssue		`json:"duplicates"`
}
```

### assets.ImportRequest

Sumber: [internal/assets/importer.go](../../internal/assets/importer.go), baris 34.

```go
type ImportRequest struct {
	CSV		string			`json:"csv"`
	Mapping		map[string]string	`json:"mapping"`
	SkipInvalid	bool			`json:"skip_invalid"`
}
```

### assets.ImportResult

Sumber: [internal/assets/importer.go](../../internal/assets/importer.go), baris 410.

```go
type ImportResult struct {
	Created		int	`json:"created"`
	Skipped		int	`json:"skipped_invalid"`
	Duplicates	int	`json:"skipped_duplicates"`
	TagsMade	int	`json:"tags_created"`
}
```

### assets.Page

Sumber: [internal/assets/service.go](../../internal/assets/service.go), baris 43.

```go
type Page struct {
	Items		[]Asset	`json:"items"`
	NextCursor	*string	`json:"next_cursor"`
}
```

### assets.PatchInput

Sumber: [internal/assets/model.go](../../internal/assets/model.go), baris 135.

```go
type PatchInput struct {
	Version			*int		`json:"version"`
	Name			*string		`json:"name"`
	Description		*string		`json:"description"`
	Platform		*string		`json:"platform"`
	Arch			*string		`json:"arch"`
	Criticality		*string		`json:"criticality"`
	IsInternetFacing	*bool		`json:"is_internet_facing"`
	TestingAuthorized	*bool		`json:"testing_authorized"`
	Attributes		json.RawMessage	`json:"attributes"`
	Addresses		*[]Address	`json:"addresses"`
	TagIDs			*[]uuid.UUID	`json:"tag_ids"`
	ClearPlatform		bool		`json:"clear_platform"`
	ClearArch		bool		`json:"clear_arch"`
}
```

### assets.RowIssue

Sumber: [internal/assets/importer.go](../../internal/assets/importer.go), baris 45.

```go
type RowIssue struct {
	Row	int	`json:"row"`
	Message	string	`json:"message"`
}
```

### assets.TagRef

Sumber: [internal/assets/model.go](../../internal/assets/model.go), baris 41.

```go
type TagRef struct {
	ID	uuid.UUID	`json:"id"`
	Name	string		`json:"name"`
	Color	string		`json:"color"`
}
```

### assets.cursor

Sumber: [internal/assets/service.go](../../internal/assets/service.go), baris 48.

```go
type cursor struct {
	U	time.Time	`json:"u"`
	I	uuid.UUID	`json:"i"`
}
```

### auditview.Entry

Sumber: [internal/auditview/http.go](../../internal/auditview/http.go), baris 20.

```go
type Entry struct {
	ID		uuid.UUID	`json:"id"`
	At		time.Time	`json:"at"`
	ActorUser	*uuid.UUID	`json:"actor_user_id"`
	Actor		string		`json:"actor"`
	Action		string		`json:"action"`
	ResourceType	string		`json:"resource_type"`
	ResourceID	*uuid.UUID	`json:"resource_id"`
	IP		string		`json:"ip"`
	RequestID	string		`json:"request_id"`
	Reason		string		`json:"reason"`
}
```

### auditview.Page

Sumber: [internal/auditview/http.go](../../internal/auditview/http.go), baris 32.

```go
type Page struct {
	Items		[]Entry	`json:"items"`
	NextCursor	*string	`json:"next_cursor"`
}
```

### auditview.cursor

Sumber: [internal/auditview/http.go](../../internal/auditview/http.go), baris 36.

```go
type cursor struct {
	At	time.Time	`json:"at"`
	ID	uuid.UUID	`json:"id"`
}
```

### auth.userDTO

Sumber: [internal/auth/http.go](../../internal/auth/http.go), baris 84.

```go
type userDTO struct {
	ID		string		`json:"id"`
	Email		string		`json:"email"`
	DisplayName	string		`json:"display_name"`
	Role		string		`json:"role"`
	Permissions	[]string	`json:"permissions"`
	UILanguage	string		`json:"ui_language"`
	Features	map[string]bool	`json:"features"`
}
```

### chain.Condition

Sumber: [internal/chain/conditions.go](../../internal/chain/conditions.go), baris 13.

```go
type Condition struct {
	Op	string		`json:"op"`
	Field	string		`json:"field,omitempty"`
	Value	any		`json:"value,omitempty"`
	Args	[]Condition	`json:"args,omitempty"`
}
```

### chain.Edge

Sumber: [internal/chain/chain.go](../../internal/chain/chain.go), baris 40.

```go
type Edge struct {
	Condition	*Condition	`json:"condition,omitempty"`
	Branch		string		`json:"branch,omitempty"`
	Mapper		*Mapper		`json:"mapper,omitempty"`
	Parent		string		`json:"parent"`
	Child		string		`json:"child"`
}
```

### chain.Mapper

Sumber: [internal/chain/conditions.go](../../internal/chain/conditions.go), baris 38.

```go
type Mapper struct {
	Source		string	`json:"source"`
	Destination	string	`json:"destination"`
}
```

### chain.Result

Sumber: [internal/chain/chain.go](../../internal/chain/chain.go), baris 238.

```go
type Result struct {
	Reasons		map[string]string	`json:"reasons,omitempty"`
	Status		map[string]string
	Launch		[]Launch
	Finished	bool
}
```

### chain.Trigger

Sumber: [internal/chain/chain.go](../../internal/chain/chain.go), baris 25.

```go
type Trigger struct {
	Type		string		`json:"type"`
	Values		[]string	`json:"values,omitempty"`
	MinSeverity	string		`json:"min_severity,omitempty"`
}
```

### contract.Address

Sumber: [internal/contract/target.go](../../internal/contract/target.go), baris 14.

```go
type Address struct {
	Kind		string	`json:"kind"`
	Value		string	`json:"value"`
	IsPrimary	bool	`json:"is_primary"`
}
```

### contract.Def

Sumber: [internal/contract/contract.go](../../internal/contract/contract.go), baris 33.

```go
type Def struct {
	Adapter		string		`json:"adapter"`
	Category	string		`json:"category"`
	Schema		Schema		`json:"input_schema"`
	Key		string		`json:"key"`
	Integration	string		`json:"integration"`
	Name		string		`json:"name"`
	Description	string		`json:"description"`
	Kind		string		`json:"kind"`
	OutputTypes	[]string	`json:"output_types"`
	TimeoutSeconds	int		`json:"timeout_seconds"`
	Version		int		`json:"version"`
	NeedsAgent	bool		`json:"needs_agent"`
}
```

### contract.NmapParams

Sumber: [internal/contract/contract.go](../../internal/contract/contract.go), baris 88.

```go
type NmapParams struct {
	Profile			string	`json:"profile"`
	Ports			string	`json:"ports,omitempty"`
	ServiceDetection	bool	`json:"service_detection"`
}
```

### contract.NucleiParams

Sumber: [internal/contract/contract.go](../../internal/contract/contract.go), baris 94.

```go
type NucleiParams struct {
	TemplateProfile	string		`json:"template_profile,omitempty"`
	Severity	[]string	`json:"severity"`
	Tags		[]string	`json:"tags,omitempty"`
}
```

### contract.Property

Sumber: [internal/contract/schema.go](../../internal/contract/schema.go), baris 13.

```go
type Property struct {
	ErrorMessage	string		`json:"error_message,omitempty"`
	Type		string		`json:"type"`
	Label		string		`json:"label"`
	Help		string		`json:"help,omitempty"`
	Enum		[]string	`json:"enum,omitempty"`
	EnumLabels	[]string	`json:"enum_labels,omitempty"`
	Pattern		string		`json:"pattern,omitempty"`
	ItemsEnum	[]string	`json:"items_enum,omitempty"`
	ItemsPattern	string		`json:"items_pattern,omitempty"`
	MinItems	int		`json:"min_items,omitempty"`
	MaxItems	int		`json:"max_items,omitempty"`
	MaxLength	int		`json:"max_length,omitempty"`
	Min		*float64	`json:"minimum,omitempty"`
	Max		*float64	`json:"maximum,omitempty"`
	Default		any		`json:"default,omitempty"`
	ShowWhen	*struct {
		Field	string	`json:"field"`
		Value	string	`json:"value"`
	}	`json:"show_when,omitempty"`
}
```

### contract.ScanScope

Sumber: [internal/contract/scan_scope.go](../../internal/contract/scan_scope.go), baris 12.

```go
type ScanScope struct {
	Target			string	`json:"target"`
	CIDR			string	`json:"cidr,omitempty"`
	TestingAuthorized	bool	`json:"testing_authorized"`
	InternetFacing		bool	`json:"internet_facing"`
	MaxHosts		int	`json:"max_hosts"`
}
```

### contract.Schema

Sumber: [internal/contract/schema.go](../../internal/contract/schema.go), baris 34.

```go
type Schema struct {
	Type			string			`json:"type"`
	AdditionalProperties	bool			`json:"additionalProperties"`
	Properties		map[string]Property	`json:"properties"`
	Order			[]string		`json:"order"`
}
```

### demo.Status

Sumber: [internal/demo/demo.go](../../internal/demo/demo.go), baris 49.

```go
type Status struct {
	FormatVersion	int		`json:"format_version,omitempty"`
	HasDemo		bool		`json:"has_demo"`
	CleanupPending	int		`json:"cleanup_pending"`
	Counts		map[string]int	`json:"counts"`
	Seed		*int		`json:"seed,omitempty"`
}
```

### endpoint.AssetData

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 291.

```go
type AssetData struct {
	Platform	string		`json:"platform"`
	Arch		string		`json:"arch"`
	Supported	bool		`json:"supported"`
	Version		int		`json:"version"`
	ReadOnly	bool		`json:"read_only"`
	AssetID		uuid.UUID	`json:"asset_id"`
	OrganizationID	*uuid.UUID	`json:"organization_id"`
	HasAgent	bool		`json:"has_agent"`
	Items		[]DatasetView	`json:"items"`
	Findings	[]FindingView	`json:"findings"`
}
```

### endpoint.Catalog

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 83.

```go
type Catalog struct {
	CollectionIntervalSeconds	int			`json:"collection_interval_seconds"`
	Consent				osquery.ConsentDocument	`json:"consent"`
	OsqueryVersion			string			`json:"osquery_version"`
	License				string			`json:"license"`
	ProvenOS			[]string		`json:"proven_os"`
	SupportedPlatforms		[]string		`json:"supported_platforms"`
	Queries				[]QueryMeta		`json:"queries"`
	Packs				[]PackView		`json:"packs"`
}
```

### endpoint.CollectionInput

Sumber: [internal/endpoint/collection.go](../../internal/endpoint/collection.go), baris 21.

```go
type CollectionInput struct {
	Platform	string			`json:"platform"`
	Keys		[]string		`json:"keys"`
	Versions	map[string]int64	`json:"versions"`
	Consent		osquery.ConsentRef	`json:"consent"`
}
```

### endpoint.DatasetView

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 266.

```go
type DatasetView struct {
	Supported	bool			`json:"supported"`
	Key		string			`json:"key"`
	Title		string			`json:"title"`
	Status		string			`json:"status"`
	Reason		string			`json:"reason"`
	CollectedAt	*time.Time		`json:"collected_at"`
	LastAttemptAt	*time.Time		`json:"last_attempt_at"`
	Version		string			`json:"osquery_version"`
	Attributes	map[string]any		`json:"attributes"`
	Rows		[]map[string]string	`json:"rows"`
	Truncated	bool			`json:"truncated"`
	Enabled		bool			`json:"enabled"`
	Synthetic	bool			`json:"synthetic"`
}
```

### endpoint.ExampleFinding

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 65.

```go
type ExampleFinding struct {
	Rule		string	`json:"rule"`
	Type		string	`json:"type"`
	Severity	string	`json:"severity"`
	Title		string	`json:"title"`
}
```

### endpoint.FindingView

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 282.

```go
type FindingView struct {
	ID		uuid.UUID	`json:"id"`
	Type		string		`json:"type"`
	Severity	string		`json:"severity"`
	Title		string		`json:"title"`
	Value		string		`json:"value"`
	Query		string		`json:"query"`
}
```

### endpoint.OrgSetting

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 156.

```go
type OrgSetting struct {
	ConsentVersion	*string		`json:"consent_version"`
	ConsentSHA256	*string		`json:"consent_sha256"`
	ConsentedBy	*uuid.UUID	`json:"consented_by"`
	ConsentedAt	*time.Time	`json:"consented_at"`
	Key		string		`json:"key"`
	Version		int64		`json:"version"`
	Enabled		bool		`json:"enabled"`
	Explicit	bool		`json:"explicit"`
	UpdatedAt	*time.Time	`json:"updated_at"`
}
```

### endpoint.PackView

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 73.

```go
type PackView struct {
	Name		string			`json:"name"`
	Description	string			`json:"description"`
	File		string			`json:"file"`
	SHA256		string			`json:"sha256"`
	Platform	string			`json:"platform"`
	TablesRead	[]string		`json:"tables_read"`
	Queries		[]osquery.PackQuery	`json:"queries"`
}
```

### endpoint.QueryMeta

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 39.

```go
type QueryMeta struct {
	Key			string			`json:"key"`
	Platform		string			`json:"platform"`
	Pack			string			`json:"pack,omitempty"`
	Title			string			`json:"title"`
	Description		string			`json:"description"`
	Category		string			`json:"category"`
	Tables			[]string		`json:"tables"`
	Columns			[]string		`json:"columns"`
	OS			[]string		`json:"os"`
	Sensitivity		string			`json:"sensitivity"`
	Load			string			`json:"load"`
	NeedsRoot		bool			`json:"needs_root"`
	DefaultEnabled		bool			`json:"default_enabled"`
	TimeoutSeconds		int			`json:"timeout_seconds"`
	MaxRows			int			`json:"max_rows"`
	Mitre			[]osquery.MitreRef	`json:"mitre"`
	Attributes		[]string		`json:"attributes"`
	Findings		[]RuleMeta		`json:"findings"`
	Report			string			`json:"report_section"`
	Example			[]map[string]string	`json:"example"`
	ExampleFindings		[]ExampleFinding	`json:"example_findings"`
	ExampleAttributes	map[string]any		`json:"example_attributes"`
}
```

### endpoint.RuleMeta

Sumber: [internal/endpoint/endpoint.go](../../internal/endpoint/endpoint.go), baris 31.

```go
type RuleMeta struct {
	Rule		string	`json:"rule"`
	Type		string	`json:"type"`
	Severity	string	`json:"severity"`
	Title		string	`json:"title"`
	Explain		string	`json:"explain"`
}
```

### execution.AtomicTest

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 51.

```go
type AtomicTest struct {
	ID		uuid.UUID		`json:"id"`
	Name		string			`json:"name"`
	ContractKey	string			`json:"contract_key"`
	ContractName	string			`json:"contract_name"`
	Params		json.RawMessage		`json:"params"`
	TargetAssetID	*uuid.UUID		`json:"target_asset_id"`
	TargetAssetName	*string			`json:"target_asset_name"`
	CreatedAt	time.Time		`json:"created_at"`
	UpdatedAt	time.Time		`json:"updated_at"`
	Version		int			`json:"version"`
	LastExecution	*ExecutionSummary	`json:"last_execution"`
	Executions	[]ExecutionSummary	`json:"executions,omitempty"`
}
```

### execution.ClaimSpec

Sumber: [internal/execution/runtime.go](../../internal/execution/runtime.go), baris 137.

```go
type ClaimSpec struct {
	ContractVersion	int			`json:"contract_version"`
	ScanScope	*contract.ScanScope	`json:"scan_scope,omitempty"`
	ExecutionID	uuid.UUID		`json:"execution_id"`
	ContractKey	string			`json:"contract_key"`
	Target		string			`json:"target"`
	Params		json.RawMessage		`json:"params"`
	TimeoutSeconds	int			`json:"timeout_seconds"`
}
```

### execution.ContractView

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 23.

```go
type ContractView struct {
	Version			int		`json:"version"`
	Category		string		`json:"category"`
	Schema			contract.Schema	`json:"input_schema"`
	Key			string		`json:"key"`
	Name			string		`json:"name"`
	Description		string		`json:"description"`
	Kind			string		`json:"kind"`
	OutputTypes		[]string	`json:"output_types"`
	TimeoutSeconds		int		`json:"timeout_seconds"`
	Integration		string		`json:"integration"`
	IntegrationStatus	string		`json:"integration_status"`
	NeedsAgent		bool		`json:"needs_agent"`
	LastHeartbeatAt		*time.Time	`json:"last_heartbeat_at"`
}
```

### execution.ExecutionDetail

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 72.

```go
type ExecutionDetail struct {
	ExecutionSummary
	AtomicTestID	*uuid.UUID	`json:"atomic_test_id"`
	ContractKey	string		`json:"contract_key"`
	TargetAssetID	uuid.UUID	`json:"target_asset_id"`
	TargetAddress	string		`json:"target_address"`
	Params		json.RawMessage	`json:"params"`
	ExitCode	*int		`json:"exit_code"`
	RawOutputKey	*string		`json:"raw_output_key"`
	Logs		[]LogLine	`json:"logs"`
	Findings	[]Finding	`json:"findings"`
}
```

### execution.ExecutionSummary

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 39.

```go
type ExecutionSummary struct {
	IsDemo		bool		`json:"is_demo"`
	ID		uuid.UUID	`json:"id"`
	State		string		`json:"state"`
	Outcome		*string		`json:"outcome"`
	Error		*string		`json:"error"`
	CreatedAt	time.Time	`json:"created_at"`
	StartedAt	*time.Time	`json:"started_at"`
	EndedAt		*time.Time	`json:"ended_at"`
	FindingCount	int		`json:"finding_count"`
}
```

### execution.Finding

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 85.

```go
type Finding struct {
	ID			uuid.UUID	`json:"id"`
	AssetID			*uuid.UUID	`json:"asset_id"`
	AssetName		*string		`json:"asset_name"`
	Type			string		`json:"type"`
	Value			string		`json:"value"`
	Title			string		`json:"title"`
	Severity		string		`json:"severity"`
	Details			json.RawMessage	`json:"details"`
	FirstSeenAt		time.Time	`json:"first_seen_at"`
	LastSeenAt		time.Time	`json:"last_seen_at"`
	OccurrenceCount		int		`json:"occurrence_count"`
	OrganizationID		*uuid.UUID	`json:"organization_id"`
	OrganizationName	*string		`json:"organization_name"`
}
```

### execution.FindingPage

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 101.

```go
type FindingPage struct {
	Items		[]Finding	`json:"items"`
	NextCursor	*string		`json:"next_cursor"`
	Search		string		`json:"search"`
}
```

### execution.LogLine

Sumber: [internal/execution/model.go](../../internal/execution/model.go), baris 66.

```go
type LogLine struct {
	At	time.Time	`json:"at"`
	Level	string		`json:"level"`
	Message	string		`json:"message"`
}
```

### execution.NewFinding

Sumber: [internal/execution/findings.go](../../internal/execution/findings.go), baris 37.

```go
type NewFinding struct {
	Type		string		`json:"type"`
	Value		string		`json:"value"`
	Title		string		`json:"title"`
	Severity	string		`json:"severity"`
	Details		json.RawMessage	`json:"details"`
}
```

### execution.Preset

Sumber: [internal/execution/presets.go](../../internal/execution/presets.go), baris 19.

```go
type Preset struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	ContractKey	string		`json:"contract_key"`
	ContractVersion	int		`json:"contract_version"`
	Params		json.RawMessage	`json:"params"`
	Version		int		`json:"version"`
}
```

### execution.Result

Sumber: [internal/execution/runtime.go](../../internal/execution/runtime.go), baris 325.

```go
type Result struct {
	ExitCode	*int		`json:"exit_code"`
	Error		string		`json:"error"`
	TimedOut	bool		`json:"timed_out"`
	Output		string		`json:"output"`
	Logs		[]LogLine	`json:"logs"`
	Findings	[]NewFinding	`json:"findings"`

	Datasets	[]osquery.Dataset	`json:"datasets,omitempty"`
}
```

### execution.TestInput

Sumber: [internal/execution/service.go](../../internal/execution/service.go), baris 104.

```go
type TestInput struct {
	Name		string		`json:"name"`
	ContractKey	string		`json:"contract_key"`
	Params		json.RawMessage	`json:"params"`
	TargetAssetID	*uuid.UUID	`json:"target_asset_id"`
}
```

### execution.TestPatch

Sumber: [internal/execution/service.go](../../internal/execution/service.go), baris 262.

```go
type TestPatch struct {
	Version		int		`json:"version"`
	Name		*string		`json:"name"`
	Params		json.RawMessage	`json:"params"`
	TargetAssetID	*uuid.UUID	`json:"target_asset_id"`
}
```

### execution.endpointPolicy

Sumber: [internal/execution/endpoint.go](../../internal/execution/endpoint.go), baris 151.

```go
type endpointPolicy struct {
	Organization	*uuid.UUID	`json:"organization_id"`
	Revision	int64		`json:"revision"`
	AssetVersion	int64		`json:"asset_version"`
}
```

### execution.findingCursor

Sumber: [internal/execution/findings.go](../../internal/execution/findings.go), baris 192.

```go
type findingCursor struct {
	T	time.Time	`json:"t"`
	I	uuid.UUID	`json:"i"`
}
```

### groups.Detail

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 45.

```go
type Detail struct {
	Group
	Members		[]Member	`json:"members"`
	TotalAssets	int		`json:"total_assets"`
	MembersCapped	bool		`json:"members_capped"`
}
```

### groups.Filter

Sumber: [internal/groups/rule.go](../../internal/groups/rule.go), baris 13.

```go
type Filter struct {
	Field		string		`json:"field"`
	Operator	string		`json:"operator"`
	Values		[]string	`json:"values"`
}
```

### groups.Group

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 23.

```go
type Group struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Description	*string		`json:"description"`
	Kind		string		`json:"kind"`
	Rule		Rule		`json:"rule"`
	IsSystem	bool		`json:"is_system"`
	MemberCount	int		`json:"member_count"`
	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`
	Version		int		`json:"version"`
}
```

### groups.Input

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 214.

```go
type Input struct {
	Name		string	`json:"name"`
	Description	*string	`json:"description"`
	Rule		Rule	`json:"rule"`
	Version		*int	`json:"version"`
}
```

### groups.Member

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 36.

```go
type Member struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Category	string		`json:"category"`
	Platform	*string		`json:"platform"`
	Arch		*string		`json:"arch"`
	Criticality	string		`json:"criticality"`
}
```

### groups.Preview

Sumber: [internal/groups/service.go](../../internal/groups/service.go), baris 329.

```go
type Preview struct {
	Count	int		`json:"count"`
	Total	int		`json:"total"`
	Sample	[]Member	`json:"sample"`
}
```

### groups.Rule

Sumber: [internal/groups/rule.go](../../internal/groups/rule.go), baris 19.

```go
type Rule struct {
	Mode	string		`json:"mode"`
	Filters	[]Filter	`json:"filters"`
}
```

### home.AssetStats

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 21.

```go
type AssetStats struct {
	Total		int	`json:"total"`
	Authorized	int	`json:"authorized"`
	InternetFacing	int	`json:"internet_facing"`
}
```

### home.AtomicStats

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 36.

```go
type AtomicStats struct {
	Tests		int	`json:"tests"`
	Active		int	`json:"active"`
	Runs7d		int	`json:"runs_7d"`
	Failed7d	int	`json:"failed_7d"`
}
```

### home.ChainStats

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 43.

```go
type ChainStats struct {
	Scenarios	int	`json:"scenarios"`
	Simulations	int	`json:"simulations"`
	Running		int	`json:"running"`
}
```

### home.ExposedAsset

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 70.

```go
type ExposedAsset struct {
	AssetID		uuid.UUID	`json:"asset_id"`
	Name		string		`json:"name"`
	Critical	int		`json:"critical"`
	High		int		`json:"high"`
	Medium		int		`json:"medium"`
	Low		int		`json:"low"`
	OpenPorts	int		`json:"open_ports"`
}
```

### home.FindingStats

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 27.

```go
type FindingStats struct {
	Total			int		`json:"total"`
	Vulnerabilities		int		`json:"vulnerabilities"`
	OpenPorts		int		`json:"open_ports"`
	VulnerableAssets	int		`json:"vulnerable_assets"`
	BySeverity		map[string]int	`json:"by_severity"`
	New7d			int		`json:"new_7d"`
}
```

### home.InjectorRow

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 80.

```go
type InjectorRow struct {
	Key	string	`json:"key"`
	Name	string	`json:"name"`
	Status	string	`json:"status"`
}
```

### home.OrganizationOverview

Sumber: [internal/home/organizations.go](../../internal/home/organizations.go), baris 9.

```go
type OrganizationOverview struct {
	ID			uuid.UUID	`json:"id"`
	Name			string		`json:"name"`
	Assets			int		`json:"assets"`
	Authorized		int		`json:"authorized"`
	Risk			string		`json:"risk"`
	Comparison		string		`json:"comparison"`
	LatestAssessment	*uuid.UUID	`json:"latest_assessment"`
	LatestStatus		string		`json:"latest_status"`
}
```

### home.RunRow

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 58.

```go
type RunRow struct {
	ExecutionID	uuid.UUID	`json:"execution_id"`
	TestID		uuid.UUID	`json:"test_id"`
	TestName	string		`json:"test_name"`
	ContractName	string		`json:"contract_name"`
	Target		*string		`json:"target"`
	State		string		`json:"state"`
	Outcome		*string		`json:"outcome"`
	CreatedAt	time.Time	`json:"created_at"`
	FindingCount	int		`json:"finding_count"`
}
```

### home.SimRow

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 49.

```go
type SimRow struct {
	ID		uuid.UUID	`json:"id"`
	Code		string		`json:"code"`
	Name		string		`json:"name"`
	Status		string		`json:"status"`
	StartedAt	*time.Time	`json:"started_at"`
	CreatedAt	time.Time	`json:"created_at"`
}
```

### home.Summary

Sumber: [internal/home/home.go](../../internal/home/home.go), baris 86.

```go
type Summary struct {
	Organizations	[]OrganizationOverview	`json:"organizations"`
	Assets		*AssetStats		`json:"assets"`
	Findings	*FindingStats		`json:"findings"`
	Atomic		*AtomicStats		`json:"atomic"`
	Chains		*ChainStats		`json:"chains"`
	LatestSims	[]SimRow		`json:"latest_simulations"`
	LatestRuns	[]RunRow		`json:"latest_runs"`
	MostExposed	[]ExposedAsset		`json:"most_exposed"`
	Injectors	[]InjectorRow		`json:"injectors"`
	GeneratedAt	time.Time		`json:"generated_at"`
}
```

### httpx.FieldError

Sumber: [internal/httpx/problem.go](../../internal/httpx/problem.go), baris 13.

```go
type FieldError struct {
	Code	string	`json:"code,omitempty"`
	Name	string	`json:"name"`
	Message	string	`json:"message"`
}
```

### httpx.Problem

Sumber: [internal/httpx/problem.go](../../internal/httpx/problem.go), baris 19.

```go
type Problem struct {
	Code		string		`json:"code"`
	Type		string		`json:"type"`
	Title		string		`json:"title"`
	Status		int		`json:"status"`
	Detail		string		`json:"detail,omitempty"`
	ErrorID		string		`json:"error_id"`
	RequestID	string		`json:"request_id,omitempty"`
	Fields		[]FieldError	`json:"fields,omitempty"`

	RetryAfter	int	`json:"retry_after_seconds,omitempty"`
}
```

### injector.nucleiLine

Sumber: [internal/injector/parse.go](../../internal/injector/parse.go), baris 90.

```go
type nucleiLine struct {
	TemplateID	string	`json:"template-id"`
	Info		struct {
		Name		string		`json:"name"`
		Severity	string		`json:"severity"`
		Description	string		`json:"description"`
		Reference	[]string	`json:"reference"`
		Tags		[]string	`json:"tags"`
	}	`json:"info"`
	Type			string		`json:"type"`
	MatchedAt		string		`json:"matched-at"`
	MatcherName		string		`json:"matcher-name"`
	ExtractedResults	[]string	`json:"extracted-results"`
}
```

### integrations.Integration

Sumber: [internal/integrations/integrations.go](../../internal/integrations/integrations.go), baris 29.

```go
type Integration struct {
	IsDemo		bool		`json:"is_demo"`
	ID		uuid.UUID	`json:"id"`
	Kind		string		`json:"kind"`
	Key		string		`json:"key"`
	Name		string		`json:"name"`
	Description	*string		`json:"description"`
	IsBuiltin	bool		`json:"is_builtin"`
	Enabled		bool		`json:"enabled"`
	Status		string		`json:"status"`
	SoftwareVersion	*string		`json:"software_version"`
	LastHeartbeatAt	*time.Time	`json:"last_heartbeat_at"`
	HasToken	bool		`json:"has_token"`
	Logs		[]LogLine	`json:"logs"`
}
```

### integrations.LogLine

Sumber: [internal/integrations/integrations.go](../../internal/integrations/integrations.go), baris 23.

```go
type LogLine struct {
	At	time.Time	`json:"at"`
	Level	string		`json:"level"`
	Message	string		`json:"message"`
}
```

### integrations.NewToken

Sumber: [internal/integrations/integrations.go](../../internal/integrations/integrations.go), baris 129.

```go
type NewToken struct {
	Token string `json:"token"`
}
```

### mitre.Cell

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 26.

```go
type Cell struct {
	ID			string		`json:"id"`
	Name			string		`json:"name"`
	Mapped			bool		`json:"mapped"`
	Exercised		bool		`json:"exercised"`
	Simulated		bool		`json:"simulated"`
	SimulatedRuns		int		`json:"simulated_runs"`
	SimulatedFindings	int		`json:"simulated_findings"`
	Platforms		[]string	`json:"platforms"`
	Runs			int		`json:"runs"`
	LastRunAt		*time.Time	`json:"last_run_at"`
	Subs			[]Cell		`json:"subs,omitempty"`
}
```

### mitre.Column

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 40.

```go
type Column struct {
	ID		string	`json:"id"`
	Name		string	`json:"name"`
	Shortname	string	`json:"shortname"`
	Techniques	[]Cell	`json:"techniques"`
}
```

### mitre.Detail

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 172.

```go
type Detail struct {
	ID			string		`json:"id"`
	Name			string		`json:"name"`
	Description		string		`json:"description"`
	Platforms		[]string	`json:"platforms"`
	Tactics			[]string	`json:"tactics"`
	URL			string		`json:"url"`
	Actions			[]string	`json:"actions"`
	Mappings		[]Mapping	`json:"mappings"`
	Mitigations		[]Mitigation	`json:"mitigations"`
	Parent			*string		`json:"parent"`
	Deprecated		bool		`json:"deprecated"`
	Simulated		bool		`json:"simulated"`
	SimulatedRuns		int		`json:"simulated_runs"`
	SimulatedFindings	int		`json:"simulated_findings"`
	Runs			int		`json:"runs"`
	LastRunAt		*time.Time	`json:"last_run_at"`
	Subs			[]Cell		`json:"subs"`
}
```

### mitre.Manifest

Sumber: [internal/mitre/freeze.go](../../internal/mitre/freeze.go), baris 19.

```go
type Manifest struct {
	Matrix		string		`json:"matrix"`
	Version		string		`json:"version"`
	File		string		`json:"file"`
	SHA256		string		`json:"sha256"`
	Bytes		int64		`json:"bytes"`
	SourceURL	string		`json:"source_url"`
	Counts		ManifestCount	`json:"counts"`
}
```

### mitre.ManifestCount

Sumber: [internal/mitre/freeze.go](../../internal/mitre/freeze.go), baris 29.

```go
type ManifestCount struct {
	Tactics		int	`json:"tactics"`
	Techniques	int	`json:"techniques"`
	Subtechniques	int	`json:"subtechniques"`
	Deprecated	int	`json:"deprecated"`
	RevokedSkipped	int	`json:"revoked_skipped"`
}
```

### mitre.Mapping

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 165.

```go
type Mapping struct {
	Action	string	`json:"action"`
	Reason	string	`json:"reason"`
	Basis	string	`json:"basis"`
	Via	string	`json:"via,omitempty"`
}
```

### mitre.Matrix

Sumber: [internal/mitre/matrix.go](../../internal/mitre/matrix.go), baris 47.

```go
type Matrix struct {
	Tactics		[]Column	`json:"tactics"`
	Total		int		`json:"total"`
	Mapped		int		`json:"mapped"`
	Exercised	int		`json:"exercised"`
	Simulated	int		`json:"simulated"`
	Platforms	[]string	`json:"platforms"`
	Imported	bool		`json:"imported"`
	LastRun		*Run		`json:"last_run"`
}
```

### mitre.Mitigation

Sumber: [internal/mitre/stix.go](../../internal/mitre/stix.go), baris 28.

```go
type Mitigation struct {
	ID		string	`json:"id"`
	Name		string	`json:"name"`
	Description	string	`json:"description"`
}
```

### mitre.Run

Sumber: [internal/mitre/import.go](../../internal/mitre/import.go), baris 83.

```go
type Run struct {
	ID		uuid.UUID	`json:"id"`
	Status		string		`json:"status"`
	Version		*string		`json:"source_version"`
	Techniques	*int		`json:"techniques"`
	Tactics		*int		`json:"tactics"`
	Subs		*int		`json:"subtechniques"`
	Deprecated	*int		`json:"deprecated"`
	SHA256		*string		`json:"source_sha256"`
	Error		*string		`json:"error"`
	StartedAt	time.Time	`json:"started_at"`
	FinishedAt	*time.Time	`json:"finished_at"`
}
```

### mitre.stixObject

Sumber: [internal/mitre/stix.go](../../internal/mitre/stix.go), baris 80.

```go
type stixObject struct {
	Type		string		`json:"type"`
	ID		string		`json:"id"`
	Name		string		`json:"name"`
	Description	string		`json:"description"`
	Revoked		bool		`json:"revoked"`
	Deprecated	bool		`json:"x_mitre_deprecated"`
	IsSub		bool		`json:"x_mitre_is_subtechnique"`
	Platforms	[]string	`json:"x_mitre_platforms"`
	TacticRefs	[]string	`json:"tactic_refs"`
	RelType		string		`json:"relationship_type"`
	SourceRef	string		`json:"source_ref"`
	TargetRef	string		`json:"target_ref"`
	Shortname	string		`json:"x_mitre_shortname"`
	Version		string		`json:"x_mitre_version"`
	ExternalRefs	[]struct {
		Source	string	`json:"source_name"`
		ID	string	`json:"external_id"`
		URL	string	`json:"url"`
	}	`json:"external_references"`
	KillChain	[]struct {
		Name	string	`json:"kill_chain_name"`
		Phase	string	`json:"phase_name"`
	}	`json:"kill_chain_phases"`
}
```

### organizations.Assessment

Sumber: [internal/organizations/assessments.go](../../internal/organizations/assessments.go), baris 31.

```go
type Assessment struct {
	ResultsExpiredAt	*time.Time		`json:"results_expired_at,omitempty"`
	ID			uuid.UUID		`json:"id"`
	OrganizationID		uuid.UUID		`json:"organization_id"`
	Name			string			`json:"name"`
	Profile			string			`json:"profile"`
	Status			string			`json:"status"`
	CreatedAt		time.Time		`json:"created_at"`
	EndedAt			*time.Time		`json:"ended_at"`
	Targets			[]AssessmentTarget	`json:"targets,omitempty"`
}
```

### organizations.AssessmentInput

Sumber: [internal/organizations/assessments.go](../../internal/organizations/assessments.go), baris 24.

```go
type AssessmentInput struct {
	Language	string		`json:"language,omitempty"`
	NmapParams	json.RawMessage	`json:"nmap_params,omitempty"`
	NucleiParams	json.RawMessage	`json:"nuclei_params,omitempty"`
	Profile		string		`json:"profile"`
	AssetIDs	[]uuid.UUID	`json:"asset_ids"`
}
```

### organizations.AssessmentTarget

Sumber: [internal/organizations/assessments.go](../../internal/organizations/assessments.go), baris 42.

```go
type AssessmentTarget struct {
	AssetID		uuid.UUID		`json:"asset_id"`
	Name		string			`json:"name"`
	Addresses	[]contract.Address	`json:"addresses"`
	SimulationID	*uuid.UUID		`json:"simulation_id"`
	Status		string			`json:"status"`
	SkipReason	string			`json:"skip_reason"`
}
```

### organizations.Input

Sumber: [internal/organizations/organizations.go](../../internal/organizations/organizations.go), baris 42.

```go
type Input struct {
	Name		string	`json:"name"`
	Slug		string	`json:"slug"`
	Description	string	`json:"description"`
	Industry	string	`json:"industry"`
	Contact		string	`json:"contact"`
	Version		int	`json:"version"`
}
```

### organizations.MembershipInput

Sumber: [internal/organizations/organizations.go](../../internal/organizations/organizations.go), baris 181.

```go
type MembershipInput struct {
	Assets	[]struct {
		ID	uuid.UUID	`json:"id"`
		Version	int		`json:"version"`
	}	`json:"assets"`
	Authorized	*bool	`json:"testing_authorized"`
	Unassign	bool	`json:"unassign"`
}
```

### organizations.Organization

Sumber: [internal/organizations/organizations.go](../../internal/organizations/organizations.go), baris 31.

```go
type Organization struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Slug		string		`json:"slug"`
	Description	string		`json:"description"`
	Industry	string		`json:"industry"`
	Contact		string		`json:"contact"`
	Version		int		`json:"version"`
	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`
}
```

### organizations.Page

Sumber: [internal/organizations/organizations.go](../../internal/organizations/organizations.go), baris 114.

```go
type Page struct {
	Items		[]Organization	`json:"items"`
	NextCursor	*string		`json:"next_cursor"`
}
```

### organizations.Readiness

Sumber: [internal/organizations/readiness.go](../../internal/organizations/readiness.go), baris 19.

```go
type Readiness struct {
	Total		int			`json:"total"`
	Authorized	int			`json:"authorized"`
	Ready		int			`json:"ready"`
	Items		[]ReadinessAsset	`json:"items"`
}
```

### organizations.ReadinessAsset

Sumber: [internal/organizations/readiness.go](../../internal/organizations/readiness.go), baris 13.

```go
type ReadinessAsset struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Authorized	bool		`json:"authorized"`
	Reason		string		`json:"reason"`
}
```

### osquery.Aggregate

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 50.

```go
type Aggregate struct {
	Key	string		`json:"key"`
	Op	string		`json:"op"`
	Cols	[]string	`json:"cols"`
	Limit	int		`json:"limit"`
	Where	[]Match		`json:"where"`
}
```

### osquery.Artifact

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 95.

```go
type Artifact struct {
	File		string	`json:"file"`
	URL		string	`json:"url"`
	SHA256		string	`json:"sha256"`
	Bytes		int64	`json:"bytes"`
	Binary		string	`json:"binary"`
	MaxBinaryBytes	int64	`json:"max_binary_bytes"`
}
```

### osquery.Attributes

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 58.

```go
type Attributes struct {
	FirstRow	map[string]string	`json:"first_row"`
	Aggregates	[]Aggregate		`json:"aggregates"`
}
```

### osquery.ConsentDocument

Sumber: [internal/osquery/consent.go](../../internal/osquery/consent.go), baris 19.

```go
type ConsentDocument struct {
	ConsentRef
	Text		string			`json:"text"`
	Translations	map[string]string	`json:"translations"`
}
```

### osquery.ConsentRef

Sumber: [internal/osquery/consent.go](../../internal/osquery/consent.go), baris 15.

```go
type ConsentRef struct {
	Version	string	`json:"version"`
	SHA256	string	`json:"sha256"`
}
```

### osquery.Dataset

Sumber: [internal/osquery/data.go](../../internal/osquery/data.go), baris 36.

```go
type Dataset struct {
	Query		string			`json:"query"`
	Status		string			`json:"status"`
	Reason		string			`json:"reason,omitempty"`
	Rows		[]map[string]string	`json:"rows"`
	Truncated	bool			`json:"truncated,omitempty"`
	CollectedAt	time.Time		`json:"collected_at"`
	Version		string			`json:"osquery_version,omitempty"`
}
```

### osquery.FindingTpl

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 29.

```go
type FindingTpl struct {
	Type		string	`json:"type"`
	Severity	string	`json:"severity"`
	Title		string	`json:"title"`
	Value		string	`json:"value"`
}
```

### osquery.Match

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 23.

```go
type Match struct {
	Col	string	`json:"col"`
	Op	string	`json:"op"`
	Val	any	`json:"val"`
}
```

### osquery.MitreRef

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 63.

```go
type MitreRef struct {
	ID	string	`json:"id"`
	Reason	string	`json:"reason"`
}
```

### osquery.Pack

Sumber: [internal/osquery/packs.go](../../internal/osquery/packs.go), baris 30.

```go
type Pack struct {
	Name		string		`json:"name"`
	Description	string		`json:"description"`
	File		string		`json:"file"`
	SHA256		string		`json:"sha256"`
	Platform	string		`json:"platform"`
	Queries		[]PackQuery	`json:"queries"`
}
```

### osquery.PackQuery

Sumber: [internal/osquery/packs.go](../../internal/osquery/packs.go), baris 20.

```go
type PackQuery struct {
	Name		string		`json:"name"`
	Platform	string		`json:"platform"`
	Key		string		`json:"key,omitempty"`
	Description	string		`json:"description"`
	Status		string		`json:"status"`
	Reason		string		`json:"reason"`
	Tables		[]string	`json:"tables"`
}
```

### osquery.PortCompare

Sumber: [internal/osquery/posture.go](../../internal/osquery/posture.go), baris 12.

```go
type PortCompare struct {
	Both		[]int	`json:"both"`
	ScanOnly	[]int	`json:"scan_only"`
	HostOnly	[]int	`json:"host_only"`
	Note		string	`json:"note"`
}
```

### osquery.Posture

Sumber: [internal/osquery/posture.go](../../internal/osquery/posture.go), baris 26.

```go
type Posture struct {
	CollectedAt	time.Time		`json:"collected_at"`
	Source		string			`json:"source"`
	Version		string			`json:"osquery_version,omitempty"`
	Attributes	map[string]any		`json:"attributes"`
	Findings	[]PostureFinding	`json:"findings"`
	Ports		*PortCompare		`json:"ports,omitempty"`
}
```

### osquery.PostureFinding

Sumber: [internal/osquery/posture.go](../../internal/osquery/posture.go), baris 19.

```go
type PostureFinding struct {
	Severity	string	`json:"severity"`
	Title		string	`json:"title"`
	Query		string	`json:"query"`
}
```

### osquery.Query

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 69.

```go
type Query struct {
	Key		string	`json:"key"`
	Platform	string	`json:"platform"`

	Pack		string		`json:"pack,omitempty"`
	Title		string		`json:"title"`
	Description	string		`json:"description"`
	Category	string		`json:"category"`
	SQL		string		`json:"sql"`
	Tables		[]string	`json:"tables"`
	Columns		[]string	`json:"columns"`
	OS		[]string	`json:"os"`
	Sensitivity	string		`json:"sensitivity"`
	Load		string		`json:"load"`
	NeedsRoot	bool		`json:"needs_root"`
	MaxRows		int		`json:"max_rows"`
	TimeoutSeconds	int		`json:"timeout_seconds"`
	Mitre		[]MitreRef	`json:"mitre"`
	Attributes	Attributes	`json:"attributes"`
	Rules		[]Rule		`json:"rules"`

	Example	[]map[string]string	`json:"example"`
}
```

### osquery.Rule

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 36.

```go
type Rule struct {
	ID		string	`json:"id"`
	Scope		string	`json:"scope"`
	Match		[]Match	`json:"match"`
	CountWhere	[]Match	`json:"count_where"`
	CountGT		int	`json:"count_gt"`

	CountLT	int		`json:"count_lt"`
	Require	[]Match		`json:"require"`
	Finding	FindingTpl	`json:"finding"`
	Explain	string		`json:"explain"`
}
```

### osquery.catalogFile

Sumber: [internal/osquery/osquery.go](../../internal/osquery/osquery.go), baris 104.

```go
type catalogFile struct {
	Version		string			`json:"osquery_version"`
	License		string			`json:"license"`
	Artifacts	map[string]Artifact	`json:"artifacts"`
	Queries		[]Query			`json:"queries"`
}
```

### osquery.packsFile

Sumber: [internal/osquery/packs.go](../../internal/osquery/packs.go), baris 39.

```go
type packsFile struct {
	OsqueryVersion	string	`json:"osquery_version"`
	ArtifactSHA256	string	`json:"artifact_sha256"`
	Packs		[]Pack	`json:"packs"`
	Queries		[]Query	`json:"queries"`
}
```

### people.MemberRef

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 51.

```go
type MemberRef struct {
	ID	uuid.UUID	`json:"id"`
	Name	string		`json:"name"`
	Email	*string		`json:"email"`
}
```

### people.MembersInput

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 421.

```go
type MembersInput struct {
	Version		int		`json:"version"`
	PersonIDs	[]uuid.UUID	`json:"person_ids"`
}
```

### people.Person

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 30.

```go
type Person struct {
	ID		uuid.UUID	`json:"id"`
	FirstName	string		`json:"first_name"`
	LastName	string		`json:"last_name"`
	Email		*string		`json:"email"`
	Organization	*string		`json:"organization"`
	Teams		[]TeamRef	`json:"teams"`
	CreatedAt	time.Time	`json:"created_at"`
	Version		int		`json:"version"`
}
```

### people.PersonInput

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 88.

```go
type PersonInput struct {
	FirstName	string	`json:"first_name"`
	LastName	string	`json:"last_name"`
	Email		*string	`json:"email"`
	Organization	*string	`json:"organization"`
}
```

### people.PersonPatch

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 219.

```go
type PersonPatch struct {
	Version	int	`json:"version"`
	PersonInput
}
```

### people.Team

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 41.

```go
type Team struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Description	*string		`json:"description"`
	Kind		string		`json:"kind"`
	MemberCount	int		`json:"member_count"`
	CreatedAt	time.Time	`json:"created_at"`
	Version		int		`json:"version"`
}
```

### people.TeamDetail

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 57.

```go
type TeamDetail struct {
	Team
	Members	[]MemberRef	`json:"members"`
}
```

### people.TeamInput

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 274.

```go
type TeamInput struct {
	Name		string	`json:"name"`
	Description	*string	`json:"description"`
	Kind		string	`json:"kind"`
}
```

### people.TeamPatch

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 371.

```go
type TeamPatch struct {
	Version	int	`json:"version"`
	TeamInput
}
```

### people.TeamRef

Sumber: [internal/people/people.go](../../internal/people/people.go), baris 25.

```go
type TeamRef struct {
	ID	uuid.UUID	`json:"id"`
	Name	string		`json:"name"`
}
```

### reporting.Asset

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 83.

```go
type Asset struct {
	Discovered	bool			`json:"discovered,omitempty"`
	Endpoint	*osquery.Posture	`json:"endpoint,omitempty"`
	ID		uuid.UUID		`json:"id"`
	Name		string			`json:"name"`
	Status		string			`json:"status"`
	SkipReason	string			`json:"skip_reason"`
	Findings	[]Finding		`json:"findings"`
	Methods		[]Method		`json:"methods"`
}
```

### reporting.ChangeCounts

Sumber: [internal/reporting/comparison.go](../../internal/reporting/comparison.go), baris 16.

```go
type ChangeCounts struct {
	New		int	`json:"new"`
	Persistent	int	`json:"persistent"`
	Resolved	int	`json:"resolved"`
	Uncompared	int	`json:"uncompared"`
}
```

### reporting.Comparison

Sumber: [internal/reporting/comparison.go](../../internal/reporting/comparison.go), baris 44.

```go
type Comparison struct {
	Version		int			`json:"version"`
	Totals		ChangeCounts		`json:"totals"`
	BySeverity	map[string]ChangeCounts	`json:"by_severity"`
	Rows		[]FindingChange		`json:"rows"`
	Checks		[]ComparisonCheck	`json:"checks"`
}
```

### reporting.ComparisonCheck

Sumber: [internal/reporting/comparison.go](../../internal/reporting/comparison.go), baris 34.

```go
type ComparisonCheck struct {
	AssetID			uuid.UUID	`json:"asset_id"`
	Action			string		`json:"action"`
	Target			string		`json:"target"`
	BaselineAssessmentID	*uuid.UUID	`json:"baseline_assessment_id,omitempty"`
	BaselineReportID	*uuid.UUID	`json:"baseline_report_id,omitempty"`
	BaselineAt		*time.Time	`json:"baseline_at,omitempty"`
	Comparable		bool		`json:"comparable"`
	Reason			string		`json:"reason,omitempty"`
}
```

### reporting.EndpointSummary

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 77.

```go
type EndpointSummary struct {
	AssetsTotal	int		`json:"assets_total"`
	AssetsWithData	int		`json:"assets_with_data"`
	BySeverity	map[string]int	`json:"by_severity"`
}
```

### reporting.Finding

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 54.

```go
type Finding struct {
	Coverage	[]string		`json:"coverage,omitempty"`
	MITRE		[]FindingTechnique	`json:"mitre,omitempty"`
	ID		string			`json:"id"`
	Type		string			`json:"type"`
	Value		string			`json:"value"`
	Title		string			`json:"title"`
	Severity	string			`json:"severity"`
	Evidence	map[string]any		`json:"evidence"`
}
```

### reporting.FindingChange

Sumber: [internal/reporting/comparison.go](../../internal/reporting/comparison.go), baris 22.

```go
type FindingChange struct {
	AssetID		uuid.UUID	`json:"asset_id"`
	AssetName	string		`json:"asset_name"`
	FindingID	string		`json:"finding_id"`
	Type		string		`json:"type"`
	Value		string		`json:"value"`
	Title		string		`json:"title"`
	Severity	string		`json:"severity"`
	Change		string		`json:"change"`
	Reason		string		`json:"reason,omitempty"`
	Actions		[]string	`json:"actions"`
}
```

### reporting.FindingTechnique

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 36.

```go
type FindingTechnique struct {
	ID	string	`json:"id"`
	Name	string	`json:"name"`
	Basis	string	`json:"basis"`
	Reason	string	`json:"reason"`
}
```

### reporting.Method

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 64.

```go
type Method struct {
	MITRE		[]string	`json:"mitre"`
	Action		string		`json:"action"`
	Version		int		`json:"version"`
	Params		json.RawMessage	`json:"params"`
	Target		string		`json:"target"`
	Outcome		string		`json:"outcome"`
	Error		string		`json:"error"`
	StartedAt	*time.Time	`json:"started_at"`
	EndedAt		*time.Time	`json:"ended_at"`
}
```

### reporting.Report

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 113.

```go
type Report struct {
	ID		uuid.UUID	`json:"id"`
	OrganizationID	uuid.UUID	`json:"organization_id"`
	AssessmentID	uuid.UUID	`json:"assessment_id"`
	Version		int		`json:"version"`
	Status		string		`json:"status"`
	Error		string		`json:"error"`
	CreatedAt	time.Time	`json:"created_at"`
	Snapshot	Snapshot	`json:"snapshot"`
}
```

### reporting.Snapshot

Sumber: [internal/reporting/reporting.go](../../internal/reporting/reporting.go), baris 93.

```go
type Snapshot struct {
	Language		string			`json:"language,omitempty"`
	FindingsEpoch		int64			`json:"findings_epoch,omitempty"`
	ComparisonDetails	*Comparison		`json:"comparison_details,omitempty"`
	SchemaVersion		string			`json:"schema_version,omitempty"`
	ScanID			*uuid.UUID		`json:"scan_id,omitempty"`
	EndpointSummary		*EndpointSummary	`json:"endpoint_summary,omitempty"`
	IsDemo			bool			`json:"is_demo,omitempty"`
	Label			string			`json:"label,omitempty"`
	Organization		string			`json:"organization"`
	AssessmentID		uuid.UUID		`json:"assessment_id"`
	Profile			string			`json:"profile"`
	CreatedAt		time.Time		`json:"created_at"`
	Status			string			`json:"status"`
	Risk			string			`json:"risk"`
	BySeverity		map[string]int		`json:"by_severity"`
	Assets			[]Asset			`json:"assets"`
	Remediation		string			`json:"remediation"`
	Comparison		string			`json:"comparison"`
}
```

### retention.Counts

Sumber: [internal/retention/organization.go](../../internal/retention/organization.go), baris 19.

```go
type Counts struct {
	Assets			int	`json:"assets"`
	Agents			int	`json:"agents"`
	Findings		int	`json:"findings"`
	Reports			int	`json:"reports"`
	EndpointDatasets	int	`json:"endpoint_datasets"`
	Scans			int	`json:"scans"`
	Executions		int	`json:"executions"`
}
```

### retention.DeleteInput

Sumber: [internal/retention/organization.go](../../internal/retention/organization.go), baris 35.

```go
type DeleteInput struct {
	Name	string	`json:"name"`
	Version	int	`json:"version"`
	Counts	Counts	`json:"counts"`
}
```

### retention.Deletion

Sumber: [internal/retention/organization.go](../../internal/retention/organization.go), baris 40.

```go
type Deletion struct {
	ID			uuid.UUID	`json:"id"`
	OrganizationID		uuid.UUID	`json:"organization_id"`
	OrganizationName	string		`json:"organization_name"`
	Counts			Counts		`json:"counts"`
	CreatedAt		time.Time	`json:"created_at"`
	CompletedAt		*time.Time	`json:"completed_at"`
	Pending			int		`json:"cleanup_pending"`
	Status			string		`json:"status"`
}
```

### retention.Policy

Sumber: [internal/retention/policy.go](../../internal/retention/policy.go), baris 24.

```go
type Policy struct {
	FindingsDays	int		`json:"findings_days"`
	ScanDays	int		`json:"scan_days"`
	ReportDays	int		`json:"report_days"`
	EndpointDays	int		`json:"endpoint_days"`
	AuditDays	int		`json:"audit_days"`
	Version		int		`json:"version"`
	LastErrorCode	string		`json:"last_error_code,omitempty"`
	LastSweepAt	*time.Time	`json:"last_sweep_at,omitempty"`
	CleanupPending	int		`json:"cleanup_pending"`
}
```

### retention.Preview

Sumber: [internal/retention/organization.go](../../internal/retention/organization.go), baris 28.

```go
type Preview struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Version		int		`json:"version"`
	Counts		Counts		`json:"counts"`
	Blockers	[]string	`json:"blockers"`
}
```

### scancontrol.Input

Sumber: [internal/scancontrol/service.go](../../internal/scancontrol/service.go), baris 21.

```go
type Input struct {
	Version	int	`json:"version"`
	Window	Window	`json:"window"`
}
```

### scancontrol.State

Sumber: [internal/scancontrol/policy.go](../../internal/scancontrol/policy.go), baris 26.

```go
type State struct {
	GlobalPausedBy	string		`json:"global_paused_by,omitempty"`
	AllowedNow	bool		`json:"allowed_now"`
	Restriction	string		`json:"restriction,omitempty"`
	OrganizationID	*uuid.UUID	`json:"organization_id"`
	Paused		bool		`json:"paused"`
	GlobalPaused	bool		`json:"global_paused"`
	PausedBy	string		`json:"paused_by,omitempty"`
	Window		Window		`json:"window"`
	Version		int		`json:"version"`
}
```

### scancontrol.Window

Sumber: [internal/scancontrol/window.go](../../internal/scancontrol/window.go), baris 14.

```go
type Window struct {
	Timezone	string	`json:"timezone,omitempty"`
	Days		[]int	`json:"days,omitempty"`
	Start		string	`json:"start,omitempty"`
	End		string	`json:"end,omitempty"`
}
```

### scans.ApproveInput

Sumber: [internal/scans/ranges.go](../../internal/scans/ranges.go), baris 33.

```go
type ApproveInput struct {
	CIDR		string	`json:"cidr"`
	Authorized	bool	`json:"authorized"`
	Version		int	`json:"version"`
}
```

### scans.Input

Sumber: [internal/scans/scans.go](../../internal/scans/scans.go), baris 64.

```go
type Input struct {
	Language	string		`json:"language,omitempty"`
	OrganizationID	uuid.UUID	`json:"organization_id"`
	AssetIDs	[]uuid.UUID	`json:"asset_ids"`
	RangeIDs	[]uuid.UUID	`json:"range_ids"`
	Tools		[]string	`json:"tools"`
	Preset		string		`json:"preset"`
}
```

### scans.Job

Sumber: [internal/scans/scans.go](../../internal/scans/scans.go), baris 85.

```go
type Job struct {
	ID		uuid.UUID		`json:"id"`
	AssetID		*uuid.UUID		`json:"asset_id"`
	RangeID		*uuid.UUID		`json:"range_id"`
	Name		string			`json:"name"`
	Target		string			`json:"target"`
	Tool		string			`json:"tool"`
	Status		string			`json:"status"`
	Error		string			`json:"error"`
	SimulationID	*uuid.UUID		`json:"simulation_id"`
	Findings	[]reporting.Finding	`json:"findings"`
}
```

### scans.Networks

Sumber: [internal/scans/ranges.go](../../internal/scans/ranges.go), baris 26.

```go
type Networks struct {
	Reported	[]string	`json:"reported"`
	Approvals	[]Range		`json:"approvals"`
	MaxHosts	int		`json:"max_hosts"`
	Runner		string		`json:"runner"`
	SourceError	string		`json:"source_error,omitempty"`
}
```

### scans.Range

Sumber: [internal/scans/ranges.go](../../internal/scans/ranges.go), baris 18.

```go
type Range struct {
	ID		uuid.UUID	`json:"id"`
	CIDR		string		`json:"cidr"`
	Authorized	bool		`json:"authorized"`
	Version		int		`json:"version"`
	AuthorizedBy	uuid.UUID	`json:"authorized_by"`
	AuthorizedAt	time.Time	`json:"authorized_at"`
}
```

### scans.Scan

Sumber: [internal/scans/scans.go](../../internal/scans/scans.go), baris 72.

```go
type Scan struct {
	ResultsExpiredAt	*time.Time		`json:"results_expired_at,omitempty"`
	ComparisonDetails	*reporting.Comparison	`json:"comparison_details,omitempty"`
	ID			uuid.UUID		`json:"id"`
	OrganizationID		uuid.UUID		`json:"organization_id"`
	Name			string			`json:"name"`
	Status			string			`json:"status"`
	Runner			string			`json:"runner"`
	Options			Input			`json:"options"`
	CreatedAt		time.Time		`json:"created_at"`
	EndedAt			*time.Time		`json:"ended_at"`
	Jobs			[]Job			`json:"jobs,omitempty"`
}
```

### scenario.Detail

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 65.

```go
type Detail struct {
	Scenario
	Targets		[]TargetRef	`json:"targets"`
	Steps		[]StepDTO	`json:"steps"`
	Edges		[]EdgeDTO	`json:"edges"`
	Problems	[]string	`json:"problems"`
}
```

### scenario.GraphInput

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 414.

```go
type GraphInput struct {
	Version	int		`json:"version"`
	Steps	[]StepDTO	`json:"steps"`
	Edges	[]EdgeDTO	`json:"edges"`
}
```

### scenario.Input

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 114.

```go
type Input struct {
	Name		string	`json:"name"`
	Description	*string	`json:"description"`
	Severity	string	`json:"severity"`
	Category	*string	`json:"category"`
}
```

### scenario.Patch

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 289.

```go
type Patch struct {
	Version	int	`json:"version"`
	Input
}
```

### scenario.Scenario

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 28.

```go
type Scenario struct {
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Description	*string		`json:"description"`
	Severity	string		`json:"severity"`
	Category	*string		`json:"category"`
	Status		string		`json:"status"`
	StepCount	int		`json:"step_count"`
	EdgeCount	int		`json:"edge_count"`
	TargetCount	int		`json:"target_count"`
	Simulations	int		`json:"simulations"`
	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`
	Version		int		`json:"version"`
}
```

### scenario.StepDTO

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 51.

```go
type StepDTO struct {
	Key		string		`json:"key"`
	Title		string		`json:"title"`
	Description	string		`json:"description"`
	ContractKey	string		`json:"contract_key"`
	Params		json.RawMessage	`json:"params"`
	DelaySeconds	int		`json:"delay_seconds"`
	TriggerMode	string		`json:"trigger_mode"`
	Trigger		*chain.Trigger	`json:"trigger"`
	IsEnabled	bool		`json:"is_enabled"`
}
```

### scenario.TargetRef

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 44.

```go
type TargetRef struct {
	Kind		string		`json:"kind"`
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Authorized	*bool		`json:"authorized,omitempty"`
}
```

### scenario.TargetsInput

Sumber: [internal/scenario/scenario.go](../../internal/scenario/scenario.go), baris 346.

```go
type TargetsInput struct {
	Version		int		`json:"version"`
	AssetIDs	[]uuid.UUID	`json:"asset_ids"`
	GroupIDs	[]uuid.UUID	`json:"group_ids"`
}
```

### search.FindingDoc

Sumber: [internal/search/search.go](../../internal/search/search.go), baris 83.

```go
type FindingDoc struct {
	TenantID	uuid.UUID	`json:"tenant_id"`
	AssetID		*uuid.UUID	`json:"asset_id,omitempty"`
	AssetName	string		`json:"asset_name,omitempty"`
	Type		string		`json:"type"`
	Severity	string		`json:"severity"`
	Title		string		`json:"title"`
	Value		string		`json:"value"`
	LastSeenAt	time.Time	`json:"last_seen_at"`
}
```

### settings.Component

Sumber: [internal/settings/settings.go](../../internal/settings/settings.go), baris 40.

```go
type Component struct {
	Name	string	`json:"name"`
	Status	string	`json:"status"`
	Detail	string	`json:"detail,omitempty"`
}
```

### settings.Settings

Sumber: [internal/settings/settings.go](../../internal/settings/settings.go), baris 46.

```go
type Settings struct {
	PlatformName	string		`json:"platform_name"`
	Version		string		`json:"version"`
	Components	[]Component	`json:"components"`
}
```

### simulation.AttackPath

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 723.

```go
type AttackPath struct {
	Nodes		[]PathNode	`json:"nodes"`
	Edges		[]PathEdge	`json:"edges"`
	Truncated	bool		`json:"truncated"`
}
```

### simulation.CancelInput

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 578.

```go
type CancelInput struct {
	Reason string `json:"reason"`
}
```

### simulation.Detail

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 73.

```go
type Detail struct {
	Simulation
	Steps	[]StepView	`json:"steps"`
	Edges	[]chain.Edge	`json:"edges"`
	Targets	[]TargetView	`json:"targets"`
}
```

### simulation.LaunchInput

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 106.

```go
type LaunchInput struct {
	Name string `json:"name"`
}
```

### simulation.PathEdge

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 717.

```go
type PathEdge struct {
	From	string	`json:"from"`
	To	string	`json:"to"`
	Kind	string	`json:"kind"`
}
```

### simulation.PathNode

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 707.

```go
type PathNode struct {
	ID		string	`json:"id"`
	Kind		string	`json:"kind"`
	Label		string	`json:"label"`
	Sub		string	`json:"sub,omitempty"`
	Status		string	`json:"status,omitempty"`
	Severity	string	`json:"severity,omitempty"`
	Column		int	`json:"column"`
}
```

### simulation.Simulation

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 33.

```go
type Simulation struct {
	IsDemo		bool		`json:"is_demo"`
	ID		uuid.UUID	`json:"id"`
	Number		int64		`json:"number"`
	Code		string		`json:"code"`
	Name		string		`json:"name"`
	Status		string		`json:"status"`
	ScenarioID	*uuid.UUID	`json:"scenario_id"`
	ScenarioName	*string		`json:"scenario_name"`
	SafeMode	bool		`json:"safe_mode"`
	TimeoutSecs	int		`json:"timeout_seconds"`
	CancelReason	*string		`json:"cancel_reason"`
	StartedAt	*time.Time	`json:"started_at"`
	EndedAt		*time.Time	`json:"ended_at"`
	CreatedAt	time.Time	`json:"created_at"`
	TargetCount	int		`json:"target_count"`
	StepCount	int		`json:"step_count"`
	StepsDone	int		`json:"steps_done"`
	FindingCount	int		`json:"finding_count"`
}
```

### simulation.StepView

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 54.

```go
type StepView struct {
	Key		string		`json:"key"`
	Title		string		`json:"title"`
	ContractKey	string		`json:"contract_key"`
	ContractName	string		`json:"contract_name"`
	Status		string		`json:"status"`
	TriggerMode	string		`json:"trigger_mode"`
	Trigger		*chain.Trigger	`json:"trigger"`
	DelaySeconds	int		`json:"delay_seconds"`
	IsEnabled	bool		`json:"is_enabled"`
	Executions	map[string]int	`json:"executions"`
	FindingCount	int		`json:"finding_count"`
}
```

### simulation.TargetView

Sumber: [internal/simulation/service.go](../../internal/simulation/service.go), baris 68.

```go
type TargetView struct {
	ID	uuid.UUID	`json:"id"`
	Name	string		`json:"name"`
}
```

### tags.Tag

Sumber: [internal/tags/tags.go](../../internal/tags/tags.go), baris 23.

```go
type Tag struct {
	ID	uuid.UUID	`json:"id"`
	Name	string		`json:"name"`
	Color	string		`json:"color"`
	Assets	int		`json:"assets"`
}
```

### users.CreateInput

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 136.

```go
type CreateInput struct {
	Email		string	`json:"email"`
	DisplayName	string	`json:"display_name"`
	Role		string	`json:"role"`
}
```

### users.Created

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 142.

```go
type Created struct {
	User		UserRow		`json:"user"`
	ResetLink	ResetLink	`json:"reset_link"`
}
```

### users.NewToken

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 550.

```go
type NewToken struct {
	Token
	Secret	string	`json:"secret"`
}
```

### users.PatchInput

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 205.

```go
type PatchInput struct {
	AcceptRolePermissions	bool	`json:"accept_role_permissions"`
	Version			int	`json:"version"`
	DisplayName		*string	`json:"display_name"`
	Role			*string	`json:"role"`
	Status			*string	`json:"status"`
}
```

### users.ResetLink

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 131.

```go
type ResetLink struct {
	Token		string		`json:"token"`
	ExpiresAt	time.Time	`json:"expires_at"`
}
```

### users.RoleRow

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 97.

```go
type RoleRow struct {
	Key		string		`json:"key"`
	Name		string		`json:"name"`
	Description	*string		`json:"description"`
	Permissions	[]string	`json:"permissions"`
	IsSystem	bool		`json:"is_system"`
	Users		int		`json:"users"`
}
```

### users.Token

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 518.

```go
type Token struct {
	IsDemo		bool		`json:"is_demo"`
	RevokedAt	*time.Time	`json:"revoked_at"`
	ID		uuid.UUID	`json:"id"`
	Name		string		`json:"name"`
	Prefix		string		`json:"prefix"`
	CreatedAt	time.Time	`json:"created_at"`
	LastUsedAt	*time.Time	`json:"last_used_at"`
	ExpiresAt	*time.Time	`json:"expires_at"`
}
```

### users.UserRow

Sumber: [internal/users/users.go](../../internal/users/users.go), baris 59.

```go
type UserRow struct {
	ID			uuid.UUID	`json:"id"`
	Email			string		`json:"email"`
	DisplayName		string		`json:"display_name"`
	RoleKey			string		`json:"role"`
	RoleName		string		`json:"role_name"`
	Status			string		`json:"status"`
	LastLoginAt		*time.Time	`json:"last_login_at"`
	CreatedAt		time.Time	`json:"created_at"`
	Version			int		`json:"version"`
	RoleReviewRequired	bool		`json:"role_review_required"`
	PreviousRoleKey		*string		`json:"previous_role_key"`
	HasPassword		bool		`json:"has_password"`
}
```
