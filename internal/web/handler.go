package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// DefaultPINSessionTTL is how long a session opened with a room PIN lasts.
const DefaultPINSessionTTL = 24 * time.Hour

type Handler struct {
	store  *store.Store
	matrix matrix.MatrixGateway
	mux    *http.ServeMux
	tmpl   *template.Template

	pinSessionTTL time.Duration
}

// Option customises a Handler.
type Option func(*Handler)

// WithPINSessionTTL sets how long a PIN session lasts. Non-positive values
// are ignored and the default is kept.
func WithPINSessionTTL(ttl time.Duration) Option {
	return func(h *Handler) {
		if ttl > 0 {
			h.pinSessionTTL = ttl
		}
	}
}

func NewHandler(s *store.Store, mg matrix.MatrixGateway, opts ...Option) (*Handler, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed parsing web templates: %w", err)
	}

	h := &Handler{
		store:  s,
		matrix: mg,
		mux:    http.NewServeMux(),
		tmpl:   tmpl,

		pinSessionTTL: DefaultPINSessionTTL,
	}
	for _, opt := range opts {
		opt(h)
	}
	h.registerRoutes()
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) registerRoutes() {
	h.mux.HandleFunc("GET /healthz", h.handleHealthz)
	h.mux.Handle("GET /static/", http.FileServerFS(staticFS))
	h.mux.HandleFunc("GET /{$}", h.handleRoot)
	h.mux.HandleFunc("GET /widget/home", h.authWrap(h.handleHome))
	h.mux.HandleFunc("GET /widget/tab/setup", h.authWrap(h.handleTabSetup))
	h.mux.HandleFunc("POST /widget/setup", h.authWrap(h.handlePostSetup))
	h.mux.HandleFunc("GET /widget/tab/roster", h.authWrap(h.handleTabRoster))
	h.mux.HandleFunc("POST /widget/students", h.authWrap(h.handlePostStudent))
	h.mux.HandleFunc("POST /widget/students/import", h.authWrap(h.handleImportStudents))
	h.mux.HandleFunc("GET /widget/students/template.csv", h.handleStudentsTemplate)
	h.mux.HandleFunc("DELETE /widget/students/{id}", h.authWrap(h.handleDeleteStudent))
	h.mux.HandleFunc("GET /widget/tab/attendance", h.authWrap(h.handleTabAttendance))
	h.mux.HandleFunc("POST /widget/attendance", h.authWrap(h.handlePostAttendance))
	h.mux.HandleFunc("POST /widget/attendance/cancel", h.authWrap(h.handleCancelClass))
	h.mux.HandleFunc("GET /widget/tab/summary", h.authWrap(h.handleTabSummary))
	h.mux.HandleFunc("GET /widget/export.xlsx", h.authWrap(h.handleExportXLSX))
	h.mux.HandleFunc("POST /widget/pin/login", h.handlePINLogin)
	h.mux.HandleFunc("POST /widget/pin/regenerate", h.teacherOnly(h.handleRegeneratePIN))
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleRoot serves the page. Element does not put the OpenID token in the
// widget URL: the page asks for it through the Widget API and then loads its
// content. So without a token in the request it serves an empty shell, which
// holds no data and needs no authorization. With an explicit token (links used
// for testing) the full page is rendered directly.
func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if credentialsFrom(r).token == "" {
		_ = h.tmpl.ExecuteTemplate(w, "layout.html", map[string]any{"Shell": true})
		return
	}
	h.authWrap(h.handleIndex)(w, r)
}

func (h *Handler) pageData(ctx context.Context, ident matrix.UserIdentity) map[string]any {
	cfg, _ := h.store.GetCourseConfig(ctx, ident.RoomID)
	if cfg.RoomID == "" {
		return h.buildSetupData(ctx, ident.RoomID, ident.UserID, cfg, false)
	}
	return h.buildAttendanceData(ctx, ident.RoomID, ident.UserID, cfg, h.defaultClassDate(cfg))
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	_ = h.tmpl.ExecuteTemplate(w, "layout.html", h.pageData(r.Context(), ident))
}

// handleHome renders the widget body that the shell loads once it has a token.
func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	_ = h.tmpl.ExecuteTemplate(w, "app.html", h.pageData(r.Context(), ident))
}
