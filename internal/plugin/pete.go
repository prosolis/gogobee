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
	// Author the prose in Pete's voice from the FINAL fact, so the names in the
	// dispatch match the Actors allow-list Pete guards against. Best-effort: an
	// empty pair (LLM off or authoring failed) just means Pete templates the
	// fact. Synchronous, like the holdem tip rewrite — news facts are infrequent
	// and the call is tightly bounded (dispatchLLMTimeout).
	f.Headline, f.Lede = authorDispatch(f)
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
// realm's first clear of a zone is a "zone_first"; later clears are a
// "zone_clear". The event_type and the GUID prefix track which, so Pete
// templates and labels a repeat as a repeat, not a first-ever. Character name
// only; no-op unless the seam is enabled.
//
// BULLETIN either way: TwinBee announces the clear in the room as it happens,
// and priority tier is what makes Pete post a live beat — so a priority
// zone_first would just echo TwinBee back at the same people. Bulletin still
// gets the story onto the site and into the daily digest, where it reads as a
// roundup rather than a repeat.
// emitBoredomDeparture announces that an adventurer got restless and let itself
// out. The one thing gogobee never used to tell Pete was that an expedition
// *started* — every dispatch was an outcome — which is why the two live boredom
// runs produced no news at all.
//
// The event_type no longer has to be one Pete already knows. It used to: an
// unknown type was a 400, which retried to the cap and then parked the bulletin
// forever, so shipping a new event type meant remembering to deploy Pete first.
// Pete now publishes an untemplated type on a neutral fallback and counts it for
// the operator, so the ordering rule is a property of the system rather than
// something a human has to hold.
func emitBoredomDeparture(userID id.UserID, zone ZoneDefinition, level int) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	name := charName(userID)
	if name == "" {
		return
	}
	ts := nowUnix()
	disc := fmt.Sprintf("departure:%s:%d", zone.ID, ts)
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("departure:%s:%s:%d", eventToken(userID, disc), zone.ID, ts),
		EventType:  "departure",
		Tier:       "bulletin",
		Subject:    name,
		Zone:       zone.Display,
		Level:      level,
		Outcome:    "departed",
		OccurredAt: ts,
	}, userID, "")
}

func emitZoneClearNews(userID id.UserID, exp *Expedition) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	// Claim the realm-first BEFORE the name guard, so an unnamed straggler's
	// genuine first clear still seeds news_realm_firsts. Otherwise the next
	// named clearer would claim it and be mis-announced as the first-ever.
	// Mirrors backfillZoneFirsts, which claims before its own name check.
	eventType := "zone_clear"
	if claimRealmFirst("zone", string(exp.ZoneID)) {
		eventType = "zone_first"
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
	ts := nowUnix()
	disc := fmt.Sprintf("%s:%d", exp.ZoneID, ts)
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("%s:%s:%s:%d", eventType, eventToken(userID, disc), exp.ZoneID, ts),
		EventType:  eventType,
		Tier:       "bulletin",
		Subject:    name,
		Zone:       zone.Display,
		Region:     region,
		Boss:       zone.Boss.Name,
		Level:      lvl,
		Outcome:    "cleared",
		RunID:      latestRunIDForNews(userID),
		OccurredAt: ts,
	}, userID, "")
}

// treasureRarityWord maps a treasure def's tier to the rarity adjective Pete
// weaves into a find's dispatch. Story-grade treasures are typically tier 5, so
// "legendary" is the common case; the lower tiers are here for completeness.
func treasureRarityWord(tier int) string {
	switch {
	case tier >= 5:
		return "legendary"
	case tier == 4:
		return "epic"
	case tier == 3:
		return "rare"
	case tier == 2:
		return "uncommon"
	default:
		return "common"
	}
}

// emitTreasureFound files a story-grade treasure find. Only treasures carrying a
// RoomAnnounce string reach here — the same gate that earns them a public moment
// — so a copper-piece pickup never becomes news. The realm's first finder of a
// given treasure is a PRIORITY hoard; a later finder of the same item is a
// BULLETIN, the first/repeat split zone_first already uses. Character name only;
// no-op unless the seam is enabled.
//
// treasure_found is an event_type Pete's ingest must already know: an unknown
// type 400s, retries to the cap, then parks the bulletin forever. Deploy Pete
// first.
func emitTreasureFound(userID id.UserID, def *AdvTreasureDef, loc *AdvLocation) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	if def == nil || loc == nil {
		return
	}
	// Claim the realm-first BEFORE the name guard, so an unnamed straggler's
	// genuine first find still seeds the ledger and the next finder isn't
	// mis-billed as the first-ever. Mirrors emitZoneClearNews.
	tier := "bulletin"
	if claimRealmFirst("treasure", def.Key) {
		tier = "priority"
	}
	name := charName(userID)
	if name == "" {
		return
	}
	ts := nowUnix()
	disc := fmt.Sprintf("%s:%d", def.Key, ts)
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("treasure_found:%s:%s:%d", eventToken(userID, disc), def.Key, ts),
		EventType:  "treasure_found",
		Tier:       tier,
		Subject:    name,
		Zone:       loc.Name,
		Level:      charLevel(userID),
		Stakes:     def.Name,
		Outcome:    treasureRarityWord(def.Tier),
		OccurredAt: ts,
	}, userID, "")
}
