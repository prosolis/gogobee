package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// coopTicker fires once per UTC day to:
//  1. Lock open invites whose 24h window has elapsed.
//  2. For each active run, resolve the day's floor and advance/complete.
//
// Reuses the daily_prefetch job-completion guard so a bot restart on the same
// UTC day does not double-process.
func (p *AdventurePlugin) coopTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		// Run at the same minute as the morning DM (08:00 UTC) — same cadence as
		// the rest of the adventure system.
		if now.Hour() != p.morningHour || now.Minute() != 0 {
			continue
		}
		dateKey := now.Format("2006-01-02")
		jobName := "coop_dungeon_daily"
		if db.JobCompleted(jobName, dateKey) {
			continue
		}
		slog.Info("coop: daily tick")
		p.coopProcessLocks()
		p.coopProcessActiveRuns()
		db.MarkJobCompleted(jobName, dateKey)
	}
}

// ── Lock Phase ──────────────────────────────────────────────────────────────

// coopProcessLocks closes invites whose 24h window has elapsed: locks parties
// of 2+, cancels solo runs.
func (p *AdventurePlugin) coopProcessLocks() {
	runs, err := loadOpenCoopRuns()
	if err != nil {
		slog.Error("coop: load open runs", "err", err)
		return
	}
	now := time.Now().UTC()
	for _, run := range runs {
		if now.Before(run.CreatedAt.Add(coopInviteWindow)) {
			continue
		}
		members, err := loadCoopMembers(run.ID)
		if err != nil {
			slog.Error("coop: load members for lock", "run", run.ID, "err", err)
			continue
		}
		if len(members) < coopMinPartySize {
			// Cancel solo runs and refund nothing (no funding has been spent yet).
			_ = setCoopRunStatus(run.ID, "cancelled")
			gr := gamesRoom()
			if gr != "" {
				_ = p.SendMessage(gr, fmt.Sprintf("⚔️ Tier %d Co-op #%d expired with no party. Cancelled.", run.Tier, run.ID))
			}
			_ = p.SendDM(run.LeaderID, fmt.Sprintf("Co-op #%d expired before anyone joined. No party, no run.", run.ID))
			continue
		}

		// Lock: deduct combat action from each member, mark active, post lock notice.
		if err := lockCoopRun(run.ID); err != nil {
			slog.Error("coop: lock run", "run", run.ID, "err", err)
			continue
		}
		for _, m := range members {
			char, err := loadAdvCharacter(m.UserID)
			if err != nil {
				continue
			}
			if char.CanDoCombat(false) {
				char.CombatActionsUsed++
				char.ActionTakenToday = true
				char.LastActionDate = now.Format("2006-01-02")
				_ = saveAdvCharacter(char)
			}
		}
		fresh, _ := loadCoopRun(run.ID)
		gr := gamesRoom()
		if gr != "" {
			_ = p.SendMessage(gr, renderCoopLock(fresh, members, p))
		}
		// Per-player DM with funding instructions.
		for _, m := range members {
			_ = p.SendDM(m.UserID, renderCoopFundingPrompt(fresh, 1))
		}
		// Generate and post day-1 floor event.
		p.coopGenerateAndPostEvent(fresh, members, 1)
	}
}

// coopGenerateAndPostEvent picks a category-weighted event for the given day,
// stores it, and posts the vote prompt to the game room with TwinBee narration.
func (p *AdventurePlugin) coopGenerateAndPostEvent(run *CoopRun, members []CoopMember, day int) {
	cat := pickCoopEventCategory(run.Tier)
	idx := pickCoopEvent(cat)
	rec := coopEventRecommended(cat, idx)
	if err := createCoopEvent(run.ID, day, cat, idx, rec); err != nil {
		slog.Error("coop: create event", "run", run.ID, "day", day, "err", err)
		return
	}
	gr := gamesRoom()
	if gr == "" {
		return
	}
	event, _ := loadCoopEvent(run.ID, day)
	if event == nil {
		return
	}
	postID, err := p.SendMessageID(gr, renderCoopEventPost(run, members, event))
	if err != nil {
		slog.Error("coop: post event", "err", err)
		return
	}
	_ = saveCoopEventPostID(run.ID, day, postID)
}

// ── Daily Resolution ────────────────────────────────────────────────────────

func (p *AdventurePlugin) coopProcessActiveRuns() {
	runs, err := loadActiveCoopRuns()
	if err != nil {
		slog.Error("coop: load active runs", "err", err)
		return
	}
	today := time.Now().UTC().Format("2006-01-02")
	for _, run := range runs {
		// Skip runs locked on this same tick — they need a full day for
		// funding decisions and voting before the first floor resolves.
		if run.LockedAt != nil && run.LockedAt.UTC().Format("2006-01-02") == today {
			continue
		}
		if err := p.coopResolveFloor(run); err != nil {
			slog.Error("coop: resolve floor", "run", run.ID, "err", err)
		}
	}
}

