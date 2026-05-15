package plugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Race passives ────────────────────────────────────────────────────────────

func TestApplyRacePassives(t *testing.T) {
	cases := []struct {
		race                                          DnDRace
		wantLucky, wantRage, wantPoisonOK, wantFireOK bool
	}{
		{RaceHalfling, true, false, false, false},
		{RaceOrc, false, true, false, false},
		{RaceDwarf, false, false, true, false},
		{RaceHuman, false, false, false, false},
		{RaceElf, false, false, false, false},
		{RaceTiefling, false, false, false, true},
		{RaceHalfElf, false, false, false, false},
	}
	for _, tc := range cases {
		mods := CombatModifiers{}
		stats := CombatStats{}
		applyRacePassives(&stats, &mods, &DnDCharacter{Race: tc.race})
		if mods.LuckyReroll != tc.wantLucky {
			t.Errorf("%s LuckyReroll = %v, want %v", tc.race, mods.LuckyReroll, tc.wantLucky)
		}
		if mods.RageReady != tc.wantRage {
			t.Errorf("%s RageReady = %v, want %v", tc.race, mods.RageReady, tc.wantRage)
		}
		if mods.PoisonResist != tc.wantPoisonOK {
			t.Errorf("%s PoisonResist = %v, want %v", tc.race, mods.PoisonResist, tc.wantPoisonOK)
		}
		if mods.FireResist != tc.wantFireOK {
			t.Errorf("%s FireResist = %v, want %v", tc.race, mods.FireResist, tc.wantFireOK)
		}
	}
}

// TestHalflingLuckyEmitsReroll: a Halfling fighter rolling many d20s should
// emit at least one lucky_reroll event over a long fight, and never more than
// one (single-use per combat).
func TestHalflingLuckyEmitsReroll(t *testing.T) {
	player := Combatant{
		IsPlayer: true,
		Stats:    CombatStats{MaxHP: 100000, Attack: 5, Defense: 0, Speed: 50, AC: 10, AttackBonus: 5},
		Mods:     CombatModifiers{DamageReduct: 1.0, LuckyReroll: true},
	}
	enemy := Combatant{
		Stats: CombatStats{MaxHP: 100000, Attack: 1, Defense: 0, Speed: 50, AC: 13, AttackBonus: 0},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}
	phases := []CombatPhase{{Name: "Long", Rounds: 200, AttackWeight: 1.0, DefenseWeight: 1.0, SpeedWeight: 1.0}}

	saw := 0
	for trial := 0; trial < 5; trial++ {
		result := SimulateCombat(player, enemy, phases)
		rerolls := 0
		for _, ev := range result.Events {
			if ev.Action == "lucky_reroll" {
				rerolls++
			}
		}
		if rerolls > 1 {
			t.Errorf("trial %d: %d lucky_reroll events in one fight (should be ≤1)", trial, rerolls)
		}
		if rerolls == 1 {
			saw++
		}
	}
	if saw == 0 {
		t.Error("never observed a Halfling Lucky reroll over 5 long fights — extremely unlikely (~5%^5)")
	}
}

// TestOrcRageFiresOnLowHP: an Orc whose HP gets driven below 50% should emit
// a "rage" event and have higher damage on the following attack.
func TestOrcRageFiresOnLowHP(t *testing.T) {
	// Player starts with 50 max HP, low attack. Enemy is a tank that hits
	// back hard so the player crosses the 50% threshold.
	player := Combatant{
		IsPlayer: true,
		Stats:    CombatStats{MaxHP: 50, Attack: 10, Defense: 5, Speed: 5, AC: 10, AttackBonus: 5},
		Mods:     CombatModifiers{DamageReduct: 1.0, RageReady: true},
	}
	enemy := Combatant{
		Stats: CombatStats{MaxHP: 100000, Attack: 30, Defense: 5, Speed: 5, AC: 10, AttackBonus: 8},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}
	phases := []CombatPhase{{Name: "Brawl", Rounds: 40, AttackWeight: 1.0, DefenseWeight: 1.0, SpeedWeight: 1.0}}

	rageFiredEver := false
	for trial := 0; trial < 5; trial++ {
		result := SimulateCombat(player, enemy, phases)
		rageCount := 0
		for _, ev := range result.Events {
			if ev.Action == "rage" {
				rageCount++
			}
		}
		if rageCount > 1 {
			t.Errorf("trial %d: %d rage events (should be ≤1)", trial, rageCount)
		}
		if rageCount == 1 {
			rageFiredEver = true
		}
	}
	if !rageFiredEver {
		t.Error("Orc Rage never fired over 5 brutal fights — should always trigger when player crosses 50%")
	}
}

