package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

var thankRe = regexp.MustCompile(`(?i)\b(thanks|thank\s+you|thankyou|thx|ty|tysm|tyvm)\b`)

// userMentionRe matches Matrix user IDs like @user:server.tld in plain text.
var userMentionRe = regexp.MustCompile(`@[a-zA-Z0-9._=-]+:[a-zA-Z0-9.-]+`)

// matrixToMentionRe extracts Matrix user IDs from HTML formatted_body mentions.
// Element and most clients format mentions as: <a href="https://matrix.to/#/@user:server">Name</a>
var matrixToMentionRe = regexp.MustCompile(`https://matrix\.to/#/(@[a-zA-Z0-9._=-]+:[a-zA-Z0-9.-]+)`)

// ReputationPlugin tracks gratitude and awards reputation XP.
type ReputationPlugin struct {
	Base
	xp *XPPlugin
}

// NewReputationPlugin creates a new reputation plugin.
func NewReputationPlugin(client *mautrix.Client, xp *XPPlugin) *ReputationPlugin {
	return &ReputationPlugin{
		Base: NewBase(client),
		xp:   xp,
	}
}

func (p *ReputationPlugin) Name() string { return "reputation" }

func (p *ReputationPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "rep", Description: "Show reputation count for a user", Usage: "!rep [@user]", Category: "Leveling & Stats"},
		{Name: "repboard", Description: "Show top 10 reputation receivers", Usage: "!repboard", Category: "Leveling & Stats"},
	}
}

func (p *ReputationPlugin) Init() error { return nil }

func (p *ReputationPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *ReputationPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "rep") {
		return p.handleRep(ctx)
	}
	if p.IsCommand(ctx.Body, "repboard") {
		return p.handleRepboard(ctx)
	}

	// Passive: detect thank-you messages
	if thankRe.MatchString(ctx.Body) {
		return p.handleThank(ctx)
	}

	return nil
}

func (p *ReputationPlugin) handleThank(ctx MessageContext) error {
	// Find mentioned users from both plain text and HTML formatted body
	mentions := extractMentions(ctx)
	if len(mentions) == 0 {
		return nil
	}

	d := db.Get()
	now := time.Now().UTC().Unix()
	cooldownDuration := int64(24 * 60 * 60) // 24 hours

	for _, mention := range mentions {
		receiver := id.UserID(mention)

		// Can't thank yourself
		if receiver == ctx.Sender {
			continue
		}

		// Check cooldown
		var lastGiven int64
		err := d.QueryRow(
			`SELECT last_given FROM rep_cooldowns WHERE giver = ? AND receiver = ?`,
			string(ctx.Sender), string(receiver),
		).Scan(&lastGiven)

		if err != nil && err != sql.ErrNoRows {
			slog.Error("rep: cooldown check", "err", err)
			continue
		}

		if err == nil && now-lastGiven < cooldownDuration {
			continue // Still on cooldown
		}

		// Update cooldown
		_, err = d.Exec(
			`INSERT INTO rep_cooldowns (giver, receiver, last_given) VALUES (?, ?, ?)
			 ON CONFLICT(giver, receiver) DO UPDATE SET last_given = ?`,
			string(ctx.Sender), string(receiver), now, now,
		)
		if err != nil {
			slog.Error("rep: update cooldown", "err", err)
			continue
		}

		// Award 5 XP via the XP plugin
		if p.xp != nil {
			p.xp.GrantXP(receiver, 5, "reputation")
		}

		if err := p.SendReact(ctx.RoomID, ctx.EventID, "💜"); err != nil {
			slog.Error("rep: send react", "err", err)
		}
	}

	return nil
}

func (p *ReputationPlugin) handleRep(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "rep")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	var count int
	err := d.QueryRow(
		`SELECT COALESCE(SUM(amount), 0) FROM xp_log WHERE user_id = ? AND reason = 'reputation'`,
		string(target),
	).Scan(&count)
	if err != nil {
		slog.Error("rep: query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to look up reputation.")
	}

	// Count is total XP from rep, each rep gives 5 XP, so rep count = count / 5
	repCount := count / 5
	msg := fmt.Sprintf("💜 %s has received %s reputation points (%s XP from gratitude).",
		string(target), formatNumber(repCount), formatNumber(count))
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *ReputationPlugin) handleRepboard(ctx MessageContext) error {
	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, SUM(amount) as total
		 FROM xp_log WHERE reason = 'reputation'
		 GROUP BY user_id ORDER BY total DESC`,
	)
	if err != nil {
		slog.Error("rep: repboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load reputation board.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("💜 Reputation Board — Top 10\n\n")

	medals := []string{"🥇", "🥈", "🥉"}
	i := 0
	for rows.Next() && i < 10 {
		var userID string
		var totalXP int
		if err := rows.Scan(&userID, &totalXP); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(userID)] {
			continue
		}
		repCount := totalXP / 5
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %s rep (%s XP)\n", prefix, userID, formatNumber(repCount), formatNumber(totalXP)))
		i++
	}

	if i == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No reputation data yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// extractMentions pulls Matrix user IDs from both the plain text body and
// the HTML formatted_body (where most clients put actual @user:server mentions).
func extractMentions(ctx MessageContext) []string {
	seen := make(map[string]bool)
	var result []string

	// Check plain text body for raw @user:server mentions
	for _, m := range userMentionRe.FindAllString(ctx.Body, -1) {
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}

	// Check HTML formatted_body for matrix.to mention links
	if ctx.Event != nil {
		content := ctx.Event.Content.AsMessage()
		if content != nil && content.FormattedBody != "" {
			for _, match := range matrixToMentionRe.FindAllStringSubmatch(content.FormattedBody, -1) {
				if len(match) > 1 && !seen[match[1]] {
					seen[match[1]] = true
					result = append(result, match[1])
				}
			}
		}
	}

	return result
}
