package web

import (
	"errors"
	"net/http"
	"strings"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
)

func (h *Handler) renderRoster(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	students, err := h.store.ListStudents(r.Context(), ident.RoomID, true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	data := h.buildRosterData(ident.RoomID, ident.UserID, students)
	_ = h.tmpl.ExecuteTemplate(w, "roster.html", data)
}

func (h *Handler) handleTabRoster(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	h.renderRoster(w, r, ident)
}

func (h *Handler) handlePostStudent(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
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

	h.renderRoster(w, r, ident)
}

func (h *Handler) handleDeleteStudent(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	studentID := r.PathValue("id")
	if err := h.store.DeactivateStudent(r.Context(), ident.RoomID, studentID); err != nil && !errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.renderRoster(w, r, ident)
}

func (h *Handler) buildRosterData(roomID, userID string, students []domain.Student) map[string]any {
	grad, posGrad := domain.SortAndGroupStudents(students)
	return map[string]any{
		"RoomID":          roomID,
		"UserID":          userID,
		"TotalStudents":   len(students),
		"GradStudents":    grad,
		"PosGradStudents": posGrad,
	}
}
