package web

import (
	"context"
	"net/http"
	"time"

	"frequencia-matrix-integration/internal/matrix"
)

const (
	headerOpenIDToken = "X-OpenID-Token"
	headerDevUser     = "X-Dev-User"
	// headerSessionToken carries the token of a session opened with the room
	// PIN. It is never accepted from the URL.
	headerSessionToken = "X-Session-Token"

	// pinUserID is the identity of a PIN session. It cannot be a Matrix id,
	// which always starts with "@".
	pinUserID = "acesso por PIN"
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
	// session is a PIN session token, from the X-Session-Token header.
	session string
}

func credentialsFrom(r *http.Request) credentials {
	q := r.URL.Query()
	return credentials{
		roomID:  q.Get("roomId"),
		token:   firstNonEmpty(r.Header.Get(headerOpenIDToken), q.Get("openIdToken")),
		devUser: firstNonEmpty(r.Header.Get(headerDevUser), q.Get("userId")),
		session: r.Header.Get(headerSessionToken),
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

// authWrap lets only teachers in: room moderators and creators, identified by
// an OpenID token, or whoever opened a session with the room PIN.
func (h *Handler) authWrap(next authHandlerFunc) http.HandlerFunc {
	return h.authenticate(next, true)
}

// teacherOnly is authWrap without PIN sessions. It guards what a PIN must not
// be able to do, such as replacing the PIN.
func (h *Handler) teacherOnly(next authHandlerFunc) http.HandlerFunc {
	return h.authenticate(next, false)
}

func (h *Handler) authenticate(next authHandlerFunc, allowPIN bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		creds := credentialsFrom(r)
		if allowPIN && creds.token == "" {
			if ident, ok := h.pinIdentity(r.Context(), creds); ok {
				next(w, r, ident)
				return
			}
		}

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

// pinIdentity accepts a PIN session, but only for the room it was opened for.
func (h *Handler) pinIdentity(ctx context.Context, c credentials) (matrix.UserIdentity, bool) {
	if c.session == "" || c.roomID == "" {
		return matrix.UserIdentity{}, false
	}
	room, err := h.store.PINSessionRoom(ctx, c.session, time.Now())
	if err != nil || room != c.roomID {
		return matrix.UserIdentity{}, false
	}
	return matrix.UserIdentity{UserID: pinUserID, RoomID: room}, true
}

func (h *Handler) roomHasPIN(ctx context.Context, roomID string) bool {
	has, err := h.store.HasRoomPIN(ctx, roomID)
	return err == nil && has
}

// renderUnauthorized answers 403. Requests made by the widget script (HTMX) get
// only the card, so it can be placed inside the page.
//
// When nobody was identified and the room has a PIN, it offers the PIN form
// instead: that is how clients without OpenID support get in. Someone who was
// identified but is not a teacher, such as a student, only sees the card.
func (h *Handler) renderUnauthorized(w http.ResponseWriter, r *http.Request, roomID, userID string) {
	if userID == "" && h.roomHasPIN(r.Context(), roomID) {
		h.renderPINForm(w, r, roomID, false)
		return
	}
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
