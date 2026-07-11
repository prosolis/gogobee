package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// This file is gogobee's game-side entry to Pete's adventure news. Plugins call
// emitFact at event chokepoints; it enforces the kill-switch + per-player
// opt-out, then hands a durable fact to peteclient. gogobee supplies facts only
// — Pete owns the voice. See pete_adventure_news_plan.md / _voice.md.

const (
	newsEnabledKey = "adventure_news_enabled"
	// foreverTTL makes CacheGet a persistent flag: the row never "expires".
	foreverTTL = 100 * 365 * 24 * 3600
	anonName   = "an adventurer"
)

// newsEmissionOn is the runtime kill-switch (default on). An operator flips it
// with `!news off` without a redeploy; FEATURE_PETE_NEWS is the source-level
// master switch checked separately by peteclient.Enabled.
func newsEmissionOn() bool {
	return db.CacheGet(newsEnabledKey, foreverTTL) != "0"
}

// setNewsEmission persists the runtime kill-switch.
func setNewsEmission(on bool) {
	v := "1"
	if !on {
		v = "0"
	}
	db.CacheSet(newsEnabledKey, v)
}

// isNewsOptedOut reports whether a player asked not to be named in the news.
func isNewsOptedOut(userID id.UserID) bool {
	var one int
	err := db.Get().QueryRow(`SELECT 1 FROM news_optout WHERE user_id = ?`, string(userID)).Scan(&one)
	return err == nil
}

// setNewsOptout records or clears a player's opt-out. Opting out anonymizes
// their name in future dispatches ("an adventurer…"); it does not stop events
// being reported. Idempotent either direction.
func setNewsOptout(userID id.UserID, out bool) {
	if out {
		db.Exec("news optout set",
			`INSERT OR IGNORE INTO news_optout (user_id, opted_out_at) VALUES (?, unixepoch())`,
			string(userID))
		return
	}
	db.Exec("news optout clear", `DELETE FROM news_optout WHERE user_id = ?`, string(userID))
}

// userHash is a stable, one-way digest of a Matrix user id. It keys per-player
// event GUIDs (which become PUBLIC permalink paths on Pete) WITHOUT leaking the
// Matrix handle. Not reversible, stable across restarts.
func userHash(userID id.UserID) string {
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:6]) // 12 hex chars — ample for 4–thousands of players
}

// charName returns a player's in-game character name, never their Matrix handle.
// Empty when unknown — callers skip the emit rather than fall back to a handle.
func charName(userID id.UserID) string {
	name, _ := loadDisplayName(userID)
	return strings.TrimSpace(name)
}

// classRaceLabel renders a character's race + class for the arrival fact, e.g.
// "Elf Ranger".
func classRaceLabel(c *DnDCharacter) string {
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	return strings.TrimSpace(ri.Display + " " + ci.Display)
}

// emitFact is the single game-side entry to Pete's news. It enforces the runtime
// kill-switch and per-player opt-out (anonymize the name, never drop the event),
// derives the actors allow-list from the FINAL names (so Pete's fact-guard and
// the rendered content agree), then durably queues the fact.
//
// subjectUser/opponentUser are the Matrix users behind Subject/Opponent (either
// may be empty for realm-level events). They are used ONLY for the opt-out
// lookup and are never sent.
func emitFact(f peteclient.Fact, subjectUser, opponentUser id.UserID) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	if subjectUser != "" && isNewsOptedOut(subjectUser) {
		f.Subject = anonName
	}
	if opponentUser != "" && isNewsOptedOut(opponentUser) {
		f.Opponent = anonName
	}
	var actors []string
	for _, n := range []string{f.Subject, f.Opponent} {
		if n != "" {
			actors = append(actors, n)
		}
	}
	f.Actors = actors
	peteclient.Emit(f)
}

