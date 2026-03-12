package plugin

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

type modConfig struct {
	// Word list
	WordListPath       string
	WordListVariations bool

	// Strike system
	StrikeExpiryDays    int
	MuteDurationMinutes int
	MaxStrikes          int

	// Text flood
	FloodMessageCount         int
	FloodMessageWindowSeconds int

	// Image/file flood
	FloodImageCount         int
	FloodImageWindowSeconds int

	// Wall of text
	MaxMessageLength int

	// Repeated messages
	RepeatCount               int
	RepeatWindowSeconds       int
	RepeatSimilarityThreshold float64

	// Mention flood
	MentionMax              int
	MentionFloodCount       int
	MentionFloodWindowSeconds int

	// Link rate (new members)
	LinkRateNewMember        int
	LinkRateWindowSeconds    int

	// Invite flooding
	InviteMaxPerHour int

	// Join/leave cycling
	JoinLeaveCount         int
	JoinLeaveWindowMinutes int

	// New member
	NewMemberDays            int
	NewMemberFloodMultiplier float64

	// Admin
	AdminRoom  id.RoomID
	DMOnAction bool
}

func loadModConfig() modConfig {
	return modConfig{
		WordListPath:       envOrDefault("MOD_WORDLIST", ""),
		WordListVariations: envOrDefault("MOD_WORDLIST_VARIATIONS", "true") == "true",

		StrikeExpiryDays:    envInt("MOD_STRIKE_EXPIRY_DAYS", 30),
		MuteDurationMinutes: envInt("MOD_MUTE_DURATION_MINUTES", 60),
		MaxStrikes:          envInt("MOD_MAX_STRIKES", 3),

		FloodMessageCount:         envInt("MOD_FLOOD_MESSAGE_COUNT", 5),
		FloodMessageWindowSeconds: envInt("MOD_FLOOD_MESSAGE_WINDOW_SECONDS", 10),

		FloodImageCount:         envInt("MOD_FLOOD_IMAGE_COUNT", 3),
		FloodImageWindowSeconds: envInt("MOD_FLOOD_IMAGE_WINDOW_SECONDS", 30),

		MaxMessageLength: envInt("MOD_MAX_MESSAGE_LENGTH", 2000),

		RepeatCount:               envInt("MOD_REPEAT_COUNT", 3),
		RepeatWindowSeconds:       envInt("MOD_REPEAT_WINDOW_SECONDS", 60),
		RepeatSimilarityThreshold: envFloat("MOD_REPEAT_SIMILARITY_THRESHOLD", 0.85),

		MentionMax:                envInt("MOD_MENTION_MAX", 5),
		MentionFloodCount:         envInt("MOD_MENTION_FLOOD_COUNT", 3),
		MentionFloodWindowSeconds: envInt("MOD_MENTION_FLOOD_WINDOW_SECONDS", 30),

		LinkRateNewMember:     envInt("MOD_LINK_RATE_NEW_MEMBER", 3),
		LinkRateWindowSeconds: envInt("MOD_LINK_RATE_WINDOW_SECONDS", 60),

		InviteMaxPerHour: envInt("MOD_INVITE_MAX_PER_HOUR", 5),

		JoinLeaveCount:         envInt("MOD_JOIN_LEAVE_COUNT", 3),
		JoinLeaveWindowMinutes: envInt("MOD_JOIN_LEAVE_WINDOW_MINUTES", 10),

		NewMemberDays:            envInt("MOD_NEW_MEMBER_DAYS", 14),
		NewMemberFloodMultiplier: envFloat("MOD_NEW_MEMBER_FLOOD_MULTIPLIER", 0.5),

		AdminRoom:  id.RoomID(envOrDefault("MOD_ADMIN_ROOM", "")),
		DMOnAction: envOrDefault("MOD_DM_ON_ACTION", "true") == "true",
	}
}

// ---------------------------------------------------------------------------
// DM Templates
// ---------------------------------------------------------------------------

var dmTemplates = map[string]string{
	"strike1_word":  "Hey — that word isn't something we allow in this community. Your message has been removed. This has been noted. Please keep it in mind going forward.",
	"strike1_flood": "Hey — looks like you might have gotten a bit trigger-happy there. Slow it down a little. Your messages have been removed and this has been noted.",
	"strike1_wall":  "Hey — that message was way too long for this room. It's been removed and noted. Please keep it shorter.",
	"strike1_repeat": "Hey — repeating the same message isn't cool. Your messages have been removed and this has been noted.",
	"strike1_mention": "Hey — too many mentions in a short time. Please don't mass-ping. This has been noted.",
	"strike1_link":  "Hey — slow down on the links. New members have a limit. This has been noted.",
	"strike1_invite": "Hey — you've been sending too many invites. This has been flagged and noted.",
	"strike2":       "This is the second time we've had to step in. You've been muted for %s. When you're back, please keep the community guidelines in mind. This is your last warning before a permanent ban.",
	"strike3":       "This is the third strike. You're being removed from the community permanently. Take care.",
	"manual_warn":   "A community admin has issued you a formal warning: %s. Please keep this in mind.",
}

// ---------------------------------------------------------------------------
// Sliding window tracker
// ---------------------------------------------------------------------------

type windowKey struct {
	UserID id.UserID
	RoomID id.RoomID
}

type slidingWindow struct {
	mu      sync.Mutex
	entries map[windowKey][]time.Time
}