// coopResolveFloor processes the current day's floor: auto-plays missing
// funders, rolls outcome, advances or completes the run.
func (p *AdventurePlugin) coopResolveFloor(run *CoopRun) error {
	day := run.CurrentDay

	// Skip only if this run is fully terminal. A "resolved" day with the run
	// still in 'active' status means a prior tick crashed mid-distribution;
	// we must re-enter to finish via idempotent operations (per-row claims).
	if run.LastResolvedDay >= day && run.Status != "active" {
		return nil
	}

	members, err := loadCoopMembers(run.ID)
	if err != nil {
		return err
	}

	// Lock all party members in stable UserID order so concurrent !coop fund /
	// vote / giftvote commands don't race the resolver. Stable order rules
	// out lock-order deadlock with other goroutines that lock multiple users.
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
	mutexes := make([]*sync.Mutex, 0, len(members))
	for _, m := range members {
		mu := p.advUserLock(m.UserID)
		mu.Lock()
		mutexes = append(mutexes, mu)
	}
	defer func() {
		for _, mu := range mutexes {
			mu.Unlock()
		}
	}()

	// Reload the run after acquiring locks. A concurrent admin action or
	// another scheduler tick may have moved the state.
	freshRun, err := loadCoopRun(run.ID)
	if err != nil || freshRun == nil {
		return err
	}
	run = freshRun
	if run.Status != "active" {
		return nil
	}
	// Re-check skip after lock acquisition (another tick could have already
	// fully finalized — though our per-tick guard makes that unlikely).
	if run.LastResolvedDay >= day && run.Status != "active" {
		return nil
	}
	// Reload members under lock for fresh funding state.
	members, err = loadCoopMembers(run.ID)
	if err != nil {
		return err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })

	// Resume vs. fresh detection: if the day's event already has an outcome,
	// a prior tick rolled this floor before crashing. Reuse that outcome —
	// re-rolling would be the worst sin (different result on retry).
	event, _ := loadCoopEvent(run.ID, day)
	resuming := event != nil && event.Outcome != ""

	autoPlayed := []*CoopMember{}
	choices := make(map[string]CoopFundingTier, len(members))
	var floorWon bool
	var winning string
	var eventMod, successPct int
	var giftSummary string

	if resuming {
		floorWon = event.Outcome == "success"
		winning = event.WinningVote
		eventMod = event.ModifierApplied
		// Reconstruct choices for display purposes.
		for _, m := range members {
			tier, _ := coopMemberModifier(&m, day)
			choices[string(m.UserID)] = tier
		}
		successPct = 0 // unknown on resume; render hides it
	} else {
		// Auto-play any member who didn't fund.
		for i := range members {
			m := &members[i]
			if _, ok := m.DailyFunding[day]; !ok {
				m.DailyFunding[day] = CoopFundNone
				if err := saveCoopMemberFunding(m); err != nil {
					slog.Error("coop: auto-play save", "run", run.ID, "user", m.UserID, "err", err)
				}
				autoPlayed = append(autoPlayed, m)
			}
		}

		totalMod := 0
		tierDef := coopTierTable[run.Tier]
		for _, m := range members {
			tier, mod := coopMemberModifier(&m, day)
			choices[string(m.UserID)] = tier
			totalMod += mod
			char, err := loadAdvCharacter(m.UserID)
			if err != nil {
				continue
			}
			totalMod += coopLevelBonus(char.CombatLevel, tierDef.minLevel)
			totalMod += coopPetBonus(char)
		}

		// Lazy-create the day's event if missing — covers the crash window
		// between lock and the original event creation.
		if event == nil {
			cat := pickCoopEventCategory(run.Tier)
			idx := pickCoopEvent(cat)
			_ = createCoopEvent(run.ID, day, cat, idx, coopEventRecommended(cat, idx))
			event, _ = loadCoopEvent(run.ID, day)
		}

		if event != nil {
			winning = coopTallyVote(event, run.LeaderID)
			eventMod = coopEventOptionModifier(event.Category, event.EventIndex, winning)
			totalMod += eventMod
		}

		var giftMod int
		giftMod, giftSummary = p.coopResolveGifts(run, run.LeaderID, day)
		totalMod += giftMod

		successPct = 100 - run.BaseDifficulty + totalMod
		if successPct < 5 {
			successPct = 5
		}
		if successPct > 95 {
			successPct = 95
		}

		roll := rand.IntN(100)
		floorWon = roll < successPct

		// Persist event resolution — turns this into a "resume" path on retry.
		if event != nil {
			outcomeStr := "failure"
			if floorWon {
				outcomeStr = "success"
			}
			_ = resolveCoopEvent(run.ID, day, winning, outcomeStr, eventMod)
		}

		dailyPost := renderCoopDailyResult(p, run, members, day, choices, successPct, floorWon, autoPlayed, event, winning, eventMod)
		if giftSummary != "" {
			dailyPost += "\n\n" + giftSummary
		}
		if gr := gamesRoom(); gr != "" {
			_ = p.SendMessage(gr, dailyPost)
		}
	}

	gr := gamesRoom()
	if !floorWon {
		// Resolve bets first (idempotent via claimCoopBetPayout), then mark
		// the run wiped. Order matters: if status is set first and the bot
		// crashes, the next tick won't pick this run up.
		summary := p.coopResolveBets(run, false)
		giftLog := renderCoopGiftLog(p, run.ID)
		parts := []string{}
		if summary != "" {
			parts = append(parts, summary)
		}
		if giftLog != "" {
			parts = append(parts, giftLog)
		}
		if len(parts) > 0 && gr != "" {
			_ = p.SendMessage(gr, strings.Join(parts, "\n\n"))
		}
		_ = completeCoopRun(run.ID, "wiped", 0)
		_ = setCoopRunLastResolvedDay(run.ID, day)
		return nil
	}

	if day >= run.TotalDays {
		def := coopTierTable[run.Tier]
		reward := def.rewardBase
		p.coopDistributeReward(run, reward, members)
		_ = completeCoopRun(run.ID, "complete", reward)
		_ = setCoopRunLastResolvedDay(run.ID, day)
		return nil
	}
	if err := advanceCoopDay(run.ID, day+1); err != nil {
		return err
	}
	for _, m := range members {
		_ = p.SendDM(m.UserID, renderCoopFundingPrompt(run, day+1))
	}
	freshRun, _ = loadCoopRun(run.ID)
	if freshRun != nil {
		p.coopGenerateAndPostEvent(freshRun, members, day+1)
	}
	_ = setCoopRunLastResolvedDay(run.ID, day)
	return nil
}

