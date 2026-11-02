# Snapshot laporan Orchis

Snapshot Scan baru memakai `schema_version: "orchis.report.v1"`. JSON ini adalah
sumber halaman cetak/PDF, bukan pembacaan ulang inventaris setelah scan. Snapshot
lama tanpa `schema_version` tetap format legacy; tidak dimigrasikan atau ditulis ulang.
Penambahan field opsional kompatibel. Perubahan arti/tipe field wajib versi baru.

| Field | Tipe | Arti |
|---|---|---|
| schema_version | string | `orchis.report.v1` untuk Scan baru |
| scan_id | UUID | Identitas Scan, hanya pada laporan Scan |
| assessment_id | UUID | ID kelompok hasil internal; sama dengan scan_id pada Scan |
| organization | string | Nama organisasi yang dibekukan dan diredaksi |
| profile | string | Preset `quick` atau `standard` pada Scan baru |
| created_at | RFC3339 UTC | Waktu peluncuran, bukan waktu unduhan PDF |
| status | string | `finished` atau `canceled`; tidak berarti seluruh tool berhasil |
| risk | string | Risiko dari severity kerentanan tertinggi; kegagalan bukan aman |
| by_severity | object integer | Jumlah vulnerability critical/high/medium/low/info |
| assets | array | Sasaran yang digabung per aset/alamat; definisi di bawah |
| endpoint_summary | object opsional | Jumlah sasaran dengan postur endpoint yang disetujui |
| comparison | string | Perbandingan hanya untuk metode/scope sebanding dan berhasil |
| remediation | string | Arahan untuk meninjau dan memvalidasi remediasi |
| is_demo, label | boolean/string opsional | Provenance sintetis yang tampil juga pada PDF |

Setiap `assets[]` berisi `id`, `name`, `status`, `skip_reason`, `findings`, `methods`
dan `endpoint` opsional. `discovered: true` berarti ID deterministik sasaran laporan,
bukan FK aset inventaris. Menambah host ke inventaris tidak mengubah snapshot ini.
`methods[]` memuat action/version/params/target/outcome/error/started_at/ended_at;
parameter merupakan hasil normalisasi skema, bukan argumen CLI pengguna.

`findings[]` berisi id/type/value/title/severity/evidence dan MITRE opsional. Evidence
hanya mengizinkan template_id/template/matcher_name/matched_at/port/protocol/service/
product/version/cve/cwe/mitre dengan nilai skalar. String dipotong dan diredaksi;
response, request, raw output, nested objects dan kredensial tidak otomatis disalin.
Semua teks di-escape saat dicetak. Pemetaan MITRE perkiraan tidak mengklaim eksploitasi.

Laporan otomatis dibekukan sekali setelah semua job terminal. Perubahan inventaris,
hasil berikutnya, atau retry worker PDF tidak mengganti JSON ini. Status rendering
`queued/rendering/ready/failed` ada pada envelope `/reports/{id}`, terpisah dari
`status` snapshot di atas. Ekspor JSON adalah snapshot; envelope memiliki id,
organization_id, assessment_id, version, status, error, created_at dan snapshot.

Ekspor publik hanya `/api/v1/reports/{id}/export/pdf` dan `/export/json`. HTML/CSV
mengembalikan404. `/export/pdf?preview=1` membuka PDF inline; ini memakai snapshot
dan renderer yang sama. Suplemen AI pada mode lanjutan juga hanya mengekspor PDF/JSON.
