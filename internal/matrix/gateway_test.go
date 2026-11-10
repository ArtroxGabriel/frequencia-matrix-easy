package matrix_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"frequencia-matrix-integration/internal/matrix"
)

func TestFakeMatrixClientAuthAndRoles(t *testing.T) {
	ctx := context.Background()
	client := matrix.NewFakeMatrixClient(false)

	roomID := "!course:matrix.dc.ufc.br"
	teacherID := "@professor:matrix.dc.ufc.br"
	studentID := "@aluno:matrix.dc.ufc.br"

	client.RegisterOpenIDToken("valid-teacher-token", matrix.UserIdentity{
		UserID: teacherID,
		RoomID: roomID,
	})
	client.SetUserPowerLevel(roomID, teacherID, 100)
	client.SetUserPowerLevel(roomID, studentID, 0)

	// Valid token verification
	ident, err := client.VerifyUser(ctx, roomID, "valid-teacher-token")
	if err != nil {
		t.Fatalf("expected valid token verification, got: %v", err)
	}
	if ident.UserID != teacherID || ident.RoomID != roomID {
		t.Errorf("unexpected identity: %+v", ident)
	}

	// Invalid token verification
	_, err = client.VerifyUser(ctx, roomID, "invalid-token")
	if err == nil {
		t.Errorf("expected error for invalid token, got nil")
	}

	// Teacher power level check (100 >= 50)
	isTeacher, err := client.IsTeacher(ctx, roomID, teacherID)
	if err != nil || !isTeacher {
		t.Errorf("expected teacher to have teacher access, got %v (err: %v)", isTeacher, err)
	}

	// Student power level check (0 < 50)
	isStudentTeacher, err := client.IsTeacher(ctx, roomID, studentID)
	if err != nil || isStudentTeacher {
		t.Errorf("expected student to NOT have teacher access, got %v (err: %v)", isStudentTeacher, err)
	}

	// Send notice check
	if err := client.SendNotice(ctx, roomID, "Chamada salva"); err != nil {
		t.Fatalf("failed sending notice: %v", err)
	}
	notices := client.GetSentNotices()
	if len(notices) != 1 || notices[0].Message != "Chamada salva" {
		t.Errorf("unexpected sent notices: %+v", notices)
	}
}

func TestFakeMatrixClientDevMode(t *testing.T) {
	ctx := context.Background()
	client := matrix.NewFakeMatrixClient(true)

	roomID := "!course:matrix.dc.ufc.br"

	// Dev token fallback
	ident, err := client.VerifyUser(ctx, roomID, "dev")
	if err != nil {
		t.Fatalf("expected dev mode token to succeed, got: %v", err)
	}
	if ident.UserID != "@professor:matrix.dc.ufc.br" {
		t.Errorf("expected default prof user, got %s", ident.UserID)
	}

	isTeacher, err := client.IsTeacher(ctx, roomID, ident.UserID)
	if err != nil || !isTeacher {
		t.Errorf("expected dev prof to be recognized as teacher")
	}
}

func TestMautrixGatewayVerifyUser(t *testing.T) {
	ctx := context.Background()

	// Mock homeserver for OpenID verification
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("access_token")
		switch token {
		case "valid-token":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"sub":"@prof:matrix.dc.ufc.br"}`))
			return
		case "empty-sub-token":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"sub":"   "}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	gw, err := matrix.NewMautrixGateway(ts.URL, "@bot:matrix.dc.ufc.br", "bot-token")
	if err != nil {
		t.Fatalf("failed creating gateway: %v", err)
	}

	// Empty token
	if _, err := gw.VerifyUser(ctx, "!room:matrix.org", ""); err == nil {
		t.Errorf("expected error for empty token")
	}

	// Invalid token
	if _, err := gw.VerifyUser(ctx, "!room:matrix.org", "bad-token"); err == nil {
		t.Errorf("expected error for bad token")
	}

	// Empty sub in 200 OK response
	if _, err := gw.VerifyUser(ctx, "!room:matrix.org", "empty-sub-token"); err == nil {
		t.Errorf("expected error when openid response has empty sub")
	}

	// Valid token
	ident, err := gw.VerifyUser(ctx, "!room:matrix.org", "valid-token")
	if err != nil {
		t.Fatalf("failed verifying valid token: %v", err)
	}
	if ident.UserID != "@prof:matrix.dc.ufc.br" || ident.RoomID != "!room:matrix.org" {
		t.Errorf("unexpected identity: %+v", ident)
	}
}

func TestMautrixGatewayRoomOperations(t *testing.T) {
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Power levels state event
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"users": {
					"@teacher:matrix.dc.ufc.br": 100,
					"@student:matrix.dc.ufc.br": 0
				},
				"users_default": 0
			}`))
			return
		}
		if r.Method == http.MethodPut {
			// Send message
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"event_id":"$event123"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	gw, err := matrix.NewMautrixGateway(ts.URL, "@bot:matrix.dc.ufc.br", "bot-token")
	if err != nil {
		t.Fatalf("failed creating gateway: %v", err)
	}

	roomID := "!testroom:matrix.dc.ufc.br"

	// Teacher check
	isTeacher, err := gw.IsTeacher(ctx, roomID, "@teacher:matrix.dc.ufc.br")
	if err != nil || !isTeacher {
		t.Errorf("expected teacher to be true, got %v (err: %v)", isTeacher, err)
	}

	// Student check
	isStudentTeacher, err := gw.IsTeacher(ctx, roomID, "@student:matrix.dc.ufc.br")
	if err != nil || isStudentTeacher {
		t.Errorf("expected student to be false, got %v (err: %v)", isStudentTeacher, err)
	}

	// Send notice
	if err := gw.SendNotice(ctx, roomID, "Test Notice"); err != nil {
		t.Errorf("expected SendNotice to succeed, got: %v", err)
	}
}

