package domain_test

import (
	"strings"
	"testing"

	"frequencia-matrix-integration/internal/domain"
)

func TestNewStudentIDIsUniqueAndPrefixed(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 5000; i++ {
		id := domain.NewStudentID()
		if !strings.HasPrefix(id, "stu_") || len(id) != 4+24 {
			t.Fatalf("unexpected id shape: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = true
	}
}
