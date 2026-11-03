CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS courses (
    room_id TEXT PRIMARY KEY,
    course_name TEXT NOT NULL,
    weekdays_json TEXT NOT NULL,
    total_periods INTEGER NOT NULL,
    start_date TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS students (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL,
    full_name TEXT NOT NULL,
    registration_number TEXT NOT NULL DEFAULT '',
    level TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_students_room ON students(room_id, active);

CREATE TABLE IF NOT EXISTS attendance_sessions (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL,
    class_date TEXT NOT NULL,
    period_count INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE(room_id, class_date)
);
CREATE INDEX IF NOT EXISTS idx_sessions_room ON attendance_sessions(room_id, class_date);

CREATE TABLE IF NOT EXISTS attendance_entries (
    session_id TEXT NOT NULL,
    student_id TEXT NOT NULL,
    is_present BOOLEAN NOT NULL,
    PRIMARY KEY(session_id, student_id),
    FOREIGN KEY(session_id) REFERENCES attendance_sessions(id) ON DELETE CASCADE,
    FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
);
