package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// Pete, the realm's embedded correspondent — the hireable NPC companion.
// (pete_adventure_news_plan.md, "Pete as a character", surface 2.)
//
// A leader short a body can hire Pete into an expedition party. He fills the
// role the party is missing, fights on autopilot, and files a dispatch about it
// afterwards.
//
// The load-bearing rule, and the reason this file exists rather than a
// player_meta row for @pete: **Pete is not a player and must never become one.**
// He has no player_meta, no dnd_character, no inventory, no euros, and no DM
// room. Mint him a player_meta row and ensureDnDCharacterForCombat will happily
// auto-build him a real character on his first swing, at which point he shows up
// in the graveyard, the leaderboards, the news as a subject, and the daily event
// rolls — all of which would be a bot reporting on itself.
//
// So his seat is synthesized: companionCombatant builds a Combatant in memory
// from the same tuned layers a player's sheet goes through, and every seam that
// assumes a seat is a person is guarded by isCompanionSeat. The guards live at
// four chokepoints, which between them cover the whole blast radius:
//
//   - expeditionAudience  — he is never in the DM fan-out (and so never in the
//     per-member pet-arrival rolls or the daily event rolls that ride it)
//   - partySize           — he is not a mouth: he doesn't inflate the supply
//     burn he never bought packs for, and an NPC-only roster doesn't lock the
//     leader out of their next expedition
//   - partyCombatantsForSession / the seat builders — synthesize, don't load
//   - the close-out loops  — no XP, no loot, no death row, no achievements
//
// He does count toward the enemy-HP scalar, because he is a body in the fight
// and the boss can feel him.

// companionUserIDDefault is Pete's real Matrix account. He is an independent bot
// (his own repo, his own voice); gogobee already ignores him as a sender via
// IGNORED_BOTS, so seating him here can never round-trip into command handling.
// PETE_USER_ID overrides for a differently-homed deployment.
const companionUserIDDefault = "@pete:parodia.dev"

// companionDisplayName is what the party sees on his seat. gogobee names him but
// does NOT voice him: his hire banter and his dispatch are written by his own
// bot from the fact emitted below (project_pete_bot_architecture).
const companionDisplayName = "Pete"

// Hire pricing. The plan files cost as an open tuning question; this is the
// first answer, not the final one. It scales with both the level he shows up at
// and the tier he's walking into, so hiring him for a T5 boss run is not the
// same 300 coins as a T1 stroll. Supply packs are 50/90 coins for reference —
// he is deliberately a real expense, not a rounding error.
//
// PROD WATCH: at endgame coin balances this is likely too cheap to be a sink.
// Raise companionHireCoinsPerLevel before raising the base — the base is what a
// low-level player short a friend has to find.
const (
	companionHireBaseCoins     = 300
	companionHireCoinsPerLevel = 60
)

// companionLevelPenalty is what makes him help rather than carry. He arrives one
// level below the party's average — a competent below-median member, per the
// difficulty plan's standing rule: lift the trailing case, never nerf the
// leaders and never touch monster scaling.
const companionLevelPenalty = 1

var (
	ErrCompanionAlreadyHired = errors.New("pete is already with this party")
	ErrCompanionOnAssignment = errors.New("pete is out on assignment")
	ErrCompanionNotHired     = errors.New("pete is not with this party")
)

// companionUserID resolves Pete's Matrix id.
func companionUserID() id.UserID {
	if v := strings.TrimSpace(os.Getenv("PETE_USER_ID")); v != "" {
		return id.UserID(v)
	}
	return id.UserID(companionUserIDDefault)
}

// isCompanionSeat reports whether a roster seat / combat seat is Pete rather
// than a player. This is the predicate every guard in the codebase keys on.
func isCompanionSeat(userID id.UserID) bool { return userID == companionUserID() }

// isCompanionUser is the string-keyed form, for the many seams that carry a raw
// user_id out of the database.
func isCompanionUser(userID string) bool { return id.UserID(userID) == companionUserID() }

// companionHireCost is what the leader pays to bring him along, in coins.
func companionHireCost(level int, tier ZoneTier) int {
	if level < 1 {
		level = 1
	}
	t := int(tier)
	if t < 1 {
		t = 1
	}
	return (companionHireBaseCoins + companionHireCoinsPerLevel*level) * t
}

