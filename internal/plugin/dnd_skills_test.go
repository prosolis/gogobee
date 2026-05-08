package plugin

import (
	"testing"
)

func TestSkillTableComplete(t *testing.T) {
	expectedStats := map[DnDSkill]string{
		SkillAthletics: "str", SkillAcrobatics: "dex", SkillStealth: "dex",
		SkillSleightOfHand: "dex",
		SkillArcana:        "int", SkillInvestigation: "int",
		SkillPerception: "wis", SkillInsight: "wis",
		SkillPersuasion: "cha", SkillIntimidation: "cha", SkillDeception: "cha",
	}
	if len(dndSkillTable) != len(expectedStats) {
		t.Errorf("dndSkillTable size = %d, want %d", len(dndSkillTable), len(expectedStats))
	}
	for _, si := range dndSkillTable {
		want, ok := expectedStats[si.Key]
		if !ok {
			t.Errorf("unexpected skill %s", si.Key)
			continue
		}
		if si.Stat != want {
			t.Errorf("%s stat = %s, want %s", si.Key, si.Stat, want)
		}
	}
}

func TestParseSkill(t *testing.T) {
	for _, s := range dndSkillTable {
		got, ok := parseSkill(s.Display)
		if !ok || got != s.Key {
			t.Errorf("parseSkill(%q) = %v, %v; want %v, true", s.Display, got, ok, s.Key)
		}
	}
	if _, ok := parseSkill("acrobatics"); !ok {
		t.Errorf("parseSkill lowercase failed")
	}
	if _, ok := parseSkill("flying"); ok {
		t.Errorf("parseSkill bogus skill returned ok")
	}
}

func TestParseDC(t *testing.T) {
	cases := []struct {
		s    string
		want int
		ok   bool
	}{
		{"trivial", 5, true},
		{"easy", 10, true},
		{"medium", 15, true},
		{"med", 15, true},
		{"hard", 20, true},
		{"veryhard", 25, true},
		{"very_hard", 25, true},
		{"impossible", 30, true},
		{"17", 17, true},
		{"99", 99, true},
		{"-5", 0, false},
		{"banana", 0, false},
	}
	for _, c := range cases {
		got, ok := parseDC(c.s)
		if got != c.want || ok != c.ok {
			t.Errorf("parseDC(%q) = (%d, %v), want (%d, %v)", c.s, got, ok, c.want, c.ok)
		}
	}
}

func TestStatValue(t *testing.T) {
	c := &DnDCharacter{STR: 14, DEX: 16, CON: 12, INT: 10, WIS: 8, CHA: 18}
	cases := []struct {
		stat string
		want int
	}{
		{"str", 14}, {"dex", 16}, {"con", 12},
		{"int", 10}, {"wis", 8}, {"cha", 18},
		{"unknown", 10}, // default
	}
	for _, tc := range cases {
		if got := statValue(c, tc.stat); got != tc.want {
			t.Errorf("statValue(%s) = %d, want %d", tc.stat, got, tc.want)
		}
	}
}

func TestRaceSkillBonus(t *testing.T) {
	athletics, _ := skillInfo(SkillAthletics)
	persuasion, _ := skillInfo(SkillPersuasion)

	// Half-Elf: +1 to all skills
	he := &DnDCharacter{Race: RaceHalfElf}
	if got := raceSkillBonus(he, athletics); got != 1 {
		t.Errorf("HalfElf athletics = %d, want 1", got)
	}
	if got := raceSkillBonus(he, persuasion); got != 1 {
		t.Errorf("HalfElf persuasion = %d, want 1", got)
	}

	// Tiefling: +2 only on CHA
	tf := &DnDCharacter{Race: RaceTiefling}
	if got := raceSkillBonus(tf, athletics); got != 0 {
		t.Errorf("Tiefling athletics = %d, want 0", got)
	}
	if got := raceSkillBonus(tf, persuasion); got != 2 {
		t.Errorf("Tiefling persuasion = %d, want 2", got)
	}

	// Other races: 0
	hu := &DnDCharacter{Race: RaceHuman}
	if got := raceSkillBonus(hu, persuasion); got != 0 {
		t.Errorf("Human persuasion = %d, want 0", got)
	}
}

// TestPerformSkillCheck_HitRate: a +5 mod vs DC 15 should pass on roll 10+.
// That's 11/20 outcomes (10..20) plus auto-success on nat 20 = 11/20 = 55%.
// Run many trials and check the rate is in band.
func TestPerformSkillCheck_HitRate(t *testing.T) {
	// Fighter STR 16 (+3 mod) → vs DC 15, hits on roll 12+ = 9/20 = 45%.
	c := &DnDCharacter{Class: ClassFighter, Race: RaceHuman, STR: 16}
	hits, trials := 0, 5000
	for i := 0; i < trials; i++ {
		if performSkillCheck(c, SkillAthletics, 15).Success {
			hits++
		}
	}
	rate := float64(hits) / float64(trials)
	// 45% expected, allow 41-49%.
	if rate < 0.41 || rate > 0.49 {
		t.Errorf("hit rate = %.3f, want ~0.45", rate)
	}
}

// TestPerformSkillCheck_AutoSuccess: nat 20 auto-succeeds even when total < DC.
// nat 1 auto-fails even when total >= DC.
func TestPerformSkillCheck_NatExtremes(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Race: RaceHuman, INT: 1}
	// INT 1 → mod -5. Vs DC 30 (impossible), only nat 20 wins.
	saw20, sawNon20 := 0, 0
	for i := 0; i < 500; i++ {
		r := performSkillCheck(c, SkillArcana, 30)
		if r.Success {
			if r.Roll == 20 {
				saw20++
			} else {
				sawNon20++
			}
		}
	}
	if saw20 == 0 {
		t.Error("never saw a nat-20 success at impossible DC over 500 rolls")
	}
	if sawNon20 > 0 {
		t.Errorf("got %d non-nat-20 successes at DC 30 with -5 mod (impossible)", sawNon20)
	}
}

func TestSkillCheckResult_Auto(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, STR: 18}
	// Force run until we see at least one auto-result and verify shape.
	for i := 0; i < 200; i++ {
		r := performSkillCheck(c, SkillAthletics, 15)
		if r.Roll == 20 {
			if !r.Auto || !r.Success {
				t.Errorf("nat 20 should be auto-success: %+v", r)
			}
		}
		if r.Roll == 1 {
			if !r.Auto || r.Success {
				t.Errorf("nat 1 should be auto-fail: %+v", r)
			}
		}
	}
}
