package plugin

import (
	"hash/fnv"
	"math/rand/v2"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func newDeterministicRNG(t *testing.T, seed string) *rand.Rand {
	t.Helper()
	h := fnv.New64a()
	h.Write([]byte(seed))
	return rand.New(rand.NewPCG(h.Sum64(), 0xC0FFEE))
}

// Phase 11 D1e — combat / trap / boss resolution tests.
//
// These exercise the pure helpers in dnd_zone_combat.go (deterministic
// enemy pick, coin-pattern parsing, loot rolling) and the integrated
// !zone advance path (trap nick + boss combat with a beefed-up char).

func TestPickZoneEnemy_NonElite_RespectsRoster(t *testing.T) {
	zone, _ := getZone(ZoneGoblinWarrens)
	seen := map[string]bool{}
	for room := 0; room < 32; room++ {
		m, ok := pickZoneEnemy(zone, "deterministic-run", room, false)
		if !ok {
			t.Fatalf("pickZoneEnemy returned !ok at room %d", room)
		}
		// Non-elite picks must never include elite-flagged entries.
		for _, e := range zone.Enemies {
			if e.IsElite && e.BestiaryID == m.ID {
				t.Errorf("non-elite pick returned elite roster entry: %s", m.ID)
			}
		}
		seen[m.ID] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected variety across 32 rooms, only saw %d unique enemies: %v", len(seen), seen)
	}
}

func TestPickZoneEnemy_Elite_FiltersToEliteRoster(t *testing.T) {
	zone, _ := getZone(ZoneGoblinWarrens)
	for room := 0; room < 16; room++ {
		m, ok := pickZoneEnemy(zone, "elite-run", room, true)
		if !ok {
			t.Fatalf("elite pickZoneEnemy returned !ok at room %d", room)
		}
		// Confirm the picked monster's ID is one of the roster's elite-flagged entries.
		isElite := false
		for _, e := range zone.Enemies {
			if e.BestiaryID == m.ID && e.IsElite {
				isElite = true
				break
			}
		}
		if !isElite {
			t.Errorf("elite pick picked non-elite roster entry: %s", m.ID)
		}
	}
}

func TestPickZoneEnemy_Deterministic(t *testing.T) {
	zone, _ := getZone(ZoneCryptValdris)
	a, _ := pickZoneEnemy(zone, "stable", 3, false)
	b, _ := pickZoneEnemy(zone, "stable", 3, false)
	if a.ID != b.ID {
		t.Errorf("deterministic pick diverged: %s vs %s", a.ID, b.ID)
	}
}

func TestRollCoinPattern_Parses(t *testing.T) {
	rng := newDeterministicRNG(t, "coins")
	v := rollCoinPattern("coins_2d10x5", rng)
	// 2d10×5: min 10, max 100.
	if v < 10 || v > 100 {
		t.Errorf("2d10x5 = %d, out of [10,100]", v)
	}
}

func TestRollCoinPattern_BadInputFallsBack(t *testing.T) {
	rng := newDeterministicRNG(t, "bad-coins")
	if got := rollCoinPattern("coins_garbage", rng); got != 25 {
		t.Errorf("bad coin pattern fallback = %d, want 25", got)
	}
}

func TestZoneCombatXP_LossFraction(t *testing.T) {
	loss := zoneCombatXP(CombatResult{PlayerWon: false}, 1.0, 1)
	win := zoneCombatXP(CombatResult{PlayerWon: true}, 1.0, 1)
	if loss >= win {
		t.Errorf("loss (%d) should be < win (%d)", loss, win)
	}
}

func TestZoneCombatXP_NearDeathBonus(t *testing.T) {
	plain := zoneCombatXP(CombatResult{PlayerWon: true}, 1.0, 1)
	nd := zoneCombatXP(CombatResult{PlayerWon: true, NearDeath: true}, 1.0, 1)
	if nd <= plain {
		t.Errorf("near-death (%d) should beat plain win (%d)", nd, plain)
	}
}

func TestZoneCombatXP_TierFloor(t *testing.T) {
	// CR 0.25 at T5 should hit the tier floor, not 25 raw.
	got := zoneCombatXP(CombatResult{PlayerWon: true}, 0.25, 5)
	if got <= int(0.25*100) {
		t.Errorf("expected tier floor to lift CR 0.25 at T5 above 25, got %d", got)
	}
}

func TestTitleCaseUnderscored(t *testing.T) {
	cases := map[string]string{
		"wpn_handaxe_+1":     "Wpn Handaxe +1",
		"valdris_phylactery": "Valdris Phylactery",
		"":                   "",
		"single":             "Single",
	}
	for in, want := range cases {
		if got := titleCaseUnderscored(in); got != want {
			t.Errorf("titleCaseUnderscored(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestZoneTrapRoom_DamagesAndPersists(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-trap:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := getZone(ZoneGoblinWarrens)

	p := &AdventurePlugin{}
	dmg, narration := p.resolveTrapRoom(uid, run, zone)
	if dmg <= 0 {
		t.Errorf("trap dealt no damage: %d", dmg)
	}
	if !strings.Contains(narration, "HP") {
		t.Errorf("trap narration missing HP line: %q", narration)
	}
	c, _ := LoadDnDCharacter(uid)
	if c.HPCurrent >= c.HPMax {
		t.Errorf("trap did not persist HP loss: cur=%d max=%d", c.HPCurrent, c.HPMax)
	}
}

func TestZoneTrapRoom_DoesNotKO(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-trap-low-hp:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	c, _ := LoadDnDCharacter(uid)
	c.HPCurrent = 2
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	run, _ := startZoneRun(uid, ZoneCryptValdris, 1, nil)
	zone, _ := getZone(ZoneCryptValdris)

	p := &AdventurePlugin{}
	_, _ = p.resolveTrapRoom(uid, run, zone)
	c, _ = LoadDnDCharacter(uid)
	if c.HPCurrent < 1 {
		t.Errorf("trap KO'd a player below 1 HP: %d", c.HPCurrent)
	}
}

func TestRollZoneLoot_BossLootDrops(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-loot:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	run, _ := startZoneRun(uid, ZoneCryptValdris, 1, nil)
	zone, _ := getZone(ZoneCryptValdris)

	p := &AdventurePlugin{}
	granted := p.rollZoneLoot(uid, run, zone)
	// Crypt of Valdris has UniqueAlways "valdris_phylactery_shard" + always-drop coins.
	foundShard := false
	foundCoins := false
	for _, id := range granted {
		if id == "valdris_phylactery_shard" {
			foundShard = true
		}
		if strings.HasPrefix(id, "coins_") {
			foundCoins = true
		}
	}
	if !foundShard {
		t.Errorf("UniqueAlways quest item missing from drops: %v", granted)
	}
	if !foundCoins {
		t.Errorf("100%%-drop coins missing from drops: %v", granted)
	}
}
