package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"frequencia-matrix-integration/internal/matrix"
)

func (e *teacherEnv) request(method, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func roomQuery() string {
	return "?roomId=" + url.QueryEscape(testRoom)
}

func TestRootWithoutTokenServesTheShell(t *testing.T) {
	env := newTeacherEnv(t)

	for _, target := range []string{"/", "/" + roomQuery() + "&widgetId=w1", "/" + roomQuery() + "&userId=%40prof%3Amatrix.dc.ufc.br"} {
		rec := env.request(http.MethodGet, target, nil)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, `data-shell="true"`) || !strings.Contains(body, "/static/widget.js") {
			t.Errorf("%s: expected the shell, got %d", target, rec.Code)
		}
		if strings.Contains(body, "Professor:") || strings.Contains(body, testRoom) {
			t.Errorf("%s: the shell must not contain any course or user data", target)
		}
	}
}

func TestRootWithExplicitTokenRendersFullPageWithoutLeakingIt(t *testing.T) {
	env := newTeacherEnv(t)

	rec := env.request(http.MethodGet, "/"+env.auth, nil)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Professor:") {
		t.Fatalf("expected the full page, got %d", rec.Code)
	}
	if strings.Contains(body, "teacher-token") || strings.Contains(body, "openIdToken") {
		t.Errorf("the token must never be written into the page")
	}
}

func TestHomeAndTabsAcceptTheTokenInAHeader(t *testing.T) {
	env := newTeacherEnv(t)
	header := map[string]string{"X-OpenID-Token": "teacher-token"}

	for _, path := range []string{"/widget/home", "/widget/tab/setup", "/widget/tab/roster", "/widget/tab/attendance", "/widget/tab/summary"} {
		rec := env.request(http.MethodGet, path+roomQuery(), header)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200 with a header token, got %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "openIdToken") {
			t.Errorf("%s: URLs in the page must not carry a token", path)
		}
	}
	home := env.request(http.MethodGet, "/widget/home"+roomQuery(), header).Body.String()
	if !strings.Contains(home, `id="tabs"`) || strings.Contains(home, "<html") {
		t.Errorf("/widget/home must be the widget body, not a whole document")
	}
}

func TestDevUserHeaderIsUsedWhenTheOpenIDTokenIsNotValid(t *testing.T) {
	env := newTeacherEnv(t)

	rec := env.request(http.MethodGet, "/widget/home"+roomQuery(), map[string]string{
		"X-OpenID-Token": "token-the-fake-does-not-know",
		"X-Dev-User":     "@prof:matrix.dc.ufc.br",
	})

	if rec.Code != http.StatusOK {
		t.Errorf("the development user must be accepted as a fallback, got %d", rec.Code)
	}
}

func TestInvalidCredentialsAreRejected(t *testing.T) {
	env := newTeacherEnv(t)

	for name, headers := range map[string]map[string]string{
		"no credentials":  nil,
		"unknown token":   {"X-OpenID-Token": "nope"},
		"unknown devUser": {"X-Dev-User": "stranger"},
	} {
		rec := env.request(http.MethodGet, "/widget/home"+roomQuery(), headers)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403, got %d", name, rec.Code)
		}
	}
}

func TestStudentGetsTheRestrictedCardInsideThePage(t *testing.T) {
	env := newTeacherEnv(t)
	env.matrix.SetUserPowerLevel(testRoom, "@aluno:matrix.dc.ufc.br", 0)
	env.matrix.RegisterOpenIDToken("student-token", matrix.UserIdentity{UserID: "@aluno:matrix.dc.ufc.br", RoomID: testRoom})

	partial := env.request(http.MethodGet, "/widget/home"+roomQuery(), map[string]string{"X-OpenID-Token": "student-token", "HX-Request": "true"})
	if partial.Code != http.StatusForbidden || !strings.Contains(partial.Body.String(), "Acesso restrito ao professor") {
		t.Fatalf("expected the restricted card with 403, got %d", partial.Code)
	}
	if strings.Contains(partial.Body.String(), "<html") {
		t.Errorf("a widget request must get only the card, not a whole document")
	}

	full := env.request(http.MethodGet, "/widget/home"+roomQuery(), map[string]string{"X-OpenID-Token": "student-token"})
	if !strings.Contains(full.Body.String(), "<html") {
		t.Errorf("a plain request gets the whole document")
	}
}

