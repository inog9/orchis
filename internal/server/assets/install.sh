#!/bin/sh
# ORCHIS agent installer (disajikan server di /install.sh; tidak berisi rahasia).
#
#   sh install.sh --server URL --token TOKEN --sha256 HEX --release-key BASE64 [--ca-fingerprint HEX | --pin sha256//BASE64]
#                 [--name NAMA] [--mode service|user] [--dir DIREKTORI]
#   sh install.sh --uninstall [--dir DIREKTORI] [--purge]
#
# Nilai --token, --sha256, --release-key, dan --pin diambil dari konsol (jalur terpisah dari server yang menyajikan biner).
# Biner hanya dipasang bila SHA-256 cocok DAN tanda tangan Ed25519 (kunci rilis) valid untuk versi/platform/hash itu.
# Verifikasi TLS tidak pernah dimatikan: sertifikat server divalidasi oleh CA sistem, atau, dengan --pin, oleh sertifikat yang SPKI-nya cocok dengan pin.
set -eu
umask 022

die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }
note() { printf 'install.sh: %s\n' "$*" >&2; }

server=""; token=""; want_sha=""; relkey=""; pin=""; name=""; mode="service"; dir=""; uninstall=0; purge=0; cafp=""
while [ $# -gt 0 ]; do
	case "$1" in
	--server) [ $# -ge 2 ] || die "--server needs a value"; server="$2"; shift 2 ;;
	--token) [ $# -ge 2 ] || die "--token needs a value"; token="$2"; shift 2 ;;
	--sha256) [ $# -ge 2 ] || die "--sha256 needs a value"; want_sha="$2"; shift 2 ;;
	--release-key) [ $# -ge 2 ] || die "--release-key needs a value"; relkey="$2"; shift 2 ;;
	--pin) [ $# -ge 2 ] || die "--pin needs a value"; pin="$2"; shift 2 ;;
	--ca-fingerprint) [ $# -ge 2 ] || die "--ca-fingerprint needs a value"; cafp="$2"; shift 2 ;;
	--name) [ $# -ge 2 ] || die "--name needs a value"; name="$2"; shift 2 ;;
	--mode) [ $# -ge 2 ] || die "--mode needs a value"; mode="$2"; shift 2 ;;
	--dir) [ $# -ge 2 ] || die "--dir needs a value"; dir="$2"; shift 2 ;;
	--uninstall) uninstall=1; shift ;;
	--purge) purge=1; shift ;;
	-h | --help) sed -n '2,12p' "$0"; exit 0 ;;
	*) die "unknown option: $1" ;;
	esac
done

case "$name" in
"") ;;
*) printf '%s' "$name" | grep -Eq '^[a-z0-9][a-z0-9_-]{0,39}$' || die "--name must match [a-z0-9][a-z0-9_-]{0,39}" ;;
esac
case "$mode" in service | user) ;; *) die "--mode must be service or user" ;; esac

sys="$(uname -s)"
case "$sys" in
Linux) goos=linux ;;
Darwin) goos=darwin
	note "macOS: osquery endpoint data (used by DefenseBot) is experimental on macOS." ;;
*) die "unsupported OS '$sys'. On Windows use the PowerShell command from the console (osquery on Windows is not yet supported)." ;;
esac

if [ -z "$dir" ]; then
	if [ "$mode" = service ]; then dir="/opt/orchis-agent${name:+-$name}"; else dir="${HOME:?}/orchis-agent${name:+-$name}"; fi
fi

as_root() { if [ "$(id -u)" -eq 0 ]; then "$@"; else command -v sudo >/dev/null 2>&1 || die "root is required (install sudo or run as root)"; sudo "$@"; fi; }

if [ "$uninstall" -eq 1 ]; then
	bin="$dir/orchis-agent"
	[ -d "$dir" ] || die "nothing to uninstall: $dir does not exist"
	if [ -x "$bin" ]; then
		as_root "$bin" stop >/dev/null 2>&1 || true
		as_root "$bin" uninstall >/dev/null 2>&1 || note "service was not registered (or already removed)"
	fi
	if [ "$purge" -eq 1 ]; then
		case "$dir" in "" | / | /opt | /usr | /home | /root) die "refusing to remove '$dir'" ;; esac
		as_root rm -rf -- "$dir"
		note "removed $dir including its configuration and collected data"
	else
		as_root rm -f -- "$bin"
		note "removed the agent binary; configuration and data in $dir were kept (use --purge to delete them)"
	fi
	note "revoke the agent in the console as well if it will not return"
	exit 0
fi

