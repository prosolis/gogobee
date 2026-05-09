package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── TwinBee Action Selection ─────────────────────────────────────────────────

type twinBeeActionWeight struct {
	Activity AdvActivityType
	Tier     int
	Weight   float64
}

var twinBeeWeights = []twinBeeActionWeight{
	{AdvActivityDungeon, 3, 0.35},
	{AdvActivityDungeon, 4, 0.25},
	{AdvActivityDungeon, 5, 0.10},
	{AdvActivityMining, 3, 0.15},
	{AdvActivityForaging, 3, 0.15},
}

// twinBeeMaxTier returns the highest tier TwinBee should visit,
// based on the best player's combined adventure level.
func twinBeeMaxTier() int {
	chars, err := loadAllAdvCharacters()
	if err != nil || len(chars) == 0 {
		return 3
	}
	bestLevel := 0
	for _, c := range chars {
		if !c.Alive {
			continue
		}
		combined := c.CombatLevel + c.MiningSkill + c.ForagingSkill + c.FishingSkill
		if combined > bestLevel {
			bestLevel = combined
		}
	}
	// Tier 3: anyone (combined 3+)
	// Tier 4: best player has combined 12+ (avg level 4 per skill)
	// Tier 5: best player has combined 21+ (avg level 7 per skill)
	switch {
	case bestLevel >= 21:
		return 5
	case bestLevel >= 12:
		return 4
	default:
		return 3
	}
}

func selectTwinBeeAction() (AdvActivityType, *AdvLocation) {
	maxTier := twinBeeMaxTier()

	// Filter weights to only include tiers within range.
	var filtered []twinBeeActionWeight
	var totalWeight float64
	for _, w := range twinBeeWeights {
		if w.Tier <= maxTier {
			filtered = append(filtered, w)
			totalWeight += w.Weight
		}
	}

	if len(filtered) == 0 {
		loc := findAdvLocationByTier(AdvActivityDungeon, 3)
		return AdvActivityDungeon, loc
	}

	roll := rand.Float64() * totalWeight
	cumulative := 0.0
	for _, w := range filtered {
		cumulative += w.Weight
		if roll < cumulative {
			loc := findAdvLocationByTier(w.Activity, w.Tier)
			return w.Activity, loc
		}
	}
	// Fallback
	loc := findAdvLocationByTier(filtered[0].Activity, filtered[0].Tier)
	return filtered[0].Activity, loc
}

// ── TwinBee Result ───────────────────────────────────────────────────────────

type TwinBeeResult struct {
	Activity   AdvActivityType
	Location   *AdvLocation
	Outcome    AdvOutcomeType
	LootValue  int64
	LootDesc   string
	FlavorText string
}

// simulateTwinBeeOutcome rolls a flat outcome distribution for TwinBee's
// off-screen daily run. TwinBee never dies — death rolls become empty.
// Distribution by tier roughly matches the legacy simulator's spread:
// tier 3 → 60% success / 25% exceptional / 15% empty; tier 4 → 55/20/25;
// tier 5 → 50/15/35 (deeper places are stingier even for TwinBee).
func simulateTwinBeeOutcome(tier int) AdvOutcomeType {
	roll := rand.IntN(100)
	var emptyPct, successPct int
	switch tier {
	case 5:
		emptyPct, successPct = 35, 50
	case 4:
		emptyPct, successPct = 25, 55
	default:
		emptyPct, successPct = 15, 60
	}
	switch {
	case roll < emptyPct:
		return AdvOutcomeEmpty
	case roll < emptyPct+successPct:
		return AdvOutcomeSuccess
	default:
		return AdvOutcomeExceptional
	}
}