// coopTallyVote counts votes; majority wins. Ties resolve to the leader's
// vote if it's among the tied options; otherwise the lexicographically first
// tied label (deterministic, since Go map iteration is randomized).
func coopTallyVote(event *CoopEvent, leader id.UserID) string {
	tally := map[string]int{}
	for _, v := range event.Votes {
		tally[v]++
	}
	bestCount := -1
	var winners []string
	for label, n := range tally {
		if n > bestCount {
			bestCount = n
			winners = []string{label}
		} else if n == bestCount {
			winners = append(winners, label)
		}
	}
	if len(winners) == 1 {
		return winners[0]
	}
	sort.Strings(winners)
	if leaderVote, ok := event.Votes[leader]; ok {
		for _, w := range winners {
			if w == leaderVote {
				return w
			}
		}
	}
	if len(winners) > 0 {
		return winners[0]
	}
	return "A"
}

func (p *AdventurePlugin) coopDistributeReward(run *CoopRun, totalReward int, members []CoopMember) {
	if len(members) == 0 || totalReward <= 0 {
		return
	}
	share := totalReward / len(members)
	gr := gamesRoom()
	var lines []string
	for _, m := range members {
		// Atomic claim: only credit if this is the first time we're paying
		// this member for this run. Skip silently on retry.
		claimed, err := claimCoopMemberPayout(run.ID, m.UserID, share)
		if err != nil || !claimed {
			continue
		}
		net, _ := communityTax(m.UserID, float64(share), coopAdventureRake)
		p.euro.Credit(m.UserID, net, "coop_dungeon_reward")
		lines = append(lines, fmt.Sprintf("  %s: €%.0f (after tax)", coopDisplayName(p, m.UserID), net))
		_ = p.SendDM(m.UserID, fmt.Sprintf("⚔️ Co-op #%d cleared. Your share: €%.0f.", run.ID, net))
	}

	// Generate item drops and distribute via weighted roll.
	itemSummary := p.coopDistributeItems(run, members)

	bettingSummary := p.coopResolveBets(run, true)
	giftLog := renderCoopGiftLog(p, run.ID)
	if gr != "" {
		msg := fmt.Sprintf("⚔️ Co-op #%d cleared. Reward pool: €%d.\n\n%s",
			run.ID, totalReward, strings.Join(lines, "\n"))
		if itemSummary != "" {
			msg += "\n\n" + itemSummary
		}
		if bettingSummary != "" {
			msg += "\n\n" + bettingSummary
		}
		if giftLog != "" {
			msg += "\n\n" + giftLog
		}
		_ = p.SendMessage(gr, msg)
	}
}

