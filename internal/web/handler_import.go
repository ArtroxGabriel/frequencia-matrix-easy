package web

import (
	"errors"
	"log"
	"net/http"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/importer"
	"frequencia-matrix-integration/internal/matrix"
)

const (
	// multipartOverhead leaves room for the form fields around the file.
	multipartOverhead = 64 << 10
	templateCSV       = "\xEF\xBB\xBFaluno;matricula\r\nMaria da Silva;501234\r\nJoão Pereira;501235\r\n"
)

// handleImportStudents reads a [aluno, matricula] CSV into the chosen level.
// Problems are answered with an empty status code so the roster stays on
// screen; unusable lines are only logged.
func (h *Handler) handleImportStudents(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	r.Body = http.MaxBytesReader(w, r.Body, importer.MaxBytes+multipartOverhead)
	if err := r.ParseMultipartForm(importer.MaxBytes); err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	level := domain.StudentLevel(r.FormValue("level"))
	file, _, err := r.FormFile("file")
	if err != nil || !level.IsValid() {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	defer func() { _ = file.Close() }()

	parsed, err := importer.ParseStudentsCSV(file)
	if err != nil {
		w.WriteHeader(statusForImportError(err))
		return
	}
	result, err := h.store.ImportStudents(r.Context(), ident.RoomID, toStudents(parsed.Rows, level))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("[INFO] roster import room=%s created=%d updated=%d skipped=%d unreadable=%d",
		ident.RoomID, result.Created, result.Updated, result.Skipped, len(parsed.Skipped))

	h.renderRoster(w, r, ident)
}

func statusForImportError(err error) int {
	if errors.Is(err, importer.ErrTooLarge) || errors.Is(err, importer.ErrTooManyRow) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusUnprocessableEntity
}

func toStudents(rows []importer.StudentRow, level domain.StudentLevel) []domain.Student {
	students := make([]domain.Student, 0, len(rows))
	for _, row := range rows {
		students = append(students, domain.Student{FullName: row.Name, RegistrationNumber: row.Registration, Level: level})
	}
	return students
}

// handleStudentsTemplate serves a sample file. It carries no data, so it needs
// no authorization and can be fetched by a plain link.
func (h *Handler) handleStudentsTemplate(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="modelo-alunos.csv"`)
	_, _ = w.Write([]byte(templateCSV))
}
