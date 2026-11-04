package web

import (
	"net/http"
	"strings"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
)

func (h *Handler) renderRoster(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, true)
	data := h.buildRosterData(ident.RoomID, token, ident.UserID, students)
	_ = h.tmpl.ExecuteTemplate(w, "roster.html", data)
}

func (h *Handler) handleTabRoster(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, true)
	data := h.buildRosterData(ident.RoomID, token, ident.UserID, students)
	_ = h.tmpl.ExecuteTemplate(w, "roster.html", data)
}

func (h *Handler) handlePostStudent(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newStu := domain.Student{
		ID:                 domain.NewStudentID(),
		RoomID:             ident.RoomID,
		FullName:           strings.TrimSpace(r.FormValue("fullName")),
		RegistrationNumber: strings.TrimSpace(r.FormValue("registrationNumber")),
		Level:              domain.StudentLevel(r.FormValue("level")),
		Active:             true,
	}

	if err := h.store.AddStudent(r.Context(), newStu); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, true)
	data := h.buildRosterData(ident.RoomID, token, ident.UserID, students)
	_ = h.tmpl.ExecuteTemplate(w, "roster.html", data)
}

func (h *Handler) handleDeleteStudent(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	studentID := r.PathValue("id")
	_ = h.store.DeactivateStudent(r.Context(), ident.RoomID, studentID)

	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, true)
	data := h.buildRosterData(ident.RoomID, token, ident.UserID, students)
	_ = h.tmpl.ExecuteTemplate(w, "roster.html", data)
}

func (h *Handler) buildRosterData(roomID, token, userID string, students []domain.Student) map[string]any {
	grad, posGrad := domain.SortAndGroupStudents(students)
	return map[string]any{
		"RoomID":          roomID,
		"UserID":          userID,
		"Token":           token,
		"TotalStudents":   len(students),
		"GradStudents":    grad,
		"PosGradStudents": posGrad,
	}
}
