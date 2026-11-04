package web

import (
	"context"
	"net/http"

	"frequencia-matrix-integration/internal/matrix"
)

const (
	headerOpenIDToken = "X-OpenID-Token"
	headerDevUser     = "X-Dev-User"
)

// credentials are what a request offers to prove who is calling.
type credentials struct {
	roomID string
	// token is a Matrix OpenID token, from the X-OpenID-Token header (sent by
	// the widget script) or, for direct links, the openIdToken query parameter.
	token string
	// devUser is the user id given in the userId query parameter or the
	// X-Dev-User header. It is only meaningful to the development gateway,
	// which accepts it as an identity; a real homeserver rejects it.
	devUser string
}

func credentialsFrom(r *http.Request) credentials {
	q := r.URL.Query()
	return credentials{
		roomID:  q.Get("roomId"),
		token:   firstNonEmpty(r.Header.Get(headerOpenIDToken), q.Get("openIdToken")),
		devUser: firstNonEmpty(r.Header.Get(headerDevUser), q.Get("userId")),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// verifyUser checks the OpenID token and, if that fails, the development user.
func (h *Handler) verifyUser(ctx context.Context, c credentials) (matrix.UserIdentity, error) {
	ident, err := h.matrix.VerifyUser(ctx, c.roomID, c.token)
	if err == nil || c.devUser == "" || c.devUser == c.token {
		return ident, err
	}
	return h.matrix.VerifyUser(ctx, c.roomID, c.devUser)
}

type authHandlerFunc func(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity)

// authWrap lets only teachers (room moderators, or creators of the room) in.
func (h *Handler) authWrap(next authHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		creds := credentialsFrom(r)
		ident, err := h.verifyUser(r.Context(), creds)
		if err != nil {
			h.renderUnauthorized(w, r, creds.roomID, creds.devUser)
			return
		}

		isTeacher, err := h.matrix.IsTeacher(r.Context(), ident.RoomID, ident.UserID)
		if err != nil || !isTeacher {
			h.renderUnauthorized(w, r, ident.RoomID, ident.UserID)
			return
		}

		next(w, r, ident)
	}
}

// renderUnauthorized answers 403. Requests made by the widget script (HTMX) get
// only the card, so it can be placed inside the page.
func (h *Handler) renderUnauthorized(w http.ResponseWriter, r *http.Request, roomID, userID string) {
	if userID == "" {
		userID = "—"
	}
	w.WriteHeader(http.StatusForbidden)
	name := "unauthorized.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "unauthorized_card"
	}
	data := map[string]string{"RoomID": roomID, "UserID": userID}
	_ = h.tmpl.ExecuteTemplate(w, name, data)
}
