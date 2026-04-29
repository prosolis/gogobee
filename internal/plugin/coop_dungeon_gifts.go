package plugin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

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

	// Per-gift voting window. After this elapses, the gift's votes are
	// tallied independently of the daily floor tick. Senders get DM feedback
	// at expiry; the modifier sits on the run until the next floor resolves.
	coopGiftWindow = 6 * time.Hour
)

type CoopGift struct {
	ID          int
	RunID       int
	SenderID    id.UserID
	Day         int
	GiftType    string
	Votes       map[id.UserID]string // "open" or "leave" — only set on lead
	VoteResult  string               // "opened" / "left" / ""
	Outcome     string               // "boost" / "reduction" / ""
	Modifier    int
	PostEventID id.EventID
	ExpiresAt   *time.Time // voting closes — only set on lead
	AppliedAt   *time.Time // modifier merged into a floor roll
	StackLeadID *int       // NULL = lead/standalone; otherwise points at lead's id
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

	giftID, leadID, isNewLead, err := createCoopGift(runID, ctx.Sender, run.CurrentDay, giftType)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't send the gift.")
	}

	// Charge the harvest action and persist.
	char.HarvestActionsUsed++
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("coop: save sender after gift", "err", err)
	}

	// Post (new lead) or edit (added to existing stack).
	gr := gamesRoom()
	if gr != "" {
		members, _ := loadCoopMembers(runID)
		if isNewLead {
			gift, _ := loadCoopGift(giftID)
			postID, perr := p.SendMessageID(gr, renderCoopGiftPost(p, run, gift, members))
			if perr == nil {
				_ = saveCoopGiftPostID(giftID, postID)
			}
		} else {
			lead, _ := loadCoopGift(leadID)
			if lead != nil && lead.PostEventID != "" {
				_ = p.EditMessage(gr, lead.PostEventID, renderCoopGiftPost(p, run, lead, members))
			}
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
	// If the user voted on a follower, redirect to its lead — votes always
	// live on the lead.
	if gift.StackLeadID != nil {
		lead, _ := loadCoopGift(*gift.StackLeadID)
		if lead == nil {
			return p.SendDM(ctx.Sender, "Couldn't find the gift's stack lead.")
		}
		gift = lead
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
		_ = p.EditMessage(gr, gift.PostEventID, renderCoopGiftPost(p, run, gift, members))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("Gift #%d vote recorded: %s.", gift.ID, choice))
}

// ── Resolution ──────────────────────────────────────────────────────────────

// resolveSingleGift tallies a stack lead's votes, persists the outcome, DMs
// every sender in the stack, and edits the game-room post to show the
// resolved state. Used by both the per-minute expiry ticker and the
// floor-resolution catchall.
//
// Followers passed in here will be redirected to their lead. Idempotent:
// always re-loads the lead from DB before checking vote_result, so an
// in-memory snapshot from earlier in the same loop can't trigger a double
// resolve.
func (p *AdventurePlugin) resolveSingleGift(g *CoopGift, leader id.UserID) {
	// Redirect follower → lead, then re-fetch fresh state.
	leadID := g.ID
	if g.StackLeadID != nil {
		leadID = *g.StackLeadID
	}
	fresh, _ := loadCoopGift(leadID)
	if fresh == nil {
		return
	}
	g = fresh
	if g.VoteResult != "" {
		return
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
		voteResult = "left"
	}

	// Per-gift modifier (single ±6 unit). Stack-wide modifier is N × this,
	// applied at floor resolution by summing the rows individually.
	var perMod int
	var outcome string
	switch g.GiftType {
	case coopGiftBasket:
		if voteResult == "opened" {
			perMod = coopGiftBasketOpened
			outcome = "boost"
		} else {
			perMod = coopGiftBasketUnopened
			outcome = "reduction"
		}
	case coopGiftMimic:
		if voteResult == "opened" {
			perMod = coopGiftMimicOpened
			outcome = "reduction"
		} else {
			perMod = coopGiftMimicUnopened
			outcome = "boost"
		}
	}

	stack, _ := loadCoopGiftStack(g.ID)
	totalMod := 0
	for _, m := range stack {
		_ = resolveCoopGift(m.ID, voteResult, outcome, perMod)
		_ = p.SendDM(m.SenderID, renderCoopGiftSenderDM(&m, voteResult, perMod))
		totalMod += perMod
	}

	g.VoteResult = voteResult
	g.Outcome = outcome
	g.Modifier = perMod

	// Edit the live game-room post to show the resolved state — total swing
	// reflects the full stack.
	gr := gamesRoom()
	if gr != "" && g.PostEventID != "" {
		_ = p.EditMessage(gr, g.PostEventID, renderCoopGiftResolvedPost(g, len(stack), totalMod))
	}
}

// coopProcessGiftExpiries runs each ticker minute, resolving any gifts whose
// voting window has elapsed. Modifiers are recorded but not yet applied to a
// floor — they wait for the next floor resolution.
func (p *AdventurePlugin) coopProcessGiftExpiries() {
	gifts, err := loadExpiredUnresolvedGifts()
	if err != nil {
		slog.Error("coop: load expired gifts", "err", err)
		return
	}
	for i := range gifts {
		g := &gifts[i]
		run, _ := loadCoopRun(g.RunID)
		if run == nil || run.Status != "active" {
			// Run gone or already terminal — still mark the gift resolved so
			// it stops cluttering the expiry query.
			_ = resolveCoopGift(g.ID, "left", "reduction", 0)
			continue
		}
		p.resolveSingleGift(g, run.LeaderID)
	}
}

// coopResolveGifts is called inside coopResolveFloor. Two responsibilities:
//
//  1. Catchall: force-resolve any gifts on this day that still have no
//     vote_result (the timer ticker should normally have caught them, but
//     this is the defensive backstop, e.g., bot was down).
//  2. Apply: sum modifiers from any resolved-but-not-yet-applied gifts on
//     this run and atomically claim them via markCoopGiftApplied so the
//     same modifier is never double-applied across retries.
//
// Returns total modifier and a summary line for the daily-result post.
func (p *AdventurePlugin) coopResolveGifts(run *CoopRun, leader id.UserID, day int) (int, string) {
	// Catchall: any unresolved leads for this day get force-resolved now.
	// Iterate leads only — resolveSingleGift propagates to followers, so
	// touching followers here would just be redundant work.
	dayGifts, _ := loadDayCoopGifts(run.ID, day)
	for i := range dayGifts {
		if dayGifts[i].StackLeadID != nil {
			continue
		}
		if dayGifts[i].VoteResult == "" {
			p.resolveSingleGift(&dayGifts[i], leader)
		}
	}

	// Apply: pending resolved gifts (any day, this run) get their modifier
	// merged into the floor roll.
	pending, _ := loadResolvedUnappliedGifts(run.ID)
	totalMod := 0
	var lines []string
	for _, g := range pending {
		claimed, err := markCoopGiftApplied(g.ID)
		if err != nil || !claimed {
			continue
		}
		totalMod += g.Modifier
		lines = append(lines, fmt.Sprintf("  %s gift from %s — %s → %+d%%",
			titleCaseWord(g.GiftType), coopDisplayName(p, g.SenderID), g.VoteResult, g.Modifier))
	}
	if len(lines) == 0 {
		return 0, ""
	}
	return totalMod, "Gifts:\n" + strings.Join(lines, "\n")
}

// ── DB ──────────────────────────────────────────────────────────────────────

// findCoopGiftStackLead returns the active lead gift for a (run, day, type)
// stack, or nil if no unresolved lead exists. Lead = oldest unresolved gift
// of that type with stack_lead_id IS NULL.
func findCoopGiftStackLead(runID, day int, giftType string) (*CoopGift, error) {
	d := db.Get()
	var leadID int
	err := d.QueryRow(`SELECT id FROM coop_dungeon_gifts
		WHERE run_id = ? AND day = ? AND gift_type = ?
		  AND vote_result IS NULL AND stack_lead_id IS NULL
		ORDER BY id ASC LIMIT 1`, runID, day, giftType).Scan(&leadID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return loadCoopGift(leadID)
}

// loadCoopGiftStack returns the lead and all followers (in id order) for a
// stack identified by lead_id. Used to propagate resolution + render.
func loadCoopGiftStack(leadID int) ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id, expires_at, applied_at, stack_lead_id
		FROM coop_dungeon_gifts
		WHERE id = ? OR stack_lead_id = ?
		ORDER BY id ASC`, leadID, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoopGift
	for rows.Next() {
		g := CoopGift{}
		var sender, votesJSON string
		var voteResult, outcome, postID sql.NullString
		var expiresAt, appliedAt sql.NullTime
		var stackLeadID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID, &expiresAt, &appliedAt, &stackLeadID); err != nil {
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
		if expiresAt.Valid {
			t := expiresAt.Time
			g.ExpiresAt = &t
		}
		if appliedAt.Valid {
			t := appliedAt.Time
			g.AppliedAt = &t
		}
		if stackLeadID.Valid {
			v := int(stackLeadID.Int64)
			g.StackLeadID = &v
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// createCoopGift inserts a new gift. If an unresolved same-type stack exists
// for the run+day, the new gift becomes a follower (no expires_at, no own
// post — lead's post is edited to bump the count). Otherwise a new lead is
// created with its own 6h expiry.
//
// Returns (giftID, leadID, isNewLead, error). leadID is the gift's effective
// stack lead — either itself (new lead) or the existing lead it joined.
func createCoopGift(runID int, sender id.UserID, day int, giftType string) (giftID int, leadID int, isNewLead bool, err error) {
	d := db.Get()

	existing, lerr := findCoopGiftStackLead(runID, day, giftType)
	if lerr != nil {
		return 0, 0, false, lerr
	}
	if existing != nil {
		// Follower: no expires_at, no post. Inherits lead's deadline.
		res, ierr := d.Exec(`INSERT INTO coop_dungeon_gifts
			(run_id, sender_id, day, gift_type, stack_lead_id) VALUES (?, ?, ?, ?, ?)`,
			runID, string(sender), day, giftType, existing.ID)
		if ierr != nil {
			return 0, 0, false, ierr
		}
		id64, _ := res.LastInsertId()
		return int(id64), existing.ID, false, nil
	}

	// New lead: own expiry, will get its own post.
	expires := time.Now().UTC().Add(coopGiftWindow)
	res, ierr := d.Exec(`INSERT INTO coop_dungeon_gifts
		(run_id, sender_id, day, gift_type, expires_at) VALUES (?, ?, ?, ?, ?)`,
		runID, string(sender), day, giftType, expires)
	if ierr != nil {
		return 0, 0, false, ierr
	}
	id64, _ := res.LastInsertId()
	return int(id64), int(id64), true, nil
}

func loadCoopGift(giftID int) (*CoopGift, error) {
	d := db.Get()
	g := &CoopGift{}
	var sender, votesJSON string
	var voteResult, outcome, postID sql.NullString
	var expiresAt, appliedAt sql.NullTime
	var stackLeadID sql.NullInt64
	err := d.QueryRow(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id, expires_at, applied_at, stack_lead_id
		FROM coop_dungeon_gifts WHERE id = ?`, giftID).Scan(
		&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
		&votesJSON, &voteResult, &outcome, &g.Modifier, &postID, &expiresAt, &appliedAt, &stackLeadID)
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
	if expiresAt.Valid {
		t := expiresAt.Time
		g.ExpiresAt = &t
	}
	if stackLeadID.Valid { v := int(stackLeadID.Int64); g.StackLeadID = &v }
		if appliedAt.Valid {
		t := appliedAt.Time
		g.AppliedAt = &t
	}
	return g, nil
}

