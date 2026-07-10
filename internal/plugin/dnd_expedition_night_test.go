package plugin

import (
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// Phase 12 E2b — wandering monster / night phase tests.

func TestWanderingOutcome_BucketBoundaries(t *testing.T) {
	cases := []struct {
		total int
		want  NightOutcome
	}{
		{1, NightOutcomePeaceful},
		{5, NightOutcomePeaceful},
		{6, NightOutcomePassage},
		{10, NightOutcomePassage},
		{11, NightOutcomeMinor},
		{14, NightOutcomeMinor},
		{15, NightOutcomeStandard},
		{17, NightOutcomeStandard},
		{18, NightOutcomeElite},
		{19, NightOutcomeElite},
		{20, NightOutcomeAmbush},
		{25, NightOutcomeAmbush},
	}
	for _, c := range cases {
		got := wanderingOutcome(c.total)
		if got != c.want {
			t.Errorf("total %d → %v, want %v", c.total, got, c.want)
		}
	}
}

func TestResolveWanderingCheck_RoughCampBumpsRoll(t *testing.T) {
	exp := &Expedition{
		ZoneID:      ZoneGoblinWarrens,
		ThreatLevel: 0,
		Camp:        &CampState{Active: true, Type: CampTypeRough},
	}
	// roll 8 → +3 rough → total 11 → Minor (would have been Passage at 8).
	nc := resolveWanderingCheck(exp, ClassFighter, func() int { return 8 })
	if nc.Outcome != NightOutcomeMinor {
		t.Errorf("rough+8 → %v, want Minor", nc.Outcome)
	}
	if nc.CampMod != 3 {
		t.Errorf("camp mod = %d, want 3", nc.CampMod)
	}
}

func TestResolveWanderingCheck_FortifiedCampReducesRoll(t *testing.T) {
	exp := &Expedition{
		ZoneID:      ZoneGoblinWarrens,
		ThreatLevel: 0,
		Camp:        &CampState{Active: true, Type: CampTypeFortified},
	}
	// roll 14 → -4 fortified → total 10 → Passage.
	nc := resolveWanderingCheck(exp, ClassFighter, func() int { return 14 })
	if nc.Outcome != NightOutcomePassage {
		t.Errorf("fortified+14 → %v, want Passage", nc.Outcome)
	}
}

func TestResolveWanderingCheck_ThreatModAbove30(t *testing.T) {
	exp := &Expedition{
		ZoneID:      ZoneGoblinWarrens,
		ThreatLevel: 60,
		Camp:        &CampState{Active: true, Type: CampTypeStandard},
	}
	// (60-30)/10 = 3 mod. roll 12 → +3 → total 15 → Standard.
	nc := resolveWanderingCheck(exp, ClassFighter, func() int { return 12 })
	if nc.ThreatMod != 3 {
		t.Errorf("threat mod = %d, want 3", nc.ThreatMod)
	}
	if nc.Outcome != NightOutcomeStandard {
		t.Errorf("outcome = %v, want Standard", nc.Outcome)
	}
}

func TestResolveWanderingCheck_RangerWildernessDiscount(t *testing.T) {
	exp := &Expedition{
		ZoneID:      ZoneForestShadows,
		ThreatLevel: 0,
		Camp:        &CampState{Active: true, Type: CampTypeStandard},
	}
	nc := resolveWanderingCheck(exp, ClassRanger, func() int { return 12 })
	if nc.ClassMod != -2 {
		t.Errorf("ranger wilderness mod = %d, want -2", nc.ClassMod)
	}
	// In a non-wilderness zone, no class mod.
	exp2 := *exp
	exp2.ZoneID = ZoneGoblinWarrens
	nc2 := resolveWanderingCheck(&exp2, ClassRanger, func() int { return 12 })
	if nc2.ClassMod != 0 {
		t.Errorf("ranger non-wilderness mod = %d, want 0", nc2.ClassMod)
	}
}

func TestResolveWanderingCheck_NoCampNoEncounter(t *testing.T) {
	exp := &Expedition{ZoneID: ZoneGoblinWarrens}
	nc := resolveWanderingCheck(exp, ClassFighter, func() int { return 20 })
	if nc.Outcome != NightOutcomePeaceful {
		t.Errorf("no camp should yield peaceful, got %v", nc.Outcome)
	}
}

func TestProcessNightCheck_LogsAndBumpsThreatOnPassage(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-night-passage:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	exp.Camp = &CampState{Active: true, Type: CampTypeStandard, EstablishedAt: time.Now().UTC()}
	if err := updateCamp(exp.ID, exp.Camp); err != nil {
		t.Fatal(err)
	}
	nc := NightCheck{
		Outcome: NightOutcomePassage, Roll: 8, Total: 8,
		Summary:      "Signs of passage near camp; no encounter.",
		ThreatBumped: true,
	}
	if err := processNightCheck(exp, nc); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.ThreatLevel != 2 {
		t.Errorf("threat after signs-of-passage = %d, want 2", got.ThreatLevel)
	}
	entries, _ := recentExpeditionLog(exp.ID, 5)
	foundNight := false
	for _, e := range entries {
		if e.Type == "night" && strings.Contains(e.Summary, "Signs of passage") {
			foundNight = true
		}
	}
	if !foundNight {
		t.Error("night log entry missing")
	}
}

func TestProcessNightCheck_PersistsCampNightEvents(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-night-campevents:example")
	defer cleanupExpeditions(uid)

	exp, _ := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	exp.Camp = &CampState{Active: true, Type: CampTypeStandard, EstablishedAt: time.Now().UTC(), NightEvents: []string{}}
	_ = updateCamp(exp.ID, exp.Camp)

	nc := NightCheck{Outcome: NightOutcomePeaceful, Roll: 3, Total: 3, Summary: "Peaceful night."}
	if err := processNightCheck(exp, nc); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.Camp == nil || len(got.Camp.NightEvents) != 1 {
		t.Errorf("camp.NightEvents not appended: %+v", got.Camp)
	}
}

func TestPickWanderingMonster_RespectsEliteFlag(t *testing.T) {
	// Run several picks; elite outcomes should never return non-elite when
	// the zone has elite entries. Goblin Warrens should have at least one
	// IsElite enemy in its roster.
	zone, _ := getZone(ZoneGoblinWarrens)
	hasElite := false
	for _, e := range zone.Enemies {
		if e.IsElite {
			hasElite = true
			break
		}
	}
	if !hasElite {
		t.Skip("goblin_warrens roster has no elite — pick test would be vacuous")
	}
	for i := 0; i < 20; i++ {
		id, _ := pickWanderingMonster(ZoneGoblinWarrens, NightOutcomeElite)
		if id == "" {
			t.Fatalf("expected an elite pick, got empty")
		}
		isElite := false
		for _, e := range zone.Enemies {
			if e.BestiaryID == id && e.IsElite {
				isElite = true
				break
			}
		}
		if !isElite {
			t.Errorf("elite outcome picked non-elite %q", id)
		}
	}
}
