package domain

import "testing"

func TestNewPINHasSixDigits(t *testing.T) {
	for i := 0; i < 200; i++ {
		pin, err := NewPIN()
		if err != nil {
			t.Fatalf("NewPIN() error: %v", err)
		}
		if !ValidPIN(pin) {
			t.Fatalf("NewPIN() = %q, want %d digits", pin, PINLength)
		}
	}
}

func TestNewPINIsNotConstant(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		pin, _ := NewPIN()
		seen[pin] = true
	}
	if len(seen) < 40 {
		t.Errorf("only %d distinct pins in 50 draws, the generator looks broken", len(seen))
	}
}

func TestValidPIN(t *testing.T) {
	tests := []struct {
		pin  string
		want bool
	}{
		{"123456", true},
		{"000000", true},
		{"12345", false},
		{"1234567", false},
		{"12345a", false},
		{"12 456", false},
		{"１２３４５６", false}, // full-width digits are not ASCII
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidPIN(tt.pin); got != tt.want {
			t.Errorf("ValidPIN(%q) = %v, want %v", tt.pin, got, tt.want)
		}
	}
}
