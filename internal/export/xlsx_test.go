package export_test

import (
	"bytes"
	"testing"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/export"

	"github.com/xuri/excelize/v2"
)

func TestExportAttendanceXLSX(t *testing.T) {
	course := domain.CourseConfig{
		RoomID:       "!course:matrix.dc.ufc.br",
		CourseName:   "Sistemas Distribuídos",
		TotalPeriods: 4,
	}

	students := []domain.Student{
		{ID: "s1", FullName: "João Silva", RegistrationNumber: "101", Level: domain.LevelGraduation},
		{ID: "s2", FullName: "Érika Meireles", RegistrationNumber: "102", Level: domain.LevelGraduation},
		{ID: "s3", FullName: "Ágatha Nunes", RegistrationNumber: "103", Level: domain.LevelGraduation},
		{ID: "s4", FullName: "Bruno Rocha", RegistrationNumber: "201", Level: domain.LevelPosGraduation},
	}

	sessions := []domain.AttendanceSession{
		{
			ID:          "sess-1",
			ClassDate:   "2026-03-03",
			PeriodCount: 2,
			Entries: []domain.AttendanceEntry{
				{StudentID: "s1", IsPresent: true},
				{StudentID: "s2", IsPresent: true},
				{StudentID: "s3", IsPresent: false},
				{StudentID: "s4", IsPresent: true},
			},
		},
		{
			ID:          "sess-2",
			ClassDate:   "2026-03-05",
			PeriodCount: 2,
			Entries: []domain.AttendanceEntry{
				{StudentID: "s1", IsPresent: false},
				{StudentID: "s2", IsPresent: true},
				{StudentID: "s3", IsPresent: false},
				{StudentID: "s4", IsPresent: true},
			},
		},
	}

	data, err := export.ExportAttendanceXLSX(course, students, sessions)
	if err != nil {
		t.Fatalf("failed to export xlsx: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	defer f.Close()

	// Verify sheets exist
	sheetList := f.GetSheetList()
	hasGrad := false
	hasPos := false
	for _, sheet := range sheetList {
		if sheet == export.SheetGraduacao {
			hasGrad = true
		}
		if sheet == export.SheetPosGraduacao {
			hasPos = true
		}
	}
	if !hasGrad || !hasPos {
		t.Fatalf("missing required sheets in: %v", sheetList)
	}

	// Verify Graduation sheet rows and sorting:
	// Row 1: Headers
	// Row 2: Ágatha Nunes (0 presences, 4 absences, 0.0%, Reprovado por Falta)
	// Row 3: Érika Meireles (4 presences, 0 absences, 100.0%, Aprovado por Frequência)
	// Row 4: João Silva (2 presences, 2 absences, 50.0%, Reprovado por Falta)
	gradRows, err := f.GetRows(export.SheetGraduacao)
	if err != nil {
		t.Fatalf("failed getting grad rows: %v", err)
	}

	if len(gradRows) < 4 {
		t.Fatalf("expected at least 4 rows in Graduacao, got %d", len(gradRows))
	}

	if gradRows[1][1] != "Ágatha Nunes" {
		t.Errorf("expected row 2 to be Ágatha Nunes, got %s", gradRows[1][1])
	}
	if gradRows[2][1] != "Érika Meireles" {
		t.Errorf("expected row 3 to be Érika Meireles, got %s", gradRows[2][1])
	}
	if gradRows[3][1] != "João Silva" {
		t.Errorf("expected row 4 to be João Silva, got %s", gradRows[3][1])
	}

	// Check attendance marks for Érika (col 3: 03/03 -> P, col 4: 05/03 -> P)
	if gradRows[2][3] != "P" || gradRows[2][4] != "P" {
		t.Errorf("expected P and P for Érika, got %s and %s", gradRows[2][3], gradRows[2][4])
	}

	// Verify Pos-Graduation sheet
	posRows, err := f.GetRows(export.SheetPosGraduacao)
	if err != nil {
		t.Fatalf("failed getting pos grad rows: %v", err)
	}
	if len(posRows) < 2 {
		t.Fatalf("expected at least 2 rows in Pos-Graduacao, got %d", len(posRows))
	}
	if posRows[1][1] != "Bruno Rocha" {
		t.Errorf("expected row 2 in Pos-Graduacao to be Bruno Rocha, got %s", posRows[1][1])
	}
}
