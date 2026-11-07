package export_test

import (
	"bytes"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/export"

	"github.com/xuri/excelize/v2"
)

func TestExportAttendanceXLSX(t *testing.T) {
	course := domain.CourseConfig{
		RoomID:       "!course:matrix.dc.ufc.br",
		CourseName:   "Sistemas Distribuídos",
		Weekdays:     []time.Weekday{time.Tuesday, time.Thursday},
		TotalPeriods: 4,
		StartDate:    time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	}

	students := []domain.Student{
		{ID: "s1", Active: true, FullName: "João Silva", RegistrationNumber: "101", Level: domain.LevelGraduation},
		{ID: "s2", Active: true, FullName: "Érika Meireles", RegistrationNumber: "102", Level: domain.LevelGraduation},
		{ID: "s3", Active: true, FullName: "Ágatha Nunes", RegistrationNumber: "103", Level: domain.LevelGraduation},
		{ID: "s4", Active: true, FullName: "Bruno Rocha", RegistrationNumber: "201", Level: domain.LevelPosGraduation},
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
	defer func() { _ = f.Close() }()

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

	gradRows, err := f.GetRows(export.SheetGraduacao)
	if err != nil {
		t.Fatalf("failed getting grad rows: %v", err)
	}

	// Verify headers: Row 0 has "Data", "Ágatha Nunes (103)", "Érika Meireles (102)", "João Silva (101)"
	if len(gradRows) < 11 {
		t.Fatalf("expected at least 11 rows in Graduacao, got %d", len(gradRows))
	}

	if gradRows[0][0] != "Data" {
		t.Errorf("expected header col 0 to be 'Data', got %q", gradRows[0][0])
	}
	if gradRows[0][1] != "Ágatha Nunes (103)" {
		t.Errorf("expected header col 1 to be Ágatha Nunes (103), got %q", gradRows[0][1])
	}
	if gradRows[0][2] != "Érika Meireles (102)" {
		t.Errorf("expected header col 2 to be Érika Meireles (102), got %q", gradRows[0][2])
	}
	if gradRows[0][3] != "João Silva (101)" {
		t.Errorf("expected header col 3 to be João Silva (101), got %q", gradRows[0][3])
	}

	// Check Class Dates (Rows 1 to 4)
	expectedDates := []string{"03/03/2026", "05/03/2026", "10/03/2026", "12/03/2026"}
	for i, d := range expectedDates {
		rowIdx := i + 1
		if gradRows[rowIdx][0] != d {
			t.Errorf("expected row %d col 0 to be %s, got %s", rowIdx, d, gradRows[rowIdx][0])
		}
	}

	// Row 1: 03/03/2026 -> Ágatha: F, Érika: P, João: P
	if gradRows[1][1] != "F" || gradRows[1][2] != "P" || gradRows[1][3] != "P" {
		t.Errorf("expected F, P, P for 03/03/2026, got %s, %s, %s", gradRows[1][1], gradRows[1][2], gradRows[1][3])
	}

	// Row 2: 05/03/2026 -> Ágatha: F, Érika: P, João: F
	if gradRows[2][1] != "F" || gradRows[2][2] != "P" || gradRows[2][3] != "F" {
		t.Errorf("expected F, P, F for 05/03/2026, got %s, %s, %s", gradRows[2][1], gradRows[2][2], gradRows[2][3])
	}

	// Row 3: 10/03/2026 (future) -> Ágatha: -, Érika: -, João: -
	if gradRows[3][1] != "-" || gradRows[3][2] != "-" || gradRows[3][3] != "-" {
		t.Errorf("expected -, -, - for 10/03/2026, got %s, %s, %s", gradRows[3][1], gradRows[3][2], gradRows[3][3])
	}

	// Summary rows:
	// Row 5: Presenças
	if gradRows[5][0] != "Presenças" || gradRows[5][1] != "0" || gradRows[5][2] != "4" || gradRows[5][3] != "2" {
		t.Errorf("unexpected Presenças row: %v", gradRows[5])
	}
	// Row 6: Faltas
	if gradRows[6][0] != "Faltas" || gradRows[6][1] != "4" || gradRows[6][2] != "0" || gradRows[6][3] != "2" {
		t.Errorf("unexpected Faltas row: %v", gradRows[6])
	}
	// Row 7: % Faltas (absences against the total course load of 4 periods)
	if gradRows[7][0] != "% Faltas" || gradRows[7][1] != "100.0%" || gradRows[7][2] != "0.0%" || gradRows[7][3] != "50.0%" {
		t.Errorf("unexpected %% Faltas row: %v", gradRows[7])
	}
	// Row 8: Faltas restantes (limit is 1 period out of 4)
	if gradRows[8][0] != "Faltas restantes" || gradRows[8][1] != "0" || gradRows[8][2] != "1" || gradRows[8][3] != "0" {
		t.Errorf("unexpected Faltas restantes row: %v", gradRows[8])
	}
	// Row 9: % Frequência
	if gradRows[9][0] != "% Frequência" || gradRows[9][1] != "0.0%" || gradRows[9][2] != "100.0%" || gradRows[9][3] != "50.0%" {
		t.Errorf("unexpected %% Frequência row: %v", gradRows[9])
	}
	// Row 10: Situação
	if gradRows[10][0] != "Situação" || gradRows[10][1] != "Reprovado por Falta" || gradRows[10][2] != "Aprovado por Frequência" || gradRows[10][3] != "Reprovado por Falta" {
		t.Errorf("unexpected Situação row: %v", gradRows[10])
	}

	// Verify Pos-Graduation sheet
	posRows, err := f.GetRows(export.SheetPosGraduacao)
	if err != nil {
		t.Fatalf("failed getting pos grad rows: %v", err)
	}
	if len(posRows) < 11 {
		t.Fatalf("expected at least 11 rows in Pos-Graduacao, got %d", len(posRows))
	}
	if posRows[0][1] != "Bruno Rocha (201)" {
		t.Errorf("expected header col 1 in Pos-Graduacao to be Bruno Rocha (201), got %s", posRows[0][1])
	}
	if posRows[1][1] != "P" || posRows[2][1] != "P" {
		t.Errorf("expected P and P for Bruno Rocha on held classes, got %s, %s", posRows[1][1], posRows[2][1])
	}
	if posRows[3][1] != "-" {
		t.Errorf("expected '-' for Bruno Rocha on future class, got %s", posRows[3][1])
	}
	if posRows[5][1] != "4" || posRows[6][1] != "0" {
		t.Errorf("expected 4 presences and 0 absences for Bruno Rocha, got %s and %s", posRows[5][1], posRows[6][1])
	}
}

func openExport(t *testing.T, course domain.CourseConfig, students []domain.Student, sessions []domain.AttendanceSession) *excelize.File {
	t.Helper()
	data, err := export.ExportAttendanceXLSX(course, students, sessions)
	if err != nil {
		t.Fatalf("failed to export xlsx: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestExportMarksCancelledClassesAndInactiveStudents(t *testing.T) {
	course := domain.CourseConfig{
		Weekdays:     []time.Weekday{time.Tuesday},
		TotalPeriods: 4,
		StartDate:    time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	}
	students := []domain.Student{
		{ID: "s1", Active: true, FullName: "Ana", Level: domain.LevelGraduation},
		{ID: "s2", Active: false, FullName: "Bia", RegistrationNumber: "7", Level: domain.LevelGraduation},
	}
	sessions := []domain.AttendanceSession{
		{ClassDate: "2026-03-03", Cancelled: true},
		{ClassDate: "2026-03-10", PeriodCount: 2, Entries: []domain.AttendanceEntry{
			{StudentID: "s1", IsPresent: false}, {StudentID: "s2", IsPresent: true},
		}},
	}

	rows, err := openExport(t, course, students, sessions).GetRows(export.SheetGraduacao)
	if err != nil {
		t.Fatalf("failed getting rows: %v", err)
	}

	if rows[0][2] != "Bia (7) — inativo" || rows[0][1] != "Ana" {
		t.Errorf("unexpected headers: %v", rows[0])
	}
	if rows[1][1] != "S/A" || rows[1][2] != "S/A" {
		t.Errorf("a cancelled class must read S/A for everyone, got %v", rows[1])
	}
	if rows[2][1] != "F" || rows[2][2] != "P" {
		t.Errorf("unexpected marks for the held class: %v", rows[2])
	}
	faltas := rows[len(rows)-5]
	if faltas[0] != "Faltas" || faltas[1] != "2" || faltas[2] != "0" {
		t.Errorf("the cancelled class must not count as an absence, got %v", faltas)
	}
}
