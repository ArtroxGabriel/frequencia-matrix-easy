package domain

import (
	"sort"
	"time"
)

const isoDate = "2006-01-02"

var weekdayAbbrevPT = [...]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

// ScheduledDates returns the planned class dates (YYYY-MM-DD) of a course.
func ScheduledDates(cfg CourseConfig) []string {
	if cfg.StartDate.IsZero() {
		return nil
	}
	dates := CalculateClassDates(cfg.StartDate, cfg.Weekdays, cfg.TotalPeriods)
	out := make([]string, 0, len(dates))
	for _, d := range dates {
		out = append(out, d.Format(isoDate))
	}
	return out
}

// ClassDateWindow keeps the scheduled dates up to and including the first one
// after today, so the teacher is not offered the whole semester at once.
func ClassDateWindow(scheduled []string, today string) []string {
	window := make([]string, 0, len(scheduled))
	for _, d := range scheduled {
		window = append(window, d)
		if d > today {
			break
		}
	}
	return window
}

// DefaultClassDate picks the latest scheduled date that is not in the future.
// Before the course starts it falls back to the first class, and with no
// schedule at all it returns today.
func DefaultClassDate(scheduled []string, today string) string {
	if len(scheduled) == 0 {
		return today
	}
	chosen := scheduled[0]
	for _, d := range scheduled {
		if d > today {
			break
		}
		chosen = d
	}
	return chosen
}

// MergeDates returns the union of date lists, sorted and without duplicates.
func MergeDates(lists ...[]string) []string {
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, d := range list {
			seen[d] = true
		}
	}
	merged := make([]string, 0, len(seen))
	for d := range seen {
		merged = append(merged, d)
	}
	sort.Strings(merged)
	return merged
}

// IsValidDate reports whether s is a real calendar date in YYYY-MM-DD form.
func IsValidDate(s string) bool {
	_, err := time.Parse(isoDate, s)
	return err == nil
}

// FormatDateLabel renders "2026-03-03" as "ter 03/03/2026".
func FormatDateLabel(s string) string {
	t, err := time.Parse(isoDate, s)
	if err != nil {
		return s
	}
	return weekdayAbbrevPT[t.Weekday()] + " " + t.Format("02/01/2006")
}