// ── who he shows up as ───────────────────────────────────────────────────────

// companionRoleFill picks the class Pete plays. He is role-fluid — that is the
// whole point of hiring him — so by default he fills the hole in the roster: no
// healer, he's a Cleric; no damage, he's a Mage; nobody up front, he's a
// Fighter. A leader who knows better can override.
//
// The order of the checks is the priority order: a party with neither a healer
// nor a front line gets the healer, because a party that cannot heal is the one
// that dies.
func companionRoleFill(partyClasses []DnDClass) DnDClass {
	var hasHealer, hasFront, hasDamage bool
	for _, c := range partyClasses {
		switch c {
		case ClassCleric, ClassDruid, ClassBard:
			hasHealer = true
		case ClassPaladin:
			// The one chassis that answers two questions at once.
			hasHealer, hasFront = true, true
		case ClassFighter:
			hasFront = true
		case ClassMage, ClassSorcerer, ClassWarlock, ClassRogue, ClassRanger:
			hasDamage = true
		}
	}
	switch {
	case !hasHealer:
		return ClassCleric
	case !hasFront:
		return ClassFighter
	case !hasDamage:
		return ClassMage
	default:
		// A complete party that hires him anyway gets a second pair of hands up
		// front — the least redundant thing he can be.
		return ClassFighter
	}
}

// parseCompanionClass resolves an explicit `!expedition hire cleric` override.
// Empty (or unknown) means auto-fill.
func parseCompanionClass(arg string) (DnDClass, bool) {
	arg = strings.ToLower(strings.TrimSpace(arg))
	if arg == "" || arg == "auto" {
		return "", false
	}
	for _, ci := range dndClasses {
		if !ci.Playable {
			continue
		}
		if string(ci.Key) == arg || strings.EqualFold(ci.Display, arg) {
			return ci.Key, true
		}
	}
	return "", false
}

// companionHumans is every *player* on the expedition — the roster if one exists,
// and otherwise the owner alone.
//
// The fallback is the whole point. A solo expedition has NO expedition_party rows
// (see partyMembers: absence means solo, and the roster only materializes on the
// first successful invite). Reading the roster alone therefore answers "nobody" for
// exactly the player this feature exists for: the one with no friends around, who
// is hiring Pete *because* they are alone.
//
// Getting this wrong is not a small error. It hired every solo player a **level-1**
// Pete — in a tier-4 zone, against a boss that had just gained 15% HP and a full
// extra set of actions to account for him. He died on contact and left the leader
// fighting an inflated boss alone. A 1500-run sweep measured it: solo 65% clear,
// two humans 87%, solo+Pete 33%. The companion was worse than no companion, and
// this line is why.
func companionHumans(expeditionID string) []*DnDCharacter {
	var owner string
	if err := db.Get().QueryRow(
		`SELECT user_id FROM dnd_expedition WHERE expedition_id = ?`,
		expeditionID).Scan(&owner); err != nil {
		return nil
	}
	seats, err := partyHumans(expeditionID, owner)
	if err != nil {
		return nil
	}
	out := make([]*DnDCharacter, 0, len(seats))
	for _, s := range seats {
		if dc, _ := LoadDnDCharacter(s.UserID); dc != nil {
			out = append(out, dc)
		}
	}
	return out
}

// companionPartyLevel is the level he arrives at: the party's average, less
// companionLevelPenalty, floored at 1.
func companionPartyLevel(expeditionID string) int {
	chars := companionHumans(expeditionID)
	if len(chars) == 0 {
		return 1
	}
	sum := 0
	for _, dc := range chars {
		sum += dc.Level
	}
	lvl := sum/len(chars) - companionLevelPenalty
	if lvl < 1 {
		lvl = 1
	}
	return lvl
}

// companionPartyClasses reads the classes already on the expedition, for the role
// fill. Solo resolves to the owner's class, so a lone fighter gets a healer rather
// than the empty-party default.
func companionPartyClasses(expeditionID string) []DnDClass {
	chars := companionHumans(expeditionID)
	out := make([]DnDClass, 0, len(chars))
	for _, dc := range chars {
		out = append(out, dc.Class)
	}
	return out
}

// ── his sheet, which lives only in memory ────────────────────────────────────

