package plugin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Gift Constants ──────────────────────────────────────────────────────────
//
// Magnitudes equalized across all four outcomes so neither "always open" nor
// "always leave" is variance-dominant at a 50/50 sender mix. Asymmetric
// magnitudes (e.g., open=±7, leave=±4) gave leave the same EV with lower
// variance, making it a dominant strategy for risk-averse parties — which
// collapses the dilemma into "always leave."
//
//   basket opened:    +6        basket unopened:  -6
//   mimic opened:     -6        mimic unopened:   +6
//
// Both strategies: EV=0, σ=6. The choice becomes "read sender intent" only.
const (
	coopGiftBasketOpened   = 6
	coopGiftBasketUnopened = -6
	coopGiftMimicOpened    = -6
	coopGiftMimicUnopened  = 6
)

const (
	coopGiftBasket = "basket"
	coopGiftMimic  = "mimic"
)

type CoopGift struct {
	ID            int
	RunID         int
	SenderID      id.UserID
	Day           int
	GiftType      string
	Votes         map[id.UserID]string // "open" or "leave"
	VoteResult    string               // "opened" / "left" / ""
	Outcome       string               // "boost" / "reduction" / ""
	Modifier      int
	PostEventID   id.EventID
}

// ── Command: send a gift ────────────────────────────────────────────────────

func (p *AdventurePlugin) handleCoopGift(ctx MessageContext, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return p.SendDM(ctx.Sender, "Usage: `!coop gift <run_id> <basket|mimic>`. Costs one harvest action; party never knows what you sent.")
	}
	runID, err := strconv.Atoi(parts[0])
	if err != nil {
		return p.SendDM(ctx.Sender, "Run ID must be a number.")
	}
	giftType := strings.ToLower(parts[1])
	if giftType != coopGiftBasket && giftType != coopGiftMimic {
		return p.SendDM(ctx.Sender, "Gift type must be `basket` or `mimic`.")
	}

	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. The post office does not deliver from the ditch.")
	}
	if !char.CanDoHarvest(false) {
		return p.SendDM(ctx.Sender, "You're out of harvest actions today.")
	}

	run, err := loadCoopRun(runID)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "Run not found.")
	}
	if run.Status != "active" {
		return p.SendDM(ctx.Sender, "Run is not active. Gifts can only be sent during a running co-op.")
	}

	// Senders cannot be party members.
	if existing, _ := loadCoopMember(runID, ctx.Sender); existing != nil {
		return p.SendDM(ctx.Sender, "You're in the party. You can't send gifts to yourself.")
	}

	giftID, err := createCoopGift(runID, ctx.Sender, run.CurrentDay, giftType)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't send the gift.")
	}

	// Charge the harvest action and persist.
	char.HarvestActionsUsed++
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("coop: save sender after gift", "err", err)
	}

	// Post the vote prompt to the game room.
	gr := gamesRoom()
	if gr != "" {
		gift, _ := loadCoopGift(giftID)
		members, _ := loadCoopMembers(runID)
		postID, err := p.SendMessageID(gr, renderCoopGiftPost(run, gift, members))
		if err == nil {
			_ = saveCoopGiftPostID(giftID, postID)
		}
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("📦 Gift dispatched (run #%d, day %d). One harvest action consumed. The party doesn't know what you sent.", runID, run.CurrentDay))
}

// ── Command: vote on a gift ─────────────────────────────────────────────────

func (p *AdventurePlugin) handleCoopGiftVote(ctx MessageContext, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 1 {
		return p.SendDM(ctx.Sender, "Usage: `!coop giftvote <gift_id> <open|leave>` or `!coop giftvote <open|leave>` for the most recent unresolved gift.")
	}

	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadCoopRunForUser(ctx.Sender)
	if err != nil || run == nil || run.Status != "active" {
		return p.SendDM(ctx.Sender, "You're not in an active co-op run.")
	}

	var gift *CoopGift
	var choice string
	if len(parts) >= 2 {
		giftID, perr := strconv.Atoi(parts[0])
		if perr != nil {
			return p.SendDM(ctx.Sender, "First arg must be the gift id, or just `open`/`leave` for the most recent.")
		}
		gift, _ = loadCoopGift(giftID)
		choice = strings.ToLower(parts[1])
	} else {
		gift, _ = loadMostRecentUnresolvedGift(run.ID, run.CurrentDay)
		choice = strings.ToLower(parts[0])
	}
	if gift == nil {
		return p.SendDM(ctx.Sender, "No unresolved gift found.")
	}
	if gift.RunID != run.ID || gift.Day != run.CurrentDay {
		return p.SendDM(ctx.Sender, "That gift isn't for your current run/day.")
	}
	if gift.VoteResult != "" {
		return p.SendDM(ctx.Sender, "That gift has already resolved.")
	}
	if choice != "open" && choice != "leave" {
		return p.SendDM(ctx.Sender, "Vote `open` or `leave`.")
	}

	if gift.Votes == nil {
		gift.Votes = map[id.UserID]string{}
	}
	gift.Votes[ctx.Sender] = choice
	if err := saveCoopGiftVotes(gift); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save your vote.")
	}

	gr := gamesRoom()
	if gr != "" && gift.PostEventID != "" {
		members, _ := loadCoopMembers(run.ID)
		_ = p.EditMessage(gr, gift.PostEventID, renderCoopGiftPost(run, gift, members))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("Gift #%d vote recorded: %s.", gift.ID, choice))
}

