package retention

import "testing"

func TestPolicyBoundsForEveryDataKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		min  int
		set  func(*Policy, int)
	}{
		{"findings", 1, func(p *Policy, v int) { p.FindingsDays = v }}, {"scans", 1, func(p *Policy, v int) { p.ScanDays = v }}, {"reports", 1, func(p *Policy, v int) { p.ReportDays = v }}, {"endpoint", 1, func(p *Policy, v int) { p.EndpointDays = v }}, {"audit", 90, func(p *Policy, v int) { p.AuditDays = v }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, valid := range []int{tc.min, 3650} {
				p := Defaults()
				tc.set(&p, valid)
				if err := p.Validate(); err != nil {
					t.Fatal("valid retention boundary rejected", err)
				}
			}
			for _, invalid := range []int{tc.min - 1, 3651} {
				p := Defaults()
				tc.set(&p, invalid)
				problem(t, p.Validate(), 422)
			}
		})
	}
	p := Defaults()
	p.Version = -1
	problem(t, p.Validate(), 422)
}
