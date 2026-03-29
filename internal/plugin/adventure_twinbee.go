package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── TwinBee Character (fixed stats) ──────────────────────────────────────────

var twinBeeChar = AdventureCharacter{
	DisplayName: "TwinBee 🐝",
	CombatLevel: 35,
	MiningSkill: 28,
	ForagingSkill: 22,
	Alive:       true,
}

var twinBeeEquip = map[EquipmentSlot]*AdvEquipment{
	SlotWeapon: {Slot: SlotWeapon, Tier: 4, Condition: 100, Name: "The Spread Gun"},
	SlotArmor:  {Slot: SlotArmor, Tier: 4, Condition: 100, Name: "Enchanted Plate"},
	SlotHelmet: {Slot: SlotHelmet, Tier: 4, Condition: 100, Name: "Guardian's Helm"},
	SlotBoots:  {Slot: SlotBoots, Tier: 4, Condition: 100, Name: "Ranger's Boots"},
	SlotTool:   {Slot: SlotTool, Tier: 4, Condition: 100, Name: "Mithril Pickaxe"},
}

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
		combined := c.CombatLevel + c.MiningSkill + c.ForagingSkill
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

func (p *AdventurePlugin) runTwinBeeDaily() *TwinBeeResult {
	activity, loc := selectTwinBeeAction()
	if loc == nil {
		slog.Error("adventure: twinbee could not find location")
		return nil
	}

	// Copy equipment so mutations don't accumulate on the global template.
	equip := make(map[EquipmentSlot]*AdvEquipment, len(twinBeeEquip))
	for k, v := range twinBeeEquip {
		copy := *v
		equip[k] = &copy
	}

	bonuses := &AdvBonusSummary{} // TwinBee has no treasures/buffs
	result := resolveAdvAction(&twinBeeChar, equip, loc, bonuses, false)

	// TwinBee never dies — reroll death to empty
	if result.Outcome == AdvOutcomeDeath {
		result.Outcome = AdvOutcomeEmpty
		result.LootItems = nil
		result.TotalLootValue = 0
		result.EquipDamage = nil
		result.EquipBroken = nil
	}

	// No treasure drops for TwinBee
	result.TreasureFound = nil

	// Select TwinBee-specific flavor text
	tbResult := &TwinBeeResult{
		Activity:  activity,
		Location:  loc,
		Outcome:   result.Outcome,
		LootValue: result.TotalLootValue,
	}

	// Build loot description
	if len(result.LootItems) > 0 {
		names := make([]string, len(result.LootItems))
		for i, item := range result.LootItems {
			names[i] = item.Name
		}
		tbResult.LootDesc = joinAdvItems(names)
	}

	// Select flavor
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
		if c.ActionTakenToday {
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
		weight := 3 // minimum (level 1 in all 3 skills)
		for _, c := range chars {
			if c.UserID == uid {
				weight = c.CombatLevel + c.MiningSkill + c.ForagingSkill
				if weight < 3 {
					weight = 3
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
		p.euro.Credit(ep.uid, float64(share), "twinbee_daily_share")
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