// ── Resolution ──────────────────────────────────────────────────────────────

// coopResolveGifts is called inside coopResolveFloor to tally and apply each
// unresolved gift for the current day. Returns the total modifier and a brief
// summary line.
func (p *AdventurePlugin) coopResolveGifts(run *CoopRun, leader id.UserID, day int) (int, string) {
	gifts, err := loadDayCoopGifts(run.ID, day)
	if err != nil || len(gifts) == 0 {
		return 0, ""
	}
	totalMod := 0
	var lines []string
	for _, g := range gifts {
		if g.VoteResult != "" {
			continue
		}
		opens, leaves := 0, 0
		var leaderVote string
		for u, v := range g.Votes {
			if v == "open" {
				opens++
			} else if v == "leave" {
				leaves++
			}
			if u == leader {
				leaderVote = v
			}
		}
		var voteResult string
		switch {
		case opens > leaves:
			voteResult = "opened"
		case leaves > opens:
			voteResult = "left"
		case leaderVote == "open":
			voteResult = "opened"
		case leaderVote == "leave":
			voteResult = "left"
		default:
			voteResult = "left" // default for full abstention
		}

		var mod int
		var outcome string
		switch g.GiftType {
		case coopGiftBasket:
			if voteResult == "opened" {
				mod = coopGiftBasketOpened
				outcome = "boost"
			} else {
				mod = coopGiftBasketUnopened
				outcome = "reduction"
			}
		case coopGiftMimic:
			if voteResult == "opened" {
				mod = coopGiftMimicOpened
				outcome = "reduction"
			} else {
				mod = coopGiftMimicUnopened
				outcome = "boost"
			}
		}
		totalMod += mod
		_ = resolveCoopGift(g.ID, voteResult, outcome, mod)
		lines = append(lines, fmt.Sprintf("  %s gift from %s — %s → %+d%%",
			titleCaseWord(g.GiftType), coopDisplayName(p, g.SenderID), voteResult, mod))
		_ = p.SendDM(g.SenderID, renderCoopGiftSenderDM(&g, voteResult, mod))
	}
	if len(lines) == 0 {
		return 0, ""
	}
	return totalMod, "Gifts:\n" + strings.Join(lines, "\n")
}

// ── DB ──────────────────────────────────────────────────────────────────────

func createCoopGift(runID int, sender id.UserID, day int, giftType string) (int, error) {
	d := db.Get()
	res, err := d.Exec(`INSERT INTO coop_dungeon_gifts (run_id, sender_id, day, gift_type)
		VALUES (?, ?, ?, ?)`, runID, string(sender), day, giftType)
	if err != nil {
		return 0, err
	}
	id64, _ := res.LastInsertId()
	return int(id64), nil
}

