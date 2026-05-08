package plugin

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

const advLocationCooldownDuration = 3 * time.Hour

// ── Activity & Outcome Types ─────────────────────────────────────────────────

type AdvActivityType string

const (
	AdvActivityDungeon  AdvActivityType = "dungeon"
	AdvActivityMining   AdvActivityType = "mining"
	AdvActivityForaging AdvActivityType = "foraging"
	AdvActivityFishing  AdvActivityType = "fishing"
	AdvActivityRest     AdvActivityType = "rest"
	AdvActivityShop     AdvActivityType = "shop"
)

type AdvOutcomeType string

const (
	AdvOutcomeDeath       AdvOutcomeType = "death"
	AdvOutcomeEmpty       AdvOutcomeType = "empty"
	AdvOutcomeSuccess     AdvOutcomeType = "success"
	AdvOutcomeExceptional AdvOutcomeType = "exceptional"
	AdvOutcomeCaveIn      AdvOutcomeType = "cave_in"
	AdvOutcomeHornets     AdvOutcomeType = "hornets"
	AdvOutcomeBear        AdvOutcomeType = "bear"
	AdvOutcomeRiver       AdvOutcomeType = "river"
)

// ── Location Definitions ─────────────────────────────────────────────────────

type AdvLocation struct {
	Name         string
	Activity     AdvActivityType
	Tier         int
	Denizens     string // monsters/resources description
	BaseDeathPct float64
	EmptyPct     float64
	MinLevel     int
	MinEquipTier int
}

type AdvLootDef struct {
	Name     string
	Type     string
	MinValue int64
	MaxValue int64
}

var advDungeons = []AdvLocation{
	{"The Soggy Cellar", AdvActivityDungeon, 1, "Giant Rats, Angry Badgers, Wet Slimes", 8, 15, 1, 0},
	{"Goblin Warrens", AdvActivityDungeon, 2, "Goblins, Kobolds, Trap Spiders", 18, 15, 8, 1},
	{"The Cursed Crypt", AdvActivityDungeon, 3, "Skeletons, Ghosts, Draugr", 30, 15, 20, 2},
	{"Troll Bridge Depths", AdvActivityDungeon, 4, "Trolls, Stone Giants, Cursed Knights", 45, 15, 35, 3},
	{"The Abyssal Maw", AdvActivityDungeon, 5, "Demons, Elder Drakes, The Unnamed", 60, 15, 48, 4},
}

var advMines = []AdvLocation{
	{"Surface Pits", AdvActivityMining, 1, "Copper, Tin, Coal", 3, 20, 1, 0},
	{"Iron Ridge", AdvActivityMining, 2, "Iron, Lead, Saltpetre", 8, 20, 8, 1},
	{"Silver Seam", AdvActivityMining, 3, "Silver, Quartz, Nickel", 15, 20, 18, 2},
	{"The Deeprock", AdvActivityMining, 4, "Gold, Sapphire, Titanium", 25, 20, 30, 3},
	{"Mythril Caverns", AdvActivityMining, 5, "Mythril, Dragon Crystal, Voidstone", 35, 20, 44, 4},
}

var advForests = []AdvLocation{
	{"The Meadow", AdvActivityForaging, 1, "Berries, Twigs, Common Herbs", 1, 10, 1, 0},
	{"Old Forest", AdvActivityForaging, 2, "Hardwood, Wild Fruit, Mushrooms", 3, 10, 8, 1},
	{"Ancient Grove", AdvActivityForaging, 3, "Ancient Timber, Rare Herbs, Honey", 7, 10, 16, 2},
	{"The Deep Jungle", AdvActivityForaging, 4, "Exotic Wood, Tropical Fruits, Spores", 12, 10, 28, 3},
	{"Primal Wilds", AdvActivityForaging, 5, "Primordial Bark, Spirit Herbs, Starfruit", 20, 10, 40, 4},
}

var advFishingSpots = []AdvLocation{
	{"Muddy Pond", AdvActivityFishing, 1, "Sad Fish, Boots, Tin Cans", 5, 20, 1, 0},
	{"Iron Creek", AdvActivityFishing, 2, "Creek Fish, Cheep Cheeps, Wet Rocks", 12, 18, 8, 1},
	{"Silver Lake", AdvActivityFishing, 3, "Lake Fish, Bloopers, Legends", 22, 16, 18, 2},
	{"The Deep Current", AdvActivityFishing, 4, "River Monsters, Sea Serpents, Whirlpools", 35, 14, 30, 3},
	{"Abyssal Trench", AdvActivityFishing, 5, "Ancient Things, The Unnamed, The Deep", 50, 12, 44, 4},
}

