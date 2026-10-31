package reporting

// CoverageCounts diturunkan dari snapshot beku, bukan inventaris yang bisa berubah.
type CoverageCounts struct{ Total, Tested, Skipped, Incomplete int }

func Coverage(assets []Asset) CoverageCounts {
	c := CoverageCounts{Total: len(assets)}
	for _, a := range assets {
		if a.Status == "skipped" || a.SkipReason != "" {
			c.Skipped++
			continue
		}
		complete := a.Status == "finished" && len(a.Methods) > 0
		for _, m := range a.Methods {
			if m.Outcome != "success" {
				complete = false
			}
		}
		if complete {
			c.Tested++
		} else {
			c.Incomplete++
		}
	}
	return c
}
func exposureRisk(s Snapshot) string {
	for _, v := range []struct{ severity, label string }{{"critical", "High exposure"}, {"high", "Elevated exposure"}, {"medium", "Moderate exposure"}, {"low", "Low exposure"}, {"info", "Low exposure"}} {
		if s.BySeverity[v.severity] > 0 {
			return v.label
		}
	}
	if Coverage(s.Assets).Tested > 0 {
		return "No observed vulnerabilities"
	}
	return "No completed scan coverage"
}
