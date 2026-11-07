package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"frequencia-matrix-integration/internal/domain"
)

// ImportResult counts what a roster import did.
type ImportResult struct {
	Created int
	Updated int
	// Skipped counts lines that could be two or more existing students with
	// the same name; they are left untouched rather than guessed.
	Skipped int
}

// ImportStudents upserts a roster in a single transaction. A student is the
// same person when the registration matches, or, when the file has none, when
// exactly one existing student has the same name ignoring accents and case.
// Existing students are updated and reactivated instead of duplicated, so
// importing the same file twice changes nothing.
func (s *Store) ImportStudents(ctx context.Context, roomID string, incoming []domain.Student) (ImportResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("failed starting tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := queryStudents(ctx, tx, roomID, false)
	if err != nil {
		return ImportResult{}, err
	}
	index := newRosterIndex(existing)

	var result ImportResult
	for _, stu := range incoming {
		stu.RoomID = roomID
		stu.Active = true
		outcome, err := upsertImported(ctx, tx, index, stu)
		if err != nil {
			return ImportResult{}, err
		}
		result.count(outcome)
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("failed committing import: %w", err)
	}
	return result, nil
}

type importOutcome int

const (
	outcomeCreated importOutcome = iota
	outcomeUpdated
	outcomeSkipped
)

func (r *ImportResult) count(o importOutcome) {
	switch o {
	case outcomeCreated:
		r.Created++
	case outcomeUpdated:
		r.Updated++
	default:
		r.Skipped++
	}
}

func upsertImported(ctx context.Context, tx *sql.Tx, index *rosterIndex, stu domain.Student) (importOutcome, error) {
	match, kind := index.find(stu)
	switch kind {
	case matchAmbiguous:
		return outcomeSkipped, nil
	case matchNone:
		return outcomeCreated, insertImported(ctx, tx, index, stu)
	default:
		return outcomeUpdated, updateImported(ctx, tx, index, match, stu)
	}
}

func insertImported(ctx context.Context, tx *sql.Tx, index *rosterIndex, stu domain.Student) error {
	stu.ID = domain.NewStudentID()
	stu.CreatedAt = time.Now().UTC()
	if err := stu.Validate(); err != nil {
		return fmt.Errorf("invalid student %q: %w", stu.FullName, err)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO students (id, room_id, full_name, registration_number, level, active, created_at) VALUES (?, ?, ?, ?, ?, 1, ?)`,
		stu.ID, stu.RoomID, stu.FullName, stu.RegistrationNumber, string(stu.Level), stu.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed inserting imported student %q: %w", stu.FullName, err)
	}
	index.add(stu)
	return nil
}

func updateImported(ctx context.Context, tx *sql.Tx, index *rosterIndex, match, stu domain.Student) error {
	registration := match.RegistrationNumber
	if stu.RegistrationNumber != "" {
		registration = stu.RegistrationNumber
	}
	_, err := tx.ExecContext(ctx, `UPDATE students SET full_name = ?, registration_number = ?, level = ?, active = 1 WHERE id = ? AND room_id = ?`,
		stu.FullName, registration, string(stu.Level), match.ID, stu.RoomID)
	if err != nil {
		return fmt.Errorf("failed updating imported student %q: %w", stu.FullName, err)
	}
	stu.ID, stu.RegistrationNumber = match.ID, registration
	index.add(stu)
	return nil
}

type matchKind int

const (
	matchNone matchKind = iota
	matchOne
	matchAmbiguous
)

// rosterIndex finds existing students by registration or by folded name.
type rosterIndex struct {
	byRegistration map[string]domain.Student
	byName         map[string][]domain.Student
}

func newRosterIndex(students []domain.Student) *rosterIndex {
	idx := &rosterIndex{
		byRegistration: make(map[string]domain.Student),
		byName:         make(map[string][]domain.Student),
	}
	for _, s := range students {
		idx.add(s)
	}
	return idx
}

func (i *rosterIndex) add(s domain.Student) {
	if s.RegistrationNumber != "" {
		i.byRegistration[s.RegistrationNumber] = s
	}
	key := domain.NormalizeForSort(s.FullName)
	list := i.byName[key]
	for n := range list {
		if list[n].ID == s.ID {
			list[n] = s
			return
		}
	}
	i.byName[key] = append(list, s)
}

func (i *rosterIndex) find(s domain.Student) (domain.Student, matchKind) {
	if s.RegistrationNumber != "" {
		if match, ok := i.byRegistration[s.RegistrationNumber]; ok {
			return match, matchOne
		}
		return i.byNameWithoutRegistration(s)
	}
	return pickUnique(i.byName[domain.NormalizeForSort(s.FullName)])
}

// byNameWithoutRegistration lets an import adopt a student that was added by
// hand without a registration number.
func (i *rosterIndex) byNameWithoutRegistration(s domain.Student) (domain.Student, matchKind) {
	var candidates []domain.Student
	for _, c := range i.byName[domain.NormalizeForSort(s.FullName)] {
		if c.RegistrationNumber == "" {
			candidates = append(candidates, c)
		}
	}
	return pickUnique(candidates)
}

func pickUnique(candidates []domain.Student) (domain.Student, matchKind) {
	switch len(candidates) {
	case 0:
		return domain.Student{}, matchNone
	case 1:
		return candidates[0], matchOne
	default:
		return domain.Student{}, matchAmbiguous
	}
}
