package plugin

import (
	"strings"
	"testing"
)

// ── Plug 1: HP scaling ──────────────────────────────────────────────────────

func TestApplyDnDHPScaling_FullHP(t *testing.T) {
	stats := CombatStats{MaxHP: 100}
	c := &DnDCharacter{HPMax: 50, HPCurrent: 50}
	applyDnDHPScaling(&stats, c)
	if stats.MaxHP != 100 {
		t.Errorf("full HP: MaxHP scaled to %d, want unchanged 100", stats.MaxHP)
	}
}

func TestApplyDnDHPScaling_HalfHP(t *testing.T) {
	stats := CombatStats{MaxHP: 100}
	c := &DnDCharacter{HPMax: 50, HPCurrent: 25}
	applyDnDHPScaling(&stats, c)
	if stats.MaxHP != 50 {
		t.Errorf("50%% HP: MaxHP = %d, want 50", stats.MaxHP)
	}
}

func TestApplyDnDHPScaling_FloorAt25Pct(t *testing.T) {
	stats := CombatStats{MaxHP: 100}
	c := &DnDCharacter{HPMax: 50, HPCurrent: 0}
	applyDnDHPScaling(&stats, c)
	if stats.MaxHP != 25 {
		t.Errorf("0%% HP: MaxHP = %d, want 25 (floor)", stats.MaxHP)
	}
}

func TestApplyDnDHPScaling_NoOpIfNilOrZero(t *testing.T) {
	stats := CombatStats{MaxHP: 100}
	applyDnDHPScaling(&stats, nil)
	if stats.MaxHP != 100 {
		t.Errorf("nil char: scaled to %d, want unchanged", stats.MaxHP)
	}
	c := &DnDCharacter{HPMax: 0}
	applyDnDHPScaling(&stats, c)
	if stats.MaxHP != 100 {
		t.Errorf("zero HPMax: scaled to %d, want unchanged", stats.MaxHP)
	}
}

// ── Plug 2: roll summary ────────────────────────────────────────────────────

func TestDnDRollSummaryLine_Empty(t *testing.T) {
	r := CombatResult{Events: []CombatEvent{
		{Actor: "player", Action: "hit", Roll: 0}, // no Roll → not D&D
	}}
	if got := dndRollSummaryLine(r); got != "" {
		t.Errorf("expected empty summary; got %q", got)
	}
}

func TestDnDRollSummaryLine_Basic(t *testing.T) {
	r := CombatResult{Events: []CombatEvent{
		{Actor: "player", Action: "hit", Roll: 14, RollAgainst: 12},
		{Actor: "player", Action: "miss", Roll: 5, RollAgainst: 12},
		{Actor: "player", Action: "crit", Roll: 20, RollAgainst: 12},
		{Actor: "player", Action: "miss", Roll: 1, RollAgainst: 12, Desc: "fumble"},
		{Actor: "enemy", Action: "hit", Roll: 18}, // enemy ignored
	}}
	got := dndRollSummaryLine(r)
	for _, want := range []string{"d20", "2/4 hit", "1 crit", "1 fumble", "nat 20"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %s", want, got)
		}
	}
}

func TestDnDRollSummaryLine_HighestNonNat20(t *testing.T) {
	r := CombatResult{Events: []CombatEvent{
		{Actor: "player", Action: "hit", Roll: 17, RollAgainst: 14},
		{Actor: "player", Action: "miss", Roll: 8, RollAgainst: 14},
	}}
	got := dndRollSummaryLine(r)
	if !strings.Contains(got, "Best: 17") {
		t.Errorf("expected 'Best: 17' in summary: %s", got)
	}
}

// ── Plug 3: !roll dice parser ──────────────────────────────────────────────

func TestParseDice(t *testing.T) {
	cases := []struct {
		in                          string
		count, sides, mod           int
		ok                          bool
	}{
		{"d20", 1, 20, 0, true},
		{"1d20", 1, 20, 0, true},
		{"2d6+3", 2, 6, 3, true},
		{"4d6-1", 4, 6, -1, true},
		{"3D8", 3, 8, 0, true},      // case-insensitive
		{"  2d6+3 ", 2, 6, 3, true}, // whitespace
		{"d1", 0, 0, 0, false},      // sides too few
		{"100d6", 100, 6, 0, true},  // max count
		{"101d6", 0, 0, 0, false},   // over max
		{"banana", 0, 0, 0, false},
		{"d6+", 0, 0, 0, false},
		{"", 0, 0, 0, false},
		{"2d6+", 0, 0, 0, false},
	}
	for _, c := range cases {
		ct, sd, mod, ok := parseDice(c.in)
		if ok != c.ok || ct != c.count || sd != c.sides || mod != c.mod {
			t.Errorf("parseDice(%q) = (%d,%d,%d,%v); want (%d,%d,%d,%v)",
				c.in, ct, sd, mod, ok, c.count, c.sides, c.mod, c.ok)
		}
	}
}

func TestRollDice_Bounds(t *testing.T) {
	// 1000 trials: every roll must be in [1, sides].
	for i := 0; i < 1000; i++ {
		rolls, total := rollDice(4, 6, 2)
		sum := 2
		for _, r := range rolls {
			if r < 1 || r > 6 {
				t.Fatalf("roll out of range: %d", r)
			}
			sum += r
		}
		if sum != total {
			t.Fatalf("total mismatch: rolls sum=%d, total=%d", sum, total)
		}
	}
}

// ── Plug 4: !stats / !level format ─────────────────────────────────────────
// (No DB integration here — just smoke-test that the helpers don't panic
// against a constructed character. Full DB-integrated tests would be
// duplicative with existing prod-DB tests.)