// simulateTwinBeeLoot returns gold value + 1–3 themed item names for the
// rolled outcome. Empty → zero/no items. Exceptional ≈ 2.2× success base.
func simulateTwinBeeLoot(loc *AdvLocation, outcome AdvOutcomeType) (int64, []string) {
	if outcome == AdvOutcomeEmpty || loc == nil {
		return 0, nil
	}
	tier := loc.Tier
	if tier < 1 {
		tier = 1
	}
	// Base loot scales as tier^2 * 75 with ±25% jitter.
	base := int64(tier*tier*75) + int64(rand.IntN(tier*tier*40))
	if outcome == AdvOutcomeExceptional {
		base = base * 22 / 10
	}
	// Item names borrow from the location's denizens string for flavor; no
	// inventory is actually deposited for TwinBee — these are display only.
	names := strings.Split(loc.Denizens, ", ")
	picks := 1 + rand.IntN(2)
	if outcome == AdvOutcomeExceptional {
		picks = 2 + rand.IntN(2)
	}
	if picks > len(names) {
		picks = len(names)
	}
	chosen := make([]string, 0, picks)
	rand.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
	for i := 0; i < picks && i < len(names); i++ {
		chosen = append(chosen, strings.TrimSpace(names[i]))
	}
	return base, chosen
}

func (p *AdventurePlugin) runTwinBeeDaily() *TwinBeeResult {
	activity, loc := selectTwinBeeAction()
	if loc == nil {
		slog.Error("adventure: twinbee could not find location")
		return nil
	}

	outcome := simulateTwinBeeOutcome(loc.Tier)
	value, items := simulateTwinBeeLoot(loc, outcome)

	tbResult := &TwinBeeResult{
		Activity:  activity,
		Location:  loc,
		Outcome:   outcome,
		LootValue: value,
	}
	if len(items) > 0 {
		tbResult.LootDesc = joinAdvItems(items)
	}

	tbResult.FlavorText = p.selectTwinBeeFlavor(tbResult)

	return tbResult
}

func (p *AdventurePlugin) selectTwinBeeFlavor(result *TwinBeeResult) string {
	var pool []string
	switch result.Outcome {
	case AdvOutcomeExceptional:
		pool = TwinBeeExceptional
	case AdvOutcomeSuccess:
		pool = TwinBeeSuccess
	case AdvOutcomeEmpty, AdvOutcomeHornets, AdvOutcomeCaveIn, AdvOutcomeBear, AdvOutcomeRiver:
		// Check if it was a "tactical withdrawal" (would have been death)
		// Since we rerolled death → empty, we can't distinguish here.
		// Use the empty pool; the withdrawal pool is for the summary one-liners.
		pool = TwinBeeEmpty
	default:
		pool = TwinBeeEmpty
	}

	if len(pool) == 0 {
		return "TwinBee went to " + result.Location.Name + ". Results pending."
	}

	text := pool[rand.IntN(len(pool))]
	vars := map[string]string{
		"{location}": result.Location.Name,
		"{loot}":     result.LootDesc,
		"{value}":    fmt.Sprintf("%.0f", float64(result.LootValue)),
		"{xp}":       "0",
	}
	return advSubstituteFlavor(text, vars)
}

func joinAdvItems(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return fmt.Sprintf("%s, and %s", joinAdvItems(names[:len(names)-1]), names[len(names)-1])
	}
}

// ── TwinBee Reward Distribution ──────────────────────────────────────────────

type TwinBeeRewardSummary struct {
	Eligible  int
	GoldShare int64
	GiftCount int
}

