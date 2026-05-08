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

// failedDetect — fabricated SkillCheckResult that always reads as a
// missed detection. Used by trap-room tests to avoid d20 flake.
func failedDetect(skill DnDSkill, dc int) SkillCheckResult {
	return SkillCheckResult{Skill: skill, DC: dc, Roll: 5, Mod: 0, Total: 5, Success: false}
}

// passedDetect — fabricated successful detection.
func passedDetect(skill DnDSkill, dc int) SkillCheckResult {
	return SkillCheckResult{Skill: skill, DC: dc, Roll: 18, Mod: 0, Total: 18, Success: true}
}

func TestZoneTrapRoom_PitDamagesAndPersists(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-trap-pit:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := getZone(ZoneGoblinWarrens)
	dndChar, _ := LoadDnDCharacter(uid)

	pit := tier1TrapCatalog[0]
	if pit.Kind != TrapPit {
		t.Fatalf("expected pit at index 0, got %s", pit.Kind)
	}

	p := &AdventurePlugin{}
	dmg, narration := p.applyTrapEffectWithDetect(uid, run, zone, dndChar, pit,
		failedDetect(pit.DetectSkill, pit.DetectDC))
	if dmg <= 0 {
		t.Errorf("pit trap dealt no damage on failed detect: %d", dmg)
	}
	if dmg > 12 {
		t.Errorf("pit trap exceeded 2d6 max: %d", dmg)
	}
	if !strings.Contains(narration, "HP") {
		t.Errorf("trap narration missing HP line: %q", narration)
	}
	c, _ := LoadDnDCharacter(uid)
	if c.HPCurrent >= c.HPMax {
		t.Errorf("trap did not persist HP loss: cur=%d max=%d", c.HPCurrent, c.HPMax)
	}
}

func TestZoneTrapRoom_DetectedSpotsHarmless(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-trap-spot:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	run, _ := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	zone, _ := getZone(ZoneGoblinWarrens)
	dndChar, _ := LoadDnDCharacter(uid)
	startHP := dndChar.HPCurrent

	pit := tier1TrapCatalog[0]
	p := &AdventurePlugin{}
	dmg, narration := p.applyTrapEffectWithDetect(uid, run, zone, dndChar, pit,
		passedDetect(pit.DetectSkill, pit.DetectDC))
	if dmg != 0 {
		t.Errorf("detected trap should deal 0 damage, got %d", dmg)
	}
	if !strings.Contains(narration, "spotted") {
		t.Errorf("detected trap narration missing 'spotted': %q", narration)
	}
	c, _ := LoadDnDCharacter(uid)
	if c.HPCurrent != startHP {
		t.Errorf("detected trap should not persist HP change: cur=%d, want=%d", c.HPCurrent, startHP)
	}
}