func loadDayCoopGifts(runID, day int) ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id, expires_at, applied_at, stack_lead_id
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
		var expiresAt, appliedAt sql.NullTime
	var stackLeadID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID, &expiresAt, &appliedAt, &stackLeadID); err != nil {
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
		if expiresAt.Valid {
			t := expiresAt.Time
			g.ExpiresAt = &t
		}
		if stackLeadID.Valid { v := int(stackLeadID.Int64); g.StackLeadID = &v }
		if appliedAt.Valid {
			t := appliedAt.Time
			g.AppliedAt = &t
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadAllCoopGifts(runID int) ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id, expires_at, applied_at, stack_lead_id
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
		var expiresAt, appliedAt sql.NullTime
	var stackLeadID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID, &expiresAt, &appliedAt, &stackLeadID); err != nil {
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
		if expiresAt.Valid {
			t := expiresAt.Time
			g.ExpiresAt = &t
		}
		if stackLeadID.Valid { v := int(stackLeadID.Int64); g.StackLeadID = &v }
		if appliedAt.Valid {
			t := appliedAt.Time
			g.AppliedAt = &t
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

// loadExpiredUnresolvedGifts returns gifts whose voting window has elapsed
// but whose votes haven't yet been tallied. Driven by the per-minute
// expiry ticker.
func loadExpiredUnresolvedGifts() ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id, expires_at, applied_at, stack_lead_id
		FROM coop_dungeon_gifts
		WHERE vote_result IS NULL AND expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP
		  AND stack_lead_id IS NULL
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoopGift
	for rows.Next() {
		g := CoopGift{}
		var sender, votesJSON string
		var voteResult, outcome, postID sql.NullString
		var expiresAt, appliedAt sql.NullTime
	var stackLeadID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID, &expiresAt, &appliedAt, &stackLeadID); err != nil {
			return nil, err
		}
		g.SenderID = id.UserID(sender)
		g.Votes = parseCoopVotes(votesJSON)
		if postID.Valid {
			g.PostEventID = id.EventID(postID.String)
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			g.ExpiresAt = &t
		}
		if stackLeadID.Valid {
			v := int(stackLeadID.Int64)
			g.StackLeadID = &v
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// loadResolvedUnappliedGifts returns gifts that have been tallied but whose
// modifiers have not yet been merged into a floor success roll. Used by
// floor resolution to sum and apply pending gift modifiers.
func loadResolvedUnappliedGifts(runID int) ([]CoopGift, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, run_id, sender_id, day, gift_type,
		votes, vote_result, outcome, modifier, post_event_id, expires_at, applied_at, stack_lead_id
		FROM coop_dungeon_gifts
		WHERE run_id = ? AND vote_result IS NOT NULL AND applied_at IS NULL
		ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoopGift
	for rows.Next() {
		g := CoopGift{}
		var sender, votesJSON string
		var voteResult, outcome, postID sql.NullString
		var expiresAt, appliedAt sql.NullTime
	var stackLeadID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.RunID, &sender, &g.Day, &g.GiftType,
			&votesJSON, &voteResult, &outcome, &g.Modifier, &postID, &expiresAt, &appliedAt, &stackLeadID); err != nil {
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
		if stackLeadID.Valid {
			v := int(stackLeadID.Int64)
			g.StackLeadID = &v
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// markCoopGiftApplied claims a gift's modifier as merged into a floor roll.
// Idempotent — only the first call sets applied_at; subsequent calls return
// false (claimed=false) so the same modifier is never double-applied.
func markCoopGiftApplied(giftID int) (bool, error) {
	d := db.Get()
	res, err := d.Exec(`UPDATE coop_dungeon_gifts SET applied_at = CURRENT_TIMESTAMP
		WHERE id = ? AND applied_at IS NULL`, giftID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ── Render ──────────────────────────────────────────────────────────────────

// renderCoopGiftPost is called only for stack leads. Stack count is derived
// from how many followers point at this lead. Flavor is picked deterministically
// by lead id so re-renders don't rotate.
func renderCoopGiftPost(p *AdventurePlugin, run *CoopRun, gift *CoopGift, members []CoopMember) string {
	var sb strings.Builder

	// Stack size = lead + followers
	stack, _ := loadCoopGiftStack(gift.ID)
	stackSize := len(stack)
	if stackSize == 0 {
		stackSize = 1
	}

	// Tally on the lead's votes
	opens, leaves := 0, 0
	for _, v := range gift.Votes {
		if v == "open" {
			opens++
		} else if v == "leave" {
			leaves++
		}
	}
	leaderName := coopDisplayName(p, run.LeaderID)

	header := fmt.Sprintf("📦 **Gift #%d arrived — Co-op #%d, Day %d**", gift.ID, run.ID, gift.Day)
	if stackSize > 1 {
		header = fmt.Sprintf("📦 **Gift Stack #%d (×%d) — Co-op #%d, Day %d**", gift.ID, stackSize, run.ID, gift.Day)
	}
	sb.WriteString(header)
	sb.WriteString("\n\n")

	// Pick flavor deterministically so edits don't rotate the joke.
	if len(TwinBeeGiftArrival) > 0 {
		flavor := TwinBeeGiftArrival[gift.ID%len(TwinBeeGiftArrival)]
		flavor = strings.Replace(flavor, "{count}", strconv.Itoa(opens), 1)
		flavor = strings.Replace(flavor, "{count}", strconv.Itoa(leaves), 1)
		flavor = strings.ReplaceAll(flavor, "{leader}", leaderName)
		sb.WriteString(flavor)
		sb.WriteString("\n\n")
	}

	// "REALLY special" line, picked once per stack lead, shown for size 2+.
	if stackSize >= 2 && len(TwinBeeGiftStackEscalation) > 0 {
		line := TwinBeeGiftStackEscalation[gift.ID%len(TwinBeeGiftStackEscalation)]
		sb.WriteString("_")
		sb.WriteString(line)
		sb.WriteString("_\n\n")
	}

	closesIn := ""
	if gift.ExpiresAt != nil {
		remaining := time.Until(*gift.ExpiresAt).Truncate(time.Minute)
		if remaining > 0 {
			closesIn = fmt.Sprintf(" · voting closes in %s", remaining)
		} else {
			closesIn = " · voting closing"
		}
	}
	if stackSize > 1 {
		sb.WriteString(fmt.Sprintf("One vote covers the whole stack of %d. ", stackSize))
	}
	sb.WriteString("Vote with `!coop giftvote " + strconv.Itoa(gift.ID) + " open|leave`" + closesIn + ".")

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

// renderCoopGiftResolvedPost replaces the live vote post when a gift's
// voting window closes. Total modifier = stackSize × per-gift mod.
func renderCoopGiftResolvedPost(g *CoopGift, stackSize, totalMod int) string {
	verb := "left"
	if g.VoteResult == "opened" {
		verb = "opened"
	}
	if stackSize > 1 {
		return fmt.Sprintf("📦 **Gift Stack #%d (×%d) resolved** — %s, %s → **%+d%%** to next floor.",
			g.ID, stackSize, titleCaseWord(g.GiftType), verb, totalMod)
	}
	return fmt.Sprintf("📦 **Gift #%d resolved** — %s, %s → **%+d%%** to next floor.",
		g.ID, titleCaseWord(g.GiftType), verb, totalMod)
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

// renderCoopGiftLog returns the public end-of-run gift log, grouped by stack.
// One line per stack with all senders listed and the total swing.
func renderCoopGiftLog(p *AdventurePlugin, runID int) string {
	gifts, err := loadAllCoopGifts(runID)
	if err != nil || len(gifts) == 0 {
		return ""
	}
	// Group gifts by their effective lead id.
	type groupKey struct {
		day      int
		giftType string
		leadID   int
	}
	groups := map[groupKey][]CoopGift{}
	order := []groupKey{}
	for _, g := range gifts {
		leadID := g.ID
		if g.StackLeadID != nil {
			leadID = *g.StackLeadID
		}
		k := groupKey{day: g.Day, giftType: g.GiftType, leadID: leadID}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], g)
	}

	var sb strings.Builder
	sb.WriteString("📦 Gift Log:\n")
	for _, k := range order {
		stack := groups[k]
		// Senders for the line — ordered by id for stable display.
		names := make([]string, 0, len(stack))
		totalMod := 0
		voteResult := stack[0].VoteResult
		for _, g := range stack {
			names = append(names, coopDisplayName(p, g.SenderID))
			totalMod += g.Modifier
		}
		result := voteResult
		if result == "" {
			result = "unresolved"
		}
		typeLabel := titleCaseWord(k.giftType)
		if len(stack) > 1 {
			typeLabel = fmt.Sprintf("%s ×%d", typeLabel, len(stack))
		}
		sb.WriteString(fmt.Sprintf("  Day %d: %s → %s → %s → %+d%%\n",
			k.day, strings.Join(names, " + "), typeLabel, result, totalMod))
	}
	return sb.String()
}
