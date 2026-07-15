package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// The adventurer detail-page push has two halves, matched to two sensitivities:
// the public sheet (stats + gear) rides the roster snapshot keyed by the
// anonymous token, and the private self-view (inventory/vault/house/pets) rides
// its own push keyed by localpart. These tests pin both halves at the gogobee
// end — that the public detail carries no handle, and that the private detail is
// keyed so Pete can only ever hand it back to its owner.

// seedDetailPlayer builds a real, playable adventurer: player_meta + tier-0
// equipment (via createAdvCharacter) plus a confirmed combat sheet. Real rows on
// purpose — the same modernc scan hazard the roster snapshot dances around.
func seedDetailPlayer(t *testing.T, uid id.UserID, name string, level int) {
	t.Helper()
	if err := createAdvCharacter(uid, name); err != nil {
		t.Fatalf("createAdvCharacter(%s): %v", uid, err)
	}
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 16, DEX: 14, CON: 15, INT: 10, WIS: 12, CHA: 8,
		HPMax: 42, HPCurrent: 30, TempHP: 5, ArmorClass: 17,
		PendingSetup: false,
	}); err != nil {
		t.Fatalf("SaveDnDCharacter(%s): %v", uid, err)
	}
}

// TestRosterDetailPublicSheet: buildRosterSnapshot hangs the public sheet on
// each board entry — HP/AC/abilities/mods and equipped gear — and nothing that
// could name the player. This is the click-through page's whole payload.
func TestRosterDetailPublicSheet(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@josie:test")
	seedDetailPlayer(t, uid, "Josie", 5)

	snap, err := buildRosterSnapshot(time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	if len(snap.Adventurers) != 1 {
		t.Fatalf("board has %d adventurers, want 1", len(snap.Adventurers))
	}
	e := snap.Adventurers[0]
	if e.Detail == nil {
		t.Fatal("board entry carries no detail sheet — the detail page would fall back to the bare row")
	}
	d := e.Detail
	if d.HPMax != 42 || d.HPCurrent != 30 || d.TempHP != 5 || d.ArmorClass != 17 {
		t.Errorf("detail sheet = %+v, want HP 30/42 (+5 temp), AC 17", d)
	}
	if d.Abilities != [6]int{16, 14, 15, 10, 12, 8} {
		t.Errorf("abilities = %v, want STR..CHA 16,14,15,10,12,8", d.Abilities)
	}
	// CON 15 → +2; a mismatched mods slice would misprint the whole sheet.
	if d.Modifiers[2] != 2 {
		t.Errorf("CON modifier = %d, want +2", d.Modifiers[2])
	}
	// createAdvCharacter seeds tier-0 gear in every slot, so the panel is full.
	if len(d.Gear) != len(allSlots) {
		t.Errorf("gear panel has %d pieces, want %d (one per slot)", len(d.Gear), len(allSlots))
	}
	for _, g := range d.Gear {
		if g.Slot == "" || g.Name == "" {
			t.Errorf("gear piece is unlabelled: %+v", g)
		}
	}
}

// TestRosterDetailExpeditionContext: a player who is out on a run gets the live
// expedition fields layered onto their sheet — supplies, threat, and the room
// counter — so the detail page shows where they are, not just who they are.
func TestRosterDetailExpeditionContext(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@onrun:test")
	exp := seedMischiefTarget(t, uid, 5, 60) // createAdvCharacter + sheet + live run
	_ = exp

	snap, err := buildRosterSnapshot(time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	if len(snap.Adventurers) != 1 {
		t.Fatalf("board has %d adventurers, want 1", len(snap.Adventurers))
	}
	e := snap.Adventurers[0]
	if e.Status != "expedition" {
		t.Fatalf("status = %q, want expedition", e.Status)
	}
	if e.Detail == nil {
		t.Fatal("on-run entry carries no detail")
	}
	if e.Detail.Room == "" {
		t.Error("expedition detail has no room counter — the run context didn't layer on")
	}
}

// TestDetailSnapshotKeyedByLocalpart is the ownership contract at the source.
// The private set is keyed by localpart and carries the player's current board
// token, so Pete can answer "is this signed-in viewer the owner of this page?"
// by a join — never by reversing the one-way token. And it carries the actual
// private goods: inventory, vault, house, pets.
func TestDetailSnapshotKeyedByLocalpart(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@quack:test")
	seedDetailPlayer(t, uid, "Quack", 7)

	// Backpack + vault: two distinct stashes that must not blur together.
	if err := addAdvInventoryItem(uid, AdvItem{Name: "Iron Ore", Type: "ore", Tier: 1, Value: 10}); err != nil {
		t.Fatal(err)
	}
	if err := addAdvInventoryItem(uid, AdvItem{Name: "Jeweled Crown", Type: "treasure", Tier: 4, Value: 5000}); err != nil {
		t.Fatal(err)
	}
	if got := vaultStoreItem(uid, "crown"); got == "" {
		t.Fatal("failed to vault the crown")
	}

	// House + a pet, through the canonical save path.
	adv, err := loadAdvCharacter(uid)
	if err != nil {
		t.Fatalf("loadAdvCharacter: %v", err)
	}
	adv.HouseTier = 2
	adv.HouseLoanBalance = 1500
	adv.PetType = "cat"
	adv.PetName = "Mittens"
	adv.PetLevel = 3
	if err := saveAdvCharacter(adv); err != nil {
		t.Fatalf("saveAdvCharacter: %v", err)
	}

	snap, err := buildDetailSnapshot(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildDetailSnapshot: %v", err)
	}
	if len(snap.Players) != 1 {
		t.Fatalf("detail set has %d players, want 1", len(snap.Players))
	}
	pd := snap.Players[0]
	if pd.Localpart != "quack" {
		t.Errorf("localpart = %q, want the lowercase mxid localpart 'quack'", pd.Localpart)
	}
	if pd.Token != eventToken(uid, "roster") {
		t.Error("detail token is not the board token — the ownership join on Pete would never match the page")
	}
	// Inventory holds the ore (crown was vaulted out of it); vault holds the crown.
	if len(pd.Inventory) != 1 || pd.Inventory[0].Name != "Iron Ore" {
		t.Errorf("inventory = %+v, want just the ore", pd.Inventory)
	}
	if len(pd.Vault) != 1 || pd.Vault[0].Name != "Jeweled Crown" {
		t.Errorf("vault = %+v, want just the crown", pd.Vault)
	}
	if pd.House.Tier != 2 || pd.House.LoanBalance != 1500 {
		t.Errorf("house = %+v, want tier 2 / loan 1500", pd.House)
	}
	if len(pd.Pets) != 1 || pd.Pets[0].Name != "Mittens" || pd.Pets[0].Level != 3 {
		t.Errorf("pets = %+v, want the cat Mittens L3", pd.Pets)
	}
}

// TestDetailSnapshotIgnoresOptOut: the news opt-out governs the *public* board,
// not the private self-view. A player who opted out must still receive their own
// sheet — Pete only ever serves this back to them — so buildDetailSnapshot must
// NOT filter on it, unlike buildRosterSnapshot.
func TestDetailSnapshotIgnoresOptOut(t *testing.T) {
	newMischiefTestDB(t)
	shown := id.UserID("@shown:test")
	hidden := id.UserID("@hidden:test")
	seedDetailPlayer(t, shown, "Shown", 4)
	seedDetailPlayer(t, hidden, "Hidden", 4)
	setNewsOptout(hidden, true)

	// The public board drops the opted-out player...
	roster, err := buildRosterSnapshot(time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	if len(roster.Adventurers) != 1 || roster.Adventurers[0].Name != "Shown" {
		t.Fatalf("public board = %d entries, want just Shown", len(roster.Adventurers))
	}

	// ...but the private detail set keeps them both.
	detail, err := buildDetailSnapshot(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildDetailSnapshot: %v", err)
	}
	if len(detail.Players) != 2 {
		t.Fatalf("private detail set = %d players, want 2 — opt-out must not deny a player their own self-view", len(detail.Players))
	}
	byLP := map[string]bool{}
	for _, p := range detail.Players {
		byLP[p.Localpart] = true
	}
	if !byLP["hidden"] {
		t.Error("opted-out player is missing from the private self-view set")
	}
}

// TestDetailSnapshotSkipsDeadPlayers: the set is drawn from alive players only.
// A dead character has no live board page to own, and pushing their stale
// inventory would leave it standing on Pete after they're gone.
func TestDetailSnapshotSkipsDeadPlayers(t *testing.T) {
	newMischiefTestDB(t)
	alive := id.UserID("@alive:test")
	dead := id.UserID("@dead:test")
	seedDetailPlayer(t, alive, "Alive", 4)
	seedDetailPlayer(t, dead, "Dead", 4)
	if _, err := db.Get().Exec(`UPDATE player_meta SET alive = 0 WHERE user_id = ?`, string(dead)); err != nil {
		t.Fatalf("kill player: %v", err)
	}

	snap, err := buildDetailSnapshot(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildDetailSnapshot: %v", err)
	}
	if len(snap.Players) != 1 || snap.Players[0].Localpart != "alive" {
		t.Fatalf("detail set = %+v, want just the living player", snap.Players)
	}
}
