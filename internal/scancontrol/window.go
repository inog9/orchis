// Package scancontrol enforces manual scan windows and persistent emergency holds.
package scancontrol

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // IANA rules remain available in the minimal server image.
)

// Window uses ISO weekday numbers: Monday=1, Sunday=7. An empty window is unrestricted.
// Overnight windows belong to the day on which they start; the end is exclusive.
type Window struct {
	Timezone string `json:"timezone,omitempty"`
	Days     []int  `json:"days,omitempty"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
}

var zoneName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+/-]{0,63}$`)

func (w Window) Empty() bool {
	return w.Timezone == "" && len(w.Days) == 0 && w.Start == "" && w.End == ""
}
func minute(s string) (int, error) {
	t, e := time.Parse("15:04", s)
	if e != nil || len(s) != 5 {
		return 0, fmt.Errorf("use HH:MM in 24-hour time")
	}
	return t.Hour()*60 + t.Minute(), nil
}
func (w Window) Validate() error {
	if w.Empty() {
		return nil
	}
	if !zoneName.MatchString(w.Timezone) || strings.Contains(w.Timezone, "..") || w.Timezone == "Local" {
		return fmt.Errorf("choose an IANA timezone")
	}
	if _, e := time.LoadLocation(w.Timezone); e != nil {
		return fmt.Errorf("unknown IANA timezone")
	}
	if len(w.Days) < 1 || len(w.Days) > 7 {
		return fmt.Errorf("choose one to seven weekdays")
	}
	seen := map[int]bool{}
	for _, d := range w.Days {
		if d < 1 || d > 7 || seen[d] {
			return fmt.Errorf("weekdays must be unique numbers from 1 to 7")
		}
		seen[d] = true
	}
	a, e := minute(w.Start)
	if e != nil {
		return e
	}
	b, e := minute(w.End)
	if e != nil {
		return e
	}
	if a == b {
		return fmt.Errorf("start and end must differ; remove the window for unrestricted scans")
	}
	return nil
}
func (w Window) Allows(now time.Time) (bool, error) {
	if e := w.Validate(); e != nil {
		return false, e
	}
	if w.Empty() {
		return true, nil
	}
	zone, e := time.LoadLocation(w.Timezone)
	if e != nil {
		return false, e
	}
	local := now.In(zone)
	m := local.Hour()*60 + local.Minute()
	day := int(local.Weekday())
	if day == 0 {
		day = 7
	}
	start, _ := minute(w.Start)
	end, _ := minute(w.End)
	if start < end {
		if m < start || m >= end {
			return false, nil
		}
	} else {
		if m >= start {
		} else if m < end {
			day--
			if day == 0 {
				day = 7
			}
		} else {
			return false, nil
		}
	}
	for _, d := range w.Days {
		if d == day {
			return true, nil
		}
	}
	return false, nil
}
