package plugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// EmailNagPlugin DMs a fixed set of local users who have no email on file in
// Authentik, collects an address, verifies inbox ownership with an emailed
// 6-digit code, and only then writes the address to Authentik via its API.
//
// Nothing is written to Authentik until the code is confirmed, so a mistyped
// or hostile address never becomes a live recovery address. State lives in
// SQLite so restarts don't re-nag anyone already done or mid-flow. Modeled on
// SpaceInviterPlugin.
type EmailNagPlugin struct {
	Base

	enabled bool
	cfg     emailNagConfig
	http    *http.Client

	mu      sync.Mutex                // serializes per-user FSM transitions
	sendLog map[id.UserID][]time.Time // per-user code-send timestamps (rolling 1h window)
}

type emailNagConfig struct {
	AuthentikURL     string
	AuthentikToken   string
	ResendKey        string
	MailFrom         string
	HomeDomain       string
	Targets          []string // Authentik usernames == Matrix localparts
	CodeTTL          time.Duration
	MaxAttempts      int
	SweepDelay       time.Duration
	RepromptCooldown time.Duration // 0 = nag once, never re-nag
	MaxSendsPerHour  int           // cap on verification emails per user per rolling hour
}

const (
	nagStageEmail = "awaiting_email"
	nagStageCode  = "awaiting_code"
	nagStageDone  = "done"
)

var emailNagRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

func loadEmailNagConfig() emailNagConfig {
	targets := []string{}
	for _, t := range strings.Split(os.Getenv("EMAIL_NAG_TARGETS"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			targets = append(targets, t)
		}
	}
	maxAttempts := 5
	if v := os.Getenv("EMAIL_NAG_MAX_ATTEMPTS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			maxAttempts = n
		}
	}
	maxSends := 5
	if v := os.Getenv("EMAIL_NAG_MAX_SENDS_PER_HOUR"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			maxSends = n
		}
	}
	return emailNagConfig{
		AuthentikURL:     strings.TrimRight(os.Getenv("EMAIL_NAG_AUTHENTIK_URL"), "/"),
		AuthentikToken:   os.Getenv("EMAIL_NAG_AUTHENTIK_TOKEN"),
		ResendKey:        os.Getenv("EMAIL_NAG_RESEND_KEY"),
		MailFrom:         os.Getenv("EMAIL_NAG_MAIL_FROM"),
		HomeDomain:       os.Getenv("EMAIL_NAG_HOME_DOMAIN"),
		Targets:          targets,
		CodeTTL:          envDur("EMAIL_NAG_CODE_TTL", 30*time.Minute),
		MaxAttempts:      maxAttempts,
		SweepDelay:       envDur("EMAIL_NAG_SWEEP_DELAY", 2*time.Second),
		RepromptCooldown: envDur("EMAIL_NAG_REPROMPT_COOLDOWN", 0),
		MaxSendsPerHour:  maxSends,
	}
}

func NewEmailNagPlugin(client *mautrix.Client) *EmailNagPlugin {
	enabled := strings.EqualFold(os.Getenv("FEATURE_EMAIL_NAG"), "true")
	if !enabled {
		slog.Info("email_nag: disabled (set FEATURE_EMAIL_NAG=true to enable)")
	}
	return &EmailNagPlugin{
		Base:    NewBase(client),
		enabled: enabled,
		cfg:     loadEmailNagConfig(),
		http:    &http.Client{Timeout: 30 * time.Second},
		sendLog: make(map[id.UserID][]time.Time),
	}
}

func (p *EmailNagPlugin) Name() string                     { return "email_nag" }
func (p *EmailNagPlugin) Commands() []CommandDef           { return nil }
func (p *EmailNagPlugin) OnReaction(ReactionContext) error { return nil }

func (p *EmailNagPlugin) Init() error {
	if !p.enabled {
		return nil
	}
	if err := p.validateConfig(); err != nil {
		// Don't take the whole bot down over a misconfigured one-shot helper —
		// just disable this plugin and keep everything else running.
		slog.Error("email_nag: disabled — invalid config", "err", err)
		p.enabled = false
		return nil
	}
	slog.Info("email_nag: ready",
		"authentik", p.cfg.AuthentikURL,
		"home_domain", p.cfg.HomeDomain,
		"targets", len(p.cfg.Targets),
	)
	go p.runStartupSweep()
	return nil
}

