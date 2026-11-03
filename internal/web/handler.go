package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/export"
	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

type Handler struct {
	store  *store.Store
	matrix matrix.MatrixGateway
	mux    *http.ServeMux
	tmpl   *template.Template
}

func NewHandler(s *store.Store, mg matrix.MatrixGateway) (*Handler, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed parsing web templates: %w", err)
	}

	h := &Handler{
		store:  s,
		matrix: mg,
		mux:    http.NewServeMux(),
		tmpl:   tmpl,
	}
	h.registerRoutes()
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) registerRoutes() {
	h.mux.HandleFunc("GET /healthz", h.handleHealthz)
	h.mux.HandleFunc("GET /", h.authWrap(h.handleIndex))
	h.mux.HandleFunc("GET /widget/tab/setup", h.authWrap(h.handleTabSetup))
	h.mux.HandleFunc("POST /widget/setup", h.authWrap(h.handlePostSetup))
	h.mux.HandleFunc("GET /widget/tab/roster", h.authWrap(h.handleTabRoster))
	h.mux.HandleFunc("POST /widget/students", h.authWrap(h.handlePostStudent))
	h.mux.HandleFunc("DELETE /widget/students/{id}", h.authWrap(h.handleDeleteStudent))
	h.mux.HandleFunc("GET /widget/tab/attendance", h.authWrap(h.handleTabAttendance))
	h.mux.HandleFunc("POST /widget/attendance", h.authWrap(h.handlePostAttendance))
	h.mux.HandleFunc("GET /widget/tab/summary", h.authWrap(h.handleTabSummary))
	h.mux.HandleFunc("GET /widget/export.xlsx", h.authWrap(h.handleExportXLSX))
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type authHandlerFunc func(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string)

func (h *Handler) authWrap(next authHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomID := r.URL.Query().Get("roomId")
		token := r.URL.Query().Get("openIdToken")
		if token == "" {
			token = r.URL.Query().Get("userId")
		}

		ident, err := h.matrix.VerifyUser(r.Context(), roomID, token)
		if err != nil {
			h.renderUnauthorized(w, roomID, token)
			return
		}

		isTeacher, err := h.matrix.IsTeacher(r.Context(), ident.RoomID, ident.UserID)
		if err != nil || !isTeacher {
			h.renderUnauthorized(w, ident.RoomID, ident.UserID)
			return
		}

		next(w, r, ident, token)
	}
}

func (h *Handler) renderUnauthorized(w http.ResponseWriter, roomID, userID string) {
	w.WriteHeader(http.StatusForbidden)
	data := map[string]string{"RoomID": roomID, "UserID": userID}
	_ = h.tmpl.ExecuteTemplate(w, "unauthorized.html", data)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	data := h.buildAttendanceData(r.Context(), ident.RoomID, token, ident.UserID, cfg, time.Now().Format("2006-01-02"), "")

	tName := "attendance.html"
	if cfg.RoomID == "" {
		tName = "setup.html"
		data = h.buildSetupData(ident.RoomID, token, ident.UserID, cfg, "")
	}

	clone, _ := h.tmpl.Clone()
	_ = template.Must(clone.Parse(fmt.Sprintf(`{{define "tab_content"}}{{template "%s" .}}{{end}}`, tName)))
	_ = clone.ExecuteTemplate(w, "layout.html", data)
}

func (h *Handler) handleTabSetup(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	data := h.buildSetupData(ident.RoomID, token, ident.UserID, cfg, "")
	_ = h.tmpl.ExecuteTemplate(w, "setup.html", data)
}