[ -n "$server" ] && [ -n "$token" ] && [ -n "$want_sha" ] && [ -n "$relkey" ] || die "--server, --token, --sha256 and --release-key are required"
printf '%s' "$want_sha" | grep -Eq '^[0-9a-fA-F]{64}$' || die "--sha256 must be 64 hex characters"
server="${server%/}"
case "$server" in
https://*) ;;
http://127.0.0.1 | http://127.0.0.1:* | http://localhost | http://localhost:* | 'http://[::1]' | 'http://[::1]:'*) note "plain HTTP allowed only for a loopback server" ;;
*) die "--server must be https:// (plain http is only accepted for loopback)" ;;
esac

for tool in curl openssl; do command -v "$tool" >/dev/null 2>&1 || die "$tool is required"; done
ossl_major="$(openssl version | sed -n 's/^[A-Za-z]* \([0-9]*\)\..*/\1/p')"
[ "${ossl_major:-0}" -ge 3 ] 2>/dev/null || die "OpenSSL 3 or newer is required for Ed25519 verification ($(openssl version))"
if command -v sha256sum >/dev/null 2>&1; then sha_of() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then sha_of() { shasum -a 256 "$1" | cut -d' ' -f1; }
else die "sha256sum or shasum is required"; fi

case "$(uname -m)" in
x86_64 | amd64) goarch=amd64 ;;
aarch64 | arm64) goarch=arm64 ;;
*) die "unsupported CPU architecture '$(uname -m)'" ;;
esac

tmp="$(mktemp -d)" || die "cannot create a temporary directory"
trap 'rm -rf -- "$tmp"' EXIT INT TERM
chmod 700 "$tmp"

# CA sendiri (instalasi dengan CA internal): ambil /ca.pem TANPA mempercayainya, terima hanya bila SHA-256 DER-nya sama dengan
# fingerprint dari konsol, lalu percayai persis CA itu (bukan -k). Agent juga memakainya (--ca-file) untuk update berikutnya.
cacert=""; agent_ca=""
if [ -n "$cafp" ]; then
	[ -z "$pin" ] || die "use either --ca-fingerprint or --pin, not both"
	case "$server" in https://*) ;; *) die "--ca-fingerprint only applies to https servers" ;; esac
	want_fp="$(printf '%s' "$cafp" | sed 's/^sha256[:/]*//' | tr -d ': ' | tr 'A-F' 'a-f')"
	printf '%s' "$want_fp" | grep -Eq '^[0-9a-f]{64}$' || die "--ca-fingerprint must be the SHA-256 (64 hex characters) of the CA certificate"
	hostport="${server#https://}"; hostport="${hostport%%/*}"
	host="${hostport%%:*}"; port="${hostport##*:}"; [ "$port" = "$hostport" ] && port=443
	printf 'GET /ca.pem HTTP/1.0\r\nHost: %s\r\n\r\n' "$hostport" | openssl s_client -quiet -ign_eof -connect "$host:$port" -servername "$host" 2>/dev/null |
		awk '/-----BEGIN CERTIFICATE-----/{on=1} on{print} /-----END CERTIFICATE-----/{exit}' >"$tmp/ca.pem"
	[ -s "$tmp/ca.pem" ] || die "the server did not provide a CA certificate at /ca.pem"
	got_fp="$(openssl x509 -in "$tmp/ca.pem" -outform DER 2>/dev/null | openssl dgst -sha256 -binary | od -An -v -tx1 | tr -d ' \n')"
	[ "$got_fp" = "$want_fp" ] || die "CA certificate fingerprint does not match (expected $want_fp, got $got_fp): refusing"
	cacert="$tmp/ca.pem"; agent_ca=1
fi

# Pin: pastikan SPKI sertifikat yang dilayani cocok, lalu percayai tepat sertifikat itu (bukan -k).
if [ -n "$pin" ]; then
	case "$server" in https://*) ;; *) die "--pin only applies to https servers" ;; esac
	case "$pin" in sha256//*) ;; *) die "--pin must look like sha256//BASE64" ;; esac
	hostport="${server#https://}"; hostport="${hostport%%/*}"
	host="${hostport%%:*}"; port="${hostport##*:}"; [ "$port" = "$hostport" ] && port=443
	openssl s_client -connect "$host:$port" -servername "$host" </dev/null 2>/dev/null | openssl x509 -outform PEM >"$tmp/server.pem" 2>/dev/null || die "cannot read the server certificate from $host:$port"
	got="sha256//$(openssl x509 -in "$tmp/server.pem" -pubkey -noout | openssl pkey -pubin -outform DER 2>/dev/null | openssl dgst -sha256 -binary | openssl base64 -A)"
	[ "$got" = "$pin" ] || die "server certificate does not match the pin (expected $pin, got $got): refusing"
	cacert="$tmp/server.pem"
