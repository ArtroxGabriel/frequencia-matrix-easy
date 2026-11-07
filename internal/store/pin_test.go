package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/store"
)

const (
	pinRoom = "!course:matrix.dc.ufc.br"
	goodPIN = "123456"
	badPIN  = "654321"
	ttl     = 24 * time.Hour
)

var t0 = time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)

func newPINStore(t *testing.T) *store.Store {
	t.Helper()
	s := newMemoryStore(t)
	if err := s.SetRoomPIN(context.Background(), pinRoom, goodPIN, t0); err != nil {
		t.Fatalf("SetRoomPIN: %v", err)
	}
	return s
}

func login(s *store.Store, pin string, now time.Time) (string, error) {
	return s.LoginWithPIN(context.Background(), pinRoom, pin, ttl, now)
}

func TestHasRoomPIN(t *testing.T) {
	ctx := context.Background()
	s := newMemoryStore(t)
	if has, err := s.HasRoomPIN(ctx, pinRoom); err != nil || has {
		t.Fatalf("HasRoomPIN before = %v, %v; want false", has, err)
	}
	if err := s.SetRoomPIN(ctx, pinRoom, goodPIN, t0); err != nil {
		t.Fatal(err)
	}
	if has, err := s.HasRoomPIN(ctx, pinRoom); err != nil || !has {
		t.Fatalf("HasRoomPIN after = %v, %v; want true", has, err)
	}
	if has, _ := s.HasRoomPIN(ctx, "!other:server"); has {
		t.Error("another room must not have a PIN")
	}
}

func TestSetRoomPINRejectsMalformedPIN(t *testing.T) {
	s := newMemoryStore(t)
	for _, pin := range []string{"", "12345", "1234567", "abcdef"} {
		if err := s.SetRoomPIN(context.Background(), pinRoom, pin, t0); !errors.Is(err, store.ErrPINInvalid) {
			t.Errorf("SetRoomPIN(%q) = %v, want ErrPINInvalid", pin, err)
		}
	}
}

func TestLoginOpensASessionForTheRoom(t *testing.T) {
	s := newPINStore(t)
	token, err := login(s, goodPIN, t0)
	if err != nil || token == "" {
		t.Fatalf("login = %q, %v", token, err)
	}
	room, err := s.PINSessionRoom(context.Background(), token, t0.Add(time.Hour))
	if err != nil || room != pinRoom {
		t.Errorf("PINSessionRoom = %q, %v; want %q", room, err, pinRoom)
	}
}

func TestSessionsHaveDistinctTokens(t *testing.T) {
	s := newPINStore(t)
	a, _ := login(s, goodPIN, t0)
	b, _ := login(s, goodPIN, t0)
	if a == b {
		t.Error("two logins returned the same token")
	}
}

func TestSessionExpires(t *testing.T) {
	s := newPINStore(t)
	token, _ := login(s, goodPIN, t0)
	ctx := context.Background()
	if _, err := s.PINSessionRoom(ctx, token, t0.Add(ttl-time.Second)); err != nil {
		t.Errorf("session just before expiry: %v", err)
	}
	if _, err := s.PINSessionRoom(ctx, token, t0.Add(ttl)); !errors.Is(err, store.ErrSessionInvalid) {
		t.Errorf("session at expiry = %v, want ErrSessionInvalid", err)
	}
}

func TestUnknownSessionTokenIsRejected(t *testing.T) {
	s := newPINStore(t)
	if _, err := s.PINSessionRoom(context.Background(), "not-a-token", t0); !errors.Is(err, store.ErrSessionInvalid) {
		t.Errorf("got %v, want ErrSessionInvalid", err)
	}
}

func TestLoginWithoutPIN(t *testing.T) {
	s := newMemoryStore(t)
	if _, err := login(s, goodPIN, t0); !errors.Is(err, store.ErrNoPIN) {
		t.Errorf("got %v, want ErrNoPIN", err)
	}
}

func TestWrongPINIsRejected(t *testing.T) {
	s := newPINStore(t)
	if _, err := login(s, badPIN, t0); !errors.Is(err, store.ErrPINInvalid) {
		t.Errorf("got %v, want ErrPINInvalid", err)
	}
}

func TestFifthWrongPINLocksTheRoomEvenForTheRightPIN(t *testing.T) {
	s := newPINStore(t)
	for i := 1; i <= 4; i++ {
		if _, err := login(s, badPIN, t0); !errors.Is(err, store.ErrPINInvalid) {
			t.Fatalf("wrong guess %d = %v, want ErrPINInvalid", i, err)
		}
	}
	if _, err := login(s, badPIN, t0); !errors.Is(err, store.ErrPINLocked) {
		t.Fatalf("fifth wrong guess = %v, want ErrPINLocked", err)
	}
	if _, err := login(s, goodPIN, t0.Add(14*time.Minute)); !errors.Is(err, store.ErrPINLocked) {
		t.Errorf("right PIN while locked = %v, want ErrPINLocked", err)
	}
	if _, err := login(s, goodPIN, t0.Add(15*time.Minute)); err != nil {
		t.Errorf("right PIN after the lockout = %v, want success", err)
	}
}

