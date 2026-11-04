package web

import (
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

func (h *Handler) handleTabSummary(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, false)
	sessions, _ := h.store.ListAttendanceSessions(r.Context(), ident.RoomID)
	grad, posGrad := domain.SortAndGroupStudents(students)

	data := map[string]any{
		"RoomID":        ident.RoomID,
		"UserID":        ident.UserID,
		"Token":         token,
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

func (h *Handler) handleExportXLSX(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, _ string) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, false)
	sessions, _ := h.store.ListAttendanceSessions(r.Context(), ident.RoomID)

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
