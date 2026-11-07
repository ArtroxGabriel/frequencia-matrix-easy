package domain

import (
	"fmt"
	"strings"
	"time"
)

type StudentLevel string

const (
	LevelGraduation    StudentLevel = "GRADUATION"
	LevelPosGraduation StudentLevel = "POS_GRADUATION"
)

func (l StudentLevel) IsValid() bool {
	return l == LevelGraduation || l == LevelPosGraduation
}

type CourseConfig struct {
	RoomID       string         `json:"roomId"`
	CourseName   string         `json:"courseName"`
	Weekdays     []time.Weekday `json:"weekdays"`
	TotalPeriods int            `json:"totalPeriods"`
	StartDate    time.Time      `json:"startDate"`
	CreatedAt    time.Time      `json:"createdAt"`
}

func (c CourseConfig) Validate() error {
	if strings.TrimSpace(c.RoomID) == "" {
		return fmt.Errorf("course config roomID cannot be empty, got: %q", c.RoomID)
	}
	if strings.TrimSpace(c.CourseName) == "" {
		return fmt.Errorf("course config courseName cannot be empty, got: %q", c.CourseName)
	}
	if len(c.Weekdays) == 0 {
		return fmt.Errorf("course config weekdays must have at least 1 day, got: 0")
	}
	if c.TotalPeriods <= 0 {
		return fmt.Errorf("course config totalPeriods must be > 0, got: %d", c.TotalPeriods)
	}
	return nil
}

type Student struct {
	ID                 string       `json:"id"`
	RoomID             string       `json:"roomId"`
	FullName           string       `json:"fullName"`
	RegistrationNumber string       `json:"registrationNumber"`
	Level              StudentLevel `json:"level"`
	Active             bool         `json:"active"`
	CreatedAt          time.Time    `json:"createdAt"`
}

func (s Student) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("student id cannot be empty, got: %q", s.ID)
	}
	if strings.TrimSpace(s.RoomID) == "" {
		return fmt.Errorf("student roomId cannot be empty, got: %q", s.RoomID)
	}
	if strings.TrimSpace(s.FullName) == "" {
		return fmt.Errorf("student fullName cannot be empty, got: %q", s.FullName)
	}
	if !s.Level.IsValid() {
		return fmt.Errorf("student level must be GRADUATION or POS_GRADUATION, got: %q", s.Level)
	}
	return nil
}

type AttendanceEntry struct {
	StudentID string `json:"studentId"`
	IsPresent bool   `json:"isPresent"`
}

// AttendanceSession is one class day. A Cancelled session records that the
// class did not happen: it carries no entries and counts as no absence for
// anyone.
type AttendanceSession struct {
	ID          string            `json:"id"`
	RoomID      string            `json:"roomId"`
	ClassDate   string            `json:"classDate"` // YYYY-MM-DD
	PeriodCount int               `json:"periodCount"`
	Cancelled   bool              `json:"cancelled"`
	Entries     []AttendanceEntry `json:"entries"`
	CreatedAt   time.Time         `json:"createdAt"`
}

func (a AttendanceSession) Validate() error {
	if strings.TrimSpace(a.RoomID) == "" {
		return fmt.Errorf("attendance session roomId cannot be empty, got: %q", a.RoomID)
	}
	if strings.TrimSpace(a.ClassDate) == "" {
		return fmt.Errorf("attendance session classDate cannot be empty, got: %q", a.ClassDate)
	}
	if !a.Cancelled && a.PeriodCount <= 0 {
		return fmt.Errorf("attendance session periodCount must be > 0, got: %d", a.PeriodCount)
	}
	return nil
}

// AttendanceStatus classifies a student's absences against the allowed limit.
type AttendanceStatus string

const (
	StatusOK      AttendanceStatus = "OK"
	StatusWarning AttendanceStatus = "WARNING"
	StatusFailed  AttendanceStatus = "FAILED"
)

type StudentSummary struct {
	Student           Student          `json:"student"`
	Presences         int              `json:"presences"`
	Absences          int              `json:"absences"`
	AbsencePercent    float64          `json:"absencePercent"`
	AttendancePercent float64          `json:"attendancePercent"`
	MaxAbsences       int              `json:"maxAbsences"`
	RemainingAbsences int              `json:"remainingAbsences"`
	Status            AttendanceStatus `json:"status"`
	IsApproved        bool             `json:"isApproved"`
}
