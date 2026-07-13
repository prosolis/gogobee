package plugin

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Bored adventurers (gogobee_boredom_plan.md).
//
// A player who stops tending their adventurer doesn't stop having one. After
// boredomIdleThreshold with no action against Adventure, the character gets
// restless and leaves on an expedition by itself: the easiest zone its level
// band allows, the cheapest supplies it can afford, and whatever gear was
// already on the rack.
//
// Everything downstream of the start is already autonomous — the autopilot
// (expedition_autorun.go) walks rooms, drives elite and boss fights on the real
// turn engine, camps, harvests, rolls days over, and auto-picks forks after 8h.
// The only thing that ever needed a human was `!expedition start`, so that is
// the only thing this file does.
//
// It never buys or equips gear, and that is the entire mechanic: a neglected
// adventurer grinds half-starved runs on rusting equipment and comes home taxed.

const (
	// boredomIdleThreshold — silence, measured against the player's last
	// action *against Adventure*, before the adventurer leaves on its own.
	boredomIdleThreshold = 24 * time.Hour

	// boredomCooldown — minimum gap between boredom starts for one player.
	// Also the re-trigger cadence: when a bored expedition ends, another
	// boredomCooldown of silence starts the next one. An abandoned character
	// keeps going until it runs out of coins for supplies.
	boredomCooldown = 24 * time.Hour

	// boredomTickInterval — how often the ticker wakes. The cooldown gate is
	// what actually paces starts; the tick only has to be frequent enough to
	// enforce it cleanly.
	boredomTickInterval = 30 * time.Minute
)

// ── The idle clock ───────────────────────────────────────────────────────────

// markPlayerAction stamps player_meta.last_player_action_at — the clock the
// boredom ticker reads.
//
// It is a direct UPDATE and deliberately does NOT route through
// saveAdvCharacter. Every other candidate timestamp in the schema is unusable
// here for the same reason (gogobee_boredom_plan.md §1):
//
//   - last_active_at is auto-bumped inside saveAdvCharacter, so the autopilot's
//     own combat writes would refresh a bored character's idle clock and it
//     would never qualify again.
//   - user_stats.updated_at is bumped by any Matrix message in any room. That's
//     chat presence, not a game action — a player can talk all day without
//     touching their adventurer, and the adventurer should still get bored.
//   - loadAdvDailyActivity is unified across dnd_expedition_log, so the
//     autopilot's own walk entries read back as player activity.
//
// Any future non-Matrix entry point (web, API, Pete) should call this too — the
// clock tracks actions against Adventure, not Matrix traffic.
func markPlayerAction(userID id.UserID) {
	db.Exec("boredom: mark player action",
		`UPDATE player_meta SET last_player_action_at = ? WHERE user_id = ?`,
		time.Now().UTC(), string(userID))
}

// advActionCommands — every `!command` routed by AdventurePlugin.OnMessage.
//
// This is NOT Commands() (adventure.go): that registers 28 names for the help
// surface and is missing expedition, zone, fight, camp, extract, resume and
// more. Routing is a 47-branch if-chain, and the boredom clock has to see all
// of it, so the set is spelled out here. TestAdvActionCommandsCoverDispatch
// greps the IsCommand literals out of adventure.go and fails if one is missing,
// so a new command can't silently fall out of the clock.
var advActionCommands = map[string]bool{
	"abilities": true, "adv": true, "adventure": true, "arena": true,
	"arm": true, "attack": true, "bail": true, "camp": true,
	"cast": true, "check": true, "commune": true, "consume": true,
	"craft": true, "duel": true, "essence": true, "expedition": true,
	"explore": true, "extract": true, "fight": true, "fish": true,
	"flee": true, "forage": true, "give": true, "graveyard": true,
	"hospital": true, "level": true, "lore": true, "map": true,
	"mine": true, "news": true, "prepare": true, "region": true,
	"resources": true, "respec": true, "rest": true, "resume": true,
	"revisit": true, "rivals": true, "roll": true, "scavenge": true,
	"sell": true, "setup": true, "sheet": true, "spells": true,
	"stats": true, "subclass": true, "thom": true, "threat": true,
	"town": true, "zone": true,
}

