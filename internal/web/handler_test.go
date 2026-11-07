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
	"frequencia-matrix-integration/internal/web"
)

func setupTestServer(t *testing.T) (*store.Store, *matrix.FakeMatrixClient, http.Handler) {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}

	fakeMatrix := matrix.NewFakeMatrixClient(true)
	handler, err := web.NewHandler(s, fakeMatrix)
	if err != nil {
		t.Fatalf("failed creating web handler: %v", err)
	}
	return s, fakeMatrix, handler
}

func TestHandlerUnauthorized(t *testing.T) {
	s, fakeMatrix, handler := setupTestServer(t)
	defer func() { _ = s.Close() }()

	roomID := "!course:matrix.dc.ufc.br"
	studentID := "@student:matrix.dc.ufc.br"

	fakeMatrix.SetUserPowerLevel(roomID, studentID, 0)
	fakeMatrix.RegisterOpenIDToken("student-token", matrix.UserIdentity{
		UserID: studentID,
		RoomID: roomID,
	})

	req := httptest.NewRequest(http.MethodGet, "/?roomId="+url.QueryEscape(roomID)+"&openIdToken=student-token", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for student, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Acesso restrito ao professor") {
		t.Errorf("expected unauthorized message in body, got: %s", rec.Body.String())
	}
}

func TestHandlerCourseSetup(t *testing.T) {
	s, fakeMatrix, handler := setupTestServer(t)
	defer func() { _ = s.Close() }()

	roomID := "!course:matrix.dc.ufc.br"
	teacherID := "@prof:matrix.dc.ufc.br"

	fakeMatrix.SetUserPowerLevel(roomID, teacherID, 100)
	fakeMatrix.RegisterOpenIDToken("teacher-token", matrix.UserIdentity{
		UserID: teacherID,
		RoomID: roomID,
	})

	form := url.Values{
		"courseName":   {"Sistemas Distribuídos"},
		"weekdays":     {"2", "4"}, // Tuesday, Thursday
		"totalPeriods": {"32"},
		"startDate":    {"2026-03-03"},
	}

	req := httptest.NewRequest(http.MethodPost, "/widget/setup?roomId="+url.QueryEscape(roomID)+"&openIdToken=teacher-token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	cfg, err := s.GetCourseConfig(context.Background(), roomID)
	if err != nil {
		t.Fatalf("expected course config to be saved in store: %v", err)
	}
	if cfg.CourseName != "Sistemas Distribuídos" || cfg.TotalPeriods != 32 {
		t.Errorf("unexpected saved config: %+v", cfg)
	}
}

func TestHandlerStudentAndAttendanceWorkflow(t *testing.T) {
	s, fakeMatrix, handler := setupTestServer(t)
	defer func() { _ = s.Close() }()

	roomID := "!course:matrix.dc.ufc.br"
	teacherID := "@prof:matrix.dc.ufc.br"
	fakeMatrix.SetUserPowerLevel(roomID, teacherID, 100)
	fakeMatrix.RegisterOpenIDToken("teacher-token", matrix.UserIdentity{
		UserID: teacherID,
		RoomID: roomID,
	})

	// Setup course
	_ = s.SaveCourseConfig(context.Background(), domain.CourseConfig{
		RoomID:       roomID,
		CourseName:   "Sistemas Distribuídos",
		Weekdays:     []time.Weekday{time.Tuesday, time.Thursday},
		TotalPeriods: 32,
		StartDate:    time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	})

	authQuery := "?roomId=" + url.QueryEscape(roomID) + "&openIdToken=teacher-token"

	// 1. Add student
	addForm := url.Values{
		"fullName":           {"Érika Meireles"},
		"registrationNumber": {"5011"},
		"level":              {string(domain.LevelGraduation)},
	}
	reqAdd := httptest.NewRequest(http.MethodPost, "/widget/students"+authQuery, strings.NewReader(addForm.Encode()))
	reqAdd.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recAdd := httptest.NewRecorder()
	handler.ServeHTTP(recAdd, reqAdd)

	if recAdd.Code != http.StatusOK {
		t.Fatalf("failed adding student: %d - %s", recAdd.Code, recAdd.Body.String())
	}

	students, err := s.ListStudents(context.Background(), roomID, true)
	if err != nil || len(students) != 1 {
		t.Fatalf("expected 1 student in store, got %d (err: %v)", len(students), err)
	}
	stuID := students[0].ID

	// 2. Record attendance
	attForm := url.Values{
		"classDate":   {"2026-03-03"},
		"periodCount": {"2"},
	}
	reqAtt := httptest.NewRequest(http.MethodPost, "/widget/attendance"+authQuery, strings.NewReader(attForm.Encode()))
	reqAtt.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recAtt := httptest.NewRecorder()
	handler.ServeHTTP(recAtt, reqAtt)

	if recAtt.Code != http.StatusOK {
		t.Fatalf("failed recording attendance: %d - %s", recAtt.Code, recAtt.Body.String())
	}

	// Verify notice sent
	notices := fakeMatrix.GetSentNotices()
	if len(notices) != 1 {
		t.Errorf("expected 1 room notice sent, got %d", len(notices))
	}

	// 3. Export XLSX
	reqExport := httptest.NewRequest(http.MethodGet, "/widget/export.xlsx"+authQuery, nil)
	recExport := httptest.NewRecorder()
	handler.ServeHTTP(recExport, reqExport)

	if recExport.Code != http.StatusOK {
		t.Fatalf("failed exporting xlsx: %d", recExport.Code)
	}
	if recExport.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Errorf("unexpected content type: %s", recExport.Header().Get("Content-Type"))
	}

	// 4. Deactivate student
	reqDel := httptest.NewRequest(http.MethodDelete, "/widget/students/"+stuID+authQuery, nil)
	recDel := httptest.NewRecorder()
	handler.ServeHTTP(recDel, reqDel)

	if recDel.Code != http.StatusOK {
		t.Fatalf("failed deleting student: %d", recDel.Code)
	}
	activeStudents, _ := s.ListStudents(context.Background(), roomID, true)
	if len(activeStudents) != 0 {
		t.Errorf("expected 0 active students after deletion, got %d", len(activeStudents))
	}
}

