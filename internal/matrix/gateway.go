package matrix

import (
	"context"
	"errors"
)

var (
	ErrUnauthorized = errors.New("unauthorized: missing or invalid credentials")
	ErrForbidden    = errors.New("forbidden: teacher role required")
)

type UserIdentity struct {
	UserID string `json:"userId"`
	RoomID string `json:"roomId"`
}

type MatrixGateway interface {
	VerifyUser(ctx context.Context, roomID string, openIDToken string) (UserIdentity, error)
	IsTeacher(ctx context.Context, roomID string, userID string) (bool, error)
	SendNotice(ctx context.Context, roomID string, message string) error
}
