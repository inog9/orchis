package scancontrol_test

import (
	"orchis-go/internal/scancontrol"
	"testing"
	"time"
)

func TestWindowBoundariesAndTimezones(t *testing.T) {
	weekday := scancontrol.Window{Timezone: "Asia/Jakarta", Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "17:00"}
	overnight := scancontrol.Window{Timezone: "Asia/Jakarta", Days: []int{1}, Start: "22:00", End: "02:00"}
	dst := scancontrol.Window{Timezone: "America/New_York", Days: []int{7}, Start: "01:00", End: "03:00"}
	cases := []struct {
		name   string
		window scancontrol.Window
		at     string
		want   bool
	}{
		{"no window", scancontrol.Window{}, "2026-11-02T00:00:00Z", true},
		{"before start", weekday, "2026-11-02T01:59:59Z", false},
		{"start inclusive", weekday, "2026-11-02T02:00:00Z", true},
		{"last second", weekday, "2026-11-02T09:59:59Z", true},
		{"end exclusive", weekday, "2026-11-02T10:00:00Z", false},
		{"weekend", weekday, "2026-11-07T02:00:00Z", false},
		{"overnight starts Monday", overnight, "2026-11-02T15:00:00Z", true},
		{"overnight Tuesday early belongs to Monday", overnight, "2026-11-02T18:59:59Z", true},
		{"overnight closes Tuesday", overnight, "2026-11-02T19:00:00Z", false},
		{"Sunday night excluded", overnight, "2026-11-01T18:00:00Z", false},
		{"first repeated hour", dst, "2026-11-01T05:30:00Z", true},
		{"second repeated hour", dst, "2026-11-01T06:30:00Z", true},
		{"spring gap first valid time outside", dst, "2026-03-08T07:00:00Z", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, e := time.Parse(time.RFC3339, c.at)
			if e != nil {
				t.Fatal(e)
			}
			got, e := c.window.Allows(at)
			if e != nil || got != c.want {
				t.Fatalf("allowed=%v want=%v err=%v", got, c.want, e)
			}
		})
	}
}
func TestInvalidWindowFailsClosed(t *testing.T) {
	good := scancontrol.Window{Timezone: "UTC", Days: []int{1}, Start: "09:00", End: "17:00"}
	bad := []scancontrol.Window{{Timezone: "Local", Days: good.Days, Start: good.Start, End: good.End}, {Timezone: "../etc/passwd", Days: good.Days, Start: good.Start, End: good.End}, {Timezone: "UTC", Days: []int{0}, Start: good.Start, End: good.End}, {Timezone: "UTC", Days: []int{1, 1}, Start: good.Start, End: good.End}, {Timezone: "UTC", Days: good.Days, Start: "9:00", End: good.End}, {Timezone: "UTC", Days: good.Days, Start: "09:00", End: "09:00"}, {Timezone: "Missing/Zone", Days: good.Days, Start: good.Start, End: good.End}}
	for _, w := range bad {
		if ok, e := w.Allows(time.Now()); e == nil || ok {
			t.Fatalf("invalid window accepted: %+v allowed=%v err=%v", w, ok, e)
		}
	}
}
func TestWeeklyWindowDeterminism(t *testing.T) {
	w := scancontrol.Window{Timezone: "UTC", Days: []int{7}, Start: "23:00", End: "01:00"}
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 7*24*60; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		a, e := w.Allows(at)
		b, e2 := w.Allows(at.Add(7 * 24 * time.Hour))
		if e != nil || e2 != nil || a != b {
			t.Fatalf("weekly determinism at %v", at)
		}
	}
}
