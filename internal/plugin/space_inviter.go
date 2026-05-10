package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// SpaceInviterPlugin DMs the configured admin whenever a local homeserver
// user is detected as belonging to zero Matrix Spaces. The admin replies
// `yes` / `no` / `ignore` in the DM room; on `yes` the bot invites the user
// to the configured target Space.
type SpaceInviterPlugin struct {
	Base

	enabled bool
	cfg     spaceInviterConfig

	// Resolved at Init: DM room with admin
	adminDMRoom id.RoomID

	// In-memory: user IDs we've sent a (still-pending) prompt for this session.
	// DB is the source of truth across restarts; this just short-circuits before
	// hitting SQLite on the hot path of the reactive handler.
	pendingMu sync.Mutex
	pending   map[id.UserID]struct{}
}

type spaceInviterConfig struct {
	TargetSpaceID    id.RoomID
	HomeDomain       string
	SynapseBaseURL   string
	AdminAccessToken string
	AdminUserID      id.UserID
	InviteDelay      time.Duration
	RepromptCooldown time.Duration
}

func loadSpaceInviterConfig() spaceInviterConfig {
	return spaceInviterConfig{
		TargetSpaceID:    id.RoomID(os.Getenv("SPACE_INVITER_TARGET")),
		HomeDomain:       os.Getenv("SPACE_INVITER_HOME_DOMAIN"),
		SynapseBaseURL:   strings.TrimRight(os.Getenv("SPACE_INVITER_SYNAPSE_URL"), "/"),
		AdminAccessToken: os.Getenv("SPACE_INVITER_ADMIN_TOKEN"),
		AdminUserID:      id.UserID(os.Getenv("SPACE_INVITER_ADMIN_USER")),
		InviteDelay:      envDur("SPACE_INVITER_DELAY", 2*time.Second),
		RepromptCooldown: envDur("SPACE_INVITER_COOLDOWN", 30*24*time.Hour),
	}
}

func envDur(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return fallback
}

func NewSpaceInviterPlugin(client *mautrix.Client) *SpaceInviterPlugin {
	cfg := loadSpaceInviterConfig()
	enabled := strings.EqualFold(os.Getenv("FEATURE_SPACE_INVITER"), "true")
	if !enabled {
		slog.Info("space_inviter: disabled (set FEATURE_SPACE_INVITER=true to enable)")
	}
	return &SpaceInviterPlugin{
		Base:    NewBase(client),
		enabled: enabled,
		cfg:     cfg,
		pending: make(map[id.UserID]struct{}),
	}
}

func (p *SpaceInviterPlugin) Name() string             { return "space_inviter" }
func (p *SpaceInviterPlugin) Commands() []CommandDef   { return nil }
func (p *SpaceInviterPlugin) OnReaction(ReactionContext) error { return nil }

func (p *SpaceInviterPlugin) Init() error {
	if !p.enabled {
		return nil
	}

	if err := p.validateConfig(); err != nil {
		return fmt.Errorf("space_inviter: %w", err)
	}

	ctx := context.Background()

	// Permission check: bot must be in target space and able to invite.
	if _, err := p.Client.JoinedMembers(ctx, p.cfg.TargetSpaceID); err != nil {
		return fmt.Errorf("space_inviter: bot not in target space %s: %w", p.cfg.TargetSpaceID, err)
	}
	if err := p.checkInvitePower(ctx); err != nil {
		return fmt.Errorf("space_inviter: %w", err)
	}

	// Resolve DM room with admin.
	dm, err := p.GetDMRoom(p.cfg.AdminUserID)
	if err != nil {
		return fmt.Errorf("space_inviter: resolve admin DM: %w", err)
	}
	p.adminDMRoom = dm

	// Sanity log: enumerate Spaces the bot is in.
	spaces := p.discoverSpaces(ctx)
	slog.Info("space_inviter: ready",
		"target_space", p.cfg.TargetSpaceID,
		"admin", p.cfg.AdminUserID,
		"dm_room", p.adminDMRoom,
		"known_spaces", len(spaces),
	)
	if len(spaces) <= 1 {
		slog.Warn("space_inviter: bot is in only the target space — 'in any space' check degenerates to 'in target space'")
	}

	// Hydrate pending set from DB (rows with reply IS NULL).
	p.hydratePending()

	// Kick off startup sweep in the background; we don't want to block bot startup.
	go p.runStartupSweep()

	return nil
}