// roomServer answers the power levels and create event endpoints. An empty
// createJSON makes the create event endpoint fail.
func roomServer(t *testing.T, powerLevelsJSON, createJSON string, createCalls *int) *matrix.MautrixGateway {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "m.room.power_levels"):
			_, _ = w.Write([]byte(powerLevelsJSON))
		case strings.Contains(r.URL.Path, "m.room.create"):
			*createCalls++
			if createJSON == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errcode":"M_NOT_FOUND","error":"missing"}`))
				return
			}
			_, _ = w.Write([]byte(createJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	gw, err := matrix.NewMautrixGateway(ts.URL, "@bot:matrix.dc.ufc.br", "bot-token")
	if err != nil {
		t.Fatalf("failed creating gateway: %v", err)
	}
	return gw
}

func TestMautrixGatewayRecognisesCreatorsOfRoomV12(t *testing.T) {
	const room = "!room:matrix.dc.ufc.br"
	calls := 0
	gw := roomServer(t,
		`{"users":{"@mod:matrix.dc.ufc.br":50},"users_default":0}`,
		`{"type":"m.room.create","state_key":"","sender":"@creator:matrix.dc.ufc.br","content":{"room_version":"12","additional_creators":["@coprof:matrix.dc.ufc.br"]}}`,
		&calls)

	cases := map[string]bool{
		"@creator:matrix.dc.ufc.br": true,  // creator is absent from power levels
		"@coprof:matrix.dc.ufc.br":  true,  // additional creator
		"@mod:matrix.dc.ufc.br":     true,  // moderator listed in power levels
		"@student:matrix.dc.ufc.br": false, // default level
	}
	for user, want := range cases {
		got, err := gw.IsTeacher(context.Background(), room, user)
		if err != nil || got != want {
			t.Errorf("IsTeacher(%s) = %v (err: %v), want %v", user, got, err, want)
		}
	}
	if calls != 1 {
		t.Errorf("the create event never changes and must be fetched once, got %d fetches", calls)
	}
}

func TestMautrixGatewayDoesNotGrantCreatorPowerBeforeRoomV12(t *testing.T) {
	calls := 0
	gw := roomServer(t,
		`{"users":{},"users_default":0}`,
		`{"type":"m.room.create","sender":"@creator:matrix.dc.ufc.br","content":{"room_version":"11"}}`,
		&calls)

	got, err := gw.IsTeacher(context.Background(), "!room:matrix.dc.ufc.br", "@creator:matrix.dc.ufc.br")

	if err != nil || got {
		t.Errorf("in room v11 only the power levels count, got %v (err: %v)", got, err)
	}
}

func TestMautrixGatewayFallsBackToPowerLevelsWhenCreateEventIsMissing(t *testing.T) {
	calls := 0
	gw := roomServer(t, `{"users":{"@teacher:matrix.dc.ufc.br":100},"users_default":0}`, "", &calls)

	teacher, err := gw.IsTeacher(context.Background(), "!room:matrix.dc.ufc.br", "@teacher:matrix.dc.ufc.br")
	if err != nil || !teacher {
		t.Errorf("a teacher listed in the power levels must still be recognised, got %v (err: %v)", teacher, err)
	}
	other, _ := gw.IsTeacher(context.Background(), "!room:matrix.dc.ufc.br", "@creator:matrix.dc.ufc.br")
	if other {
		t.Errorf("without the create event an unlisted user must not be a teacher")
	}
}