// handleNewsCmd backs `!news`. Players toggle whether they're named in Pete's
// dispatches; admins flip the realm-wide emission kill-switch. Replies in-room
// so the effect is visible where it was invoked.
//
//	!news            → show your status + what the command does
//	!news optout     → anonymize your name in future dispatches
//	!news optin      → be named again
//	!news on|off     → (admin) master kill-switch for all emission
func (p *AdventurePlugin) handleNewsCmd(ctx MessageContext) error {
	arg := strings.ToLower(strings.TrimSpace(p.GetArgs(ctx.Body, "news")))
	switch arg {
	case "optout", "opt-out", "anonymize", "anon":
		setNewsOptout(ctx.Sender, true)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Done — Pete will refer to you as \"an adventurer\" in the news from now on. `!news optin` to be named again.")
	case "optin", "opt-in":
		setNewsOptout(ctx.Sender, false)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"You're back on the record — Pete will use your character name in future dispatches.")
	case "on", "enable":
		if !p.IsAdmin(ctx.Sender) {
			return nil
		}
		setNewsEmission(true)
		return p.SendReply(ctx.RoomID, ctx.EventID, "📣 Adventure news emission is now ON.")
	case "off", "disable":
		if !p.IsAdmin(ctx.Sender) {
			return nil
		}
		setNewsEmission(false)
		return p.SendReply(ctx.RoomID, ctx.EventID, "🔇 Adventure news emission is now OFF. No new dispatches will be sent.")
	case "", "status", "help":
		named := "using your character name"
		if isNewsOptedOut(ctx.Sender) {
			named = "anonymized (\"an adventurer\")"
		}
		msg := "📰 **Pete's Adventure News** reports realm happenings — deaths, first-clears, arrivals, duels.\n" +
			"You are currently " + named + ".\n" +
			"`!news optout` to be anonymized · `!news optin` to be named again."
		if p.IsAdmin(ctx.Sender) {
			state := "ON"
			if !newsEmissionOn() {
				state = "OFF"
			}
			msg += "\n_(admin)_ emission is **" + state + "** · `!news on` / `!news off` to toggle."
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, msg)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Unknown option. Try `!news`, `!news optout`, or `!news optin`.")
	}
}

// nowUnix is the event timestamp helper (kept in one place so the emit sites
// read uniformly).
func nowUnix() int64 { return time.Now().Unix() }

// claimRealmFirst records the first occurrence of a (kind,target) across the
// realm and reports whether THIS call was the first — true exactly once. Used to
// tier realm-firsts (PRIORITY) apart from repeat clears (BULLETIN). The backfill
// pre-seeds this ledger so historical firsts aren't re-announced after a deploy.
func claimRealmFirst(kind, target string) bool {
	res := db.ExecResult("news realm-first claim",
		`INSERT OR IGNORE INTO news_realm_firsts (kind, target, first_at) VALUES (?, ?, unixepoch())`,
		kind, target)
	if res == nil {
		return false
	}
	n, err := res.RowsAffected()
	return err == nil && n > 0
}

// emitZoneClearNews files a zone-clear dispatch (boss down = zone cleared). The
// realm's first clear of a zone is PRIORITY; later clears are BULLETIN. Character
// name only; no-op unless the seam is enabled.
func emitZoneClearNews(userID id.UserID, exp *Expedition) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	name := charName(userID)
	if name == "" {
		return
	}
	zone := zoneOrFallback(exp.ZoneID)
	region := ""
	if IsMultiRegionZone(exp.ZoneID) {
		if r, ok := CurrentRegion(exp); ok {
			region = r.Name
		}
	}
	lvl := 0
	if dc, _ := LoadDnDCharacter(userID); dc != nil {
		lvl = dc.Level
	}
	tier := "bulletin"
	if claimRealmFirst("zone", string(exp.ZoneID)) {
		tier = "priority"
	}
	ts := nowUnix()
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("zone:%s:%s:%d", userHash(userID), exp.ZoneID, ts),
		EventType:  "zone_first",
		Tier:       tier,
		Subject:    name,
		Zone:       zone.Display,
		Region:     region,
		Boss:       zone.Boss.Name,
		Level:      lvl,
		Outcome:    "cleared",
		OccurredAt: ts,
	}, userID, "")
}
