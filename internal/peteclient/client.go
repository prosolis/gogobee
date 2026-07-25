// Package peteclient is gogobee's outbound seam to the Pete news bot.
//
// gogobee is the source of game-event *facts* and owns delivery; Pete owns
// voice, authoring, and publishing. This package carries structured facts (not
// prose) to Pete's ingest endpoint over the tailnet, bearer-authed.
//
// Delivery is durable: Emit writes the fact to a SQLite queue and returns
// immediately, so a game-loop hook never blocks on the network and a Pete
// restart loses nothing. A background sender drains the queue with retry.
// Idempotency is on the fact GUID, so retries and duplicate emits are no-ops.
package peteclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"
)

// Fact is the flat, pre-sanitized payload gogobee POSTs to Pete. Names must be
// character names only (never Matrix handles); Actors is the allow-list of the
// only names permitted to appear in Pete's rendered output. See
// pete_adventure_news_voice.md for the field contract.
type Fact struct {
	GUID       string   `json:"guid"` // stable idempotency key, e.g. "death:<token>:<ts>"; prefix == event_type
	EventType  string   `json:"event_type"`
	Tier       string   `json:"tier"` // "priority" | "bulletin"
	Actors     []string `json:"actors"`
	Subject    string   `json:"subject,omitempty"`
	Opponent   string   `json:"opponent,omitempty"`
	Boss       string   `json:"boss,omitempty"`
	Zone       string   `json:"zone,omitempty"`
	Region     string   `json:"region,omitempty"`
	Level      int      `json:"level,omitempty"`
	Count      int      `json:"count,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
	Stakes     string   `json:"stakes,omitempty"`
	ClassRace  string   `json:"class_race,omitempty"`
	Milestone  string   `json:"milestone,omitempty"`
	OccurredAt int64    `json:"occurred_at"`
	NoPush     bool     `json:"no_push,omitempty"` // backfill: suppress Pete web-push
	// RunID names the expedition this fact is the ENDING of, and only the three
	// facts that are one carry it: a clear, a retreat, a death. It is what lets
	// Pete's dispatch link back to the run's own report — the log, the numbers,
	// the moment it turned — instead of leaving a paragraph about an outcome with
	// no way back to what produced it. Empty everywhere else, and safe to be
	// empty: Pete renders the dispatch exactly as it did before the report existed.
	RunID string `json:"run_id,omitempty"`
	// Headline/Lede are LLM-authored prose for this fact, both optional. Pete
	// prefers them over its own template when present and past its prose-guard,
	// and falls back to the template otherwise — so an empty pair (LLM off, or
	// authoring failed) is the normal, safe case. Populated by emitFact; see
	// authorDispatch. Names in the prose must come only from Actors.
	Headline string `json:"headline,omitempty"`
	Lede     string `json:"lede,omitempty"`
}

// Config controls the seam. Enabled=false makes Emit a durable no-op (nothing
// queued), matching the FEATURE_PETE_NEWS master switch that kills emission at
// the source.
type Config struct {
	IngestURL string
	Token     string
	Enabled   bool
}

// Client is the transport half. It is a package singleton initialized by Init,
// so emit hooks scattered across plugins (and free functions like
// markAdventureDead) can call Emit without threading a handle through.
type Client struct {
	cfg      Config
	http     *http.Client
	draining sync.Mutex // one drain at a time; see drain
}

var std *Client

// factPath is where an adventure fact goes. Every queue row carries its own
// destination now, because escrow verdicts ride the same queue to a different
// endpoint.
const factPath = "/api/ingest/adventure"

// Tuning for the background sender.
const (
	senderTick    = 15 * time.Second
	senderBatch   = 20
	maxAttempts   = 8 // ~ up to a few hours of backoff, then park
	backoffBase   = 30 * time.Second
	backoffCapSec = 3600
	sendTimeout   = 15 * time.Second
)

// Init wires the singleton from the environment. Mirrors the per-plugin config
// pattern (email_nag.go): PETE_INGEST_URL, PETE_INGEST_TOKEN, FEATURE_PETE_NEWS.
func Init() {
	cfg := Config{
		IngestURL: strings.TrimRight(os.Getenv("PETE_INGEST_URL"), "/"),
		Token:     os.Getenv("PETE_INGEST_TOKEN"),
		Enabled:   strings.EqualFold(os.Getenv("FEATURE_PETE_NEWS"), "true"),
	}
	if cfg.Enabled && (cfg.IngestURL == "" || cfg.Token == "") {
		slog.Warn("peteclient: FEATURE_PETE_NEWS=true but PETE_INGEST_URL/PETE_INGEST_TOKEN unset — disabling")
		cfg.Enabled = false
	}
	std = &Client{cfg: cfg, http: &http.Client{Timeout: sendTimeout}}
	if cfg.Enabled {
		slog.Info("peteclient: adventure news emission enabled", "ingest", cfg.IngestURL)
	} else {
		slog.Info("peteclient: adventure news emission disabled (set FEATURE_PETE_NEWS=true)")
	}
}

// Enabled reports whether emission is on. Callers can skip building an
// (expensive) fact when it would be dropped anyway.
func Enabled() bool { return std != nil && std.cfg.Enabled }

// Emit durably queues a fact for delivery to Pete. It never blocks on the
// network. A no-op (but safe) when the seam is disabled or the GUID was already
// queued — idempotency is on the GUID primary key.
func Emit(f Fact) {
	if !Enabled() {
		return
	}
	if f.GUID == "" {
		slog.Error("peteclient: refusing to queue fact with empty guid", "event_type", f.EventType)
		return
	}
	payload, err := json.Marshal(f)
	if err != nil {
		slog.Error("peteclient: marshal fact", "guid", f.GUID, "err", err)
		return
	}
	enqueue(f.GUID, factPath, payload)
}

// enqueue puts one payload on the durable queue, addressed to a Pete endpoint.
//
// OR IGNORE gives GUID-idempotency: a re-emit of the same key is dropped. That
// is the whole safety story for money — an escrow verdict is queued under its
// escrow guid, so a verdict can never be enqueued twice and can never be
// delivered as two different answers.
func enqueue(guid, path string, payload []byte) {
	db.Exec("pete emit enqueue",
		`INSERT OR IGNORE INTO pete_emit_queue (guid, path, payload, created_at, attempts, next_attempt_at)
		 VALUES (?, ?, ?, unixepoch(), 0, 0)`,
		guid, path, string(payload))
}

// StartSender launches the background drain loop. It runs until ctx is
// canceled. Safe to call when disabled — it simply idles.
func StartSender(ctx context.Context) {
	if std == nil {
		return
	}
	go func() {
		t := time.NewTicker(senderTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if std.cfg.Enabled {
					std.drain(ctx)
				}
			}
		}
	}()
}

// Flush drains the queue right now instead of waiting for the next tick.
//
// The escrow loop needs this. A player who clicked "buy chips" is watching a
// spinner, and a verdict that sat in the queue for a 15-second sender tick would
// make the whole border feel broken even though nothing is. Durability is not
// weakened: the row is written first and only then sent, exactly as the ticker
// does it.
func Flush(ctx context.Context) {
	if std == nil || !std.cfg.Enabled {
		return
	}
	std.drain(ctx)
}

// drain sends up to senderBatch due rows, one at a time.
//
// Serialized: the ticker and Flush can both call this, and two drains racing
// would send the same row twice. Every Pete endpoint we push to is idempotent,
// so that would be survivable rather than harmful — but it would also mean an
// escrow verdict arriving twice as a matter of routine, and "harmless in theory"
// is not how the money path should be run.
func (c *Client) drain(ctx context.Context) {
	c.draining.Lock()
	defer c.draining.Unlock()

	rows, err := db.Get().Query(
		`SELECT guid, path, payload FROM pete_emit_queue
		 WHERE sent_at IS NULL AND attempts < ? AND next_attempt_at <= unixepoch()
		 ORDER BY created_at LIMIT ?`,
		maxAttempts, senderBatch)
	if err != nil {
		slog.Error("peteclient: drain query", "err", err)
		return
	}
	type item struct{ guid, path, payload string }
	var batch []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.guid, &it.path, &it.payload); err != nil {
			slog.Error("peteclient: drain scan", "err", err)
			continue
		}
		batch = append(batch, it)
	}
	rows.Close()

	for _, it := range batch {
		if ctx.Err() != nil {
			return
		}
		if err := c.post(ctx, it.path, []byte(it.payload)); err != nil {
			if ctx.Err() != nil {
				// Shutdown canceled the in-flight send — Pete didn't reject
				// anything. Don't burn a durable retry attempt; the row is picked
				// up on the next boot's drain.
				return
			}
			db.Exec("pete emit retry",
				`UPDATE pete_emit_queue
				 SET attempts = attempts + 1, next_attempt_at = unixepoch() + ?
				 WHERE guid = ?`,
				backoffSec(it.guid), it.guid)
			slog.Warn("peteclient: emit failed, will retry", "guid", it.guid, "err", err)
			continue
		}
		db.Exec("pete emit sent",
			`UPDATE pete_emit_queue SET sent_at = unixepoch() WHERE guid = ?`, it.guid)
	}
}

// RosterEntry is one adventurer's currently-true state for Pete's live board.
// Unlike a Fact, nothing here is an event — it is what is true right now.
type RosterEntry struct {
	Token     string `json:"token"` // stable per-player board token, never a Matrix handle
	Name      string `json:"name"`  // character name only
	Level     int    `json:"level"`
	ClassRace string `json:"class_race,omitempty"`
	Status    string `json:"status"` // "expedition" | "idle"
	Zone      string `json:"zone,omitempty"`
	Region    string `json:"region,omitempty"`
	Day       int    `json:"day,omitempty"`
	IdleHours int    `json:"idle_hours,omitempty"`

	// Detail is the public, expanded sheet — stats and equipped gear — shown on
	// the click-through detail page. Nil for an entry we couldn't fully load.
	// Public-tier: no Matrix handle, keyed only by the anonymous Token, same as
	// the summary fields above.
	Detail *RosterDetail `json:"detail,omitempty"`
}

// RosterDetail is everything a visitor may see on an adventurer's detail page:
// the current combat sheet and equipped gear, plus live expedition context. It
// rides the roster push because it is small and shares the board's semantics —
// a photograph of the present, fine to drop and refresh, never a handle.
type RosterDetail struct {
	HPCurrent  int        `json:"hp_current"`
	HPMax      int        `json:"hp_max"`
	TempHP     int        `json:"temp_hp,omitempty"`
	ArmorClass int        `json:"armor_class"`
	Abilities  [6]int     `json:"abilities"` // STR, DEX, CON, INT, WIS, CHA scores
	Modifiers  [6]int     `json:"modifiers"` // matching ability modifiers
	Gear       []GearItem `json:"gear,omitempty"`
	// Expedition context, present only while on a run.
	Supplies    int        `json:"supplies,omitempty"`
	ThreatLevel int        `json:"threat_level,omitempty"`
	Room        string     `json:"room,omitempty"`
	Map         *RosterMap `json:"map,omitempty"`
}

// RosterMap is the fog-of-war cut of an adventurer's zone graph: every node
// they have visited, plus the one-hop frontier of doors leading out of visited
// nodes, with the rooms behind those doors withheld. It is per-adventurer and
// rides the roster push beside Room. Only ids and kinds cross the wire — a
// ZoneNode's Label and Content (encounter, loot bias, narration) are spoilers
// and never leave the game box. Frontier nodes carry kind "unknown".
type RosterMap struct {
	ZoneID      string          `json:"zone_id"`
	CurrentNode string          `json:"current_node"`
	Visited     []string        `json:"visited"`
	Nodes       []RosterMapNode `json:"nodes"`
	Edges       []RosterMapEdge `json:"edges"`
}

// RosterMapNode is one room reduced to what a public map may show.
type RosterMapNode struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // ZoneNodeKind, or "unknown" for an unreached frontier room
}

// RosterMapEdge is one directed passage. Lock names the gate kind
// (perception_check, key_required, ...) so the map can mark a door as barred;
// LockData and Hint stay behind on the game box.
type RosterMapEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Lock string `json:"lock,omitempty"`
}

// GearItem is one equipped piece for the armor/gear panel.
type GearItem struct {
	Slot       string `json:"slot"` // weapon | armor | helmet | boots | tool
	Name       string `json:"name"`
	Tier       int    `json:"tier"`
	Condition  int    `json:"condition"`
	Masterwork bool   `json:"masterwork,omitempty"`
}

// MischiefBalance is one buyer's advisory euro balance, ridden along with the
// board. It is keyed by localpart — a buyer's own sign-in name — not by the
// anonymous roster token, so it lives in a separate keyspace on Pete and only
// ever surfaces for the one authenticated user asking about themselves. That is
// what lets the storefront grey out tiers a buyer can't afford without ever
// putting a number next to a name on the public board.
type MischiefBalance struct {
	Username string  `json:"username"`
	Euro     float64 `json:"euro"`
}

// MischiefTier is one rung of the storefront price list. gogobee is the sole
// authority on prices, so it pushes the whole catalog on every tick: a fee
// retune reaches the storefront within a snapshot and Pete never hardcodes a
// number that can silently drift out of step with the game.
type MischiefTier struct {
	Key       string `json:"key"`
	Display   string `json:"display"`
	Fee       int    `json:"fee"`
	SignedFee int    `json:"signed_fee"`
	Blurb     string `json:"blurb,omitempty"`
}

// RosterSnapshot is the complete board. Complete is load-bearing: Pete replaces
// its whole board with this, so anyone omitted (opted out, no character) drops
// off the public page. A partial snapshot would silently strand people on it.
//
// Balances and Tiers ride the same tick — advisory affordability and the live
// price list for the mischief storefront. Both are best-effort on Pete's side; a
// board that lands without them is still a good board.
type RosterSnapshot struct {
	SnapshotAt  int64             `json:"snapshot_at"`
	Adventurers []RosterEntry     `json:"adventurers"`
	Balances    []MischiefBalance `json:"balances,omitempty"`
	Tiers       []MischiefTier    `json:"tiers,omitempty"`
}

// PushRoster sends the board to Pete, synchronously, and drops it on failure.
//
// Deliberately NOT on the durable queue that carries Facts. A fact is history —
// losing "Josie died" loses it forever, so it retries. A snapshot is a
// photograph of the present, and a retried one is a *lie*: by the time it lands,
// Josie has moved. The next tick carries the truth anyway, so a failed push is
// simply forgotten. That is also what lets Pete's staleness timer work — if we
// stay down, nothing arrives, and the board correctly stops claiming to be live.
func PushRoster(ctx context.Context, snap RosterSnapshot) error {
	if !Enabled() {
		return nil
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/ingest/roster", payload)
}

// SiegeDefender is one adventurer's standing in the current Siege muster.
//
// Token is the same public board token the roster uses, so Pete can link a
// defender to their page — and it is EMPTY for an opted-out player, with Name
// carrying anonName instead. That is the whole opt-out story here: their damage
// still counts and still holds its rank (the town's effort is the town's), but
// there is no name to click. It differs from the board's rule (omit entirely)
// on purpose — a defender board that silently dropped contributors would
// understate what the town actually did to the boss.
type SiegeDefender struct {
	Token       string `json:"token,omitempty"`
	Name        string `json:"name"`
	Level       int    `json:"level,omitempty"`
	Fights      int    `json:"fights"`
	Damage      int    `json:"damage"`
	FoughtToday bool   `json:"fought_today"`
}

// SiegePast is one closed-out Siege for the history table.
type SiegePast struct {
	BossID      int64  `json:"boss_id"`
	BossName    string `json:"boss_name"`
	Tier        int    `json:"tier"`
	Outcome     string `json:"outcome"` // "defeated" | "survived"
	HPRemaining int    `json:"hp_remaining"`
	HPMax       int    `json:"hp_max"`
	Defenders   int    `json:"defenders"`
	MVP         string `json:"mvp,omitempty"`
	MVPFights   int    `json:"mvp_fights,omitempty"`
	EndedAt     int64  `json:"ended_at"`
}

// SiegeSnapshot is the complete war room: the live boss (if any), its muster,
// and every Siege that came before. Same complete-snapshot contract as the
// roster — Pete replaces its copy — so a defender omitted here leaves the board
// and a resolved Siege stops showing a live bar.
//
// Defenders carries EVERY alive adventurer, not just contributors. The zero-fight
// rows are the point: one bout per person per day means somebody who hasn't
// swung today is a hit the town hasn't taken, and the page can only show that gap
// if the people in it are on the wire.
type SiegeSnapshot struct {
	SnapshotAt int64           `json:"snapshot_at"`
	Active     bool            `json:"active"`
	BossID     int64           `json:"boss_id,omitempty"`
	BossName   string          `json:"boss_name,omitempty"`
	Tier       int             `json:"tier,omitempty"`
	HPCurrent  int             `json:"hp_current"`
	HPMax      int             `json:"hp_max"`
	StartsAt   int64           `json:"starts_at,omitempty"`
	EndsAt     int64           `json:"ends_at,omitempty"`
	BoutsToday int             `json:"bouts_today"`
	Defenders  []SiegeDefender `json:"defenders,omitempty"`
	History    []SiegePast     `json:"history,omitempty"`
}

// PushSiege sends the war room to Pete, synchronously, and drops it on failure —
// the same drop-the-lie semantics as PushRoster. A retried snapshot would claim
// a pool level that has since moved, and the next tick carries the truth anyway.
func PushSiege(ctx context.Context, snap SiegeSnapshot) error {
	if !Enabled() {
		return nil
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/ingest/siege", payload)
}

// RunBeat is one structured moment inside an expedition run: a room entered, a
// fight resolved, a trap sprung, a haul taken. Facts, never prose — Pete owns
// the words, exactly as it does for a Fact. The engine already narrates every
// one of these to Matrix and then throws the narration away; this carries the
// shape underneath it so Pete can retell the run to somebody who wasn't there.
//
// (RunID, Seq) is the identity. Seq is monotonic per run and assigned at record
// time, so Pete can order a batch that arrives out of order and drop a duplicate
// without comparing contents.
//
// Nothing here is player-identifying except Token, which rides the `start` beat
// only and is the same public board token the roster uses. An opted-out player's
// beats are never pushed at all — see pushRunBeats.
type RunBeat struct {
	RunID      string `json:"run_id"`
	Seq        int64  `json:"seq"`
	Kind       string `json:"kind"` // start|room|combat|trap|treasure|haul|lock|camp|region|end|summary
	OccurredAt int64  `json:"occurred_at"`

	Token      string `json:"token,omitempty"` // `start` only: whose run this is
	Name       string `json:"name,omitempty"`  // `start` only: character name
	Level      int    `json:"level,omitempty"` // `start` only
	Zone       string `json:"zone,omitempty"`
	Region     string `json:"region,omitempty"`
	Room       int    `json:"room,omitempty"`        // 1-based, as the player sees it
	TotalRooms int    `json:"total_rooms,omitempty"` // 0 when unknown
	RoomKind   string `json:"room_kind,omitempty"`   // entry|exploration|trap|elite|boss|secret
	Target     string `json:"target,omitempty"`      // monster, item, region, lock — the noun
	Outcome    string `json:"outcome,omitempty"`
	Amount     int    `json:"amount,omitempty"` // damage taken, or a total quantity
	Count      int    `json:"count,omitempty"`  // how many distinct things Amount covers
	HP         int    `json:"hp,omitempty"`
	HPMax      int    `json:"hp_max,omitempty"`
	Crits      int    `json:"crits,omitempty"`
	Fumbles    int    `json:"fumbles,omitempty"`
	// Prose is the single exception to "nouns and numbers only", and it is
	// confined to the one kind that has any: `summary`, the three sentences the
	// local model writes over a finished run. Pete guards it exactly as it guards
	// a dispatch lede and drops the words (not the beat) on a rejection. Every
	// other kind must leave this empty — Pete scrubs it if they don't.
	Prose string `json:"prose,omitempty"`
}

// RealmZone is one zone as the realm map draws it: what it is, who first got
// through it, how many have since, and who is inside it right now.
//
// FirstClearBy is a character name and FirstClearToken the public board token,
// exactly as the Siege muster pairs them — and the token is EMPTY for a player
// who has opted out, keeping the name off the page too (see buildRealmSnapshot:
// an opted-out first-clearer is anonymised, not deleted, because deleting the
// claim would make the zone read as never-cleared, which is a different and
// false statement about the realm).
type RealmZone struct {
	ID         string `json:"id"`
	Display    string `json:"display"`
	Tier       int    `json:"tier"`
	LevelMin   int    `json:"level_min"`
	LevelMax   int    `json:"level_max"`
	Faction    string `json:"faction,omitempty"`
	Atmosphere string `json:"atmosphere,omitempty"`
	Postgame   bool   `json:"postgame,omitempty"` // T6 mythic: gated, drawn apart

	FirstClearBy    string `json:"first_clear_by,omitempty"`
	FirstClearToken string `json:"first_clear_token,omitempty"`
	FirstClearAt    int64  `json:"first_clear_at,omitempty"`

	Clears   int `json:"clears"`   // boss-defeated runs, all time
	Clearers int `json:"clearers"` // distinct adventurers who have managed it

	Occupants []RealmOccupant `json:"occupants,omitempty"` // in there right now
}

// RealmOccupant is somebody currently on an expedition in a zone. Same
// name+token pair as everywhere else, and an opted-out player is omitted
// outright rather than anonymised: unlike a first clear, presence is not part of
// a shared tally that stops adding up without them, and "who is in there right
// now" is exactly the live-location fact the liveblog is careful about.
type RealmOccupant struct {
	Token string `json:"token,omitempty"`
	Name  string `json:"name"`
	Level int    `json:"level,omitempty"`
	Day   int    `json:"day,omitempty"`
}

// RealmFirst is one row of the hall of firsts: a thing that happened in the
// realm exactly once ever, and who it happened to. The ledger
// (news_realm_firsts) records only (kind, target, first_at) — the holder is
// recovered by gogobee at push time from the run history, which is why this is
// pushed rather than derived on Pete.
type RealmFirst struct {
	Kind    string `json:"kind"`              // "zone" | "treasure"
	Target  string `json:"target"`            // the zone id or treasure key
	Display string `json:"display"`           // the human name for it
	Tier    int    `json:"tier,omitempty"`    // zone tier, when kind is "zone"
	Holder  string `json:"holder,omitempty"`  // character name, empty when unrecoverable
	Token   string `json:"token,omitempty"`   // board token; empty when opted out
	AtUnix  int64  `json:"at_unix"`           // when the realm first saw it
}

// RealmStanding is one adventurer's line on the board. Every number here is a
// lifetime total from the game's own run history — nothing is a rate, an
// average, or anything that would move on its own while nobody played.
type RealmStanding struct {
	Token       string `json:"token,omitempty"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	ClassRace   string `json:"class_race,omitempty"`
	DeepestTier int    `json:"deepest_tier"` // deepest zone tier actually cleared
	Clears      int    `json:"clears"`
	Zones       int    `json:"zones"`  // distinct zones cleared
	Firsts      int    `json:"firsts"` // realm-firsts held
	SiegeDamage int    `json:"siege_damage"`
	SiegeFights int    `json:"siege_fights"`
}

