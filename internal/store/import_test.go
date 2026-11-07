package store_test

import (
	"context"
	"testing"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/store"
)

const importRoom = "!import:matrix.dc.ufc.br"

func incoming(level domain.StudentLevel, pairs ...string) []domain.Student {
	out := make([]domain.Student, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, domain.Student{FullName: pairs[i], RegistrationNumber: pairs[i+1], Level: level})
	}
	return out
}

func listAll(t *testing.T, s *store.Store) []domain.Student {
	t.Helper()
	students, err := s.ListStudents(context.Background(), importRoom, false)
	if err != nil {
		t.Fatalf("failed listing students: %v", err)
	}
	return students
}

func TestImportStudentsCreatesNewStudents(t *testing.T) {
	s := newMemoryStore(t)

	res, err := s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Érika Souza", "5011", "Daniel Lima", "5012"))

	if err != nil || res != (store.ImportResult{Created: 2}) {
		t.Fatalf("expected 2 created, got %+v (err: %v)", res, err)
	}
	students := listAll(t, s)
	if len(students) != 2 || !students[0].Active || students[0].RoomID != importRoom || students[0].ID == "" {
		t.Errorf("unexpected students: %+v", students)
	}
}

func TestImportStudentsIsIdempotent(t *testing.T) {
	s := newMemoryStore(t)
	file := incoming(domain.LevelGraduation, "Ana", "1", "Bia", "2", "Caio", "")

	_, _ = s.ImportStudents(context.Background(), importRoom, file)
	res, err := s.ImportStudents(context.Background(), importRoom, file)

	if err != nil || res != (store.ImportResult{Updated: 3}) {
		t.Fatalf("expected 3 updated and none created, got %+v (err: %v)", res, err)
	}
	if n := len(listAll(t, s)); n != 3 {
		t.Errorf("re-importing must not duplicate students, got %d", n)
	}
}

func TestImportStudentsUpdatesByRegistration(t *testing.T) {
	s := newMemoryStore(t)
	_, _ = s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Ana Silva", "1"))
	before := listAll(t, s)[0]

	_, err := s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelPosGraduation, "Ana Souza Silva", "1"))

	after := listAll(t, s)
	if err != nil || len(after) != 1 || after[0].ID != before.ID {
		t.Fatalf("expected the same student to be updated, got %+v (err: %v)", after, err)
	}
	if after[0].FullName != "Ana Souza Silva" || after[0].Level != domain.LevelPosGraduation {
		t.Errorf("name and level must follow the file, got %+v", after[0])
	}
}

func TestImportStudentsAdoptsStudentAddedByHandWithoutRegistration(t *testing.T) {
	s := newMemoryStore(t)
	addStudentNamed(t, s, "stu-hand", "Érika Souza", "")

	res, _ := s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Erika Souza", "5011"))

	students := listAll(t, s)
	if res.Updated != 1 || len(students) != 1 || students[0].ID != "stu-hand" || students[0].RegistrationNumber != "5011" {
		t.Errorf("expected the hand-added student to receive the registration, got %+v (%+v)", students, res)
	}
}

func TestImportStudentsKeepsRegistrationWhenFileHasNone(t *testing.T) {
	s := newMemoryStore(t)
	_, _ = s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Ana", "77"))

	_, _ = s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "ANA", ""))

	students := listAll(t, s)
	if len(students) != 1 || students[0].RegistrationNumber != "77" {
		t.Errorf("a missing registration must not erase the stored one, got %+v", students)
	}
}

func TestImportStudentsReactivatesInactiveStudent(t *testing.T) {
	s := newMemoryStore(t)
	_, _ = s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Ana", "1"))
	id := listAll(t, s)[0].ID
	_ = s.DeactivateStudent(context.Background(), importRoom, id)

	_, _ = s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Ana", "1"))

	students := listAll(t, s)
	if len(students) != 1 || !students[0].Active || students[0].ID != id {
		t.Errorf("expected the same student to be active again, got %+v", students)
	}
}

func TestImportStudentsSkipsAmbiguousNames(t *testing.T) {
	s := newMemoryStore(t)
	addStudentNamed(t, s, "stu-1", "João Silva", "")
	addStudentNamed(t, s, "stu-2", "Joao Silva", "")

	res, err := s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "João Silva", ""))

	if err != nil || res != (store.ImportResult{Skipped: 1}) || len(listAll(t, s)) != 2 {
		t.Errorf("an ambiguous line must be skipped, got %+v (err: %v)", res, err)
	}
}

func TestImportStudentsKeepsRoomsSeparate(t *testing.T) {
	s := newMemoryStore(t)
	_, _ = s.ImportStudents(context.Background(), importRoom, incoming(domain.LevelGraduation, "Ana", "1"))

	res, _ := s.ImportStudents(context.Background(), "!other:matrix.dc.ufc.br", incoming(domain.LevelGraduation, "Ana", "1"))

	if res.Created != 1 || len(listAll(t, s)) != 1 {
		t.Errorf("another room must get its own student, got %+v", res)
	}
}

func TestImportStudentsRollsBackOnInvalidStudent(t *testing.T) {
	s := newMemoryStore(t)
	file := incoming(domain.LevelGraduation, "Ana", "1")
	file = append(file, domain.Student{FullName: "Bia", Level: "INVALID"})

	if _, err := s.ImportStudents(context.Background(), importRoom, file); err == nil {
		t.Fatal("expected an error for an invalid level")
	}
	if n := len(listAll(t, s)); n != 0 {
		t.Errorf("a failed import must not leave partial data, got %d students", n)
	}
}

func addStudentNamed(t *testing.T, s *store.Store, id, name, registration string) {
	t.Helper()
	stu := domain.Student{ID: id, RoomID: importRoom, FullName: name, RegistrationNumber: registration, Level: domain.LevelGraduation, Active: true}
	if err := s.AddStudent(context.Background(), stu); err != nil {
		t.Fatalf("failed adding student %s: %v", id, err)
	}
}
