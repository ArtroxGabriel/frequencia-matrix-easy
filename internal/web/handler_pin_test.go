package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
	"frequencia-matrix-integration/internal/web"
)

// disabledAttr matches the disabled attribute, not the disabled: CSS variant.
var disabledAttr = regexp.MustCompile(`\sdisabled[\s>]`)

const (
	pinRoomID  = "!course:matrix.dc.ufc.br"
	otherRoom  = "!other:matrix.dc.ufc.br"
	roomPIN    = "482916"
	wrongPIN   = "111111"
	pinTeacher = "@prof:matrix.dc.ufc.br"
	pinStudent = "@aluno:matrix.dc.ufc.br"
)

type pinEnv struct {
	store   *store.Store
	handler http.Handler
}

// newPINEnv builds a server whose room already has roomPIN, with a teacher and
// a student that can identify themselves through OpenID.
func newPINEnv(t *testing.T, opts ...web.Option) pinEnv {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	fake := matrix.NewFakeMatrixClient(false)
	fake.SetUserPowerLevel(pinRoomID, pinTeacher, 100)
	fake.SetUserPowerLevel(pinRoomID, pinStudent, 0)
	fake.RegisterOpenIDToken("teacher-token", matrix.UserIdentity{UserID: pinTeacher, RoomID: pinRoomID})
	fake.RegisterOpenIDToken("student-token", matrix.UserIdentity{UserID: pinStudent, RoomID: pinRoomID})

	if err := s.SetRoomPIN(context.Background(), pinRoomID, roomPIN, time.Now()); err != nil {
		t.Fatal(err)
	}
	h, err := web.NewHandler(s, fake, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return pinEnv{store: s, handler: h}
}

func (e pinEnv) do(method, path string, headers map[string]string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("HX-Request", "true")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e pinEnv) loginPath(room string) string {
	return "/widget/pin/login?roomId=" + url.QueryEscape(room)
}

func (e pinEnv) tab(room, name string, headers map[string]string) *httptest.ResponseRecorder {
	return e.do(http.MethodGet, "/widget/tab/"+name+"?roomId="+url.QueryEscape(room), headers, nil)
}

func (e pinEnv) login(room, pin string) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, e.loginPath(room), nil, url.Values{"pin": {pin}})
}

func (e pinEnv) sessionFor(t *testing.T, room, pin string) map[string]string {
	t.Helper()
	rec := e.login(room, pin)
	token := rec.Header().Get("X-Session-Token")
	if rec.Code != http.StatusOK || token == "" {
		t.Fatalf("login = %d, token %q: %s", rec.Code, token, rec.Body.String())
	}
	return map[string]string{"X-Session-Token": token}
}

