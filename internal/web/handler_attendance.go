package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
)

const defaultPeriodsPerClass = 2

type dateOption struct {
	Value string
	Label string
}

type studentRow struct {
	ID           string
	Name         string
	Registration string
	Absent       bool
	Base         int
	Total        int
}

type studentGroup struct {
	Title string
	Theme string
	Rows  []studentRow
}

func todayISO() string {
	return time.Now().Format("2006-01-02")
}

func (h *Handler) defaultClassDate(cfg domain.CourseConfig) string {
	return domain.DefaultClassDate(domain.ScheduledDates(cfg), todayISO())
}

func (h *Handler) handleTabAttendance(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	cfg, err := h.loadCourseConfig(r.Context(), ident.RoomID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	date := r.URL.Query().Get("date")
	if !domain.IsValidDate(date) {
		date = h.defaultClassDate(cfg)
	}
	h.renderAttendance(w, r.Context(), ident, cfg, date)
}

func (h *Handler) renderAttendance(w http.ResponseWriter, ctx context.Context, ident matrix.UserIdentity, cfg domain.CourseConfig, date string) {
	data, err := h.buildAttendanceData(ctx, ident.RoomID, ident.UserID, cfg, date)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "attendance.html", data)
}

// handlePostAttendance saves the roll call. Everyone is present unless the
// teacher marked an absence, so only the "absent" ids are read from the form.
// Invalid input is rejected without a body: the widget keeps its screen.
func (h *Handler) handlePostAttendance(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	classDate, ok := formClassDate(w, r)
	if !ok {
		return
	}
	students, err := h.store.ListStudents(r.Context(), ident.RoomID, true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	previous, prevErr := h.store.GetAttendanceSession(r.Context(), ident.RoomID, classDate)
	if prevErr != nil && !errors.Is(prevErr, store.ErrNotFound) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	session := newSession(ident.RoomID, classDate, parsePeriods(r.FormValue("periodCount")), students, r.Form["absent"])
	if err := h.store.RecordAttendanceSession(r.Context(), session); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if errors.Is(prevErr, store.ErrNotFound) || previous.Cancelled {
		if err := h.matrix.SendNotice(r.Context(), ident.RoomID, attendanceNotice(session)); err != nil {
			log.Printf("[WARN] failed sending attendance notice to room %s: %v", ident.RoomID, err)
		}
	}
	cfg, err := h.loadCourseConfig(r.Context(), ident.RoomID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.renderAttendance(w, r.Context(), ident, cfg, classDate)
}

// handleCancelClass marks a date as "no class" (nobody gets an absence) or
// returns it to the not-recorded state.
func (h *Handler) handleCancelClass(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	classDate, ok := formClassDate(w, r)
	if !ok {
		return
	}
	var err error
	if r.FormValue("cancelled") == "true" {
		err = h.store.RecordAttendanceSession(r.Context(), domain.AttendanceSession{
			RoomID: ident.RoomID, ClassDate: classDate, Cancelled: true,
		})
	} else {
		err = h.reopenClass(r.Context(), ident.RoomID, classDate)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	cfg, err := h.loadCourseConfig(r.Context(), ident.RoomID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.renderAttendance(w, r.Context(), ident, cfg, classDate)
}

// reopenClass deletes a cancelled session only; recorded roll calls are kept.
func (h *Handler) reopenClass(ctx context.Context, roomID, classDate string) error {
	existing, err := h.store.GetAttendanceSession(ctx, roomID, classDate)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !existing.Cancelled) {
		return nil
	}
	if err != nil {
		return err
	}
	return h.store.DeleteAttendanceSession(ctx, roomID, classDate)
}

func formClassDate(w http.ResponseWriter, r *http.Request) (string, bool) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return "", false
	}
	classDate := r.FormValue("classDate")
	if !domain.IsValidDate(classDate) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return "", false
	}
	return classDate, true
}

func parsePeriods(raw string) int {
	if raw == "1" {
		return 1
	}
	return defaultPeriodsPerClass
}

func newSession(roomID, classDate string, periods int, students []domain.Student, absentIDs []string) domain.AttendanceSession {
	absent := make(map[string]bool, len(absentIDs))
	for _, id := range absentIDs {
		absent[id] = true
	}
	entries := make([]domain.AttendanceEntry, 0, len(students))
	for _, stu := range students {
		entries = append(entries, domain.AttendanceEntry{StudentID: stu.ID, IsPresent: !absent[stu.ID]})
	}
	return domain.AttendanceSession{RoomID: roomID, ClassDate: classDate, PeriodCount: periods, Entries: entries}
}