func (p *SpaceInviterPlugin) validateConfig() error {
	missing := []string{}
	if p.cfg.TargetSpaceID == "" {
		missing = append(missing, "SPACE_INVITER_TARGET")
	}
	if p.cfg.HomeDomain == "" {
		missing = append(missing, "SPACE_INVITER_HOME_DOMAIN")
	}
	if p.cfg.SynapseBaseURL == "" {
		missing = append(missing, "SPACE_INVITER_SYNAPSE_URL")
	}
	if p.cfg.AdminAccessToken == "" {
		missing = append(missing, "SPACE_INVITER_ADMIN_TOKEN")
	}
	if p.cfg.AdminUserID == "" {
		missing = append(missing, "SPACE_INVITER_ADMIN_USER")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (p *SpaceInviterPlugin) checkInvitePower(ctx context.Context) error {
	var pl event.PowerLevelsEventContent
	if err := p.Client.StateEvent(ctx, p.cfg.TargetSpaceID, event.StatePowerLevels, "", &pl); err != nil {
		return fmt.Errorf("read target space power levels: %w", err)
	}
	want := pl.Invite()
	have := pl.GetUserLevel(p.Client.UserID)
	if have < want {
		return fmt.Errorf("bot power level %d in target space < required invite level %d", have, want)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Space membership
// ---------------------------------------------------------------------------

// discoverSpaces returns the set of Space room IDs the bot is currently joined to.
func (p *SpaceInviterPlugin) discoverSpaces(ctx context.Context) []id.RoomID {
	resp, err := p.Client.JoinedRooms(ctx)
	if err != nil {
		slog.Error("space_inviter: list joined rooms", "err", err)
		return nil
	}
	spaces := make([]id.RoomID, 0)
	for _, room := range resp.JoinedRooms {
		var ce event.CreateEventContent
		if err := p.Client.StateEvent(ctx, room, event.StateCreate, "", &ce); err != nil {
			continue
		}
		if ce.Type == event.RoomTypeSpace {
			spaces = append(spaces, room)
		}
	}
	return spaces
}

// usersInAnySpace returns the union of joined+invited members across every
// Space the bot is in. Pending invites are read from the room state.
func (p *SpaceInviterPlugin) usersInAnySpace(ctx context.Context) map[id.UserID]struct{} {
	out := make(map[id.UserID]struct{})
	for _, space := range p.discoverSpaces(ctx) {
		// Joined members
		jm, err := p.Client.JoinedMembers(ctx, space)
		if err != nil {
			slog.Warn("space_inviter: joined members", "space", space, "err", err)
		} else {
			for uid := range jm.Joined {
				out[uid] = struct{}{}
			}
		}
		// Pending invites — pull all m.room.member state events
		members, err := p.Client.Members(ctx, space)
		if err != nil {
			slog.Warn("space_inviter: members state", "space", space, "err", err)
			continue
		}
		for _, evt := range members.Chunk {
			mem := evt.Content.AsMember()
			if mem == nil {
				continue
			}
			if mem.Membership == event.MembershipInvite {
				out[id.UserID(evt.GetStateKey())] = struct{}{}
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Synapse admin API: enumerate local users
// ---------------------------------------------------------------------------

type synapseUser struct {
	Name        string `json:"name"`
	UserType    string `json:"user_type"`
	Deactivated int    `json:"deactivated"`
	IsGuest     int    `json:"is_guest"`
	Admin       int    `json:"admin"`
}

func (p *SpaceInviterPlugin) listLocalUsers(ctx context.Context) ([]synapseUser, error) {
	out := []synapseUser{}
	from := 0
	const limit = 100
	for {
		u := fmt.Sprintf("%s/_synapse/admin/v2/users?guests=false&deactivated=false&from=%d&limit=%d",
			p.cfg.SynapseBaseURL, from, limit)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+p.cfg.AdminAccessToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("admin api: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("admin api status %d: %s", resp.StatusCode, string(body))
		}
		var page struct {
			Users     []synapseUser `json:"users"`
			NextToken string        `json:"next_token"`
			Total     int           `json:"total"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("admin api decode: %w", err)
		}
		out = append(out, page.Users...)
		if page.NextToken == "" {
			break
		}
		n, err := strconv.Atoi(page.NextToken)
		if err != nil {
			break
		}
		from = n
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// User filter
// ---------------------------------------------------------------------------

func (p *SpaceInviterPlugin) eligibleUser(uid id.UserID, userType string, isGuest, deactivated int) bool {
	if uid == "" {
		return false
	}
	if !strings.HasSuffix(string(uid), ":"+p.cfg.HomeDomain) {
		return false
	}
	if uid == p.Client.UserID || uid == p.cfg.AdminUserID {
		return false
	}
	if userType != "" {
		// Synapse marks bots/appservice puppets with non-empty user_type
		// (e.g. "bot", "support", or appservice-set values). Skip them.
		return false
	}
	if isGuest != 0 || deactivated != 0 {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Eligibility (DB-backed)
// ---------------------------------------------------------------------------

// dbEligible returns true if a fresh prompt may be sent for user. Encodes the
// pending / ignore / cooldown rules from the spec.
func (p *SpaceInviterPlugin) dbEligible(uid id.UserID) bool {
	cooldownCutoff := time.Now().Add(-p.cfg.RepromptCooldown).Unix()
	row := db.Get().QueryRow(`
		SELECT 1 FROM space_inviter_prompts
		WHERE user_id = ?
		  AND (reply IS NULL OR reply = 'ignore' OR prompt_sent_at > ?)
		LIMIT 1`,
		string(uid), cooldownCutoff)
	var n int
	err := row.Scan(&n)
	if err == nil {
		return false // a blocking row exists
	}
	return true
}

// hydratePending preloads the in-memory pending set from DB rows with NULL reply.
func (p *SpaceInviterPlugin) hydratePending() {
	rows, err := db.Get().Query(`SELECT user_id FROM space_inviter_prompts WHERE reply IS NULL`)
	if err != nil {
		slog.Warn("space_inviter: hydrate pending", "err", err)
		return
	}
	defer rows.Close()
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			p.pending[id.UserID(u)] = struct{}{}
		}
	}
}

// ---------------------------------------------------------------------------
// Startup sweep
// ---------------------------------------------------------------------------

func (p *SpaceInviterPlugin) runStartupSweep() {
	ctx := context.Background()
	users, err := p.listLocalUsers(ctx)
	if err != nil {
		slog.Error("space_inviter: startup sweep aborted", "err", err)
		return
	}
	inSpace := p.usersInAnySpace(ctx)

	totalEligible, prompts := 0, 0
	for _, u := range users {
		uid := id.UserID(u.Name)
		if !p.eligibleUser(uid, u.UserType, u.IsGuest, u.Deactivated) {
			continue
		}
		totalEligible++
		if _, ok := inSpace[uid]; ok {
			continue
		}
		if !p.dbEligible(uid) {
			continue
		}
		// Pending invite to target?
		if p.hasPendingInvite(ctx, uid) {
			continue
		}
		if err := p.sendPrompt(uid, "sweep"); err != nil {
			slog.Warn("space_inviter: send prompt failed", "user", uid, "err", err)
			continue
		}
		prompts++
		time.Sleep(p.cfg.InviteDelay)
	}
	slog.Info("space_inviter: startup sweep complete",
		"total_local_users", len(users),
		"eligible_after_filter", totalEligible,
		"prompts_sent", prompts,
	)
}

// hasPendingInvite returns true if uid is currently invited to the target Space.
func (p *SpaceInviterPlugin) hasPendingInvite(ctx context.Context, uid id.UserID) bool {
	var mem event.MemberEventContent
	err := p.Client.StateEvent(ctx, p.cfg.TargetSpaceID, event.StateMember, string(uid), &mem)
	if err != nil {
		return false
	}
	return mem.Membership == event.MembershipInvite
}

// ---------------------------------------------------------------------------
// Reactive handler — called from main.go on m.room.member joins
// ---------------------------------------------------------------------------

// OnMemberJoin is invoked by the global member event handler in main.go for
// any join event. It only acts on local-domain users.
func (p *SpaceInviterPlugin) OnMemberJoin(uid id.UserID) {
	if !p.enabled {
		return
	}
	if !p.eligibleUser(uid, "", 0, 0) {
		return
	}

	p.pendingMu.Lock()
	if _, already := p.pending[uid]; already {
		p.pendingMu.Unlock()
		return
	}
	p.pendingMu.Unlock()

	if !p.dbEligible(uid) {
		return
	}

	ctx := context.Background()
	inSpace := p.usersInAnySpace(ctx)
	if _, ok := inSpace[uid]; ok {
		return
	}
	if p.hasPendingInvite(ctx, uid) {
		return
	}

	if err := p.sendPrompt(uid, "reactive"); err != nil {
		slog.Warn("space_inviter: reactive prompt failed", "user", uid, "err", err)
	}
}

// ---------------------------------------------------------------------------
// Prompt + reply
// ---------------------------------------------------------------------------

func (p *SpaceInviterPlugin) sendPrompt(uid id.UserID, trigger string) error {
	text := fmt.Sprintf(
		"🐝 `%s` isn't in any Space on the homeserver.\n"+
			"Want me to invite them to the target Space?\n\n"+
			"Reply with:\n"+
			"  `yes`    — invite them\n"+
			"  `no`     — skip for now (won't ask again for %s)\n"+
			"  `ignore` — never ask about this user again",
		uid, formatCooldown(p.cfg.RepromptCooldown),
	)
	evtID, err := p.SendMessageID(p.adminDMRoom, text)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = db.Get().Exec(`
		INSERT INTO space_inviter_prompts
		    (user_id, prompt_sent_at, trigger_kind, dm_room_id, dm_event_id)
		VALUES (?, ?, ?, ?, ?)`,
		string(uid), now, trigger, string(p.adminDMRoom), string(evtID),
	)
	if err != nil {
		return fmt.Errorf("record prompt: %w", err)
	}
	p.pendingMu.Lock()
	p.pending[uid] = struct{}{}
	p.pendingMu.Unlock()
	slog.Info("space_inviter: prompt_sent", "user", uid, "trigger", trigger, "event", evtID)
	return nil
}

func formatCooldown(d time.Duration) string {
	if d >= 24*time.Hour {
		return fmt.Sprintf("%d days", int(d.Hours())/24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return d.String()
}

// OnMessage handles admin replies in the DM room. Other messages pass through.
func (p *SpaceInviterPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		return nil
	}
	if ctx.RoomID != p.adminDMRoom {
		return nil
	}
	if ctx.Sender != p.cfg.AdminUserID {
		return nil
	}

	reply := strings.ToLower(strings.TrimSpace(ctx.Body))
	if reply != "yes" && reply != "no" && reply != "ignore" {
		return nil // silently ignore unknown replies
	}

	// Find the most recent pending prompt in this room. If the admin's message
	// is a Matrix reply, prefer the row matching that target event.
	targetEvent := ""
	if rel := ctx.Event.Content.AsMessage().RelatesTo; rel != nil && rel.InReplyTo != nil {
		targetEvent = string(rel.InReplyTo.EventID)
	}

	user, promptAt, err := p.findPendingPrompt(targetEvent)
	if err != nil {
		return nil // nothing pending
	}

	now := time.Now().Unix()
	_, _ = db.Get().Exec(`
		UPDATE space_inviter_prompts
		SET reply = ?, replied_at = ?
		WHERE user_id = ? AND prompt_sent_at = ?`,
		reply, now, string(user), promptAt,
	)
	p.pendingMu.Lock()
	delete(p.pending, user)
	p.pendingMu.Unlock()

	slog.Info("space_inviter: prompt_reply", "user", user, "reply", reply)

	if reply == "yes" {
		p.attemptInvite(user, promptAt)
	}
	return nil
}

func (p *SpaceInviterPlugin) findPendingPrompt(targetEvent string) (id.UserID, int64, error) {
	if targetEvent != "" {
		row := db.Get().QueryRow(`
			SELECT user_id, prompt_sent_at FROM space_inviter_prompts
			WHERE dm_event_id = ? AND reply IS NULL
			LIMIT 1`, targetEvent)
		var u string
		var t int64
		if err := row.Scan(&u, &t); err == nil {
			return id.UserID(u), t, nil
		}
	}
	// Fallback: most recent pending prompt in the admin DM.
	row := db.Get().QueryRow(`
		SELECT user_id, prompt_sent_at FROM space_inviter_prompts
		WHERE dm_room_id = ? AND reply IS NULL
		ORDER BY prompt_sent_at DESC
		LIMIT 1`, string(p.adminDMRoom))
	var u string
	var t int64
	if err := row.Scan(&u, &t); err != nil {
		return "", 0, err
	}
	return id.UserID(u), t, nil
}

func (p *SpaceInviterPlugin) attemptInvite(uid id.UserID, promptAt int64) {
	ctx := context.Background()

	// Re-validate at reply time: still Space-less, no pending invite.
	if p.hasPendingInvite(ctx, uid) {
		_, _ = db.Get().Exec(`UPDATE space_inviter_prompts SET invite_attempted=1, invite_outcome='skipped_race' WHERE user_id=? AND prompt_sent_at=?`,
			string(uid), promptAt)
		_ = p.SendMessage(p.adminDMRoom, fmt.Sprintf("ℹ️ `%s` already has a pending invite — skipped.", uid))
		return
	}
	if _, ok := p.usersInAnySpace(ctx)[uid]; ok {
		_, _ = db.Get().Exec(`UPDATE space_inviter_prompts SET invite_attempted=1, invite_outcome='skipped_race' WHERE user_id=? AND prompt_sent_at=?`,
			string(uid), promptAt)
		_ = p.SendMessage(p.adminDMRoom, fmt.Sprintf("ℹ️ `%s` is already in a Space — skipped.", uid))
		return
	}

	_, err := p.Client.InviteUser(ctx, p.cfg.TargetSpaceID, &mautrix.ReqInviteUser{UserID: uid})
	if err == nil {
		_, _ = db.Get().Exec(`UPDATE space_inviter_prompts SET invite_attempted=1, invite_outcome='ok' WHERE user_id=? AND prompt_sent_at=?`,
			string(uid), promptAt)
		_ = p.SendMessage(p.adminDMRoom, fmt.Sprintf("✅ Invited `%s` to the Space.", uid))
		slog.Info("space_inviter: invite_attempt", "user", uid, "outcome", "ok")
		return
	}

	errStr := err.Error()
	outcome := "error"
	if strings.Contains(errStr, "M_FORBIDDEN") {
		outcome = "forbidden"
		_ = p.SendMessage(p.adminDMRoom, fmt.Sprintf(
			"🚨 Tried to invite `%s` but got M_FORBIDDEN.\nMy power level may have dropped — check Space permissions.", uid))
	} else {
		_ = p.SendMessage(p.adminDMRoom, fmt.Sprintf("⚠️ Invite failed for `%s`: %s", uid, errStr))
	}
	_, _ = db.Get().Exec(`UPDATE space_inviter_prompts SET invite_attempted=1, invite_outcome=?, invite_error=? WHERE user_id=? AND prompt_sent_at=?`,
		outcome, errStr, string(uid), promptAt)
	slog.Error("space_inviter: invite_attempt", "user", uid, "outcome", outcome, "err", errStr)
}
