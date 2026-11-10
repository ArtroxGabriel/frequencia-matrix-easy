package web

import (
	"context"
	"net/http"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/export"
	"frequencia-matrix-integration/internal/matrix"
)

type summaryGroup struct {
	Title string
	Theme string
	Rows  []domain.StudentSummary
}

func (h *Handler) loadSummaryInputs(ctx context.Context, roomID string) (domain.CourseConfig, []domain.Student, []domain.AttendanceSession, error) {
	cfg, err := h.loadCourseConfig(ctx, roomID)
	if err != nil {
		return domain.CourseConfig{}, nil, nil, err
	}
	students, err := h.store.ListStudents(ctx, roomID, false)
	if err != nil {
		return domain.CourseConfig{}, nil, nil, err
	}
	sessions, err := h.store.ListAttendanceSessions(ctx, roomID)
	if err != nil {
		return domain.CourseConfig{}, nil, nil, err
	}
	return cfg, students, sessions, nil
}

func (h *Handler) handleTabSummary(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	cfg, students, sessions, err := h.loadSummaryInputs(r.Context(), ident.RoomID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	grad, posGrad := domain.SortAndGroupStudents(students)

	data := map[string]any{
		"RoomID":        ident.RoomID,
		"UserID":        ident.UserID,
		"Course":        cfg,
		"TotalStudents": countActive(students),
		"TotalSessions": countHeldClasses(sessions),
		"MaxAbsences":   domain.MaxAbsencesFor(cfg.TotalPeriods),
		"MinAttendance": domain.MinAttendancePercent,
		"SummaryGroups": []summaryGroup{
			summarize("Graduação", "indigo", grad, sessions, cfg.TotalPeriods),
			summarize("Pós-Graduação", "purple", posGrad, sessions, cfg.TotalPeriods),
		},
	}
	_ = h.tmpl.ExecuteTemplate(w, "summary.html", data)
}

func summarize(title, theme string, students []domain.Student, sessions []domain.AttendanceSession, totalPeriods int) summaryGroup {
	rows := make([]domain.StudentSummary, 0, len(students))
	for _, s := range students {
		rows = append(rows, domain.CalculateStudentSummary(s, sessions, totalPeriods))
	}
	return summaryGroup{Title: title, Theme: theme, Rows: rows}
}

func countActive(students []domain.Student) int {
	n := 0
	for _, s := range students {
		if s.Active {
			n++
		}
	}
	return n
}

func countHeldClasses(sessions []domain.AttendanceSession) int {
	n := 0
	for _, s := range sessions {
		if !s.Cancelled {
			n++
		}
	}
	return n
}

func (h *Handler) handleExportXLSX(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	cfg, students, sessions, err := h.loadSummaryInputs(r.Context(), ident.RoomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	bytesData, err := export.ExportAttendanceXLSX(cfg, students, sessions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="frequencia.xlsx"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bytesData)
}
