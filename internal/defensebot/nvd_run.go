package defensebot

import (
	"context"
	"errors"
	"time"
)

// nvdRetryRounds/nvdRetryPause: putaran ulang untuk produk yang gagal diambil dari NVD.
const (
	nvdRetryRounds = 2
	nvdRetryPause  = 15 * time.Second
)

// nvdCandidates looks up software that is in the reviewed CPE dictionary. A failing NVD never produces a clean result:
// the host is marked incomplete with a note.
func nvdCandidates(ctx context.Context, p *plan, d Deps, sum *Summary) []*candidate {
	if len(p.CPE) == 0 {
		return nil
	}
	if d.NVD == nil {
		sum.Notes = append(sum.Notes, Note{Code: "source_down", Detail: "NVD", Limiting: true})
		return nil
	}
	sum.Provenance.Sources = appendUnique(sum.Provenance.Sources, "NVD (nvd.nist.gov)")
	var out []*candidate
	seen := map[cpeComp]bool{}
	handle := func(c cpeComp, vulns []NVDVuln) {
		comp := &component{Pair: Pair{Ecosystem: "cpe:" + c.Vendor + ":" + c.Product, Name: c.Name, Version: c.Version}, Display: c.Name}
		for _, v := range vulns {
			status := statusFix
			if v.Fixed == "" {
				status = statusNoFix
			}
			adv := &Advisory{ID: v.CVE, CVEs: []string{v.CVE}, Summary: v.Summary, Severity: v.Severity, SeveritySource: v.Source, CVSS: v.Score, URL: v.URL}
			out = append(out, &candidate{comp: comp, adv: adv, key: comp.Pair.Ecosystem, cve: v.CVE, fixed: v.Fixed, status: status, method: MethodCPE, severity: v.Severity, source: v.Source, ids: []string{v.CVE}})
		}
	}
	var failed []cpeComp
	var lastErr error
	for _, c := range p.CPE {
		if seen[c] {
			continue
		}
		seen[c] = true
		sum.SoftwareChecked++
		vulns, err := d.NVD.Query(ctx, c.Vendor, c.Product, c.Version)
		if err != nil {
			lastErr = err
			// Satu produk yang gagal (NVD sering 503 sesaat) tidak boleh menggugurkan sisanya; hanya berhenti bila
			// sumber benar-benar mati (circuit breaker terbuka) atau konteks habis.
			failed = append(failed, c)
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, ErrSourceDown) && d.NVD.breakerOpen() {
				break
			}
			continue
		}
		handle(c, vulns)
	}
	// Coba ulang yang gagal di akhir (dua putaran, jeda makin panjang): 503 NVD biasanya sesaat. Hasil yang sudah berhasil
	// tersimpan di cache, jadi putaran ulang hanya menyentuh produk yang gagal.
	for round := 1; round <= nvdRetryRounds && len(failed) > 0 && ctx.Err() == nil; round++ {
		pause := time.Duration(round) * nvdRetryPause
		if d.NVD.Sleep != nil {
			d.NVD.Sleep(pause)
		} else {
			time.Sleep(pause)
		}
		var still []cpeComp
		for _, c := range failed {
			vulns, err := d.NVD.Query(ctx, c.Vendor, c.Product, c.Version)
			if err != nil {
				lastErr = err
				still = append(still, c)
				continue
			}
			handle(c, vulns)
		}
		failed = still
	}
	if len(failed) > 0 {
		code := "source_down"
		if errors.Is(lastErr, context.DeadlineExceeded) || errors.Is(lastErr, context.Canceled) {
			code = "timeout"
		}
		sum.Notes = append(sum.Notes, Note{Code: code, Detail: "NVD", Limiting: true})
	}
	// the same note must not repeat for every CPE component
	seenNote := map[string]bool{}
	notes := sum.Notes[:0]
	for _, n := range sum.Notes {
		k := n.Code + "|" + n.Detail
		if n.Limiting && seenNote[k] {
			continue
		}
		seenNote[k] = true
		notes = append(notes, n)
	}
	sum.Notes = notes
	return out
}
