package plugin

import (
	"testing"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// webMischiefPlugin builds a plugin the web placement path can run against: a
// euro ledger, a capture sink so DMs go nowhere, and a bare client carrying only
// a user id — enough for mischiefBuyerMXID to read the homeserver, never used for
// a real call (DisplayName fast-fails to the localpart, sends hit the sink).
func webMischiefPlugin(t *testing.T) *AdventurePlugin {
	t.Helper()
	p := &AdventurePlugin{euro: NewEuroPlugin(nil)}
	p.Sink = &captureSink{}
	// A well-formed client pointed at an unroutable host: mischiefBuyerMXID reads
	// only its user id (for the homeserver), and the DM courtesy path's lone real
	// call — GetDisplayName — fails fast against 127.0.0.1:1 and falls back to the
	// localpart, so nothing here touches a network or hangs.
	cli, err := mautrix.NewClient("http://127.0.0.1:1", id.UserID("@gogobee:example.org"), "")
	if err != nil {
		t.Fatal(err)
	}
	p.Client = cli
	return p
}

func webOrder(guid, buyer, targetToken, tier string, signed bool) peteclient.MischiefOrder {
	return peteclient.MischiefOrder{
		GUID: guid, BuyerUsername: buyer, TargetToken: targetToken,
		TargetName: "Josie", Tier: tier, Signed: signed,
	}
}

// TestResolveRosterToken round-trips the one-way board token: gogobee can't
// invert it, but recomputing every live player's token finds the match, which is
// how a web order names its mark.
func TestResolveRosterToken(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@josie:example.org")
	seedMischiefTarget(t, uid, 5, 60)

	tok := eventToken(uid, "roster")
	got, ok := resolveRosterToken(tok)
	if !ok || got != uid {
		t.Fatalf("resolveRosterToken(%s) = %q, %v; want %s", tok, got, ok, uid)
	}
	if _, ok := resolveRosterToken("not-a-real-token"); ok {
		t.Error("resolveRosterToken matched a token no player owns")
	}
}

// TestPlaceWebMischiefHappyPath: a funded buyer, a mark on a live expedition, a
// tier gogobee offers — a web-sourced contract opens, stamped with the order.
func TestPlaceWebMischiefHappyPath(t *testing.T) {
	newMischiefTestDB(t)
	p := webMischiefPlugin(t)

	target := id.UserID("@josie:example.org")
	seedMischiefTarget(t, target, 5, 60)
	buyer := id.UserID("@reala:example.org")
	p.euro.Credit(buyer, 1000, "test seed")

	order := webOrder("ord-1", "reala", eventToken(target, "roster"), "grunt", false)
	res := p.placeWebMischief(order)

	if res.Status != mischiefWebPlaced {
		t.Fatalf("status = %q (%s), want placed", res.Status, res.Detail)
	}
	c := mischiefContractByOrderGUID("ord-1")
	if c == nil {
		t.Fatal("no contract stamped with the order guid")
	}
	if c.Source != "web" || c.TargetID != target || c.BuyerID != buyer {
		t.Fatalf("contract = %+v", c)
	}
	if bal := p.euro.GetBalance(buyer); bal != 960 {
		t.Errorf("buyer balance = %v, want 960 (1000 - 40 grunt)", bal)
	}
}

// TestPlaceWebMischiefIdempotent is the whole point of keying on the order guid:
// the poll loop retries, so a second placement of the same order must not open a
// second contract or debit the buyer twice.
func TestPlaceWebMischiefIdempotent(t *testing.T) {
	newMischiefTestDB(t)
	p := webMischiefPlugin(t)

	target := id.UserID("@josie:example.org")
	seedMischiefTarget(t, target, 5, 60)
	buyer := id.UserID("@reala:example.org")
	p.euro.Credit(buyer, 1000, "test seed")

	order := webOrder("ord-dup", "reala", eventToken(target, "roster"), "mob", false)
	if res := p.placeWebMischief(order); res.Status != mischiefWebPlaced {
		t.Fatalf("first placement = %q", res.Status)
	}
	balAfterFirst := p.euro.GetBalance(buyer)

	// Same order again — the retry.
	if res := p.placeWebMischief(order); res.Status != mischiefWebPlaced {
		t.Fatalf("replay = %q, want placed", res.Status)
	}
	if bal := p.euro.GetBalance(buyer); bal != balAfterFirst {
		t.Errorf("replay moved money: balance %v -> %v", balAfterFirst, bal)
	}

	// Exactly one contract exists for this target.
	var n int
	_ = db.Get().QueryRow(`SELECT COUNT(*) FROM mischief_contracts WHERE order_guid = ?`, "ord-dup").Scan(&n)
	if n != 1 {
		t.Fatalf("order opened %d contracts, want 1", n)
	}
}

// TestPlaceWebMischiefBouncedFunds: a broke buyer is turned away before any money
// moves — a mischief buy takes no credit, same as the Matrix path.
func TestPlaceWebMischiefBouncedFunds(t *testing.T) {
	newMischiefTestDB(t)
	p := webMischiefPlugin(t)

	target := id.UserID("@josie:example.org")
	seedMischiefTarget(t, target, 5, 60)
	buyer := id.UserID("@broke:example.org")
	p.euro.Credit(buyer, 10, "test seed") // a grunt is 40

	order := webOrder("ord-broke", "broke", eventToken(target, "roster"), "grunt", false)
	res := p.placeWebMischief(order)

	if res.Status != mischiefWebBouncedFunds {
		t.Fatalf("status = %q, want bounced_funds", res.Status)
	}
	if mischiefContractByOrderGUID("ord-broke") != nil {
		t.Error("a bounced order still opened a contract")
	}
	if bal := p.euro.GetBalance(buyer); bal != 10 {
		t.Errorf("balance = %v, want 10 untouched (no debt for a mischief buy)", bal)
	}
}

// TestPlaceWebMischiefBouncedIneligibleRefunds: a mark who isn't on an expedition
// can't be hit; the buyer is refunded whole, net zero.
func TestPlaceWebMischiefBouncedIneligibleRefunds(t *testing.T) {
	newMischiefTestDB(t)
	p := webMischiefPlugin(t)

	// A real, alive character but NOT on an expedition — ineligible.
	idle := id.UserID("@idle:example.org")
	if err := createAdvCharacter(idle, "idle"); err != nil {
		t.Fatal(err)
	}
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: idle, Race: RaceHuman, Class: ClassFighter, Level: 5,
		HPMax: 40, HPCurrent: 40, ArmorClass: 16,
	}); err != nil {
		t.Fatal(err)
	}
	buyer := id.UserID("@reala:example.org")
	p.euro.Credit(buyer, 1000, "test seed")

	order := webOrder("ord-inelig", "reala", eventToken(idle, "roster"), "elite", false)
	res := p.placeWebMischief(order)

	if res.Status != mischiefWebBouncedIneligible {
		t.Fatalf("status = %q (%s), want bounced_ineligible", res.Status, res.Detail)
	}
	if bal := p.euro.GetBalance(buyer); bal != 1000 {
		t.Errorf("buyer balance = %v, want 1000 (debited then refunded whole)", bal)
	}
}

// TestPlaceWebMischiefSelfTarget: the reconstructed buyer and the resolved target
// are the same person — refused, like the Matrix path.
func TestPlaceWebMischiefSelfTarget(t *testing.T) {
	newMischiefTestDB(t)
	p := webMischiefPlugin(t)

	me := id.UserID("@reala:example.org")
	seedMischiefTarget(t, me, 5, 60)
	p.euro.Credit(me, 1000, "test seed")

	order := webOrder("ord-self", "reala", eventToken(me, "roster"), "grunt", false)
	if res := p.placeWebMischief(order); res.Status != mischiefWebBouncedIneligible {
		t.Fatalf("self-target status = %q, want bounced_ineligible", res.Status)
	}
	if bal := p.euro.GetBalance(me); bal != 1000 {
		t.Errorf("self-target moved money: balance %v", bal)
	}
}