func TestZoneTrapRoom_TripwireZeroDamageButAlerts(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-trap-tripwire:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	run, _ := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	zone, _ := getZone(ZoneGoblinWarrens)
	dndChar, _ := LoadDnDCharacter(uid)
	startHP := dndChar.HPCurrent

	var tripwire zoneTrapDef
	for _, tr := range tier1TrapCatalog {
		if tr.Kind == TrapTripwireAlarm {
			tripwire = tr
			break
		}
	}
	if tripwire.Kind != TrapTripwireAlarm {
		t.Fatal("tripwire not present in catalog")
	}

	p := &AdventurePlugin{}
	dmg, narration := p.applyTrapEffectWithDetect(uid, run, zone, dndChar, tripwire,
		failedDetect(tripwire.DetectSkill, tripwire.DetectDC))
	if dmg != 0 {
		t.Errorf("tripwire should deal 0 damage even on fail, got %d", dmg)
	}
	if !strings.Contains(narration, "Tripwire") {
		t.Errorf("tripwire narration missing trap name: %q", narration)
	}
	c, _ := LoadDnDCharacter(uid)
	if c.HPCurrent != startHP {
		t.Errorf("tripwire should not change HP: cur=%d, want=%d", c.HPCurrent, startHP)
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
	dndChar, _ := LoadDnDCharacter(uid)

	pit := tier1TrapCatalog[0]
	p := &AdventurePlugin{}
	_, _ = p.applyTrapEffectWithDetect(uid, run, zone, dndChar, pit,
		failedDetect(pit.DetectSkill, pit.DetectDC))
	c, _ = LoadDnDCharacter(uid)
	if c.HPCurrent < 1 {
		t.Errorf("trap KO'd a player below 1 HP: %d", c.HPCurrent)
	}
}

func TestPickZoneTrap_DeterministicAndTier1Only(t *testing.T) {
	zone, _ := getZone(ZoneGoblinWarrens)
	a, ok := pickZoneTrap(zone, "fixed-run", 3)
	if !ok {
		t.Fatal("pickZoneTrap returned !ok for tier 1 zone")
	}
	b, _ := pickZoneTrap(zone, "fixed-run", 3)
	if a.Kind != b.Kind {
		t.Errorf("pickZoneTrap not deterministic: %s vs %s", a.Kind, b.Kind)
	}
	if a.Tier != ZoneTierBeginner {
		t.Errorf("tier 1 zone picked tier %d trap", a.Tier)
	}
}

func TestPickZoneTrap_Tier2UsesTier2Catalog(t *testing.T) {
	zone, ok := getZone(ZoneForestShadows)
	if !ok {
		t.Fatal("forest_shadows zone missing")
	}
	if zone.Tier != ZoneTierApprentice {
		t.Fatalf("forest_shadows expected tier 2, got %d", zone.Tier)
	}
	seen := map[ZoneTrapKind]bool{}
	for room := 0; room < 64; room++ {
		tr, ok := pickZoneTrap(zone, "t2-run", room)
		if !ok {
			t.Fatalf("pickZoneTrap !ok at room %d", room)
		}
		if tr.Tier != ZoneTierApprentice {
			t.Errorf("tier 2 zone picked tier %d trap (%s)", tr.Tier, tr.Kind)
		}
		seen[tr.Kind] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected variety across 64 rooms in tier 2, only saw %d kinds: %v", len(seen), seen)
	}
}

func TestRollTrapDamage_GlyphOfWardingIn5d8Range(t *testing.T) {
	var glyph zoneTrapDef
	for _, tr := range tier2TrapCatalog {
		if tr.Kind == TrapGlyphOfWarding {
			glyph = tr
			break
		}
	}
	if glyph.Kind != TrapGlyphOfWarding {
		t.Fatal("glyph of warding not in tier 2 catalog")
	}
	for room := 0; room < 64; room++ {
		dmg := rollTrapDamage(glyph, "glyph-run", room)
		if dmg < 5 || dmg > 40 {
			t.Errorf("glyph damage out of 5..40 range at room %d: %d", room, dmg)
		}
	}
}

func TestTier2TrapCatalog_AllValid(t *testing.T) {
	if len(tier2TrapCatalog) < 3 {
		t.Fatalf("tier 2 catalog should have at least 3 traps, got %d", len(tier2TrapCatalog))
	}
	for _, tr := range tier2TrapCatalog {
		if tr.Tier != ZoneTierApprentice {
			t.Errorf("trap %s tagged tier %d, expected 2", tr.Kind, tr.Tier)
		}
		if tr.Display == "" || tr.Trigger == "" {
			t.Errorf("trap %s missing display/trigger text", tr.Kind)
		}
		if !tr.AlertsEnemies && (tr.DamageDiceN == 0 || tr.DamageDiceD == 0) {
			t.Errorf("trap %s deals no damage and is not an alarm", tr.Kind)
		}
		if tr.DetectDC <= 12 {
			t.Errorf("trap %s detect DC %d looks tier 1; tier 2 should be 13+", tr.Kind, tr.DetectDC)
		}
	}
}

func TestPickZoneTrap_VariesAcrossRooms(t *testing.T) {
	zone, _ := getZone(ZoneGoblinWarrens)
	seen := map[ZoneTrapKind]bool{}
	for room := 0; room < 64; room++ {
		tr, ok := pickZoneTrap(zone, "varied-run", room)
		if !ok {
			t.Fatalf("pickZoneTrap !ok at room %d", room)
		}
		seen[tr.Kind] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected variety across 64 rooms, only saw %d kinds: %v", len(seen), seen)
	}
}

func TestRollTrapDamage_PoisonDartIn2d4Range(t *testing.T) {
	var dart zoneTrapDef
	for _, tr := range tier1TrapCatalog {
		if tr.Kind == TrapPoisonDart {
			dart = tr
			break
		}
	}
	if dart.Kind != TrapPoisonDart {
		t.Fatal("poison dart not in catalog")
	}
	for room := 0; room < 64; room++ {
		dmg := rollTrapDamage(dart, "dart-run", room)
		if dmg < 2 || dmg > 8 { // 2d4 range
			t.Errorf("poison dart damage out of 2..8 range at room %d: %d", room, dmg)
		}
	}
}

func TestRollTrapDamage_TripwireZero(t *testing.T) {
	var tripwire zoneTrapDef
	for _, tr := range tier1TrapCatalog {
		if tr.Kind == TrapTripwireAlarm {
			tripwire = tr
			break
		}
	}
	if dmg := rollTrapDamage(tripwire, "any-run", 0); dmg != 0 {
		t.Errorf("tripwire damage should be 0, got %d", dmg)
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
