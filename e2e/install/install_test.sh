#!/bin/bash
# Tes install.sh terhadap server tiruan: pemasangan sah, biner diubah, tanda tangan/kunci salah, pin, http non-loopback, uninstall.
# Pakai INSTALL_SH=/jalur/install.sh untuk menguji varian lain (mis. mutan).
set -u
here="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="${INSTALL_SH:-$here/../../internal/server/assets/install.sh}"
W="$(mktemp -d)"; pids=()
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null; done; rm -rf "$W"; }
trap cleanup EXIT
fail=0
ok() { echo "LULUS: $*"; }
bad() { echo "GAGAL: $*"; fail=1; }
expect_ok() { local n="$1"; shift; if "$@" >"$W/out" 2>&1; then ok "$n"; else bad "$n"; sed 's/^/    /' "$W/out"; fi; }
expect_refused() { local n="$1" pat="$2"; shift 2; if "$@" >"$W/out" 2>&1; then bad "$n: seharusnya ditolak"; else if grep -q -- "$pat" "$W/out"; then ok "$n"; else bad "$n: ditolak tapi alasan tak sesuai"; sed 's/^/    /' "$W/out"; fi; fi; }

mkkey() { openssl genpkey -algorithm ed25519 -out "$1.pem" 2>/dev/null; openssl pkey -in "$1.pem" -pubout -outform DER | tail -c 32 | openssl base64 -A >"$1.pub"; }
sign() { # key version sha -> signature base64
	printf 'orchis-agent-release\n%s\nlinux/amd64\n%s' "$2" "$3" >"$W/msg"
	openssl pkeyutl -sign -inkey "$1.pem" -rawin -in "$W/msg" 2>/dev/null | openssl base64 -A
}
mkkey "$W/rel"; mkkey "$W/evil"
RELKEY="$(cat "$W/rel.pub")"
cat >"$W/agent" <<'AGENT'
#!/bin/sh
echo "$@" >>"$(dirname "$0")/calls.log"
AGENT
chmod +x "$W/agent"
GOOD_SHA="$(sha256sum "$W/agent" | cut -d' ' -f1)"

serve() { # dir port [cert key]
	python3 -I "$here/mock_server.py" "$@" & pids+=($!)
	for _ in $(seq 50); do curl -ks -o /dev/null "http://127.0.0.1:$2/" 2>/dev/null && return; curl -ks -o /dev/null "https://127.0.0.1:$2/" 2>/dev/null && return; sleep 0.1; done
}
publish() { # dir : writes a valid release into dir
	mkdir -p "$1"; cp "$W/agent" "$1/bin"; echo 9.9.9 >"$1/ver"; echo "$GOOD_SHA" >"$1/sha"; sign "$W/rel" 9.9.9 "$GOOD_SHA" >"$1/sig"
}
run() { sh "$SCRIPT" --server "$SERVER" --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user "$@"; }

D1="$W/rel1"; publish "$D1"; serve "$D1" 18391; SERVER=http://127.0.0.1:18391

# 1. Pemasangan sah
expect_ok "pemasangan sah" run --dir "$W/inst1" --name uji
[ -x "$W/inst1/orchis-agent" ] && grep -q -- "enroll --server $SERVER --token oen_testtoken --name uji" "$W/inst1/calls.log" && ok "agent terdaftar dengan argumen benar" || bad "enroll tidak dipanggil"

# 2. Biner diubah di jalan (hash konsol tetap yang asli)
D2="$W/rel2"; publish "$D2"; printf 'X' >>"$D2/bin"; serve "$D2" 18392
SERVER=http://127.0.0.1:18392 expect_refused "biner diubah ditolak" "SHA-256 mismatch" run --dir "$W/inst2"
[ ! -e "$W/inst2/orchis-agent" ] && ok "biner yang diubah tidak terpasang" || bad "biner yang diubah terpasang"

# 3. Tanda tangan rusak (hash cocok)
D3="$W/rel3"; publish "$D3"; sign "$W/evil" 9.9.9 "$GOOD_SHA" >"$D3/sig"; serve "$D3" 18393
SERVER=http://127.0.0.1:18393 expect_refused "tanda tangan dari kunci lain ditolak" "signature is invalid" run --dir "$W/inst3"
[ ! -e "$W/inst3/orchis-agent" ] && ok "tanpa tanda tangan sah tidak terpasang" || bad "terpasang meski tanda tangan salah"

# 4. Tanda tangan untuk versi lain (replay versi lama)
D4="$W/rel4"; publish "$D4"; sign "$W/rel" 1.0.0 "$GOOD_SHA" >"$D4/sig"; serve "$D4" 18394
SERVER=http://127.0.0.1:18394 expect_refused "tanda tangan versi lain ditolak" "signature is invalid" run --dir "$W/inst4"

# 5. Kunci rilis di konsol berbeda
SERVER=http://127.0.0.1:18391 expect_refused "kunci rilis salah ditolak" "signature is invalid" sh "$SCRIPT" --server http://127.0.0.1:18391 --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$(cat "$W/evil.pub")" --mode user --dir "$W/inst5"

# 6. http bukan loopback ditolak sebelum jaringan
expect_refused "http non-loopback ditolak" "must be https" sh "$SCRIPT" --server http://example.invalid --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst6"

