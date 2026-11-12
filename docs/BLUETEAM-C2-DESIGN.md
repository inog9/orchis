# Desain Fitur: Deteksi Malware / C2 via Koneksi Keluar Device (orchis-blueteam-agent)

> Status: riset + rancangan (branch `feat/blueteam-c2-detection`, belum di-push).
> Ruang lingkup: macOS, Linux, Windows.

## 1. Tujuan
Mendeteksi indikasi malware/C2 dari **koneksi jaringan KELUAR (outbound)** yang dibuat proses di device,
kemudian melaporkannya sebagai finding yang bisa ditindaklanjuti — tanpa packet capture (fase 1),
sehingga ringan dan privasi-aman.

## 2. Kenapa kolektor native (bukan sekadar osquery)
osquery di sini hanya punya `listening_ports` (port masuk), **tidak ada tabel koneksi outbound yang portabel**.
`socket_events` ada tapi event-based dan tidak konsisten antar-OS. Karena agent kita binary Go native
(sudah ada pola `//go:build` per-OS di `internal/agent`), kolektor koneksi paling andal dibuat native per-OS.

## 3. Sumber data per-OS (kolektor `connections`)
Skema baris seragam (jsonb, sama seperti dataset lain):
`{pid, ppid, process, path, user, proto, src_ip, src_port, dst_ip, dst_port, state, started_at}`

| OS | Sumber utama | Catatan |
|----|--------------|---------|
| Linux | `/proc/net/tcp{,6}`, `/proc/net/udp{,6}` + mapping inode→pid via `/proc/<pid>/fd` | agent jalan sebagai service/root → bisa baca semua proses; fallback `ss -tnp` |
| macOS | `lsof -nP -iTCP -iUDP` (atau sysctl `net.inet.tcp.pcblist`/`udp.pcblist` sebagai root) | path proses via `proc_pidinfo`; tanda tangan via `codesign -dv` (fase 2) |
| Windows | `GetExtendedTcpTable`/`GetExtendedUdpTable` (iphlpapi via `golang.org/x/sys/windows`) + snapshot proses (toolhelp) untuk nama/path | jalan sebagai service; tanda tangan via WinVerifyTrust (fase 2) |

Daya proses (ppid, path) di-enrich dari tabel proses (osquery `processes` atau native) — untuk deteksi
"browser/office meng-spawn proses jaringan aneh".

Batas: `max_rows` (mis. 500) + agregasi per `(process, dst_ip, dst_port)` dengan count; snapshot tiap N detik
(mis. 60s) + bisa mode delta.

## 4. Transport & penyimpanan (reuse pipeline yang ada)
- Query key baru di katalog/pipeline: `blueteam.connections` (kategori baru `Blueteam`, sensitivitas **high**).
- Consent: item consent baru (auto-consent saat agent enroll, sama seperti osquery); bisa dimatikan per-org.
- Tersimpan di `endpoint_datasets` (rows jsonb) — tanpa ubahan skema.
- Redaksi: IP lokal diganti `"local"`; dst IP publik disimpan (diperlukan untuk reputasi); **tidak ada payload/isi koneksi** (fase 1).
- Retensi mengikuti aturan dataset yang sudah ada.

## 5. Engine analisis server-side: `blueteam.analyze` (kontrak baru)
Deterministik dulu (tanpa AI), berjalan seperti defensebot dari data tersimpan:

1. **Known-bad destination**: blocklist IP/CIDR/domain yang bisa dikonfigurasi + opsional AbuseIPDB/URLhaus
   (API key, rate-limit seperti NVD; 404/empty = tidak ada data, bukan error).
2. **Beaconing**: per `(asset, process, dst)` — hitung jumlah koneksi + keteraturan interval
   (stddev inter-arrival rendah + count tinggi dalam window) → finding `c2_beacon`.
3. **Port non-standar**: dst_port di luar {53,80,443,853,123,993,995} + koneksi長-lived → `c2_suspect_port`.
4. **Anomali proses**: path executable di direktori temporari (`/tmp`, `/var/folders`, `Downloads`, `%TEMP%`),
   parent-nya browser/office yang spawn proses jaringan → `malware_process`.
5. **Baseline per aset**: dst/ASN/negara yang baru pertama kali terlihat untuk aset itu (butuh data historis) → `new_outbound_dest`.
6. Fase 2 (opsional): log DNS (untuk DGA heuristic: entropy, panjang, rasio vokal, TLD langka), JA3/JA3S (butuh
   libpcap), tanda tangan kode (codesign/WinVerifyTrust).

MITRE: mapping `blueteam.analyze` → T1071 (C2 Application Layer), T1571 (Non-Standard Port), T1568 (Dynamic Resolution),
T1105 (Ingress Tool Transfer) via `DefaultLinks` — otomatis muncul di `/mitre` dan laporan.

## 6. Output / surfaces
- Finding type baru (`c2_beacon`, `c2_suspect_ip`, `c2_suspect_port`, `malware_process`, `new_outbound_dest`)
  dengan evidence (pid, process, path, dst, port, count, interval stats) — muncul otomatis di daftar findings,
  laporan (blok MITRE + tabel), dan comparison.
- Halaman `/mitre` otomatis menampilkan teknik blueteam (mapped/exercised setelah scan sukses).
- Overview (peta): fase berikutnya bisa menampilkan koneksi sebagai garis dari aset ke tujuan (mirip arc yang sudah ada).

## 7. Privasi & keamanan
- Consent eksplisit per-org; data sensitif (high) ikut aturan retensi & redaction yang sudah ada.
- Tidak ada isi koneksi/payload; hanya metadata 5-tuple + proses.
- Agent sendiri bisa terdeteksi AV — perlu pengecualian/signing (sudah ada proses agent-release signing).

## 8. Rencana bertahap
- **P1**: kolektor `connections` 3 OS + simpan dataset + engine aturan 1–4 + finding + MITRE + consent. 
- **P2**: DNS log/DGA, JA3, signature status, baseline geo/ASN.
- **P3 (opsional)**: triage AI untuk finding blueteam (jalur ai_jobs yang sudah ada, tetap perlu approval/auto-approve).

## 9. Risiko
- Privilege: butuh root/admin untuk memetakan semua koneksi ke proses (kalau tidak, hanya koneksi milik sendiri).
- False positive pada beacon (software update, telemetry) — perlu threshold & allowlist per-organisasi.
- Performa: scan /proc bisa berat pada server dengan ribuan koneksi — pakai sampling + batas waktu.
- Privasi: data koneksi sangat sensitif — jaga redaction, retensi, dan jangan log payload.