func newSlidingWindow() *slidingWindow {
	return &slidingWindow{entries: make(map[windowKey][]time.Time)}
}

func (sw *slidingWindow) add(userID id.UserID, roomID id.RoomID) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	k := windowKey{userID, roomID}
	sw.entries[k] = append(sw.entries[k], time.Now())
}

func (sw *slidingWindow) count(userID id.UserID, roomID id.RoomID, window time.Duration) int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	k := windowKey{userID, roomID}
	cutoff := time.Now().Add(-window)
	// prune and count
	kept := sw.entries[k][:0]
	for _, t := range sw.entries[k] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	sw.entries[k] = kept
	return len(kept)
}

// messageWindow tracks recent message text for repeat detection
type messageWindow struct {
	mu      sync.Mutex
	entries map[windowKey][]msgEntry
}

type msgEntry struct {
	text string
	at   time.Time
}

func newMessageWindow() *messageWindow {
	return &messageWindow{entries: make(map[windowKey][]msgEntry)}
}

func (mw *messageWindow) addAndCheck(userID id.UserID, roomID id.RoomID, text string, window time.Duration, threshold float64, count int) bool {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	k := windowKey{userID, roomID}
	cutoff := time.Now().Add(-window)

	// prune old entries
	kept := mw.entries[k][:0]
	for _, e := range mw.entries[k] {
		if e.at.After(cutoff) {
			kept = append(kept, e)
		}
	}

	// count similar
	similar := 0
	for _, e := range kept {
		if normalizedSimilarity(e.text, text) >= threshold {
			similar++
		}
	}

	kept = append(kept, msgEntry{text: text, at: time.Now()})
	mw.entries[k] = kept

	return similar >= count-1 // current message is the Nth
}

// ---------------------------------------------------------------------------
// Word list
// ---------------------------------------------------------------------------

type wordList struct {
	mu            sync.RWMutex
	words         []string
	variationPats []*regexp.Regexp // precompiled variation patterns (1:1 with words)
	variations    bool
}

func newWordList(path string, variations bool) *wordList {
	wl := &wordList{variations: variations}
	if path != "" {
		wl.load(path)
	}
	return wl
}

func (wl *wordList) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var words []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			words = append(words, strings.ToLower(line))
		}
	}

	// Precompile variation patterns
	var pats []*regexp.Regexp
	if wl.variations {
		for _, word := range words {
			pat := buildVariationPattern(word)
			if re, err := regexp.Compile(pat); err == nil {
				pats = append(pats, re)
			} else {
				pats = append(pats, nil)
			}
		}
	}

	wl.mu.Lock()
	wl.words = words
	wl.variationPats = pats
	wl.mu.Unlock()

	slog.Info("moderation: word list loaded", "count", len(words), "path", path)
	return scanner.Err()
}

func (wl *wordList) check(text string) (bool, string) {
	wl.mu.RLock()
	defer wl.mu.RUnlock()

	normalized := normalizeText(text)

	for i, word := range wl.words {
		if containsWord(normalized, word) {
			return true, word
		}
		if wl.variations && i < len(wl.variationPats) && wl.variationPats[i] != nil {
			if wl.variationPats[i].MatchString(normalized) {
				return true, word
			}
		}
	}
	return false, ""
}

func (wl *wordList) count() int {
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	return len(wl.words)
}

// normalizeText lowercases and strips non-alphanumeric (keeping spaces)
func normalizeText(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// containsWord checks for whole-word match
func containsWord(text, word string) bool {
	idx := strings.Index(text, word)
	for idx >= 0 {
		before := idx == 0 || text[idx-1] == ' '
		after := idx+len(word) >= len(text) || text[idx+len(word)] == ' '
		if before && after {
			return true
		}
		next := strings.Index(text[idx+1:], word)
		if next < 0 {
			break
		}
		idx = idx + 1 + next
	}
	return false
}

// buildVariationPattern builds a regex for leetspeak and character-separated variants.
func buildVariationPattern(word string) string {
	var pattern strings.Builder
	pattern.WriteString(`(?i)`)
	for i, ch := range strings.ToLower(word) {
		if i > 0 {
			pattern.WriteString(`[\s.\-_]*`)
		}
		switch ch {
		case 'a':
			pattern.WriteString(`[a4@]+`)
		case 'e':
			pattern.WriteString(`[e3]+`)
		case 'i':
			pattern.WriteString(`[i1!|]+`)
		case 'o':
			pattern.WriteString(`[o0]+`)
		case 's':
			pattern.WriteString(`[s5$]+`)
		case 'l':
			pattern.WriteString(`[l1|]+`)
		case 't':
			pattern.WriteString(`[t7+]+`)
		default:
			pattern.WriteRune(ch)
			pattern.WriteString(`+`)
		}
	}
	return pattern.String()
}

// ---------------------------------------------------------------------------
// Similarity (Levenshtein-based)
// ---------------------------------------------------------------------------

func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func normalizedSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return 1.0
	}
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshteinDistance(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}

// ---------------------------------------------------------------------------
// Mention / link extraction
// ---------------------------------------------------------------------------

var modLinkRe = regexp.MustCompile(`https?://\S+`)

func modCountMentions(text string) int {
	return len(mentionRe.FindAllString(text, -1))
}

