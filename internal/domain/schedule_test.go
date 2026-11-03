package domain_test

import (
	"testing"
	"time"

	"frequencia-matrix-integration/internal/domain"
)

func TestCalculateClassDates(t *testing.T) {
	startDate := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	weekdays := []time.Weekday{time.Tuesday, time.Thursday}
	totalPeriods := 4

	dates := domain.CalculateClassDates(startDate, weekdays, totalPeriods)

	if len(dates) != 4 {
		t.Fatalf("expected 4 dates, got %d", len(dates))
	}

	expectedDates := []string{
		"2026-03-03", // Tuesday
		"2026-03-05", // Thursday
		"2026-03-10", // Tuesday
		"2026-03-12", // Thursday
	}

	for i, expected := range expectedDates {
		actual := dates[i].Format("2006-01-02")
		if actual != expected {
			t.Errorf("at index %d: expected %s, got %s", i, expected, actual)
		}
	}
}

func TestSortAndGroupStudentsWithAccentFolding(t *testing.T) {
	students := []domain.Student{
		{ID: "1", FullName: "João Silva", Level: domain.LevelGraduation},
		{ID: "2", FullName: "Ágatha Nunes", Level: domain.LevelGraduation},
		{ID: "3", FullName: "Érika Meireles", Level: domain.LevelGraduation}, // Should sort as "Erika"
		{ID: "4", FullName: "Daniel Costa", Level: domain.LevelGraduation},
		{ID: "5", FullName: "Fabio Lima", Level: domain.LevelGraduation},
		{ID: "6", FullName: "Bruno Rocha", Level: domain.LevelPosGraduation},
		{ID: "7", FullName: "Ícaro Paz", Level: domain.LevelPosGraduation}, // Should sort as "Icaro"
	}

	grad, posGrad := domain.SortAndGroupStudents(students)

	if len(grad) != 5 {
		t.Fatalf("expected 5 graduation students, got %d", len(grad))
	}
	if len(posGrad) != 2 {
		t.Fatalf("expected 2 pos-graduation students, got %d", len(posGrad))
	}

	// Expected graduation order with accent folding:
	// Ágatha (Agatha) -> Daniel -> Érika (Erika) -> Fabio -> João (Joao)
	expectedGrad := []string{"Ágatha Nunes", "Daniel Costa", "Érika Meireles", "Fabio Lima", "João Silva"}
	for i, expected := range expectedGrad {
		if grad[i].FullName != expected {
			t.Errorf("grad index %d: expected %q, got %q", i, expected, grad[i].FullName)
		}
	}

	// Expected pos-grad order:
	// Bruno -> Ícaro (Icaro)
	expectedPos := []string{"Bruno Rocha", "Ícaro Paz"}
	for i, expected := range expectedPos {
		if posGrad[i].FullName != expected {
			t.Errorf("posGrad index %d: expected %q, got %q", i, expected, posGrad[i].FullName)
		}
	}
}

func TestCalculateStudentSummary(t *testing.T) {
	student := domain.Student{
		ID:       "stu-1",
		FullName: "Érika Meireles",
		Level:    domain.LevelGraduation,
	}

	sessions := []domain.AttendanceSession{
		{
			ID:          "sess-1",
			PeriodCount: 2,
			Entries: []domain.AttendanceEntry{
				{StudentID: "stu-1", IsPresent: true},
			},
		},
		{
			ID:          "sess-2",
			PeriodCount: 2,
			Entries: []domain.AttendanceEntry{
				{StudentID: "stu-1", IsPresent: false},
			},
		},
		{
			ID:          "sess-3",
			PeriodCount: 2,
			Entries: []domain.AttendanceEntry{
				{StudentID: "stu-1", IsPresent: true},
			},
		},
	}

	// 2 present sessions (4 periods), 1 absent (2 periods). Total course periods = 8 (4 classes * 2 periods).
	summary := domain.CalculateStudentSummary(student, sessions, 8)

	if summary.Presences != 4 {
		t.Errorf("expected 4 presences, got %d", summary.Presences)
	}
	if summary.Absences != 2 {
		t.Errorf("expected 2 absences, got %d", summary.Absences)
	}
	// 4 / 8 = 50.0%
	if summary.AttendancePercent != 50.0 {
		t.Errorf("expected 50.0%% attendance, got %.2f%%", summary.AttendancePercent)
	}
	if summary.IsApproved {
		t.Errorf("expected IsApproved=false for 50%% attendance, got true")
	}

	// Passing scenario: 6 presences out of 8 = 75.0%
	passSessions := append(sessions, domain.AttendanceSession{
		ID:          "sess-4",
		PeriodCount: 2,
		Entries: []domain.AttendanceEntry{
			{StudentID: "stu-1", IsPresent: true},
		},
	})
	passSummary := domain.CalculateStudentSummary(student, passSessions, 8)
	if passSummary.AttendancePercent != 75.0 {
		t.Errorf("expected 75.0%% attendance, got %.2f%%", passSummary.AttendancePercent)
	}
	if !passSummary.IsApproved {
		t.Errorf("expected IsApproved=true for 75%% attendance, got false")
	}

	// Zero total periods guard
	zeroSummary := domain.CalculateStudentSummary(student, sessions, 0)
	if zeroSummary.AttendancePercent != 0.0 || zeroSummary.IsApproved {
		t.Errorf("expected 0.0%% and false for zero total periods")
	}
}