func TestWithoutCredentialsTheRoomWithAPINGetsThePINForm(t *testing.T) {
	e := newPINEnv(t)
	rec := e.tab(pinRoomID, "roster", nil)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="pin-form"`) || strings.Contains(body, "Acesso restrito") {
		t.Errorf("expected the PIN form and not the restricted card, got: %s", body)
	}
}

func TestWithoutCredentialsAFullPageGetsTheFormInsideAnApp(t *testing.T) {
	e := newPINEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/widget/home?roomId="+url.QueryEscape(pinRoomID), nil)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req) // no HX-Request header

	body := rec.Body.String()
	if rec.Code != http.StatusForbidden || !strings.Contains(body, `id="app"`) || !strings.Contains(body, `id="pin-form"`) {
		t.Errorf("expected a full page with #app and the form, got %d: %s", rec.Code, body)
	}
}

func TestRoomWithoutAPINKeepsTheRestrictedCard(t *testing.T) {
	e := newPINEnv(t)
	rec := e.tab(otherRoom, "roster", nil)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Acesso restrito") {
		t.Errorf("expected the restricted card, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnIdentifiedStudentSeesTheCardNotThePINForm(t *testing.T) {
	e := newPINEnv(t)
	rec := e.tab(pinRoomID, "roster", map[string]string{"X-OpenID-Token": "student-token"})
	body := rec.Body.String()
	if rec.Code != http.StatusForbidden || !strings.Contains(body, "Acesso restrito") || strings.Contains(body, "pin-form") {
		t.Errorf("a verified student must get the card only, got %d: %s", rec.Code, body)
	}
}

func TestRightPINOpensASessionAndHomePage(t *testing.T) {
	e := newPINEnv(t)
	rec := e.login(pinRoomID, roomPIN)

	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Session-Token") == "" {
		t.Error("the session token is missing from the response header")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rec.Body.String(), `id="tab-content"`) || !strings.Contains(rec.Body.String(), "acesso por PIN") {
		t.Errorf("expected the widget home, got: %s", rec.Body.String())
	}
}

func TestSessionGivesTeacherAccessToItsRoomOnly(t *testing.T) {
	e := newPINEnv(t)
	session := e.sessionFor(t, pinRoomID, roomPIN)

	if rec := e.tab(pinRoomID, "roster", session); rec.Code != http.StatusOK {
		t.Errorf("own room = %d, want 200", rec.Code)
	}
	if rec := e.tab(otherRoom, "roster", session); rec.Code != http.StatusForbidden {
		t.Errorf("another room = %d, want 403", rec.Code)
	}
	if rec := e.tab(pinRoomID, "roster", map[string]string{"X-Session-Token": "forged"}); rec.Code != http.StatusForbidden {
		t.Errorf("forged token = %d, want 403", rec.Code)
	}
}

func TestSessionTokenIsNotAcceptedFromTheURL(t *testing.T) {
	e := newPINEnv(t)
	token := e.sessionFor(t, pinRoomID, roomPIN)["X-Session-Token"]
	rec := e.do(http.MethodGet, "/widget/tab/roster?roomId="+url.QueryEscape(pinRoomID)+"&session="+token, nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("token in the URL = %d, want 403", rec.Code)
	}
}

func TestWrongPINGetsTheFormBackWithoutAMessageOrToken(t *testing.T) {
	e := newPINEnv(t)
	rec := e.login(pinRoomID, wrongPIN)
	body := rec.Body.String()

	if rec.Code != http.StatusForbidden || rec.Header().Get("X-Session-Token") != "" {
		t.Fatalf("wrong PIN = %d, token %q", rec.Code, rec.Header().Get("X-Session-Token"))
	}
	if !strings.Contains(body, `id="pin-form"`) || strings.Contains(body, "Muitas tentativas") || disabledAttr.MatchString(body) {
		t.Errorf("expected a clean, enabled form, got: %s", body)
	}
}

func TestFiveWrongPINsLockTheFormEvenForTheRightPIN(t *testing.T) {
	e := newPINEnv(t)
	for i := 0; i < 4; i++ {
		e.login(pinRoomID, wrongPIN)
	}
	locked := e.login(pinRoomID, wrongPIN)
	if !strings.Contains(locked.Body.String(), "Muitas tentativas") || !disabledAttr.MatchString(locked.Body.String()) {
		t.Errorf("the lockout should show the note and disable the form, got: %s", locked.Body.String())
	}

	rec := e.login(pinRoomID, roomPIN)
	if rec.Code != http.StatusForbidden || rec.Header().Get("X-Session-Token") != "" {
		t.Errorf("right PIN while locked = %d, token %q; want 403 and no token", rec.Code, rec.Header().Get("X-Session-Token"))
	}
}

func TestLoginForARoomWithoutAPINIsDenied(t *testing.T) {
	e := newPINEnv(t)
	rec := e.login(otherRoom, roomPIN)
	if rec.Code != http.StatusForbidden || rec.Header().Get("X-Session-Token") != "" || !strings.Contains(rec.Body.String(), "Acesso restrito") {
		t.Errorf("got %d, token %q: %s", rec.Code, rec.Header().Get("X-Session-Token"), rec.Body.String())
	}
}

func TestTeacherGeneratesASixDigitPINThatWorks(t *testing.T) {
	e := newPINEnv(t)
	teacher := map[string]string{"X-OpenID-Token": "teacher-token"}
	rec := e.do(http.MethodPost, "/widget/pin/regenerate?roomId="+url.QueryEscape(pinRoomID), teacher, nil)

	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("regenerate = %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	m := regexp.MustCompile(`id="pin-value"[^>]*>(\d{6})<`).FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("no six-digit PIN in the response: %s", rec.Body.String())
	}
	if m[1] == roomPIN {
		t.Skip("drew the same PIN as before, one in a million")
	}
	if rec := e.login(pinRoomID, m[1]); rec.Code != http.StatusOK {
		t.Errorf("the new PIN does not log in: %d", rec.Code)
	}
	if rec := e.login(pinRoomID, roomPIN); rec.Code != http.StatusForbidden {
		t.Errorf("the old PIN still logs in: %d", rec.Code)
	}
}

func TestNewPINEndsSessionsOpenedWithTheOldOne(t *testing.T) {
	e := newPINEnv(t)
	session := e.sessionFor(t, pinRoomID, roomPIN)
	teacher := map[string]string{"X-OpenID-Token": "teacher-token"}
	e.do(http.MethodPost, "/widget/pin/regenerate?roomId="+url.QueryEscape(pinRoomID), teacher, nil)

	if rec := e.tab(pinRoomID, "roster", session); rec.Code != http.StatusForbidden {
		t.Errorf("old session after a new PIN = %d, want 403", rec.Code)
	}
}

func TestAPINSessionCannotReplaceThePIN(t *testing.T) {
	e := newPINEnv(t)
	session := e.sessionFor(t, pinRoomID, roomPIN)
	rec := e.do(http.MethodPost, "/widget/pin/regenerate?roomId="+url.QueryEscape(pinRoomID), session, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("regenerate with a PIN session = %d, want 403", rec.Code)
	}
	if rec := e.login(pinRoomID, roomPIN); rec.Code != http.StatusOK {
		t.Errorf("the PIN changed anyway: login = %d", rec.Code)
	}
}

func TestAStudentCannotGenerateAPIN(t *testing.T) {
	e := newPINEnv(t)
	rec := e.do(http.MethodPost, "/widget/pin/regenerate?roomId="+url.QueryEscape(pinRoomID),
		map[string]string{"X-OpenID-Token": "student-token"}, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("student regenerate = %d, want 403", rec.Code)
	}
	if rec := e.login(pinRoomID, roomPIN); rec.Code != http.StatusOK {
		t.Errorf("the PIN changed anyway: login = %d", rec.Code)
	}
}

func TestSetupTabOffersPINManagementOnlyToIdentifiedTeachers(t *testing.T) {
	e := newPINEnv(t)

	teacher := e.tab(pinRoomID, "setup", map[string]string{"X-OpenID-Token": "teacher-token"})
	if !strings.Contains(teacher.Body.String(), `id="pin-card"`) || !strings.Contains(teacher.Body.String(), "Gerar novo PIN") {
		t.Errorf("the teacher should manage the PIN (and see 'Gerar novo PIN' since one exists): %s", teacher.Body.String())
	}
	if !strings.Contains(teacher.Body.String(), "hx-confirm") {
		t.Error("replacing an existing PIN should ask for confirmation")
	}

	viaPIN := e.tab(pinRoomID, "setup", e.sessionFor(t, pinRoomID, roomPIN))
	if viaPIN.Code != http.StatusOK || strings.Contains(viaPIN.Body.String(), `id="pin-card"`) {
		t.Errorf("a PIN session must not see the PIN controls, got %d", viaPIN.Code)
	}
}

func TestFirstPINOffersGenerateWithoutConfirmation(t *testing.T) {
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	fake := matrix.NewFakeMatrixClient(false)
	fake.SetUserPowerLevel(otherRoom, pinTeacher, 100)
	fake.RegisterOpenIDToken("t", matrix.UserIdentity{UserID: pinTeacher, RoomID: otherRoom})
	h, err := web.NewHandler(s, fake)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/widget/tab/setup?roomId="+url.QueryEscape(otherRoom), nil)
	req.Header.Set("X-OpenID-Token", "t")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Gerar PIN") || strings.Contains(body, "hx-confirm") {
		t.Errorf("a room without PIN should offer 'Gerar PIN' without confirmation: %s", body)
	}
}

func TestPINSessionLastsAsConfigured(t *testing.T) {
	e := newPINEnv(t, web.WithPINSessionTTL(time.Millisecond))
	rec := e.login(pinRoomID, roomPIN)
	token := rec.Header().Get("X-Session-Token")
	if token == "" {
		t.Fatalf("login failed: %d", rec.Code)
	}
	if got := e.tab(pinRoomID, "roster", map[string]string{"X-Session-Token": token}); got.Code != http.StatusForbidden {
		t.Errorf("session past its configured lifetime = %d, want 403", got.Code)
	}
}

func TestNonPositivePINSessionTTLKeepsTheDefault(t *testing.T) {
	e := newPINEnv(t, web.WithPINSessionTTL(0), web.WithPINSessionTTL(-time.Hour))
	session := e.sessionFor(t, pinRoomID, roomPIN)
	if rec := e.tab(pinRoomID, "roster", session); rec.Code != http.StatusOK {
		t.Errorf("session with the default lifetime = %d, want 200", rec.Code)
	}
}

func TestOpenIDStillWorksWhenTheRoomHasAPIN(t *testing.T) {
	e := newPINEnv(t)
	rec := e.tab(pinRoomID, "roster", map[string]string{"X-OpenID-Token": "teacher-token"})
	if rec.Code != http.StatusOK {
		t.Errorf("teacher with OpenID = %d, want 200", rec.Code)
	}
}