func modCountLinks(text string) int {
	return len(modLinkRe.FindAllString(text, -1))
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type ModerationPlugin struct {
	Base
	cfg     modConfig
	wl      *wordList
	enabled bool

	// Sliding windows (in-memory, reset on restart)
	textFlood    *slidingWindow
	imageFlood   *slidingWindow
	mentionFlood *slidingWindow
	linkRate     *slidingWindow
	inviteFlood  *slidingWindow
	joinLeave    *slidingWindow
	repeatMsgs   *messageWindow

	// Mute scheduler
	muteMu    sync.Mutex
	muteTimer map[windowKey]*time.Timer
}

func NewModerationPlugin(client *mautrix.Client) *ModerationPlugin {
	cfg := loadModConfig()
	enabled := strings.ToLower(envOrDefault("FEATURE_MODERATION", "")) != "" &&
		strings.ToLower(envOrDefault("FEATURE_MODERATION", "")) != "false"

	if !enabled {
		slog.Info("moderation: disabled (set FEATURE_MODERATION=true to enable)")
	}

	return &ModerationPlugin{
		Base:         Base{Client: client},
		cfg:          cfg,
		enabled:      enabled,
		wl:           newWordList(cfg.WordListPath, cfg.WordListVariations),
		textFlood:    newSlidingWindow(),
		imageFlood:   newSlidingWindow(),
		mentionFlood: newSlidingWindow(),
		linkRate:     newSlidingWindow(),
		inviteFlood:  newSlidingWindow(),
		joinLeave:    newSlidingWindow(),
		repeatMsgs:   newMessageWindow(),
		muteTimer:    make(map[windowKey]*time.Timer),
	}
}

func (p *ModerationPlugin) Name() string { return "moderation" }

func (p *ModerationPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "mod", Description: "Moderation commands (admin only)", Usage: "!mod <warn|mute|unmute|ban|strikes|forgive|history|reload|status|test> [args]", AdminOnly: true, Category: "Admin"},
	}
}

func (p *ModerationPlugin) Init() error {
	if !p.enabled {
		return nil
	}

	// Validate admin room
	if p.cfg.AdminRoom != "" {
		_, err := p.Client.JoinedMembers(context.Background(), p.cfg.AdminRoom)
		if err != nil {
			slog.Warn("moderation: bot not in admin room, admin notifications disabled", "room", p.cfg.AdminRoom, "err", err)
			p.cfg.AdminRoom = ""
		}
	}
	return nil
}

func (p *ModerationPlugin) OnReaction(_ ReactionContext) error { return nil }

// OnMessage handles both detection and admin commands.
func (p *ModerationPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		return nil
	}

	// Admin commands first
	if p.IsCommand(ctx.Body, "mod") {
		if !p.IsAdmin(ctx.Sender) {
			return nil
		}
		return p.handleModCommand(ctx)
	}

	// Run detectors (skip admins)
	if p.IsAdmin(ctx.Sender) {
		return nil
	}

	return p.runDetectors(ctx)
}

// OnMemberEvent is called from main.go for join/leave/invite tracking.
func (p *ModerationPlugin) OnMemberEvent(roomID id.RoomID, userID id.UserID, membership event.Membership) {
	if !p.enabled {
		return
	}
	if p.IsAdmin(userID) || userID == p.Client.UserID {
		return
	}

	switch membership {
	case event.MembershipInvite:
		// Track invite flooding (invites issued BY this user)
		p.inviteFlood.add(userID, roomID)
		window := time.Hour
		cnt := p.inviteFlood.count(userID, roomID, window)
		if cnt >= p.cfg.InviteMaxPerHour {
			p.issueStrike(userID, roomID, id.EventID(""), "invite flooding", fmt.Sprintf("%d invites in 1 hour", cnt))
		}

	case event.MembershipJoin, event.MembershipLeave:
		p.joinLeave.add(userID, roomID)
		window := time.Duration(p.cfg.JoinLeaveWindowMinutes) * time.Minute
		cnt := p.joinLeave.count(userID, roomID, window)
		if cnt >= p.cfg.JoinLeaveCount {
			// Flag only — no strike
			p.notifyAdmin(fmt.Sprintf(
				"👀 **Join/Leave Pattern Detected**\nUser: `%s`\nRoom: `%s`\nEvents: %d join/leave cycles in %d minutes\nNo action taken. Monitor or use `!mod warn` if needed.",
				userID, roomID, cnt, p.cfg.JoinLeaveWindowMinutes,
			))
		}
	}
}

// ---------------------------------------------------------------------------
// Detection pipeline
// ---------------------------------------------------------------------------