// RealmSnapshot is the whole realm as one photograph: every zone, the hall of
// firsts, and the board. Snapshot semantics, like the roster and the Siege —
// Pete replaces its copy and a failed push is dropped, not retried.
//
// It is pushed on the roster ticker but NOT every tick: none of it moves fast
// enough to be worth the aggregate queries every two minutes, and the page's
// staleness window is generous for exactly that reason. See realmPushInterval.
type RealmSnapshot struct {
	SnapshotAt int64           `json:"snapshot_at"`
	Zones      []RealmZone     `json:"zones,omitempty"`
	Firsts     []RealmFirst    `json:"firsts,omitempty"`
	Standings  []RealmStanding `json:"standings,omitempty"`
}

// PushRealm sends the realm pages' backing data to Pete. Drop-on-failure, same
// as the other two snapshots.
func PushRealm(ctx context.Context, snap RealmSnapshot) error {
	if !Enabled() {
		return nil
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/ingest/realm", payload)
}

// PushRunBeats delivers a batch of beats. Unlike the snapshots this is
// append-only and IS retried — a dropped beat is a hole in a story, not a stale
// number that the next tick corrects. The caller only marks rows sent on success.
func PushRunBeats(ctx context.Context, beats []RunBeat) error {
	if !Enabled() || len(beats) == 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Beats []RunBeat `json:"beats"`
	}{beats})
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/ingest/run", payload)
}

