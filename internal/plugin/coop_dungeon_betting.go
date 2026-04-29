package plugin

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Betting Constants ───────────────────────────────────────────────────────

const (
	coopBetMin      = 100
	coopBetMax      = 1_000_000
	coopBetRake     = 0.10 // 10% house cut, per spec
	coopOddsWindow  = 30   // helpfulness rolling-window size
	coopOddsHelpInf = 0.20 // helpfulness contribution to estimated success
)

// ── Bet Types ───────────────────────────────────────────────────────────────

type CoopBet struct {
	RunID    int
	PlayerID id.UserID
	Position string // "success" | "failure"
	Amount   int
	Payout   sql.NullInt64
}

// ── Command Handler ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleCoopBet(ctx MessageContext, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return p.SendDM(ctx.Sender, "Usage: `!coop bet <run_id> <amount> <success|failure>` or `!coop bet <amount> <success|failure>` for the most recent open/active run.")
	}

	// Two forms: with or without explicit run id.
	var runID, amount int
	var position string
	var err error

	if len(parts) >= 3 {
		runID, err = strconv.Atoi(parts[0])
		if err != nil {
			return p.SendDM(ctx.Sender, "First arg must be the run id.")
		}
		amount, err = strconv.Atoi(parts[1])
		if err != nil {
			return p.SendDM(ctx.Sender, "Amount must be a number.")
		}
		position = strings.ToLower(parts[2])
	} else {
		amount, err = strconv.Atoi(parts[0])
		if err != nil {
			return p.SendDM(ctx.Sender, "Amount must be a number.")
		}
		position = strings.ToLower(parts[1])
		run, _ := loadMostRecentBettableCoopRun()
		if run == nil {
			return p.SendDM(ctx.Sender, "No co-op runs accept bets right now.")
		}
		runID = run.ID
	}

	if position != "success" && position != "failure" {
		return p.SendDM(ctx.Sender, "Position must be `success` or `failure`.")
	}
	if amount < coopBetMin || amount > coopBetMax {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Bet must be between €%d and €%d.", coopBetMin, coopBetMax))
	}

	run, err := loadCoopRun(runID)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "Run not found.")
	}
	if run.Status != "open" && run.Status != "active" {
		return p.SendDM(ctx.Sender, "Bets are closed on this run.")
	}

	balance := p.euro.GetBalance(ctx.Sender)
	if balance < float64(amount) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You have €%.0f. Bet was €%d.", balance, amount))
	}

	existing, _ := loadCoopBet(runID, ctx.Sender)
	if existing != nil && existing.Position != position {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You already bet %s on this run. Positions can't be switched.", existing.Position))
	}

	if !p.euro.Debit(ctx.Sender, float64(amount), "coop_bet") {
		return p.SendDM(ctx.Sender, "Payment failed.")
	}

	if existing == nil {
		err = createCoopBet(runID, ctx.Sender, position, amount)
	} else {
		err = increaseCoopBet(runID, ctx.Sender, amount)
	}
	if err != nil {
		p.euro.Credit(ctx.Sender, float64(amount), "coop_bet_refund")
		return p.SendDM(ctx.Sender, "Couldn't save bet. Refunded.")
	}

	total := amount
	if existing != nil {
		total += existing.Amount
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("📊 Bet placed: €%d on %s (run #%d). Total stake: €%d.",
		amount, position, runID, total))
}

// ── Odds Calculation ────────────────────────────────────────────────────────

// coopEstimatedRunSuccess returns the estimated P(success) for the rest of the
// run given current party state. Used for spectator odds display.
func coopEstimatedRunSuccess(run *CoopRun, members []CoopMember) float64 {
	if run.Status != "open" && run.Status != "active" {
		return 0
	}
	if len(members) == 0 {
		return 0
	}
	tierDef := coopTierTable[run.Tier]

	// Per-floor success at "Standard" funding by all members (a neutral
	// baseline — actual funding varies day to day).
	mod := len(members) * coopFundingTable[CoopFundStandard].modifier
	for _, m := range members {
		char, err := loadAdvCharacter(m.UserID)
		if err != nil {
			continue
		}
		mod += coopLevelBonus(char.CombatLevel, tierDef.minLevel)
		mod += coopPetBonus(char)
	}
	successPct := 100 - run.BaseDifficulty + mod

	// TwinBee helpfulness shifts the line: a "helpful" history (close to 1)
	// bumps odds; an "unhelpful" history pulls them down.
	help := coopTwinBeeHelpfulness(coopOddsWindow)
	successPct += int(math.Round((help - 0.5) * 2 * coopOddsHelpInf * 100))

	if successPct < 5 {
		successPct = 5
	}
	if successPct > 95 {
		successPct = 95
	}

	floorsRemaining := run.TotalDays - run.CurrentDay
	if floorsRemaining < 0 {
		floorsRemaining = 0
	}
	if run.Status == "open" {
		floorsRemaining = run.TotalDays
	}

	return math.Pow(float64(successPct)/100.0, float64(floorsRemaining))
}

// coopParimutuelPayouts returns winning side, losers' side, rake, and
// per-player payout map (winning bettors only).
func coopParimutuelPayouts(bets []CoopBet, winningPosition string) (winners []CoopBet, totalPool, rake int, payouts map[id.UserID]int) {
	for _, b := range bets {
		totalPool += b.Amount
	}
	rake = int(math.Floor(float64(totalPool) * coopBetRake))
	netPool := totalPool - rake

	winningPool := 0
	for _, b := range bets {
		if b.Position == winningPosition {
			winners = append(winners, b)
			winningPool += b.Amount
		}
	}

	payouts = map[id.UserID]int{}
	if winningPool == 0 {
		// No winners — house keeps everything (this happens if e.g. all bets
		// were on the wrong side).
		return
	}
	for _, b := range winners {
		share := float64(b.Amount) / float64(winningPool)
		payouts[b.PlayerID] = int(math.Floor(share * float64(netPool)))
	}
	return
}