// isPlayerAdventureAction reports whether this message is the player doing
// something to their adventurer — an Adventure command, or a reply to a prompt
// we're holding open for them (shop, blacksmith, pet, treasure). Both count:
// answering "2" to a shop menu is as much an action as typing `!shop`.
//
// This runs on every message the bot sees, in every room, so the command test
// is a single tokenise-and-look-up rather than 50 passes of IsCommand over the
// body. It matches util.IsCommand's rule exactly: trimmed, lowercased, the
// command is the whole body or is followed by a space.
//
// The pending-prompt check honours ExpiresAt. Nothing sweeps p.pending, so an
// abandoned prompt (offered, never answered) sits there forever — without the
// expiry test, every idle remark that player ever makes in any room would read
// as tending their adventurer, and the clock would never run down.
func (p *AdventurePlugin) isPlayerAdventureAction(ctx MessageContext) bool {
	body := strings.ToLower(strings.TrimSpace(ctx.Body))
	if strings.HasPrefix(body, p.Prefix) {
		word := strings.TrimPrefix(body, p.Prefix)
		if i := strings.IndexByte(word, ' '); i >= 0 {
			word = word[:i]
		}
		if advActionCommands[word] {
			return true
		}
	}
	if v, ok := p.pending.Load(string(ctx.Sender)); ok {
		pi, isPrompt := v.(*advPendingInteraction)
		// A zero ExpiresAt is a prompt with no deadline, not an expired one.
		if isPrompt && (pi.ExpiresAt.IsZero() || time.Now().Before(pi.ExpiresAt)) {
			return true
		}
	}
	return false
}

// ── The ticker ───────────────────────────────────────────────────────────────

func (p *AdventurePlugin) expeditionBoredomTicker() {
	ticker := time.NewTicker(boredomTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.fireExpeditionBoredom(time.Now().UTC())
		}
	}
}

// fireExpeditionBoredom sends every eligible idle adventurer into a dungeon.
func (p *AdventurePlugin) fireExpeditionBoredom(now time.Time) {
	// Same politeness gate as the ambient ticker: don't drop a departure DM on
	// top of the 06:00 briefing or 21:00 recap.
	if inAmbientQuietWindow(now) {
		return
	}
	uids, err := loadBoredomCandidates(now)
	if err != nil {
		slog.Error("boredom: load candidates", "err", err)
		return
	}
	for _, uid := range uids {
		if err := p.tryBoredomStart(uid, now); err != nil {
			slog.Warn("boredom: start failed", "user", uid, "err", err)
		}
	}
}

// loadBoredomCandidates returns players whose last action against Adventure is
// older than boredomIdleThreshold and who haven't been sent out inside the
// cooldown.
//
// COALESCE onto created_at so a character that was made and then never played
// still qualifies — but not on its very first tick, since created_at is the
// clock until they touch anything.
func loadBoredomCandidates(now time.Time) ([]id.UserID, error) {
	idleCutoff := now.Add(-boredomIdleThreshold)
	coolCutoff := now.Add(-boredomCooldown)
	rows, err := db.Get().Query(`
		SELECT user_id
		  FROM player_meta
		 WHERE alive = 1
		   AND COALESCE(last_player_action_at, created_at) IS NOT NULL
		   AND COALESCE(last_player_action_at, created_at) < ?
		   AND (last_boredom_at IS NULL OR last_boredom_at < ?)`,
		idleCutoff, coolCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []id.UserID
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, id.UserID(s))
	}
	return out, rows.Err()
}

