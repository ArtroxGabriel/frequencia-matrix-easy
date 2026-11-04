package web

import (
	"net/http"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/export"
	"frequencia-matrix-integration/internal/matrix"
)

func (h *Handler) handleTabSummary(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, false)
	sessions, _ := h.store.ListAttendanceSessions(r.Context(), ident.RoomID)

	gradStudents, posGradStudents := domain.SortAndGroupStudents(students)
	gradSummaries := make([]domain.StudentSummary, 0, len(gradStudents))
	for _, s := range gradStudents {
		gradSummaries = append(gradSummaries, domain.CalculateStudentSummary(s, sessions, cfg.TotalPeriods))
	}
	posSummaries := make([]domain.StudentSummary, 0, len(posGradStudents))
	for _, s := range posGradStudents {
		posSummaries = append(posSummaries, domain.CalculateStudentSummary(s, sessions, cfg.TotalPeriods))
	}

	data := map[string]any{
		"RoomID":           ident.RoomID,
		"UserID":           ident.UserID,
		"Token":            token,
		"Course":           cfg,
		"TotalStudents":    len(students),
		"TotalSessions":    len(sessions),
		"GradSummaries":    gradSummaries,
		"PosGradSummaries": posSummaries,
	}
	_ = h.tmpl.ExecuteTemplate(w, "summary.html", data)
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
