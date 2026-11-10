package web

import (
	"errors"
	"net/http"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
)

// renderPINForm answers 403 with the PIN form. Like the restricted-access
// card, requests made by the widget script (HTMX) get only the card.
func (h *Handler) renderPINForm(w http.ResponseWriter, r *http.Request, roomID string, locked bool) {
	w.WriteHeader(http.StatusForbidden)
	name := "pin.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "pin_card"
	}
	_ = h.tmpl.ExecuteTemplate(w, name, map[string]any{"RoomID": roomID, "Locked": locked})
}

// handlePINLogin opens a session when the PIN is right. It does not need a
// session or token: the PIN is the credential. A wrong PIN gets the form back,
// without any message.
func (h *Handler) handlePINLogin(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("roomId")
	token, err := h.store.LoginWithPIN(r.Context(), roomID, r.PostFormValue("pin"), h.pinSessionTTL, time.Now())
	switch {
	case err == nil:
		ident := matrix.UserIdentity{UserID: pinUserID, RoomID: roomID}
		data, dataErr := h.pageData(r.Context(), ident)
		if dataErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerSessionToken, token)
		w.Header().Set("Cache-Control", "no-store")
		_ = h.tmpl.ExecuteTemplate(w, "app.html", data)
	case errors.Is(err, store.ErrPINLocked):
		h.renderPINForm(w, r, roomID, true)
	case errors.Is(err, store.ErrPINInvalid):
		h.renderPINForm(w, r, roomID, false)
	case errors.Is(err, store.ErrNoPIN):
		h.renderUnauthorized(w, r, roomID, "")
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// handleRegeneratePIN creates a new PIN for the room and shows it once. The
// old PIN and every session opened with it stop working.
func (h *Handler) handleRegeneratePIN(w http.ResponseWriter, r *http.Request, ident matrix.UserIdentity) {
	pin, err := domain.NewPIN()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := h.store.SetRoomPIN(r.Context(), ident.RoomID, pin, time.Now()); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	_ = h.tmpl.ExecuteTemplate(w, "pin_manage", map[string]any{
		"RoomID": ident.RoomID,
		"PinSet": true,
		"Pin":    pin,
	})
}
