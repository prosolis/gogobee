package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase L2 step 4 — smoke test for resolveArenaBoss. Asserts the helper
// returns staged narration (intro + phases + outcome) for a representative
// arena round and that the outcome carries the boss-flow signatures: the
// arena-styled headline and a dice-roll summary.

func TestResolveArenaBoss_T1Round1_Smoke(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@arena-bossflow:example")
	t.Cleanup(func() { cleanupZoneRuns(uid) })

	if err := createAdvCharacter(uid, "bossflow"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 10, CHA: 10,
		HPMax: 60, HPCurrent: 60, ArmorClass: 18,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}

	p := &AdventurePlugin{}
	intro, phases, outcome, result, err := p.resolveArenaBoss(uid, ArenaBossEncounter{
		Tier: 1, Round: 1, DisplayName: "Smoke",
	})
	if err != nil {
		t.Fatalf("resolveArenaBoss: %v", err)
	}
	if intro == "" {
		t.Error("intro empty")
	}
	if !strings.Contains(intro, "Arena T1 R1") {
		t.Errorf("intro missing tier/round label: %q", intro)
	}
	if len(phases) == 0 {
		t.Error("phases empty — combat log did not render")
	}
	if outcome == "" {
		t.Error("outcome empty")
	}
	if result.PlayerWon {
		// On a win the headline mentions "falls"; on a loss "stands over".
		if !strings.Contains(outcome, "falls") {
			t.Errorf("win outcome missing 'falls' headline: %q", outcome)
		}
	} else {
		if !strings.Contains(outcome, "stands over") {
			t.Errorf("loss outcome missing defeat headline: %q", outcome)
		}
	}
}

func TestResolveArenaBoss_BadTierRound(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@arena-bossflow-bad:example")
	if err := createAdvCharacter(uid, "bad"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	p := &AdventurePlugin{}
	if _, _, _, _, err := p.resolveArenaBoss(uid, ArenaBossEncounter{Tier: 9, Round: 9}); err == nil {
		t.Error("expected error for unknown tier/round")
	}
}

func TestArenaBossPhaseTwoAt(t *testing.T) {
	cases := []struct {
		tier int
		want float64
	}{{1, 0}, {2, 0}, {3, 0.5}, {4, 0.5}, {5, 0.5}}
	for _, c := range cases {
		if got := arenaBossPhaseTwoAt(c.tier); got != c.want {
			t.Errorf("arenaBossPhaseTwoAt(%d)=%v want %v", c.tier, got, c.want)
		}
	}
}

// Phase L2 step 4b — flag gate. Empty / "0" / "false" disable the new
// boss-flow path; anything else (including "1") enables it.
func TestArenaBossFlowEnabled(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"False", false},
		{"1", true},
		{"true", true},
		{"on", true},
	}
	for _, c := range cases {
		t.Setenv("ARENA_BOSS_FLOW", c.val)
		if got := arenaBossFlowEnabled(); got != c.want {
			t.Errorf("arenaBossFlowEnabled with %q = %v want %v", c.val, got, c.want)
		}
	}
}

// Phase L2 step 4b — staged-narration assembly. The intro line leads,
// followed by the combat-log phases, mirroring streamOrSend's
// intro+phases pattern in dnd_zone_cmd.go.
func TestBossFlowPhaseMessages(t *testing.T) {
	n := &arenaBossNarration{
		intro:  "🏟️ **Arena T1 R1 — Slug** (HP 12, AC 10)",
		phases: []string{"phase A", "phase B"},
	}
	got := bossFlowPhaseMessages(n)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (intro + 2 phases): %v", len(got), got)
	}
	if got[0] != n.intro {
		t.Errorf("got[0] = %q, want intro %q", got[0], n.intro)
	}
	if got[1] != "phase A" || got[2] != "phase B" {
		t.Errorf("phases out of order: %v", got)
	}

	// No intro → just the phases, unchanged.
	n2 := &arenaBossNarration{phases: []string{"only"}}
	got2 := bossFlowPhaseMessages(n2)
	if len(got2) != 1 || got2[0] != "only" {
		t.Errorf("no-intro case: got %v want [only]", got2)
	}
}

func TestArenaBosses_AllTiersPopulated(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		for round := 1; round <= 4; round++ {
			id := arenaBossID(tier, round)
			m, ok := arenaBosses[id]
			if !ok {
				t.Errorf("arenaBosses missing %s", id)
				continue
			}
			if m.HP <= 0 || m.AC <= 0 || m.Attack <= 0 {
				t.Errorf("%s has bad stats: HP=%d AC=%d ATK=%d", id, m.HP, m.AC, m.Attack)
			}
		}
	}
}