func (p *AdventurePlugin) distributeTwinBeeRewards(result *TwinBeeResult) TwinBeeRewardSummary {
	summary := TwinBeeRewardSummary{}

	if result == nil {
		return summary
	}

	// Find eligible players: anyone who took action today (including dead)
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("adventure: twinbee rewards failed to load chars", "err", err)
		return summary
	}

	var eligible []id.UserID
	for _, c := range chars {
		if c.HasActedToday() {
			eligible = append(eligible, c.UserID)
		}
	}
	summary.Eligible = len(eligible)

	if len(eligible) == 0 || result.LootValue == 0 {
		// Gift rolls still happen even if no loot
		if len(eligible) > 0 {
			for _, uid := range eligible {
				if rollTwinBeeGift(uid) {
					summary.GiftCount++
				}
			}
		}
		p.logTwinBeeResult(result, summary)
		return summary
	}

	// Distribute gold weighted by combined adventure level.
	// Higher-level players get a proportionally larger share.
	type eligiblePlayer struct {
		uid    id.UserID
		weight int
	}
	var players []eligiblePlayer
	totalWeight := 0
	for _, uid := range eligible {
		weight := 4 // minimum (level 1 in all 4 skills)
		for _, c := range chars {
			if c.UserID == uid {
				weight = c.CombatLevel + c.MiningSkill + c.ForagingSkill + c.FishingSkill
				if weight < 4 {
					weight = 4
				}
				break
			}
		}
		weight = weight * weight // quadratic scaling — veterans get much more
		players = append(players, eligiblePlayer{uid: uid, weight: weight})
		totalWeight += weight
	}

	if totalWeight == 0 {
		totalWeight = 1
	}

	var totalDistributed int64
	for _, ep := range players {
		share := result.LootValue * int64(ep.weight) / int64(totalWeight)
		if share < 1 {
			share = 1
		}
		net, _ := communityTax(ep.uid, float64(share), 0.05)
		p.euro.Credit(ep.uid, net, "twinbee_daily_share")
		totalDistributed += share
		if rollTwinBeeGift(ep.uid) {
			summary.GiftCount++
		}
	}
	// Report average share for the summary display.
	summary.GoldShare = totalDistributed / int64(len(eligible))

	p.logTwinBeeResult(result, summary)
	return summary
}

// ── TwinBee Gifts (Temporary Buffs) ──────────────────────────────────────────

type twinBeeGiftDef struct {
	BuffType  string
	BuffName  string
	Modifier  float64
	Duration  time.Duration
	Flavor    string
}

var twinBeeGifts = []twinBeeGiftDef{
	{"success_chance", "TwinBee's Lucky Star ⭐", 10, 24 * time.Hour, "TwinBee sends a star. They have many stars. Use it well."},
	{"death_chance", "Bec's Blessing 🐝", -5, 48 * time.Hour, "Bec has blessed you. Bec does not do this lightly. Don't die."},
	{"loot_quality", "WinBee's Coin 🪙", 15, 24 * time.Hour, "WinBee flipped this coin and it came up you. Lucky."},
	{"mining_success", "Goemon's Pipe 🎋", 8, 48 * time.Hour, "Borrowed from Goemon. Return not expected. Results expected."},
	{"foraging_death", "Pentarou's Feather 🪶", -10, 24 * time.Hour, "Pentarou parted with this reluctantly. They like you enough. Mostly."},
	{"xp_multiplier", "TwinBee's Bell Fragment 🔔", 5, 48 * time.Hour, "A piece of the Bell. It rings when you're doing well. It will ring."},
	{"exceptional_chance", "Power Up Pod 🫛", 50, 48 * time.Hour, "TwinBee found extras. This is not a common occurrence. Don't waste it."},
}

const twinBeeGiftChance = 0.15

func rollTwinBeeGift(userID id.UserID) bool {
	if rand.Float64() >= twinBeeGiftChance {
		return false
	}

	gift := twinBeeGifts[rand.IntN(len(twinBeeGifts))]
	expiresAt := time.Now().UTC().Add(gift.Duration)

	if err := addAdvBuff(userID, gift.BuffType, gift.BuffName, gift.Modifier, expiresAt); err != nil {
		slog.Error("adventure: failed to add twinbee gift", "user", userID, "err", err)
		return false
	}

	return true
}

// ── TwinBee Log ──────────────────────────────────────────────────────────────

func (p *AdventurePlugin) logTwinBeeResult(result *TwinBeeResult, summary TwinBeeRewardSummary) {
	db.Exec("adventure: log twinbee result",
		`INSERT INTO adventure_twinbee_log (activity_type, location, outcome, loot_value, loot_desc, participant_count, gold_share, gift_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(result.Activity), result.Location.Name, string(result.Outcome),
		result.LootValue, result.LootDesc,
		summary.Eligible, summary.GoldShare, summary.GiftCount)
}