// allAdvLocations returns all locations for a given activity type.
func allAdvLocations(activity AdvActivityType) []AdvLocation {
	switch activity {
	case AdvActivityDungeon:
		return advDungeons
	case AdvActivityMining:
		return advMines
	case AdvActivityForaging:
		return advForests
	case AdvActivityFishing:
		return advFishingSpots
	}
	return nil
}

// findAdvLocation finds a location by name (case-insensitive substring match).
func findAdvLocation(activity AdvActivityType, name string) *AdvLocation {
	locs := allAdvLocations(activity)
	for i := range locs {
		if containsFold(locs[i].Name, name) {
			return &locs[i]
		}
	}
	return nil
}

// findAdvLocationByTier returns the location for a given activity and tier.
func findAdvLocationByTier(activity AdvActivityType, tier int) *AdvLocation {
	locs := allAdvLocations(activity)
	for i := range locs {
		if locs[i].Tier == tier {
			return &locs[i]
		}
	}
	return nil
}

// ── Loot Tables ──────────────────────────────────────────────────────────────

var advDungeonLoot = map[int][]AdvLootDef{
	1: {{"Copper Coins", "treasure", 5, 12}, {"Rat Pelt", "treasure", 8, 18}, {"Mouldy Bread", "treasure", 3, 8}, {"Bent Nail", "treasure", 2, 6}},
	2: {{"Iron Scraps", "ore", 20, 40}, {"Goblin Trinket", "treasure", 25, 50}, {"Small Gem", "gem", 40, 80}},
	3: {{"Silver Bar", "ore", 100, 200}, {"Ancient Artifact", "treasure", 150, 300}, {"Quality Gem", "gem", 200, 400}},
	4: {{"Gold Ingot", "ore", 500, 1000}, {"Enchanted Fragment", "treasure", 800, 1500}, {"Rare Gem", "gem", 1000, 2000}},
	5: {{"Legendary Fragment", "treasure", 2000, 5000}, {"Dragon Scale", "treasure", 3000, 8000}, {"Mythic Treasure", "treasure", 5000, 15000}},
}

var advMiningLoot = map[int][]AdvLootDef{
	1: {{"Copper Ore", "ore", 5, 12}, {"Tin Ore", "ore", 6, 14}, {"Coal", "ore", 4, 10}},
	2: {{"Iron Ore", "ore", 15, 25}, {"Lead Ore", "ore", 18, 30}, {"Saltpetre", "ore", 20, 40}},
	3: {{"Silver Ore", "ore", 60, 100}, {"Quartz", "ore", 80, 120}, {"Nickel Ore", "ore", 70, 110}},
	4: {{"Gold Ore", "ore", 200, 400}, {"Sapphire", "gem", 300, 500}, {"Titanium Ore", "ore", 250, 450}},
	5: {{"Mythril Ore", "ore", 1000, 2500}, {"Dragon Crystal", "gem", 2000, 4000}, {"Voidstone", "ore", 1500, 3500}},
}

var advForagingLoot = map[int][]AdvLootDef{
	1: {{"Berries", "fruit", 3, 10}, {"Twigs", "wood", 5, 12}, {"Common Herbs", "fruit", 6, 15}},
	2: {{"Hardwood", "wood", 10, 20}, {"Wild Fruit", "fruit", 12, 22}, {"Mushrooms", "fruit", 15, 30}},
	3: {{"Ancient Timber", "wood", 40, 80}, {"Rare Herbs", "fruit", 50, 100}, {"Honey", "fruit", 60, 120}},
	4: {{"Exotic Wood", "wood", 150, 300}, {"Tropical Fruits", "fruit", 180, 400}, {"Spores", "fruit", 200, 500}},
	5: {{"Primordial Bark", "wood", 600, 1500}, {"Spirit Herbs", "fruit", 800, 2000}, {"Starfruit", "fruit", 1000, 3000}},
}

var advFishingLoot = map[int][]AdvLootDef{
	1: {{"Sad Fish", "fish", 4, 10}, {"Old Boot", "junk", 5, 12}, {"Tin Can", "junk", 3, 8}},
	2: {{"Creek Trout", "fish", 12, 22}, {"Iron Scale", "fish", 15, 28}, {"River Pearl", "gem", 20, 40}},
	3: {{"Silver Bass", "fish", 50, 90}, {"Lake Sturgeon", "fish", 60, 110}, {"Blooper Ink", "treasure", 80, 150}},
	4: {{"Deep Eel", "fish", 180, 350}, {"River Serpent Scale", "treasure", 250, 500}, {"Abyssal Pearl", "gem", 300, 600}},
	5: {{"Ancient Leviathan Tooth", "treasure", 800, 2000}, {"Trench Horror", "fish", 1200, 3000}, {"Void Pearl", "gem", 1500, 4000}},
}

