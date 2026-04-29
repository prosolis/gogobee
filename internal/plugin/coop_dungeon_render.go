package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

func coopDisplayName(p *AdventurePlugin, userID id.UserID) string {
	char, err := loadAdvCharacter(userID)
	if err == nil && char.DisplayName != "" {
		return char.DisplayName
	}
	if p != nil {
		return p.DisplayName(userID)
	}
	return string(userID)
}

func renderCoopInvite(run *CoopRun, members []CoopMember, leaderName string) string {
	def := coopTierTable[run.Tier]
	closes := run.CreatedAt.Add(coopInviteWindow).Format("Mon 15:04 UTC")
	return fmt.Sprintf(
		"⚔️ %s is opening a Co-op Dungeon — **Tier %d (%s)**, run #%d.\n"+
			"Days: %d. Reward pool base: €%d (split among the party).\n"+
			"Party: %d/%d. Type `!coop join %d` to enter.\n"+
			"Locks at %s. Newbies are a liability — you knew that going in.",
		leaderName, run.Tier, def.difficulty, run.ID,
		def.totalDays, def.rewardBase,
		len(members), coopMaxPartySize, run.ID, closes)
}

func renderCoopLock(run *CoopRun, members []CoopMember, p *AdventurePlugin) string {
	var sb strings.Builder
	def := coopTierTable[run.Tier]
	sb.WriteString(fmt.Sprintf("⚔️ Tier %d Co-op #%d is locked.\n", run.Tier, run.ID))
	sb.WriteString(fmt.Sprintf("Difficulty: %s. Days: %d. Reward pool: €%d.\n\nParty (turn order):\n",
		def.difficulty, run.TotalDays, def.rewardBase))
	for _, m := range members {
		warn := ""
		if m.IsLiability {
			warn = " ⚠️ liability"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%s\n", m.TurnOrder+1, coopDisplayName(p, m.UserID), warn))
	}
	sb.WriteString("\nDay 1 begins now. Each member has until the next daily tick to fund. Inactive = None (-10%).")
	bets, _ := loadCoopBets(run.ID)
	sb.WriteString("\n\n")
	sb.WriteString(renderCoopOddsLine(run, members, bets))
	return sb.String()
}

func renderCoopFundingPrompt(run *CoopRun, day int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚔️ Co-op #%d — Day %d/%d. Pick your funding tier with `!coop fund <tier>`.\n\n",
		run.ID, day, run.TotalDays))
	for _, t := range coopFundingOrder {
		def := coopFundingTable[t]
		sb.WriteString(fmt.Sprintf("  • `%s` — €%d, %+d%%\n", t, def.cost, def.modifier))
	}
	sb.WriteString("\nNo decision before the next daily tick = auto-played as None (-10%). Funding is non-refundable.")
	return sb.String()
}

func renderCoopDailyResult(p *AdventurePlugin, run *CoopRun, members []CoopMember, day int,
	choices map[string]CoopFundingTier, successPct int, won bool, autoPlayed []*CoopMember,
	event *CoopEvent, winning string, eventMod int) string {

	var sb strings.Builder
	def := coopTierTable[run.Tier]
	header := "✅ Floor cleared"
	if !won {
		header = "💥 Wipe"
	}
	if won && day >= run.TotalDays {
		header = "🏆 Final floor cleared"
	}

	sb.WriteString(fmt.Sprintf("⚔️ Co-op #%d — Tier %d (%s), Day %d/%d\n",
		run.ID, run.Tier, def.difficulty, day, run.TotalDays))
	sb.WriteString(fmt.Sprintf("Success chance: %d%%.  %s.\n", successPct, header))

	if event != nil {
		sb.WriteString(fmt.Sprintf("Floor event: %s. Party voted **%s** (%+d%%).\n",
			titleCaseWord(string(event.Category)), winning, eventMod))
	}
	sb.WriteString("\nFunding:\n")

	for _, m := range members {
		t := choices[string(m.UserID)]
		fdef := coopFundingTable[t]
		auto := ""
		for _, ap := range autoPlayed {
			if ap.UserID == m.UserID {
				auto = " ← auto-played"
				break
			}
		}
		sb.WriteString(fmt.Sprintf("  %s: %s (%+d%%)%s\n",
			coopDisplayName(p, m.UserID), fdef.label, fdef.modifier, auto))
	}

	if event != nil {
		sb.WriteString("\n_TwinBee:_ ")
		sb.WriteString(coopTwinBeeReaction(event, winning, won))
	}

	if !won {
		sb.WriteString("\n\nThe dungeon does not editorialize. Combat actions for today are refunded if any. Funding is gone.")
	} else if day < run.TotalDays {
		sb.WriteString(fmt.Sprintf("\n\nNext floor unlocks at the next daily tick. %d day(s) remain.", run.TotalDays-day))
		bets, _ := loadCoopBets(run.ID)
		sb.WriteString("\n")
		sb.WriteString(renderCoopOddsLine(run, members, bets))
	}
	return sb.String()
}

// coopTwinBeeReaction picks an outcome line based on whether his recommendation
// matched the winning vote and whether the floor succeeded.
func coopTwinBeeReaction(event *CoopEvent, winning string, won bool) string {
	pickFrom := func(pool []string) string {
		if len(pool) == 0 {
			return ""
		}
		return pool[rand.IntN(len(pool))]
	}
	switch {
	case event.Recommended == winning && won:
		return pickFrom(TwinBeeOutcomeCorrect)
	case event.Recommended == winning && !won:
		return pickFrom(TwinBeeOutcomeWrong)
	case event.Recommended != winning && won:
		return pickFrom(TwinBeeOutcomeNotRecommendedGood)
	default:
		return pickFrom(TwinBeeOutcomeNotRecommendedBad)
	}
}

