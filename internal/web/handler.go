package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

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
	h.mux.HandleFunc("POST /widget/attendance/cancel", h.authWrap(h.handleCancelClass))
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
	var data map[string]any
	if cfg.RoomID == "" {
		data = h.buildSetupData(ident.RoomID, token, ident.UserID, cfg, "")
	} else {
		data = h.buildAttendanceData(r.Context(), ident.RoomID, token, ident.UserID, cfg, h.defaultClassDate(cfg))
	}
	_ = h.tmpl.ExecuteTemplate(w, "layout.html", data)
}
