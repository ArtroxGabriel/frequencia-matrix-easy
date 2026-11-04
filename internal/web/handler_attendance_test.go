package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
)

const testRoom = "!course:matrix.dc.ufc.br"

type teacherEnv struct {
	store   *store.Store
	matrix  *matrix.FakeMatrixClient
	handler http.Handler
	auth    string
}

func newTeacherEnv(t *testing.T) *teacherEnv {
	t.Helper()
	s, fake, handler := setupTestServer(t)
	t.Cleanup(func() { _ = s.Close() })
	fake.SetUserPowerLevel(testRoom, "@prof:matrix.dc.ufc.br", 100)
	fake.RegisterOpenIDToken("teacher-token", matrix.UserIdentity{UserID: "@prof:matrix.dc.ufc.br", RoomID: testRoom})
	_ = s.SaveCourseConfig(context.Background(), domain.CourseConfig{
		RoomID: testRoom, CourseName: "SD", Weekdays: []time.Weekday{time.Tuesday, time.Thursday},
		TotalPeriods: 32, StartDate: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	})
	return &teacherEnv{store: s, matrix: fake, handler: handler, auth: "?roomId=" + url.QueryEscape(testRoom) + "&openIdToken=teacher-token"}
}

func (e *teacherEnv) addStudents(t *testing.T, names ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(names))
	for i, name := range names {
		id := "stu-" + string(rune('a'+i))
		stu := domain.Student{ID: id, RoomID: testRoom, FullName: name, Level: domain.LevelGraduation, Active: true}
		if err := e.store.AddStudent(context.Background(), stu); err != nil {
			t.Fatalf("failed adding student: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (e *teacherEnv) post(path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path+e.auth, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *teacherEnv) get(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestAttendanceOnlyMarkedStudentsAreAbsent(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana", "Bia", "Caio")

	rec := env.post("/widget/attendance", url.Values{
		"classDate": {"2026-03-03"}, "periodCount": {"2"}, "absent": {ids[1]},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	session, err := env.store.GetAttendanceSession(context.Background(), testRoom, "2026-03-03")
	if err != nil {
		t.Fatalf("session not saved: %v", err)
	}
	for _, e := range session.Entries {
		wantPresent := e.StudentID != ids[1]
		if e.IsPresent != wantPresent {
			t.Errorf("student %s: present=%v, want %v", e.StudentID, e.IsPresent, wantPresent)
		}
	}
}

func TestAttendanceWithNoMarksMeansEveryonePresent(t *testing.T) {
	env := newTeacherEnv(t)
	env.addStudents(t, "Ana", "Bia")

	env.post("/widget/attendance", url.Values{"classDate": {"2026-03-03"}, "periodCount": {"2"}})

	session, _ := env.store.GetAttendanceSession(context.Background(), testRoom, "2026-03-03")
	for _, e := range session.Entries {
		if !e.IsPresent {
			t.Errorf("student %s must be present by default", e.StudentID)
		}
	}
}

func TestAttendanceNoticeIsSentOnlyOnFirstSave(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana", "Bia")
	form := url.Values{"classDate": {"2026-03-03"}, "periodCount": {"2"}, "absent": {ids[0]}}

	env.post("/widget/attendance", form)
	env.post("/widget/attendance", form)

	notices := env.matrix.GetSentNotices()
	if len(notices) != 1 {
		t.Fatalf("expected exactly 1 notice, got %d", len(notices))
	}
	if !strings.Contains(notices[0].Message, "1 falta em 2 alunos") {
		t.Errorf("unexpected notice: %q", notices[0].Message)
	}
}

func TestAttendanceNoticeWithoutAbsences(t *testing.T) {
	env := newTeacherEnv(t)
	env.addStudents(t, "Ana")

	env.post("/widget/attendance", url.Values{"classDate": {"2026-03-03"}, "periodCount": {"1"}})

	notices := env.matrix.GetSentNotices()
	if len(notices) != 1 || !strings.Contains(notices[0].Message, "nenhuma falta em 1 alunos") {
		t.Errorf("unexpected notices: %+v", notices)
	}
}

func TestCancelledClassCountsAsNoAbsence(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana")
	env.post("/widget/attendance", url.Values{"classDate": {"2026-03-03"}, "periodCount": {"2"}, "absent": {ids[0]}})

	rec := env.post("/widget/attendance/cancel", url.Values{"classDate": {"2026-03-03"}, "cancelled": {"true"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Sem aula neste dia") {
		t.Fatalf("expected the cancelled state to render, got %d", rec.Code)
	}

	sessions, _ := env.store.ListAttendanceSessions(context.Background(), testRoom)
	summary := domain.CalculateStudentSummary(domain.Student{ID: ids[0]}, sessions, 32)
	if summary.Absences != 0 {
		t.Errorf("a cancelled class must not leave absences, got %d", summary.Absences)
	}
}

func TestReopenCancelledClass(t *testing.T) {
	env := newTeacherEnv(t)
	env.addStudents(t, "Ana")
	env.post("/widget/attendance/cancel", url.Values{"classDate": {"2026-03-05"}, "cancelled": {"true"}})

	rec := env.post("/widget/attendance/cancel", url.Values{"classDate": {"2026-03-05"}, "cancelled": {"false"}})

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Salvar chamada") {
		t.Fatalf("expected the roll call form back, got %d", rec.Code)
	}
	if _, err := env.store.GetAttendanceSession(context.Background(), testRoom, "2026-03-05"); err != store.ErrNotFound {
		t.Errorf("expected the cancelled session to be removed, got %v", err)
	}
}

func TestReopenDoesNotDeleteRecordedRollCall(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana")
	env.post("/widget/attendance", url.Values{"classDate": {"2026-03-03"}, "periodCount": {"2"}, "absent": {ids[0]}})

	env.post("/widget/attendance/cancel", url.Values{"classDate": {"2026-03-03"}, "cancelled": {"false"}})

	session, err := env.store.GetAttendanceSession(context.Background(), testRoom, "2026-03-03")
	if err != nil || session.Cancelled || len(session.Entries) != 1 {
		t.Errorf("a recorded roll call must survive a reopen request, got %+v (%v)", session, err)
	}
}

func TestAttendanceRejectsInvalidInputSilently(t *testing.T) {
	env := newTeacherEnv(t)
	env.addStudents(t, "Ana")

	for _, bad := range []string{"", "03/03/2026", "2026-02-30"} {
		rec := env.post("/widget/attendance", url.Values{"classDate": {bad}, "periodCount": {"2"}})
		if rec.Code != http.StatusUnprocessableEntity || rec.Body.Len() != 0 {
			t.Errorf("classDate %q: want empty 422, got %d with body %q", bad, rec.Code, rec.Body.String())
		}
		rec = env.post("/widget/attendance/cancel", url.Values{"classDate": {bad}, "cancelled": {"true"}})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("cancel with classDate %q: want 422, got %d", bad, rec.Code)
		}
	}
	if sessions, _ := env.store.ListAttendanceSessions(context.Background(), testRoom); len(sessions) != 0 {
		t.Errorf("invalid requests must not create sessions, got %d", len(sessions))
	}
}

func TestAttendanceTabListsScheduledDatesAndRecordedState(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana")
	env.post("/widget/attendance", url.Values{"classDate": {"2026-03-03"}, "periodCount": {"2"}, "absent": {ids[0]}})
	env.post("/widget/attendance/cancel", url.Values{"classDate": {"2026-03-05"}, "cancelled": {"true"}})

	body := env.get("/widget/tab/attendance" + env.auth + "&date=2026-03-03").Body.String()

	for _, want := range []string{"ter 03/03/2026 — ✓ registrada", "qui 05/03/2026 — sem aula", "Chamada salva", `data-base="0"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in attendance tab", want)
		}
	}
}

func TestAttendanceTabShowsAbsencesFromOtherDays(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana")
	env.post("/widget/attendance", url.Values{"classDate": {"2026-03-03"}, "periodCount": {"2"}, "absent": {ids[0]}})

	body := env.get("/widget/tab/attendance" + env.auth + "&date=2026-03-05").Body.String()

	if !strings.Contains(body, `data-base="2"`) || !strings.Contains(body, `data-limit="8"`) {
		t.Errorf("expected the 2 accumulated absences and the limit of 8 in the tab")
	}
}
