package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewStudentID returns a random, collision-safe student identifier.
func NewStudentID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("failed reading random bytes for student id: %v", err))
	}
	return "stu_" + hex.EncodeToString(buf)
}
