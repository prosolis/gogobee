package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// partyCasualtyLine leans on joinNames so a party's casualty DM punctuates its
// name list the same way every other multi-name line the bot sends does.
func TestPartyCasualtyLine_JoinsNamesLikeTheRestOfTheBot(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	// No player_meta rows, so every name falls back to its localpart.
	cases := []struct {
		name   string
		downed []id.UserID
		want   string
	}{
		{"nobody fell", nil, ""},
		{"one", []id.UserID{"@ana:x"}, "💀 **ana** went down."},
		{"two", []id.UserID{"@ana:x", "@bo:x"}, "💀 **ana** and **bo** went down."},
		{"three", []id.UserID{"@ana:x", "@bo:x", "@cy:x"}, "💀 **ana**, **bo**, and **cy** went down."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := partyCasualtyLine(tc.downed); got != tc.want {
				t.Fatalf("partyCasualtyLine(%v) = %q, want %q", tc.downed, got, tc.want)
			}
		})
	}
}
