package plugin

import (
	"log/slog"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Two one-shot startup bootstraps that lift a low-DPS caster who was stuck in a
// boss-wall "death loop". Diagnosis: the boss isn't overtuned — the player is an
// over-levelled cleric whose damage output is structurally low. Two account
// gaps, fixed idempotently here so they reach the live player without a respec.
//
// 1. bootstrapCasterSpellBackfill — characters created before a default spell
//    was added to defaultKnownSpells keep their original spellbook forever:
//    ensureSpellsForCharacter only seeds when the known-spell list is EMPTY, so
//    later default additions never reach existing casters. This backfills any
//    missing default into known+prepared (e.g. inflict_wounds, added to the
//    cleric defaults after the affected character was rolled). General + future
//    proof — it fixes any caster with the same stale-default gap.
//
// 2. bootstrapGrantStarterPet — a targeted gift of a combat companion to a
//    specific endgame player who never received the 25% morning pet-arrival
//    roll. A pet adds sustained per-round damage + deflect mitigation, which
//    helps caster trailers most.

// bootstrapCasterSpellBackfill adds any missing defaultKnownSpells entry to
// every existing caster as known+prepared. addKnownSpell is idempotent and
// leaves the prepared flag of already-known spells untouched (ON CONFLICT only
// refreshes source), so this only ever adds the genuinely-missing defaults.
func bootstrapCasterSpellBackfill() {
	const jobName = "caster_default_spell_backfill_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}

	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("bootstrap: caster spell backfill — load characters failed", "err", err)
		return
	}

	added := 0
	for _, ac := range chars {
		c, err := LoadDnDCharacter(ac.UserID)
		if err != nil || c == nil || !isSpellcaster(c) {
			continue
		}
		known, err := listKnownSpells(c.UserID)
		if err != nil {
			slog.Warn("bootstrap: caster spell backfill — list known failed", "user", c.UserID, "err", err)
			continue
		}
		have := make(map[string]bool, len(known))
		for _, k := range known {
			have[k.SpellID] = true
		}
		defaults := defaultKnownSpells(c.Class, c.Level)
		if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster {
			defaults = arcaneTricksterDefaultSpells(c.Level)
		}
		for _, sid := range defaults {
			if have[sid] {
				continue
			}
			if err := addKnownSpell(c.UserID, sid, "class", true); err != nil {
				slog.Error("bootstrap: caster spell backfill — add failed", "user", c.UserID, "spell", sid, "err", err)
				continue
			}
			slog.Info("bootstrap: backfilled default spell", "user", c.UserID, "spell", sid)
			added++
		}
	}

	db.MarkJobCompleted(jobName, "once")
	if added > 0 {
		slog.Warn("bootstrap: caster default-spell backfill complete", "spells_added", added)
	}
}

// josieStarterPet identifies the one player the pet gift targets and the pet
// it grants. Kept as data so the intent is legible: this is an admin gift, not
// a game-wide policy.
var josieStarterPet = struct {
	userID id.UserID
	typ    string
	name   string
	level  int
}{
	userID: "@holymachina:parodia.dev",
	typ:    "dog",
	name:   "Biscuit",
	level:  10,
}

// bootstrapGrantStarterPet gives the targeted player a combat companion if they
// have none. No-op once they have a pet (this gift, a later arrival, or one
// chased away — we don't override the player's own pet history). Idempotent via
// the job gate AND the has-pet guard.
func bootstrapGrantStarterPet() {
	const jobName = "grant_starter_pet_holymachina_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}
	g := josieStarterPet

	char, err := loadAdvCharacter(g.userID)
	if err != nil || char == nil {
		// Target not present in this DB (e.g. fresh deploy) — mark done so we
		// don't re-scan every startup; the gift is a one-off, not a standing rule.
		db.MarkJobCompleted(jobName, "once")
		return
	}
	if char.PetType != "" || char.PetArrived {
		slog.Info("bootstrap: starter pet — target already has a pet, skipping", "user", g.userID)
		db.MarkJobCompleted(jobName, "once")
		return
	}

	char.PetType = g.typ
	char.PetName = g.name
	char.PetLevel = g.level
	char.PetXP = 0
	char.PetArrived = true
	char.PetChasedAway = false
	if g.level >= 10 {
		// Mirror the babysit path that stamps the L10 date when a pet first
		// reaches the cap, so milestone/supply-shop logic stays consistent.
		char.PetLevel10Date = time.Now().UTC().Format("2006-01-02")
	}
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("bootstrap: starter pet — save failed", "user", g.userID, "err", err)
		return
	}
	if err := upsertPlayerMetaPetState(char.UserID, petStateFromAdvChar(char)); err != nil {
		slog.Error("bootstrap: starter pet — player_meta mirror failed", "user", g.userID, "err", err)
		return
	}

	db.MarkJobCompleted(jobName, "once")
	slog.Warn("bootstrap: granted starter pet", "user", g.userID, "pet", g.name, "level", g.level)
}