func TestStaticScriptAndUnknownPaths(t *testing.T) {
	env := newTeacherEnv(t)

	script := env.request(http.MethodGet, "/static/widget.js", nil)
	if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), "get_openid") {
		t.Errorf("expected the widget script, got %d", script.Code)
	}
	if rec := env.request(http.MethodGet, "/favicon.ico", nil); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown path must be 404, not an authorization error, got %d", rec.Code)
	}
}

func TestSummaryExportIsAScriptDrivenDownload(t *testing.T) {
	env := newTeacherEnv(t)

	body := env.request(http.MethodGet, "/widget/tab/summary"+roomQuery(), map[string]string{"X-OpenID-Token": "teacher-token"}).Body.String()

	if !strings.Contains(body, `data-download="/widget/export.xlsx?roomId=`) {
		t.Errorf("the export must be a download button so the token can travel in a header")
	}
	xlsx := env.request(http.MethodGet, "/widget/export.xlsx"+roomQuery(), map[string]string{"X-OpenID-Token": "teacher-token"})
	if xlsx.Code != http.StatusOK {
		t.Errorf("expected the export to work with a header token, got %d", xlsx.Code)
	}
}

func TestSetupRejectsInvalidInputSilently(t *testing.T) {
	env := newTeacherEnv(t)
	valid := url.Values{"courseName": {"Redes"}, "weekdays": {"1", "3"}, "totalPeriods": {"40"}, "startDate": {"2026-08-03"}}

	bad := map[string]func(url.Values){
		"no weekdays":          func(v url.Values) { v.Del("weekdays") },
		"weekday out of range": func(v url.Values) { v.Set("weekdays", "9") },
		"zero periods":         func(v url.Values) { v.Set("totalPeriods", "0") },
		"periods not a number": func(v url.Values) { v.Set("totalPeriods", "abc") },
		"bad date":             func(v url.Values) { v.Set("startDate", "03/08/2026") },
		"empty name":           func(v url.Values) { v.Set("courseName", "   ") },
	}
	for name, mutate := range bad {
		form := url.Values{}
		for k, v := range valid {
			form[k] = v
		}
		mutate(form)
		rec := env.post("/widget/setup", form)
		if rec.Code != http.StatusUnprocessableEntity || rec.Body.Len() != 0 {
			t.Errorf("%s: want empty 422, got %d with body %q", name, rec.Code, rec.Body.String())
		}
	}
	cfg, _ := env.store.GetCourseConfig(context.Background(), testRoom)
	if cfg.CourseName != "SD" {
		t.Errorf("rejected requests must leave the saved course untouched, got %+v", cfg)
	}
}

func TestSetupSavesAndShowsSavedState(t *testing.T) {
	env := newTeacherEnv(t)
	form := url.Values{"courseName": {" Redes "}, "weekdays": {"3", "1", "1"}, "totalPeriods": {"40"}, "startDate": {"2026-08-03"}}

	rec := env.post("/widget/setup", form)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Configurações salvas") {
		t.Fatalf("expected the saved state, got %d", rec.Code)
	}
	cfg, _ := env.store.GetCourseConfig(context.Background(), testRoom)
	if cfg.CourseName != "Redes" || cfg.TotalPeriods != 40 || len(cfg.Weekdays) != 2 || cfg.Weekdays[0] != 1 || cfg.Weekdays[1] != 3 {
		t.Errorf("unexpected course saved: %+v", cfg)
	}
}
