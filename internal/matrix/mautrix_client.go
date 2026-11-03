package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type MautrixGateway struct {
	client        *mautrix.Client
	homeserverURL string
	httpClient    *http.Client
}

func NewMautrixGateway(homeserverURL, userID, accessToken string) (*MautrixGateway, error) {
	client, err := mautrix.NewClient(homeserverURL, id.UserID(userID), accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed creating mautrix client for user %s: %w", userID, err)
	}

	return &MautrixGateway{
		client:        client,
		homeserverURL: homeserverURL,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

type openIDUserInfoResponse struct {
	Sub string `json:"sub"`
}

func (m *MautrixGateway) VerifyUser(ctx context.Context, roomID string, openIDToken string) (UserIdentity, error) {
	if openIDToken == "" {
		return UserIdentity{}, fmt.Errorf("%w: empty openid token", ErrUnauthorized)
	}

	endpoint := fmt.Sprintf("%s/_matrix/federation/v1/openid/userinfo?access_token=%s",
		m.homeserverURL, url.QueryEscape(openIDToken))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return UserIdentity{}, fmt.Errorf("failed building openid request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return UserIdentity{}, fmt.Errorf("failed verifying openid token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return UserIdentity{}, fmt.Errorf("%w: token verification failed with status %d", ErrUnauthorized, resp.StatusCode)
	}

	var info openIDUserInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return UserIdentity{}, fmt.Errorf("failed decoding openid response: %w", err)
	}

	return UserIdentity{
		UserID: info.Sub,
		RoomID: roomID,
	}, nil
}

func (m *MautrixGateway) IsTeacher(ctx context.Context, roomID string, userID string) (bool, error) {
	var powerLevels event.PowerLevelsEventContent
	err := m.client.StateEvent(ctx, id.RoomID(roomID), event.StatePowerLevels, "", &powerLevels)
	if err != nil {
		return false, fmt.Errorf("failed fetching room %s power levels: %w", roomID, err)
	}

	level := powerLevels.GetUserLevel(id.UserID(userID))
	return level >= 50, nil
}

func (m *MautrixGateway) SendNotice(ctx context.Context, roomID string, message string) error {
	_, err := m.client.SendNotice(ctx, id.RoomID(roomID), message)
	if err != nil {
		return fmt.Errorf("failed sending notice to room %s: %w", roomID, err)
	}
	return nil
}
