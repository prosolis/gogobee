package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"gogobee/internal/db"
)

// ─── Threshold Constants ────��──────────────────────────────────────

const (
	// Communication
	thNovelistAvgWords      = 7
	thNovelistMinMsgs       = 100
	thMinimalistMaxAvgWords = 4
	thMinimalistMinMsgs     = 50
	thInquisitorPct         = 8
	thInquisitorMinMsgs     = 50
	thEnthusiastPct         = 8
	thEnthusiastMinMsgs     = 50
	thChatterboxMinMsgs     = 500
	thChatterboxMinAvgWords = 5
	thLinkmasterPct         = 5
	thLinkmasterMinMsgs     = 50

	// Temporal
	thNightOwlPct     = 40
	thNightOwlMinMsgs = 100
	thEarlyBirdPct    = 40
	thEarlyBirdMinMsgs = 100

	// Emotional (LLM-gated)
	thEmotionalMinClassified    = 100
	thCheerleaderPosPct         = 50
	thPhilosopherNeutPct        = 40
	thPhilosopherQPct           = 10
	thPhilosopherAvgWords       = 8
	thAgitatorNegPct            = 30
	thAgitatorMinMsgs           = 200
	thWildcardStdDev            = 0.5
	thWildcardMinClassified     = 150
	thHypeMachineExclPct        = 20
	thHypeMachinePosPct         = 60

	// Economy
	thBrokeSpiritedMaxBalance = 100.0
	thDegenerateMinLosses     = 10

	// Games
	thSharkWinRate           = 55
	thSharkMinGames          = 15
	thWordleMinPuzzles       = 10
	thArenaChampMinTier      = 4
	thArenaChampWinRate      = 50
	thArenaCowardMinRuns     = 3
	thArenaCowardMaxAvgTier  = 2
	thTriviaNerdMinCorrect   = 10

	// Adventure
	thAdvMinDays           = 10
	thAdvDiverseMinDays    = 15
	thAdvDiverseMinTypes   = 3
	thAdvDiverseMaxShare   = 40
	thGearheadMinMasterwork = 3

	// Communication (vocabulary)
	thWordsmithMinFancyWords = 10

	// Social
	thPatronMinRepGiven      = 5
	thPatronRatioMultiplier  = 2

	// Display
	maxDisplayArchetypes = 6
)

// ─── Flavor Text ─────���─────────────────────────────────────────────

var archetypeFlavors = map[string]string{
	// Communication
	"Novelist":    "Writes in paragraphs. Has opinions. Probably re-reads their own messages.",
	"Minimalist":  "Says a lot with very little. You're never sure if they're fine or not.",
	"Inquisitor":  "Always asking. Never satisfied with the first answer. Probably has follow-ups.",
	"Enthusiast":  "Genuinely excited about things. All the things. Possibly all at once.",
	"Chatterbox":  "Has thoughts. Many thoughts. Shares them all. You wouldn't have it any other way.",
	"Linkmaster":  "The community's unofficial curator. Their tab count is not your business.",
	"Wordsmith":   "Uses words most people have to look up. The thesaurus fears them.",

	// Temporal
	"Night Owl":  "Awake when they probably shouldn't be. Thriving despite all evidence.",
	"Early Bird": "Up before anyone else. Has already formed opinions about the day.",

	// Emotional
	"Cheerleader":  "Lifts the room. Probably the reason someone didn't log off that one time.",
	"Philosopher":  "Thinks out loud. Asks questions that don't have comfortable answers.",
	"Agitator":     "Keeps things interesting. You don't always agree but you're never bored.",
	"Wildcard":     "You never quite know what you're getting. That's the charm. Mostly.",
	"Hype Machine": "Arrived and immediately made everything louder. The room is better for it.",

	// Economy
	"Whale":             "Has money. Spends money. Has more money somehow. The math is unclear.",
	"Degenerate":        "Knows exactly what they're doing. Does it anyway. Respects it.",
	"Broke But Spirited": "Down but not out. The pot fears them anyway.",

	// Games
	"Shark":          "Wins more than they should. GogoBee has opinions about this.",
	"Wordle Devotee": "Shows up every day. Rain or shine. Five letters at a time.",
	"Arena Champion": "Walks into the arena and the monsters check their insurance.",
	"Arena Coward":   "Shows up. Wins a little. Leaves immediately. Smart, actually.",
	"Trivia Nerd":    "Has retained an unreasonable amount of information.",

	// Adventure
	"Dungeon Crawler": "Always in the dungeon. The dungeon knows their name by now.",
	"The Miner":       "Steady. Patient. Has more ore than anyone needs. Still mining.",
	"The Forager":     "Out in the field while everyone else is underground. Knows where the good stuff is.",
	"The Angler":      "Patient beyond reason. Has caught things nobody else has caught.",
	"The Merchant":    "Buys. Sells. Optimizes. The economy is a puzzle and they're solving it.",
	"Resting Face":    "Technically participating. The numbers are still going up. Good enough.",
	"The Adventurer":  "Does a bit of everything. Refuses to specialize. Somehow works.",
	"Gearhead":        "Has the good stuff. Spent the time getting it. You can tell.",

	// Social
	"Patron":  "Gives credit generously. Asks for nothing in return. Suspicious, honestly.",
	"Reactor": "Expresses everything through emoji. An entire emotional life, no words required.",

	// Fallback
	"Regular": "Shows up. Participates. The backbone of any community, honestly.",
}

