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
	"time"

	"gogobee/internal/db"
)

// Fact is the flat, pre-sanitized payload gogobee POSTs to Pete. Names must be
// character names only (never Matrix handles); Actors is the allow-list of the
// only names permitted to appear in Pete's rendered output. See
// pete_adventure_news_voice.md for the field contract.
type Fact struct {
	GUID       string   `json:"guid"` // stable idempotency key, e.g. "death:<hash>:<ts>"
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
	cfg  Config
	http *http.Client
}

var std *Client

// Tuning for the background sender.
const (
	senderTick    = 15 * time.Second
	senderBatch   = 20
	maxAttempts   = 8               // ~ up to a few hours of backoff, then park
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
	// OR IGNORE gives GUID-idempotency: a re-emit of the same event is dropped.
	db.Exec("pete emit enqueue",
		`INSERT OR IGNORE INTO pete_emit_queue (guid, payload, created_at, attempts, next_attempt_at)
		 VALUES (?, ?, unixepoch(), 0, 0)`,
		f.GUID, string(payload))
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

// drain sends up to senderBatch due rows, one at a time.
func (c *Client) drain(ctx context.Context) {
	rows, err := db.Get().Query(
		`SELECT guid, payload FROM pete_emit_queue
		 WHERE sent_at IS NULL AND attempts < ? AND next_attempt_at <= unixepoch()
		 ORDER BY created_at LIMIT ?`,
		maxAttempts, senderBatch)
	if err != nil {
		slog.Error("peteclient: drain query", "err", err)
		return
	}
	type item struct{ guid, payload string }
	var batch []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.guid, &it.payload); err != nil {
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
		if err := c.send(ctx, []byte(it.payload)); err != nil {
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

// send POSTs one payload to Pete's ingest endpoint with bearer auth. Mirrors the
// bearer-POST pattern in email_nag.go:sendCode.
func (c *Client) send(ctx context.Context, payload []byte) error {
	url := c.cfg.IngestURL + "/api/ingest/adventure"
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
