package export

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"frequencia-matrix-integration/internal/domain"

	"github.com/xuri/excelize/v2"
)

const (
	SheetGraduacao    = "Graduacao"
	SheetPosGraduacao = "Pos-Graduacao"

	markNoClass = "S/A"
)

func ExportAttendanceXLSX(course domain.CourseConfig, students []domain.Student, sessions []domain.AttendanceSession) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	gradStudents, posGradStudents := domain.SortAndGroupStudents(students)
	classDates := resolveClassDates(course, sessions)

	_ = f.SetSheetName("Sheet1", SheetGraduacao)
	_, _ = f.NewSheet(SheetPosGraduacao)

	headerStyle, err := createHeaderStyle(f)
	if err != nil {
		return nil, fmt.Errorf("failed creating header style: %w", err)
	}

	if err := populateSheet(f, SheetGraduacao, gradStudents, classDates, sessions, course.TotalPeriods, headerStyle); err != nil {
		return nil, err
	}
	if err := populateSheet(f, SheetPosGraduacao, posGradStudents, classDates, sessions, course.TotalPeriods, headerStyle); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("failed writing xlsx buffer: %w", err)
	}
	return buf.Bytes(), nil
}

func createHeaderStyle(f *excelize.File) (int, error) {
	return f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1E3A8A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
}

func resolveClassDates(course domain.CourseConfig, sessions []domain.AttendanceSession) []string {
	dateMap := make(map[string]bool)
	if !course.StartDate.IsZero() && len(course.Weekdays) > 0 && course.TotalPeriods > 0 {
		schedDates := domain.CalculateClassDates(course.StartDate, course.Weekdays, course.TotalPeriods)
		for _, d := range schedDates {
			dateMap[d.Format("2006-01-02")] = true
		}
	}
	for _, sess := range sessions {
		if sess.ClassDate != "" {
			dateMap[sess.ClassDate] = true
		}
	}

	uniqueDates := make([]string, 0, len(dateMap))
	for d := range dateMap {
		uniqueDates = append(uniqueDates, d)
	}
	sort.Strings(uniqueDates)
	return uniqueDates
}

func populateSheet(f *excelize.File, sheet string, students []domain.Student, classDates []string, sessions []domain.AttendanceSession, totalPeriods int, headerStyle int) error {
	colNameA, _ := excelize.ColumnNumberToName(1)
	_ = f.SetColWidth(sheet, colNameA, colNameA, 14)
	_ = f.SetCellValue(sheet, cellName(1, 1), "Data")
	_ = f.SetCellStyle(sheet, cellName(1, 1), cellName(1, 1), headerStyle)

	for i, stu := range students {
		colIdx := i + 2
		colName, _ := excelize.ColumnNumberToName(colIdx)
		_ = f.SetColWidth(sheet, colName, colName, 28)

		cell := cellName(colIdx, 1)
		_ = f.SetCellValue(sheet, cell, studentHeader(stu))
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	sessionByDate := make(map[string]domain.AttendanceSession, len(sessions))
	for _, s := range sessions {
		sessionByDate[s.ClassDate] = s
	}

	for rowIdx, dateStr := range classDates {
		excelRow := rowIdx + 2
		_ = f.SetCellValue(sheet, cellName(1, excelRow), formatDateDisplay(dateStr))

		sess, hasSess := sessionByDate[dateStr]
		for colIdx, stu := range students {
			mark := "-"
			if hasSess {
				mark = getAttendanceMark(sess, stu.ID)
			}
			_ = f.SetCellValue(sheet, cellName(colIdx+2, excelRow), mark)
		}
	}

	summaryStartRow := len(classDates) + 2
	return writeSummaryRows(f, sheet, summaryStartRow, students, sessions, totalPeriods)
}

func writeSummaryRows(f *excelize.File, sheet string, startRow int, students []domain.Student, sessions []domain.AttendanceSession, totalPeriods int) error {
	summaries := make([]domain.StudentSummary, len(students))
	for i, stu := range students {
		summaries[i] = domain.CalculateStudentSummary(stu, sessions, totalPeriods)
	}

	type summaryLine struct {
		label    string
		getValue func(s domain.StudentSummary) any
	}

	lines := []summaryLine{
		{label: "Presenças", getValue: func(s domain.StudentSummary) any { return fmt.Sprintf("%d", s.Presences) }},
		{label: "Faltas", getValue: func(s domain.StudentSummary) any { return fmt.Sprintf("%d", s.Absences) }},
		{label: "% Faltas", getValue: func(s domain.StudentSummary) any { return fmt.Sprintf("%.1f%%", s.AbsencePercent) }},
		{label: "Faltas restantes", getValue: func(s domain.StudentSummary) any { return fmt.Sprintf("%d", s.RemainingAbsences) }},
		{label: "% Frequência", getValue: func(s domain.StudentSummary) any { return fmt.Sprintf("%.1f%%", s.AttendancePercent) }},
		{label: "Situação", getValue: func(s domain.StudentSummary) any {
			if s.IsApproved {
				return "Aprovado por Frequência"
			}
			return "Reprovado por Falta"
		}},
	}

	for lineIdx, line := range lines {
		row := startRow + lineIdx
		_ = f.SetCellValue(sheet, cellName(1, row), line.label)
		for stuIdx, s := range summaries {
			_ = f.SetCellValue(sheet, cellName(stuIdx+2, row), line.getValue(s))
		}
	}
	return nil
}

func formatDateDisplay(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("02/01/2006")
}

// studentHeader labels a column with name and registration, flagging students
// who were removed from the roster but still have recorded history.
func studentHeader(stu domain.Student) string {
	title := stu.FullName
	if stu.RegistrationNumber != "" {
		title = fmt.Sprintf("%s (%s)", stu.FullName, stu.RegistrationNumber)
	}
	if !stu.Active {
		title += " — inativo"
	}
	return title
}

func getAttendanceMark(sess domain.AttendanceSession, studentID string) string {
	if sess.Cancelled {
		return markNoClass
	}
	for _, e := range sess.Entries {
		if e.StudentID == studentID {
			if e.IsPresent {
				return "P"
			}
			return "F"
		}
	}
	return "-"
}

func cellName(col, row int) string {
	c, _ := excelize.CoordinatesToCellName(col, row)
	return c
}