fi

# Token lewat berkas header (bukan argumen proses / URL).
umask 077
printf 'Authorization: Bearer %s\n' "$token" >"$tmp/auth"
umask 022
set -- -fsS --proto '=https,http' --max-time 300 -H "@$tmp/auth" -D "$tmp/headers" -o "$tmp/orchis-agent"
[ -n "$cacert" ] && set -- "$@" --cacert "$cacert"
curl "$@" "$server/api/v1/agent/download" || die "download failed (token invalid/expired/already used, no release available, or TLS verification failed)"

header() { grep -i "^$1:" "$tmp/headers" | tail -n 1 | cut -d: -f2- | tr -d '\r' | sed 's/^ *//; s/ *$//'; }
version="$(header X-Release-Version)"; sig_b64="$(header X-Signature-Ed25519)"; hdr_sha="$(header X-Sha256)"
printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "the server sent no valid release version: refusing"
[ -n "$sig_b64" ] || die "the server sent no release signature: refusing"

got_sha="$(sha_of "$tmp/orchis-agent")"
want_lc="$(printf '%s' "$want_sha" | tr 'A-F' 'a-f')"
[ "$got_sha" = "$want_lc" ] || die "SHA-256 mismatch (expected $want_lc, got $got_sha): the file was altered or is not the build shown in the console. Not installing."
[ -z "$hdr_sha" ] || [ "$(printf '%s' "$hdr_sha" | tr 'A-F' 'a-f')" = "$got_sha" ] || die "SHA-256 announced by the server differs from the downloaded file. Not installing."

printf '%s' "$relkey" | openssl base64 -d -A >"$tmp/key.raw" 2>/dev/null || die "--release-key is not valid base64"
[ "$(wc -c <"$tmp/key.raw" | tr -d ' ')" -eq 32 ] || die "--release-key must be a 32-byte Ed25519 public key"
{ printf '\060\052\060\005\006\003\053\145\160\003\041\000'; cat "$tmp/key.raw"; } >"$tmp/key.der"
printf '%s' "$sig_b64" | openssl base64 -d -A >"$tmp/sig.bin" 2>/dev/null || die "the release signature is not valid base64: refusing"
printf 'orchis-agent-release\n%s\n%s/%s\n%s' "$version" "$goos" "$goarch" "$got_sha" >"$tmp/msg"
openssl pkeyutl -verify -pubin -inkey "$tmp/key.der" -keyform DER -rawin -in "$tmp/msg" -sigfile "$tmp/sig.bin" >/dev/null 2>&1 || die "Ed25519 signature is invalid for version $version on $goos/$goarch. Not installing."
note "verified: version $version, SHA-256 $got_sha, signature valid"

if [ "$mode" = service ]; then
	as_root mkdir -p "$dir"
	as_root install -m 0755 "$tmp/orchis-agent" "$dir/orchis-agent"
	run() { as_root "$dir/orchis-agent" "$@"; }
else
	mkdir -p "$dir"
	install -m 0755 "$tmp/orchis-agent" "$dir/orchis-agent"
	run() { "$dir/orchis-agent" "$@"; }
fi

set -- --server "$server" --token "$token"
[ -z "$name" ] || set -- "$@" --name "$name"
if [ -n "$agent_ca" ]; then
	if [ "$mode" = service ]; then as_root install -m 0644 "$cacert" "$dir/ca.pem"; else install -m 0644 "$cacert" "$dir/ca.pem"; fi
	set -- "$@" --ca-file "$dir/ca.pem"
fi
run enroll "$@"
if [ "$mode" = service ]; then
	run install
	run start
	note "agent $version installed as a service in $dir"
else
	# Mode user: layanan tingkat pengguna (LaunchAgent di macOS, systemd --user di Linux), tanpa sudo. Jalan di latar belakang
	# dan hidup lagi saat login. Bila tidak bisa dipasang, agent tetap terdaftar dan pengguna diberi perintah run manual.
	if "$dir/orchis-agent" install >/dev/null 2>&1 && "$dir/orchis-agent" start >/dev/null 2>&1; then
		note "agent $version installed as a background service for this user in $dir (no sudo). It restarts at login."
	else
		note "agent $version enrolled in $dir, but the user service could not be started. Run it in a terminal with: $dir/orchis-agent run"
	fi
fi