func advLootTable(activity AdvActivityType) map[int][]AdvLootDef {
	switch activity {
	case AdvActivityDungeon:
		return advDungeonLoot
	case AdvActivityMining:
		return advMiningLoot
	case AdvActivityForaging:
		return advForagingLoot
	case AdvActivityFishing:
		return advFishingLoot
	}
	return nil
}

// ── XP Tables ────────────────────────────────────────────────────────────────

type advXPEntry struct {
	Success     int
	Failure     int
	Death       int
	Exceptional int
}

var advXPTable = map[AdvActivityType]map[int]advXPEntry{
	AdvActivityDungeon: {
		1: {60, 20, 10, 90},
		2: {100, 30, 15, 150},
		3: {160, 45, 20, 240},
		4: {230, 60, 25, 345},
		5: {320, 80, 30, 480},
	},
	AdvActivityMining: {
		1: {50, 18, 10, 75},
		2: {85, 28, 15, 128},
		3: {135, 40, 18, 203},
		4: {200, 55, 22, 300},
		5: {280, 70, 28, 420},
	},
	AdvActivityForaging: {
		1: {40, 15, 8, 60},
		2: {70, 22, 12, 105},
		3: {110, 35, 15, 165},
		4: {165, 48, 18, 248},
		5: {230, 62, 22, 345},
	},
	AdvActivityFishing: {
		1: {45, 16, 8, 68},
		2: {78, 25, 12, 117},
		3: {120, 38, 16, 180},
		4: {180, 52, 20, 270},
		5: {250, 68, 25, 375},
	},
}

func advXPForOutcome(activity AdvActivityType, tier int, outcome AdvOutcomeType) int {
	table, ok := advXPTable[activity]
	if !ok {
		return 0
	}
	entry, ok := table[tier]
	if !ok {
		return 0
	}
	switch outcome {
	case AdvOutcomeDeath:
		return entry.Death
	case AdvOutcomeEmpty, AdvOutcomeHornets:
		return entry.Failure
	case AdvOutcomeSuccess, AdvOutcomeCaveIn, AdvOutcomeBear, AdvOutcomeRiver:
		return entry.Failure // partial successes get failure XP
	case AdvOutcomeExceptional:
		return entry.Exceptional
	}
	return entry.Success
}

// advXPSkill returns which skill receives XP for an activity.
func advXPSkill(activity AdvActivityType) string {
	switch activity {
	case AdvActivityDungeon:
		return "combat"
	case AdvActivityMining:
		return "mining"
	case AdvActivityForaging:
		return "foraging"
	case AdvActivityFishing:
		return "fishing"
	}
	return ""
}

// ── Bonus Summary ────────────────────────────────────────────────────────────

type AdvBonusSummary struct {
	CombatBonus      int
	MiningBonus      int
	ForagingBonus    int
	FishingBonus     int
	DeathModifier    float64 // negative = less death
	LootQuality      float64 // percentage modifier
	XPMultiplier     float64 // percentage modifier
	ExceptionalBonus float64 // percentage modifier
	SuccessBonus     float64 // percentage modifier
}

