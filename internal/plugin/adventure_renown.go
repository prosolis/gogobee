package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"maunium.net/go/mautrix/id"

	"gogobee/internal/db"
)

// N7/B2 — Renown: prestige past the level cap (gogobee_engagement_plan.md §B2).
//
// A confirmed D&D character caps at L20 (dndMaxLevel). Before N7, grantDnDXP
// silently dropped any XP earned past the cap. Renown reclaims that overflow as
// a prestige track: every renownXPPerLevel of overflow is one Renown level.
//
// Storage is a single cumulative column, player_meta.renown_xp, written by an
// atomic INSERT…ON CONFLICT … += (the journal_pages pattern) so there is no
// read-modify-write race. renown_level is DERIVED (renownLevelFor) rather than
// stored, so nothing can disagree with the XP total.
//
// The reward is deliberately prestige-only — a derived rank title, a cosmetic
// marker on the sheet/leaderboard, and a small capped bundle of *activity*
// bonuses (loot quality, XP, death avoidance). It NEVER grants combat stats:
// the balance corpus (SimulateCombat / the golden) must stay valid, so renown
// touches only the AdvBonusSummary activity levers and only at the activity
// call sites, never loadCombatBonuses.

// renownXPPerLevel is the overflow XP that buys one Renown level. Steep by
// design (§B2): a capped L20 player earns ~750 XP per T5 dungeon win, so a
// Renown level is dozens of endgame clears.
const renownXPPerLevel = 25000

// Renown perk ladder. One small step every renownPerkStepLevels Renown levels,
// capped at renownPerkMaxSteps steps. At the cap the perks total +20% XP / +15%
// loot, matching a streak-30 grant's economic half — §B2's ceiling in total
// power.
//
// The perks are deliberately ONLY the two combat-neutral levers of the
// AdvBonusSummary: combat_stats.go maps DeathModifier→Defense,
// SuccessBonus→Attack and ExceptionalBonus→CritRate, so those *are* combat
// stats and are off-limits (§B2: never combat-stat inflation, the balance
// corpus must stay valid). LootQuality and XPMultiplier are read only by the
// loot/XP economy, never by combat stat derivation — so renown can pay them out
// even through loadCombatBonuses without moving the golden. That is why §B2's
// suggested "−death penalty" perk is intentionally NOT granted: it would inflate
// Defense.
const (
	renownPerkStepLevels = 3
	renownPerkMaxSteps   = 10
)

// renownLevelFor derives the Renown level from cumulative overflow XP.
func renownLevelFor(renownXP int) int {
	if renownXP <= 0 {
		return 0
	}
	return renownXP / renownXPPerLevel
}

// renownXPIntoLevel returns (progress, cost) toward the next Renown level, for
// display. cost is always renownXPPerLevel.
func renownXPIntoLevel(renownXP int) (int, int) {
	if renownXP < 0 {
		renownXP = 0
	}
	return renownXP % renownXPPerLevel, renownXPPerLevel
}

// renownRank is one rung of the derived title ladder: the first Renown level at
// which the rank applies, and its name. A rank promotion (crossing into a new
// rung) is what gets announced in the games room; plain level-ups only DM.
type renownRank struct {
	MinLevel int
	Name     string
}

var renownRanks = []renownRank{
	{1, "Renowned"},
	{3, "Storied"},
	{5, "Illustrious"},
	{10, "Fabled"},
	{15, "Mythic"},
	{20, "Ascendant"},
	{30, "Eternal"},
}

// renownRankFor returns the rank name for a Renown level, or "" below level 1.
func renownRankFor(level int) string {
	name := ""
	for _, r := range renownRanks {
		if level >= r.MinLevel {
			name = r.Name
		}
	}
	return name
}

// renownMarker is the cosmetic prestige badge shown next to a name on the sheet
// and leaderboard. Empty below Renown 1.
func renownMarker(level int) string {
	if level < 1 {
		return ""
	}
	return fmt.Sprintf("✦%d", level)
}

// applyRenownBonuses folds the capped renown perks into an AdvBonusSummary. It
// touches ONLY LootQuality and XPMultiplier — the combat-neutral economy levers
// — so it is safe to call anywhere the summary is built, including
// loadCombatBonuses, without changing any combat stat (see the const block).
func applyRenownBonuses(b *AdvBonusSummary, renownLevel int) {
	if b == nil || renownLevel < renownPerkStepLevels {
		return
	}
	steps := renownLevel / renownPerkStepLevels
	if steps > renownPerkMaxSteps {
		steps = renownPerkMaxSteps
	}
	b.XPMultiplier += float64(steps) * 2  // → +20% at the cap
	b.LootQuality += float64(steps) * 1.5 // → +15% at the cap
}