func loadCoopGift(giftID int) (*CoopGift, error) {
	d := db.Get()
	g := &CoopGift{}
	var sender, votesJSON string
	var voteResult, outcome, postID sql.NullString
	err := d.QueryRow(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id
		FROM coop_dungeon_gifts WHERE id = ?`, giftID).Scan(
		&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
		&votesJSON, &voteResult, &outcome, &g.Modifier, &postID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.SenderID = id.UserID(sender)
	g.Votes = parseCoopVotes(votesJSON)
	if voteResult.Valid {
		g.VoteResult = voteResult.String
	}
	if outcome.Valid {
		g.Outcome = outcome.String
	}
	if postID.Valid {
		g.PostEventID = id.EventID(postID.String)
	}
	return g, nil
}

func loadDayCoopGifts(runID, day int) ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id
		FROM coop_dungeon_gifts WHERE run_id = ? AND day = ? ORDER BY id`, runID, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoopGift
	for rows.Next() {
		g := CoopGift{}
		var sender, votesJSON string
		var voteResult, outcome, postID sql.NullString
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID); err != nil {
			return nil, err
		}
		g.SenderID = id.UserID(sender)
		g.Votes = parseCoopVotes(votesJSON)
		if voteResult.Valid {
			g.VoteResult = voteResult.String
		}
		if outcome.Valid {
			g.Outcome = outcome.String
		}
		if postID.Valid {
			g.PostEventID = id.EventID(postID.String)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadAllCoopGifts(runID int) ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id
		FROM coop_dungeon_gifts WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoopGift
	for rows.Next() {
		g := CoopGift{}
		var sender, votesJSON string
		var voteResult, outcome, postID sql.NullString
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID); err != nil {
			return nil, err
		}
		g.SenderID = id.UserID(sender)
		g.Votes = parseCoopVotes(votesJSON)
		if voteResult.Valid {
			g.VoteResult = voteResult.String
		}
		if outcome.Valid {
			g.Outcome = outcome.String
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadMostRecentUnresolvedGift(runID, day int) (*CoopGift, error) {
	gifts, err := loadDayCoopGifts(runID, day)
	if err != nil {
		return nil, err
	}
	for i := len(gifts) - 1; i >= 0; i-- {
		if gifts[i].VoteResult == "" {
			g := gifts[i]
			return &g, nil
		}
	}
	return nil, nil
}

func saveCoopGiftVotes(g *CoopGift) error {
	d := db.Get()
	jsonBytes, err := json.Marshal(serializeCoopVotes(g.Votes))
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE coop_dungeon_gifts SET votes = ? WHERE id = ?`,
		string(jsonBytes), g.ID)
	return err
}

func saveCoopGiftPostID(giftID int, eventID id.EventID) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_gifts SET post_event_id = ? WHERE id = ?`,
		string(eventID), giftID)
	return err
}

func resolveCoopGift(giftID int, voteResult, outcome string, modifier int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_gifts
		SET vote_result = ?, outcome = ?, modifier = ?, resolved_at = CURRENT_TIMESTAMP
		WHERE id = ?`, voteResult, outcome, modifier, giftID)
	return err
}

// ── Render ──────────────────────────────────────────────────────────────────

func renderCoopGiftPost(run *CoopRun, gift *CoopGift, members []CoopMember) string {
	var sb strings.Builder
	flavor := ""
	if len(TwinBeeGiftArrival) > 0 {
		flavor = TwinBeeGiftArrival[rand.IntN(len(TwinBeeGiftArrival))]
	}
	sb.WriteString(fmt.Sprintf("📦 **Gift #%d arrived — Co-op #%d, Day %d**\n\n", gift.ID, run.ID, gift.Day))
	if flavor != "" {
		sb.WriteString(flavor)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Vote with `!coop giftvote " + strconv.Itoa(gift.ID) + " open|leave`. Resolved at next daily tick.\n\n")
	opens, leaves := 0, 0
	for _, v := range gift.Votes {
		if v == "open" {
			opens++
		} else if v == "leave" {
			leaves++
		}
	}
	sb.WriteString(fmt.Sprintf("Current votes: open (%d) · leave (%d)", opens, leaves))
	awaiting := 0
	for _, m := range members {
		if _, ok := gift.Votes[m.UserID]; !ok {
			awaiting++
		}
	}
	if awaiting > 0 && awaiting < len(members) {
		sb.WriteString(fmt.Sprintf("\nAwaiting: %d of %d", awaiting, len(members)))
	}
	return sb.String()
}

func renderCoopGiftSenderDM(g *CoopGift, voteResult string, mod int) string {
	pickFrom := func(pool []string) string {
		if len(pool) == 0 {
			return ""
		}
		return pool[rand.IntN(len(pool))]
	}
	switch {
	case g.GiftType == coopGiftBasket && voteResult == "opened":
		return fmt.Sprintf("📦 Your care basket was opened. %s (%+d%% to today's floor)", pickFrom(TwinBeeGiftBasketOpened), mod)
	case g.GiftType == coopGiftBasket && voteResult == "left":
		return fmt.Sprintf("📦 Your care basket was not opened. %s (%+d%%)", pickFrom(TwinBeeGiftBasketUnopened), mod)
	case g.GiftType == coopGiftMimic && voteResult == "opened":
		return fmt.Sprintf("📦 Your mimic was opened. %s (%+d%%)", pickFrom(TwinBeeGiftMimicOpened), mod)
	case g.GiftType == coopGiftMimic && voteResult == "left":
		return fmt.Sprintf("📦 Your mimic was not opened. %s (%+d%%)", pickFrom(TwinBeeGiftMimicUnopened), mod)
	}
	return "📦 Your gift resolved."
}

// renderCoopGiftLog returns the public end-of-run gift log.
func renderCoopGiftLog(p *AdventurePlugin, runID int) string {
	gifts, err := loadAllCoopGifts(runID)
	if err != nil || len(gifts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("📦 Gift Log:\n")
	for _, g := range gifts {
		result := g.VoteResult
		if result == "" {
			result = "unresolved"
		}
		sb.WriteString(fmt.Sprintf("  Day %d: %s → %s → %s → %+d%%\n",
			g.Day, coopDisplayName(p, g.SenderID), titleCaseWord(g.GiftType), result, g.Modifier))
	}
	return sb.String()
}
