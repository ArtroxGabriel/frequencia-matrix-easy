package domain_test

import (
	"testing"

	"frequencia-matrix-integration/internal/domain"
)

func session(periods int, cancelled bool, entries ...domain.AttendanceEntry) domain.AttendanceSession {
	return domain.AttendanceSession{PeriodCount: periods, Cancelled: cancelled, Entries: entries}
}

func entry(id string, present bool) domain.AttendanceEntry {
	return domain.AttendanceEntry{StudentID: id, IsPresent: present}
}

func TestMaxAbsencesFor(t *testing.T) {
	cases := map[int]int{-4: 0, 0: 0, 3: 0, 4: 1, 30: 7, 32: 8, 64: 16}
	for total, want := range cases {
		if got := domain.MaxAbsencesFor(total); got != want {
			t.Errorf("MaxAbsencesFor(%d) = %d, want %d", total, got, want)
		}
	}
}

func TestClassifyAbsences(t *testing.T) {
	cases := []struct {
		absences, max int
		want          domain.AttendanceStatus
	}{
		{0, 8, domain.StatusOK},
		{5, 8, domain.StatusOK},
		{6, 8, domain.StatusWarning},
		{8, 8, domain.StatusWarning},
		{9, 8, domain.StatusFailed},
		{0, 0, domain.StatusOK},
		{1, 0, domain.StatusFailed},
	}
	for _, c := range cases {
		if got := domain.ClassifyAbsences(c.absences, c.max); got != c.want {
			t.Errorf("ClassifyAbsences(%d, %d) = %s, want %s", c.absences, c.max, got, c.want)
		}
	}
}

func TestStudentStartsWithZeroAbsences(t *testing.T) {
	student := domain.Student{ID: "stu-1"}
	sessions := []domain.AttendanceSession{session(2, false, entry("stu-1", true))}

	got := domain.CalculateStudentSummary(student, sessions, 32)

	if got.Absences != 0 || got.AbsencePercent != 0 || got.AttendancePercent != 100 {
		t.Errorf("expected 0 absences and 100%% attendance, got %+v", got)
	}
	if !got.IsApproved || got.Status != domain.StatusOK {
		t.Errorf("a student with no absences must not be flagged, got %+v", got)
	}
}

func TestAbsencesAccumulateAgainstTotalLoad(t *testing.T) {
	student := domain.Student{ID: "stu-1"}
	sessions := []domain.AttendanceSession{
		session(2, false, entry("stu-1", false)),
		session(2, false, entry("stu-1", true)),
		session(2, false, entry("stu-1", false)),
	}

	got := domain.CalculateStudentSummary(student, sessions, 8)

	if got.Presences != 2 || got.Absences != 4 {
		t.Errorf("expected 2 presences and 4 absences, got %+v", got)
	}
	if got.AbsencePercent != 50 || got.AttendancePercent != 50 {
		t.Errorf("expected 50%% absence, got %+v", got)
	}
	if got.IsApproved || got.Status != domain.StatusFailed || got.RemainingAbsences != 0 {
		t.Errorf("4 absences of a limit of 2 must fail, got %+v", got)
	}
}

func TestExactlySeventyFivePercentIsApproved(t *testing.T) {
	student := domain.Student{ID: "stu-1"}
	sessions := []domain.AttendanceSession{session(2, false, entry("stu-1", false))}

	got := domain.CalculateStudentSummary(student, sessions, 8)

	if got.AttendancePercent != 75 || !got.IsApproved {
		t.Errorf("75%% attendance must be approved, got %+v", got)
	}
	if got.Status != domain.StatusWarning || got.RemainingAbsences != 0 {
		t.Errorf("at the limit the status must be warning with no absences left, got %+v", got)
	}
}

func TestCancelledSessionCountsAsNoAbsence(t *testing.T) {
	student := domain.Student{ID: "stu-1"}
	sessions := []domain.AttendanceSession{
		session(2, true, entry("stu-1", false)),
		session(2, false, entry("stu-1", true)),
	}

	got := domain.CalculateStudentSummary(student, sessions, 8)

	if got.Absences != 0 || got.Presences != 2 {
		t.Errorf("cancelled class must not count, got %+v", got)
	}
}

func TestStudentMissingFromSessionIsNotCounted(t *testing.T) {
	student := domain.Student{ID: "late-joiner"}
	sessions := []domain.AttendanceSession{session(2, false, entry("someone-else", false))}

	got := domain.CalculateStudentSummary(student, sessions, 8)

	if got.Absences != 0 || got.Presences != 0 {
		t.Errorf("a student with no entry must have no counts, got %+v", got)
	}
}

func TestZeroTotalPeriodsGuard(t *testing.T) {
	got := domain.CalculateStudentSummary(domain.Student{ID: "stu-1"}, nil, 0)

	if got.AttendancePercent != 0 || got.AbsencePercent != 0 || got.IsApproved {
		t.Errorf("zero total must yield 0%% and not approved, got %+v", got)
	}
}