func computeAdvBonuses(treasures []AdvTreasureBonus, buffs []AdvBuff, streak int, hasGrudge bool) *AdvBonusSummary {
	b := &AdvBonusSummary{}

	// Treasure bonuses
	for _, t := range treasures {
		switch t.BonusType {
		case "combat_level":
			b.CombatBonus += int(t.BonusValue)
		case "mining_skill":
			b.MiningBonus += int(t.BonusValue)
		case "foraging_skill":
			b.ForagingBonus += int(t.BonusValue)
		case "fishing_skill":
			b.FishingBonus += int(t.BonusValue)
		case "all_skills":
			b.CombatBonus += int(t.BonusValue)
			b.MiningBonus += int(t.BonusValue)
			b.ForagingBonus += int(t.BonusValue)
			b.FishingBonus += int(t.BonusValue)
		case "death_chance":
			b.DeathModifier += t.BonusValue
		case "loot_quality":
			b.LootQuality += t.BonusValue
		case "xp_multiplier":
			b.XPMultiplier += t.BonusValue
		case "exceptional_chance":
			b.ExceptionalBonus += t.BonusValue
		case "success_chance":
			b.SuccessBonus += t.BonusValue
		}
	}

	// Buff bonuses
	for _, buf := range buffs {
		switch buf.BuffType {
		case "success_chance":
			b.SuccessBonus += buf.Modifier
		case "death_chance":
			b.DeathModifier += buf.Modifier
		case "loot_quality":
			b.LootQuality += buf.Modifier
		case "xp_multiplier":
			b.XPMultiplier += buf.Modifier
		case "exceptional_chance":
			b.ExceptionalBonus += buf.Modifier
		case "mining_success":
			b.MiningBonus += int(buf.Modifier)
		case "foraging_death":
			b.DeathModifier += buf.Modifier
		}
	}

	// Streak bonuses
	switch {
	case streak >= 30:
		b.XPMultiplier += 20
		b.LootQuality += 15
		b.DeathModifier -= 5
	case streak >= 14:
		b.XPMultiplier += 15
		b.LootQuality += 10
		b.DeathModifier -= 3
	case streak >= 7:
		b.XPMultiplier += 10
		b.LootQuality += 5
	case streak >= 3:
		b.XPMultiplier += 5
	}

	// Grudge bonus
	if hasGrudge {
		b.SuccessBonus += 10
		b.XPMultiplier += 25
	}

	return b
}

// ── Location Cooldown ───────────────────────────────────────────────────────