// ─��─ Mutual Exclusions ────��────────────────────────────────────────

var mutualExclusions = [][2]string{
	{"Novelist", "Minimalist"},
	{"Early Bird", "Night Owl"},
	{"Arena Champion", "Arena Coward"},
	{"Whale", "Broke But Spirited"},
}

// ─── Core Types ────────────���───────────────────────────────────────

type archetypeResult struct {
	Name        string
	Category    string
	SignalScore float64
	Flavor      string
}

// ─── Community Percentiles ──────────────────────���──────────────────

type communityPercentiles struct {
	whaleBalanceThreshold float64 // top 10%
	repGivenP75           int     // top 25% rep given
	reactionsGivenP75     int     // top 25% reactions given
	medianMsgCount        int     // median total_messages
}

func computePercentiles(d *sql.DB) communityPercentiles {
	var p communityPercentiles

	// Whale: top 10% balance
	var userCount int
	d.QueryRow(`SELECT COUNT(*) FROM euro_balances WHERE balance > 0`).Scan(&userCount)
	offset := userCount / 10
	if offset < 1 {
		offset = 1
	}
	d.QueryRow(`SELECT balance FROM euro_balances ORDER BY balance DESC LIMIT 1 OFFSET ?`, offset).Scan(&p.whaleBalanceThreshold)

	// Rep given: top 25%
	repGiven := queryIntList(d, `SELECT COUNT(*) FROM rep_cooldowns GROUP BY giver ORDER BY COUNT(*) DESC`)
	p.repGivenP75 = percentile75(repGiven)

	// Reactions given: top 25%
	reactionsGiven := queryIntList(d, `SELECT COUNT(*) FROM reaction_log GROUP BY sender ORDER BY COUNT(*) DESC`)
	p.reactionsGivenP75 = percentile75(reactionsGiven)

	// Median message count
	msgCounts := queryIntList(d, `SELECT total_messages FROM user_stats ORDER BY total_messages`)
	if len(msgCounts) > 0 {
		p.medianMsgCount = msgCounts[len(msgCounts)/2]
	}

	return p
}

func queryIntList(d *sql.DB, query string) []int {
	rows, err := d.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []int
	for rows.Next() {
		var v int
		if rows.Scan(&v) == nil {
			result = append(result, v)
		}
	}
	return result
}

func percentile75(sorted []int) int {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) / 4 // top 25% = first quarter of desc-sorted list
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// ─── Per-User Data Loaders ──────────────────────��──────────────────