// ── Persistence ──────────────────────────────────────────────────────────────

// renownAccrueSQL is the atomic cumulative += on player_meta.renown_xp (the
// journal_pages pattern), returning the post-update total. Shared by the
// standalone and transactional accrual paths so the SQL lives in one place.
const renownAccrueSQL = `INSERT INTO player_meta (user_id, renown_xp) VALUES (?, ?)
	 ON CONFLICT(user_id) DO UPDATE SET renown_xp = renown_xp + excluded.renown_xp
	 RETURNING renown_xp`

// sqlQueryer is the subset of *sql.DB / *sql.Tx that accrueRenownXP needs, so
// the accrual can run standalone or inside grantDnDXP's save transaction.
type sqlQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// accrueRenownXP adds delta (> 0) to renown_xp against any queryer and returns
// the cumulative totals before and after. A single statement, safe against
// concurrent grants — no lost update.
func accrueRenownXP(q sqlQueryer, userID id.UserID, delta int) (before, after int, err error) {
	if err = q.QueryRow(renownAccrueSQL, string(userID), delta).Scan(&after); err != nil {
		return 0, 0, fmt.Errorf("accrue renown_xp: %w", err)
	}
	return after - delta, after, nil
}

// addRenownXP is the standalone accrual (its own DB write). delta <= 0 is a
// no-op read. grantDnDXP uses the transactional saveDnDCharacterWithOverflow
// instead, so this is the API for callers not already saving a character.
func addRenownXP(userID id.UserID, delta int) (before, after int, err error) {
	if delta <= 0 {
		cur, e := loadRenownXP(userID)
		return cur, cur, e
	}
	return accrueRenownXP(db.Get(), userID, delta)
}

// saveDnDCharacterWithOverflow persists the character and converts overflow XP
// (> 0) to Renown in one transaction, so a crash can neither drop the overflow
// nor double-credit it. Returns the cumulative renown totals before/after.
func saveDnDCharacterWithOverflow(c *DnDCharacter, overflow int) (before, after int, err error) {
	tx, err := db.Get().Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if err = saveDnDCharacterExec(tx, c); err != nil {
		return 0, 0, err
	}
	if before, after, err = accrueRenownXP(tx, c.UserID, overflow); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return before, after, nil
}

// loadRenownXP reads the cumulative overflow XP. Absent row == 0.
func loadRenownXP(userID id.UserID) (int, error) {
	var xp int
	err := db.Get().QueryRow(
		`SELECT renown_xp FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&xp)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return xp, err
}

// renownLevelForUser is the per-user Renown level, for callers (leaderboard)
// that don't already hold the overlay. Errors resolve to 0.
func renownLevelForUser(userID id.UserID) int {
	xp, err := loadRenownXP(userID)
	if err != nil {
		return 0
	}
	return renownLevelFor(xp)
}

// ── Announce ─────────────────────────────────────────────────────────────────

// announceRenown notifies the player of Renown level-ups (DM) and, when the
// gain crossed into a new rank rung, the games room. Event-driven off a grant,
// not a scheduled recap. from/to are Renown levels before/after the grant.
func (p *AdventurePlugin) announceRenown(userID id.UserID, from, to int) {
	if p == nil || to <= from {
		return
	}
	gained := to - from
	rank := renownRankFor(to)
	if p.Client != nil {
		msg := fmt.Sprintf("✦ **Renown %d** ✦\n\nYou've earned %s past the level cap — your legend grows.",
			to, pluralLevels(gained))
		if rank != "" {
			msg += fmt.Sprintf("\nRank: **%s**.", rank)
		}
		if err := p.SendDM(userID, msg); err != nil {
			slog.Error("renown: level-up DM failed", "user", userID, "err", err)
		}
	}
	// Games-room shout only on a rank promotion, to keep the room quiet.
	if renownRankFor(from) == rank {
		return
	}
	gr := gamesRoom()
	if gr == "" {
		return
	}
	name, _ := loadDisplayName(userID)
	if name == "" {
		name = string(userID)
	}
	p.SendMessage(id.RoomID(gr), fmt.Sprintf(
		"✦ **%s** reached **Renown %d — %s.** A legend of the realm.", name, to, rank))
}

func pluralLevels(n int) string {
	if n == 1 {
		return "a Renown level"
	}
	return fmt.Sprintf("%d Renown levels", n)
}
