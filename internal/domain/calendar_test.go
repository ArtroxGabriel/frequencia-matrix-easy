package domain_test

import (
	"reflect"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/domain"
)

func tuesdayThursdayCourse() domain.CourseConfig {
	return domain.CourseConfig{
		Weekdays:     []time.Weekday{time.Tuesday, time.Thursday},
		TotalPeriods: 6,
		StartDate:    time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	}
}

func TestScheduledDates(t *testing.T) {
	got := domain.ScheduledDates(tuesdayThursdayCourse())
	want := []string{"2026-03-03", "2026-03-05", "2026-03-10", "2026-03-12", "2026-03-17", "2026-03-19"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if domain.ScheduledDates(domain.CourseConfig{}) != nil {
		t.Errorf("a course without start date has no schedule")
	}
}

func TestClassDateWindowAndDefault(t *testing.T) {
	scheduled := domain.ScheduledDates(tuesdayThursdayCourse())

	cases := []struct {
		today      string
		wantLast   string
		wantLen    int
		wantChosen string
	}{
		{"2026-03-01", "2026-03-03", 1, "2026-03-03"},
		{"2026-03-03", "2026-03-05", 2, "2026-03-03"},
		{"2026-03-04", "2026-03-05", 2, "2026-03-03"},
		{"2026-03-19", "2026-03-19", 6, "2026-03-19"},
		{"2026-12-01", "2026-03-19", 6, "2026-03-19"},
	}
	for _, c := range cases {
		window := domain.ClassDateWindow(scheduled, c.today)
		if len(window) != c.wantLen || window[len(window)-1] != c.wantLast {
			t.Errorf("today %s: window %v, want %d ending %s", c.today, window, c.wantLen, c.wantLast)
		}
		if got := domain.DefaultClassDate(scheduled, c.today); got != c.wantChosen {
			t.Errorf("today %s: default %s, want %s", c.today, got, c.wantChosen)
		}
	}
	if got := domain.DefaultClassDate(nil, "2026-03-03"); got != "2026-03-03" {
		t.Errorf("with no schedule the default is today, got %s", got)
	}
}

func TestMergeDatesSortsAndDeduplicates(t *testing.T) {
	got := domain.MergeDates([]string{"2026-03-05", "2026-03-03"}, []string{"2026-03-03", "2026-03-04"})
	want := []string{"2026-03-03", "2026-03-04", "2026-03-05"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDateValidationAndLabel(t *testing.T) {
	if !domain.IsValidDate("2026-03-03") || domain.IsValidDate("03/03/2026") || domain.IsValidDate("") || domain.IsValidDate("2026-02-30") {
		t.Errorf("unexpected date validity result")
	}
	if got := domain.FormatDateLabel("2026-03-03"); got != "ter 03/03/2026" {
		t.Errorf("unexpected label %q", got)
	}
	if got := domain.FormatDateLabel("garbage"); got != "garbage" {
		t.Errorf("an invalid date is returned as is, got %q", got)
	}
}
