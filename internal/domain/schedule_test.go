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
