package schedules

import (
	"orchis-go/internal/reporting"
	"testing"
	"time"
)

func instant(s string) time.Time {
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return v
}
func TestCalendarOccurrences(t *testing.T) {
	cases := []struct {
		name        string
		r           Rule
		after, want string
	}{
		{"Jakarta daily", Rule{Frequency: "daily", Timezone: "Asia/Jakarta", Time: "02:00"}, "2026-11-09T20:00:00Z", "2026-11-10T19:00:00Z"},
		{"exact occurrence is exclusive", Rule{Frequency: "daily", Timezone: "UTC", Time: "02:00"}, "2026-11-10T02:00:00Z", "2026-11-11T02:00:00Z"},
		{"Monday", Rule{Frequency: "weekly", Timezone: "UTC", Time: "02:00", Weekday: 1}, "2026-11-10T00:00:00Z", "2026-11-16T02:00:00Z"},
		{"month-end clamp", Rule{Frequency: "monthly", Timezone: "UTC", Time: "02:00", MonthDay: 31}, "2026-01-31T02:00:00Z", "2026-02-28T02:00:00Z"},
		{"leap year", Rule{Frequency: "monthly", Timezone: "UTC", Time: "02:00", MonthDay: 31}, "2028-01-31T02:00:00Z", "2028-02-29T02:00:00Z"},
		{"DST gap skipped", Rule{Frequency: "daily", Timezone: "America/New_York", Time: "02:30"}, "2026-03-07T08:00:00Z", "2026-03-09T06:30:00Z"},
		{"DST fold first", Rule{Frequency: "daily", Timezone: "America/New_York", Time: "01:30"}, "2026-11-01T04:00:00Z", "2026-11-01T05:30:00Z"},
		{"DST fold runs once", Rule{Frequency: "daily", Timezone: "America/New_York", Time: "01:30"}, "2026-11-01T05:30:00Z", "2026-11-02T06:30:00Z"},
		{"Lord Howe half hour fold", Rule{Frequency: "daily", Timezone: "Australia/Lord_Howe", Time: "01:45"}, "2026-04-04T14:45:00Z", "2026-04-05T15:15:00Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, e := c.r.Next(instant(c.after))
			if e != nil || !got.Equal(instant(c.want)) {
				t.Fatalf("got %s %v want %s", got, e, c.want)
			}
			preview, e := c.r.Preview(instant(c.after))
			if e != nil || len(preview) != 3 || !preview[1].After(preview[0]) || !preview[2].After(preview[1]) {
				t.Fatalf("bad preview %v %v", preview, e)
			}
		})
	}
}
func TestRuleRejectsInvalidInputs(t *testing.T) {
	for _, r := range []Rule{{Frequency: "hourly", Timezone: "UTC", Time: "02:00"}, {Frequency: "daily", Timezone: "Local", Time: "02:00"}, {Frequency: "daily", Timezone: "../../etc", Time: "02:00"}, {Frequency: "daily", Timezone: "UTC", Time: "2:00"}, {Frequency: "weekly", Timezone: "UTC", Time: "02:00", Weekday: 8}, {Frequency: "monthly", Timezone: "UTC", Time: "02:00", MonthDay: 0}} {
		if r.Validate() == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
}
func TestSummaryPreservesCoverageAndOnlyAlertsOnChanges(t *testing.T) {
	snap := reporting.Snapshot{Assets: []reporting.Asset{{Methods: []reporting.Method{{Outcome: "success"}, {Outcome: "failed"}}}}, ComparisonDetails: &reporting.Comparison{Totals: reporting.ChangeCounts{New: 1, Persistent: 2, Resolved: 1, Uncompared: 3}, Rows: []reporting.FindingChange{{Severity: "critical", Change: "new"}, {Severity: "high", Change: "persistent"}, {Severity: "critical", Change: "uncompared", Reason: "previous_not_completed"}}}}
	s := summarize(snap)
	if s.Total != 2 || s.Completed != 1 || s.Attention != 1 || s.Persistent != 2 || s.Resolved != 1 || s.Uncompared != 3 {
		t.Fatalf("wrong summary %+v", s)
	}
}
