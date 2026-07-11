package plugin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
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
	newsEnabledKey  = "adventure_news_enabled"
	newsGUIDSaltKey = "adventure_news_guid_salt"
	anonName        = "an adventurer"
)

// newsEmissionOn is the runtime kill-switch (default on). An operator flips it
// with `!news off` without a redeploy; FEATURE_PETE_NEWS is the source-level
// master switch checked separately by peteclient.Enabled. Persisted in the
// durable news_config table (NOT api_cache, which RunMaintenance prunes).
func newsEmissionOn() bool {
	var v string
	err := db.Get().QueryRow(`SELECT value FROM news_config WHERE key = ?`, newsEnabledKey).Scan(&v)
	if err != nil {
		return true // no row → default on
	}
	return v != "0"
}

// setNewsEmission persists the runtime kill-switch.
func setNewsEmission(on bool) {
	v := "1"
	if !on {
		v = "0"
	}
	db.Exec("news emission set",
		`INSERT INTO news_config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		newsEnabledKey, v)
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

var (
	guidSaltOnce sync.Once
	guidSalt     []byte
)

// newsGUIDSalt returns the secret, server-side salt that keys every public event
// token. It is 32 random bytes, generated once and persisted in the durable
// news_config table, cached in-process. Secret from anyone outside the server
// (external users never see it), stable across restarts (so a re-emit or the
// cold-start backfill reproduces the same GUID). No operator action needed.
func newsGUIDSalt() []byte {
	guidSaltOnce.Do(func() {
		var stored string
		if err := db.Get().QueryRow(
			`SELECT value FROM news_config WHERE key = ?`, newsGUIDSaltKey).Scan(&stored); err == nil {
			if b, e := hex.DecodeString(stored); e == nil && len(b) > 0 {
				guidSalt = b
				return
			}
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			// crypto/rand should never fail; if it does, refuse to fall back to a
			// predictable salt (that would reopen the enumeration attack). Leave the
			// salt empty and let eventToken sort it out.
			return
		}
		salt := hex.EncodeToString(b)
		// Persist. OR IGNORE so a concurrent first-writer wins cleanly; we then
		// re-read the row so every caller agrees on the same salt.
		db.Exec("news guid salt seed",
			`INSERT OR IGNORE INTO news_config (key, value) VALUES (?, ?)`, newsGUIDSaltKey, salt)
		_ = db.Get().QueryRow(`SELECT value FROM news_config WHERE key = ?`, newsGUIDSaltKey).Scan(&salt)
		guidSalt, _ = hex.DecodeString(salt)
	})
	return guidSalt
}

// eventToken is the per-event, one-way identifier embedded in a public GUID
// (which becomes a PUBLIC permalink path on Pete). It is HMAC-SHA256 over the
// Matrix user id AND an event-specific discriminator, keyed by the secret
// newsGUIDSalt.
//
// Two properties matter and both come from HMAC being a PRF under a secret key:
//   - Not computable from a Matrix handle. An attacker who knows @alice:server
//     cannot derive any of her tokens without the salt — so her events (including
//     ones anonymized after `!news optout`) can't be enumerated from her handle.
//   - Per-event, so tokens are mutually unlinkable. Even for the same user, two
//     events yield unrelated tokens; a story that names her (opted-in, or a duel
//     opponent) exposes only that one event's token and can't be walked back to
//     her other, anonymized events.
//
// The discriminator must uniquely identify the logical event so a re-emit or the
// backfill reproduces the same token (idempotency rides on the GUID).
func eventToken(userID id.UserID, discriminator string) string {
	mac := hmac.New(sha256.New, newsGUIDSalt())
	mac.Write([]byte(userID))
	mac.Write([]byte{0}) // domain separator: keep id and discriminator from bleeding
	mac.Write([]byte(discriminator))
	return hex.EncodeToString(mac.Sum(nil)[:9]) // 18 hex chars — ample, non-reversible
}

// charName returns a player's in-game character name, never their Matrix handle.
// Empty when unknown — callers skip the emit rather than fall back to a handle.
func charName(userID id.UserID) string {
	name, _ := loadDisplayName(userID)
	return strings.TrimSpace(name)
}

// charLevel returns a player's current character level for a news Fact, or 0 if
// no character loads (Level is omitempty, so 0 is dropped from the payload).
func charLevel(userID id.UserID) int {
	if dc, _ := LoadDnDCharacter(userID); dc != nil {
		return dc.Level
	}
	return 0
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
// realm's first clear of a zone is a PRIORITY "zone_first"; later clears are a
// BULLETIN "zone_clear". The event_type and the GUID prefix track the tier so
// Pete templates and labels a repeat as a repeat, not a first-ever. Character
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
	lvl := charLevel(userID)
	eventType, tier := "zone_clear", "bulletin"
	if claimRealmFirst("zone", string(exp.ZoneID)) {
		eventType, tier = "zone_first", "priority"
	}
	ts := nowUnix()
	disc := fmt.Sprintf("%s:%d", exp.ZoneID, ts)
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("%s:%s:%s:%d", eventType, eventToken(userID, disc), exp.ZoneID, ts),
		EventType:  eventType,
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
