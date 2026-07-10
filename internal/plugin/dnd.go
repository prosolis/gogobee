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

	// Caster classes — fully playable. Wired through the mechanical layer
	// (HP/AC, slot tables, spellcasting ability) and shipped alongside the
	// Open5e spell-data import: Playable=true and spell lists populated.
	// Each chassis has a Phase-2/3 balance rider in applyClassPassives plus
	// a subclass entry at L3/L5 (see dnd_subclass*.go).
	ClassDruid    DnDClass = "druid"
	ClassBard     DnDClass = "bard"
	ClassSorcerer DnDClass = "sorcerer"
	ClassWarlock  DnDClass = "warlock"
	ClassPaladin  DnDClass = "paladin"
)

type DnDRaceInfo struct {
	Key     DnDRace
	Display string
	// Stat modifiers applied at setup-confirm time. STR/DEX/CON/INT/WIS/CHA.
	// Human takes the Standard Human flavor: +1 to every stat, no setup-wizard
	// choice (keeps !setup uniform across races).
	Mods    [6]int
	Passive string
	// BestFit is the menu-time hint that frames a stat spread as a specialist
	// build rather than a list of penalties (Orc with three -1s reads brutal
	// out of context; "best with: Barbarian, Fighter" reframes it). Comma-
	// separated class display names; rendered by renderRaceMenu.
	BestFit string
}

type DnDClassInfo struct {
	Key      DnDClass
	Display  string
	HPDie    int    // d10 → 10, d8 → 8, d6 → 6
	HPAvg    int    // per-level average after L1 (roundup of die/2 + 1)
	PrimaryA string // primary stat for narrative (not mechanical in Phase 1)
	PrimaryB string
	// Playable gates whether the class is offered at !setup. The Open5e
	// caster classes are wired through the engine but stay false until
	// their spell lists exist — see the ClassDruid… block above.
	Playable bool
}

// Race mods are tuned for equal *effective* power, not equal net total.
// The weighted balance pass (dnd_race_balance.go) scores each race's mods
// against a blend of per-class combat priorities and class-independent
// non-combat utility (zone locks, harvest, skills, haggling). These blocks
// are tuned so every race's mean score across all playable classes lands
// near the Standard Human baseline of 6.0. Net mod totals vary (Elf +7,
// Orc +6) — a spiky race concentrated into high-value stats needs fewer
// points to match a flat one.
var dndRaces = []DnDRaceInfo{
	{RaceHuman, "Human", [6]int{1, 1, 1, 1, 1, 1}, "Versatile: +1 to every ability score", "any class"},
	{RaceElf, "Elf", [6]int{0, 3, -1, 2, 3, 0}, "Keen senses; trance keeps you sharp through long marches", "Ranger, Mage, Druid"},
	{RaceDwarf, "Dwarf", [6]int{2, -1, 3, 1, 1, -1}, "Poison resistance; sure-footed in the deep places", "Fighter, Cleric, Paladin"},
	{RaceHalfling, "Halfling", [6]int{0, 3, 1, 0, 2, 0}, "Lucky: once per combat, reroll a natural 1", "Rogue, Bard, Ranger"},
	{RaceOrc, "Orc", [6]int{6, -1, 3, -1, -1, 0}, "Rage: once per combat, +50% damage for one turn", "Fighter, Ranger"},
	{RaceTiefling, "Tiefling", [6]int{0, 2, 0, 1, 0, 3}, "Fiendish heritage: incoming fire damage halved", "Sorcerer, Warlock, Bard"},
	{RaceHalfElf, "Half-Elf", [6]int{0, 2, 0, 1, 1, 2}, "Adaptable: cross-cultural know-how, equally at ease in any company", "Bard, Paladin, Sorcerer"},
}

