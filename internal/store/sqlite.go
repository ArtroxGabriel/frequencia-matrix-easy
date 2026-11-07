package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"frequencia-matrix-integration/internal/domain"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("record not found")

type Store struct {
	db *sql.DB

	// pinMu serialises PIN logins so that concurrent wrong guesses cannot
	// slip past the attempt counter.
	pinMu sync.Mutex
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed opening sqlite database at %s: %w", dbPath, err)
	}

	if strings.Contains(dbPath, ":memory:") {
		db.SetMaxOpenConns(1)
	}

	if err := applyPragmas(db, dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := RunMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed running migrations: %w", err)
	}

	return &Store{db: db}, nil
}

func applyPragmas(db *sql.DB, dbPath string) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	if !strings.Contains(dbPath, ":memory:") {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL;", "PRAGMA synchronous = NORMAL;")
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("failed applying pragma %q: %w", p, err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveCourseConfig(ctx context.Context, cfg domain.CourseConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid course config: %w", err)
	}

	weekdaysJSON, err := json.Marshal(cfg.Weekdays)
	if err != nil {
		return fmt.Errorf("failed marshalling weekdays: %w", err)
	}

	query := `
		INSERT INTO courses (room_id, course_name, weekdays_json, total_periods, start_date, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(room_id) DO UPDATE SET
			course_name = excluded.course_name,
			weekdays_json = excluded.weekdays_json,
			total_periods = excluded.total_periods,
			start_date = excluded.start_date;
	`
	createdAt := cfg.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err = s.db.ExecContext(ctx, query,
		cfg.RoomID, cfg.CourseName, string(weekdaysJSON), cfg.TotalPeriods,
		cfg.StartDate.Format("2006-01-02"), createdAt,
	)
	if err != nil {
		return fmt.Errorf("failed saving course config for room %s: %w", cfg.RoomID, err)
	}
	return nil
}

func (s *Store) GetCourseConfig(ctx context.Context, roomID string) (domain.CourseConfig, error) {
	query := `SELECT room_id, course_name, weekdays_json, total_periods, start_date, created_at FROM courses WHERE room_id = ?`
	var cfg domain.CourseConfig
	var weekdaysJSON, startDateStr string

	row := s.db.QueryRowContext(ctx, query, roomID)
	err := row.Scan(&cfg.RoomID, &cfg.CourseName, &weekdaysJSON, &cfg.TotalPeriods, &startDateStr, &cfg.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CourseConfig{}, ErrNotFound
	}
	if err != nil {
		return domain.CourseConfig{}, fmt.Errorf("failed querying course config for room %s: %w", roomID, err)
	}

	if err := json.Unmarshal([]byte(weekdaysJSON), &cfg.Weekdays); err != nil {
		return domain.CourseConfig{}, fmt.Errorf("failed unmarshalling weekdays for room %s: %w", roomID, err)
	}
	cfg.StartDate, _ = time.Parse("2006-01-02", startDateStr)
	return cfg, nil
}

func (s *Store) AddStudent(ctx context.Context, stu domain.Student) error {
	if err := stu.Validate(); err != nil {
		return fmt.Errorf("invalid student: %w", err)
	}

	createdAt := stu.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	query := `
		INSERT INTO students (id, room_id, full_name, registration_number, level, active, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			full_name = excluded.full_name,
			registration_number = excluded.registration_number,
			level = excluded.level,
			active = excluded.active;
	`
	_, err := s.db.ExecContext(ctx, query, stu.ID, stu.RoomID, stu.FullName, stu.RegistrationNumber, string(stu.Level), stu.Active, createdAt)
	if err != nil {
		return fmt.Errorf("failed inserting student %s in room %s: %w", stu.ID, stu.RoomID, err)
	}
	return nil
}

func (s *Store) ListStudents(ctx context.Context, roomID string, activeOnly bool) ([]domain.Student, error) {
	return queryStudents(ctx, s.db, roomID, activeOnly)
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryStudents(ctx context.Context, q rowQuerier, roomID string, activeOnly bool) ([]domain.Student, error) {
	query := `SELECT id, room_id, full_name, registration_number, level, active, created_at FROM students WHERE room_id = ?`
	if activeOnly {
		query += ` AND active = 1`
	}
	query += ` ORDER BY full_name ASC`

	rows, err := q.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed listing students for room %s: %w", roomID, err)
	}
	defer func() { _ = rows.Close() }()

	var students []domain.Student
	for rows.Next() {
		var stu domain.Student
		var levelStr string
		if err := rows.Scan(&stu.ID, &stu.RoomID, &stu.FullName, &stu.RegistrationNumber, &levelStr, &stu.Active, &stu.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed scanning student row: %w", err)
		}
		stu.Level = domain.StudentLevel(levelStr)
		students = append(students, stu)
	}
	return students, rows.Err()
}

func (s *Store) DeactivateStudent(ctx context.Context, roomID string, studentID string) error {
	query := `UPDATE students SET active = 0 WHERE room_id = ? AND id = ?`
	res, err := s.db.ExecContext(ctx, query, roomID, studentID)
	if err != nil {
		return fmt.Errorf("failed deactivating student %s in room %s: %w", studentID, roomID, err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecordAttendanceSession(ctx context.Context, session domain.AttendanceSession) error {
	if err := session.Validate(); err != nil {
		return fmt.Errorf("invalid attendance session: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed starting tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	sessionID, err := upsertSessionHeader(ctx, tx, session)
	if err != nil {
		return err
	}

	if err := replaceSessionEntries(ctx, tx, sessionID, session.Entries); err != nil {
		return err
	}

	return tx.Commit()
}

func upsertSessionHeader(ctx context.Context, tx *sql.Tx, session domain.AttendanceSession) (string, error) {
	var existingID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM attendance_sessions WHERE room_id = ? AND class_date = ?`, session.RoomID, session.ClassDate).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		sessionID := session.ID
		if sessionID == "" {
			sessionID = fmt.Sprintf("%s_%s", session.RoomID, session.ClassDate)
		}
		createdAt := session.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		query := `INSERT INTO attendance_sessions (id, room_id, class_date, period_count, cancelled, created_at) VALUES (?, ?, ?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, query, sessionID, session.RoomID, session.ClassDate, session.PeriodCount, session.Cancelled, createdAt); err != nil {
			return "", fmt.Errorf("failed inserting attendance session header: %w", err)
		}
		return sessionID, nil
	} else if err != nil {
		return "", fmt.Errorf("failed checking existing session: %w", err)
	}

	updateQuery := `UPDATE attendance_sessions SET period_count = ?, cancelled = ? WHERE id = ?`
	if _, err := tx.ExecContext(ctx, updateQuery, session.PeriodCount, session.Cancelled, existingID); err != nil {
		return "", fmt.Errorf("failed updating session period_count: %w", err)
	}
	return existingID, nil
}

func replaceSessionEntries(ctx context.Context, tx *sql.Tx, sessionID string, entries []domain.AttendanceEntry) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM attendance_entries WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("failed deleting old session entries: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO attendance_entries (session_id, student_id, is_present) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed preparing entry insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, sessionID, e.StudentID, e.IsPresent); err != nil {
			return fmt.Errorf("failed inserting entry for student %s: %w", e.StudentID, err)
		}
	}
	return nil
}

func (s *Store) ListAttendanceSessions(ctx context.Context, roomID string) ([]domain.AttendanceSession, error) {
	headerQuery := `SELECT id, room_id, class_date, period_count, cancelled, created_at FROM attendance_sessions WHERE room_id = ? ORDER BY class_date ASC`
	rows, err := s.db.QueryContext(ctx, headerQuery, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed querying sessions for room %s: %w", roomID, err)
	}

	var sessions []domain.AttendanceSession
	for rows.Next() {
		var sess domain.AttendanceSession
		if err := rows.Scan(&sess.ID, &sess.RoomID, &sess.ClassDate, &sess.PeriodCount, &sess.Cancelled, &sess.CreatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed scanning session row: %w", err)
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range sessions {
		entries, err := s.getSessionEntries(ctx, sessions[i].ID)
		if err != nil {
			return nil, err
		}
		sessions[i].Entries = entries
	}
	return sessions, nil
}

func (s *Store) GetAttendanceSession(ctx context.Context, roomID string, classDate string) (domain.AttendanceSession, error) {
	query := `SELECT id, room_id, class_date, period_count, cancelled, created_at FROM attendance_sessions WHERE room_id = ? AND class_date = ?`
	var sess domain.AttendanceSession
	row := s.db.QueryRowContext(ctx, query, roomID, classDate)
	err := row.Scan(&sess.ID, &sess.RoomID, &sess.ClassDate, &sess.PeriodCount, &sess.Cancelled, &sess.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttendanceSession{}, ErrNotFound
	}
	if err != nil {
		return domain.AttendanceSession{}, fmt.Errorf("failed querying session for room %s date %s: %w", roomID, classDate, err)
	}

	entries, err := s.getSessionEntries(ctx, sess.ID)
	if err != nil {
		return domain.AttendanceSession{}, err
	}
	sess.Entries = entries
	return sess, nil
}

func (s *Store) getSessionEntries(ctx context.Context, sessionID string) ([]domain.AttendanceEntry, error) {
	query := `SELECT student_id, is_present FROM attendance_entries WHERE session_id = ?`
	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed querying entries for session %s: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()

	var entries []domain.AttendanceEntry
	for rows.Next() {
		var e domain.AttendanceEntry
		if err := rows.Scan(&e.StudentID, &e.IsPresent); err != nil {
			return nil, fmt.Errorf("failed scanning entry row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteAttendanceSession removes the recorded session of a date, returning
// the day to its "not recorded yet" state.
func (s *Store) DeleteAttendanceSession(ctx context.Context, roomID string, classDate string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed starting tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entries := `DELETE FROM attendance_entries WHERE session_id IN (SELECT id FROM attendance_sessions WHERE room_id = ? AND class_date = ?)`
	if _, err := tx.ExecContext(ctx, entries, roomID, classDate); err != nil {
		return fmt.Errorf("failed deleting entries for room %s date %s: %w", roomID, classDate, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM attendance_sessions WHERE room_id = ? AND class_date = ?`, roomID, classDate); err != nil {
		return fmt.Errorf("failed deleting session for room %s date %s: %w", roomID, classDate, err)
	}
	return tx.Commit()
}
