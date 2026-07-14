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
}

// RosterSnapshot is the complete board. Complete is load-bearing: Pete replaces
// its whole board with this, so anyone omitted (opted out, no character) drops
// off the public page. A partial snapshot would silently strand people on it.
type RosterSnapshot struct {
	SnapshotAt  int64         `json:"snapshot_at"`
	Adventurers []RosterEntry `json:"adventurers"`
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
