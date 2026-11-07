# API Orchis — Swagger / OpenAPI

Dokumentasi API tersedia sebagai Swagger UI offline dan spesifikasi OpenAPI 3.0.3.
Cakupan: **170 operasi HTTP**, termasuk ekstensi pipeline Nmap/Nuclei → Hackbot di atas UI v2 (`66d3ae4`).

Dari root checkout, jalankan:

```bash
python3 -m http.server 3333 --bind 127.0.0.1 --directory docs/api
```

Buka **http://127.0.0.1:3333/**. Alternatifnya, buka `docs/api/index.html` langsung
melalui browser; spesifikasi sudah disematkan sehingga tidak perlu fetch berkas lokal.

- [OpenAPI YAML](api/openapi.yaml)
- [OpenAPI JSON](api/openapi.json)
- [Panduan dan referensi lengkap](api/API-DOCUMENTATION.md)
- [Indeks endpoint dan role (CSV)](api/endpoints.csv)

Untuk **Try it out**, ubah variabel `baseUrl` di bagian **Servers**, lalu gunakan
**Authorize** untuk token API pengguna atau token mesin yang sesuai. Endpoint pengguna
mendukung cookie sesi, tetapi browser membatasi cookie lintas origin. Server API perlu
mengizinkan origin dokumentasi melalui CORS untuk request lintas origin; hosting
Swagger ini tidak mengubah konfigurasi API. Jangan memasukkan kredensial ke berkas spec.

Role pada tiap operasi merupakan matriks dasar. Service tetap memeriksa permission,
tenant, scope objek, consent, versi objek, dan kondisi pekerjaan. Schema diekstrak dari
model Go; aturan validasi bisnis tidak seluruhnya menjadi atribut schema. Respons yang
belum dipetakan diberi schema bebas/catatan source, bukan bentuk yang ditebak.

Swagger UI **5.34.1** disertakan di `api/swagger-ui/` agar dokumentasi dapat dibaca tanpa
CDN. Lisensi Apache-2.0 dan pemberitahuan dependensi disimpan bersama aset tersebut.
Validator online Swagger dinonaktifkan; tidak ada request API saat membuka dokumentasi.
Panduan konfigurasi upstream: [Swagger UI configuration](https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/).

Spesifikasi JSON/YAML dan `api/openapi-spec.js` harus diperbarui bersamaan bila API berubah.

## Three-stage web pentests

`POST /api/v1/scans` accepts `organization_id`, approved HTTP(S) `asset_ids`, and optional report `language`, with the existing Idempotency-Key header. New requests always create Nmap, Nuclei and Hackbot jobs per asset; omit `tools` or supply exactly all three. The standard preset is applied by the server. Network-range selection is not part of this web pentest flow.

Nmap and Nuclei run first. Hackbot is dispatched only after both finish or fail, and its execution claim includes `scanner_inputs` with immutable findings snapshots from those sibling jobs. Inputs are scoped by tenant, assessment and asset. They cannot be supplied by the browser.

`GET /api/v1/scans/{id}` returns job progress, disposition, the activity `summary` (maximum 300 words), and `report_id` / `report_status` for authorized readers. Status `incomplete` covers partial engine delivery and failed preliminary checks. All three methods and their findings are included in the existing report snapshot. The PDF is rendered and embedded automatically in Orchis through its authenticated same-origin preview endpoint.

The Hackbot workspace receives `inputs/scanners/nmap.json`, `nuclei.json` and `manifest.json` before prompting. These are preliminary observations with explicit provenance and status, not automatically verified findings. See the engine deployment guide for the folder layout and privacy boundaries.

The audited admin-only `/api/v1/scan-jobs/{id}/validation` endpoint returns JSON. Worker `progress` and `retention` POST subresources remain bounded authenticated metadata operations. Existing advanced Nmap/Nuclei contracts and historical assessment APIs remain available.
