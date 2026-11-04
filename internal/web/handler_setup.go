package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
)

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
		"IsSetupTab":         true,
	}
}