func (p *ModerationPlugin) runDetectors(ctx MessageContext) error {
	isNew := p.isNewMember(ctx.Sender)

	// 1. Word list
	if hit, word := p.wl.check(ctx.Body); hit {
		return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "prohibited word", fmt.Sprintf("matched: %s", word))
	}

	// 2. Text flood
	p.textFlood.add(ctx.Sender, ctx.RoomID)
	floodThreshold := p.cfg.FloodMessageCount
	if isNew {
		floodThreshold = p.adjustThreshold(floodThreshold)
	}
	floodWindow := time.Duration(p.cfg.FloodMessageWindowSeconds) * time.Second
	if p.textFlood.count(ctx.Sender, ctx.RoomID, floodWindow) >= floodThreshold {
		return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "text flood", fmt.Sprintf("%d+ messages in %ds", floodThreshold, p.cfg.FloodMessageWindowSeconds))
	}

	// 3. Image/file flood
	if ctx.Event != nil {
		msg := ctx.Event.Content.AsMessage()
		if msg != nil && (msg.MsgType == event.MsgImage || msg.MsgType == event.MsgFile || msg.MsgType == event.MsgVideo) {
			p.imageFlood.add(ctx.Sender, ctx.RoomID)
			imgThreshold := p.cfg.FloodImageCount
			if isNew {
				imgThreshold = p.adjustThreshold(imgThreshold)
			}
			imgWindow := time.Duration(p.cfg.FloodImageWindowSeconds) * time.Second
			if p.imageFlood.count(ctx.Sender, ctx.RoomID, imgWindow) >= imgThreshold {
				return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "image/file flood", fmt.Sprintf("%d+ files in %ds", imgThreshold, p.cfg.FloodImageWindowSeconds))
			}
		}
	}

	// 4. Wall of text
	if p.cfg.MaxMessageLength > 0 && len(ctx.Body) > p.cfg.MaxMessageLength {
		return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "wall of text", fmt.Sprintf("%d chars (limit %d)", len(ctx.Body), p.cfg.MaxMessageLength))
	}

	// 5. Repeated messages
	repeatThreshold := p.cfg.RepeatCount
	if isNew {
		repeatThreshold = p.adjustThreshold(repeatThreshold)
	}
	repeatWindow := time.Duration(p.cfg.RepeatWindowSeconds) * time.Second
	if p.repeatMsgs.addAndCheck(ctx.Sender, ctx.RoomID, ctx.Body, repeatWindow, p.cfg.RepeatSimilarityThreshold, repeatThreshold) {
		return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "repeated messages", fmt.Sprintf("%d+ similar messages in %ds", repeatThreshold, p.cfg.RepeatWindowSeconds))
	}

	// 6. Mention flood
	mentions := modCountMentions(ctx.Body)
	if mentions >= p.cfg.MentionMax {
		return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "mention flood", fmt.Sprintf("%d mentions in one message", mentions))
	}
	if mentions > 0 {
		p.mentionFlood.add(ctx.Sender, ctx.RoomID)
		mentionThreshold := p.cfg.MentionFloodCount
		if isNew {
			mentionThreshold = p.adjustThreshold(mentionThreshold)
		}
		mentionWindow := time.Duration(p.cfg.MentionFloodWindowSeconds) * time.Second
		if p.mentionFlood.count(ctx.Sender, ctx.RoomID, mentionWindow) >= mentionThreshold {
			return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "mention flood", fmt.Sprintf("%d+ mention-heavy messages in %ds", mentionThreshold, p.cfg.MentionFloodWindowSeconds))
		}
	}

	// 7. Link rate (new members only)
	if isNew {
		links := modCountLinks(ctx.Body)
		if links > 0 {
			for range links {
				p.linkRate.add(ctx.Sender, ctx.RoomID)
			}
			linkWindow := time.Duration(p.cfg.LinkRateWindowSeconds) * time.Second
			if p.linkRate.count(ctx.Sender, ctx.RoomID, linkWindow) > p.cfg.LinkRateNewMember {
				return p.issueStrike(ctx.Sender, ctx.RoomID, ctx.EventID, "link rate exceeded", fmt.Sprintf("%d+ links in %ds (new member)", p.cfg.LinkRateNewMember, p.cfg.LinkRateWindowSeconds))
			}
		}
	}

	return nil
}

func (p *ModerationPlugin) adjustThreshold(threshold int) int {
	adjusted := int(math.Ceil(float64(threshold) * p.cfg.NewMemberFloodMultiplier))
	if adjusted < 1 {
		adjusted = 1
	}
	return adjusted
}

func (p *ModerationPlugin) isNewMember(userID id.UserID) bool {
	d := db.Get()
	var firstSeen sql.NullInt64
	_ = d.QueryRow("SELECT MIN(first_seen) FROM user_stats WHERE user_id = ?", string(userID)).Scan(&firstSeen)
	if !firstSeen.Valid {
		return true // never seen = treat as new
	}
	return time.Since(time.Unix(firstSeen.Int64, 0)) < time.Duration(p.cfg.NewMemberDays)*24*time.Hour
}

// ---------------------------------------------------------------------------
// Strike system
// ---------------------------------------------------------------------------

