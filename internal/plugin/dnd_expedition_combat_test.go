package plugin

import (
	"testing"
)

// ── §4.2 Combat Interrupt brackets ─────────────────────────────────────────

func TestResolveCombatInterrupt_Brackets(t *testing.T) {
	cases := []struct {
		name   string
		roll   int
		threat int
		tier   int
		class  DnDClass
		zone   ZoneID
		want   CombatInterruptKind
	}{
		{"none", 5, 0, 1, ClassFighter, ZoneGoblinWarrens, InterruptNone}, // 5+1=6
		{"noise lower", 9, 0, 0, ClassFighter, ZoneGoblinWarrens, InterruptNoise},
		{"noise upper", 13, 0, 1, ClassFighter, ZoneGoblinWarrens, InterruptNoise},       // 13+1=14
		{"standard", 14, 0, 1, ClassFighter, ZoneGoblinWarrens, InterruptStandard},       // 15
		{"standard upper", 17, 0, 1, ClassFighter, ZoneGoblinWarrens, InterruptStandard}, // 18
		// Phase 5-B: elite bracket moved from 19+ to 23+. At totals
		// 19-21 (formerly Elite) the roll now resolves to Standard;
		// 22 still resolves to Patrol; 23+ to Elite (only reachable
		// via threat-mod, since base d20+tier+ranger maxes at ~21).
		{"standard top of band", 18, 0, 1, ClassFighter, ZoneGoblinWarrens, InterruptStandard}, // 19 → Standard now
		{"standard band upper", 18, 0, 3, ClassFighter, ZoneGoblinWarrens, InterruptStandard},  // 21 → Standard now
		{"patrol", 20, 0, 2, ClassFighter, ZoneGoblinWarrens, InterruptPatrol},                 // 22
		{"elite high threat", 18, 80, 3, ClassFighter, ZoneGoblinWarrens, InterruptElite},      // 18+3+2=23 → Elite
		{"threat bumps band", 13, 60, 1, ClassFighter, ZoneGoblinWarrens, InterruptStandard},
		{"ranger wilderness drops band", 17, 0, 1, ClassRanger, ZoneForestShadows, InterruptStandard}, // 17+1-3=15
		{"ranger non-wild no drop", 17, 0, 1, ClassRanger, ZoneGoblinWarrens, InterruptStandard},      // 17+1=18 (Standard)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := resolveCombatInterrupt(c.threat, c.tier, c.class, c.zone, func() int { return c.roll })
			if got != c.want {
				t.Errorf("kind = %v, want %v", got, c.want)
			}
		})
	}
}

func TestResolveCombatInterrupt_ThreatModifier(t *testing.T) {
	// At threat 80, +1 mod beyond tier (80-40)/20 = +2. Roll 13 + tier 1 + 2 = 16 → Standard.
	got, total := resolveCombatInterrupt(80, 1, ClassFighter, ZoneGoblinWarrens, func() int { return 13 })
	if got != InterruptStandard {
		t.Errorf("threat 80, tier 1, roll 13 → %v (total %d), want Standard", got, total)
	}
	if total != 16 {
		t.Errorf("total = %d, want 16", total)
	}
}

// ── Kill-log writer ────────────────────────────────────────────────────────

func TestMonsterKillTags_GatesKnownMonsters(t *testing.T) {
	cases := map[string][]string{
		"worg":             {"worg", "beast"},
		"owlbear":          {"owlbear", "beast"},
		"dire_wolf":        {"beast"},
		"vampire_spawn":    {"vampire_spawn"},
		"young_red_dragon": {"drake"},
		"boss_belaxath":    {"balor", "demon", "portal_boss"},
		"boss_thornmother": {"thornmother"},
		"goblin_sneak":     nil, // goblins gate nothing in §3
	}
	for id, want := range cases {
		got := monsterKillTags(id)
		if len(got) != len(want) {
			t.Errorf("%s tags = %v, want %v", id, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s tags[%d] = %s, want %s", id, i, got[i], want[i])
			}
		}
	}
}

func TestRecordZoneKill_AppendsAndDedupes(t *testing.T) {
	exp := &Expedition{
		CurrentRegion: "warrens",
		RegionState:   map[string]any{},
	}
	// Stub out persist by re-pointing at a no-op via the expedition's
	// in-memory state only — recordZoneKill calls persistRegionState which
	// requires a DB. Bypass that path by writing to RegionState directly
	// using the same helper logic.
	if err := recordZoneKillInMemory(exp, "owlbear"); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := recordZoneKillInMemory(exp, "owlbear"); err != nil {
		t.Fatalf("second record: %v", err)
	}
	if err := recordZoneKillInMemory(exp, "dire_wolf"); err != nil {
		t.Fatalf("third record: %v", err)
	}
	tags := loadKillsTable(exp)["warrens"]
	// Expect: owlbear, beast, dire_wolf was beast→already there, so no-op.
	want := []string{"owlbear", "beast"}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
	for i, w := range want {
		if tags[i] != w {
			t.Errorf("tags[%d] = %s, want %s", i, tags[i], w)
		}
	}
}

// recordZoneKillInMemory is a test-only mirror of recordZoneKill that
// skips the DB persist step. Validates the in-memory tag-merge logic;
// the persistence path is exercised by the existing harvest round-trip tests.
func recordZoneKillInMemory(exp *Expedition, bestiaryID string) error {
	tags := monsterKillTags(bestiaryID)
	if len(tags) == 0 {
		return nil
	}
	if exp.RegionState == nil {
		exp.RegionState = map[string]any{}
	}
	table := loadKillsTable(exp)
	regionKey := regionHarvestKey(exp)
	existing := table[regionKey]
	for _, t := range tags {
		if !stringSliceContains(existing, t) {
			existing = append(existing, t)
		}
	}
	table[regionKey] = existing
	exp.RegionState["kills"] = table
	return nil
}

// ── Patrol scaling ─────────────────────────────────────────────────────────

func TestRollPatrolChance_BelowAlertIsZero(t *testing.T) {
	if c := rollPatrolChance(40); c != 0 {
		t.Errorf("threat 40 → %.2f, want 0", c)
	}
	if c := rollPatrolChance(0); c != 0 {
		t.Errorf("threat 0 → %.2f, want 0", c)
	}
}

func TestRollPatrolChance_ScalesWithThreat(t *testing.T) {
	c41 := rollPatrolChance(41)
	c100 := rollPatrolChance(100)
	if c41 <= 0 || c41 > 0.15 {
		t.Errorf("at 41 chance %.3f outside [0,0.15]", c41)
	}
	if c100 <= c41 {
		t.Errorf("chance at 100 (%.3f) should exceed at 41 (%.3f)", c100, c41)
	}
	if c100 > 0.32 {
		t.Errorf("chance at 100 (%.3f) higher than expected ceiling", c100)
	}
}