// renderCoopEventPost is the game-room post for a floor event vote prompt.
// Edited in place as votes come in.
func renderCoopEventPost(run *CoopRun, members []CoopMember, event *CoopEvent) string {
	var sb strings.Builder
	flavorPool := coopEventCategoryFlavor(event.Category)
	flavor := ""
	if event.EventIndex < len(flavorPool) {
		flavor = flavorPool[event.EventIndex]
	}
	sb.WriteString(fmt.Sprintf("🗺️ **Co-op #%d — Day %d/%d, %s event**\n\n",
		run.ID, event.Day, run.TotalDays, titleCaseWord(string(event.Category))))
	sb.WriteString(flavor)
	sb.WriteString("\n\nVote with `!coop vote A` (or B/C). 24h or until next tick. Ties go to the leader.\n\n")

	tally := map[string]int{}
	for _, v := range event.Votes {
		tally[v]++
	}
	parts := []string{}
	for _, label := range coopEventOptionLabels(event.Category, event.EventIndex) {
		parts = append(parts, fmt.Sprintf("%s (%d)", label, tally[label]))
	}
	sb.WriteString("Current votes: ")
	sb.WriteString(strings.Join(parts, " · "))

	abstained := []string{}
	for _, m := range members {
		if _, voted := event.Votes[m.UserID]; !voted {
			abstained = append(abstained, string(m.UserID))
		}
	}
	if len(abstained) > 0 && len(abstained) < len(members) {
		sb.WriteString(fmt.Sprintf("\nAwaiting: %d of %d", len(abstained), len(members)))
	}
	return sb.String()
}

func titleCaseWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func renderCoopStatus(p *AdventurePlugin, run *CoopRun, members []CoopMember) string {
	var sb strings.Builder
	def := coopTierTable[run.Tier]
	sb.WriteString(fmt.Sprintf("⚔️ Co-op #%d — Tier %d (%s)\n", run.ID, run.Tier, def.difficulty))
	sb.WriteString(fmt.Sprintf("Status: %s.  Day %d/%d.  Pool: €%d.\n\nParty:\n",
		run.Status, run.CurrentDay, run.TotalDays, run.GoldPool))

	for _, m := range members {
		warn := ""
		if m.IsLiability {
			warn = " ⚠️"
		}
		todayLabel := "—"
		if run.CurrentDay >= 1 {
			if t, ok := m.DailyFunding[run.CurrentDay]; ok {
				todayLabel = coopFundingTable[t].label
			} else {
				todayLabel = "(unset)"
			}
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%s — today: %s, contributed: €%d\n",
			m.TurnOrder+1, coopDisplayName(p, m.UserID), warn, todayLabel, m.TotalContributed))
	}
	if run.Status == "open" {
		closes := run.CreatedAt.Add(coopInviteWindow)
		sb.WriteString(fmt.Sprintf("\nLocks in: %s.", time.Until(closes).Truncate(time.Minute)))
	}
	if run.Status == "open" || run.Status == "active" {
		bets, _ := loadCoopBets(run.ID)
		sb.WriteString("\n")
		sb.WriteString(renderCoopOddsLine(run, members, bets))
	}

	// Pending stacks with per-stack countdowns. Each game-room post is one
	// stack (lead + N followers); we show the lead's id and the stack size.
	if run.Status == "active" {
		all, _ := loadAllCoopGifts(run.ID)
		// Group unresolved gifts by lead. Followers contribute to the size
		// but use the lead's expiry/votes.
		type stackInfo struct {
			lead  CoopGift
			size  int
		}
		stacks := map[int]*stackInfo{}
		order := []int{}
		for _, g := range all {
			if g.VoteResult != "" {
				continue
			}
			leadID := g.ID
			if g.StackLeadID != nil {
				leadID = *g.StackLeadID
			}
			s, ok := stacks[leadID]
			if !ok {
				s = &stackInfo{}
				stacks[leadID] = s
				order = append(order, leadID)
			}
			if g.StackLeadID == nil {
				s.lead = g
			}
			s.size++
		}
		if len(stacks) > 0 {
			sb.WriteString("\n\nPending gifts:\n")
			for _, leadID := range order {
				s := stacks[leadID]
				opens, leaves := 0, 0
				for _, v := range s.lead.Votes {
					if v == "open" {
						opens++
					} else if v == "leave" {
						leaves++
					}
				}
				countdown := "—"
				if s.lead.ExpiresAt != nil {
					remaining := time.Until(*s.lead.ExpiresAt).Truncate(time.Minute)
					if remaining > 0 {
						countdown = remaining.String()
					} else {
						countdown = "closing"
					}
				}
				stackLabel := fmt.Sprintf("#%d", s.lead.ID)
				if s.size > 1 {
					stackLabel = fmt.Sprintf("#%d ×%d", s.lead.ID, s.size)
				}
				sb.WriteString(fmt.Sprintf("  %s (day %d) — open %d / leave %d — closes in %s\n",
					stackLabel, s.lead.Day, opens, leaves, countdown))
			}
		}
	}
	return sb.String()
}
