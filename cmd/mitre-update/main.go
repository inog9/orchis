// Perintah mitre-update membekukan bundel ATT&CK Enterprise ke deploy/mitre (lihat internal/mitre/freeze.go).
//
//	mitre-update -version 20.0           unduh, validasi, tampilkan diff, tulis bundel + manifest
//	mitre-update -verify                 periksa bundel beku terhadap manifest (hash, versi, hitungan)
//	mitre-update -version 20.0 -from f   pakai berkas lokal (mis. tanpa internet)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"orchis-go/internal/mitre"
)

func main() {
	dir := flag.String("dir", "deploy/mitre", "direktori bundel beku")
	version := flag.String("version", "", "versi ATT&CK, mis. 20.0")
	from := flag.String("from", "", "berkas bundel lokal (menggantikan unduhan)")
	verify := flag.Bool("verify", false, "hanya verifikasi bundel beku terhadap manifest")
	flag.Parse()
	if *verify {
		m, ds, err := mitre.Verify(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "GAGAL:", err)
			os.Exit(1)
		}
		c := ds.Count()
		fmt.Printf("OK: ATT&CK Enterprise %s sha256=%s taktik=%d teknik=%d sub-teknik=%d deprecated=%d revoked-dilewati=%d\n", m.Version, m.SHA256, c.Tactics, c.Parents, c.Subs, c.Deprecated, ds.Revoked)
		return
	}
	m, _, err := mitre.Update(context.Background(), *dir, *version, *from, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "GAGAL:", err)
		os.Exit(1)
	}
	fmt.Printf("Dibekukan: ATT&CK Enterprise %s (%d byte) sha256=%s\n  taktik=%d teknik=%d sub-teknik=%d deprecated=%d revoked-dilewati=%d\n", m.Version, m.Bytes, m.SHA256,
		m.Counts.Tactics, m.Counts.Techniques, m.Counts.Subtechniques, m.Counts.Deprecated, m.Counts.RevokedSkipped)
}