// PlayerDetail is the private, owner-only expansion for one player: inventory,
// vault, house, and pets. Like MischiefBalance it is keyed by localpart (the
// sign-in name), in its own keyspace on Pete — Pete only ever serves it back to
// the one authenticated user it belongs to, never on the public board. It
// carries the player's current board Token too, so Pete can answer "is the
// signed-in viewer the owner of the adventurer on this page?" by a join, without
// ever having to reverse the one-way token.
type PlayerDetail struct {
	Localpart string     `json:"localpart"`
	Token     string     `json:"token"`
	Inventory []ItemView `json:"inventory,omitempty"`
	Vault     []ItemView `json:"vault,omitempty"`
	Equipped  []ItemView `json:"equipped,omitempty"`
	House     HouseView  `json:"house"`
	Pets      []PetView  `json:"pets,omitempty"`
	// Slots is the 5 standard equipment slots (weapon/armor/helmet/boots/tool) for
	// the web management panel. Worn masterwork/arena pieces surface here (via
	// CanTakeOff), not in Equipped, which stays magic-only (the DnD slots).
	Slots []EquipSlotView `json:"slots,omitempty"`
	// Balance is the owner's euro balance, for the web's upgrade/repair confirm.
	// No omitempty: a €0 balance is a real, informative fact (a broke player), not
	// an absent one — dropping it would let the confirm dialog show a stale amount.
	Balance float64 `json:"balance"`
}

