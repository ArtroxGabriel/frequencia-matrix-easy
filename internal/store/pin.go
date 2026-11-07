package store

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"frequencia-matrix-integration/internal/domain"
)

const (
	pinIterations  = 120_000
	pinKeyLength   = 32
	pinSaltLength  = 16
	tokenBytes     = 32
	pinMaxAttempts = 5 // wrong guesses before a lockout
	pinBaseLockout = 15 * time.Minute
	pinMaxLockout  = 24 * time.Hour
)

var (
	ErrNoPIN          = errors.New("room has no access pin")
	ErrPINInvalid     = errors.New("invalid access pin")
	ErrPINLocked      = errors.New("access pin is locked after too many wrong attempts")
	ErrSessionInvalid = errors.New("invalid or expired pin session")
)

type pinRecord struct {
	hash        []byte
	salt        []byte
	version     int
	failed      int
	lockedUntil int64
}

func hashPIN(pin string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, pin, salt, pinIterations, pinKeyLength)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed reading random bytes: %w", err)
	}
	return b, nil
}

// SetRoomPIN stores pin as the room's access PIN, replacing any previous one.
// Replacing it ends every session opened with the old PIN.
func (s *Store) SetRoomPIN(ctx context.Context, roomID, pin string, now time.Time) error {
	if !domain.ValidPIN(pin) {
		return ErrPINInvalid
	}
	salt, err := randomBytes(pinSaltLength)
	if err != nil {
		return err
	}
	hash, err := hashPIN(pin, salt)
	if err != nil {
		return fmt.Errorf("failed hashing pin: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed starting pin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO room_pins (room_id, pin_hash, salt, version, failed_attempts, locked_until, updated_at)
		VALUES (?, ?, ?, 1, 0, 0, ?)
		ON CONFLICT(room_id) DO UPDATE SET
			pin_hash = excluded.pin_hash,
			salt = excluded.salt,
			version = room_pins.version + 1,
			failed_attempts = 0,
			locked_until = 0,
			updated_at = excluded.updated_at`,
		roomID, hash, salt, now.Unix()); err != nil {
		return fmt.Errorf("failed saving room pin: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pin_sessions WHERE room_id = ?", roomID); err != nil {
		return fmt.Errorf("failed ending old pin sessions: %w", err)
	}
	return tx.Commit()
}

// HasRoomPIN reports whether the room has an access PIN.
func (s *Store) HasRoomPIN(ctx context.Context, roomID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM room_pins WHERE room_id = ?", roomID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed checking room pin: %w", err)
	}
	return true, nil
}

// LoginWithPIN checks pin for the room and, if it is right, opens a session
// that lasts ttl and returns its token. Only the token's hash is stored.
//
// Every wrong guess is counted. After pinMaxAttempts in a row the PIN is
// locked, even for the right guess, for a time that doubles at each further
// round of attempts. A malformed PIN cannot be right, so it is not counted.
func (s *Store) LoginWithPIN(ctx context.Context, roomID, pin string, ttl time.Duration, now time.Time) (string, error) {
	s.pinMu.Lock()
	defer s.pinMu.Unlock()

	rec, err := s.getPIN(ctx, roomID)
	if err != nil {
		return "", err
	}
	if now.Unix() < rec.lockedUntil {
		return "", ErrPINLocked
	}
	if !domain.ValidPIN(pin) {
		return "", ErrPINInvalid
	}
	guess, err := hashPIN(pin, rec.salt)
	if err != nil {
		return "", fmt.Errorf("failed hashing pin: %w", err)
	}
	if subtle.ConstantTimeCompare(guess, rec.hash) != 1 {
		return "", s.recordPINFailure(ctx, roomID, rec, now)
	}
	return s.openPINSession(ctx, roomID, rec.version, ttl, now)
}

func (s *Store) getPIN(ctx context.Context, roomID string) (pinRecord, error) {
	var rec pinRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT pin_hash, salt, version, failed_attempts, locked_until
		FROM room_pins WHERE room_id = ?`, roomID).
		Scan(&rec.hash, &rec.salt, &rec.version, &rec.failed, &rec.lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return pinRecord{}, ErrNoPIN
	}
	if err != nil {
		return pinRecord{}, fmt.Errorf("failed loading room pin: %w", err)
	}
	return rec, nil
}

// recordPINFailure counts a wrong guess. It returns ErrPINLocked when this
// guess caused a lockout and ErrPINInvalid otherwise.
func (s *Store) recordPINFailure(ctx context.Context, roomID string, rec pinRecord, now time.Time) error {
	failed := rec.failed + 1
	var lockedUntil int64
	if failed%pinMaxAttempts == 0 {
		lockedUntil = now.Add(lockoutFor(failed / pinMaxAttempts)).Unix()
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE room_pins SET failed_attempts = ?, locked_until = ? WHERE room_id = ?",
		failed, lockedUntil, roomID); err != nil {
		return fmt.Errorf("failed recording wrong pin: %w", err)
	}
	if lockedUntil != 0 {
		return ErrPINLocked
	}
	return ErrPINInvalid
}

// lockoutFor returns the lockout of the given round (1 = first): 15 minutes,
// doubling each round, up to a day.
func lockoutFor(round int) time.Duration {
	d := pinBaseLockout
	for i := 1; i < round && d < pinMaxLockout; i++ {
		d *= 2
	}
	return min(d, pinMaxLockout)
}

func (s *Store) openPINSession(ctx context.Context, roomID string, version int, ttl time.Duration, now time.Time) (string, error) {
	raw, err := randomBytes(tokenBytes)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed starting session transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		"UPDATE room_pins SET failed_attempts = 0, locked_until = 0 WHERE room_id = ?", roomID); err != nil {
		return "", fmt.Errorf("failed resetting pin attempts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pin_sessions WHERE expires_at <= ?", now.Unix()); err != nil {
		return "", fmt.Errorf("failed pruning pin sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO pin_sessions (token_hash, room_id, pin_version, expires_at) VALUES (?, ?, ?, ?)",
		hashToken(token), roomID, version, now.Add(ttl).Unix()); err != nil {
		return "", fmt.Errorf("failed saving pin session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed committing pin session: %w", err)
	}
	return token, nil
}

// PINSessionRoom returns the room a session token belongs to. The token is
// valid only until it expires and while the PIN it was opened with is still
// the room's current one. SetRoomPIN already deletes the old sessions; the
// version match also covers a login that finishes while the PIN is replaced.
func (s *Store) PINSessionRoom(ctx context.Context, token string, now time.Time) (string, error) {
	var roomID string
	err := s.db.QueryRowContext(ctx, `
		SELECT s.room_id
		FROM pin_sessions s
		JOIN room_pins p ON p.room_id = s.room_id AND p.version = s.pin_version
		WHERE s.token_hash = ? AND s.expires_at > ?`,
		hashToken(token), now.Unix()).Scan(&roomID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSessionInvalid
	}
	if err != nil {
		return "", fmt.Errorf("failed loading pin session: %w", err)
	}
	return roomID, nil
}