func (h *Handler) handlePostSetup(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	totalPeriods, _ := strconv.Atoi(r.FormValue("totalPeriods"))
	startDate, _ := time.Parse("2006-01-02", r.FormValue("startDate"))
	var weekdays []time.Weekday
	for _, val := range r.Form["weekdays"] {
		if day, err := strconv.Atoi(val); err == nil {
			weekdays = append(weekdays, time.Weekday(day))
		}
	}

	cfg := domain.CourseConfig{
		RoomID:       ident.RoomID,
		CourseName:   strings.TrimSpace(r.FormValue("courseName")),
		Weekdays:     weekdays,
		TotalPeriods: totalPeriods,
		StartDate:    startDate,
	}

	if err := h.store.SaveCourseConfig(r.Context(), cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := h.buildSetupData(ident.RoomID, token, ident.UserID, cfg, "Configurações salvas com sucesso!")
	_ = h.tmpl.ExecuteTemplate(w, "setup.html", data)
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
		ID:                 fmt.Sprintf("stu_%d", time.Now().UnixNano()),
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

func (h *Handler) handleTabAttendance(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	selectedDate := r.URL.Query().Get("date")
	if selectedDate == "" {
		selectedDate = time.Now().Format("2006-01-02")
	}

	data := h.buildAttendanceData(r.Context(), ident.RoomID, token, ident.UserID, cfg, selectedDate, "")
	_ = h.tmpl.ExecuteTemplate(w, "attendance.html", data)
}

func (h *Handler) handlePostAttendance(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity, token string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	classDate := r.FormValue("classDate")
	periodCount, _ := strconv.Atoi(r.FormValue("periodCount"))
	if periodCount <= 0 {
		periodCount = 2
	}

	students, _ := h.store.ListStudents(r.Context(), ident.RoomID, true)
	presentIDs := make(map[string]bool)
	for _, idVal := range r.Form["present"] {
		presentIDs[idVal] = true
	}

	entries := make([]domain.AttendanceEntry, 0, len(students))
	for _, stu := range students {
		entries = append(entries, domain.AttendanceEntry{
			StudentID: stu.ID,
			IsPresent: presentIDs[stu.ID],
		})
	}

	session := domain.AttendanceSession{
		RoomID:      ident.RoomID,
		ClassDate:   classDate,
		PeriodCount: periodCount,
		Entries:     entries,
	}

	if err := h.store.RecordAttendanceSession(r.Context(), session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	noticeMsg := fmt.Sprintf("📋 Chamada registrada para a aula de %s: %d/%d presentes.", classDate, len(presentIDs), len(students))
	_ = h.matrix.SendNotice(r.Context(), ident.RoomID, noticeMsg)

	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	data := h.buildAttendanceData(r.Context(), ident.RoomID, token, ident.UserID, cfg, classDate, "Chamada salva com sucesso!")
	_ = h.tmpl.ExecuteTemplate(w, "attendance.html", data)
}

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

func (h *Handler) buildSetupData(roomID, token, userID string, cfg domain.CourseConfig, msg string) map[string]any {
	weekdayChecked := make(map[int]bool)
	for _, w := range cfg.Weekdays {
		weekdayChecked[int(w)] = true
	}
	startFormatted := ""
	if !cfg.StartDate.IsZero() {
		startFormatted = cfg.StartDate.Format("2006-01-02")
	}
	return map[string]any{
		"RoomID":             roomID,
		"UserID":             userID,
		"Token":              token,
		"Course":             cfg,
		"WeekdayChecked":     weekdayChecked,
		"StartDateFormatted": startFormatted,
		"Message":            msg,
	}
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

func (h *Handler) buildAttendanceData(ctx context.Context, roomID, token, userID string, cfg domain.CourseConfig, dateStr, msg string) map[string]any {
	students, _ := h.store.ListStudents(ctx, roomID, true)
	grad, posGrad := domain.SortAndGroupStudents(students)
	session, err := h.store.GetAttendanceSession(ctx, roomID, dateStr)

	presentMap := make(map[string]bool)
	periodCount := 2
	if err == nil {
		periodCount = session.PeriodCount
		for _, e := range session.Entries {
			if e.IsPresent {
				presentMap[e.StudentID] = true
			}
		}
	}

	return map[string]any{
		"RoomID":          roomID,
		"UserID":          userID,
		"Token":           token,
		"Course":          cfg,
		"SelectedDate":    dateStr,
		"PeriodCount":     periodCount,
		"PresentMap":      presentMap,
		"GradStudents":    grad,
		"PosGradStudents": posGrad,
		"Message":         msg,
	}
}