// advLocationCooldown returns how long until a player can run the same location
// again. Returns 0 if no cooldown is active. Only successful runs trigger cooldown.
func advLocationCooldown(userID id.UserID, location string) time.Duration {
	d := db.Get()
	var loggedAt string
	err := d.QueryRow(
		`SELECT logged_at FROM adventure_activity_log
		 WHERE user_id = ? AND location = ? AND outcome IN ('success', 'exceptional')
		 ORDER BY logged_at DESC LIMIT 1`,
		string(userID), location,
	).Scan(&loggedAt)
	if err != nil {
		return 0
	}
	t, err := time.Parse("2006-01-02 15:04:05", loggedAt)
	if err != nil {
		return 0
	}
	remaining := advLocationCooldownDuration - time.Since(t)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

// ── Eligibility ──────────────────────────────────────────────────────────────

// advIsEligible checks if a character can enter a location.
// Returns (eligible, inPenaltyZone).
func advIsEligible(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, loc *AdvLocation, bonuses *AdvBonusSummary) (bool, bool) {
	// Tier gating uses base skill only — buffs improve success chances, not access.
	baseLevel := advBaseSkill(char, loc.Activity)
	if baseLevel < loc.MinLevel {
		return false, false
	}

	// Check minimum equipment tier — no equipment means tier 0
	minTier := 0
	if len(equip) > 0 {
		minTier = 99
		for _, eq := range equip {
			if eq.Tier < minTier {
				minTier = eq.Tier
			}
		}
	}
	if minTier < loc.MinEquipTier {
		return false, false
	}

	// Penalty zone: within 3 levels of minimum (base skill only)
	penalty := baseLevel-loc.MinLevel < 3
	return true, penalty
}

func advBaseSkill(char *AdventureCharacter, activity AdvActivityType) int {
	switch activity {
	case AdvActivityDungeon:
		return char.CombatLevel
	case AdvActivityMining:
		return char.MiningSkill
	case AdvActivityForaging:
		return char.ForagingSkill
	case AdvActivityFishing:
		return char.FishingSkill
	}
	return 1
}

func advEffectiveSkill(char *AdventureCharacter, activity AdvActivityType, bonuses *AdvBonusSummary) int {
	switch activity {
	case AdvActivityDungeon:
		return char.CombatLevel + bonuses.CombatBonus
	case AdvActivityMining:
		return char.MiningSkill + bonuses.MiningBonus
	case AdvActivityForaging:
		return char.ForagingSkill + bonuses.ForagingBonus
	case AdvActivityFishing:
		return char.FishingSkill + bonuses.FishingBonus
	}
	return 1
}

// ── Probability Calculation ──────────────────────────────────────────────────

type advProbabilities struct {
	DeathPct       float64
	EmptyPct       float64
	SuccessPct     float64
	ExceptionalPct float64
}

func calculateAdvProbabilities(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, loc *AdvLocation, bonuses *AdvBonusSummary, inPenaltyZone bool) advProbabilities {
	eqScore := advEquipmentScore(equip)
	skillLevel := float64(advEffectiveSkill(char, loc.Activity, bonuses))

	deathPct := loc.BaseDeathPct - (eqScore * 0.8) - (skillLevel * 0.5) + bonuses.DeathModifier
	if inPenaltyZone {
		deathPct += 5
	}

	// Clamp death
	deathPct = math.Max(1, math.Min(85, deathPct))

	emptyPct := loc.EmptyPct

	// Success modifiers
	baseSuccess := 100 - deathPct - emptyPct
	successMod := (eqScore * 1.2) + (skillLevel * 0.8) + bonuses.SuccessBonus

	// Masterwork gear: +5% skill-specific success bonus (mining sword → mining, etc.)
	if advMasterworkSkillBonus(equip, loc.Activity) {
		successMod += 5
	}

	// Bloodied set: Survivor's Instinct — +3% to all activity success rates
	arenaSets := advEquippedArenaSets(equip)
	if arenaSets["bloodied"] {
		successMod += 3
	}
	if inPenaltyZone {
		successMod -= 15
	}

	// Exceptional is 10% base, modified by bonuses
	exceptionalPct := 10.0 + bonuses.ExceptionalBonus
	exceptionalPct = math.Max(2, math.Min(25, exceptionalPct))

	successPct := baseSuccess + successMod - exceptionalPct
	successPct = math.Max(5, math.Min(90-exceptionalPct, successPct))

	// Normalize if over 100
	total := deathPct + emptyPct + successPct + exceptionalPct
	if total > 100 {
		scale := 100 / total
		deathPct *= scale
		emptyPct *= scale
		successPct *= scale
		exceptionalPct *= scale
	} else if total < 100 {
		// Give remaining to success
		successPct += 100 - total
	}

	return advProbabilities{
		DeathPct:       deathPct,
		EmptyPct:       emptyPct,
		SuccessPct:     successPct,
		ExceptionalPct: exceptionalPct,
	}
}

// ── Loot Generation ──────────────────────────────────────────────────────────

func generateAdvLoot(loc *AdvLocation, exceptional bool, lootQualityMod float64) []AdvItem {
	table := advLootTable(loc.Activity)
	if table == nil {
		return nil
	}
	defs, ok := table[loc.Tier]
	if !ok || len(defs) == 0 {
		return nil
	}

	// Number of items: 1-2 for normal, 2-3 for exceptional
	count := 1 + rand.IntN(2)
	if exceptional {
		count = 2 + rand.IntN(2)
	}

	var items []AdvItem
	for i := 0; i < count; i++ {
		def := defs[rand.IntN(len(defs))]
		value := def.MinValue + rand.Int64N(def.MaxValue-def.MinValue+1)

		// Apply loot quality modifier
		if lootQualityMod != 0 {
			value = int64(float64(value) * (1 + lootQualityMod/100))
		}

		// Exceptional items are worth more
		if exceptional {
			value = int64(float64(value) * 1.5)
		}

		items = append(items, AdvItem{
			Name:  def.Name,
			Type:  def.Type,
			Tier:  loc.Tier,
			Value: value,
		})
	}
	return items
}

// ── Equipment Degradation ────────────────────────────────────────────────────

func applyAdvEquipDegradation(equip map[EquipmentSlot]*AdvEquipment, outcome AdvOutcomeType) map[EquipmentSlot]int {
	damage := make(map[EquipmentSlot]int)

	switch outcome {
	case AdvOutcomeDeath:
		for _, slot := range allSlots {
			damage[slot] = 20
		}
		damage[SlotWeapon] = 30
		damage[SlotArmor] = 30

	case AdvOutcomeCaveIn:
		damage[SlotTool] = 25
		damage[SlotArmor] = 10

	case AdvOutcomeEmpty:
		damage[SlotWeapon] = 15
		damage[SlotArmor] = 10

	case AdvOutcomeBear:
		damage[SlotArmor] = 20
		damage[SlotBoots] = 15

	case AdvOutcomeRiver:
		damage[SlotBoots] = 20

	case AdvOutcomeHornets:
	}

	return applyDegradationModifiers(damage, equip)
}

// advCheckBrokenEquipment checks which slots hit 0 condition and reverts them to tier 0.
func advCheckBrokenEquipment(equip map[EquipmentSlot]*AdvEquipment) []EquipmentSlot {
	var broken []EquipmentSlot
	for _, slot := range allSlots {
		eq, ok := equip[slot]
		if !ok || eq.Condition > 0 {
			continue
		}
		// Revert to tier 0
		def := equipmentTiers[slot][0]
		eq.Tier = 0
		eq.Condition = 100
		eq.Name = def.Name
		eq.ActionsUsed = 0
		eq.ArenaTier = 0
		eq.ArenaSet = ""
		eq.Masterwork = false
		eq.SkillSource = ""
		broken = append(broken, slot)
	}
	return broken
}

// ── Overlevel Penalty ───────────────────────────────────────────────────────

// advOverlevelMultiplier returns a multiplier (0.05–1.0) that reduces XP and
// loot when a character's effective level far exceeds the location's minimum.
// Gap 0-3: no penalty. Gap 4+: −15% per level over 3, floor 5%.
// No penalty if no higher-tier location of the same activity is accessible.
func advOverlevelMultiplier(effectiveLevel int, loc *AdvLocation) float64 {
	gap := effectiveLevel - loc.MinLevel
	if gap <= 3 {
		return 1.0
	}
	hasHigherAccessible := false
	for _, other := range allAdvLocations(loc.Activity) {
		if other.MinLevel > loc.MinLevel && other.MinLevel <= effectiveLevel {
			hasHigherAccessible = true
			break
		}
	}
	if !hasHigherAccessible {
		return 1.0
	}
	mult := 1.0 - 0.15*float64(gap-3)
	return math.Max(0.05, mult)
}

// ── Outcome Resolution ───────────────────────────────────────────────────────

type AdvActionResult struct {
	Outcome        AdvOutcomeType
	Location       *AdvLocation
	LootItems      []AdvItem
	TotalLootValue int64
	XPGained       int
	XPBreakdown    string // human-readable bonus breakdown
	XPSkill        string
	EquipDamage    map[EquipmentSlot]int
	LeveledUp      bool
	NewLevel       int
	TreasureFound  *AdvTreasureDrop
	FlavorText     string
	FlavorKey      string
	EquipBroken    []EquipmentSlot
	NearDeath      bool
	StreakBonus     int
	CombatLog      *CombatResult
	// MasteryCrossings records equipment slots whose ActionsUsed crossed a
	// mastery threshold (advMasteryThresholds) on this action. The caller
	// uses this to DM a celebration so the silent counter becomes visible.
	MasteryCrossings []AdvMasteryCrossing
}

type AdvMasteryCrossing struct {
	Slot      EquipmentSlot
	ItemName  string
	Threshold int // 50, 100, or 250
}

// advMasteryThresholds defines the action counts at which equipment mastery
// is celebrated. Picked to span an early-game milestone (50), a mid-game
// commitment (100), and a hard-to-reach veteran tier (250).
var advMasteryThresholds = []int{50, 100, 250}

// advMasteryTier reports how many mastery thresholds a piece has crossed
// (0–3) and the next threshold. nextThreshold is 0 when the piece has
// already crossed every threshold.
func advMasteryTier(actionsUsed int) (tier, nextThreshold int) {
	for _, t := range advMasteryThresholds {
		if actionsUsed >= t {
			tier++
		} else {
			nextThreshold = t
			break
		}
	}
	return tier, nextThreshold
}

// advMasteryMarker returns the visual tier marker for a piece of equipment.
// Empty for sub-50; ✦/✦✦/✦✦✦ at the 50/100/250 thresholds.
func advMasteryMarker(actionsUsed int) string {
	tier, _ := advMasteryTier(actionsUsed)
	switch tier {
	case 1:
		return "✦"
	case 2:
		return "✦✦"
	case 3:
		return "✦✦✦"
	}
	return ""
}

// advSlotRelevantToActivity reports whether using a slot during the given
// activity should count toward that slot's mastery. Combat actions exercise
// the combat loadout (weapon/armor/helmet/boots); harvest actions exercise
// only the tool. This is what makes mastery feel like "you used this
// piece" rather than "you played for a while" — without this gate every
// slot ticks in lockstep and the per-slot view is meaningless.
func advSlotRelevantToActivity(slot EquipmentSlot, activity AdvActivityType) bool {
	if isCombatActivity(activity) {
		return slot == SlotWeapon || slot == SlotArmor || slot == SlotHelmet || slot == SlotBoots
	}
	if isHarvestActivity(activity) {
		return slot == SlotTool
	}
	return false
}

// advMasteryRowSegment formats the `23/50`, `73/100 ✦`, `142/250 ✦✦`, or
// `250 ✦✦✦` segment shown inline on each equipment row in the character
// sheet so progress is visible at a glance — not just a tier marker that
// flips on at 50/100/250 with nothing in between.
func advMasteryRowSegment(actionsUsed int) string {
	if actionsUsed <= 0 {
		return ""
	}
	_, next := advMasteryTier(actionsUsed)
	marker := advMasteryMarker(actionsUsed)
	if next == 0 {
		// At max — there's no "next" threshold to count toward.
		return fmt.Sprintf("%d %s", actionsUsed, marker)
	}
	if marker == "" {
		return fmt.Sprintf("%d/%d", actionsUsed, next)
	}
	return fmt.Sprintf("%d/%d %s", actionsUsed, next, marker)
}

// AdvMasteryRollup summarizes mastery state across all equipped slots so the
// character sheet can show one informative aggregate line without a
// per-slot block. AnyProgress is true when at least one slot has any
// ActionsUsed — used to decide whether to show a "building up" hint when
// no thresholds have crossed yet.
type AdvMasteryRollup struct {
	ThresholdsCrossed int     // raw count, pre-cap (sum across all slots)
	MaxedSlots        int     // slots with all 3 thresholds crossed
	Bonus             float64 // capped %, mirrors advEquipmentMasteryBonus
	AnyProgress       bool
}

func advMasteryRollup(equip map[EquipmentSlot]*AdvEquipment) AdvMasteryRollup {
	r := AdvMasteryRollup{}
	for _, eq := range equip {
		if eq == nil {
			continue
		}
		if eq.ActionsUsed > 0 {
			r.AnyProgress = true
		}
		tier, _ := advMasteryTier(eq.ActionsUsed)
		r.ThresholdsCrossed += tier
		if tier == len(advMasteryThresholds) {
			r.MaxedSlots++
		}
	}
	r.Bonus = advEquipmentMasteryBonus(equip)
	return r
}

// advEquipmentMasteryBonus returns the loot-quality bonus from accumulated
// mastery across all equipped slots. Each crossed threshold grants +1% loot
// quality on its slot, capped at +10% total to keep balance reasonable.
func advEquipmentMasteryBonus(equip map[EquipmentSlot]*AdvEquipment) float64 {
	total := 0.0
	for _, eq := range equip {
		if eq == nil {
			continue
		}
		for _, t := range advMasteryThresholds {
			if eq.ActionsUsed >= t {
				total++
			}
		}
	}
	if total > 10 {
		total = 10
	}
	return total
}

func resolveAdvAction(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, loc *AdvLocation, bonuses *AdvBonusSummary, inPenaltyZone bool) *AdvActionResult {
	result := &AdvActionResult{
		Location: loc,
		XPSkill:  advXPSkill(loc.Activity),
	}

	// Apply equipment-mastery loot bonus on a local copy so we don't mutate
	// the caller's bonuses struct (it's reused across multiple actions).
	localBonuses := *bonuses
	localBonuses.LootQuality += advEquipmentMasteryBonus(equip)
	bonuses = &localBonuses

	probs := calculateAdvProbabilities(char, equip, loc, bonuses, inPenaltyZone)

	// Overlevel penalty — reduces loot and XP for farming low-tier content
	skillLevel := advEffectiveSkill(char, loc.Activity, bonuses)
	overlevelMult := advOverlevelMultiplier(skillLevel, loc)

	// Roll outcome
	roll := rand.Float64() * 100

	switch {
	case roll < probs.DeathPct:
		result.Outcome = AdvOutcomeDeath
	case roll < probs.DeathPct+probs.EmptyPct:
		// Activity-specific empty outcomes
		result.Outcome = resolveAdvEmptyOutcome(loc, roll)
	case roll < probs.DeathPct+probs.EmptyPct+probs.SuccessPct:
		result.Outcome = AdvOutcomeSuccess
	default:
		result.Outcome = AdvOutcomeExceptional
	}

	// Near-death check: survived within 2% of death threshold
	if result.Outcome != AdvOutcomeDeath && roll < probs.DeathPct+2 && roll >= probs.DeathPct {
		result.NearDeath = true
	}

	// Generate loot for success/exceptional
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		result.LootItems = generateAdvLoot(loc, result.Outcome == AdvOutcomeExceptional, bonuses.LootQuality)
		// Apply overlevel penalty to loot values
		if overlevelMult < 1.0 {
			for i := range result.LootItems {
				result.LootItems[i].Value = max(1, int64(float64(result.LootItems[i].Value)*overlevelMult))
			}
		}
		for _, item := range result.LootItems {
			result.TotalLootValue += item.Value
		}
	}

	// XP calculation
	xp := advXPForOutcome(loc.Activity, loc.Tier, result.Outcome)
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		xp = advXPTable[loc.Activity][loc.Tier].Success
		if result.Outcome == AdvOutcomeExceptional {
			xp = advXPTable[loc.Activity][loc.Tier].Exceptional
		}
	}

	xpResult := applyXPBonuses(XPBonusParams{
		BaseXP:        xp,
		NearDeath:     result.NearDeath,
		BonusMult:     bonuses.XPMultiplier,
		Ironclad:      advEquippedArenaSets(equip)["ironclad"],
		OverlevelMult: overlevelMult,
	})
	result.XPGained = xpResult.Total
	result.XPBreakdown = xpResult.Breakdown

	// Equipment degradation on bad outcomes
	if result.Outcome == AdvOutcomeDeath || result.Outcome == AdvOutcomeEmpty ||
		result.Outcome == AdvOutcomeCaveIn || result.Outcome == AdvOutcomeBear ||
		result.Outcome == AdvOutcomeRiver {
		result.EquipDamage = applyAdvEquipDegradation(equip, result.Outcome)
		result.EquipBroken = advCheckBrokenEquipment(equip)
	}

	// Increment actions_used for equipment mastery only on slots relevant
	// to the activity — a foraging trip shouldn't master a sword, and a
	// dungeon run shouldn't master a pickaxe. Detect threshold crossings
	// so the caller can DM a celebration.
	for slot, eq := range equip {
		if eq == nil {
			continue
		}
		if !advSlotRelevantToActivity(slot, loc.Activity) {
			continue
		}
		before := eq.ActionsUsed
		eq.ActionsUsed++
		for _, t := range advMasteryThresholds {
			if before < t && eq.ActionsUsed >= t {
				result.MasteryCrossings = append(result.MasteryCrossings, AdvMasteryCrossing{
					Slot:      slot,
					ItemName:  eq.Name,
					Threshold: t,
				})
			}
		}
	}

	return result
}

