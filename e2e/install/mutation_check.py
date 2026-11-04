#!/usr/bin/env python3
"""Mutan install.sh: setiap pengaman dimatikan satu per satu; install_test.sh HARUS gagal untuk tiap mutan.
Kasus direktori berbahaya di install_test.sh memakai rm tiruan (pencatat), jadi mutan purge-guard tidak menghapus apa pun yang nyata."""
import os, subprocess, sys, tempfile
here = os.path.dirname(os.path.abspath(__file__))
src = open(os.path.join(here, "../../internal/server/assets/install.sh")).read()
M = {
 "sha256-check": ('[ "$got_sha" = "$want_lc" ] || die', 'true || die'),
 "signature-verify": ('openssl pkeyutl -verify -pubin -inkey "$tmp/key.der" -keyform DER -rawin -in "$tmp/msg" -sigfile "$tmp/sig.bin" >/dev/null 2>&1 ||', 'true ||'),
 "signature-binds-version": ("printf 'orchis-agent-release\\n%s\\n%s/%s\\n%s' \"$version\" \"$goos\"", "printf 'orchis-agent-release\\n1.0.0\\n%s/%s\\n%s' \"$goos\""),
 "pin-compare": ('[ "$got" = "$pin" ] || die', 'true || die'),
 "ca-fingerprint-compare": ('[ "$got_fp" = "$want_fp" ] || die', 'true || die'),
 "https-only": ('*) die "--server must be https://', '*) true #'),
 "purge-guard": ('case "$dir" in "" | / | /opt | /usr | /home | /root) die "refusing to remove', 'case "$dir" in "") die "refusing to remove'),
 "uninstall-stops-service": ('as_root "$bin" stop >/dev/null 2>&1 || true', 'true'),
 "insecure-tls": ('set -- -fsS --proto', 'set -- -kfsS --proto'),
}
bad = 0
for name, (old, new) in M.items():
    assert src.count(old) == 1, f"{name}: pola tidak unik/ada ({src.count(old)})"
    with tempfile.TemporaryDirectory() as d:
        p = os.path.join(d, "install.sh"); open(p, "w").write(src.replace(old, new))
        r = subprocess.run(["bash", os.path.join(here, "install_test.sh")], env={**os.environ, "INSTALL_SH": p}, capture_output=True, text=True)
    killed = r.returncode != 0
    print(("MATI    " if killed else "HIDUP!  ") + name)
    bad += not killed
sys.exit(1 if bad else 0)
