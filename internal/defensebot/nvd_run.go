package defensebot

import (
	"context"
	"errors"
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
	for _, c := range p.CPE {
		if seen[c] {
			continue
		}
		seen[c] = true
		sum.SoftwareChecked++
		vulns, err := d.NVD.Query(ctx, c.Vendor, c.Product, c.Version)
		if err != nil {
			code := "source_down"
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				code = "timeout"
			}
			sum.Notes = append(sum.Notes, Note{Code: code, Detail: "NVD", Limiting: true})
			break
		}
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