// EquipSlotView is one of the 5 standard equipment slots, carrying what the web
// management panel needs: what's worn now, whether it round-trips to the pack
// (masterwork/arena), the next tier's name and price for an upgrade offer, and a
// repair cost when the piece is damaged. Pete renders it and trusts only these
// facts — a client-forged tier or price is resolved back against this view.
type EquipSlotView struct {
	Slot       string  `json:"slot"` // weapon|armor|helmet|boots|tool
	Name       string  `json:"name"`
	Tier       int     `json:"tier"`
	Condition  int     `json:"condition"`
	Masterwork bool    `json:"masterwork,omitempty"`
	ArenaTier  int     `json:"arena_tier,omitempty"`
	CanTakeOff bool    `json:"can_take_off,omitempty"` // masterwork/arena → round-trippable
	NextTier   int     `json:"next_tier,omitempty"`    // 0 = at max tier (5)
	NextName   string  `json:"next_name,omitempty"`
	NextPrice  float64 `json:"next_price,omitempty"`
	RepairCost int     `json:"repair_cost,omitempty"` // 0 = full condition
}

// ItemView is one item in the private panels — backpack, vault, or worn.
//
// Slot/SkillSource/Desc/Effect are display resolutions done at the push site,
// because an adventure_inventory row carries none of them: descriptions live on
// MagicItem/EquipmentDef, and the combat delta is computed, never stored.
//
// SkillSource is only the player-facing skill a masterwork piece draws on
// ("mining"). Inventory rows smuggle "magic_item:<id>" through the same column
// as an internal registry pointer; that is not a fact about the item and never
// goes on the wire.
//
// Attunement (does it need a bond) and Attuned (does it have one) are distinct:
// with a hard cap of 3 bonds, a worn item can sit inert, and a player deciding
// what to wear needs to see the difference.
type ItemView struct {
	// ID is the adventure_inventory row id, sent only for a backpack item the
	// magic-item equip path will accept — so a non-zero ID is also the signal that
	// this item can be equipped from the web. Worn and vault rows carry none: a
	// worn item unequips by slot, and a vault item can't be equipped at all. Pete
	// round-trips this id in an equip order; the table is AUTOINCREMENT, so a stale
	// id (item already moved) misses cleanly rather than hitting the wrong row.
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Tier        int    `json:"tier"`
	Value       int64  `json:"value"`
	Temper      int    `json:"temper,omitempty"`
	Slot        string `json:"slot,omitempty"`
	SkillSource string `json:"skill_source,omitempty"`
	Desc        string `json:"desc,omitempty"`
	Effect      string `json:"effect,omitempty"`
	Attunement  bool   `json:"attunement,omitempty"`
	Attuned     bool   `json:"attuned,omitempty"`
	// Compare pairs a backpack magic item against whatever is worn in the slot it
	// would equip into, so the owner can tell an upgrade from a sidegrade without
	// eyeballing two opaque effect strings. Set only on backpack magic items (the
	// ones that carry an equip ID); worn and vault rows never have it. Owner-private,
	// rides detail_json — no migration, no public surface.
	Compare *ItemCompare `json:"compare,omitempty"`
}