// companionSheet synthesizes the DnDCharacter Pete fights as. It is built, used,
// and thrown away inside a single combat build — it is never saved, and
// SaveDnDCharacter must never be called on it.
//
// Stats come from the same class-priority + race-mod pipeline autoBuildCharacter
// uses for a real character, so he is statted like a player of his level rather
// than by a bespoke NPC table that would drift away from the tuned math. Human
// is deliberate: the +1-to-all is the most neutral race in the book, so his
// class is doing the work rather than a race pick nobody chose.
func companionSheet(class DnDClass, level int) *DnDCharacter {
	if level < 1 {
		level = 1
	}
	scores := applyRaceMods(RaceHuman, classStatPriority(class))
	c := &DnDCharacter{
		UserID: companionUserID(),
		Race:   RaceHuman,
		Class:  class,
		Level:  level,
		STR:    scores[0], DEX: scores[1], CON: scores[2],
		INT: scores[3], WIS: scores[4], CHA: scores[5],
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	c.HPMax = computeMaxHP(c.Class, abilityModifier(c.CON), c.Level)
	c.HPCurrent = c.HPMax
	return c
}

// companionAdvCharacter is the AdventureCharacter half of the synthetic sheet:
// the shim DerivePlayerStats needs. CombatLevel round-trips back to the D&D
// level through dndLevelFromCombatLevel (level*5), so the two halves agree.
func companionAdvCharacter(level int) *AdventureCharacter {
	return &AdventureCharacter{
		UserID:      companionUserID(),
		DisplayName: companionDisplayName,
		CombatLevel: level * 5,
		Alive:       true,
	}
}

// companionGearTier maps his level onto the equipment tier a player of that
// level would plausibly be carrying: 1–4 → T1, 5–8 → T2, and so on to T5.
func companionGearTier(level int) int {
	t := (level + 3) / 4
	if t < 1 {
		t = 1
	}
	if t > 5 {
		t = 5
	}
	return t
}

// companionGear is the kit he walks in with — a working reporter's kit, bought
// with expenses and kept in serviceable shape.
//
// He has to have one. The gear layer is not decoration: unarmored, stats.AC is
// never set at all (computeArmorAC only fires when armor exists) and he walks in
// at AC 3, hit by everything; weaponless, stats.Weapon stays nil and he swings
// for a flat 5 at every level, which by L14 is nothing. "No gear" is not a
// below-median player, it is a broken one.
//
// The below-median comes from everywhere else: he is a level down, his gear is
// never Masterwork, and he carries no magic items, no subclass and no armed
// ability. The weapon names are chosen to hit the right branch of
// synthesizeWeaponProfile for the class — it best-fits off the name.
func companionGear(class DnDClass, level int) map[EquipmentSlot]*AdvEquipment {
	tier := companionGearTier(level)

	weapon := "Service Mace"
	switch class {
	case ClassFighter, ClassPaladin:
		weapon = "Service Sword"
	case ClassRogue:
		weapon = "Service Dagger"
	case ClassRanger:
		weapon = "Service Bow"
	case ClassMage, ClassSorcerer, ClassWarlock, ClassDruid, ClassBard:
		weapon = "Service Staff"
	}

	return map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Slot: SlotWeapon, Tier: tier, Condition: 100, Name: weapon},
		SlotArmor:  {Slot: SlotArmor, Tier: tier, Condition: 100, Name: "Service Kit"},
	}
}

