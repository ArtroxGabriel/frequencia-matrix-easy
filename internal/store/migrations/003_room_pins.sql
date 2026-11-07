CREATE TABLE IF NOT EXISTS room_pins (
    room_id TEXT PRIMARY KEY,
    pin_hash BLOB NOT NULL,
    salt BLOB NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS pin_sessions (
    token_hash TEXT PRIMARY KEY,
    room_id TEXT NOT NULL,
    pin_version INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pin_sessions_room ON pin_sessions(room_id);