// ItemCompare is the per-stat verdict for equipping a backpack magic item over
// what is currently worn in its slot. gogobee computes it (the power math folds
// in tempering and bond availability, which live in the engine); Pete only
// renders it and does no arithmetic.
type ItemCompare struct {
	// Verdict is one of: upgrade, downgrade, sidegrade, same, new, inert.
	//   upgrade   every changed stat a gain
	//   downgrade every changed stat a loss
	//   sidegrade mixed — some better, some worse; no winner claimed
	//   same      no stat differs
	//   new       the target slot is empty; equipping fills it
	//   inert     needs a bond and none is free — wearing it would do nothing
	Verdict string `json:"verdict"`
	// VsName is the worn item being replaced; "" when Verdict is new (empty slot).
	VsName string `json:"vs_name,omitempty"`
	// VsSlot is the slot the item would land in (e.g. "ring_1").
	VsSlot string `json:"vs_slot,omitempty"`
	// Deltas is one entry per changed stat, each flagged better/worse. Engine-
	// rendered player-facing text; Pete draws arrows off Better and does no math.
	Deltas []ItemDelta `json:"deltas,omitempty"`
}

// ItemDelta is one stat's change between the candidate item and the worn item.
type ItemDelta struct {
	Label  string `json:"label"`
	Better bool   `json:"better"`
	Text   string `json:"text"`
}

