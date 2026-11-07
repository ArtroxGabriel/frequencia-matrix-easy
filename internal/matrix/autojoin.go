package matrix

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// InviteAcceptor is implemented by gateways that can accept room invitations.
type InviteAcceptor interface {
	// RunAutoJoin blocks, joining every room the bot is invited to by a user
	// of one of allowedServers (any server when the list is empty). It returns
	// nil when ctx is cancelled.
	RunAutoJoin(ctx context.Context, allowedServers []string) error
}

var _ InviteAcceptor = (*MautrixGateway)(nil)

// RunAutoJoin lets a teacher add the bot to a room like any other user: invite
// it, and it joins. The bot needs to be in the room to read its power levels
// and to post notices.
//
// Invitations received while the app was offline are delivered by the first
// /sync, so nothing needs to be stored between runs.
func (m *MautrixGateway) RunAutoJoin(ctx context.Context, allowedServers []string) error {
	syncer := mautrix.NewDefaultSyncer()
	syncer.FilterJSON = inviteOnlyFilter()
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		m.answerInvite(ctx, evt, allowedServers)
	})
	m.client.Syncer = syncer

	err := m.client.SyncWithContext(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// answerInvite joins the room when evt is an allowed invitation for the bot,
// and declines it otherwise so that it does not come back at the next start.
func (m *MautrixGateway) answerInvite(ctx context.Context, evt *event.Event, allowedServers []string) {
	if !isInviteForUser(evt, m.client.UserID) {
		return
	}
	if !inviterAllowed(evt.Sender, allowedServers) {
		m.declineInvite(ctx, evt)
		return
	}
	if _, err := m.client.JoinRoomByID(ctx, evt.RoomID); err != nil {
		log.Printf("[WARN] could not join room %s invited by %s: %v", evt.RoomID, evt.Sender, err)
		return
	}
	log.Printf("[INFO] joined room %s, invited by %s", evt.RoomID, evt.Sender)
}

// declineInvite leaves a room the bot was invited to by a server that is not
// allowed. The invite seen in /sync can be stale (a restart right after
// joining may replay it), and leaving would then remove the bot from a room it
// is a member of, so the real membership is checked first.
func (m *MautrixGateway) declineInvite(ctx context.Context, evt *event.Event) {
	joined, err := m.client.JoinedRooms(ctx)
	if err != nil {
		log.Printf("[WARN] not declining invite to %s: could not list joined rooms: %v", evt.RoomID, err)
		return
	}
	if slices.Contains(joined.JoinedRooms, evt.RoomID) {
		return
	}
	log.Printf("[INFO] declining invite to %s from %s: server not allowed", evt.RoomID, evt.Sender)
	if _, err := m.client.LeaveRoom(ctx, evt.RoomID); err != nil {
		log.Printf("[WARN] could not decline invite to %s: %v", evt.RoomID, err)
	}
}

// isInviteForUser reports whether evt is a pending invitation for user. Member
// events of rooms the bot already joined are ignored.
func isInviteForUser(evt *event.Event, user id.UserID) bool {
	if evt.Mautrix.EventSource&event.SourceInvite == 0 || evt.GetStateKey() != string(user) {
		return false
	}
	member := evt.Content.AsMember()
	return member != nil && member.Membership == event.MembershipInvite
}

// inviterAllowed reports whether sender belongs to one of the allowed
// homeservers. An empty list allows everyone.
func inviterAllowed(sender id.UserID, allowedServers []string) bool {
	if len(allowedServers) == 0 {
		return true
	}
	_, server, err := sender.Parse()
	if err != nil {
		return false
	}
	for _, allowed := range allowedServers {
		if strings.EqualFold(strings.TrimSpace(allowed), server) {
			return true
		}
	}
	return false
}

// inviteOnlyFilter keeps /sync light: the bot only cares about invitations,
// which are not affected by the event type filters.
func inviteOnlyFilter() *mautrix.Filter {
	nothing := func() *mautrix.FilterPart {
		return &mautrix.FilterPart{NotTypes: []event.Type{{Type: "*"}}}
	}
	return &mautrix.Filter{
		Presence:    nothing(),
		AccountData: nothing(),
		Room: &mautrix.RoomFilter{
			Timeline:    nothing(),
			State:       nothing(),
			Ephemeral:   nothing(),
			AccountData: nothing(),
		},
	}
}
