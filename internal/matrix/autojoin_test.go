package matrix_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/matrix"
)

const botID = "@bot:ufc.br"

func memberEvent(sender, target, membership string) string {
	return fmt.Sprintf(`{"type":"m.room.member","state_key":%q,"sender":%q,"content":{"membership":%q}}`,
		target, sender, membership)
}

func invitedRoom(sender, target, membership string) string {
	return fmt.Sprintf(`{"invite_state":{"events":[%s]}}`, memberEvent(sender, target, membership))
}

// homeserver is a minimal /sync server that records which rooms were joined
// and declined.
type homeserver struct {
	mu       sync.Mutex
	joined   []string
	declined []string
	member   []string // rooms reported by /joined_rooms
	syncBody string
	served   chan struct{}
}

func newHomeserver(syncBody string) (*homeserver, *httptest.Server) {
	hs := &homeserver{syncBody: syncBody, served: make(chan struct{}, 1)}
	return hs, httptest.NewServer(http.HandlerFunc(hs.serve))
}

func (hs *homeserver) serve(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/filter"):
		_, _ = w.Write([]byte(`{"filter_id":"1"}`))
	case strings.HasSuffix(path, "/sync"):
		hs.serveSync(w, r)
	case strings.HasSuffix(path, "/joined_rooms"):
		hs.mu.Lock()
		rooms, _ := json.Marshal(map[string][]string{"joined_rooms": append([]string{}, hs.member...)})
		hs.mu.Unlock()
		_, _ = w.Write(rooms)
	case strings.HasSuffix(path, "/join"):
		hs.record(&hs.joined, path, "/join")
		_, _ = w.Write([]byte(`{"room_id":"ok"}`))
	case strings.HasSuffix(path, "/leave"):
		hs.record(&hs.declined, path, "/leave")
		_, _ = w.Write([]byte(`{}`))
	default:
		http.NotFound(w, r)
	}
}

func (hs *homeserver) serveSync(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("since") == "" {
		_, _ = w.Write([]byte(hs.syncBody))
		return
	}
	select {
	case hs.served <- struct{}{}:
	default:
	}
	<-r.Context().Done()
}

func (hs *homeserver) record(list *[]string, path, suffix string) {
	room := strings.TrimSuffix(path[strings.Index(path, "/rooms/")+len("/rooms/"):], suffix)
	hs.mu.Lock()
	defer hs.mu.Unlock()
	*list = append(*list, room)
	sort.Strings(*list)
}

func (hs *homeserver) snapshot() (joined, declined []string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return append([]string(nil), hs.joined...), append([]string(nil), hs.declined...)
}

// runUntilFirstSyncIsHandled starts RunAutoJoin and stops it once the first
// /sync response was processed (the second /sync request arrived).
func runUntilFirstSyncIsHandled(t *testing.T, hs *homeserver, url string, servers []string) {
	t.Helper()
	gw, err := matrix.NewMautrixGateway(url, botID, "bot-token")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- gw.RunAutoJoin(ctx, servers) }()

	select {
	case <-hs.served:
	case <-time.After(5 * time.Second):
		t.Fatal("the first sync was never processed")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("RunAutoJoin returned %v after cancellation", err)
	}
}

func TestAutoJoinAcceptsInvitesFromAllowedServers(t *testing.T) {
	body := fmt.Sprintf(`{"next_batch":"s1","rooms":{"invite":{
		"!teacher":%s,
		"!outsider":%s}}}`,
		invitedRoom("@prof:ufc.br", botID, "invite"),
		invitedRoom("@spam:evil.org", botID, "invite"))
	hs, ts := newHomeserver(body)
	defer ts.Close()

	runUntilFirstSyncIsHandled(t, hs, ts.URL, []string{"ufc.br"})

	joined, declined := hs.snapshot()
	if len(joined) != 1 || joined[0] != "%21teacher" && joined[0] != "!teacher" {
		t.Errorf("joined = %v, want only !teacher", joined)
	}
	if len(declined) != 1 || declined[0] != "%21outsider" && declined[0] != "!outsider" {
		t.Errorf("declined = %v, want only !outsider", declined)
	}
}

func TestAutoJoinWithoutServerListAcceptsAnyone(t *testing.T) {
	body := fmt.Sprintf(`{"next_batch":"s1","rooms":{"invite":{"!any":%s}}}`,
		invitedRoom("@someone:other.org", botID, "invite"))
	hs, ts := newHomeserver(body)
	defer ts.Close()

	runUntilFirstSyncIsHandled(t, hs, ts.URL, nil)

	joined, declined := hs.snapshot()
	if len(joined) != 1 || len(declined) != 0 {
		t.Errorf("joined = %v, declined = %v, want one join and no decline", joined, declined)
	}
}

func TestAutoJoinIgnoresEventsThatAreNotInvitesForTheBot(t *testing.T) {
	body := fmt.Sprintf(`{"next_batch":"s1","rooms":{
		"invite":{
			"!other-user":%s,
			"!not-invite":%s},
		"join":{"!already-in":{"timeline":{"events":[%s]}}}}}`,
		invitedRoom("@prof:ufc.br", "@someone:ufc.br", "invite"),
		invitedRoom("@prof:ufc.br", botID, "leave"),
		memberEvent("@prof:ufc.br", botID, "invite"))
	hs, ts := newHomeserver(body)
	defer ts.Close()

	runUntilFirstSyncIsHandled(t, hs, ts.URL, nil)

	if joined, declined := hs.snapshot(); len(joined) != 0 || len(declined) != 0 {
		t.Errorf("joined = %v, declined = %v, want nothing", joined, declined)
	}
}

func TestAutoJoinReportsFatalSyncErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errcode":"M_UNKNOWN_TOKEN","error":"bad token"}`))
	}))
	defer ts.Close()

	gw, err := matrix.NewMautrixGateway(ts.URL, botID, "bad")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := gw.RunAutoJoin(ctx, nil); err == nil {
		t.Error("expected an error for an invalid access token")
	}
}

func TestAutoJoinDoesNotLeaveRoomsTheBotAlreadyJoined(t *testing.T) {
	body := fmt.Sprintf(`{"next_batch":"s1","rooms":{"invite":{
		"!stale":%s,
		"!pending":%s}}}`,
		invitedRoom("@spam:evil.org", botID, "invite"),
		invitedRoom("@spam:evil.org", botID, "invite"))
	hs, ts := newHomeserver(body)
	defer ts.Close()
	hs.member = []string{"!stale"} // a replayed invite for a room the bot already is in

	runUntilFirstSyncIsHandled(t, hs, ts.URL, []string{"ufc.br"})

	joined, declined := hs.snapshot()
	if len(joined) != 0 {
		t.Errorf("joined = %v, want none", joined)
	}
	if len(declined) != 1 || declined[0] != "%21pending" && declined[0] != "!pending" {
		t.Errorf("declined = %v, want only !pending (never !stale, the bot is a member)", declined)
	}
}
