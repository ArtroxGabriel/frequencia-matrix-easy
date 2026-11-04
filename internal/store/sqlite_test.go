package store_test

import (
	"context"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/store"
)

func TestStoreCourseConfig(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	defer func() { _ = s.Close() }()

	roomID := "!course1:matrix.dc.ufc.br"

	// Non-existent course
	_, err = s.GetCourseConfig(ctx, roomID)
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	cfg := domain.CourseConfig{
		RoomID:       roomID,
		CourseName:   "Sistemas Distribuídos",
		Weekdays:     []time.Weekday{time.Tuesday, time.Thursday},
		TotalPeriods: 64,
		StartDate:    time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	}

	if err := s.SaveCourseConfig(ctx, cfg); err != nil {
		t.Fatalf("failed to save course config: %v", err)
	}

	saved, err := s.GetCourseConfig(ctx, roomID)
	if err != nil {
		t.Fatalf("failed to get course config: %v", err)
	}
	if saved.CourseName != cfg.CourseName || saved.TotalPeriods != 64 {
		t.Errorf("unexpected course config retrieved: %+v", saved)
	}
	if len(saved.Weekdays) != 2 || saved.Weekdays[0] != time.Tuesday {
		t.Errorf("unexpected weekdays retrieved: %+v", saved.Weekdays)
	}

	// Update existing course
	cfg.TotalPeriods = 32
	if err := s.SaveCourseConfig(ctx, cfg); err != nil {
		t.Fatalf("failed to update course config: %v", err)
	}
	updated, err := s.GetCourseConfig(ctx, roomID)
	if err != nil || updated.TotalPeriods != 32 {
		t.Errorf("expected updated TotalPeriods = 32, got %d (err: %v)", updated.TotalPeriods, err)
	}
}

func TestStoreStudents(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	roomID := "!course2:matrix.dc.ufc.br"

	s1 := domain.Student{
		ID:                 "stu-1",
		RoomID:             roomID,
		FullName:           "Érika Meireles",
		RegistrationNumber: "5001",
		Level:              domain.LevelGraduation,
		Active:             true,
	}
	s2 := domain.Student{
		ID:                 "stu-2",
		RoomID:             roomID,
		FullName:           "Bruno Rocha",
		RegistrationNumber: "5002",
		Level:              domain.LevelPosGraduation,
		Active:             true,
	}

	if err := s.AddStudent(ctx, s1); err != nil {
		t.Fatalf("failed to add student 1: %v", err)
	}
	if err := s.AddStudent(ctx, s2); err != nil {
		t.Fatalf("failed to add student 2: %v", err)
	}

	list, err := s.ListStudents(ctx, roomID, true)
	if err != nil {
		t.Fatalf("failed to list students: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 active students, got %d", len(list))
	}

	// Deactivate s1
	if err := s.DeactivateStudent(ctx, roomID, s1.ID); err != nil {
		t.Fatalf("failed to deactivate student: %v", err)
	}

	activeList, err := s.ListStudents(ctx, roomID, true)
	if err != nil || len(activeList) != 1 {
		t.Errorf("expected 1 active student after deactivation, got %d", len(activeList))
	}

	allList, err := s.ListStudents(ctx, roomID, false)
	if err != nil || len(allList) != 2 {
		t.Errorf("expected 2 total students when activeOnly=false, got %d", len(allList))
	}
}

func TestStoreAttendanceSessions(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	roomID := "!course3:matrix.dc.ufc.br"

	s1 := domain.Student{
		ID:       "stu-1",
		RoomID:   roomID,
		FullName: "Carlos Lima",
		Level:    domain.LevelGraduation,
		Active:   true,
	}
	if err := s.AddStudent(ctx, s1); err != nil {
		t.Fatalf("failed to add student: %v", err)
	}

	session1 := domain.AttendanceSession{
		ID:          "sess-1",
		RoomID:      roomID,
		ClassDate:   "2026-03-03",
		PeriodCount: 2,
		Entries: []domain.AttendanceEntry{
			{StudentID: "stu-1", IsPresent: true},
		},
	}

	if err := s.RecordAttendanceSession(ctx, session1); err != nil {
		t.Fatalf("failed to record session: %v", err)
	}

	sessions, err := s.ListAttendanceSessions(ctx, roomID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions[0].Entries) != 1 || !sessions[0].Entries[0].IsPresent {
		t.Errorf("unexpected entries in retrieved session: %+v", sessions[0])
	}

	// Update session for same date
	session1Updated := session1
	session1Updated.Entries = []domain.AttendanceEntry{
		{StudentID: "stu-1", IsPresent: false},
	}
	if err := s.RecordAttendanceSession(ctx, session1Updated); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	updatedSess, err := s.GetAttendanceSession(ctx, roomID, "2026-03-03")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if updatedSess.Entries[0].IsPresent {
		t.Errorf("expected IsPresent=false after update, got true")
	}
}

