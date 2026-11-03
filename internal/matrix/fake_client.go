package matrix

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type SentNotice struct {
	RoomID  string
	Message string
}

type FakeMatrixClient struct {
	mu            sync.RWMutex
	TeacherLevel  int
	userLevels    map[string]int // key: "roomID:userID"
	validTokens   map[string]UserIdentity
	sentNotices   []SentNotice
	allowDevUsers bool
}

func NewFakeMatrixClient(allowDevUsers bool) *FakeMatrixClient {
	return &FakeMatrixClient{
		TeacherLevel:  50,
		userLevels:    make(map[string]int),
		validTokens:   make(map[string]UserIdentity),
		allowDevUsers: allowDevUsers,
	}
}

func (f *FakeMatrixClient) SetUserPowerLevel(roomID, userID string, level int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fmt.Sprintf("%s:%s", roomID, userID)
	f.userLevels[key] = level
}

func (f *FakeMatrixClient) RegisterOpenIDToken(token string, identity UserIdentity) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validTokens[token] = identity
}

func (f *FakeMatrixClient) VerifyUser(ctx context.Context, roomID string, openIDToken string) (UserIdentity, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if identity, ok := f.validTokens[openIDToken]; ok {
		if roomID != "" && identity.RoomID != "" && identity.RoomID != roomID {
			return UserIdentity{}, fmt.Errorf("%w: token room mismatch", ErrUnauthorized)
		}
		return identity, nil
	}

	if f.allowDevUsers && (openIDToken == "dev" || strings.HasPrefix(openIDToken, "@")) {
		userID := openIDToken
		if openIDToken == "dev" {
			userID = "@professor:matrix.dc.ufc.br"
		}
		return UserIdentity{
			UserID: userID,
			RoomID: roomID,
		}, nil
	}

	return UserIdentity{}, fmt.Errorf("%w: invalid token %q", ErrUnauthorized, openIDToken)
}

func (f *FakeMatrixClient) IsTeacher(ctx context.Context, roomID string, userID string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", roomID, userID)
	level, ok := f.userLevels[key]
	if !ok {
		// By default in dev mode: @professor or @teacher is teacher (level 100)
		if f.allowDevUsers && (strings.Contains(userID, "prof") || strings.Contains(userID, "teacher")) {
			return true, nil
		}
		return false, nil
	}
	return level >= f.TeacherLevel, nil
}

func (f *FakeMatrixClient) SendNotice(ctx context.Context, roomID string, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentNotices = append(f.sentNotices, SentNotice{RoomID: roomID, Message: message})
	return nil
}

func (f *FakeMatrixClient) GetSentNotices() []SentNotice {
	f.mu.RLock()
	defer f.mu.RUnlock()
	copied := make([]SentNotice, len(f.sentNotices))
	copy(copied, f.sentNotices)
	return copied
}
