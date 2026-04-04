package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// ── Constants ────────────────────────────────────────────────────────────────

const (
	rivalMinCombatLevel   = 5
	rivalChallengeWindow  = 24 * time.Hour
	rivalSamePairCooldown = 7 * 24 * time.Hour
	rivalMinIntervalHours = 3 * 24 // 3 days in hours
	rivalMaxIntervalHours = 4 * 24 // 4 days in hours
)

// ── Types ────────────────────────────────────────────────────────────────────

type advRivalChallenge struct {
	ChallengeID  string
	ChallengerID id.UserID
	ChallengedID id.UserID
	Stake        int
	Round        int
	PlayerScore  int
	RivalScore   int
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

type advRivalRecord struct {
	RivalID    id.UserID
	Wins       int
	Losses     int
	LastDuelAt *time.Time
}

type advPendingRivalRPS struct {
	ChallengeID string
}

// ── Stake Calculation ────────────────────────────────────────────────────────

func rivalStake(combatLevel int) int {
	return (combatLevel / 5) * 1000
}

// ── Rival Pool Unlock ────────────────────────────────────────────────────────

func (p *AdventurePlugin) checkRivalPoolUnlock(char *AdventureCharacter) {
	if char.CombatLevel >= rivalMinCombatLevel && char.RivalPool == 0 {
		char.RivalPool = 1
		if !char.RivalUnlockedNotified {
			char.RivalUnlockedNotified = true
			p.SendDM(char.UserID, rivalUnlockDM)
		}
	}
}

// ── DB CRUD ──────────────────────────────────────────────────────────────────

func loadRivalChallengeByID(challengeID string) (*advRivalChallenge, error) {
	d := db.Get()
	c := &advRivalChallenge{}
	err := d.QueryRow(`
		SELECT challenge_id, challenger_id, challenged_id, stake,
		       round, player_score, rival_score, expires_at, created_at
		FROM adventure_rival_challenges WHERE challenge_id = ?`, challengeID).Scan(
		&c.ChallengeID, &c.ChallengerID, &c.ChallengedID, &c.Stake,
		&c.Round, &c.PlayerScore, &c.RivalScore, &c.ExpiresAt, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func insertRivalChallenge(c *advRivalChallenge) error {
	db.Exec("rival: insert challenge",
		`INSERT INTO adventure_rival_challenges
		 (challenge_id, challenger_id, challenged_id, stake, round, player_score, rival_score, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ChallengeID, string(c.ChallengerID), string(c.ChallengedID),
		c.Stake, c.Round, c.PlayerScore, c.RivalScore, c.ExpiresAt,
	)
	return nil
}

func saveRivalChallengeRound(c *advRivalChallenge) {
	db.Exec("rival: update round",
		`UPDATE adventure_rival_challenges
		 SET round = ?, player_score = ?, rival_score = ?
		 WHERE challenge_id = ?`,
		c.Round, c.PlayerScore, c.RivalScore, c.ChallengeID,
	)
}

func deleteRivalChallenge(challengeID string) {
	db.Exec("rival: delete challenge",
		`DELETE FROM adventure_rival_challenges WHERE challenge_id = ?`,
		challengeID,
	)
}

func upsertRivalRecord(userID, rivalID id.UserID, won bool) {
	if won {
		db.Exec("rival: upsert record win",
			`INSERT INTO adventure_rival_records (user_id, rival_id, wins, losses, last_duel_at)
			 VALUES (?, ?, 1, 0, CURRENT_TIMESTAMP)
			 ON CONFLICT(user_id, rival_id) DO UPDATE SET wins = wins + 1, last_duel_at = CURRENT_TIMESTAMP`,
			string(userID), string(rivalID),
		)
	} else {
		db.Exec("rival: upsert record loss",
			`INSERT INTO adventure_rival_records (user_id, rival_id, wins, losses, last_duel_at)
			 VALUES (?, ?, 0, 1, CURRENT_TIMESTAMP)
			 ON CONFLICT(user_id, rival_id) DO UPDATE SET losses = losses + 1, last_duel_at = CURRENT_TIMESTAMP`,
			string(userID), string(rivalID),
		)
	}
}

func loadAllRivalRecords(userID id.UserID) ([]advRivalRecord, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT rival_id, wins, losses, last_duel_at
		FROM adventure_rival_records
		WHERE user_id = ?
		ORDER BY last_duel_at DESC`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []advRivalRecord
	for rows.Next() {
		r := advRivalRecord{}
		var lastDuel sql.NullTime
		if err := rows.Scan(&r.RivalID, &r.Wins, &r.Losses, &lastDuel); err != nil {
			return nil, err
		}
		if lastDuel.Valid {
			r.LastDuelAt = &lastDuel.Time
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func communityPotAdd(amount int) {
	db.Exec("rival: community pot add",
		`INSERT INTO community_pot (id, balance, updated_at)
		 VALUES (1, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP`,
		amount, amount,
	)
}

func communityPotBalance() int {
	d := db.Get()
	var balance int
	_ = d.QueryRow(`SELECT COALESCE(balance, 0) FROM community_pot WHERE id = 1`).Scan(&balance)
	return balance
}

func communityPotDebit(amount int) bool {
	d := db.Get()
	res, err := d.Exec(`UPDATE community_pot SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1 AND balance >= ?`, amount, amount)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func loadExpiredRivalChallenges() ([]advRivalChallenge, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT challenge_id, challenger_id, challenged_id, stake,
		       round, player_score, rival_score, expires_at, created_at
		FROM adventure_rival_challenges
		WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var challenges []advRivalChallenge
	for rows.Next() {
		c := advRivalChallenge{}
		if err := rows.Scan(
			&c.ChallengeID, &c.ChallengerID, &c.ChallengedID, &c.Stake,
			&c.Round, &c.PlayerScore, &c.RivalScore, &c.ExpiresAt, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		challenges = append(challenges, c)
	}
	return challenges, rows.Err()
}

func lastRivalChallengeTime() time.Time {
	d := db.Get()
	var t sql.NullTime
	_ = d.QueryRow(`SELECT MAX(created_at) FROM adventure_rival_challenges`).Scan(&t)
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

func hasActiveChallenge(userID id.UserID) bool {
	d := db.Get()
	var count int
	_ = d.QueryRow(`
		SELECT COUNT(*) FROM adventure_rival_challenges
		WHERE (challenger_id = ? OR challenged_id = ?) AND expires_at > CURRENT_TIMESTAMP`,
		string(userID), string(userID)).Scan(&count)
	return count > 0
}

func lastDuelBetween(a, b id.UserID) time.Time {
	d := db.Get()
	var t sql.NullTime
	_ = d.QueryRow(`
		SELECT last_duel_at FROM adventure_rival_records
		WHERE user_id = ? AND rival_id = ?`, string(a), string(b)).Scan(&t)
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

// ── Pair Selection ───────────────────────────────────────────────────────────

func (p *AdventurePlugin) selectRivalPair() (*AdventureCharacter, *AdventureCharacter) {
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("rival: load characters", "err", err)
		return nil, nil
	}

	// Filter to eligible pool members.
	var pool []AdventureCharacter
	for _, c := range chars {
		if c.RivalPool == 0 || !c.Alive || c.BabysitActive {
			continue
		}
		if hasActiveChallenge(c.UserID) {
			continue
		}
		stake := rivalStake(c.CombatLevel)
		if stake <= 0 {
			continue
		}
		bal := p.euro.GetBalance(c.UserID)
		if bal < float64(stake) {
			continue
		}
		pool = append(pool, c)
	}

	if len(pool) < 2 {
		return nil, nil
	}

	// Shuffle and find first valid pair.
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })

	now := time.Now().UTC()
	for i := 0; i < len(pool); i++ {
		for j := i + 1; j < len(pool); j++ {
			a, b := &pool[i], &pool[j]
			// Check same-pair cooldown (7 days).
			last := lastDuelBetween(a.UserID, b.UserID)
			if !last.IsZero() && now.Sub(last) < rivalSamePairCooldown {
				continue
			}
			// Randomly assign challenger/challenged.
			if rand.IntN(2) == 0 {
				return a, b
			}
			return b, a
		}
	}
	return nil, nil
}

// ── Challenge Issuance ───────────────────────────────────────────────────────

func (p *AdventurePlugin) issueRivalChallenge(challenger, challenged *AdventureCharacter) {
	stake := rivalStake(challenged.CombatLevel)
	challenge := &advRivalChallenge{
		ChallengeID:  uuid.New().String()[:12],
		ChallengerID: challenger.UserID,
		ChallengedID: challenged.UserID,
		Stake:        stake,
		Round:        1,
		PlayerScore:  0,
		RivalScore:   0,
		ExpiresAt:    time.Now().UTC().Add(rivalChallengeWindow),
	}
	insertRivalChallenge(challenge)

	// Notify the rival (challenger) that a challenge was issued in their name.
	p.SendDM(challenger.UserID, fmt.Sprintf(
		"⚔️ You've been matched as a rival against **%s**. A challenge has been issued in your name. "+
			"Your throws will be generated automatically. Sit back and wait for the result.",
		challenged.DisplayName))

	// Send the dramatic challenge DM to the challenged player.
	openingTaunt := pickRivalFlavor(rivalOpeningTaunts)
	dm := fmt.Sprintf(`⚔️ RIVAL CHALLENGE

You're walking along enjoying a nice sunny day when you look across the road and notice someone staring at you intensely -- the look of someone who caught a faint but unmistakable smell at a restaurant and has traced it back to its source.

The person crosses the street. The sun disappears behind dark, angry clouds as if on cue.

*"You're looking at me like you got a problem."*

They look you over slowly. The scowl becomes a smug smile.

*"Your face looks like someone who's fought countless battles and lost just as many. Ah well. I already crossed the street so I guess I'll make this quick."*

They reach to their side. You ready yourself.

The stakes are €%d. Best of 3. Rock Paper Scissors.

*%s*

Reply with **rock**, **paper**, or **scissors** for Round 1.
You have 24 hours.`, stake, openingTaunt)

	p.SendDM(challenged.UserID, dm)

	// Store pending interaction for the challenged player.
	p.pending.Store(string(challenged.UserID), &advPendingInteraction{
		Type:      "rival_rps",
		Data:      &advPendingRivalRPS{ChallengeID: challenge.ChallengeID},
		ExpiresAt: challenge.ExpiresAt,
	})

	slog.Info("rival: challenge issued",
		"challenger", challenger.DisplayName,
		"challenged", challenged.DisplayName,
		"stake", stake)
}

// ── RPS Resolution ───────────────────────────────────────────────────────────

type rpsThrow int

const (
	rpsRock rpsThrow = iota
	rpsPaper
	rpsScissors
)

var rpsNames = map[rpsThrow]string{
	rpsRock:     "rock",
	rpsPaper:    "paper",
	rpsScissors: "scissors",
}

func parseRPS(s string) (rpsThrow, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "rock", "r":
		return rpsRock, true
	case "paper", "p":
		return rpsPaper, true
	case "scissors", "s", "scissor":
		return rpsScissors, true
	}
	return 0, false
}

func randomRPS() rpsThrow {
	return rpsThrow(rand.IntN(3))
}

// rpsResult: 1 = player wins, -1 = rival wins, 0 = tie.
func rpsResult(player, rival rpsThrow) int {
	if player == rival {
		return 0
	}
	if (player == rpsRock && rival == rpsScissors) ||
		(player == rpsPaper && rival == rpsRock) ||
		(player == rpsScissors && rival == rpsPaper) {
		return 1
	}
	return -1
}

func (p *AdventurePlugin) resolveRivalRPSRound(ctx MessageContext, interaction *advPendingInteraction) error {
	// Acquire per-user lock to prevent concurrent resolution from rapid messages.
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	data := interaction.Data.(*advPendingRivalRPS)

	playerThrow, ok := parseRPS(ctx.Body)
	if !ok {
		// Re-store the pending interaction so the player can try again.
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Rock, paper, or scissors. That's it. Those are the options.")
	}

	challenge, err := loadRivalChallengeByID(data.ChallengeID)
	if err != nil || challenge == nil {
		// Challenge was expired/deleted by the ticker — don't re-store pending.
		return p.SendDM(ctx.Sender, "That challenge is no longer active.")
	}

	// Resolve throw.
	rivalThrow := randomRPS()
	result := rpsResult(playerThrow, rivalThrow)

	// Tie — both re-throw. Re-prompt the player.
	if result == 0 {
		tieLine := pickRivalFlavor(rivalTied)
		text := fmt.Sprintf("You threw %s. They threw %s. Tie.\n\n*%s*\n\n"+
			"Reply with **rock**, **paper**, or **scissors** to re-throw.",
			rpsNames[playerThrow], rpsNames[rivalThrow], tieLine)

		// Re-store the pending interaction for the same round.
		p.pending.Store(string(ctx.Sender), &advPendingInteraction{
			Type:      "rival_rps",
			Data:      &advPendingRivalRPS{ChallengeID: challenge.ChallengeID},
			ExpiresAt: challenge.ExpiresAt,
		})

		return p.SendDM(ctx.Sender, text)
	}

	// Build the round DM.
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("⚔️ ROUND %d RESULT\n\n", challenge.Round))
	sb.WriteString(fmt.Sprintf("You threw %s. They threw %s.\n\n", rpsNames[playerThrow], rpsNames[rivalThrow]))

	if result == 1 {
		// Player won the round.
		challenge.PlayerScore++
		// Outcome line.
		outcomePool := rivalRoundOutcomeWin
		outcome := pickRivalFlavor(outcomePool)
		if strings.Contains(outcome, "%s") {
			outcome = fmt.Sprintf(outcome, rpsNames[rivalThrow], rpsNames[playerThrow])
		}
		sb.WriteString(outcome + "\n\n")
		sb.WriteString(fmt.Sprintf("Score: You %d -- Rival %d\n\n", challenge.PlayerScore, challenge.RivalScore))
		// Rival reaction (they lost the round).
		sb.WriteString(fmt.Sprintf("*%s*\n", pickRivalFlavor(rivalRoundLost)))
	} else {
		// Rival won the round.
		challenge.RivalScore++
		outcomePool := rivalRoundOutcomeLoss
		outcome := pickRivalFlavor(outcomePool)
		if strings.Contains(outcome, "%s") {
			outcome = fmt.Sprintf(outcome, rpsNames[rivalThrow], rpsNames[playerThrow])
		}
		sb.WriteString(outcome + "\n\n")
		sb.WriteString(fmt.Sprintf("Score: You %d -- Rival %d\n\n", challenge.PlayerScore, challenge.RivalScore))
		// Rival reaction (they won the round).
		sb.WriteString(fmt.Sprintf("*%s*\n", pickRivalFlavor(rivalRoundWon)))
	}

	// Check for match end (best of 3 = first to 2).
	if challenge.PlayerScore >= 2 || challenge.RivalScore >= 2 {
		playerWon := challenge.PlayerScore >= 2
		saveRivalChallengeRound(challenge)
		p.pending.Delete(string(ctx.Sender))
		p.finalizeRivalMatch(challenge, playerWon, &sb)
		return p.SendDM(ctx.Sender, sb.String())
	}

	// Match continues — next round.
	challenge.Round++
	saveRivalChallengeRound(challenge)

	sb.WriteString(fmt.Sprintf("\nReply with **rock**, **paper**, or **scissors** for Round %d.", challenge.Round))

	// Update pending interaction for next round.
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "rival_rps",
		Data:      &advPendingRivalRPS{ChallengeID: challenge.ChallengeID},
		ExpiresAt: challenge.ExpiresAt,
	})

	return p.SendDM(ctx.Sender, sb.String())
}

// ── Match Finalization ───────────────────────────────────────────────────────

func (p *AdventurePlugin) finalizeRivalMatch(challenge *advRivalChallenge, playerWon bool, sb *strings.Builder) {
	challengerChar, _ := loadAdvCharacter(challenge.ChallengerID)
	challengedChar, _ := loadAdvCharacter(challenge.ChallengedID)

	var winnerID, loserID id.UserID
	var winnerName, loserName string

	if playerWon {
		winnerID = challenge.ChallengedID
		loserID = challenge.ChallengerID
		winnerName = ""
		loserName = ""
		if challengedChar != nil {
			winnerName = challengedChar.DisplayName
		}
		if challengerChar != nil {
			loserName = challengerChar.DisplayName
		}
	} else {
		winnerID = challenge.ChallengerID
		loserID = challenge.ChallengedID
		if challengerChar != nil {
			winnerName = challengerChar.DisplayName
		}
		if challengedChar != nil {
			loserName = challengedChar.DisplayName
		}
	}

	// Gold transfer — try full stake, fall back to available balance.
	stake := challenge.Stake
	if !p.euro.Debit(loserID, float64(stake), "rival_duel_loss") {
		// Loser can't cover full stake — debit whatever they have.
		bal := p.euro.GetBalance(loserID)
		stake = int(bal)
		if stake > 0 {
			// Best-effort debit of remaining balance. If it fails (concurrent drain), stake becomes 0.
			if !p.euro.Debit(loserID, float64(stake), "rival_duel_loss_partial") {
				stake = 0
			}
		}
	}

	winnerShare := stake / 2
	potShare := stake - winnerShare
	if winnerShare > 0 {
		p.euro.Credit(winnerID, float64(winnerShare), "rival_duel_win")
	}
	if potShare > 0 {
		communityPotAdd(potShare)
	}

	// Update rival records (both directions).
	upsertRivalRecord(challenge.ChallengedID, challenge.ChallengerID, playerWon)
	upsertRivalRecord(challenge.ChallengerID, challenge.ChallengedID, !playerWon)

	// Append closing line to the challenged player's DM.
	sb.WriteString("\n⚔️ DUEL RESOLVED\n\n")
	sb.WriteString(fmt.Sprintf("Final score: You %d -- Rival %d\n", challenge.PlayerScore, challenge.RivalScore))

	if playerWon {
		sb.WriteString(fmt.Sprintf("€%d added to your balance. €%d added to the community pot.\n\n", winnerShare, potShare))
		sb.WriteString(fmt.Sprintf("*%s*\n", pickRivalFlavor(rivalClosingLoss)))
	} else {
		sb.WriteString(fmt.Sprintf("€%d removed from your balance. €%d added to the community pot.\n\n", stake, potShare))
		sb.WriteString(fmt.Sprintf("*%s*\n", pickRivalFlavor(rivalClosingWin)))
	}

	// Load W/L record for display.
	records, _ := loadAllRivalRecords(challenge.ChallengedID)
	for _, r := range records {
		if r.RivalID == challenge.ChallengerID {
			rivalName := string(challenge.ChallengerID)
			if challengerChar != nil {
				rivalName = challengerChar.DisplayName
			}
			sb.WriteString(fmt.Sprintf("Record vs %s: %dW-%dL\n", rivalName, r.Wins, r.Losses))
			break
		}
	}

	// Send closing DM to the rival (challenger).
	var rivalDM strings.Builder
	rivalDM.WriteString("⚔️ DUEL RESOLVED\n\n")
	rivalDM.WriteString(fmt.Sprintf("Your challenge against **%s** has concluded.\n",
		func() string {
			if challengedChar != nil {
				return challengedChar.DisplayName
			}
			return string(challenge.ChallengedID)
		}()))
	rivalDM.WriteString(fmt.Sprintf("Final score: %s %d -- %s %d\n",
		func() string {
			if challengedChar != nil {
				return challengedChar.DisplayName
			}
			return "Them"
		}(), challenge.PlayerScore,
		func() string {
			if challengerChar != nil {
				return challengerChar.DisplayName
			}
			return "You"
		}(), challenge.RivalScore))

	if !playerWon {
		rivalDM.WriteString(fmt.Sprintf("You won! +€%d to your balance. €%d to the community pot.\n", winnerShare, potShare))
	} else {
		rivalDM.WriteString(fmt.Sprintf("You lost. €%d removed from your balance.\n", stake))
	}
	p.SendDM(challenge.ChallengerID, rivalDM.String())

	// Room announcement.
	gr := gamesRoom()
	if gr != "" {
		var announce string
		if challenge.Round >= 3 {
			announce = fmt.Sprintf("⚔️ DUEL OUTCOME DECIDED! **%s** has snatched €%d from **%s** in a nail-biting best of 3!",
				winnerName, stake, loserName)
		} else {
			announce = fmt.Sprintf("⚔️ DUEL OUTCOME DECIDED! **%s** has snatched €%d from **%s**!",
				winnerName, stake, loserName)
		}
		p.SendMessage(gr, announce)
	}

	// Delete the challenge row.
	deleteRivalChallenge(challenge.ChallengeID)

	slog.Info("rival: match resolved",
		"winner", winnerName, "loser", loserName,
		"stake", stake, "rounds", challenge.Round)
}

// ── Expiry Handling ──────────────────────────────────────────────────────────

func (p *AdventurePlugin) expireRivalChallenges() {
	expired, err := loadExpiredRivalChallenges()
	if err != nil {
		slog.Error("rival: load expired challenges", "err", err)
		return
	}

	for _, challenge := range expired {
		// Challenged player forfeits — rival wins.
		challengerChar, _ := loadAdvCharacter(challenge.ChallengerID)
		challengedChar, _ := loadAdvCharacter(challenge.ChallengedID)

		stake := challenge.Stake
		if !p.euro.Debit(challenge.ChallengedID, float64(stake), "rival_forfeit") {
			bal := p.euro.GetBalance(challenge.ChallengedID)
			stake = int(bal)
			if stake > 0 {
				if !p.euro.Debit(challenge.ChallengedID, float64(stake), "rival_forfeit_partial") {
					stake = 0
				}
			}
		}

		winnerShare := stake / 2
		potShare := stake - winnerShare
		if winnerShare > 0 {
			p.euro.Credit(challenge.ChallengerID, float64(winnerShare), "rival_forfeit_win")
		}
		if potShare > 0 {
			communityPotAdd(potShare)
		}

		upsertRivalRecord(challenge.ChallengedID, challenge.ChallengerID, false)
		upsertRivalRecord(challenge.ChallengerID, challenge.ChallengedID, true)

		// DM the challenged player (forfeit notice).
		forfeitLine := pickRivalFlavor(rivalForfeitLines)
		p.SendDM(challenge.ChallengedID, fmt.Sprintf("⚔️ DUEL FORFEITED\n\n%s\n\n€%d removed from your balance.",
			forfeitLine, stake))

		// DM the challenger.
		challengedName := string(challenge.ChallengedID)
		if challengedChar != nil {
			challengedName = challengedChar.DisplayName
		}
		p.SendDM(challenge.ChallengerID, fmt.Sprintf(
			"⚔️ Your rival **%s** didn't respond in time. You win by default. +€%d to your balance.",
			challengedName, winnerShare))

		// Room announcement.
		gr := gamesRoom()
		if gr != "" {
			winnerName := string(challenge.ChallengerID)
			if challengerChar != nil {
				winnerName = challengerChar.DisplayName
			}
			loserName := challengedName
			p.SendMessage(gr, fmt.Sprintf(
				"⚔️ DUEL OUTCOME DECIDED! **%s** has snatched €%d from **%s**, who couldn't even be bothered to show up!",
				winnerName, stake, loserName))
		}

		deleteRivalChallenge(challenge.ChallengeID)
		p.pending.Delete(string(challenge.ChallengedID))

		slog.Info("rival: challenge expired",
			"challenger", challenge.ChallengerID,
			"challenged", challenge.ChallengedID,
			"stake", stake)
	}
}

// ── Challenge Scheduler ──────────────────────────────────────────────────────

func (p *AdventurePlugin) rivalChallengeTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Roll the next challenge interval once. Re-roll after each issued challenge.
	nextIntervalHours := rivalMinIntervalHours + rand.IntN(rivalMaxIntervalHours-rivalMinIntervalHours+1)

	for range ticker.C {
		now := time.Now().UTC()

		// Only fire on the hour.
		if now.Minute() != 0 {
			continue
		}

		// Only issue challenges between 08:00 and 22:00 UTC.
		if now.Hour() < 8 || now.Hour() >= 22 {
			// Still check for expiry outside challenge hours.
			p.expireRivalChallenges()
			continue
		}

		// Expire old challenges first.
		p.expireRivalChallenges()

		// Check if enough time has passed since last challenge.
		last := lastRivalChallengeTime()
		if !last.IsZero() && now.Sub(last) < time.Duration(nextIntervalHours)*time.Hour {
			continue
		}

		// Try to issue a challenge.
		challenger, challenged := p.selectRivalPair()
		if challenger == nil || challenged == nil {
			continue
		}

		p.issueRivalChallenge(challenger, challenged)

		// Roll a fresh interval for the next challenge.
		nextIntervalHours = rivalMinIntervalHours + rand.IntN(rivalMaxIntervalHours-rivalMinIntervalHours+1)
	}
}

// ── Rivals Command ───────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleRivalsCmd(ctx MessageContext) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return err
	}

	if char.RivalPool == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("You need Combat Level %d to enter the rival pool. Currently: %d.",
				rivalMinCombatLevel, char.CombatLevel))
	}

	records, err := loadAllRivalRecords(char.UserID)
	if err != nil || len(records) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"⚔️ You're in the rival pool but haven't been challenged yet. Your time will come.")
	}

	var sb strings.Builder
	sb.WriteString("⚔️ **Rival Record**\n\n")
	for _, r := range records {
		rivalChar, _ := loadAdvCharacter(r.RivalID)
		name := string(r.RivalID)
		if rivalChar != nil {
			name = rivalChar.DisplayName
		}
		daysAgo := ""
		if r.LastDuelAt != nil {
			days := int(time.Since(*r.LastDuelAt).Hours() / 24)
			if days == 0 {
				daysAgo = "today"
			} else if days == 1 {
				daysAgo = "1 day ago"
			} else {
				daysAgo = fmt.Sprintf("%d days ago", days)
			}
		}
		sb.WriteString(fmt.Sprintf("  %-16s %dW - %dL   last duel: %s\n", name, r.Wins, r.Losses, daysAgo))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