func (p *ModerationPlugin) issueStrike(userID id.UserID, roomID id.RoomID, eventID id.EventID, trigger, detail string) error {
	d := db.Get()
	expiresAt := time.Now().Add(time.Duration(p.cfg.StrikeExpiryDays) * 24 * time.Hour)

	// Insert strike
	_, err := d.Exec(
		`INSERT INTO mod_strikes (user_id, room_id, issued_at, expires_at, reason, issued_by)
		 VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?, 'gogobee')`,
		string(userID), string(roomID), expiresAt.Format("2006-01-02 15:04:05"),
		fmt.Sprintf("%s: %s", trigger, detail),
	)
	if err != nil {
		slog.Error("moderation: failed to insert strike", "err", err)
		return err
	}

	// Count active strikes
	activeStrikes := p.countActiveStrikes(userID)

	slog.Info("moderation: strike issued",
		"user", userID, "room", roomID, "trigger", trigger,
		"detail", detail, "total_active", activeStrikes)

	// Redact the message
	if eventID != "" {
		p.redactMessage(roomID, eventID, trigger)
	}

	// Log the action
	p.logAction(userID, roomID, "strike", fmt.Sprintf("strike %d: %s — %s", activeStrikes, trigger, detail), "gogobee")

	// Response ladder
	switch {
	case activeStrikes >= p.cfg.MaxStrikes:
		// Strike 3: ban
		p.dmUser(userID, dmTemplates["strike3"])
		p.banUser(userID, roomID, fmt.Sprintf("strike %d: %s", activeStrikes, trigger))
		p.logAction(userID, roomID, "ban", fmt.Sprintf("automatic: strike %d", activeStrikes), "gogobee")
		p.notifyAdmin(fmt.Sprintf(
			"🚨 **Moderation Action**\nUser: `%s`\nRoom: `%s`\nTrigger: %s\nStrike: %d of %d\nAction taken: **Permanent ban**\nDetail: %s",
			userID, roomID, trigger, activeStrikes, p.cfg.MaxStrikes, modTruncate(detail, 200),
		))

	case activeStrikes == p.cfg.MaxStrikes-1:
		// Strike 2: mute
		duration := time.Duration(p.cfg.MuteDurationMinutes) * time.Minute
		p.dmUser(userID, fmt.Sprintf(dmTemplates["strike2"], modFormatDuration(duration)))
		p.muteUser(userID, roomID, duration)
		p.logAction(userID, roomID, "mute", fmt.Sprintf("automatic: strike %d, %s", activeStrikes, modFormatDuration(duration)), "gogobee")
		p.notifyAdmin(fmt.Sprintf(
			"🚨 **Moderation Action**\nUser: `%s`\nRoom: `%s`\nTrigger: %s\nStrike: %d of %d (expires %s)\nAction taken: Temp mute (%s)\nDetail: %s",
			userID, roomID, trigger, activeStrikes, p.cfg.MaxStrikes,
			expiresAt.Format("2006-01-02"), modFormatDuration(duration), modTruncate(detail, 200),
		))

	default:
		// Strike 1: warn
		tmpl := "strike1_flood"
		switch trigger {
		case "prohibited word":
			tmpl = "strike1_word"
		case "wall of text":
			tmpl = "strike1_wall"
		case "repeated messages":
			tmpl = "strike1_repeat"
		case "mention flood":
			tmpl = "strike1_mention"
		case "link rate exceeded":
			tmpl = "strike1_link"
		case "invite flooding":
			tmpl = "strike1_invite"
		}
		p.dmUser(userID, dmTemplates[tmpl])
		p.logAction(userID, roomID, "warn", fmt.Sprintf("automatic: strike %d — %s", activeStrikes, trigger), "gogobee")
		p.notifyAdmin(fmt.Sprintf(
			"🚨 **Moderation Action**\nUser: `%s`\nRoom: `%s`\nTrigger: %s\nStrike: %d of %d (expires %s)\nAction taken: Warning + redact\nDetail: %s",
			userID, roomID, trigger, activeStrikes, p.cfg.MaxStrikes,
			expiresAt.Format("2006-01-02"), modTruncate(detail, 200),
		))
	}

	return nil
}

func (p *ModerationPlugin) countActiveStrikes(userID id.UserID) int {
	d := db.Get()
	var count int
	_ = d.QueryRow(
		`SELECT COUNT(*) FROM mod_strikes WHERE user_id = ? AND active = TRUE AND expires_at > CURRENT_TIMESTAMP`,
		string(userID),
	).Scan(&count)
	return count
}

// ---------------------------------------------------------------------------
// Actions: redact, mute, unmute, ban
// ---------------------------------------------------------------------------

func (p *ModerationPlugin) redactMessage(roomID id.RoomID, eventID id.EventID, reason string) {
	_, err := p.Client.RedactEvent(context.Background(), roomID, eventID, mautrix.ReqRedact{Reason: reason})
	if err != nil {
		slog.Error("moderation: failed to redact", "room", roomID, "event", eventID, "err", err)
	}
}

func (p *ModerationPlugin) muteUser(userID id.UserID, roomID id.RoomID, duration time.Duration) {
	ctx := context.Background()

	// Get current power levels
	pl := &event.PowerLevelsEventContent{}
	if err := p.Client.StateEvent(ctx, roomID, event.StatePowerLevels, "", pl); err != nil {
		slog.Error("moderation: failed to get power levels", "room", roomID, "err", err)
		return
	}

	// Store original level for restore
	originalLevel := pl.GetUserLevel(userID)

	// Set user power level to -1 (below events_default which is typically 0)
	pl.SetUserLevel(userID, -1)
	if _, err := p.Client.SendStateEvent(ctx, roomID, event.StatePowerLevels, "", pl); err != nil {
		slog.Error("moderation: failed to mute user", "user", userID, "room", roomID, "err", err)
		return
	}

	slog.Info("moderation: user muted", "user", userID, "room", roomID, "duration", duration)

	// Schedule unmute
	p.muteMu.Lock()
	k := windowKey{userID, roomID}
	if existing, ok := p.muteTimer[k]; ok {
		existing.Stop()
	}
	p.muteTimer[k] = time.AfterFunc(duration, func() {
		p.unmuteUser(userID, roomID, originalLevel)
		p.muteMu.Lock()
		delete(p.muteTimer, k)
		p.muteMu.Unlock()
	})
	p.muteMu.Unlock()
}

