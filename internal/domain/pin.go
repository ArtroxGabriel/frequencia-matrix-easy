package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// PINLength is the number of digits of a room access PIN.
const PINLength = 6

// NewPIN returns a random PIN of PINLength digits, leading zeros included.
func NewPIN() (string, error) {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(PINLength), nil)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("failed generating pin: %w", err)
	}
	return fmt.Sprintf("%0*d", PINLength, n), nil
}

// ValidPIN reports whether pin is exactly PINLength ASCII digits.
func ValidPIN(pin string) bool {
	if len(pin) != PINLength {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
