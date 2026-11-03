package domain_test

import (
	"testing"
	"time"

	"frequencia-matrix-integration/internal/domain"
)

func TestCourseConfigValidate(t *testing.T) {
	valid := domain.CourseConfig{
		RoomID:       "!course:matrix.dc.ufc.br",
		CourseName:   "Sistemas Distribuídos",
		Weekdays:     []time.Weekday{time.Tuesday, time.Thursday},
		TotalPeriods: 32,
		StartDate:    time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	invalidRoom := valid
	invalidRoom.RoomID = ""
	if err := invalidRoom.Validate(); err == nil {
		t.Errorf("expected error for empty RoomID, got nil")
	}

	invalidPeriods := valid
	invalidPeriods.TotalPeriods = 0
	if err := invalidPeriods.Validate(); err == nil {
		t.Errorf("expected error for zero TotalPeriods, got nil")
	}

	invalidWeekdays := valid
	invalidWeekdays.Weekdays = nil
	if err := invalidWeekdays.Validate(); err == nil {
		t.Errorf("expected error for nil Weekdays, got nil")
	}
}

func TestStudentValidate(t *testing.T) {
	validGrad := domain.Student{
		ID:                 "stu-1",
		RoomID:             "!course:matrix.dc.ufc.br",
		FullName:           "Érika Alcantara",
		RegistrationNumber: "501234",
		Level:              domain.LevelGraduation,
		Active:             true,
	}

	if err := validGrad.Validate(); err != nil {
		t.Fatalf("expected valid student, got error: %v", err)
	}

	invalidLevel := validGrad
	invalidLevel.Level = domain.StudentLevel("UNKNOWN")
	if err := invalidLevel.Validate(); err == nil {
		t.Errorf("expected error for invalid level, got nil")
	}

	emptyName := validGrad
	emptyName.FullName = "   "
	if err := emptyName.Validate(); err == nil {
		t.Errorf("expected error for whitespace FullName, got nil")
	}
}

func TestAttendanceSessionValidate(t *testing.T) {
	valid := domain.AttendanceSession{
		ID:          "sess-1",
		RoomID:      "!room:matrix.dc.ufc.br",
		ClassDate:   "2026-03-03",
		PeriodCount: 2,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid session, got: %v", err)
	}

	emptyRoom := valid
	emptyRoom.RoomID = ""
	if err := emptyRoom.Validate(); err == nil {
		t.Errorf("expected error for empty RoomID")
	}

	emptyDate := valid
	emptyDate.ClassDate = " "
	if err := emptyDate.Validate(); err == nil {
		t.Errorf("expected error for empty ClassDate")
	}

	invalidPeriods := valid
	invalidPeriods.PeriodCount = 0
	if err := invalidPeriods.Validate(); err == nil {
		t.Errorf("expected error for non-positive PeriodCount")
	}
}