func (p *ModerationPlugin) unmuteUser(userID id.UserID, roomID id.RoomID, restoreLevel int) {
	ctx := context.Background()

	pl := &event.PowerLevelsEventContent{}
	if err := p.Client.StateEvent(ctx, roomID, event.StatePowerLevels, "", pl); err != nil {
		slog.Error("moderation: failed to get power levels for unmute", "err", err)
		return
	}

	pl.SetUserLevel(userID, restoreLevel)
	if _, err := p.Client.SendStateEvent(ctx, roomID, event.StatePowerLevels, "", pl); err != nil {
		slog.Error("moderation: failed to unmute user", "user", userID, "room", roomID, "err", err)
		return
	}

	slog.Info("moderation: user unmuted", "user", userID, "room", roomID)
	p.logAction(userID, roomID, "unmute", "automatic: mute expired", "gogobee")
	p.notifyAdmin(fmt.Sprintf("🔊 **Unmuted** `%s` in `%s` (mute expired)", userID, roomID))
}

func (p *ModerationPlugin) banUser(userID id.UserID, roomID id.RoomID, reason string) {
	_, err := p.Client.BanUser(context.Background(), roomID, &mautrix.ReqBanUser{
		UserID: userID,
		Reason: reason,
	})
	if err != nil {
		slog.Error("moderation: failed to ban user", "user", userID, "room", roomID, "err", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *ModerationPlugin) dmUser(userID id.UserID, text string) {
	if !p.cfg.DMOnAction {
		return
	}
	if err := p.SendDM(userID, text); err != nil {
		slog.Error("moderation: failed to DM user", "user", userID, "err", err)
	}
}

func (p *ModerationPlugin) notifyAdmin(text string) {
	if p.cfg.AdminRoom == "" {
		return
	}
	if err := p.SendMessage(p.cfg.AdminRoom, text); err != nil {
		slog.Error("moderation: failed to notify admin room", "err", err)
	}
}

func (p *ModerationPlugin) logAction(userID id.UserID, roomID id.RoomID, action, reason, takenBy string) {
	d := db.Get()
	_, err := d.Exec(
		`INSERT INTO mod_actions (user_id, room_id, action, reason, taken_at, taken_by)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		string(userID), string(roomID), action, reason, takenBy,
	)
	if err != nil {
		slog.Error("moderation: failed to log action", "err", err)
	}
}

func modTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func modFormatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%d hour(s)", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// ---------------------------------------------------------------------------
// Admin commands
// ---------------------------------------------------------------------------

func (p *ModerationPlugin) handleModCommand(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "mod")
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!mod <warn|mute|unmute|ban|strikes|forgive|history|reload|status|test> [args]`")
	}

	parts := strings.Fields(args)
	sub := strings.ToLower(parts[0])
	rest := ""
	if len(parts) > 1 {
		rest = strings.Join(parts[1:], " ")
	}

	switch sub {
	case "warn":
		return p.handleWarn(ctx, rest)
	case "mute":
		return p.handleMute(ctx, rest)
	case "unmute":
		return p.handleUnmute(ctx, rest)
	case "ban":
		return p.handleBan(ctx, rest)
	case "strikes":
		return p.handleStrikes(ctx, rest)
	case "forgive":
		return p.handleForgive(ctx, rest)
	case "history":
		return p.handleHistory(ctx, rest)
	case "reload":
		return p.handleReload(ctx)
	case "status":
		return p.handleStatus(ctx)
	case "test":
		return p.handleTest(ctx, rest)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Unknown subcommand. Available: warn, mute, unmute, ban, strikes, forgive, history, reload, status, test")
	}
}

func (p *ModerationPlugin) handleWarn(ctx MessageContext, args string) error {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod warn @user [reason]`")
	}

	targetID, ok := p.ResolveUser(parts[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	reason := "No reason provided"
	if len(parts) > 1 {
		reason = parts[1]
	}

	d := db.Get()
	expiresAt := time.Now().Add(time.Duration(p.cfg.StrikeExpiryDays) * 24 * time.Hour)
	_, err := d.Exec(
		`INSERT INTO mod_strikes (user_id, room_id, issued_at, expires_at, reason, issued_by)
		 VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?, ?)`,
		string(targetID), string(ctx.RoomID), expiresAt.Format("2006-01-02 15:04:05"),
		"manual warn: "+reason, string(ctx.Sender),
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to issue strike.")
	}

	activeStrikes := p.countActiveStrikes(targetID)
	p.logAction(targetID, ctx.RoomID, "warn", reason, string(ctx.Sender))
	p.dmUser(targetID, fmt.Sprintf(dmTemplates["manual_warn"], reason))

	// Apply ladder
	switch {
	case activeStrikes >= p.cfg.MaxStrikes:
		p.dmUser(targetID, dmTemplates["strike3"])
		p.banUser(targetID, ctx.RoomID, fmt.Sprintf("manual warn strike %d: %s", activeStrikes, reason))
		p.logAction(targetID, ctx.RoomID, "ban", fmt.Sprintf("manual warn triggered ban at strike %d", activeStrikes), string(ctx.Sender))
	case activeStrikes == p.cfg.MaxStrikes-1:
		duration := time.Duration(p.cfg.MuteDurationMinutes) * time.Minute
		p.dmUser(targetID, fmt.Sprintf(dmTemplates["strike2"], modFormatDuration(duration)))
		p.muteUser(targetID, ctx.RoomID, duration)
		p.logAction(targetID, ctx.RoomID, "mute", fmt.Sprintf("manual warn triggered mute at strike %d", activeStrikes), string(ctx.Sender))
	}

	p.notifyAdmin(fmt.Sprintf(
		"⚠️ **Manual Warning**\nUser: `%s`\nRoom: `%s`\nIssued by: `%s`\nReason: %s\nActive strikes: %d/%d",
		targetID, ctx.RoomID, ctx.Sender, reason, activeStrikes, p.cfg.MaxStrikes,
	))

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Warning issued to `%s`. Active strikes: %d/%d.", targetID, activeStrikes, p.cfg.MaxStrikes))
}

func (p *ModerationPlugin) handleMute(ctx MessageContext, args string) error {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod mute @user [duration]` (default: configured mute duration)")
	}

	targetID, ok := p.ResolveUser(parts[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	duration := time.Duration(p.cfg.MuteDurationMinutes) * time.Minute
	if len(parts) > 1 {
		d, err := parseDuration(parts[1])
		if err != nil {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Invalid duration. Use format like `30m`, `2h`, `60`.")
		}
		duration = d
	}

	p.muteUser(targetID, ctx.RoomID, duration)
	p.logAction(targetID, ctx.RoomID, "mute", fmt.Sprintf("manual: %s", modFormatDuration(duration)), string(ctx.Sender))
	p.dmUser(targetID, fmt.Sprintf("You have been muted for %s by an admin.", modFormatDuration(duration)))
	p.notifyAdmin(fmt.Sprintf(
		"🔇 **Manual Mute**\nUser: `%s`\nRoom: `%s`\nDuration: %s\nIssued by: `%s`",
		targetID, ctx.RoomID, modFormatDuration(duration), ctx.Sender,
	))

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Muted `%s` for %s.", targetID, modFormatDuration(duration)))
}

func (p *ModerationPlugin) handleUnmute(ctx MessageContext, args string) error {
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod unmute @user`")
	}

	targetID, ok := p.ResolveUser(strings.Fields(args)[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	// Cancel any pending unmute timer
	p.muteMu.Lock()
	k := windowKey{targetID, ctx.RoomID}
	if timer, ok := p.muteTimer[k]; ok {
		timer.Stop()
		delete(p.muteTimer, k)
	}
	p.muteMu.Unlock()

	p.unmuteUser(targetID, ctx.RoomID, 0)
	p.logAction(targetID, ctx.RoomID, "unmute", "manual", string(ctx.Sender))
	p.dmUser(targetID, "You have been unmuted by an admin.")
	p.notifyAdmin(fmt.Sprintf(
		"🔊 **Manual Unmute**\nUser: `%s`\nRoom: `%s`\nIssued by: `%s`",
		targetID, ctx.RoomID, ctx.Sender,
	))

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Unmuted `%s`.", targetID))
}

func (p *ModerationPlugin) handleBan(ctx MessageContext, args string) error {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod ban @user [reason]`")
	}

	targetID, ok := p.ResolveUser(parts[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	reason := "No reason provided"
	if len(parts) > 1 {
		reason = parts[1]
	}

	p.dmUser(targetID, "You are being banned from this community. "+reason)
	p.banUser(targetID, ctx.RoomID, reason)
	p.logAction(targetID, ctx.RoomID, "ban", "manual: "+reason, string(ctx.Sender))
	p.notifyAdmin(fmt.Sprintf(
		"🔨 **Manual Ban**\nUser: `%s`\nRoom: `%s`\nReason: %s\nIssued by: `%s`",
		targetID, ctx.RoomID, reason, ctx.Sender,
	))

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Banned `%s`. Reason: %s", targetID, reason))
}

func (p *ModerationPlugin) handleStrikes(ctx MessageContext, args string) error {
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod strikes @user`")
	}

	targetID, ok := p.ResolveUser(strings.Fields(args)[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	d := db.Get()
	rows, err := d.Query(
		`SELECT reason, issued_at, expires_at, issued_by FROM mod_strikes
		 WHERE user_id = ? AND active = TRUE AND expires_at > CURRENT_TIMESTAMP
		 ORDER BY issued_at DESC`,
		string(targetID),
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to query strikes.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Active strikes for `%s`:**\n", targetID))
	count := 0
	for rows.Next() {
		var reason, issuedAt, expiresAt, issuedBy string
		rows.Scan(&reason, &issuedAt, &expiresAt, &issuedBy)
		count++
		sb.WriteString(fmt.Sprintf("%d. %s (by `%s`, expires %s)\n", count, reason, issuedBy, expiresAt[:10]))
	}

	if count == 0 {
		sb.WriteString("No active strikes.")
	} else {
		sb.WriteString(fmt.Sprintf("\n**Total: %d/%d**", count, p.cfg.MaxStrikes))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *ModerationPlugin) handleForgive(ctx MessageContext, args string) error {
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod forgive @user`")
	}

	targetID, ok := p.ResolveUser(strings.Fields(args)[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	d := db.Get()
	result, err := d.Exec(
		`UPDATE mod_strikes SET active = FALSE WHERE user_id = ? AND active = TRUE`,
		string(targetID),
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to clear strikes.")
	}

	affected, _ := result.RowsAffected()
	p.logAction(targetID, ctx.RoomID, "forgive", fmt.Sprintf("cleared %d strikes", affected), string(ctx.Sender))
	p.notifyAdmin(fmt.Sprintf(
		"🕊️ **Forgive**\nUser: `%s`\nCleared %d active strike(s)\nIssued by: `%s`",
		targetID, affected, ctx.Sender,
	))

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Cleared %d active strike(s) for `%s`.", affected, targetID))
}

func (p *ModerationPlugin) handleHistory(ctx MessageContext, args string) error {
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod history @user`")
	}

	targetID, ok := p.ResolveUser(strings.Fields(args)[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	d := db.Get()
	rows, err := d.Query(
		`SELECT action, reason, taken_at, taken_by FROM mod_actions
		 WHERE user_id = ? ORDER BY taken_at DESC LIMIT 20`,
		string(targetID),
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to query history.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Moderation history for `%s`** (last 20):\n", targetID))
	count := 0
	for rows.Next() {
		var action, reason, takenAt, takenBy string
		rows.Scan(&action, &reason, &takenAt, &takenBy)
		count++
		sb.WriteString(fmt.Sprintf("- **%s** — %s (by `%s`, %s)\n", action, reason, takenBy, takenAt[:10]))
	}

	if count == 0 {
		sb.WriteString("No moderation history.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *ModerationPlugin) handleReload(ctx MessageContext) error {
	if p.cfg.WordListPath == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No word list path configured.")
	}

	err := p.wl.load(p.cfg.WordListPath)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Failed to reload: %v", err))
	}

	msg := fmt.Sprintf("Word list reloaded: %d entries.", p.wl.count())
	p.notifyAdmin(fmt.Sprintf("🔄 **Word list reloaded** by `%s` — %d entries", ctx.Sender, p.wl.count()))
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *ModerationPlugin) handleStatus(ctx MessageContext) error {
	var sb strings.Builder
	sb.WriteString("**Moderation Configuration**\n")
	sb.WriteString(fmt.Sprintf("Word list: %d words (variations: %v)\n", p.wl.count(), p.cfg.WordListVariations))
	sb.WriteString(fmt.Sprintf("Strike expiry: %d days, max strikes: %d\n", p.cfg.StrikeExpiryDays, p.cfg.MaxStrikes))
	sb.WriteString(fmt.Sprintf("Mute duration: %d minutes\n", p.cfg.MuteDurationMinutes))
	sb.WriteString(fmt.Sprintf("Text flood: %d msgs / %ds\n", p.cfg.FloodMessageCount, p.cfg.FloodMessageWindowSeconds))
	sb.WriteString(fmt.Sprintf("Image flood: %d files / %ds\n", p.cfg.FloodImageCount, p.cfg.FloodImageWindowSeconds))
	sb.WriteString(fmt.Sprintf("Max message length: %d chars\n", p.cfg.MaxMessageLength))
	sb.WriteString(fmt.Sprintf("Repeat detection: %d msgs / %ds (%.0f%% similarity)\n", p.cfg.RepeatCount, p.cfg.RepeatWindowSeconds, p.cfg.RepeatSimilarityThreshold*100))
	sb.WriteString(fmt.Sprintf("Mention flood: max %d/msg, %d heavy/%ds\n", p.cfg.MentionMax, p.cfg.MentionFloodCount, p.cfg.MentionFloodWindowSeconds))
	sb.WriteString(fmt.Sprintf("Link rate (new): %d / %ds\n", p.cfg.LinkRateNewMember, p.cfg.LinkRateWindowSeconds))
	sb.WriteString(fmt.Sprintf("Invite flood: %d/hour\n", p.cfg.InviteMaxPerHour))
	sb.WriteString(fmt.Sprintf("Join/leave: %d / %d min\n", p.cfg.JoinLeaveCount, p.cfg.JoinLeaveWindowMinutes))
	sb.WriteString(fmt.Sprintf("New member: %d days, flood multiplier: %.1f\n", p.cfg.NewMemberDays, p.cfg.NewMemberFloodMultiplier))
	sb.WriteString(fmt.Sprintf("Admin room: %s\n", p.cfg.AdminRoom))
	sb.WriteString(fmt.Sprintf("DM on action: %v", p.cfg.DMOnAction))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *ModerationPlugin) handleTest(ctx MessageContext, args string) error {
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!mod test @user`")
	}

	targetID, ok := p.ResolveUser(strings.Fields(args)[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}

	activeStrikes := p.countActiveStrikes(targetID)
	isNew := p.isNewMember(targetID)

	var nextAction string
	switch {
	case activeStrikes+1 >= p.cfg.MaxStrikes:
		nextAction = "**permanent ban**"
	case activeStrikes+1 == p.cfg.MaxStrikes-1:
		nextAction = fmt.Sprintf("**temp mute** (%d min)", p.cfg.MuteDurationMinutes)
	default:
		nextAction = "**warning + redact**"
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("**Test for `%s`:**\nActive strikes: %d/%d\nNew member: %v\nNext violation would result in: %s",
			targetID, activeStrikes, p.cfg.MaxStrikes, isNew, nextAction))
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	// Try Go duration format first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Try plain number as minutes
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Minute, nil
	}
	return 0, fmt.Errorf("invalid duration: %s", s)
}