type userData struct {
	userID string

	// user_stats
	totalMsgs    int
	totalWords   int
	totalLinks   int
	totalImages  int
	totalQuestions int
	totalExcl    int
	totalEmojis  int
	nightMsgs    int
	morningMsgs  int
	fancyWords   int

	// sentiment_stats
	sentPositive int
	sentNegative int
	sentNeutral  int

	// llm_classifications
	classifiedCount int
	sentVariance    float64

	// economy
	balance       float64
	gamblingLosses int
	recentGaming   bool

	// games
	bjPlayed, bjWon             int
	hmPlayed, hmWon             int
	holdemPlayed                int
	holdemNetPositive           bool
	unoSoloPlayed, unoSoloWon  int
	unoMultiPlayed, unoMultiWon int
	wordlePlayed                int
	triviaCorrect               int

	// arena
	arenaHighestTier int
	arenaRuns        int
	arenaWins        int
	arenaAvgCashTier float64

	// adventure
	advDays       int
	advActivities map[string]int // activity_type -> count
	masterworkCount int

	// social
	repGiven     int
	repReceived  int
	reactionsGiven int
}

func loadUserData(d *sql.DB, userID string) userData {
	u := userData{userID: userID, advActivities: make(map[string]int)}

	// user_stats
	d.QueryRow(`SELECT total_messages, total_words, total_links, total_images,
		total_questions, total_exclamations, total_emojis, night_messages, morning_messages,
		COALESCE(fancy_words, 0)
		FROM user_stats WHERE user_id = ?`, userID).Scan(
		&u.totalMsgs, &u.totalWords, &u.totalLinks, &u.totalImages,
		&u.totalQuestions, &u.totalExcl, &u.totalEmojis, &u.nightMsgs, &u.morningMsgs,
		&u.fancyWords)

	// sentiment_stats
	d.QueryRow(`SELECT COALESCE(positive,0), COALESCE(negative,0), COALESCE(neutral,0)
		FROM sentiment_stats WHERE user_id = ?`, userID).Scan(
		&u.sentPositive, &u.sentNegative, &u.sentNeutral)

	// llm_classifications count + variance
	d.QueryRow(`SELECT COUNT(*) FROM llm_classifications WHERE user_id = ?`, userID).Scan(&u.classifiedCount)
	if u.classifiedCount > 1 {
		d.QueryRow(`SELECT AVG(sentiment_score * sentiment_score) - AVG(sentiment_score) * AVG(sentiment_score)
			FROM llm_classifications WHERE user_id = ?`, userID).Scan(&u.sentVariance)
		if u.sentVariance < 0 {
			u.sentVariance = 0
		}
	}

	// economy
	d.QueryRow(`SELECT COALESCE(balance, 0) FROM euro_balances WHERE user_id = ?`, userID).Scan(&u.balance)

	// Count actual gambling losses (not bets/antes which are always negative)
	d.QueryRow(`SELECT COUNT(*) FROM euro_transactions WHERE user_id = ? AND amount < 0
		AND reason IN ('holdem_loss')`,
		userID).Scan(&u.gamblingLosses)
	// Add blackjack losses (games played minus games won)
	var bjLosses int
	d.QueryRow(`SELECT MAX(COALESCE(games_played,0) - COALESCE(games_won,0), 0) FROM blackjack_scores WHERE user_id = ?`,
		userID).Scan(&bjLosses)
	u.gamblingLosses += bjLosses

	var recentCount int
	d.QueryRow(`SELECT COUNT(*) FROM euro_transactions WHERE user_id = ?
		AND created_at > datetime('now', '-30 days')
		AND reason IN ('blackjack_bet','blackjack_win','holdem_loss','holdem_win',
		'uno_wager','uno_win','uno_multi_ante','uno_multi_win')`,
		userID).Scan(&recentCount)
	u.recentGaming = recentCount > 0

	// blackjack
	d.QueryRow(`SELECT COALESCE(games_played,0), COALESCE(games_won,0) FROM blackjack_scores WHERE user_id = ?`,
		userID).Scan(&u.bjPlayed, &u.bjWon)

	// hangman
	d.QueryRow(`SELECT COALESCE(games_played,0), COALESCE(games_won,0) FROM hangman_scores WHERE user_id = ?`,
		userID).Scan(&u.hmPlayed, &u.hmWon)

	// holdem
	var totalWon, totalLost int
	d.QueryRow(`SELECT COALESCE(hands_played,0), COALESCE(total_won,0), COALESCE(total_lost,0) FROM holdem_scores WHERE user_id = ?`,
		userID).Scan(&u.holdemPlayed, &totalWon, &totalLost)
	u.holdemNetPositive = totalWon > totalLost

	// uno solo
	d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN result='player_win' THEN 1 ELSE 0 END), 0) FROM uno_games WHERE player_id = ?`,
		userID).Scan(&u.unoSoloPlayed, &u.unoSoloWon)

	// uno multi
	d.QueryRow(`SELECT COUNT(*) FROM uno_multi_games WHERE player_ids LIKE ?`,
		"%"+userID+"%").Scan(&u.unoMultiPlayed)
	d.QueryRow(`SELECT COUNT(*) FROM uno_multi_games WHERE winner_id = ?`,
		userID).Scan(&u.unoMultiWon)

	// wordle
	d.QueryRow(`SELECT COALESCE(puzzles_played,0) FROM wordle_stats WHERE user_id = ?`,
		userID).Scan(&u.wordlePlayed)

	// trivia
	d.QueryRow(`SELECT COALESCE(SUM(correct),0) FROM trivia_scores WHERE user_id = ?`,
		userID).Scan(&u.triviaCorrect)

	// arena
	d.QueryRow(`SELECT COALESCE(highest_tier,0), COALESCE(total_runs,0) FROM arena_stats WHERE user_id = ?`,
		userID).Scan(&u.arenaHighestTier, &u.arenaRuns)

	d.QueryRow(`SELECT COUNT(*) FROM arena_runs WHERE user_id = ? AND status = 'cashed_out'`,
		userID).Scan(&u.arenaWins)

	d.QueryRow(`SELECT COALESCE(AVG(tier), 0) FROM arena_runs WHERE user_id = ? AND status = 'cashed_out'`,
		userID).Scan(&u.arenaAvgCashTier)

	// adventure
	d.QueryRow(`SELECT COUNT(DISTINCT DATE(logged_at)) FROM adventure_activity_log WHERE user_id = ?`,
		userID).Scan(&u.advDays)

	rows, err := d.Query(`SELECT activity_type, COUNT(*) FROM adventure_activity_log WHERE user_id = ? GROUP BY activity_type`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var atype string
			var cnt int
			if rows.Scan(&atype, &cnt) == nil {
				u.advActivities[atype] = cnt
			}
		}
	}

	d.QueryRow(`SELECT COUNT(*) FROM adventure_equipment WHERE user_id = ? AND masterwork = 1`,
		userID).Scan(&u.masterworkCount)

	// social
	d.QueryRow(`SELECT COUNT(*) FROM rep_cooldowns WHERE giver = ?`, userID).Scan(&u.repGiven)
	d.QueryRow(`SELECT COUNT(*) FROM rep_cooldowns WHERE receiver = ?`, userID).Scan(&u.repReceived)
	d.QueryRow(`SELECT COUNT(*) FROM reaction_log WHERE sender = ?`, userID).Scan(&u.reactionsGiven)

	return u
}

// ���── Archetype Evaluators ────────���─────────────────────────────────

func evaluateArchetypes(u userData, pct communityPercentiles) []archetypeResult {
	var results []archetypeResult

	m := max1arch(u.totalMsgs)

	// ── Communication Style ──

	avgWords := u.totalWords / m
	if u.totalMsgs >= thNovelistMinMsgs && avgWords >= thNovelistAvgWords {
		results = append(results, archetypeResult{
			Name: "Novelist", Category: "Communication",
			SignalScore: clampSignal(float64(avgWords-thNovelistAvgWords) / float64(thNovelistAvgWords)),
		})
	}

	if u.totalMsgs >= thMinimalistMinMsgs && avgWords <= thMinimalistMaxAvgWords {
		results = append(results, archetypeResult{
			Name: "Minimalist", Category: "Communication",
			SignalScore: clampSignal(float64(thMinimalistMaxAvgWords-avgWords+1) / float64(thMinimalistMaxAvgWords)),
		})
	}

	qPct := u.totalQuestions * 100 / m
	if u.totalMsgs >= thInquisitorMinMsgs && qPct >= thInquisitorPct {
		results = append(results, archetypeResult{
			Name: "Inquisitor", Category: "Communication",
			SignalScore: clampSignal(float64(qPct-thInquisitorPct) / float64(thInquisitorPct)),
		})
	}

	ePct := u.totalExcl * 100 / m
	if u.totalMsgs >= thEnthusiastMinMsgs && ePct >= thEnthusiastPct {
		results = append(results, archetypeResult{
			Name: "Enthusiast", Category: "Communication",
			SignalScore: clampSignal(float64(ePct-thEnthusiastPct) / float64(thEnthusiastPct)),
		})
	}

	if u.totalMsgs >= thChatterboxMinMsgs && avgWords >= thChatterboxMinAvgWords {
		results = append(results, archetypeResult{
			Name: "Chatterbox", Category: "Communication",
			SignalScore: clampSignal(float64(u.totalMsgs-thChatterboxMinMsgs) / float64(thChatterboxMinMsgs)),
		})
	}

	lPct := u.totalLinks * 100 / m
	if u.totalMsgs >= thLinkmasterMinMsgs && lPct >= thLinkmasterPct {
		results = append(results, archetypeResult{
			Name: "Linkmaster", Category: "Communication",
			SignalScore: clampSignal(float64(lPct-thLinkmasterPct) / float64(thLinkmasterPct)),
		})
	}

	if u.fancyWords >= thWordsmithMinFancyWords {
		results = append(results, archetypeResult{
			Name: "Wordsmith", Category: "Communication",
			SignalScore: clampSignal(float64(u.fancyWords) / float64(thWordsmithMinFancyWords*3)),
		})
	}

	// ── Temporal ──

	if u.totalMsgs >= thNightOwlMinMsgs {
		nightPct := u.nightMsgs * 100 / m
		if nightPct >= thNightOwlPct {
			results = append(results, archetypeResult{
				Name: "Night Owl", Category: "Temporal",
				SignalScore: clampSignal(float64(nightPct-thNightOwlPct) / float64(100-thNightOwlPct)),
			})
		}
	}

	if u.totalMsgs >= thEarlyBirdMinMsgs {
		morningPct := u.morningMsgs * 100 / m
		if morningPct >= thEarlyBirdPct {
			results = append(results, archetypeResult{
				Name: "Early Bird", Category: "Temporal",
				SignalScore: clampSignal(float64(morningPct-thEarlyBirdPct) / float64(100-thEarlyBirdPct)),
			})
		}
	}

	// ── Emotional Signature (LLM-gated) ──

	sentTotal := u.sentPositive + u.sentNegative + u.sentNeutral
	if u.classifiedCount >= thEmotionalMinClassified && sentTotal > 0 {
		posPct := u.sentPositive * 100 / sentTotal
		negPct := u.sentNegative * 100 / sentTotal
		neutPct := u.sentNeutral * 100 / sentTotal

		// Cheerleader
		if posPct >= thCheerleaderPosPct && u.repGiven >= pct.repGivenP75 && u.reactionsGiven >= pct.reactionsGivenP75 {
			results = append(results, archetypeResult{
				Name: "Cheerleader", Category: "Emotional",
				SignalScore: clampSignal(float64(posPct-thCheerleaderPosPct) / float64(100-thCheerleaderPosPct)),
			})
		}

		// Philosopher
		if neutPct >= thPhilosopherNeutPct && qPct >= thPhilosopherQPct && avgWords >= thPhilosopherAvgWords {
			results = append(results, archetypeResult{
				Name: "Philosopher", Category: "Emotional",
				SignalScore: clampSignal(float64(neutPct-thPhilosopherNeutPct) / float64(100-thPhilosopherNeutPct)),
			})
		}

		// Agitator
		if negPct >= thAgitatorNegPct && u.totalMsgs >= thAgitatorMinMsgs {
			results = append(results, archetypeResult{
				Name: "Agitator", Category: "Emotional",
				SignalScore: clampSignal(float64(negPct-thAgitatorNegPct) / float64(100-thAgitatorNegPct)),
			})
		}

		// Hype Machine
		if ePct >= thHypeMachineExclPct && posPct >= thHypeMachinePosPct && u.reactionsGiven >= pct.reactionsGivenP75 {
			results = append(results, archetypeResult{
				Name: "Hype Machine", Category: "Emotional",
				SignalScore: clampSignal((float64(ePct-thHypeMachineExclPct)/float64(thHypeMachineExclPct) +
					float64(posPct-thHypeMachinePosPct)/float64(100-thHypeMachinePosPct)) / 2),
			})
		}
	}

	// Wildcard (separate min classified threshold)
	if u.classifiedCount >= thWildcardMinClassified {
		stddev := math.Sqrt(u.sentVariance)
		if stddev >= thWildcardStdDev {
			results = append(results, archetypeResult{
				Name: "Wildcard", Category: "Emotional",
				SignalScore: clampSignal((stddev - thWildcardStdDev) / thWildcardStdDev),
			})
		}
	}

	// ── Economy ──

	if pct.whaleBalanceThreshold > 0 && u.balance >= pct.whaleBalanceThreshold {
		results = append(results, archetypeResult{
			Name: "Whale", Category: "Economy",
			SignalScore: clampSignal(u.balance / pct.whaleBalanceThreshold / 2),
		})
	}

	if u.gamblingLosses >= thDegenerateMinLosses && u.recentGaming {
		results = append(results, archetypeResult{
			Name: "Degenerate", Category: "Economy",
			SignalScore: clampSignal(float64(u.gamblingLosses) / float64(thDegenerateMinLosses*3)),
		})
	}

	if u.balance < thBrokeSpiritedMaxBalance && u.recentGaming {
		results = append(results, archetypeResult{
			Name: "Broke But Spirited", Category: "Economy",
			SignalScore: clampSignal((thBrokeSpiritedMaxBalance - u.balance) / thBrokeSpiritedMaxBalance),
		})
	}

	// ── Games ──

	totalGames := u.bjPlayed + u.hmPlayed + u.unoSoloPlayed + u.unoMultiPlayed
	totalWins := u.bjWon + u.hmWon + u.unoSoloWon + u.unoMultiWon
	if u.holdemPlayed > 0 {
		totalGames += u.holdemPlayed
		if u.holdemNetPositive {
			totalWins += u.holdemPlayed / 2 // approximate: net positive counts as ~50% wins
		}
	}
	if totalGames >= thSharkMinGames {
		winRate := totalWins * 100 / totalGames
		if winRate >= thSharkWinRate {
			results = append(results, archetypeResult{
				Name: "Shark", Category: "Games",
				SignalScore: clampSignal(float64(winRate-thSharkWinRate) / float64(100-thSharkWinRate)),
			})
		}
	}

	if u.wordlePlayed >= thWordleMinPuzzles {
		results = append(results, archetypeResult{
			Name: "Wordle Devotee", Category: "Games",
			SignalScore: clampSignal(float64(u.wordlePlayed-thWordleMinPuzzles) / float64(thWordleMinPuzzles)),
		})
	}

	if u.arenaHighestTier >= thArenaChampMinTier && u.arenaRuns > 0 {
		arenaWinRate := u.arenaWins * 100 / max1arch(u.arenaRuns)
		if arenaWinRate >= thArenaChampWinRate {
			results = append(results, archetypeResult{
				Name: "Arena Champion", Category: "Games",
				SignalScore: clampSignal(float64(u.arenaHighestTier-thArenaChampMinTier+1) / 2.0),
			})
		}
	}

	if u.arenaWins >= thArenaCowardMinRuns && u.arenaAvgCashTier > 0 && u.arenaAvgCashTier <= float64(thArenaCowardMaxAvgTier) {
		results = append(results, archetypeResult{
			Name: "Arena Coward", Category: "Games",
			SignalScore: clampSignal((float64(thArenaCowardMaxAvgTier) - u.arenaAvgCashTier + 1) / float64(thArenaCowardMaxAvgTier)),
		})
	}

	if u.triviaCorrect >= thTriviaNerdMinCorrect {
		results = append(results, archetypeResult{
			Name: "Trivia Nerd", Category: "Games",
			SignalScore: clampSignal(float64(u.triviaCorrect-thTriviaNerdMinCorrect) / float64(thTriviaNerdMinCorrect)),
		})
	}

	// ── Adventure ──

	if u.advDays >= thAdvMinDays {
		totalAdv := 0
		for _, c := range u.advActivities {
			totalAdv += c
		}
		if totalAdv > 0 {
			// Find plurality activity
			var topActivity string
			var topCount int
			for act, cnt := range u.advActivities {
				if cnt > topCount {
					topActivity = act
					topCount = cnt
				}
			}
			topShare := topCount * 100 / totalAdv

			activityArchetypes := map[string]string{
				"dungeon":  "Dungeon Crawler",
				"mining":   "The Miner",
				"foraging": "The Forager",
				"fishing":  "The Angler",
				"shop":     "The Merchant",
				"rest":     "Resting Face",
			}

			// Check for diverse adventurer first
			if len(u.advActivities) >= thAdvDiverseMinTypes && u.advDays >= thAdvDiverseMinDays && topShare <= thAdvDiverseMaxShare {
				results = append(results, archetypeResult{
					Name: "The Adventurer", Category: "Adventure",
					SignalScore: clampSignal(float64(len(u.advActivities)) / float64(thAdvDiverseMinTypes+2)),
				})
			} else if name, ok := activityArchetypes[topActivity]; ok {
				results = append(results, archetypeResult{
					Name: name, Category: "Adventure",
					SignalScore: clampSignal(float64(topShare) / 100.0),
				})
			}
		}
	}

	if u.masterworkCount >= thGearheadMinMasterwork {
		results = append(results, archetypeResult{
			Name: "Gearhead", Category: "Adventure",
			SignalScore: clampSignal(float64(u.masterworkCount) / float64(thGearheadMinMasterwork*2)),
		})
	}

	// ── Social ──

	if u.repGiven >= thPatronMinRepGiven {
		repReceived := max1arch(u.repReceived)
		if u.repGiven >= repReceived*thPatronRatioMultiplier {
			results = append(results, archetypeResult{
				Name: "Patron", Category: "Social",
				SignalScore: clampSignal(float64(u.repGiven) / float64(thPatronMinRepGiven*3)),
			})
		}
	}

	if u.reactionsGiven >= pct.reactionsGivenP75 && pct.reactionsGivenP75 > 0 && u.totalMsgs <= pct.medianMsgCount {
		results = append(results, archetypeResult{
			Name: "Reactor", Category: "Social",
			SignalScore: clampSignal(float64(u.reactionsGiven) / float64(pct.reactionsGivenP75*2)),
		})
	}

	// Fill in flavor text
	for i := range results {
		results[i].Flavor = archetypeFlavors[results[i].Name]
	}

	// Apply mutual exclusions
	results = applyExclusions(results)

	// Sort by signal score desc
	sort.Slice(results, func(i, j int) bool {
		return results[i].SignalScore > results[j].SignalScore
	})

	return results
}

func applyExclusions(results []archetypeResult) []archetypeResult {
	nameIdx := make(map[string]int)
	for i, r := range results {
		nameIdx[r.Name] = i
	}

	remove := make(map[int]bool)
	for _, pair := range mutualExclusions {
		idxA, hasA := nameIdx[pair[0]]
		idxB, hasB := nameIdx[pair[1]]
		if hasA && hasB {
			if results[idxA].SignalScore >= results[idxB].SignalScore {
				remove[idxB] = true
			} else {
				remove[idxA] = true
			}
		}
	}

	if len(remove) == 0 {
		return results
	}

	var filtered []archetypeResult
	for i, r := range results {
		if !remove[i] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// ─── Cache Read/Write ────────��─────────────────────────────────────

// GetUserArchetypes returns cached archetypes for a user, sorted by signal_score desc.
func GetUserArchetypes(userID string) []archetypeResult {
	d := db.Get()
	rows, err := d.Query(`SELECT archetype, category, signal_score, flavor
		FROM user_archetypes WHERE user_id = ? ORDER BY signal_score DESC`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []archetypeResult
	for rows.Next() {
		var r archetypeResult
		if rows.Scan(&r.Name, &r.Category, &r.SignalScore, &r.Flavor) == nil {
			results = append(results, r)
		}
	}
	return results
}

// GetUserArchetypesLimited returns up to maxDisplayArchetypes for profile cards.
func GetUserArchetypesLimited(userID string) []archetypeResult {
	results := GetUserArchetypes(userID)
	if len(results) > maxDisplayArchetypes {
		results = results[:maxDisplayArchetypes]
	}
	if len(results) == 0 {
		return []archetypeResult{{
			Name: "Regular", Category: "Fallback",
			Flavor: archetypeFlavors["Regular"],
		}}
	}
	return results
}

// ─── Refresh Engine ─────────────────────��──────────────────────────

// RefreshAllArchetypes recalculates archetypes for all users and writes to user_archetypes.
func RefreshAllArchetypes() {
	d := db.Get()

	// Get all user IDs
	rows, err := d.Query(`SELECT user_id FROM user_stats WHERE total_messages > 0`)
	if err != nil {
		slog.Error("archetype: failed to list users", "err", err)
		return
	}
	var userIDs []string
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			userIDs = append(userIDs, uid)
		}
	}
	rows.Close()

	if len(userIDs) == 0 {
		return
	}

	// Compute community percentiles once
	pct := computePercentiles(d)

	// Evaluate each user
	type userResult struct {
		userID string
		archs  []archetypeResult
	}
	var allResults []userResult

	for _, uid := range userIDs {
		u := loadUserData(d, uid)
		archs := evaluateArchetypes(u, pct)
		if len(archs) > 0 {
			allResults = append(allResults, userResult{uid, archs})
		}
	}

	// Write to cache in a transaction
	tx, err := d.Begin()
	if err != nil {
		slog.Error("archetype: failed to begin tx", "err", err)
		return
	}

	_, err = tx.Exec(`DELETE FROM user_archetypes`)
	if err != nil {
		tx.Rollback()
		slog.Error("archetype: failed to clear cache", "err", err)
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO user_archetypes (user_id, archetype, category, signal_score, flavor)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		slog.Error("archetype: failed to prepare insert", "err", err)
		return
	}
	defer stmt.Close()

	totalAssigned := 0
	for _, ur := range allResults {
		for _, a := range ur.archs {
			if _, err := stmt.Exec(ur.userID, a.Name, a.Category, a.SignalScore, a.Flavor); err != nil {
				slog.Error("archetype: insert failed", "user", ur.userID, "arch", a.Name, "err", err)
			}
			totalAssigned++
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("archetype: commit failed", "err", err)
		return
	}

	slog.Info("archetype: refresh complete", "users", len(allResults), "archetypes_assigned", totalAssigned)
}

// ─── Helpers ─────────���─────────────────────────────────────────────

func max1arch(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func clampSignal(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// FormatArchetypeNames returns a " · " separated list of archetype names.
func FormatArchetypeNames(archs []archetypeResult) string {
	if len(archs) == 0 {
		return "Regular"
	}
	names := make([]string, len(archs))
	for i, a := range archs {
		names[i] = a.Name
	}
	return strings.Join(names, " · ")
}

// FormatArchetypesFull returns a full display with names and flavor text.
func FormatArchetypesFull(archs []archetypeResult) string {
	if len(archs) == 0 {
		return fmt.Sprintf("**Regular**\n_%s_", archetypeFlavors["Regular"])
	}
	var sb strings.Builder
	for i, a := range archs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("**%s**\n_%s_", a.Name, a.Flavor))
	}
	return sb.String()
}