// coopDistributeItems generates item drops for a successful run and assigns
// each via weighted roll across party members (weight = total_contributed).
// Items are added to the winner's inventory; returns a summary line.
func (p *AdventurePlugin) coopDistributeItems(run *CoopRun, members []CoopMember) string {
	loc := findAdvLocationByTier(AdvActivityDungeon, run.Tier)
	if loc == nil {
		return ""
	}
	// Item count scales with tier: T1 = 2, T5 = 6.
	count := 2 + (run.Tier - 1)
	var allItems []AdvItem
	for i := 0; i < count; i++ {
		allItems = append(allItems, generateAdvLoot(loc, false, 0)...)
	}
	if len(allItems) == 0 {
		return ""
	}
	totalWeight := 0
	for _, m := range members {
		totalWeight += m.TotalContributed
	}

	var lines []string
	for _, item := range allItems {
		winner := coopWeightedItemWinner(members, totalWeight)
		_ = addAdvInventoryItem(winner, item)
		lines = append(lines, fmt.Sprintf("  %s (€%d) → %s", item.Name, item.Value, coopDisplayName(p, winner)))
	}
	if mwLine := p.coopMaybeMasterwork(run, members, totalWeight); mwLine != "" {
		lines = append(lines, mwLine)
	}
	return "Item drops:\n" + strings.Join(lines, "\n")
}

// coopMaybeMasterwork grants a tier-appropriate masterwork drop on T4 (25%)
// or T5 (guaranteed). Picks a random masterwork def at the run's tier from
// the existing pool (mining sword / fishing armor / foraging boots), awards
// it via the same weighted roll, and announces in the game room.
//
// Returns a one-line summary to splice into the completion post, or "".
func (p *AdventurePlugin) coopMaybeMasterwork(run *CoopRun, members []CoopMember, totalWeight int) string {
	if run.Tier < 4 {
		return ""
	}
	if run.Tier == 4 && rand.Float64() >= 0.25 {
		return ""
	}

	// Collect all masterwork defs at the run's tier.
	candidates := []MasterworkDef{}
	for _, def := range masterworkDefs {
		if def.Tier == run.Tier {
			candidates = append(candidates, def)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	def := candidates[rand.IntN(len(candidates))]
	winner := coopWeightedItemWinner(members, totalWeight)
	if winner == "" {
		return ""
	}

	// Add to inventory as MasterworkGear (don't auto-equip — let the winner
	// choose via !adventure equip; their existing gear may be better).
	item := AdvItem{
		Name:        def.Name,
		Type:        "MasterworkGear",
		Tier:        def.Tier,
		Value:       0,
		Slot:        def.Slot,
		SkillSource: def.SkillSource,
	}
	if err := addAdvInventoryItem(winner, item); err != nil {
		slog.Error("coop: masterwork inventory add", "winner", winner, "err", err)
		return ""
	}

	char, _ := loadAdvCharacter(winner)
	if char != nil {
		char.MasterworkDropsReceived++
		_ = saveAdvCharacter(char)
	}

	dmText := fmt.Sprintf("⭐ **Co-op Masterwork Drop: %s** (T%d %s)\n\n_%s_\n\nMasterwork %s — 1.25x effectiveness, +5%% %s success.\n\nAdded to inventory. `!adventure equip` to use it.",
		def.Name, def.Tier, slotTitle(def.Slot), def.Description, slotTitle(def.Slot), def.SkillSource)
	_ = p.SendDM(winner, dmText)

	return fmt.Sprintf("⭐ Masterwork: %s (T%d %s) → %s",
		def.Name, def.Tier, slotTitle(def.Slot), coopDisplayName(p, winner))
}

// coopWeightedItemWinner picks a member with probability proportional to their
// total funding contribution. If no one has contributed (e.g., all None on
// every day), falls back to uniform random.
func coopWeightedItemWinner(members []CoopMember, totalWeight int) id.UserID {
	if len(members) == 0 {
		return ""
	}
	if totalWeight <= 0 {
		return members[rand.IntN(len(members))].UserID
	}
	r := rand.IntN(totalWeight)
	cum := 0
	for _, m := range members {
		cum += m.TotalContributed
		if r < cum {
			return m.UserID
		}
	}
	return members[len(members)-1].UserID
}