// resolveAdvEmptyOutcome returns an activity-specific "empty" outcome.
func resolveAdvEmptyOutcome(loc *AdvLocation, _ float64) AdvOutcomeType {
	switch loc.Activity {
	case AdvActivityMining:
		// 40% chance of cave-in on "empty" result
		if rand.Float64() < 0.4 {
			return AdvOutcomeCaveIn
		}
		return AdvOutcomeEmpty

	case AdvActivityForaging:
		// Split empty into specific outcomes
		r := rand.Float64()
		switch {
		case r < 0.35:
			return AdvOutcomeHornets
		case r < 0.55:
			return AdvOutcomeBear
		case r < 0.70:
			return AdvOutcomeRiver
		default:
			return AdvOutcomeEmpty
		}

	case AdvActivityFishing:
		// Fishing empty is just empty — no sub-outcomes
		return AdvOutcomeEmpty

	default:
		return AdvOutcomeEmpty
	}
}

// ── Eligible Locations for DM Menu ───────────────────────────────────────────

type AdvEligibleLocation struct {
	Location       *AdvLocation
	InPenaltyZone  bool
	DeathPct       float64
	ExceptionalPct float64
}

func advEligibleLocations(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, activity AdvActivityType, bonuses *AdvBonusSummary) []AdvEligibleLocation {
	var eligible []AdvEligibleLocation
	for _, loc := range allAdvLocations(activity) {
		loc := loc
		ok, penalty := advIsEligible(char, equip, &loc, bonuses)
		if !ok {
			continue
		}
		probs := calculateAdvProbabilities(char, equip, &loc, bonuses, penalty)
		eligible = append(eligible, AdvEligibleLocation{
			Location:       &loc,
			InPenaltyZone:  penalty,
			DeathPct:       probs.DeathPct,
			ExceptionalPct: probs.ExceptionalPct,
		})
	}
	return eligible
}

// ── Party Bonus Check ────────────────────────────────────────────────────────

// advCheckPartyBonus checks if other players visited the same location today.
func advCheckPartyBonus(userID id.UserID, location string) bool {
	logs, err := loadAdvLogsForDate(time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		return false
	}
	for _, l := range logs {
		if l.UserID != userID && l.Location == location {
			return true
		}
	}
	return false
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	sl := make([]byte, len(s))
	subl := make([]byte, len(substr))
	for i := range s {
		if s[i] >= 'A' && s[i] <= 'Z' {
			sl[i] = s[i] + 32
		} else {
			sl[i] = s[i]
		}
	}
	for i := range substr {
		if substr[i] >= 'A' && substr[i] <= 'Z' {
			subl[i] = substr[i] + 32
		} else {
			subl[i] = substr[i]
		}
	}
	return containsBytes(sl, subl)
}

func containsBytes(s, sub []byte) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
