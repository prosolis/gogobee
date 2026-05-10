package plugin

import (
	"testing"
)

// TestCryptValdrisGraph_Registered verifies the POC graph is in the
// registry (init-time registration) so loadZoneGraph returns it instead
// of the legacy linear compile.
func TestCryptValdrisGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneCryptValdris]
	if !ok {
		t.Fatal("zoneCryptValdrisGraph not registered")
	}
	if g.Entry != "crypt_valdris.entry" {
		t.Errorf("entry node = %q, want crypt_valdris.entry", g.Entry)
	}
	if g.Boss != "crypt_valdris.boss" {
		t.Errorf("boss node = %q, want crypt_valdris.boss", g.Boss)
	}
	if len(g.Nodes) != 8 {
		t.Errorf("nodes = %d, want 8", len(g.Nodes))
	}
}

// TestCryptValdrisGraph_BothPathsReachBoss confirms that both the
// main_hall (elite) path and the side_chapel (perception-gated) path
// can reach the boss. Validates G7 design intent: no path is a
// dead-end.
func TestCryptValdrisGraph_BothPathsReachBoss(t *testing.T) {
	g := zoneCryptValdrisGraph()
	if !reachable(g, "crypt_valdris.main_hall", "crypt_valdris.boss") {
		t.Error("main_hall path unreachable to boss")
	}
	if !reachable(g, "crypt_valdris.side_chapel", "crypt_valdris.boss") {
		t.Error("side_chapel path unreachable to boss")
	}
	if !reachable(g, "crypt_valdris.secret_chamber", "crypt_valdris.boss") {
		t.Error("secret_chamber path unreachable to boss")
	}
}

// TestCryptValdrisGraph_ForkLayout asserts the fork has both options
// and that side_chapel is the locked / hinted one.
func TestCryptValdrisGraph_ForkLayout(t *testing.T) {
	g := zoneCryptValdrisGraph()
	outs := g.outgoingEdges("crypt_valdris.fork")
	if len(outs) != 2 {
		t.Fatalf("fork outgoing edges = %d, want 2", len(outs))
	}
	// outgoingEdges sorts by weight asc — main_hall (weight 1) first.
	if outs[0].To != "crypt_valdris.main_hall" || outs[0].Lock != LockNone {
		t.Errorf("first edge = %+v, want main_hall/LockNone", outs[0])
	}
	if outs[1].To != "crypt_valdris.side_chapel" || outs[1].Lock != LockPerception {
		t.Errorf("second edge = %+v, want side_chapel/LockPerception", outs[1])
	}
	if outs[1].Hint == "" {
		t.Error("side_chapel edge missing player-facing hint")
	}
}

// TestCryptValdrisGraph_SecretGated verifies secret_chamber sits behind
// a higher-DC perception lock and carries a loot bias.
func TestCryptValdrisGraph_SecretGated(t *testing.T) {
	g := zoneCryptValdrisGraph()
	outs := g.outgoingEdges("crypt_valdris.side_chapel")
	var secretEdge *ZoneEdge
	for i := range outs {
		if outs[i].To == "crypt_valdris.secret_chamber" {
			secretEdge = &outs[i]
			break
		}
	}
	if secretEdge == nil {
		t.Fatal("no edge from side_chapel to secret_chamber")
	}
	if secretEdge.Lock != LockPerception {
		t.Errorf("secret edge lock = %s, want perception", secretEdge.Lock)
	}
	if dc := lockDataInt(secretEdge.LockData, "dc", 0); dc < 15 {
		t.Errorf("secret DC = %d, want >= 15", dc)
	}
	secret := g.Nodes["crypt_valdris.secret_chamber"]
	if secret.Content.LootBias < 1.5 {
		t.Errorf("secret LootBias = %v, want >= 1.5 (cruel without loot)", secret.Content.LootBias)
	}
}

// TestCurrentRoomType_GraphAuthored verifies that CurrentRoomType
// resolves via the registered graph's node kind rather than the legacy
// RoomSeq lookup. This is what makes side_path nodes (e.g. the
// secret_chamber) resolve to the right RoomType when the player
// diverges from the canonical RoomSeq layout.
func TestCurrentRoomType_GraphAuthored(t *testing.T) {
	// Mismatched RoomSeq vs. live CurrentNode: CurrentRoom=2 would land
	// on RoomTrap in the legacy fallback, but the graph node kind is
	// Elite (main_hall). Graph wins.
	run := &DungeonRun{
		ZoneID:      ZoneCryptValdris,
		CurrentRoom: 2,
		CurrentNode: "crypt_valdris.main_hall",
		RoomSeq:     []RoomType{RoomEntry, RoomExploration, RoomTrap, RoomElite, RoomBoss},
	}
	if got := run.CurrentRoomType(); got != RoomElite {
		t.Errorf("graph mode + main_hall: got %s, want %s", got, RoomElite)
	}

	run.CurrentNode = "crypt_valdris.boss"
	if got := run.CurrentRoomType(); got != RoomBoss {
		t.Errorf("graph mode + boss node: got %s, want %s", got, RoomBoss)
	}

	run.CurrentNode = "crypt_valdris.secret_chamber"
	// Secret nodes flatten to RoomExploration for legacy resolveRoom
	// dispatch (per nodeKindToRoomType — no separate Secret RoomType yet).
	if got := run.CurrentRoomType(); got != RoomExploration {
		t.Errorf("graph mode + secret node: got %s, want %s", got, RoomExploration)
	}
}