// tryBoredomStart runs the eligibility guards for one player and, if they pass,
// outfits and launches the expedition.
func (p *AdventurePlugin) tryBoredomStart(uid id.UserID, now time.Time) error {
	// The foreground commands mutate the same rows under this lock; a boredom
	// start racing `!expedition start` would double-book the character.
	userMu := p.advUserLock(uid)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(uid)
	if err != nil {
		return err
	}
	if c == nil || c.PendingSetup {
		return nil // never finished character creation — nothing to get bored
	}

	// Structural blockers. Every one of these is a state the player has to
	// resolve themselves; the adventurer can't just walk out of it. Mirrors the
	// guard set in expeditionCmdStart.
	if restingLockoutRemaining(c) > 0 {
		return nil
	}
	if hasActiveCombatSession(uid) {
		return nil
	}
	if seated, _ := seatedExpeditionFor(uid); seated != nil {
		return nil // riding someone else's party
	}
	if active, _ := getActiveExpedition(uid); active != nil {
		return nil
	}
	if pending, _ := getResumableExpedition(uid); pending != nil {
		return nil // extracted, holding a roster for `!resume` — theirs to settle
	}
	if run, _ := getActiveZoneRun(uid); run != nil {
		return nil
	}

	zone, ok := pickBoredomZone(uid, c.Level)
	if !ok {
		return nil // no zone this character's level band can be sent to
	}

	// Claim the cooldown BEFORE spending anything. If the start then fails, or
	// the process dies mid-flight, we wait out the cooldown instead of looping
	// on a broken character every 30 minutes.
	res, err := db.Get().Exec(`
		UPDATE player_meta
		   SET last_boredom_at = ?
		 WHERE user_id = ?
		   AND (last_boredom_at IS NULL OR last_boredom_at < ?)`,
		now, string(uid), now.Add(-boredomCooldown))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // another tick claimed it
	}

	// Cheapest pack that exists. A zero-supply start is impossible —
	// SupplyPurchase.Validate rejects it — so a bored run always costs coins,
	// and a broke adventurer simply doesn't go. That's the floor that stops an
	// abandoned character looping forever.
	purchase := loadoutPurchase(zone.Tier, LoadoutLean)
	cost := float64(purchase.Cost())
	if p.euro == nil {
		return nil
	}
	if balance := p.euro.GetBalance(uid); balance < cost {
		return p.SendDM(uid, fmt.Sprintf(
			"🪙 I got as far as the supply counter. Outfitting for **%s** runs **%d** coins and we have **%.0f**. So I've put the pack back and come home. Nothing to be done about it from where I'm standing.",
			zone.Display, int(cost), balance))
	}

	exp, supplies, _, err := p.beginExpedition(uid, c.Level, zone, purchase, "expedition outfitting (restless)")
	if err != nil {
		return err
	}

	// Mark it as self-started. The idle reaper reads this so an expedition the
	// player never asked for can't hold their streak or spare them the shame DM
	// (gogobee_boredom_plan.md §6).
	db.Exec("boredom: flag expedition",
		`UPDATE dnd_expedition SET boredom = 1 WHERE expedition_id = ?`, exp.ID)

	slog.Info("boredom: adventurer left on its own",
		"user", uid, "zone", zone.ID, "level", c.Level, "cost", int(cost))
	return p.SendDM(uid, renderBoredomDepartureDM(zone, supplies, purchase))
}

// ── Reading the idle clock ───────────────────────────────────────────────────

// playerIsIdle reports whether the player has gone boredomIdleThreshold without
// an action against Adventure. A player with no recorded action at all (never
// played, or predates the column) counts as idle.
// The COALESCE is done in Go, not SQL, on purpose. modernc.org/sqlite rebuilds
// a time.Time from the column's declared type, and COALESCE() erases it — the
// value comes back a string and the Scan fails. Selecting the two declared
// DATETIME columns and folding them here keeps the affinity. This function
// fails open (an unreadable row reads as idle), so a broken scan here would
// declare every player idle and march the whole server into a dungeon at once.
func playerIsIdle(userID id.UserID, now time.Time) bool {
	var lastAction, created *time.Time
	err := db.Get().QueryRow(
		`SELECT last_player_action_at, created_at FROM player_meta WHERE user_id = ?`,
		string(userID)).Scan(&lastAction, &created)
	if err != nil {
		return true
	}
	last := lastAction
	if last == nil {
		last = created // never acted — the clock runs from when they were made
	}
	if last == nil {
		return true
	}
	return last.Before(now.Add(-boredomIdleThreshold))
}

// isBoredomDriven reports that this character is running on its own and its
// player has not come back: the last expedition they set out on was one the
// adventurer started for itself, and they still haven't touched Adventure.
//
// This is the gate the idle reaper uses. It's deliberately narrower than "is
// the player idle": an expedition the player *did* start still holds their
// streak while the autopilot walks it, exactly as it always has. Only an
// expedition nobody asked for loses that protection.
//
// Reading the most recent expedition rather than the active one keeps it honest
// for the night after a bored run ends — the logs it left behind would
// otherwise read as player activity for another day.
func isBoredomDriven(userID id.UserID, now time.Time) bool {
	if !playerIsIdle(userID, now) {
		return false // they came back; nothing to reap
	}
	var boredom bool
	err := db.Get().QueryRow(`
		SELECT boredom
		  FROM dnd_expedition
		 WHERE user_id = ?
		 ORDER BY start_date DESC
		 LIMIT 1`, string(userID)).Scan(&boredom)
	if err != nil {
		return false // no expedition history, or unreadable — don't reap on a guess
	}
	return boredom
}

// ── Zone selection ───────────────────────────────────────────────────────────

