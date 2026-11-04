package scans

import "testing"

func TestEffectiveStatus(t *testing.T) {
	failed, ok := Job{Status: "failed"}, Job{Status: "finished"}
	cases := []struct {
		status string
		jobs   []Job
		want   string
	}{
		{"finished", []Job{failed, failed}, "failed"},
		{"finished", []Job{failed, ok}, "finished"},
		{"finished", []Job{ok}, "finished"},
		{"finished", nil, "finished"},
		{"running", []Job{failed}, "running"},
		{"canceled", []Job{failed}, "canceled"},
	}
	for _, c := range cases {
		if got := effectiveStatus(c.status, c.jobs); got != c.want {
			t.Errorf("%s %v: got %s want %s", c.status, c.jobs, got, c.want)
		}
	}
}
