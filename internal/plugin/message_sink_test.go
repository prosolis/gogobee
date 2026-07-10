package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// captureSink is the test implementation of MessageSink. It records every
// outbound message a handler emits so a test can assert on a handler whose
// only observable output is a DM or a room announcement — the paths that had
// no seam below the live client before this. Install it with installSink.
type captureSink struct {
	msgs []outboundMessage
}

func (s *captureSink) Capture(m outboundMessage) (id.EventID, error) {
	s.msgs = append(s.msgs, m)
	// A stable, non-empty id keeps the *ID send variants honest for callers
	// that expect an event id back (e.g. one they would later edit).
	return id.EventID("$sink"), nil
}

// dmsTo returns the text of every DM captured for a given user, in order.
func (s *captureSink) dmsTo(user id.UserID) []string {
	var out []string
	for _, m := range s.msgs {
		if m.ToUser == user {
			out = append(out, m.Text)
		}
	}
	return out
}

// roomMsgs returns the text of every room message captured for a room.
func (s *captureSink) roomMsgs(room id.RoomID) []string {
	var out []string
	for _, m := range s.msgs {
		if m.ToRoom == room {
			out = append(out, m.Text)
		}
	}
	return out
}

// installSink points a plugin's outbound sends at a fresh captureSink and
// returns it. The Base carries no client in these tests, so without the sink
// SendDM would silently no-op — which is exactly why these paths were untestable.
func installSink(p *AdventurePlugin) *captureSink {
	s := &captureSink{}
	p.Sink = s
	return s
}

// TestMessageSink_CapturesDMAndRoom is the seam's own unit test: it proves both
// the DM and room-message routes divert into the sink with the right target and
// text, and that the *ID variants report back the id the sink chose.
func TestMessageSink_CapturesDMAndRoom(t *testing.T) {
	p := &AdventurePlugin{}
	sink := installSink(p)

	user := id.UserID("@player:example.org")
	room := id.RoomID("!hall:example.org")

	if err := p.SendDM(user, "a direct message"); err != nil {
		t.Fatalf("SendDM: %v", err)
	}
	if err := p.SendMessage(room, "a room announcement"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	evID, err := p.SendDMID(user, "an editable dm")
	if err != nil {
		t.Fatalf("SendDMID: %v", err)
	}
	if evID != "$sink" {
		t.Errorf("SendDMID returned %q, want the sink's id", evID)
	}

	if got := sink.dmsTo(user); len(got) != 2 || got[0] != "a direct message" || got[1] != "an editable dm" {
		t.Errorf("DMs to %s = %v", user, got)
	}
	if got := sink.roomMsgs(room); len(got) != 1 || got[0] != "a room announcement" {
		t.Errorf("room messages = %v", got)
	}
	// A DM must not leak into the room bucket, or vice versa.
	if len(sink.roomMsgs(id.RoomID(user))) != 0 {
		t.Error("a DM was miscaptured as a room message")
	}
}

// TestExpeditionCmdLeave_DMsBothEnds drives the real handler behind review
// fix #2 end to end — the member soft-lock path — and asserts on the two DMs it
// sends. Before the sink this was unreachable in a unit test: SendDM no-ops
// without a client, so nothing could observe that the leader was notified and
// the departing member was told they turned back.
func TestExpeditionCmdLeave_DMsBothEnds(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead-leave:example.org")
	member := id.UserID("@member-leave:example.org")
	seedExpedition(t, "exp-leave", owner, "active")
	seatLeaderFixture(t, "exp-leave")
	if err := joinParty("exp-leave", member); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	sink := installSink(p)

	if err := p.expeditionCmdLeave(MessageContext{Sender: member}); err != nil {
		t.Fatalf("expeditionCmdLeave: %v", err)
	}

	// The departing member is freed and told so.
	memberDMs := sink.dmsTo(member)
	if len(memberDMs) != 1 || memberDMs[0] != "You turn back for town. Your supplies stay with the party." {
		t.Errorf("member DMs = %v", memberDMs)
	}
	// The leader is notified their party shrank.
	leaderDMs := sink.dmsTo(owner)
	if len(leaderDMs) != 1 || leaderDMs[0] == "" {
		t.Fatalf("leader DMs = %v, want one departure notice", leaderDMs)
	}
	if !strings.Contains(leaderDMs[0], "turned back") || !strings.Contains(leaderDMs[0], "supplies stay") {
		t.Errorf("leader notice = %q", leaderDMs[0])
	}

	// And the DB actually reflects the departure the DMs described.
	if n, _ := partySize("exp-leave"); n != 1 {
		t.Errorf("after leave, partySize = %d, want 1", n)
	}
}