func (p *EmailNagPlugin) validateConfig() error {
	missing := []string{}
	if p.cfg.AuthentikURL == "" {
		missing = append(missing, "EMAIL_NAG_AUTHENTIK_URL")
	}
	if p.cfg.AuthentikToken == "" {
		missing = append(missing, "EMAIL_NAG_AUTHENTIK_TOKEN")
	}
	if p.cfg.ResendKey == "" {
		missing = append(missing, "EMAIL_NAG_RESEND_KEY")
	}
	if p.cfg.MailFrom == "" {
		missing = append(missing, "EMAIL_NAG_MAIL_FROM")
	}
	if p.cfg.HomeDomain == "" {
		missing = append(missing, "EMAIL_NAG_HOME_DOMAIN")
	}
	if len(p.cfg.Targets) == 0 {
		missing = append(missing, "EMAIL_NAG_TARGETS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (p *EmailNagPlugin) mxid(username string) id.UserID {
	return id.UserID(fmt.Sprintf("@%s:%s", username, p.cfg.HomeDomain))
}

// ---------------------------------------------------------------------------
// Authentik API
// ---------------------------------------------------------------------------

type authentikUser struct {
	PK       int    `json:"pk"`
	Username string `json:"username"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
}

// lookupUser returns the Authentik user for an exact username, or nil if absent.
func (p *EmailNagPlugin) lookupUser(ctx context.Context, username string) (*authentikUser, error) {
	u := fmt.Sprintf("%s/api/v3/core/users/?username=%s", p.cfg.AuthentikURL, username)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+p.cfg.AuthentikToken)
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentik users status %d: %s", resp.StatusCode, string(body))
	}
	var page struct {
		Results []authentikUser `json:"results"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("authentik decode: %w", err)
	}
	for i := range page.Results {
		if page.Results[i].Username == username {
			return &page.Results[i], nil
		}
	}
	return nil, nil
}

// setEmail writes an email to an Authentik user by pk.
func (p *EmailNagPlugin) setEmail(ctx context.Context, pk int, email string) error {
	payload, _ := json.Marshal(map[string]string{"email": email})
	u := fmt.Sprintf("%s/api/v3/core/users/%d/", p.cfg.AuthentikURL, pk)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+p.cfg.AuthentikToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentik patch status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Resend API
// ---------------------------------------------------------------------------

func (p *EmailNagPlugin) sendCode(ctx context.Context, to, code string) error {
	payload, _ := json.Marshal(map[string]any{
		"from":    p.cfg.MailFrom,
		"to":      []string{to},
		"subject": "Your parodia.dev verification code",
		"text": fmt.Sprintf(
			"Your parodia.dev email-verification code is: %s\n\n"+
				"Reply to the bot with this code within %s to confirm this address.\n"+
				"If you didn't request this, you can ignore this message.",
			code, formatCooldown(p.cfg.CodeTTL)),
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+p.cfg.ResendKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("resend status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// allowSend enforces a per-user cap on verification emails over a rolling hour,
// so a target can't make the bot spray branded code emails at arbitrary
// addresses. Caller must hold p.mu (sendLog is not separately synchronized).
func (p *EmailNagPlugin) allowSend(mxid id.UserID, now time.Time) bool {
	cutoff := now.Add(-time.Hour)
	kept := p.sendLog[mxid][:0]
	for _, t := range p.sendLog[mxid] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	p.sendLog[mxid] = kept
	if len(kept) >= p.cfg.MaxSendsPerHour {
		return false
	}
	p.sendLog[mxid] = append(kept, now)
	return true
}

func gen6() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "000000"
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// ---------------------------------------------------------------------------
// Startup sweep
// ---------------------------------------------------------------------------

func (p *EmailNagPlugin) runStartupSweep() {
	ctx := context.Background()
	sent, skipped := 0, 0
	for _, username := range p.cfg.Targets {
		mxid := p.mxid(username)
		// Already finished in a prior run? Never re-nag.
		if st, _, _, ok := p.rowStatus(mxid); ok && st == nagStageDone {
			skipped++
			continue
		}
		// Email already set in Authentik (set by us before, or out-of-band)?
		u, err := p.lookupUser(ctx, username)
		if err != nil {
			slog.Warn("email_nag: authentik lookup failed", "user", username, "err", err)
			continue
		}
		if u == nil {
			slog.Warn("email_nag: no Authentik user", "user", username)
			continue
		}
		if !u.IsActive {
			continue
		}
		if strings.TrimSpace(u.Email) != "" {
			p.markDone(mxid, username, "", u.Email) // record so we stop checking
			skipped++
			continue
		}
		// In-flight already (DM sent, awaiting reply)? Re-nag only if a cooldown
		// is configured and the last prompt is older than it; otherwise leave alone.
		if st, promptAt, dmRoom, ok := p.rowStatus(mxid); ok && (st == nagStageEmail || st == nagStageCode) {
			if p.cfg.RepromptCooldown > 0 && time.Since(time.Unix(promptAt, 0)) > p.cfg.RepromptCooldown {
				if err := p.renag(id.RoomID(dmRoom), mxid, st); err != nil {
					slog.Warn("email_nag: renag failed", "user", username, "err", err)
				} else {
					sent++
					time.Sleep(p.cfg.SweepDelay)
				}
			} else {
				skipped++
			}
			continue
		}
		if err := p.startNag(ctx, username); err != nil {
			slog.Warn("email_nag: start nag failed", "user", username, "err", err)
			continue
		}
		sent++
		time.Sleep(p.cfg.SweepDelay)
	}
	slog.Info("email_nag: startup sweep complete", "nagged", sent, "skipped", skipped)
}

func (p *EmailNagPlugin) startNag(ctx context.Context, username string) error {
	mxid := p.mxid(username)
	room, err := p.GetDMRoom(mxid)
	if err != nil {
		return fmt.Errorf("get dm room: %w", err)
	}
	now := time.Now().Unix()
	_, err = db.Get().Exec(`
		INSERT INTO email_nag_prompts (user_id, username, dm_room_id, stage, prompt_sent_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			dm_room_id=excluded.dm_room_id, stage=excluded.stage, updated_at=excluded.updated_at`,
		string(mxid), username, string(room), nagStageEmail, now, now)
	if err != nil {
		return fmt.Errorf("record prompt: %w", err)
	}
	return p.SendMessage(room,
		"Hi! We're migrating parodia.dev to a new login system, and your account "+
			"has no email on file — it's needed for password recovery. "+
			"Please reply here with your email address and I'll verify it.")
}

// ---------------------------------------------------------------------------
// Reply handling — FSM driven from DM replies
// ---------------------------------------------------------------------------

type nagRow struct {
	Username     string
	DMRoom       string
	Stage        string
	PendingEmail sql.NullString
	Code         sql.NullString
	CodeExpires  sql.NullInt64
	Attempts     int
}

func (p *EmailNagPlugin) rowStatus(mxid id.UserID) (stage string, promptAt int64, dmRoom string, ok bool) {
	err := db.Get().QueryRow(
		`SELECT stage, prompt_sent_at, dm_room_id FROM email_nag_prompts WHERE user_id = ?`,
		string(mxid)).Scan(&stage, &promptAt, &dmRoom)
	return stage, promptAt, dmRoom, err == nil
}

// renag sends a stage-appropriate reminder and bumps prompt_sent_at so the
// cooldown restarts. Used by the sweep when EMAIL_NAG_REPROMPT_COOLDOWN > 0.
func (p *EmailNagPlugin) renag(room id.RoomID, mxid id.UserID, stage string) error {
	msg := "Just a reminder — we still need an email on your account for the login " +
		"migration. Please reply here with your email address."
	if stage == nagStageCode {
		msg = "Reminder: I'm still waiting for the verification code I emailed you. " +
			"Reply with it, or send your email again for a fresh code."
	}
	now := time.Now().Unix()
	if _, err := db.Get().Exec(
		`UPDATE email_nag_prompts SET prompt_sent_at=?, updated_at=? WHERE user_id=?`,
		now, now, string(mxid)); err != nil {
		return err
	}
	return p.SendMessage(room, msg)
}

func (p *EmailNagPlugin) load(mxid id.UserID, room id.RoomID) (*nagRow, bool) {
	var r nagRow
	err := db.Get().QueryRow(`
		SELECT username, dm_room_id, stage, pending_email, code, code_expires, attempts
		FROM email_nag_prompts WHERE user_id = ? AND dm_room_id = ?`,
		string(mxid), string(room)).Scan(
		&r.Username, &r.DMRoom, &r.Stage, &r.PendingEmail, &r.Code, &r.CodeExpires, &r.Attempts)
	if err != nil {
		return nil, false
	}
	return &r, true
}

func (p *EmailNagPlugin) markDone(mxid id.UserID, username, room, email string) {
	now := time.Now().Unix()
	_, _ = db.Get().Exec(`
		INSERT INTO email_nag_prompts (user_id, username, dm_room_id, stage, prompt_sent_at, verified_email, verified_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			stage=excluded.stage, verified_email=excluded.verified_email,
			verified_at=excluded.verified_at, updated_at=excluded.updated_at`,
		string(mxid), username, room, nagStageDone, now, email, now, now)
}

// OnMessage drives the per-user FSM. Only acts on messages in a tracked DM room
// sent by that room's target user.
func (p *EmailNagPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		return nil
	}
	r, ok := p.load(ctx.Sender, ctx.RoomID)
	if !ok || r.Stage == nagStageDone {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Re-read under lock in case a concurrent transition already advanced us.
	r, ok = p.load(ctx.Sender, ctx.RoomID)
	if !ok || r.Stage == nagStageDone {
		return nil
	}

	bgctx := context.Background()
	body := strings.TrimSpace(ctx.Body)
	now := time.Now()

	// An email anywhere in the message (re)starts verification for that address,
	// whether we're awaiting the first email or a code-with-corrected-address.
	if m := emailNagRe.FindString(body); m != "" {
		email := strings.ToLower(m)
		if !p.allowSend(ctx.Sender, now) {
			return p.SendMessage(ctx.RoomID, fmt.Sprintf(
				"That's a lot of attempts in a short window — please wait a bit before "+
					"requesting another code (max %d per hour).", p.cfg.MaxSendsPerHour))
		}
		code := gen6()
		exp := now.Add(p.cfg.CodeTTL).Unix()
		if _, err := db.Get().Exec(`
			UPDATE email_nag_prompts
			SET stage=?, pending_email=?, code=?, code_expires=?, attempts=0, updated_at=?
			WHERE user_id=?`,
			nagStageCode, email, code, exp, now.Unix(), string(ctx.Sender)); err != nil {
			return err
		}
		if err := p.sendCode(bgctx, email, code); err != nil {
			slog.Error("email_nag: send code failed", "to", email, "err", err)
			return p.SendMessage(ctx.RoomID,
				"I couldn't send a code to that address just now — please double-check it and send it again.")
		}
		return p.SendMessage(ctx.RoomID, fmt.Sprintf(
			"I emailed a 6-digit code to %s. Reply with the code to confirm (valid %s). "+
				"Wrong address? Just send the correct one.", email, formatCooldown(p.cfg.CodeTTL)))
	}

	if r.Stage == nagStageEmail {
		return p.SendMessage(ctx.RoomID,
			"That doesn't look like an email address. Please send just your address, e.g. you@example.com")
	}

	// Stage awaiting_code: expect the digits.
	entered := regexp.MustCompile(`\D`).ReplaceAllString(body, "")
	if entered == "" {
		return nil // not a code and not an email — ignore chatter
	}
	if !r.CodeExpires.Valid || now.Unix() > r.CodeExpires.Int64 {
		_, _ = db.Get().Exec(`UPDATE email_nag_prompts SET stage=?, updated_at=? WHERE user_id=?`,
			nagStageEmail, now.Unix(), string(ctx.Sender))
		return p.SendMessage(ctx.RoomID, "That code expired. Send your email again and I'll issue a new one.")
	}
	if !r.Code.Valid || entered != r.Code.String {
		attempts := r.Attempts + 1
		if attempts >= p.cfg.MaxAttempts {
			_, _ = db.Get().Exec(`UPDATE email_nag_prompts SET stage=?, attempts=0, updated_at=? WHERE user_id=?`,
				nagStageEmail, now.Unix(), string(ctx.Sender))
			return p.SendMessage(ctx.RoomID, "Too many wrong codes. Send your email again to restart.")
		}
		_, _ = db.Get().Exec(`UPDATE email_nag_prompts SET attempts=?, updated_at=? WHERE user_id=?`,
			attempts, now.Unix(), string(ctx.Sender))
		return p.SendMessage(ctx.RoomID, "That code doesn't match. Try again.")
	}

	// Verified — write to Authentik (re-check it's still empty to avoid clobbering).
	u, err := p.lookupUser(bgctx, r.Username)
	if err != nil {
		slog.Error("email_nag: verify lookup failed", "user", r.Username, "err", err)
		return p.SendMessage(ctx.RoomID, "Something went wrong on my end — please try the code again in a moment.")
	}
	if u == nil {
		return p.SendMessage(ctx.RoomID, "I can't find your account anymore — please contact an admin.")
	}
	if strings.TrimSpace(u.Email) == "" {
		if err := p.setEmail(bgctx, u.PK, r.PendingEmail.String); err != nil {
			slog.Error("email_nag: set email failed", "user", r.Username, "err", err)
			return p.SendMessage(ctx.RoomID, "I verified the code but couldn't save it — please try again shortly.")
		}
	}
	p.markDone(ctx.Sender, r.Username, string(ctx.RoomID), r.PendingEmail.String)
	slog.Info("email_nag: verified", "user", r.Username, "email", r.PendingEmail.String)
	return p.SendMessage(ctx.RoomID, fmt.Sprintf(
		"Verified ✓ %s is now on your account. Thanks — that's all we needed!", r.PendingEmail.String))
}