var dndClasses = []DnDClassInfo{
	{ClassFighter, "Fighter", 10, 6, "STR", "CON", true},
	{ClassRogue, "Rogue", 8, 5, "DEX", "INT", true},
	{ClassMage, "Mage", 6, 4, "INT", "WIS", true},
	{ClassCleric, "Cleric", 8, 5, "WIS", "CHA", true},
	{ClassRanger, "Ranger", 8, 5, "DEX", "WIS", true},
	// Open5e caster scaffold — Playable flipped on once the SRD spell import
	// gave each of them a real spell list (see dnd_spells_srd_data.go).
	{ClassDruid, "Druid", 8, 5, "WIS", "CON", true},
	{ClassBard, "Bard", 8, 5, "CHA", "DEX", true},
	{ClassSorcerer, "Sorcerer", 6, 4, "CHA", "CON", true},
	{ClassWarlock, "Warlock", 8, 5, "CHA", "CON", true},
	{ClassPaladin, "Paladin", 10, 6, "STR", "CHA", true},
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

// phase5BHPMult is the Phase 5-B HP-floor multiplier. Applied uniformly
// in computeMaxHP so every class/level scales together — preserves the
// class-balance harness's parity assertions (which test in-tier spread,
// not absolute win rates) while lifting durability enough to land the
// expedition harness in the "fairly breezy with some death" band the
// user chose for Phase 5-B. See gogobee_expedition_difficulty.md for
// the sweep that picked 1.5 (gear-bonus floor +3, HP ×1.5 → T1=85 T2=73
// T3=60 T4=78 T5=81 mean completion at the tier-centerline cell).
//
// Bumping this changes the *persisted* HPMax for new characters and for
// existing characters on their next computeMaxHP recall (level-up,
// character reset, bootstrap refresh). The Phase 5-B bootstrap
// (bootstrap_phase5b_hp.go) walks the dnd_character table once at
// startup to refresh stale rows.
const phase5BHPMult = 1.5

// casterHPMult is the J3 D8-d-fix caster durability lift. T4 sim showed
// caster HPMax sat ~30% below martial (~95 vs 141 at L10) and the AC-floor
// lift alone wasn't enough to crack the wall. Applied multiplicatively in
// computeMaxHP on top of phase5BHPMult so the existing Phase 5-B floor
// stays intact for martials. Refreshed for existing rows by
// bootstrapCasterHPRefresh (job key caster_hp_refresh_v1).
func casterHPMult(class DnDClass) float64 {
	switch class {
	case ClassCleric, ClassDruid, ClassBard, ClassWarlock, ClassMage, ClassSorcerer:
		return 1.25
	}
	return 1.0
}

// computeMaxHP — D&D5e style. L1: full HP die + CON mod (min 1).
// Per level after: average HP die (roundup of die/2 + 1) + CON mod.
// The result is scaled by phase5BHPMult — see that constant for the
// difficulty-pass motivation.
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
	// Phase 5-B player power floor + J3 D8-d-fix caster lift — round to nearest int.
	scaled := int(float64(hp)*phase5BHPMult*casterHPMult(class) + 0.5)
	if scaled < 1 {
		scaled = 1
	}
	return scaled
}

// computeAC — Phase 1 baseline. Equipment-derived AC arrives in Phase 4
// (per design doc §7.3 slot mapping). For now: 10 + DEX modifier, with a
// small class armor floor reflecting starting proficiency.
func computeAC(class DnDClass, dexMod int) int {
	floor := 0
	switch class {
	case ClassFighter, ClassPaladin:
		floor = 6 // heavy/medium-armor baseline
	case ClassCleric, ClassDruid:
		floor = 5
	case ClassRanger:
		floor = 3
	case ClassBard, ClassWarlock:
		floor = 3
	case ClassRogue:
		floor = 1
	case ClassMage, ClassSorcerer:
		floor = 2
	}
	return 10 + dexMod + floor
}

// ── DnDCharacter struct + repository ─────────────────────────────────────────

type DnDCharacter struct {
	UserID         id.UserID
	Race           DnDRace
	Class          DnDClass
	Level          int
	XP             int
	STR, DEX, CON  int
	INT, WIS, CHA  int
	HPCurrent      int
	HPMax          int
	TempHP         int
	ArmorClass     int
	PendingSetup   bool
	AutoMigrated   bool
	OnboardingSent bool
	ArmedAbility   string
	// Phase 9 — spell layer.
	// PendingCast: JSON blob describing the queued spell to fire on next
	// one-shot combat (empty string = nothing queued). Set by !cast for
	// damage/control/buff spells; cleared after combat resolution.
	// ConcentrationSpell: id of the currently-active concentration spell
	// (empty = nothing active). ConcentrationExpiresAt is the wall-clock
	// expiry; nil for indefinite. Casting another concentration spell
	// supersedes whatever's here.
	PendingCast            string
	ConcentrationSpell     string
	ConcentrationExpiresAt *time.Time
	// Phase 10 — subclass.
	// Subclass: empty until the player runs !subclass at L5+. Once set,
	// changing requires `!subclass <name>` again subject to a 30-day
	// cooldown (LastSubclassRespecAt) and 500-coin cost.
	Subclass             DnDSubclass
	LastSubclassRespecAt *time.Time
	// Phase 10 SUB2a — Berserker Frenzy increments after each rage'd
	// combat; future class abilities will also tick this. Cleared on
	// long rest. 5e caps at 6 (death); we let it grow and let consumers
	// clamp where needed.
	Exhaustion int
	// 2026-05-10 immersion: short rest charges (1/level, restored on long
	// rest). resting_until gates !zone enter / !expedition start while the
	// character is "still resting" — short rest = 1h, long rest = 8h.
	ShortRestCharges int
	RestingUntil     *time.Time
	LastRespecAt     *time.Time
	LastShortRestAt  *time.Time
	LastLongRestAt   *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
		       short_rest_charges, resting_until,
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
		&c.ShortRestCharges, &c.RestingUntil,
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
		    short_rest_charges, resting_until,
		    last_respec_at, last_short_rest_at, last_long_rest_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
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
		    short_rest_charges=excluded.short_rest_charges,
		    resting_until=excluded.resting_until,
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
		c.ShortRestCharges, c.RestingUntil,
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

// dndLevelForUser returns the player's display-facing D&D level. Post-L5g
// every legacy player has a DnDCharacter row (auto-migrated by the one-shot
// mass-backfill on Init), so the legacy CombatLevel fallback that lived here
// during the soak window has been retired. Returns 1 as a safe default for
// users without any D&D row (e.g. brand-new accounts before first combat).
func dndLevelForUser(userID id.UserID) int {
	if c, err := LoadDnDCharacter(userID); err == nil && c != nil && c.Level > 0 {
		return c.Level
	}
	return 1
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
