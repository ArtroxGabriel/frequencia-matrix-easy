package export

import (
	"bytes"
	"fmt"
	"time"

	"frequencia-matrix-integration/internal/domain"

	"github.com/xuri/excelize/v2"
)

const (
	SheetGraduacao    = "Graduacao"
	SheetPosGraduacao = "Pos-Graduacao"
)

type ColumnHeader struct {
	Title string
	Width float64
}

func ExportAttendanceXLSX(course domain.CourseConfig, students []domain.Student, sessions []domain.AttendanceSession) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	gradStudents, posGradStudents := domain.SortAndGroupStudents(students)

	_ = f.SetSheetName("Sheet1", SheetGraduacao)
	_, _ = f.NewSheet(SheetPosGraduacao)

	headerStyle, err := createHeaderStyle(f)
	if err != nil {
		return nil, fmt.Errorf("failed creating header style: %w", err)
	}

	if err := populateSheet(f, SheetGraduacao, gradStudents, sessions, course.TotalPeriods, headerStyle); err != nil {
		return nil, err
	}

	if err := populateSheet(f, SheetPosGraduacao, posGradStudents, sessions, course.TotalPeriods, headerStyle); err != nil {
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

func populateSheet(f *excelize.File, sheet string, students []domain.Student, sessions []domain.AttendanceSession, totalPeriods int, headerStyle int) error {
	headers := buildHeaders(sessions)
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, h.Title)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
		colName, _ := excelize.ColumnNumberToName(colIdx + 1)
		_ = f.SetColWidth(sheet, colName, colName, h.Width)
	}

	for rowIdx, stu := range students {
		summary := domain.CalculateStudentSummary(stu, sessions, totalPeriods)
		if err := writeStudentRow(f, sheet, rowIdx+2, summary, sessions); err != nil {
			return err
		}
	}
	return nil
}

func buildHeaders(sessions []domain.AttendanceSession) []ColumnHeader {
	headers := []ColumnHeader{
		{Title: "#", Width: 6},
		{Title: "Nome", Width: 32},
		{Title: "Matrícula", Width: 16},
	}

	for _, sess := range sessions {
		formattedDate := formatDateHeader(sess.ClassDate)
		headers = append(headers, ColumnHeader{Title: formattedDate, Width: 10})
	}

	headers = append(headers,
		ColumnHeader{Title: "Presenças", Width: 12},
		ColumnHeader{Title: "Faltas", Width: 10},
		ColumnHeader{Title: "% Frequência", Width: 14},
		ColumnHeader{Title: "Situação", Width: 26},
	)
	return headers
}

func formatDateHeader(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("02/01")
}

func writeStudentRow(f *excelize.File, sheet string, rowNum int, summary domain.StudentSummary, sessions []domain.AttendanceSession) error {
	col := 1
	_ = f.SetCellValue(sheet, cellName(col, rowNum), rowNum-1)
	col++
	_ = f.SetCellValue(sheet, cellName(col, rowNum), summary.Student.FullName)
	col++
	_ = f.SetCellValue(sheet, cellName(col, rowNum), summary.Student.RegistrationNumber)
	col++

	for _, sess := range sessions {
		mark := getAttendanceMark(sess, summary.Student.ID)
		_ = f.SetCellValue(sheet, cellName(col, rowNum), mark)
		col++
	}

	_ = f.SetCellValue(sheet, cellName(col, rowNum), summary.Presences)
	col++
	_ = f.SetCellValue(sheet, cellName(col, rowNum), summary.Absences)
	col++
	_ = f.SetCellValue(sheet, cellName(col, rowNum), fmt.Sprintf("%.1f%%", summary.AttendancePercent))
	col++

	situacao := "Reprovado por Falta"
	if summary.IsApproved {
		situacao = "Aprovado por Frequência"
	}
	_ = f.SetCellValue(sheet, cellName(col, rowNum), situacao)
	return nil
}

func getAttendanceMark(sess domain.AttendanceSession, studentID string) string {
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
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}
