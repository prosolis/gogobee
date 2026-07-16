package plugin

import (
	"fmt"
	"log/slog"
	"time"

	"maunium.net/go/mautrix/id"
)

// AdminBuildConfirmedCharacter mints (or overwrites) a confirmed D&D character
// for one user with an explicitly chosen race/class/level, reusing the same
// constructors as auto-migration and !setup confirm: class-tuned standard
// array + racial mods, the canonical HP/AC formulas, resource pool, and — for
// casters — the starter spell list and slot pool.
//
// It exists for the "stuck adventurer" cases the boredom ticker can't reach on
// its own:
//
//   - A veteran legacy player with an adventure_characters row but no
//     dnd_character (never triggered auto-migration because auto-migration only
//     fires on active play — an idle player never does). Pass the class/race the
//     faithful migration would infer to reproduce it exactly.
//   - A player who abandoned !setup at race pick (pending_setup=1, no class), so
//     tryBoredomStart skips them forever. This overwrites the draft with a
//     finished sheet.
//
// AutoMigrated is set so the player can freely rebuild via !setup (no respec
// cooldown) when they return — the class was assigned for them, not chosen.
//
// SaveDnDCharacter upserts on user_id, so an existing pending draft is replaced.
// initResources / ensureSpellsForCharacter are idempotent.
func AdminBuildConfirmedCharacter(userID id.UserID, race DnDRace, class DnDClass, level int) (*DnDCharacter, error) {
	if level < 1 {
		level = 1
	}
	scores := applyRaceMods(race, classStatPriority(class))
	c := &DnDCharacter{
		UserID:       userID,
		Race:         race,
		Class:        class,
		Level:        level,
		STR:          scores[0], DEX: scores[1], CON: scores[2],
		INT:          scores[3], WIS: scores[4], CHA: scores[5],
		PendingSetup: false,
		AutoMigrated: true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	conMod := abilityModifier(c.CON)
	dexMod := abilityModifier(c.DEX)
	c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, dexMod)
	c.ShortRestCharges = c.Level

	if err := ensurePlayerMetaSeed(userID); err != nil {
		return nil, fmt.Errorf("seed player_meta: %w", err)
	}
	if err := SaveDnDCharacter(c); err != nil {
		return nil, fmt.Errorf("save dnd_character: %w", err)
	}
	if err := initResources(userID, c.Class); err != nil {
		return nil, fmt.Errorf("init resources: %w", err)
	}
	if err := ensureSpellsForCharacter(c); err != nil {
		return nil, fmt.Errorf("seed spells: %w", err)
	}
	slog.Info("admin: built confirmed character",
		"user", userID, "race", race, "class", class, "level", level,
		"hp", c.HPMax, "ac", c.ArmorClass)
	return c, nil
}