// companionCombatant builds Pete's seat for a fight, mirroring buildZoneCombatants
// but sourcing the sheet from memory instead of the database. He carries no
// equipment, no treasure bonuses, no magic items, no chat level and no streak —
// the three layers a player accumulates and he never will. That absence *is* the
// below-median: he is a bare class chassis at a level below yours, and the gap
// between him and a geared player of the same level is exactly the gear.
func (p *AdventurePlugin) companionCombatant(
	class DnDClass, level int, monster DnDMonsterTemplate, tier int, dmMood int,
) (Combatant, Combatant, *DnDCharacter) {
	tilt := dmMoodCombatTilt(dmMood)
	char := companionAdvCharacter(level)
	dc := companionSheet(class, level)

	// The layer order is buildZoneCombatants', deliberately — a companion statted
	// by a different pipeline would drift away from the tuned math the moment
	// anyone touched one and not the other.
	//
	// What he does NOT get is the subclass layer, magic items, and an armed
	// ability: three of the things a player accumulates and a hireling never will.
	// Those absences, plus the level penalty and gear that is never Masterwork,
	// are the "below median" — see companionGear for why the gear itself is not
	// one of the things we take away.
	gear := companionGear(class, level)
	stats, mods := DerivePlayerStats(char, gear, &AdvBonusSummary{}, 0, 0, false)
	applyDnDPlayerLayer(&stats, dc)
	applyDnDEquipmentLayer(&stats, dc, gear)
	applyDnDHPScaling(&stats, dc)
	applyClassPassives(&stats, &mods, dc)
	applyRacePassives(&stats, &mods, dc)

	enemyStats, enemyMods := monster.toCombatStats()
	if tier > 1 {
		if floorAC := dndDungeonACBase + tier; enemyStats.AC < floorAC {
			enemyStats.AC = floorAC
		}
		if floorAB := dndDungeonAtkBase + tier; enemyStats.AttackBonus < floorAB {
			enemyStats.AttackBonus = floorAB
		}
	}
	enemyStats.Attack += tilt.EnemyAttackDelta
	if enemyStats.Attack < 1 {
		enemyStats.Attack = 1
	}
	mods.InitiativeBias += tilt.InitiativeBias

	player := Combatant{
		Name:     companionDisplayName,
		Stats:    stats,
		Mods:     mods,
		IsPlayer: true,
	}
	enemy := Combatant{
		Name:    monster.Name,
		Stats:   enemyStats,
		Mods:    enemyMods,
		Ability: monster.Ability,
	}
	return player, enemy, dc
}

// companionRosterLine is how he reads on `!expedition party`. DisplayName would
// come back empty for him — he has no player_meta row to hold a name — so the
// roster names him here, along with what he is currently playing, because "Pete
// (member)" tells the leader nothing about the hole they paid to fill.
func companionRosterLine(expeditionID string) string {
	class, level := companionLoadout(expeditionID)
	ci, _ := classInfo(class)
	return fmt.Sprintf("**%s** _(hired — level %d %s)_\n", companionDisplayName, level, ci.Display)
}

// ── the hire, persisted on the roster ────────────────────────────────────────

// companionLoadout reads back the class and level he was hired at. It is stored
// on the roster row rather than re-derived per fight, so a party that levels
// mid-expedition doesn't quietly re-roll their hireling into a different class
// three rooms in.
func companionLoadout(expeditionID string) (DnDClass, int) {
	var class string
	var level int
	err := db.Get().QueryRow(`
		SELECT companion_class, companion_level
		  FROM expedition_party
		 WHERE expedition_id = ? AND user_id = ?`,
		expeditionID, string(companionUserID())).Scan(&class, &level)
	if err != nil || class == "" {
		return ClassFighter, 1
	}
	return DnDClass(class), level
}

// ── his spell slots, which live on the expedition ────────────────────────────
//
// A human caster's slots are dnd_spell_slots rows: one pool, spent across every
// fight of the run, refilled only at camp. Rationing it is the caster's game. The
// companion has no rows, so his pool lives on his roster row — the same row his
// class and level live on, and with the same lifetime.
//
// It must NOT live on his combat seat. A seat is per-session and every fight opens
// a new one, so a seat-scoped pool refills itself between fights: an infinite
// caster. That is not a theory — the first cut did exactly that, and the sim
// measured a gearless, level-penalized hireling out-clearing a human cleric of the
// leader's own level by 15pp.