// pickBoredomZone chooses where a restless adventurer goes.
//
// It uses the LevelMin/LevelMax bands on ZoneDefinition rather than
// zonesForLevel, which filters on tier alone and ignores those fields — which
// is why a level-1 player can currently walk into a T3 zone. A bored adventurer
// should neither farm trivial content nor wander somewhere absurd, so the band
// is the right gate.
//
// Order of preference:
//  1. Zones whose level band contains the character.
//  2. Among those, the non-raid ones (raidContentWarning) — the party-tuned T5
//     bosses have a 0% solo clear rate, so they're avoided while an alternative
//     exists.
//  3. Among those, the lowest tier: the "easiest at-level" rule.
//  4. Ties break on least-recently-visited, so a bored adventurer rotates zones
//     instead of grinding one forever.
//
// If step 2 leaves nothing, raid zones are allowed back in. That only bites at
// L13+, where dragons_lair and abyss_portal are the only zones in band and both
// are raids. Those runs cannot be won solo — they wall at the boss-safety gate,
// starve, and force-extract with the haul tax. That is the intended fate of an
// abandoned endgame character, and it drains the coins that fund the next one.
func pickBoredomZone(uid id.UserID, level int) (ZoneDefinition, bool) {
	var inBand, nonRaid []ZoneDefinition
	for _, z := range allZones() {
		if level < z.LevelMin || level > z.LevelMax {
			continue
		}
		inBand = append(inBand, z)
		if raidContentWarning(z.ID) == "" {
			nonRaid = append(nonRaid, z)
		}
	}
	pool := nonRaid
	if len(pool) == 0 {
		pool = inBand
	}
	if len(pool) == 0 {
		return ZoneDefinition{}, false
	}

	lastVisit, err := lastExpeditionByZone(uid)
	if err != nil {
		slog.Warn("boredom: zone history unavailable, falling back to tier order", "user", uid, "err", err)
		lastVisit = map[ZoneID]time.Time{}
	}
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].Tier != pool[j].Tier {
			return pool[i].Tier < pool[j].Tier
		}
		return lastVisit[pool[i].ID].Before(lastVisit[pool[j].ID])
	})
	return pool[0], true
}

// lastExpeditionByZone returns when this player last set out for each zone.
// Zones they've never visited are absent, so they sort first (zero time).
//
// No MAX(start_date)/GROUP BY: the aggregate erases the column's declared type
// and modernc.org/sqlite hands back a string the Scan can't take. Walking the
// rows in ascending order and letting each overwrite the last gives the same
// answer with the affinity intact.
func lastExpeditionByZone(uid id.UserID) (map[ZoneID]time.Time, error) {
	rows, err := db.Get().Query(`
		SELECT zone_id, start_date
		  FROM dnd_expedition
		 WHERE user_id = ?
		 ORDER BY start_date ASC`, string(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ZoneID]time.Time{}
	for rows.Next() {
		var zone string
		var last time.Time
		if err := rows.Scan(&zone, &last); err != nil {
			return nil, err
		}
		out[ZoneID(zone)] = last // ascending, so the last write is the most recent
	}
	return out, rows.Err()
}

// ── Rendering ────────────────────────────────────────────────────────────────

// renderBoredomDepartureDM is the note left on the table. It states plainly
// that nothing was bought and nothing was upgraded — the player should be able
// to read this and know exactly why the haul is going to be bad.
func renderBoredomDepartureDM(zone ZoneDefinition, supplies ExpeditionSupplies, purchase SupplyPurchase) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎒 **Gone on ahead — %s** _(T%d)_\n\n", zone.Display, int(zone.Tier)))
	b.WriteString("_" + zone.Hook + "_\n\n")
	b.WriteString(flavor.Pick(flavor.ExpeditionBoredomStart))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("**Packed:** %.0f SU (%d standard) — %d coins, the cheapest that would get us through the gate\n",
		supplies.Max, purchase.StandardPacks, purchase.Cost()))
	b.WriteString(fmt.Sprintf("**Daily burn:** %.1f SU/day — roughly **%d days** before we're rationing\n",
		supplies.DailyBurn, estimateDays(supplies.Max, supplies.DailyBurn)))
	b.WriteString("**Equipment:** unchanged. I don't do the shopping.\n\n")
	if w := raidContentWarning(zone.ID); w != "" {
		b.WriteString(w)
		b.WriteString("\n\n")
	}
	b.WriteString("`!expedition status` to see where we've got to. Come and find us.")
	return b.String()
}