func TestLockoutDoublesAtEachRound(t *testing.T) {
	s := newPINStore(t)
	now := t0
	for round, want := range []time.Duration{15 * time.Minute, 30 * time.Minute, 60 * time.Minute} {
		for i := 0; i < 5; i++ {
			_, _ = login(s, badPIN, now)
		}
		if _, err := login(s, goodPIN, now.Add(want-time.Second)); !errors.Is(err, store.ErrPINLocked) {
			t.Fatalf("round %d: still locked just before %v, got %v", round+1, want, err)
		}
		now = now.Add(want)
		if round < 2 { // the next round starts right after this lockout ends
			continue
		}
		if _, err := login(s, goodPIN, now); err != nil {
			t.Errorf("round %d: right PIN after %v = %v", round+1, want, err)
		}
	}
}

func TestSuccessfulLoginResetsTheAttemptCounter(t *testing.T) {
	s := newPINStore(t)
	for i := 0; i < 4; i++ {
		_, _ = login(s, badPIN, t0)
	}
	if _, err := login(s, goodPIN, t0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ { // would have locked at the first one if the counter was not reset
		if _, err := login(s, badPIN, t0); !errors.Is(err, store.ErrPINInvalid) {
			t.Fatalf("guess %d after reset = %v, want ErrPINInvalid", i, err)
		}
	}
}

func TestMalformedPINsAreNotCounted(t *testing.T) {
	s := newPINStore(t)
	for i := 0; i < 20; i++ {
		if _, err := login(s, "12ab", t0); !errors.Is(err, store.ErrPINInvalid) {
			t.Fatalf("malformed guess = %v, want ErrPINInvalid", err)
		}
	}
	if _, err := login(s, goodPIN, t0); err != nil {
		t.Errorf("right PIN after malformed guesses = %v, want success", err)
	}
}

func TestNewPINEndsOldSessionsAndOldPIN(t *testing.T) {
	s := newPINStore(t)
	ctx := context.Background()
	old, _ := login(s, goodPIN, t0)

	if err := s.SetRoomPIN(ctx, pinRoom, badPIN, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PINSessionRoom(ctx, old, t0.Add(time.Minute)); !errors.Is(err, store.ErrSessionInvalid) {
		t.Errorf("old session after a new PIN = %v, want ErrSessionInvalid", err)
	}
	if _, err := login(s, goodPIN, t0.Add(time.Minute)); !errors.Is(err, store.ErrPINInvalid) {
		t.Errorf("old PIN after a new PIN = %v, want ErrPINInvalid", err)
	}
	if _, err := login(s, badPIN, t0.Add(time.Minute)); err != nil {
		t.Errorf("new PIN = %v, want success", err)
	}
}

func TestNewPINClearsALockout(t *testing.T) {
	s := newPINStore(t)
	for i := 0; i < 5; i++ {
		_, _ = login(s, "000000", t0)
	}
	if err := s.SetRoomPIN(context.Background(), pinRoom, badPIN, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := login(s, badPIN, t0); err != nil {
		t.Errorf("login with a new PIN right after = %v, want success", err)
	}
}

func TestPINsAreIndependentPerRoom(t *testing.T) {
	ctx := context.Background()
	s := newPINStore(t)
	other := "!other:matrix.dc.ufc.br"
	if err := s.SetRoomPIN(ctx, other, badPIN, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoginWithPIN(ctx, other, goodPIN, ttl, t0); !errors.Is(err, store.ErrPINInvalid) {
		t.Errorf("another room's PIN = %v, want ErrPINInvalid", err)
	}
	token, _ := login(s, goodPIN, t0)
	if room, _ := s.PINSessionRoom(ctx, token, t0); room != pinRoom {
		t.Errorf("session room = %q, want %q", room, pinRoom)
	}
}

func TestExpiredSessionsArePrunedAtLogin(t *testing.T) {
	s := newPINStore(t)
	stale, _ := login(s, goodPIN, t0)
	if _, err := login(s, goodPIN, t0.Add(2*ttl)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PINSessionRoom(context.Background(), stale, t0); !errors.Is(err, store.ErrSessionInvalid) {
		t.Errorf("pruned session still valid: %v", err)
	}
}