// TestDwarfPoisonResistance: poison_tick damage applied to a Dwarf should be
// roughly half of the unprotected baseline. We measure by summing the actual
// poison_tick events rather than HP delta, since the engine's exhaustion
// tiebreaker can zero HP independently of poison damage.
func TestDwarfPoisonResistance(t *testing.T) {
	enemy := Combatant{
		Stats:   CombatStats{MaxHP: 1000, Attack: 5, Defense: 0, Speed: 5, AC: 10, AttackBonus: 0},
		Mods:    CombatModifiers{DamageReduct: 1.0},
		Ability: &MonsterAbility{Name: "Spit", Phase: "any", ProcChance: 1.0, Effect: "poison"},
	}
	phases := []CombatPhase{{Name: "Tick", Rounds: 5, AttackWeight: 0.1, DefenseWeight: 1.0, SpeedWeight: 0.1}}

	makePlayer := func(resist bool) Combatant {
		return Combatant{
			IsPlayer: true,
			Stats:    CombatStats{MaxHP: 200, Attack: 1, Defense: 100, Speed: 100, AC: 99, AttackBonus: 0},
			Mods:     CombatModifiers{DamageReduct: 1.0, PoisonResist: resist},
		}
	}
	sumPoison := func(r CombatResult) int {
		s := 0
		for _, ev := range r.Events {
			if ev.Action == "poison_tick" {
				s += ev.Damage
			}
		}
		return s
	}
	totalUnprot, totalProt := 0, 0
	const trials = 300
	for i := 0; i < trials; i++ {
		totalUnprot += sumPoison(SimulateCombat(makePlayer(false), enemy, phases))
		totalProt += sumPoison(SimulateCombat(makePlayer(true), enemy, phases))
	}
	if totalUnprot == 0 {
		t.Fatal("no unprotected poison damage observed; test setup broken")
	}
	avgU := float64(totalUnprot) / float64(trials)
	avgP := float64(totalProt) / float64(trials)
	ratio := avgP / avgU
	if ratio < 0.35 || ratio > 0.65 {
		t.Errorf("dwarf poison ratio = %.3f (avg unprot=%.1f, prot=%.1f); want ~0.5",
			ratio, avgU, avgP)
	}
}

// TestTieflingFireResistance: a fire-tagged monster's primary attack should
// land for ~half damage on a Tiefling. Measured by summing the per-hit damage
// events from a stat-locked encounter so the dice noise averages out.
func TestTieflingFireResistance(t *testing.T) {
	enemy := Combatant{
		// FireAttacker tags this as fire-themed (e.g., fire elemental).
		// AttackBonus high enough to land reliably against player AC 10.
		Stats: CombatStats{MaxHP: 100000, Attack: 12, Defense: 0, Speed: 5, AC: 10, AttackBonus: 10, FireAttacker: true},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}
	phases := []CombatPhase{{Name: "Bake", Rounds: 6, AttackWeight: 1.0, DefenseWeight: 1.0, SpeedWeight: 1.0}}

	makePlayer := func(resist bool) Combatant {
		// Sturdy player that won't die mid-trial; low attack so the enemy
		// keeps swinging the whole phase. No block, no DR ride, no crit
		// path on the enemy side beyond a nat 20.
		return Combatant{
			IsPlayer: true,
			Stats:    CombatStats{MaxHP: 200000, Attack: 1, Defense: 0, Speed: 5, AC: 10, AttackBonus: 0},
			Mods:     CombatModifiers{DamageReduct: 1.0, FireResist: resist},
		}
	}
	sumEnemyHits := func(r CombatResult) int {
		s := 0
		for _, ev := range r.Events {
			if ev.Actor == "enemy" && (ev.Action == "hit" || ev.Action == "crit") {
				s += ev.Damage
			}
		}
		return s
	}
	totalUnprot, totalProt := 0, 0
	const trials = 300
	for i := 0; i < trials; i++ {
		totalUnprot += sumEnemyHits(SimulateCombat(makePlayer(false), enemy, phases))
		totalProt += sumEnemyHits(SimulateCombat(makePlayer(true), enemy, phases))
	}
	if totalUnprot == 0 {
		t.Fatal("no unprotected fire damage observed; test setup broken")
	}
	avgU := float64(totalUnprot) / float64(trials)
	avgP := float64(totalProt) / float64(trials)
	ratio := avgP / avgU
	if ratio < 0.40 || ratio > 0.65 {
		t.Errorf("tiefling fire ratio = %.3f (avg unprot=%.1f, prot=%.1f); want ~0.5",
			ratio, avgU, avgP)
	}
}

// ── combat_level freeze ──────────────────────────────────────────────────────

func TestCheckAdvLevelUp_FrozenForDnDChars(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	uid := id.UserID("@freeze_test:example")

	// Case 1: no dnd_character → combat XP advances combat_level normally.
	char := &AdventureCharacter{
		UserID:      uid,
		DisplayName: "freeze_test",
		CombatLevel: 5,
		CombatXP:    1000, // far over the threshold for L6
	}
	leveled, newLvl := checkAdvLevelUp(char, "combat")
	if !leveled || newLvl <= 5 {
		t.Errorf("non-DnD player: leveled=%v newLvl=%d, want leveled=true, newLvl>5", leveled, newLvl)
	}

	// Case 2: confirmed dnd_character → frozen.
	dnd := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 1,
		STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8,
		HPMax: 12, HPCurrent: 12, ArmorClass: 16,
		PendingSetup: false, AutoMigrated: false,
	}
	if err := SaveDnDCharacter(dnd); err != nil {
		t.Fatal(err)
	}
	char2 := &AdventureCharacter{
		UserID:      uid,
		DisplayName: "freeze_test",
		CombatLevel: 5,
		CombatXP:    1000,
	}
	leveled, newLvl = checkAdvLevelUp(char2, "combat")
	if leveled || newLvl != 5 {
		t.Errorf("DnD player: leveled=%v newLvl=%d, want leveled=false, newLvl=5", leveled, newLvl)
	}

	// Case 3: skill levels still advance for the same player.
	char2.MiningSkill = 5
	char2.MiningXP = 1000
	leveled, newLvl = checkAdvLevelUp(char2, "mining")
	if !leveled || newLvl <= 5 {
		t.Errorf("DnD player mining: leveled=%v newLvl=%d, want leveled=true", leveled, newLvl)
	}
}

// ── !respec cooldown logic ──────────────────────────────────────────────────

func TestRespecCooldownFormat(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{7 * 24 * time.Hour, "7d"},
		{6*24*time.Hour + 5*time.Hour, "6d 5h"},
		{3 * time.Hour, "3h"},
		{45 * time.Minute, "45m"},
	}
	for _, c := range cases {
		got := formatRespecDuration(c.d)
		if got != c.want {
			t.Errorf("formatRespecDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