func TestHandlerTabsAndViews(t *testing.T) {
	s, fakeMatrix, handler := setupTestServer(t)
	defer func() { _ = s.Close() }()

	roomID := "!course:matrix.dc.ufc.br"
	teacherID := "@prof:matrix.dc.ufc.br"
	fakeMatrix.SetUserPowerLevel(roomID, teacherID, 100)
	fakeMatrix.RegisterOpenIDToken("teacher-token", matrix.UserIdentity{
		UserID: teacherID,
		RoomID: roomID,
	})

	authQuery := "?roomId=" + url.QueryEscape(roomID) + "&openIdToken=teacher-token"

	// 1. Healthz
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Errorf("expected 200 for healthz, got %d", recHealth.Code)
	}

	// 2. Index (before setup)
	reqIndex := httptest.NewRequest(http.MethodGet, "/"+authQuery, nil)
	recIndex := httptest.NewRecorder()
	handler.ServeHTTP(recIndex, reqIndex)
	if recIndex.Code != http.StatusOK {
		t.Errorf("expected 200 for index, got %d", recIndex.Code)
	}

	// 3. Setup Tab
	reqSetupTab := httptest.NewRequest(http.MethodGet, "/widget/tab/setup"+authQuery, nil)
	recSetupTab := httptest.NewRecorder()
	handler.ServeHTTP(recSetupTab, reqSetupTab)
	if recSetupTab.Code != http.StatusOK {
		t.Errorf("expected 200 for setup tab, got %d", recSetupTab.Code)
	}

	// 4. Roster Tab
	reqRosterTab := httptest.NewRequest(http.MethodGet, "/widget/tab/roster"+authQuery, nil)
	recRosterTab := httptest.NewRecorder()
	handler.ServeHTTP(recRosterTab, reqRosterTab)
	if recRosterTab.Code != http.StatusOK {
		t.Errorf("expected 200 for roster tab, got %d", recRosterTab.Code)
	}

	// 5. Attendance Tab
	reqAttTab := httptest.NewRequest(http.MethodGet, "/widget/tab/attendance"+authQuery, nil)
	recAttTab := httptest.NewRecorder()
	handler.ServeHTTP(recAttTab, reqAttTab)
	if recAttTab.Code != http.StatusOK {
		t.Errorf("expected 200 for attendance tab, got %d", recAttTab.Code)
	}

	// 6. Summary Tab
	reqSummaryTab := httptest.NewRequest(http.MethodGet, "/widget/tab/summary"+authQuery, nil)
	recSummaryTab := httptest.NewRecorder()
	handler.ServeHTTP(recSummaryTab, reqSummaryTab)
	if recSummaryTab.Code != http.StatusOK {
		t.Errorf("expected 200 for summary tab, got %d", recSummaryTab.Code)
	}
}

func TestHandlerErrorBranches(t *testing.T) {
	s, fakeMatrix, handler := setupTestServer(t)
	defer func() { _ = s.Close() }()

	roomID := "!course:matrix.dc.ufc.br"
	teacherID := "@prof:matrix.dc.ufc.br"
	fakeMatrix.SetUserPowerLevel(roomID, teacherID, 100)
	fakeMatrix.RegisterOpenIDToken("teacher-token", matrix.UserIdentity{
		UserID: teacherID,
		RoomID: roomID,
	})
	authQuery := "?roomId=" + url.QueryEscape(roomID) + "&openIdToken=teacher-token"

	// Add student with invalid name
	badStuForm := url.Values{
		"fullName": {"   "},
		"level":    {string(domain.LevelGraduation)},
	}
	reqBadStu := httptest.NewRequest(http.MethodPost, "/widget/students"+authQuery, strings.NewReader(badStuForm.Encode()))
	reqBadStu.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recBadStu := httptest.NewRecorder()
	handler.ServeHTTP(recBadStu, reqBadStu)
	if recBadStu.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for whitespace name, got %d", recBadStu.Code)
	}

	// Attendance with default period count when periodCount <= 0
	attForm := url.Values{
		"classDate":   {"2026-03-03"},
		"periodCount": {"0"},
	}
	reqAtt := httptest.NewRequest(http.MethodPost, "/widget/attendance"+authQuery, strings.NewReader(attForm.Encode()))
	reqAtt.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recAtt := httptest.NewRecorder()
	handler.ServeHTTP(recAtt, reqAtt)
	if recAtt.Code != http.StatusOK {
		t.Errorf("expected 200 OK for attendance with default periodCount, got %d", recAtt.Code)
	}

	// Verify unauthorized access then immediate index access works without template panic
	reqUnauth := httptest.NewRequest(http.MethodGet, "/?roomId="+url.QueryEscape(roomID)+"&openIdToken=invalid-token", nil)
	recUnauth := httptest.NewRecorder()
	handler.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for invalid token, got %d", recUnauth.Code)
	}

	reqIndexAfter := httptest.NewRequest(http.MethodGet, "/"+authQuery, nil)
	recIndexAfter := httptest.NewRecorder()
	handler.ServeHTTP(recIndexAfter, reqIndexAfter)
	if recIndexAfter.Code != http.StatusOK {
		t.Errorf("expected 200 OK for index after unauthorized request, got %d", recIndexAfter.Code)
	}
}
