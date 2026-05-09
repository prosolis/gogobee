package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 1 of the D&D integration layer. See gogobee_dnd_design_doc_v1.1.md.
//
// This file defines the additive D&D character layer that sits alongside
// adventure_characters. A player without a dnd_character row continues to
// use the legacy adventure system unchanged.

// ── Race / Class ─────────────────────────────────────────────────────────────

type DnDRace string
type DnDClass string

const (
	RaceHuman    DnDRace = "human"
	RaceElf      DnDRace = "elf"
	RaceDwarf    DnDRace = "dwarf"
	RaceHalfling DnDRace = "halfling"
	RaceOrc      DnDRace = "orc"
	RaceTiefling DnDRace = "tiefling"
	RaceHalfElf  DnDRace = "half_elf"

	ClassFighter DnDClass = "fighter"
	ClassRogue   DnDClass = "rogue"
	ClassMage    DnDClass = "mage"
	ClassCleric  DnDClass = "cleric"
	ClassRanger  DnDClass = "ranger"
)

type DnDRaceInfo struct {
	Key     DnDRace
	Display string
	// Stat modifiers applied at setup-confirm time. STR/DEX/CON/INT/WIS/CHA.
	// Human's "+1 to any" is not yet implemented in Phase 1 — Human gets +0
	// across the board and we'll add the floating bonus in a later phase.
	Mods    [6]int
	Passive string
}

type DnDClassInfo struct {
	Key      DnDClass
	Display  string
	HPDie    int    // d10 → 10, d8 → 8, d6 → 6
	HPAvg    int    // per-level average after L1 (roundup of die/2 + 1)
	PrimaryA string // primary stat for narrative (not mechanical in Phase 1)
	PrimaryB string
}

var dndRaces = []DnDRaceInfo{
	{RaceHuman, "Human", [6]int{0, 0, 0, 0, 0, 0}, "Versatile (floating +1 bonus not yet implemented)"},
	{RaceElf, "Elf", [6]int{0, 2, -1, 1, 1, 0}, "Darkvision; immune to sleep effects"},
	{RaceDwarf, "Dwarf", [6]int{1, -1, 2, 0, 1, -1}, "Poison resistance; bonus vs. underground enemies"},
	{RaceHalfling, "Halfling", [6]int{0, 2, 0, 0, 1, 0}, "Lucky: once per combat, reroll a natural 1"},
	{RaceOrc, "Orc", [6]int{3, -1, 2, -1, -1, -1}, "Rage: once per combat, +50% damage for one turn"},
	{RaceTiefling, "Tiefling", [6]int{0, 1, 0, 1, 0, 2}, "Fire resistance; bonus on CHA checks"},
	{RaceHalfElf, "Half-Elf", [6]int{0, 1, 0, 1, 0, 2}, "Two bonus skill proficiencies"},
}

var dndClasses = []DnDClassInfo{
	{ClassFighter, "Fighter", 10, 6, "STR", "CON"},
	{ClassRogue, "Rogue", 8, 5, "DEX", "INT"},
	{ClassMage, "Mage", 6, 4, "INT", "WIS"},
	{ClassCleric, "Cleric", 8, 5, "WIS", "CHA"},
	{ClassRanger, "Ranger", 8, 5, "DEX", "WIS"},
}

func raceInfo(r DnDRace) (DnDRaceInfo, bool) {
	for _, ri := range dndRaces {
		if ri.Key == r {
			return ri, true
		}
	}
	return DnDRaceInfo{}, false
}

func classInfo(c DnDClass) (DnDClassInfo, bool) {
	for _, ci := range dndClasses {
		if ci.Key == c {
			return ci, true
		}
	}
	return DnDClassInfo{}, false
}

// ── Stat math ────────────────────────────────────────────────────────────────

// StandardArray is the set of values a player assigns at !setup stats.
var standardArray = [6]int{15, 14, 13, 12, 10, 8}

// abilityModifier implements the standard D&D modifier formula:
// floor((score - 10) / 2). Works for negative-going scores (Go's integer
// division rounds toward zero, so we shift by 10 first via score-10
// arithmetic; for the legal score range 1..30 this matches floor exactly).
func abilityModifier(score int) int {
	diff := score - 10
	if diff >= 0 || diff%2 == 0 {
		return diff / 2
	}
	// negative odd → step toward -inf
	return diff/2 - 1
}

// computeMaxHP — D&D5e style. L1: full HP die + CON mod (min 1).
// Per level after: average HP die (roundup of die/2 + 1) + CON mod.
func computeMaxHP(class DnDClass, conMod, level int) int {
	ci, ok := classInfo(class)
	if !ok {
		return 1
	}
	hp := ci.HPDie + conMod
	if hp < 1 {
		hp = 1
	}
	for i := 2; i <= level; i++ {
		gain := ci.HPAvg + conMod
		if gain < 1 {
			gain = 1
		}
		hp += gain
	}
	return hp
}

