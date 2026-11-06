# API Orchis — Swagger / OpenAPI

Dokumentasi API tersedia sebagai Swagger UI offline dan spesifikasi OpenAPI 3.0.3.
Cakupan: **167 operasi HTTP**, berdasarkan source commit `84392fc`.

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