func TestStoreErrorCases(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Deactivate non-existent student
	if err := s.DeactivateStudent(ctx, "!room:matrix.org", "no-id"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent student, got %v", err)
	}

	// Invalid course config
	invalidCfg := domain.CourseConfig{}
	if err := s.SaveCourseConfig(ctx, invalidCfg); err == nil {
		t.Errorf("expected error saving invalid config")
	}

	// Invalid student
	invalidStu := domain.Student{}
	if err := s.AddStudent(ctx, invalidStu); err == nil {
		t.Errorf("expected error adding invalid student")
	}

	// Invalid session
	invalidSess := domain.AttendanceSession{}
	if err := s.RecordAttendanceSession(ctx, invalidSess); err == nil {
		t.Errorf("expected error recording invalid session")
	}

	// Get non-existent session
	if _, err := s.GetAttendanceSession(ctx, "!room:matrix.org", "2026-01-01"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent session, got %v", err)
	}
}

func newMemoryStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func addStudent(t *testing.T, s *store.Store, room, id string) {
	t.Helper()
	stu := domain.Student{ID: id, RoomID: room, FullName: "Aluno " + id, Level: domain.LevelGraduation, Active: true}
	if err := s.AddStudent(context.Background(), stu); err != nil {
		t.Fatalf("failed adding student %s: %v", id, err)
	}
}

func TestStoreCancelledSessionRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newMemoryStore(t)
	room := "!course:matrix.dc.ufc.br"
	addStudent(t, s, room, "stu-1")

	cancelled := domain.AttendanceSession{RoomID: room, ClassDate: "2026-03-05", Cancelled: true}
	if err := s.RecordAttendanceSession(ctx, cancelled); err != nil {
		t.Fatalf("expected a cancelled session without periods to be stored: %v", err)
	}

	got, err := s.GetAttendanceSession(ctx, room, "2026-03-05")
	if err != nil || !got.Cancelled || len(got.Entries) != 0 {
		t.Fatalf("expected a cancelled session with no entries, got %+v (err: %v)", got, err)
	}

	reopened := domain.AttendanceSession{
		RoomID: room, ClassDate: "2026-03-05", PeriodCount: 2,
		Entries: []domain.AttendanceEntry{{StudentID: "stu-1", IsPresent: false}},
	}
	if err := s.RecordAttendanceSession(ctx, reopened); err != nil {
		t.Fatalf("failed overwriting cancelled session: %v", err)
	}
	all, _ := s.ListAttendanceSessions(ctx, room)
	if len(all) != 1 || all[0].Cancelled || len(all[0].Entries) != 1 {
		t.Errorf("expected the session to be a regular one again, got %+v", all)
	}
}

func TestStoreDeleteAttendanceSession(t *testing.T) {
	ctx := context.Background()
	s := newMemoryStore(t)
	room := "!course:matrix.dc.ufc.br"
	addStudent(t, s, room, "stu-1")

	session := domain.AttendanceSession{
		RoomID: room, ClassDate: "2026-03-03", PeriodCount: 2,
		Entries: []domain.AttendanceEntry{{StudentID: "stu-1", IsPresent: true}},
	}
	if err := s.RecordAttendanceSession(ctx, session); err != nil {
		t.Fatalf("failed recording session: %v", err)
	}

	if err := s.DeleteAttendanceSession(ctx, room, "2026-03-03"); err != nil {
		t.Fatalf("failed deleting session: %v", err)
	}
	if _, err := s.GetAttendanceSession(ctx, room, "2026-03-03"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
	if err := s.DeleteAttendanceSession(ctx, room, "2026-03-03"); err != nil {
		t.Errorf("deleting a missing session must be a no-op, got %v", err)
	}
}