// ── DB ──────────────────────────────────────────────────────────────────────

func createCoopBet(runID int, userID id.UserID, position string, amount int) error {
	d := db.Get()
	_, err := d.Exec(`INSERT INTO coop_dungeon_bets (run_id, player_id, position, amount)
		VALUES (?, ?, ?, ?)`, runID, string(userID), position, amount)
	return err
}

func increaseCoopBet(runID int, userID id.UserID, delta int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_bets SET amount = amount + ?
		WHERE run_id = ? AND player_id = ?`, delta, runID, string(userID))
	return err
}

func loadCoopBet(runID int, userID id.UserID) (*CoopBet, error) {
	d := db.Get()
	b := &CoopBet{}
	var uid string
	err := d.QueryRow(`SELECT run_id, player_id, position, amount, payout
		FROM coop_dungeon_bets WHERE run_id = ? AND player_id = ?`, runID, string(userID)).Scan(
		&b.RunID, &uid, &b.Position, &b.Amount, &b.Payout)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.PlayerID = id.UserID(uid)
	return b, nil
}

func loadCoopBets(runID int) ([]CoopBet, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT run_id, player_id, position, amount, payout
		FROM coop_dungeon_bets WHERE run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoopBet
	for rows.Next() {
		b := CoopBet{}
		var uid string
		if err := rows.Scan(&b.RunID, &uid, &b.Position, &b.Amount, &b.Payout); err != nil {
			return nil, err
		}
		b.PlayerID = id.UserID(uid)
		out = append(out, b)
	}
	return out, rows.Err()
}

func setCoopBetPayout(runID int, userID id.UserID, payout int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_bets SET payout = ?
		WHERE run_id = ? AND player_id = ?`, payout, runID, string(userID))
	return err
}

// claimCoopBetPayout atomically claims a bet for payout: returns true only if
// this call wins the race (the row had payout IS NULL and we set it). Used to
// prevent double-credit on retry after a mid-resolution crash.
func claimCoopBetPayout(runID int, userID id.UserID, payout int) (bool, error) {
	d := db.Get()
	res, err := d.Exec(`UPDATE coop_dungeon_bets SET payout = ?
		WHERE run_id = ? AND player_id = ? AND payout IS NULL`,
		payout, runID, string(userID))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func loadMostRecentBettableCoopRun() (*CoopRun, error) {
	runs, err := queryCoopRuns(`status IN ('open','active') ORDER BY id DESC LIMIT 1`)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return runs[0], nil
}

// ── Resolution ──────────────────────────────────────────────────────────────

// coopResolveBets is called when a run completes. It computes payouts,
// credits winners, persists payout values, and posts a summary line.
func (p *AdventurePlugin) coopResolveBets(run *CoopRun, won bool) string {
	bets, err := loadCoopBets(run.ID)
	if err != nil || len(bets) == 0 {
		return ""
	}
	winningPosition := "failure"
	if won {
		winningPosition = "success"
	}
	winners, total, rake, payouts := coopParimutuelPayouts(bets, winningPosition)

	// Idempotent: claim each bet's payout slot (UPDATE...WHERE payout IS NULL)
	// before crediting. If the claim fails, this bet was already paid by a
	// prior resolution attempt — skip the credit. Prevents double-pay on
	// crash-restart.
	for _, b := range winners {
		payout := payouts[b.PlayerID]
		claimed, err := claimCoopBetPayout(run.ID, b.PlayerID, payout)
		if err != nil || !claimed {
			continue
		}
		if payout > 0 {
			p.euro.Credit(b.PlayerID, float64(payout), "coop_bet_payout")
		}
	}
	for _, b := range bets {
		if b.Position != winningPosition && !b.Payout.Valid {
			_ = setCoopBetPayout(run.ID, b.PlayerID, 0)
		}
	}

	if len(winners) == 0 {
		return fmt.Sprintf("📊 Parimutuel pool: €%d. No winners — house (TwinBee) keeps the pool.", total)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Parimutuel pool: €%d. Rake: €%d. Net: €%d.\n", total, rake, total-rake))
	sb.WriteString(fmt.Sprintf("Winners (bet on %s):\n", winningPosition))
	for _, b := range winners {
		sb.WriteString(fmt.Sprintf("  %s: €%d → €%d\n", coopDisplayName(p, b.PlayerID), b.Amount, payouts[b.PlayerID]))
	}
	return sb.String()
}

// ── Render ──────────────────────────────────────────────────────────────────

func renderCoopOddsLine(run *CoopRun, members []CoopMember, bets []CoopBet) string {
	pSuccess := coopEstimatedRunSuccess(run, members)
	successPool, failurePool := 0, 0
	for _, b := range bets {
		if b.Position == "success" {
			successPool += b.Amount
		} else {
			failurePool += b.Amount
		}
	}
	total := successPool + failurePool
	return fmt.Sprintf("📊 Estimated success: %.0f%%. Pool €%d (success €%d / failure €%d). Rake %.0f%%. `!coop bet <amount> success|failure`.",
		pSuccess*100, total, successPool, failurePool, coopBetRake*100)
}

