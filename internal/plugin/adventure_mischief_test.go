package plugin

import (
	"math/rand/v2"
	"testing"
	"time"

	"gogobee/internal/db"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

func newMischiefTestDB(t *testing.T) {
	t.Helper()
	db.Close()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

func seedContract(t *testing.T, buyer, target id.UserID, tier string, signed bool) *mischiefContract {
	t.Helper()
	td, ok := mischiefTierByKey(tier)
	if !ok {
		t.Fatalf("unknown tier %q", tier)
	}
	paid := td.Fee
	if signed {
		paid = mischiefSignedFee(td.Fee)
	}
	c := &mischiefContract{
		ID:           uuid.NewString(),
		BuyerID:      buyer,
		TargetID:     target,
		Tier:         td.Key,
		Fee:          td.Fee,
		Paid:         paid,
		Status:       mischiefStatusOpen,
		Signed:       signed,
		Source:       "matrix",
		CreatedAt:    time.Now().UTC(),
		WindowEndsAt: time.Now().UTC().Add(mischiefWindow),
	}
	if err := insertMischiefContract(c); err != nil {
		t.Fatalf("seed contract (%s → %s): %v", buyer, target, err)
	}
	return c
}

// The anti-collusion invariant, and the only reason no danger multiplier is
// needed: whatever the tier and whether or not the buyer signed, the target's
// survival purse is strictly less than what the buyer parted with. Two players
// who try to move money this way lose to !baltransfer, which is free.
func TestMischiefPurseNeverExceedsOutlay(t *testing.T) {
	for _, tier := range mischiefTiers {
		purse := mischiefPurse(tier.Fee, tier.PayoutPct)
		for _, paid := range []int{tier.Fee, mischiefSignedFee(tier.Fee)} {
			if purse >= paid {
				t.Errorf("%s: purse %d >= buyer outlay %d — collusion beats !baltransfer",
					tier.Key, purse, paid)
			}
		}
		if tier.PayoutPct > 0.75 {
			t.Errorf("%s: payout %.2f exceeds the 75%% cap", tier.Key, tier.PayoutPct)
		}
	}
}

// The cap is enforced in code, not just in the table — an escalation (M2) raises
// the payout basis, and nothing it can do may push the purse past 75%.
func TestMischiefPurseHardCap(t *testing.T) {
	if got := mischiefPurse(1000, 0.95); got != 750 {
		t.Fatalf("mischiefPurse(1000, 0.95) = %d, want 750 (capped)", got)
	}
}

func TestMischiefSignedFee(t *testing.T) {
	// The tier table's signed prices, as priced in M0.
	want := map[string]int{"grunt": 50, "mob": 125, "elite": 438, "boss": 1500}
	for _, tier := range mischiefTiers {
		if got := mischiefSignedFee(tier.Fee); got != want[tier.Key] {
			t.Errorf("%s signed fee = %d, want %d", tier.Key, got, want[tier.Key])
		}
	}
}

// Exactly one of delivery and fizzle may claim a contract, however many ticks
// race for it. Without this a restart mid-delivery could both maim the target
// and refund the buyer.
func TestMischiefClaimIsExactlyOnce(t *testing.T) {
	newMischiefTestDB(t)
	c := seedContract(t, "@buyer:x", "@target:x", "elite", false)

	if !claimMischiefForDelivery(c.ID) {
		t.Fatal("first claim should win")
	}
	if claimMischiefForDelivery(c.ID) {
		t.Error("second claim won — a contract could be delivered twice")
	}
	if fizzleMischiefContract(c.ID) {
		t.Error("fizzle won after the delivery claim — buyer would be refunded a hit that landed")
	}

	c2 := seedContract(t, "@buyer:x", "@other:x", "grunt", false)
	if !fizzleMischiefContract(c2.ID) {
		t.Fatal("first fizzle should win")
	}
	if claimMischiefForDelivery(c2.ID) {
		t.Error("delivery claimed a fizzled contract — buyer would pay twice")
	}
}

// Only a closed window is due, and a resolved contract never comes back.
func TestMischiefDueWindow(t *testing.T) {
	newMischiefTestDB(t)
	now := time.Now().UTC()
	c := seedContract(t, "@buyer:x", "@target:x", "mob", false)

	if due := loadMischiefDue(now); len(due) != 0 {
		t.Fatalf("contract is due before its window closed: %d", len(due))
	}
	due := loadMischiefDue(now.Add(mischiefWindow + time.Minute))
	if len(due) != 1 || due[0].ID != c.ID {
		t.Fatalf("want the contract due after its window; got %d", len(due))
	}

	resolveMischiefContract(c.ID, mischiefStatusDelivered, mischiefOutcomeSurvived)
	if due := loadMischiefDue(now.Add(2 * mischiefWindow)); len(due) != 0 {
		t.Error("a resolved contract came back around")
	}
}

// One live contract per target: two monsters converging on the same person is a
// mugging, not mischief.
func TestMischiefOneLiveContractPerTarget(t *testing.T) {
	newMischiefTestDB(t)
	target := id.UserID("@target:x")
	if liveMischiefForTarget(target) != nil {
		t.Fatal("no contract seeded, but one is live")
	}
	c := seedContract(t, "@buyer:x", target, "grunt", false)
	if live := liveMischiefForTarget(target); live == nil || live.ID != c.ID {
		t.Fatal("open contract should be live")
	}
	// A claimed-but-unresolved contract still blocks — the monster is en route.
	claimMischiefForDelivery(c.ID)
	if liveMischiefForTarget(target) == nil {
		t.Error("a delivering contract should still block a second one")
	}
	resolveMischiefContract(c.ID, mischiefStatusDelivered, mischiefOutcomeDowned)
	if liveMischiefForTarget(target) != nil {
		t.Error("a resolved contract still reads as live")
	}
}

// The post-resolution breather. Also the regression guard for the modernc
// datetime hazard: resolved_at round-trips through the driver's own encoding
// (bound Go time in, time.Time out), so the row has to be a real one — a
// hand-written stamp in some other format would compare lexicographically and
// silently never expire.
func TestMischiefTargetCooldown(t *testing.T) {
	newMischiefTestDB(t)
	target := id.UserID("@target:x")
	now := time.Now().UTC()

	if cooling, _ := mischiefTargetCoolingDown(target, now); cooling {
		t.Fatal("a target with no history is cooling down")
	}
	c := seedContract(t, "@buyer:x", target, "grunt", false)
	resolveMischiefContract(c.ID, mischiefStatusDelivered, mischiefOutcomeSurvived)

	cooling, wait := mischiefTargetCoolingDown(target, now)
	if !cooling {
		t.Fatal("a just-resolved target should be cooling down")
	}
	// resolved_at is stamped a hair after `now`, so allow a second of slack.
	if wait <= 0 || wait > mischiefTargetCooldown+time.Second {
		t.Errorf("wait = %s, want within (0, %s]", wait, mischiefTargetCooldown)
	}
	if cooling, _ := mischiefTargetCoolingDown(target, now.Add(mischiefTargetCooldown+time.Minute)); cooling {
		t.Error("still cooling down after the cooldown elapsed")
	}
}

// Rate limits: contracts per buyer per day, and boss-tier per target per week.
// Fizzled contracts count against the buyer — the limit is on how often you may
// point a monster at someone, not on how often it connects.
func TestMischiefRateLimits(t *testing.T) {
	newMischiefTestDB(t)
	buyer := id.UserID("@buyer:x")
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	if n := mischiefBuyerCountSince(buyer, dayAgo); n != 0 {
		t.Fatalf("fresh buyer count = %d, want 0", n)
	}
	c1 := seedContract(t, buyer, "@a:x", "grunt", false)
	seedContract(t, buyer, "@b:x", "mob", false)
	fizzleMischiefContract(c1.ID)
	if n := mischiefBuyerCountSince(buyer, dayAgo); n != mischiefBuyerDailyCap {
		t.Errorf("buyer count = %d, want %d (a fizzle still counts)", n, mischiefBuyerDailyCap)
	}

	target := id.UserID("@whale:x")
	weekAgo := now.Add(-mischiefBossPerTargetWindow)
	if n := mischiefBossCountSince(target, weekAgo); n != 0 {
		t.Fatalf("fresh boss count = %d, want 0", n)
	}
	elite := seedContract(t, "@other:x", target, "elite", false)
	if n := mischiefBossCountSince(target, weekAgo); n != 0 {
		t.Error("an elite contract counted against the boss-per-week limit")
	}
	// Resolve it first: only one contract may be live against a target at a time,
	// and the database enforces that (idx_mischief_one_live_per_target).
	resolveMischiefContract(elite.ID, mischiefStatusDelivered, mischiefOutcomeSurvived)
	seedContract(t, "@other:x", target, "boss", false)
	if n := mischiefBossCountSince(target, weekAgo); n != 1 {
		t.Errorf("boss count = %d, want 1", n)
	}
}

// The database, not a read-then-write check, is what stops two buyers racing a
// monster at the same target. The placement path holds only the buyer's lock, so
// without this index both inserts would land and the target would be hit twice.
func TestMischiefSecondLiveContractIsRejected(t *testing.T) {
	newMischiefTestDB(t)
	target := id.UserID("@target:x")
	seedContract(t, "@buyer1:x", target, "grunt", false)

	second := &mischiefContract{
		ID: uuid.NewString(), BuyerID: "@buyer2:x", TargetID: target,
		Tier: "elite", Fee: 350, Paid: 350, Status: mischiefStatusOpen,
		Source: "matrix", CreatedAt: time.Now().UTC(),
		WindowEndsAt: time.Now().UTC().Add(mischiefWindow),
	}
	if err := insertMischiefContract(second); err == nil {
		t.Fatal("a second live contract was accepted — the target can be hit twice at once")
	}
}

// A delivery that dies mid-flight (restart, panic) must not strand the contract:
// the row would block the target forever and the buyer's money would be gone.
func TestMischiefStaleDeliverySweep(t *testing.T) {
	newMischiefTestDB(t)
	now := time.Now().UTC()
	c := seedContract(t, "@buyer:x", "@target:x", "boss", false)
	if !claimMischiefForDelivery(c.ID) {
		t.Fatal("claim should win")
	}

	// Inside the grace period a slow delivery is left alone.
	if stale := loadStaleMischiefDeliveries(now.Add(mischiefWindow)); len(stale) != 0 {
		t.Errorf("a delivery still inside its grace window was called stranded: %d", len(stale))
	}

	stale := loadStaleMischiefDeliveries(now.Add(mischiefWindow + mischiefDeliveryGrace + time.Minute))
	if len(stale) != 1 || stale[0].ID != c.ID {
		t.Fatalf("stranded delivery not found: %d", len(stale))
	}
	if !abandonStaleMischief(c.ID) {
		t.Fatal("abandon should win on a delivering row")
	}
	if abandonStaleMischief(c.ID) {
		t.Error("abandon won twice — the buyer would be refunded twice")
	}
	// And the target is free again.
	if liveMischiefForTarget("@target:x") != nil {
		t.Error("a swept contract still blocks the target")
	}
}

// The monster is priced against the TARGET's bracket, not the dungeon they are
// standing in — that is what the M0 fee table measured.
func TestMischiefBracketZones(t *testing.T) {
	for _, tc := range []struct {
		level int
		tier  ZoneTier
	}{{1, 1}, {3, 1}, {4, 2}, {7, 2}, {8, 3}, {12, 3}, {13, 4}, {17, 4}, {18, 5}, {20, 5}} {
		zone := mischiefBracketZone(tc.level)
		if zone.Tier != tc.tier {
			t.Errorf("level %d draws from tier %d, want %d", tc.level, zone.Tier, tc.tier)
		}
	}
}

// The fight shapes the fee table was priced against. Changing any of these
// invalidates the pricing, so they are pinned.
func TestMischiefMonsterChains(t *testing.T) {
	zone := mischiefBracketZone(10) // tier 3
	rng := rand.New(rand.NewPCG(0x6d69, 1))

	for _, tc := range []struct {
		tier       string
		wantLen    int
		wantAmbush bool
	}{
		{"grunt", 1, false},
		{"mob", 3, false},
		{"elite", 1, true},
		{"boss", 1, true},
	} {
		chain, ambush := mischiefMonsters(tc.tier, zone, rng)
		if len(chain) != tc.wantLen {
			t.Errorf("%s: chain of %d, want %d", tc.tier, len(chain), tc.wantLen)
		}
		if ambush != tc.wantAmbush {
			t.Errorf("%s: ambush = %v, want %v", tc.tier, ambush, tc.wantAmbush)
		}
		for _, m := range chain {
			if m.Name == "" {
				t.Errorf("%s: chain holds an empty monster — the bestiary lookup missed", tc.tier)
			}
		}
	}

	// Boss tier sends the zone's actual boss; elite sends something flagged elite.
	if chain, _ := mischiefMonsters("boss", zone, rng); chain[0].ID != zone.Boss.BestiaryID {
		t.Errorf("boss tier sent %q, want the zone boss %q", chain[0].ID, zone.Boss.BestiaryID)
	}
	elite, _ := mischiefMonsters("elite", zone, rng)
	found := false
	for _, e := range zone.Enemies {
		if e.BestiaryID == elite[0].ID && e.IsElite {
			found = true
		}
	}
	if !found {
		t.Errorf("elite tier sent %q, which isn't an elite in %s", elite[0].ID, zone.ID)
	}
}

// ── End-to-end: the delivery path against a real character ───────────────────

// seedMischiefTarget builds a real, playable adventurer on a live expedition —
// the thing a contract actually lands on. Unit tests on the contract row prove
// nothing about the fight; these drive runMischiefInterrupt and the close-outs.
func seedMischiefTarget(t *testing.T, uid id.UserID, level, hp int) *Expedition {
	t.Helper()
	if err := createAdvCharacter(uid, "target"+uid.Localpart()); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 10, CHA: 10,
		HPMax: hp, HPCurrent: hp, ArmorClass: 18,
	}); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, run_id, status, start_date, coins_earned)
		VALUES (?, ?, 'goblin_warrens', ?, 'active', ?, 500)`,
		"exp-"+uid.Localpart(), string(uid), run.RunID, time.Now().UTC()); err != nil {
		t.Fatalf("seed expedition: %v", err)
	}
	exp, err := getActiveExpedition(uid)
	if err != nil || exp == nil {
		t.Fatalf("getActiveExpedition: %v", err)
	}
	return exp
}

// A grunt against a healthy level-5 fighter: the fight actually runs, it costs
// HP, and they live. This is the "mostly theatre" tier doing what it was priced
// to do.
func TestMischiefDelivery_GruntIsSurvivable(t *testing.T) {
	newMischiefTestDB(t)
	p := &AdventurePlugin{}
	uid := id.UserID("@grunted:example.org")
	exp := seedMischiefTarget(t, uid, 5, 60)

	bracket := mischiefBracketZone(5)
	chain, ambush := mischiefMonsters("grunt", bracket, rand.New(rand.NewPCG(1, 2)))

	_, monster, survived := p.runMischiefInterrupt(uid, exp, bracket, chain, ambush)
	if !survived {
		t.Fatalf("a level-5 fighter at full HP died to a grunt (%s) — the tier is mispriced", monster)
	}
	hp, _ := dndHPSnapshot(uid)
	if hp <= 0 {
		t.Errorf("survivor is at %d HP", hp)
	}
}

// The boss tier is the dangerous one, and the one where the no-perma-death rule
// has to hold. A level-3 caster-grade sheet against a tier-1 boss goes down —
// and must come back up at 1 HP with the expedition force-extracted, never dead.
//
// This is the property that separates mischief from paid murder: it is bought,
// and it lands while the victim may well be asleep.
func TestMischiefDelivery_DownedTargetIsMaimedNotKilled(t *testing.T) {
	newMischiefTestDB(t)
	p := &AdventurePlugin{euro: NewEuroPlugin(nil)}
	uid := id.UserID("@doomed:example.org")
	// 1 HP: whatever the dice do, this fight ends with them on the floor.
	exp := seedMischiefTarget(t, uid, 3, 1)

	c := seedContract(t, "@buyer:example.org", uid, "boss", false)
	if !claimMischiefForDelivery(c.ID) {
		t.Fatal("claim should win")
	}
	if err := p.deliverMischief(c, exp); err != nil {
		t.Fatalf("deliverMischief: %v", err)
	}

	// Alive, floored, and carried home.
	char, err := loadAdvCharacter(uid)
	if err != nil || char == nil {
		t.Fatal("target's character is gone")
	}
	if !char.Alive {
		t.Fatal("a PURCHASED monster killed a player outright — mischief must maim, never murder")
	}
	if hp, _ := dndHPSnapshot(uid); hp != 1 {
		t.Errorf("downed target is at %d HP, want the 1 HP floor", hp)
	}
	if e, _ := getActiveExpedition(uid); e != nil {
		t.Error("the expedition survived a downing — it must be force-extracted")
	}

	// The buyer gets nothing back; the whole fee is a sink.
	got, err := mischiefContractByID(c.ID)
	if err != nil || got == nil {
		t.Fatal("contract row vanished")
	}
	if got.Status != mischiefStatusDelivered || got.Outcome != mischiefOutcomeDowned {
		t.Errorf("contract resolved as %s/%s, want delivered/downed", got.Status, got.Outcome)
	}
	if pot := communityPotBalance(); pot < c.Paid {
		t.Errorf("community pot holds %d, want at least the %d fee — a landed hit refunds nobody", pot, c.Paid)
	}
	if bal := p.euro.GetBalance(c.BuyerID); bal > 0 {
		t.Errorf("buyer was credited %.0f on a successful hit; glory only", bal)
	}
}

// Survival pays the target out of the buyer's money, and never more than the
// buyer spent. Driven through the real close-out, not the arithmetic helper.
func TestMischiefDelivery_SurvivalPaysTheTarget(t *testing.T) {
	newMischiefTestDB(t)
	p := &AdventurePlugin{euro: NewEuroPlugin(nil)}
	uid := id.UserID("@tough:example.org")
	exp := seedMischiefTarget(t, uid, 12, 400) // comfortably survives a tier-1 grunt

	c := seedContract(t, "@buyer:example.org", uid, "grunt", false)
	if !claimMischiefForDelivery(c.ID) {
		t.Fatal("claim should win")
	}
	before := p.euro.GetBalance(uid)
	if err := p.deliverMischief(c, exp); err != nil {
		t.Fatalf("deliverMischief: %v", err)
	}

	got, _ := mischiefContractByID(c.ID)
	if got.Outcome != mischiefOutcomeSurvived {
		t.Fatalf("a level-12 fighter with 400 HP lost to a tier-1 grunt (outcome %s)", got.Outcome)
	}
	tier, _ := mischiefTierByKey("grunt")
	purse := mischiefPurse(tier.Fee, tier.PayoutPct)
	if gained := int(p.euro.GetBalance(uid) - before); gained != purse {
		t.Errorf("survivor was paid %d, want the %d purse", gained, purse)
	}
	if purse >= c.Paid {
		t.Errorf("purse %d >= outlay %d", purse, c.Paid)
	}
	// The remainder is a sink, not a rebate.
	if pot := communityPotBalance(); pot != c.Paid-purse {
		t.Errorf("pot holds %d, want the %d remainder", pot, c.Paid-purse)
	}
	// The expedition is untouched — they fought it off and walk on.
	if e, _ := getActiveExpedition(uid); e == nil {
		t.Error("surviving a contract ended the expedition")
	}
}

// An expedition that ends before the window closes fizzles: the buyer is
// refunded 90%, the town rakes the rest, and nobody is attacked.
func TestMischiefDelivery_FizzlesWhenTargetWentHome(t *testing.T) {
	newMischiefTestDB(t)
	p := &AdventurePlugin{euro: NewEuroPlugin(nil)}
	uid := id.UserID("@gone:example.org")
	seedMischiefTarget(t, uid, 5, 60)

	c := seedContract(t, "@buyer:example.org", uid, "elite", false)
	// They extract before the monster arrives.
	if _, _, err := forcedExtractExpedition("exp-"+uid.Localpart(), "went home"); err != nil {
		t.Fatalf("forcedExtractExpedition: %v", err)
	}

	p.fireMischiefDeliveries(time.Now().UTC().Add(mischiefWindow + time.Minute))

	got, _ := mischiefContractByID(c.ID)
	if got.Status != mischiefStatusFizzled {
		t.Fatalf("contract status %s, want fizzled — the dungeon was empty", got.Status)
	}
	refund := int(float64(c.Paid) * mischiefFizzleRefund)
	if bal := int(p.euro.GetBalance(c.BuyerID)); bal != refund {
		t.Errorf("buyer refunded %d, want %d (90%%)", bal, refund)
	}
	if pot := communityPotBalance(); pot != c.Paid-refund {
		t.Errorf("pot raked %d, want %d", pot, c.Paid-refund)
	}
	if hp, max := dndHPSnapshot(uid); hp != max {
		t.Errorf("a fizzled contract still hurt the target (%d/%d HP)", hp, max)
	}
}

// The limbo state. A leader can win a fight their friend went down in — and a
// mischief delivery deliberately skips the close-outs that would normally mark a
// 0-HP seat dead. So the seat must be floored on BOTH outcomes, or the member is
// left alive at 0 HP: not dead enough for the hospital, too dead to act, and
// read as broken by every gate that tests `HPCurrent <= 0`.
func TestMischiefDelivery_SurvivalFloorsADroppedPartyMember(t *testing.T) {
	newMischiefTestDB(t)
	p := &AdventurePlugin{euro: NewEuroPlugin(nil)}
	leader := id.UserID("@leader:example.org")
	member := id.UserID("@member:example.org")

	exp := seedMischiefTarget(t, leader, 12, 400) // wins comfortably
	seatLeaderFixture(t, exp.ID)
	if err := createAdvCharacter(member, "member"); err != nil {
		t.Fatal(err)
	}
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: member, Race: RaceHuman, Class: ClassFighter, Level: 3,
		STR: 14, DEX: 12, CON: 12, INT: 10, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 12,
	}); err != nil {
		t.Fatal(err)
	}
	if err := joinParty(exp.ID, member); err != nil {
		t.Fatalf("joinParty: %v", err)
	}
	// Put the member on the floor the way the fight would.
	if _, err := db.Get().Exec(
		`UPDATE dnd_character SET hp_current = 0 WHERE user_id = ?`, string(member)); err != nil {
		t.Fatal(err)
	}

	c := seedContract(t, "@buyer:example.org", leader, "grunt", false)
	if !claimMischiefForDelivery(c.ID) {
		t.Fatal("claim should win")
	}
	if err := p.deliverMischief(c, exp); err != nil {
		t.Fatalf("deliverMischief: %v", err)
	}

	got, _ := mischiefContractByID(c.ID)
	if got.Outcome != mischiefOutcomeSurvived {
		t.Fatalf("leader lost; this test needs a survival (outcome %s)", got.Outcome)
	}
	hp, _ := dndHPSnapshot(member)
	if hp <= 0 {
		t.Errorf("dropped party member left at %d HP on a survived contract — alive, but stuck at zero", hp)
	}
	if mc, _ := loadAdvCharacter(member); mc == nil || !mc.Alive {
		t.Error("a bought monster killed a party member — mischief maims, it never murders")
	}
}