func attendanceNotice(session domain.AttendanceSession) string {
	absent := 0
	for _, e := range session.Entries {
		if !e.IsPresent {
			absent++
		}
	}
	return fmt.Sprintf("📋 Chamada registrada para a aula de %s: %s em %d alunos.",
		domain.FormatDateLabel(session.ClassDate), absencesPhrase(absent), len(session.Entries))
}

func absencesPhrase(n int) string {
	switch n {
	case 0:
		return "nenhuma falta"
	case 1:
		return "1 falta"
	default:
		return fmt.Sprintf("%d faltas", n)
	}
}

func (h *Handler) buildAttendanceData(ctx context.Context, roomID, userID string, cfg domain.CourseConfig, date string) (map[string]any, error) {
	students, err := h.store.ListStudents(ctx, roomID, true)
	if err != nil {
		return nil, err
	}
	sessions, err := h.store.ListAttendanceSessions(ctx, roomID)
	if err != nil {
		return nil, err
	}
	current, hasCurrent, others := splitSessions(sessions, date)

	periods := defaultPeriodsPerClass
	if hasCurrent && !current.Cancelled {
		periods = current.PeriodCount
	}
	absent := absentSet(current, hasCurrent)
	grad, posGrad := domain.SortAndGroupStudents(students)

	return map[string]any{
		"RoomID":        roomID,
		"UserID":        userID,
		"Course":        cfg,
		"SelectedDate":  date,
		"PeriodCount":   periods,
		"Cancelled":     hasCurrent && current.Cancelled,
		"SessionExists": hasCurrent && !current.Cancelled,
		"MaxAbsences":   domain.MaxAbsencesFor(cfg.TotalPeriods),
		"AbsentCount":   len(absent),
		"TotalStudents": len(students),
		"DateOptions":   dateOptions(cfg, sessions, date),
		"Groups": []studentGroup{
			buildGroup("Graduação", "indigo", grad, absent, others, periods),
			buildGroup("Pós-Graduação", "purple", posGrad, absent, others, periods),
		},
		"IsSetupTab": false,
	}, nil
}

func splitSessions(sessions []domain.AttendanceSession, date string) (current domain.AttendanceSession, found bool, others []domain.AttendanceSession) {
	for _, s := range sessions {
		if s.ClassDate == date {
			current, found = s, true
			continue
		}
		others = append(others, s)
	}
	return current, found, others
}

func absentSet(session domain.AttendanceSession, found bool) map[string]bool {
	absent := make(map[string]bool)
	if !found || session.Cancelled {
		return absent
	}
	for _, e := range session.Entries {
		if !e.IsPresent {
			absent[e.StudentID] = true
		}
	}
	return absent
}

func buildGroup(title, theme string, students []domain.Student, absent map[string]bool, others []domain.AttendanceSession, periods int) studentGroup {
	rows := make([]studentRow, 0, len(students))
	for _, stu := range students {
		_, base := domain.CountPeriods(stu.ID, others)
		row := studentRow{ID: stu.ID, Name: stu.FullName, Registration: stu.RegistrationNumber, Absent: absent[stu.ID], Base: base, Total: base}
		if row.Absent {
			row.Total += periods
		}
		rows = append(rows, row)
	}
	return studentGroup{Title: title, Theme: theme, Rows: rows}
}

func dateOptions(cfg domain.CourseConfig, sessions []domain.AttendanceSession, selected string) []dateOption {
	state := make(map[string]string, len(sessions))
	recorded := make([]string, 0, len(sessions))
	for _, s := range sessions {
		recorded = append(recorded, s.ClassDate)
		state[s.ClassDate] = " — ✓ registrada"
		if s.Cancelled {
			state[s.ClassDate] = " — sem aula"
		}
	}
	window := domain.ClassDateWindow(domain.ScheduledDates(cfg), todayISO())
	dates := domain.MergeDates(window, recorded, []string{selected})

	options := make([]dateOption, 0, len(dates))
	for _, d := range dates {
		options = append(options, dateOption{Value: d, Label: domain.FormatDateLabel(d) + state[d]})
	}
	return options
}
