package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
)

func (h *Handler) handleTabSetup(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	cfg, _ := h.store.GetCourseConfig(r.Context(), ident.RoomID)
	h.renderSetup(r.Context(), w, ident, cfg, false)
}

func (h *Handler) renderSetup(ctx context.Context, w http.ResponseWriter, ident matrix.UserIdentity, cfg domain.CourseConfig, saved bool) {
	data := h.buildSetupData(ctx, ident.RoomID, ident.UserID, cfg, saved)
	_ = h.tmpl.ExecuteTemplate(w, "setup.html", data)
}

// handlePostSetup saves the course. Invalid input is rejected with an empty
// 422 so the form stays as the teacher left it, without an error message.
func (h *Handler) handlePostSetup(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	cfg, ok := parseCourseForm(r, ident.RoomID)
	if !ok {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	if err := h.store.SaveCourseConfig(r.Context(), cfg); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.renderSetup(r.Context(), w, ident, cfg, true)
}

func parseCourseForm(r *http.Request, roomID string) (domain.CourseConfig, bool) {
	if err := r.ParseForm(); err != nil {
		return domain.CourseConfig{}, false
	}
	totalPeriods, err := strconv.Atoi(strings.TrimSpace(r.FormValue("totalPeriods")))
	if err != nil {
		return domain.CourseConfig{}, false
	}
	startDate, err := time.Parse("2006-01-02", r.FormValue("startDate"))
	if err != nil {
		return domain.CourseConfig{}, false
	}
	cfg := domain.CourseConfig{
		RoomID:       roomID,
		CourseName:   strings.TrimSpace(r.FormValue("courseName")),
		Weekdays:     parseWeekdays(r.Form["weekdays"]),
		TotalPeriods: totalPeriods,
		StartDate:    startDate,
	}
	return cfg, cfg.Validate() == nil
}

// parseWeekdays keeps the valid, distinct weekdays (0=Sunday..6=Saturday) in order.
func parseWeekdays(values []string) []time.Weekday {
	seen := make(map[time.Weekday]bool)
	var days []time.Weekday
	for _, v := range values {
		n, err := strconv.Atoi(v)
		day := time.Weekday(n)
		if err != nil || n < 0 || n > 6 || seen[day] {
			continue
		}
		seen[day] = true
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	return days
}

func (h *Handler) buildSetupData(ctx context.Context, roomID, userID string, cfg domain.CourseConfig, saved bool) map[string]any {
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
		"Course":             cfg,
		"WeekdayChecked":     weekdayChecked,
		"StartDateFormatted": startFormatted,
		"Saved":              saved,
		"IsSetupTab":         true,
		"CanManagePIN":       userID != pinUserID,
		"PinSet":             h.roomHasPIN(ctx, roomID),
	}
}