// computeAC — Phase 1 baseline. Equipment-derived AC arrives in Phase 4
// (per design doc §7.3 slot mapping). For now: 10 + DEX modifier, with a
// small class armor floor reflecting starting proficiency.
func computeAC(class DnDClass, dexMod int) int {
	floor := 0
	switch class {
	case ClassFighter:
		floor = 6 // medium-armor baseline
	case ClassCleric, ClassRanger:
		floor = 3
	case ClassRogue:
		floor = 1
	}
	return 10 + dexMod + floor
}

// ── DnDCharacter struct + repository ─────────────────────────────────────────

type DnDCharacter struct {
	UserID       id.UserID
	Race         DnDRace
	Class        DnDClass
	Level        int
	XP           int
	STR, DEX, CON int
	INT, WIS, CHA int
	HPCurrent    int
	HPMax        int
	TempHP       int
	ArmorClass   int
	PendingSetup    bool
	AutoMigrated    bool
	OnboardingSent  bool
	ArmedAbility    string
	// Phase 9 — spell layer.
	// PendingCast: JSON blob describing the queued spell to fire on next
	// one-shot combat (empty string = nothing queued). Set by !cast for
	// damage/control/buff spells; cleared after combat resolution.
	// ConcentrationSpell: id of the currently-active concentration spell
	// (empty = nothing active). ConcentrationExpiresAt is the wall-clock
	// expiry; nil for indefinite. Casting another concentration spell
	// supersedes whatever's here.
	PendingCast              string
	ConcentrationSpell       string
	ConcentrationExpiresAt   *time.Time
	// Phase 10 — subclass.
	// Subclass: empty until the player runs !subclass at L5+. Once set,
	// changing requires `!subclass <name>` again subject to a 30-day
	// cooldown (LastSubclassRespecAt) and 500-coin cost.
	Subclass                 DnDSubclass
	LastSubclassRespecAt     *time.Time
	// Phase 10 SUB2a — Berserker Frenzy increments after each rage'd
	// combat; future class abilities will also tick this. Cleared on
	// long rest. 5e caps at 6 (death); we let it grow and let consumers
	// clamp where needed.
	Exhaustion               int
	LastRespecAt    *time.Time
	LastShortRestAt *time.Time
	LastLongRestAt  *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Modifiers returns the six ability modifiers in STR/DEX/CON/INT/WIS/CHA order.
func (c *DnDCharacter) Modifiers() [6]int {
	return [6]int{
		abilityModifier(c.STR),
		abilityModifier(c.DEX),
		abilityModifier(c.CON),
		abilityModifier(c.INT),
		abilityModifier(c.WIS),
		abilityModifier(c.CHA),
	}
}

// LoadDnDCharacter returns the player's D&D row, or (nil, nil) if absent.
// Absent means the player has not run !setup at all.
func LoadDnDCharacter(userID id.UserID) (*DnDCharacter, error) {
	row := db.Get().QueryRow(`
		SELECT user_id, race, class, dnd_level, dnd_xp,
		       str_score, dex_score, con_score, int_score, wis_score, cha_score,
		       hp_current, hp_max, temp_hp, armor_class,
		       pending_setup, auto_migrated, onboarding_sent, armed_ability,
		       pending_cast, concentration_spell, concentration_expires_at,
		       subclass, last_subclass_respec_at, exhaustion,
		       last_respec_at, last_short_rest_at, last_long_rest_at,
		       created_at, updated_at
		FROM dnd_character WHERE user_id = ?`, string(userID))
	c := &DnDCharacter{}
	var pending, autoMig, onboard int
	var raceStr, classStr, subclassStr string
	var uidStr string
	err := row.Scan(&uidStr, &raceStr, &classStr, &c.Level, &c.XP,
		&c.STR, &c.DEX, &c.CON, &c.INT, &c.WIS, &c.CHA,
		&c.HPCurrent, &c.HPMax, &c.TempHP, &c.ArmorClass,
		&pending, &autoMig, &onboard, &c.ArmedAbility,
		&c.PendingCast, &c.ConcentrationSpell, &c.ConcentrationExpiresAt,
		&subclassStr, &c.LastSubclassRespecAt, &c.Exhaustion,
		&c.LastRespecAt, &c.LastShortRestAt, &c.LastLongRestAt,
		&c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load dnd_character: %w", err)
	}
	c.UserID = id.UserID(uidStr)
	c.Race = DnDRace(raceStr)
	c.Class = DnDClass(classStr)
	c.Subclass = DnDSubclass(subclassStr)
	c.PendingSetup = pending == 1
	c.AutoMigrated = autoMig == 1
	c.OnboardingSent = onboard == 1
	return c, nil
}

// SaveDnDCharacter upserts the row.
func SaveDnDCharacter(c *DnDCharacter) error {
	pending := 0
	if c.PendingSetup {
		pending = 1
	}
	autoMig := 0
	if c.AutoMigrated {
		autoMig = 1
	}
	onboard := 0
	if c.OnboardingSent {
		onboard = 1
	}
	_, err := db.Get().Exec(`
		INSERT INTO dnd_character (user_id, race, class, dnd_level, dnd_xp,
		    str_score, dex_score, con_score, int_score, wis_score, cha_score,
		    hp_current, hp_max, temp_hp, armor_class,
		    pending_setup, auto_migrated, onboarding_sent, armed_ability,
		    pending_cast, concentration_spell, concentration_expires_at,
		    subclass, last_subclass_respec_at, exhaustion,
		    last_respec_at, last_short_rest_at, last_long_rest_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
		    race=excluded.race, class=excluded.class,
		    dnd_level=excluded.dnd_level, dnd_xp=excluded.dnd_xp,
		    str_score=excluded.str_score, dex_score=excluded.dex_score,
		    con_score=excluded.con_score, int_score=excluded.int_score,
		    wis_score=excluded.wis_score, cha_score=excluded.cha_score,
		    hp_current=excluded.hp_current, hp_max=excluded.hp_max,
		    temp_hp=excluded.temp_hp, armor_class=excluded.armor_class,
		    pending_setup=excluded.pending_setup,
		    auto_migrated=excluded.auto_migrated,
		    onboarding_sent=excluded.onboarding_sent,
		    armed_ability=excluded.armed_ability,
		    pending_cast=excluded.pending_cast,
		    concentration_spell=excluded.concentration_spell,
		    concentration_expires_at=excluded.concentration_expires_at,
		    subclass=excluded.subclass,
		    last_subclass_respec_at=excluded.last_subclass_respec_at,
		    exhaustion=excluded.exhaustion,
		    last_respec_at=excluded.last_respec_at,
		    last_short_rest_at=excluded.last_short_rest_at,
		    last_long_rest_at=excluded.last_long_rest_at,
		    updated_at=CURRENT_TIMESTAMP`,
		string(c.UserID), string(c.Race), string(c.Class), c.Level, c.XP,
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA,
		c.HPCurrent, c.HPMax, c.TempHP, c.ArmorClass,
		pending, autoMig, onboard, c.ArmedAbility,
		c.PendingCast, c.ConcentrationSpell, c.ConcentrationExpiresAt,
		string(c.Subclass), c.LastSubclassRespecAt, c.Exhaustion,
		c.LastRespecAt, c.LastShortRestAt, c.LastLongRestAt)
	if err != nil {
		return fmt.Errorf("save dnd_character: %w", err)
	}
	return nil
}

// HasCompletedSetup returns true if the player has a confirmed D&D character.
// Returns false for both "no row" and "row with pending_setup=1".
func HasCompletedSetup(userID id.UserID) bool {
	var pending int
	err := db.Get().QueryRow(
		`SELECT pending_setup FROM dnd_character WHERE user_id = ?`,
		string(userID),
	).Scan(&pending)
	if err != nil {
		return false
	}
	return pending == 0
}

// dndLevelFromCombatLevel maps the legacy combat_level onto the D&D level
// scale. Empirical: legacy levels reach 50+, but the D&D progression curve
// is specced through L20. We compress 5:1 — five legacy levels of grinding
// roughly equals one D&D level — and clamp into [1, 20].
//
// Examples: legacy 0-4 → 1, 5-9 → 1, 10 → 2, 20 → 4, 49 → 9, 100+ → 20.
//
// Used by !setup confirm and by ensureDnDCharacterForCombat (auto-migration)
// to seed the initial D&D level. After migration, dnd_level advances on its
// own via XP earned in combat (see dnd_xp.go).
func dndLevelFromCombatLevel(combatLevel int) int {
	lvl := combatLevel / 5
	if lvl < 1 {
		lvl = 1
	}
	if lvl > dndMaxLevel {
		lvl = dndMaxLevel
	}
	return lvl
}

// applyRaceMods adds the race's ability modifiers to a base score block.
func applyRaceMods(race DnDRace, scores [6]int) [6]int {
	ri, ok := raceInfo(race)
	if !ok {
		return scores
	}
	for i := 0; i < 6; i++ {
		scores[i] += ri.Mods[i]
	}
	return scores
}