// companionSlotsCSV encodes/decodes the ledger. CSV of six ints rather than JSON
// because it is six ints.
func companionSlotsDecode(s string) [6]int {
	var out [6]int
	for i, f := range strings.Split(s, ",") {
		if i >= len(out) {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 {
			continue
		}
		out[i] = n
	}
	return out
}

func companionSlotsEncode(used [6]int) string {
	parts := make([]string, len(used))
	for i, n := range used {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// companionSlotsForRun reads the ledger for the companion on the expedition that
// owns runID. A run with no companion (or no expedition) reads as an empty pool,
// which is the correct answer: nobody spent anything.
func companionSlotsForRun(runID string) [6]int {
	var raw string
	err := db.Get().QueryRow(`
		SELECT p.companion_slots_used
		  FROM expedition_party p
		  JOIN dnd_expedition e ON e.expedition_id = p.expedition_id
		 WHERE e.run_id = ? AND p.user_id = ?`,
		runID, string(companionUserID())).Scan(&raw)
	if err != nil {
		return [6]int{}
	}
	return companionSlotsDecode(raw)
}

// setCompanionSlotsForRun writes it back.
func setCompanionSlotsForRun(runID string, used [6]int) error {
	_, err := db.Get().Exec(`
		UPDATE expedition_party
		   SET companion_slots_used = ?
		 WHERE user_id = ?
		   AND expedition_id = (SELECT expedition_id FROM dnd_expedition WHERE run_id = ?)`,
		companionSlotsEncode(used), string(companionUserID()), runID)
	return err
}

// refreshCompanionSlots empties the ledger — his half of the camp rest that calls
// refreshSpellSlots for every human. Keyed by expedition, because camp is.
func refreshCompanionSlots(expeditionID string) error {
	_, err := db.Get().Exec(`
		UPDATE expedition_party SET companion_slots_used = ''
		 WHERE expedition_id = ? AND user_id = ?`,
		expeditionID, string(companionUserID()))
	return err
}

// ── his body, which is also carried across the run ───────────────────────────
//
// companionUnsetHP is "no wound recorded" — a fresh hire, or a companion who has
// just broken camp. Seating reads it as full.
const companionUnsetHP = -1

// companionHPFor reads the HP he carries into his next fight, or companionUnsetHP
// when he is unhurt. A run with no companion reads unset, which is harmless: there
// is nobody to seat.
func companionHPFor(expeditionID string) int {
	hp := companionUnsetHP
	err := db.Get().QueryRow(`
		SELECT companion_hp FROM expedition_party
		 WHERE expedition_id = ? AND user_id = ?`,
		expeditionID, string(companionUserID())).Scan(&hp)
	if err != nil {
		return companionUnsetHP
	}
	return hp
}

// companionSeatHP is what he actually sits down with: his carried wound, clamped
// into [1, maxHP].
//
// The floor of 1 is deliberate. He can be dropped *inside* a fight — the engine
// counts him out like any other seat — but he does not stay dead between them,
// because there is no companion-death mechanic and inventing one here would be a
// second feature smuggled into a bug fix. Coming back on 1 HP is a real penalty
// (one hit and he is down again) without pretending to be a corpse rule.
func companionSeatHP(expeditionID string, maxHP int) int {
	hp := companionHPFor(expeditionID)
	if hp == companionUnsetHP || hp > maxHP {
		return maxHP
	}
	if hp < 1 {
		return 1
	}
	return hp
}

// setCompanionHP records the HP he walked out of a fight with.
func setCompanionHP(expeditionID string, hp int) error {
	_, err := db.Get().Exec(`
		UPDATE expedition_party SET companion_hp = ?
		 WHERE expedition_id = ? AND user_id = ?`,
		hp, expeditionID, string(companionUserID()))
	return err
}

// setCompanionHPForRun is setCompanionHP for the turn-based close-out, which
// holds a run id rather than an expedition id.
func setCompanionHPForRun(runID string, hp int) error {
	_, err := db.Get().Exec(`
		UPDATE expedition_party
		   SET companion_hp = ?
		 WHERE user_id = ?
		   AND expedition_id = (SELECT expedition_id FROM dnd_expedition WHERE run_id = ?)`,
		hp, string(companionUserID()), runID)
	return err
}

// refreshCompanionHP is his half of the camp heal: back to full, like every human
// at a standard rest.
func refreshCompanionHP(expeditionID string) error {
	_, err := db.Get().Exec(`
		UPDATE expedition_party SET companion_hp = ?
		 WHERE expedition_id = ? AND user_id = ?`,
		companionUnsetHP, expeditionID, string(companionUserID()))
	return err
}

// companionLoadoutForRun is companionLoadout keyed by the zone run instead of
// the expedition. A CombatSession carries a RunID, not an expedition id, and the
// per-turn rebuild is the hottest caller — so the join lives here rather than
// making every caller resolve the expedition first.
//
// A run with no expedition (a standalone !zone run, which can have no companion)
// finds no row and falls back, which is the correct reading.
func companionLoadoutForRun(runID string) (DnDClass, int) {
	var class string
	var level int
	err := db.Get().QueryRow(`
		SELECT p.companion_class, p.companion_level
		  FROM expedition_party p
		  JOIN dnd_expedition e ON e.expedition_id = p.expedition_id
		 WHERE e.run_id = ? AND p.user_id = ?`,
		runID, string(companionUserID())).Scan(&class, &level)
	if err != nil || class == "" {
		return ClassFighter, 1
	}
	return DnDClass(class), level
}

// companionExpeditionFor resolves the expedition a leader is running, for the
// seat builder — which has the roster but not the expedition id. Empty string
// when there is none, which companionLoadout reads as "no row" and falls back.
func companionExpeditionFor(leader id.UserID) string {
	e, _, err := activeExpeditionFor(leader)
	if err != nil || e == nil {
		return ""
	}
	return e.ID
}

// companionSeated reports whether Pete is on this roster.
func companionSeated(expeditionID string) bool {
	var one int
	err := db.Get().QueryRow(`
		SELECT 1 FROM expedition_party WHERE expedition_id = ? AND user_id = ?`,
		expeditionID, string(companionUserID())).Scan(&one)
	return err == nil
}

// hireCompanion seats Pete. The seat check, the availability check and the
// insert share a transaction, so two leaders racing for him cannot both win.
//
// He is globally exclusive — one party at a time. That is not a limitation to
// route around: "Pete is out on assignment" is the scarcity knob the plan asks
// for, and it is also why he cannot be the answer to every run.
func hireCompanion(expeditionID string, class DnDClass, level int) error {
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := seatLeader(tx, expeditionID); err != nil {
		return err
	}

	// Is he already out with somebody? Scoped to live expeditions, so a roster
	// row stranded by a crash cannot make him unhireable forever.
	var busyOn string
	err = tx.QueryRow(`
		SELECT p.expedition_id
		  FROM expedition_party p
		  JOIN dnd_expedition e ON e.expedition_id = p.expedition_id
		 WHERE p.user_id = ? AND e.status IN ('active', 'extracting')
		 LIMIT 1`, string(companionUserID())).Scan(&busyOn)
	switch {
	case err == nil && busyOn == expeditionID:
		return ErrCompanionAlreadyHired
	case err == nil:
		return ErrCompanionOnAssignment
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	var n int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM expedition_party WHERE expedition_id = ?`,
		expeditionID).Scan(&n); err != nil {
		return err
	}
	if n >= expeditionPartyMax {
		return ErrPartyFull
	}

	if _, err := tx.Exec(`
		INSERT INTO expedition_party (expedition_id, user_id, role, companion_class, companion_level)
		VALUES (?, ?, 'member', ?, ?)`,
		expeditionID, string(companionUserID()), string(class), level); err != nil {
		return fmt.Errorf("seat companion: %w", err)
	}
	return tx.Commit()
}

// dismissCompanion sends him home. Unlike a player member he can be removed
// mid-run — he is staff, not a guest — but the fee is not refunded: he already
// walked in.
func dismissCompanion(expeditionID string) error {
	res, err := db.Get().Exec(`
		DELETE FROM expedition_party
		 WHERE expedition_id = ? AND user_id = ?`,
		expeditionID, string(companionUserID()))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCompanionNotHired
	}
	return nil
}

// ── the dispatch ─────────────────────────────────────────────────────────────

// emitCompanionHireFact tells Pete's bot he has been hired. gogobee states the
// fact; Pete writes the sentence. Bulletin, not priority: a hire is a diary
// entry, not a bulletin-interrupting event.
//
// The subject is the *leader*, so the leader's news opt-out is honoured — Pete
// reporting "filled in as cleric for <someone who asked not to be named>" would
// be exactly the leak the opt-out exists to prevent. Pete himself is never a
// subject: he has no opt-out row and needs none.
func emitCompanionHireFact(leader id.UserID, class DnDClass, level int, zone ZoneDefinition) {
	name := charName(leader)
	if name == "" {
		return
	}
	ci, _ := classInfo(class)
	emitFact(peteclient.Fact{
		GUID:       "companion_hire:" + eventToken(leader, "companion_hire") + ":" + fmt.Sprint(time.Now().UTC().Unix()),
		EventType:  "companion_hire",
		Tier:       "bulletin",
		Subject:    name,
		Zone:       zone.Display,
		Level:      level,
		ClassRace:  ci.Display,
		OccurredAt: time.Now().UTC().Unix(),
	}, leader, "")
}