# 7. HTTPS self-signed: tanpa pin gagal (tidak ada -k), pin benar sukses, pin salah ditolak
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "$W/tls.key" -out "$W/tls.crt" -days 2 -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" 2>/dev/null
PIN="sha256//$(openssl x509 -in "$W/tls.crt" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -binary | openssl base64 -A)"
D7="$W/rel7"; publish "$D7"; serve "$D7" 18397 "$W/tls.crt" "$W/tls.key"
expect_refused "self-signed tanpa pin ditolak (TLS tetap diverifikasi)" "download failed" sh "$SCRIPT" --server https://127.0.0.1:18397 --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst7a"
expect_ok "pin benar menerima sertifikat self-signed" sh "$SCRIPT" --server https://127.0.0.1:18397 --pin "$PIN" --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst7b"
expect_refused "pin salah ditolak" "does not match the pin" sh "$SCRIPT" --server https://127.0.0.1:18397 --pin "sha256//AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst7c"

# 7b. CA sendiri (seperti Caddy internal CA): fingerprint CA dari konsol; agent diberi --ca-file
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "$W/ca.key" -out "$W/ca.crt" -days 2 -subj "/CN=Uji CA" -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -keyout "$W/leaf.key" -out "$W/leaf.csr" -subj "/CN=localhost" 2>/dev/null
printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n' >"$W/leaf.ext"
openssl x509 -req -in "$W/leaf.csr" -CA "$W/ca.crt" -CAkey "$W/ca.key" -CAcreateserial -out "$W/leaf.crt" -days 2 -extfile "$W/leaf.ext" 2>/dev/null
CAFP="$(openssl x509 -in "$W/ca.crt" -outform DER | openssl dgst -sha256 -binary | od -An -v -tx1 | tr -d ' \n')"
D7B="$W/rel7b"; publish "$D7B"; cp "$W/ca.crt" "$D7B/ca.pem"; serve "$D7B" 18398 "$W/leaf.crt" "$W/leaf.key"
CASRV=https://127.0.0.1:18398
expect_refused "CA tanpa fingerprint ditolak (TLS tetap diverifikasi)" "download failed" sh "$SCRIPT" --server $CASRV --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst7d"
expect_ok "fingerprint CA benar menerima server ber-CA sendiri" sh "$SCRIPT" --server $CASRV --ca-fingerprint "sha256:$CAFP" --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst7e"
[ -s "$W/inst7e/ca.pem" ] && grep -q -- "--ca-file $W/inst7e/ca.pem" "$W/inst7e/calls.log" && ok "CA disimpan di direktori agent dan diteruskan ke enroll --ca-file" || bad "ca.pem/--ca-file tidak ada"
expect_refused "fingerprint CA salah ditolak" "fingerprint does not match" sh "$SCRIPT" --server $CASRV --ca-fingerprint "$(printf '0%.0s' $(seq 64))" --token oen_testtoken --sha256 "$GOOD_SHA" --release-key "$RELKEY" --mode user --dir "$W/inst7f"
[ ! -e "$W/inst7f/orchis-agent" ] && ok "CA tak dikenal: tidak ada yang terpasang" || bad "terpasang meski fingerprint CA salah"

# Mode user memasang layanan tingkat pengguna tanpa sudo; bila gagal, pengguna diberi perintah run manual.
expect_ok "user mode installs a background service" run --mode user --dir "$W/user-instructions"
grep -qx "install" "$W/user-instructions/calls.log" && grep -qx "start" "$W/user-instructions/calls.log" && grep -Fq "background service for this user" "$W/out" && ok "user mode runs install and start without sudo" || bad "user mode did not install/start the service"

# 8. Mode service (uji sebagai root) dan token tidak muncul di URL
expect_ok "mode service memanggil install dan start" run --mode service --dir "$W/inst8"
grep -qx "install" "$W/inst8/calls.log" && grep -qx "start" "$W/inst8/calls.log" && ok "service didaftarkan lalu dijalankan" || bad "install/start tidak dipanggil"

# 9. Uninstall: tanpa --purge data tetap; dengan --purge hilang
echo keep >"$W/inst1/agent.json"
expect_ok "uninstall" sh "$SCRIPT" --uninstall --dir "$W/inst1"
[ ! -e "$W/inst1/orchis-agent" ] && [ -e "$W/inst1/agent.json" ] && grep -qx stop "$W/inst1/calls.log" && grep -qx uninstall "$W/inst1/calls.log" && ok "uninstall menghentikan layanan, menghapus biner, menyimpan data" || bad "perilaku uninstall salah"
expect_ok "uninstall --purge" sh "$SCRIPT" --uninstall --purge --dir "$W/inst1"
[ ! -e "$W/inst1" ] && ok "--purge menghapus direktori" || bad "--purge tidak menghapus"
# Direktori berbahaya: rm diganti pencatat agar tes (termasuk mutan tanpa pengaman) TIDAK PERNAH menghapus apa pun yang nyata.
mkdir -p "$W/fakebin"; printf '#!/bin/sh\necho "$@" >>"%s/rm.log"\n' "$W" >"$W/fakebin/rm"; chmod +x "$W/fakebin/rm"
for danger in /usr / /root /home; do
	PATH="$W/fakebin:$PATH" expect_refused "purge menolak direktori berbahaya $danger" "refusing" sh "$SCRIPT" --uninstall --purge --dir "$danger"
done
[ ! -s "$W/rm.log" ] && ok "rm tidak pernah dipanggil untuk direktori berbahaya" || bad "rm dipanggil: $(cat "$W/rm.log")"
exit $fail