// HouseView is the owner's housing summary.
type HouseView struct {
	Tier        int     `json:"tier"`
	LoanBalance int     `json:"loan_balance,omitempty"`
	Autopay     bool    `json:"autopay,omitempty"`
	Rate        float64 `json:"rate,omitempty"`
}

// PetView is one pet slot.
type PetView struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	XP        int    `json:"xp,omitempty"`
	ArmorTier int    `json:"armor_tier,omitempty"`
}

// DetailSnapshot is the complete private-detail set, pushed whole and replacing
// Pete's copy — same complete-snapshot contract as the roster, so a player who
// drops out of the game stops having a stale self-view on Pete.
type DetailSnapshot struct {
	SnapshotAt int64          `json:"snapshot_at"`
	Players    []PlayerDetail `json:"players"`
}

// PushDetails sends the private self-detail set to Pete, best-effort. Like the
// roster it is dropped on failure — the next tick carries a fresher copy — and
// it rides its own endpoint (not the roster body) so a fat inventory can't blow
// the roster push's size budget or its drop-the-lie semantics.
func PushDetails(ctx context.Context, snap DetailSnapshot) error {
	if !Enabled() {
		return nil
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/ingest/detail", payload)
}

// post sends one payload to a Pete endpoint with bearer auth. Mirrors the
// bearer-POST pattern in email_nag.go:sendCode.
func (c *Client) post(ctx context.Context, path string, payload []byte) error {
	url := c.cfg.IngestURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("pete ingest status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// The euro/chip border
//
// Pete holds chips; we hold the euros. A player buying in or cashing out opens
// an escrow row on Pete, and we are the only one who can move the money for it —
// Pete has no route into this box's network and is not getting one. So we poll.
//
// This is the first GET gogobee has ever made to Pete. Everything else in this
// package is us pushing facts outward; here we are asking for work.
//
// The escrow guid is the idempotency key end to end: it names the row on Pete,
// it is the external_id on our euro transaction, and it is the queue key of the
// verdict we push back. That is what makes every step here safe to retry, which
// matters because every step here can be interrupted between moving real money
// and saying so.
// ---------------------------------------------------------------------------

// Escrow is one pending crossing, as Pete describes it. Amounts are whole euros:
// chips are 1:1 and there is no sub-unit to lose.
type Escrow struct {
	GUID       string `json:"guid"`
	MatrixUser string `json:"matrix_user"`
	Kind       string `json:"kind"` // "buyin" | "cashout"
	Amount     int64  `json:"amount"`
	State      string `json:"state"`
}

// EscrowVerdict is our answer: did the euros move, and what is the balance now.
// A rejected buy-in carries the reason, which Pete shows the player.
type EscrowVerdict struct {
	GUID         string  `json:"guid"`
	OK           bool    `json:"ok"`
	Reason       string  `json:"reason,omitempty"`
	BalanceAfter float64 `json:"balance_after"`
}

const escrowVerdictPath = "/api/games/escrow/settled"

// PendingEscrow asks Pete for crossings waiting on us. Includes rows we claimed
// but never answered — if we died holding one, the player's money is stranded
// until we pick it up again.
func PendingEscrow(ctx context.Context) ([]Escrow, error) {
	if !Enabled() {
		return nil, nil
	}
	var out []Escrow
	if err := std.getJSON(ctx, "/api/games/escrow/pending", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ClaimEscrow tells Pete we are taking a row, and returns the row as Pete now
// holds it. Move the money against *this*, not against the copy from the poll:
// the claim is the moment the amount and the player are fixed.
//
// A row Pete has already decided comes back in a terminal state rather than
// "claimed". That is not an error — it means the work is done, and it is exactly
// what stops a settled cash-out from being paid a second time.
func ClaimEscrow(ctx context.Context, guid string) (Escrow, error) {
	var e Escrow
	payload, err := json.Marshal(map[string]string{"guid": guid})
	if err != nil {
		return e, err
	}
	if err := std.postJSON(ctx, "/api/games/escrow/claim", payload, &e); err != nil {
		return e, err
	}
	return e, nil
}

// EmitEscrowVerdict durably queues our answer and returns immediately. Keyed on
// the escrow guid, so a verdict is enqueued once and only once, and the sender's
// retry/backoff/parking machinery carries it the rest of the way.
//
// The caller should Flush after this: a player is watching a spinner.
func EmitEscrowVerdict(v EscrowVerdict) {
	if !Enabled() {
		return
	}
	payload, err := json.Marshal(v)
	if err != nil {
		slog.Error("peteclient: marshal escrow verdict", "guid", v.GUID, "err", err)
		return
	}
	// Namespaced so an escrow guid can never collide with a fact guid in the
	// queue's primary key. Fact guids are "<event_type>:<token>:<ts>"; escrow
	// guids are random. A collision would be a lost verdict, so don't rely on
	// luck for it.
	enqueue("escrow:"+v.GUID, escrowVerdictPath, payload)
}

// ---------------------------------------------------------------------------
// The mischief storefront's reverse pipe
//
// A buyer places a hit on Pete's web board; we poll for it, do the real work
// against our own ledger, and hand back a verdict. Same shape as the escrow
// border above — Pete has no route in, so we poll — but simpler: the order guid
// is the end-to-end idempotency key (external_id on the euro debit, stamped on
// the contract), and the *verdict rides the claim itself* rather than a durable
// queue. If a claim fails, the order stays pending and the next poll re-offers
// it; re-running is a no-op, so the poll loop is its own retry.
// ---------------------------------------------------------------------------

// MischiefOrder is one storefront order as Pete describes it. buyer_sub stays on
// Pete (it is the OIDC subject, only for "my orders"); we get the username, which
// we turn into a Matrix id, and the anonymous target token.
type MischiefOrder struct {
	GUID          string `json:"guid"`
	BuyerUsername string `json:"buyer_username"`
	TargetToken   string `json:"target_token"`
	TargetName    string `json:"target_name"`
	Tier          string `json:"tier"`
	Signed        bool   `json:"signed"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"created_at"`
}

// PendingMischief asks Pete for orders waiting on us. A Pete that predates the
// storefront answers 404, which surfaces here as an error; the poll loop treats
// any error as "nothing to do this tick" and logs it quietly, so gogobee can ship
// ahead of Pete without noise.
func PendingMischief(ctx context.Context) ([]MischiefOrder, error) {
	if !Enabled() {
		return nil, nil
	}
	var out []MischiefOrder
	if err := std.getJSON(ctx, "/api/mischief/pending", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ClaimMischief files our verdict on an order: the terminal status Pete should
// show and a human note. Idempotent on Pete, so a retried claim is safe. The
// verdict rides this call directly — there is no separate durable emit, because
// a lost claim just leaves the order pending for the next poll to re-run.
func ClaimMischief(ctx context.Context, guid, status, detail string) error {
	payload, err := json.Marshal(map[string]string{"guid": guid, "status": status, "detail": detail})
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/mischief/claim", payload)
}

// ---------------------------------------------------------------------------
// The equip queue's reverse pipe
//
// An owner asks, on their own detail page, to wear or take off an item. Pete
// records the intent; we poll for it, run the real equip against our own
// equipment tables, and file a verdict. Same shape as mischief — Pete has no
// route in — but with one crucial difference: the game action is NOT naturally
// idempotent (equipping consumes an inventory row and regenerates it on
// unequip), so re-running a drained order would double-move items. The poller
// therefore short-circuits on the order guid before it mutates, the way
// placeWebMischief does on its contract; the guid is still the end-to-end key,
// but here it guards a non-idempotent action rather than riding a naturally
// idempotent one.
// ---------------------------------------------------------------------------

// EquipOrder is one equip/unequip as Pete describes it. owner_localpart is the
// Matrix localpart of the character to dress; item_id is the adventure_inventory
// row id for an equip (0 for an unequip, which keys on slot). character_name and
// item_name are display copy Pete froze at order time; we don't need them.
type EquipOrder struct {
	GUID           string `json:"guid"`
	OwnerLocalpart string `json:"owner_localpart"`
	ItemID         int64  `json:"item_id"`
	ItemName       string `json:"item_name"`
	Slot           string `json:"slot"`
	Action         string `json:"action"` // equip / unequip / upgrade / repair
	Tier           int    `json:"tier"`   // upgrade target tier (an EquipmentSlot tier); unused by the others
	Status         string `json:"status"`
	CreatedAt      int64  `json:"created_at"`
}

// PendingEquip asks Pete for equip orders waiting on us. A Pete predating the
// queue answers 404, surfaced here as an error the poll loop logs quietly.
func PendingEquip(ctx context.Context) ([]EquipOrder, error) {
	if !Enabled() {
		return nil, nil
	}
	var out []EquipOrder
	if err := std.getJSON(ctx, "/api/equip/pending", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// VerdictEquip files our verdict on an equip order. Idempotent on Pete, so a
// retried verdict is safe; the verdict rides this call directly.
func VerdictEquip(ctx context.Context, guid, status, detail string) error {
	payload, err := json.Marshal(map[string]string{"guid": guid, "status": status, "detail": detail})
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/equip/verdict", payload)
}

// ---------------------------------------------------------------------------
// The action queue
//
// The equip queue's sibling, and the first one that plays the game rather than
// dressing the character. An owner clicks "Pull out" on their own adventurer
// page or "Take your bout" on the war room; Pete records the intent and we drain
// it here. Same non-idempotent problem, same answer: the poller guards on the
// order guid before it runs anything, because an extraction ends a run and a
// bout spends the day's only swing, and neither converges on a replay.
//
// Nothing in an order names a character. Pete resolves that from the session
// (one account, one localpart, one adventurer) so there is no id on the wire for
// a client to forge — the contrast with EquipOrder, which has to carry an item
// id and a slot, is deliberate.
// ---------------------------------------------------------------------------

// AdvOrder is one requested action as Pete describes it. owner_localpart is the
// Matrix localpart whose adventurer acts; token and character_name are display
// copy Pete froze at order time and we ignore both.
type AdvOrder struct {
	GUID           string `json:"guid"`
	OwnerLocalpart string `json:"owner_localpart"`
	Token          string `json:"token"`
	CharacterName  string `json:"character_name"`
	Action         string `json:"action"` // extract / siege_join
	Status         string `json:"status"`
	CreatedAt      int64  `json:"created_at"`
}

// Action names, the wire contract's half of storage.AdvAction* on Pete.
const (
	AdvOrderExtract   = "extract"
	AdvOrderSiegeJoin = "siege_join"
)

// PendingOrders asks Pete for web actions waiting on us. A Pete predating the
// queue answers 404, surfaced here as an error the poll loop logs quietly.
func PendingOrders(ctx context.Context) ([]AdvOrder, error) {
	if !Enabled() {
		return nil, nil
	}
	var out []AdvOrder
	if err := std.getJSON(ctx, "/api/adventure/orders/pending", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// VerdictOrder files our verdict on a web action. Idempotent on Pete, so a
// retried verdict is safe.
func VerdictOrder(ctx context.Context, guid, status, detail string) error {
	payload, err := json.Marshal(map[string]string{"guid": guid, "status": status, "detail": detail})
	if err != nil {
		return err
	}
	return std.post(ctx, "/api/adventure/orders/verdict", payload)
}

// getJSON does a bearer-authed GET and decodes the body.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.IngestURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	return c.do(req, out)
}

// postJSON does a bearer-authed POST and decodes the body. Distinct from post,
// which is the fire-and-forget path the queue uses and ignores the response.
func (c *Client) postJSON(ctx context.Context, path string, payload []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.IngestURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("pete %s status %d: %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("pete %s: decode: %w", req.URL.Path, err)
	}
	return nil
}

// backoffSec computes the retry delay for a row. It re-reads the current attempt
// count so the delay grows geometrically without needing it passed in.
func backoffSec(guid string) int {
	var attempts int
	_ = db.Get().QueryRow(`SELECT attempts FROM pete_emit_queue WHERE guid = ?`, guid).Scan(&attempts)
	// attempts is the count *before* this failure's increment; delay off it.
	delay := int(backoffBase.Seconds()) << attempts
	if delay > backoffCapSec {
		delay = backoffCapSec
	}
	return delay
}
