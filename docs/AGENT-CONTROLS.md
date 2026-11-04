# Kontrol agent oleh Admin

Buka **Assets → Agents → Manage**, atau detail organisasi → Endpoints. Admin
melihat versi agent, versi osquery yang dilaporkan setelah pemeriksaan lokal,
OS/arsitektur, kontak terakhir, jeda, status update, dan diagnostik aman. Versi
osquery yang belum diketahui ditampilkan sebagai *not reported*; versi artefak
yang tersedia di server tidak dianggap sebagai bukti pemasangan pada endpoint.
Windows/macOS tetap hanya terbukti ter-build sampai diuji pada perangkat nyata.

## Rollout dan Update now

Bawaan **Automatic for all agents** mempertahankan perilaku lama: setiap agent
mendapat tawaran rilis lebih baru yang cocok dengan platformnya. Pilih **Manual
per agent** sebelum menguji rilis pada satu endpoint, lalu tekan **Update now**
pada agent tersebut. Permintaan mengikat versi manifest saat dikirim: rilis baru
lainnya memerlukan permintaan baru dalam mode manual. Update yang sudah diunduh
atau ditawarkan sebelum perubahan kebijakan masih dapat selesai.

Agent tetap memverifikasi tanda tangan Ed25519, versi/platform/hash, anti-downgrade,
dan smoke test biner sebelum menggantinya. Versi sama/lebih tua ditolak juga oleh
server. Tidak ada pergantian kunci kepercayaan melalui tombol ini. Kegagalan
update mempertahankan biner terpasang dan tampil sebagai kode diagnostik aman.
Pada Unix, exec mengganti proses dengan PID yang sama; agent tidak meminta restart
systemd secara sinkron dari proses yang sedang dihentikan. Windows memakai
kebijakan recovery service dan tetap memerlukan bukti runtime perangkat nyata.

Kebijakan rollout dan kontrol agent memakai versi untuk mencegah perubahan Admin
lain tertimpa. Heartbeat/status worker tidak menaikkan versi kontrol konfigurasi.
Muat ulang state bila terjadi konflik.

## Pause / Resume dan Collect data now

Jeda menghentikan **tugas baru** dan koleksi osquery baru. Tugas yang sudah
berjalan boleh selesai. Server menegakkan jeda pada queue, claim langsung, claim
polling, koleksi periodik, dan koleksi manual; agent lama yang belum memahami
field jeda tetap tidak dapat mengambil tugas. Resume tidak memberikan otorisasi
asset maupun consent baru.

**Collect data now** mengantrekan query tetap yang sudah disetujui dengan dokumen
consent saat ini. Agent harus online, aktif, tidak dijeda, terikat pada asset dan
organisasi yang sama, serta asset tetap diotorisasi. Koleksi endpoint saat ini
hanya didukung Linux amd64. Query yang sudah antre/berjalan dipakai ulang;
pengiriman ulang tidak menggandakan query pending. Semua tugas tetap melewati
pemeriksaan claim/result yang ada. Status queued tidak membuktikan hasil sukses.

## Revoke dan Uninstall remotely

**Revoke** menolak kredensial segera dan membatalkan pekerjaan agent. Agent yang
kembali menghubungi server menghapus kredensialnya dan berhenti bekerja. Token
pendaftaran lama tetap sekali pakai. Revoke tidak menjamin berkas endpoint dihapus.

**Uninstall remotely** memerlukan ID agent yang diketik persis dan capability
kontrol yang benar-benar dilaporkan agent. Biner lama perlu diperbarui dahulu.
Windows belum mengiklankan remote removal; penghapusan dilakukan lokal sampai
helper Windows dan bukti perangkat tersedia.

Perintah tetap hanya `uninstall`, terikat pada UUID command dan identitas agent,
melalui kanal bearer agent yang sama dengan tugas. Tidak ada shell, SQL, flag,
atau path dari server. Receipt privat mode 0600 disimpan sebelum helper dimulai.
Restart melanjutkan receipt pending. Pada service Linux, helper berjalan pada unit
systemd sementara di luar cgroup agent agar tetap hidup saat service agent
berhenti. Foreground memakai proses detached; service pengguna memakai manager
pengguna yang sama. Uninstall antar instance tidak berbagi nama service/config.

Helper menghentikan dan unregister service sendiri, menghapus biner sendiri,
rollback `.old`, konfigurasi/kredensial, dan biner osquery di direktori versi
privat. Direktori symlink ditolak; direktori induk, file tak terkait, paket osquery
sistem, dan instance saudara dipertahankan. Direktori kosong saja dibuang.
Asset, findings, dan laporan di server tetap ada.

Server menampilkan queued/dispatched sampai agent **mengakui setelah penghapusan
fisik**. ACK hanya berlaku untuk agent dan command yang sesuai. ACK sukses
mencabut kredensial dan membatalkan sisa tugas secara atomik. Agent offline tetap
pending hingga kontak berikutnya. Tidak adanya kontak bukan bukti uninstall.
Bila berkas sudah terhapus tetapi ACK hilang, server tetap menampilkan belum
terkonfirmasi; verifikasi lokal diperlukan. Helper mencoba ACK terbatas dengan
kredensial dalam memori dan tidak menulis ulang kredensial setelah removal sukses.
Kegagalan ditampilkan sebagai failed, dapat membutuhkan perbaikan lokal sebelum
Admin mencoba ulang.

## Izin, audit, dan diagnostik

Kontrol memerlukan `settings.manage` serta `agent.manage`; Collect now juga
memeriksa izin asset, organisasi, dan menjalankan tugas. Analyst/Viewer tidak
mendapat kontrol Admin. Seluruh operasi per tenant, termasuk RLS kebijakan
rollout. Perintah, perubahan kebijakan, koleksi, dan ACK dicatat di audit.

Diagnostik hanya menerima kode tetap: update gagal, kunci rilis tidak ada,
koleksi gagal, uninstall gagal, atau uninstall tidak didukung. UI memetakan kode
ke penjelasan aman. Log mentah, path, respons server, dan field legacy
`last_update_error` tidak disimpan. Untuk investigasi detail gunakan log lokal
dengan kewenangan operator endpoint.

## API

- `GET/PUT /api/v1/agents/rollout`: mode dan CAS version.
- `POST /api/v1/agents/{id}/control`: action update/pause/resume/uninstall, CAS
  version; uninstall juga confirmation berupa ID yang persis.
- `POST /api/v1/agents/{id}/collect`: koleksi query yang sudah disetujui.
- `POST /api/v1/agent/control/ack`: bearer agent saja, UUID command dan hasil.

Operasi konsol memakai CSRF yang sama dengan API lain. Pembacaan daftar agent
mengikuti izin baca agent; field kontrol bukan izin menjalankan perintah.
