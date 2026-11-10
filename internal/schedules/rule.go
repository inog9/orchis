// Package schedules runs recurring audits through the same launch controls as manual scans.
package schedules

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

type Rule struct {
	Frequency string `json:"frequency"`
	Timezone  string `json:"timezone"`
	Time      string `json:"time"`
	Weekday   int    `json:"weekday"`   // ISO: Monday=1, Sunday=7
	MonthDay  int    `json:"month_day"` // 29-31 clamp to the last day of shorter months
}

func (r Rule) Validate() error {
	if r.Frequency != "daily" && r.Frequency != "weekly" && r.Frequency != "monthly" {
		return fmt.Errorf("Choose daily, weekly, or monthly.")
	}
	if len(r.Timezone) > 64 || r.Timezone == "Local" || strings.Contains(r.Timezone, "..") || r.Timezone == "" {
		return fmt.Errorf("Choose an IANA timezone, such as Asia/Jakarta.")
	}
	if _, e := time.LoadLocation(r.Timezone); e != nil {
		return fmt.Errorf("Choose an IANA timezone, such as Asia/Jakarta.")
	}
	if _, e := time.Parse("15:04", r.Time); e != nil || len(r.Time) != 5 {
		return fmt.Errorf("Choose a time in HH:MM format.")
	}
	if r.Frequency == "weekly" && (r.Weekday < 1 || r.Weekday > 7) {
		return fmt.Errorf("Choose a weekday.")
	}
	if r.Frequency == "monthly" && (r.MonthDay < 1 || r.MonthDay > 31) {
		return fmt.Errorf("Choose a day from 1 to 31.")
	}
	return nil
}

// Next is strictly after the supplied instant. Nonexistent DST wall times are
// skipped. A repeated wall time runs only once, at its first occurrence.
func (r Rule) Next(after time.Time) (time.Time, error) {
	if e := r.Validate(); e != nil {
		return time.Time{}, e
	}
	loc, _ := time.LoadLocation(r.Timezone)
	clock, _ := time.Parse("15:04", r.Time)
	local := after.In(loc)
	for d := 0; d < 400; d++ {
		date := time.Date(local.Year(), local.Month(), local.Day()+d, 12, 0, 0, 0, loc)
		weekday := int(date.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if r.Frequency == "weekly" && weekday != r.Weekday {
			continue
		}
		last := time.Date(date.Year(), date.Month()+1, 0, 12, 0, 0, 0, loc).Day()
		want := r.MonthDay
		if want > last {
			want = last
		}
		if r.Frequency == "monthly" && date.Day() != want {
			continue
		}
		candidate := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		first := time.Time{}
		// Resolve folds, including half-hour daylight transitions, deterministically.
		for delta := -180; delta <= 180; delta++ {
			t := candidate.Add(time.Duration(delta) * time.Minute)
			wall := t.In(loc)
			if wall.Year() == date.Year() && wall.Month() == date.Month() && wall.Day() == date.Day() && wall.Hour() == clock.Hour() && wall.Minute() == clock.Minute() {
				first = t
				break
			}
		}
		if !first.IsZero() && first.After(after) {
			return first.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("No upcoming occurrence was found.")
}
func (r Rule) Preview(after time.Time) ([]time.Time, error) {
	out := []time.Time{}
	for i := 0; i < 3; i++ {
		n, e := r.Next(after)
		if e != nil {
			return nil, e
		}
		out = append(out, n)
		after = n
	}
	return out, nil
}
