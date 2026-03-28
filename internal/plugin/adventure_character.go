package plugin

import (
	"database/sql"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Equipment Slot Constants ─────────────────────────────────────────────────

type EquipmentSlot string

const (
	SlotWeapon EquipmentSlot = "weapon"
	SlotArmor  EquipmentSlot = "armor"
	SlotHelmet EquipmentSlot = "helmet"
	SlotBoots  EquipmentSlot = "boots"
	SlotTool   EquipmentSlot = "tool"
)

var allSlots = []EquipmentSlot{SlotWeapon, SlotArmor, SlotHelmet, SlotBoots, SlotTool}

// ── Core Types ───────────────────────────────────────────────────────────────

type AdventureCharacter struct {
	UserID           id.UserID
	DisplayName      string
	CombatLevel      int
	MiningSkill      int
	ForagingSkill    int
	FishingSkill     int // v2
	CombatXP         int
	MiningXP         int
	ForagingXP       int
	FishingXP        int // v2
	Alive            bool
	DeadUntil        *time.Time
	ActionTakenToday   bool
	HolidayActionTaken bool
	ArenaWins          int // v2
	ArenaLosses      int    // v2
	InvasionScore    int    // v2
	Title            string // v2
	CurrentStreak    int
	BestStreak       int
	LastActionDate   string
	GrudgeLocation   string
	CreatedAt        time.Time
	LastActiveAt     time.Time
}

type AdvEquipment struct {
	Slot       EquipmentSlot
	Tier       int
	Condition  int
	Name       string
	ActionsUsed int
}

type AdvItem struct {
	ID    int64
	Name  string
	Type  string // ore, wood, fruit, treasure, gem
	Tier  int
	Value int64
}

type AdvBuff struct {
	ID        int64
	UserID    id.UserID
	BuffType  string
	BuffName  string
	Modifier  float64
	ExpiresAt time.Time
}

// ── Equipment Tier Definitions ───────────────────────────────────────────────

type EquipmentDef struct {
	Name        string
	Tier        int
	Description string
	Price       float64
}

var equipmentTiers = map[EquipmentSlot][]EquipmentDef{
	SlotWeapon: {
		{Name: "Basic Ass Sword", Tier: 0, Description: "It's a sword in the same way a parking ticket is legal documentation. A profound object of shame.", Price: 0},
		{Name: "Sad Iron Sword", Tier: 1, Description: "It's iron. It holds an edge if you squint. An improvement over the last thing in the same way a bruise improves on a fracture.", Price: 100},
		{Name: "Dull Steel Sword of Mediocrity", Tier: 2, Description: "Steel, technically. Holds an edge longer than the last one, which isn't saying much. A sword for someone who has given up dreaming but not yet given up entirely.", Price: 450},
		{Name: "Sword (It's Fine)", Tier: 3, Description: "Fine. It's a fine sword. Not good. Not impressive. Nobody will write songs about it. It will not let you down in a straightforward fight.", Price: 1500},
		{Name: "Enchanted Blade", Tier: 4, Description: "Enchanted. Glows faintly, hums with something that feels like intent. For the first time in your miserable adventuring career, the weapon is not the problem.", Price: 7500},
		{Name: "Vorpal Sword", Tier: 5, Description: "Vorpal. You know what it does. The things in the dark know too. They remember the last person who carried this.", Price: 30000},
	},
	SlotArmor: {
		{Name: "Shitty Armor", Tier: 0, Description: "Offers the protection of a strongly-worded letter. Looks worse than it sounds.", Price: 0},
		{Name: "Leather Scraps (Stitched)", Tier: 1, Description: "Leather. Most of it. Keeps the wind out and occasionally a very polite blade.", Price: 100},
		{Name: "Embarrassing Chain Mail", Tier: 2, Description: "Chain mail. Heavy, loud, and genuinely better than dying, which is its only selling point and honestly sufficient.", Price: 450},
		{Name: "Armor (Functional, Ugly)", Tier: 3, Description: "Plate armor. Heavy as bad decisions, protective as a real piece of equipment. Doesn't fit great. Works fine.", Price: 1500},
		{Name: "Enchanted Plate", Tier: 4, Description: "Enchanted plate. Lighter than it has any right to be, tougher than physics should allow. You feel, for the first time, like someone who is supposed to be doing this.", Price: 7500},
		{Name: "Dragonscale", Tier: 5, Description: "Dragonscale. An actual dragon died for this. Someone killed it. Maybe you, eventually. For now you wear the proof that such things are possible.", Price: 30000},
	},
	SlotHelmet: {
		{Name: "Goddamn Offensive Helmet", Tier: 0, Description: "Bad for your head. An insult to everyone in the immediate vicinity.", Price: 0},
		{Name: "Iron Pot with Eyeholes", Tier: 1, Description: "Someone used this as a chamber pot before it was a helmet. There are theories. It has eyeholes now.", Price: 75},
		{Name: "Helmet of Questionable Provenance", Tier: 2, Description: "The scratches were there when you bought it. The steel is sound. Nobody will compliment this helmet.", Price: 350},
		{Name: "Helm of Unremarkable Adequacy", Tier: 3, Description: "Reinforced. Fitted, roughly. Doesn't make you look competent but stops the top of your head from becoming someone else's problem.", Price: 1200},
		{Name: "Guardian's Helm", Tier: 4, Description: "Guardian-grade. This helm has seen real battles, kept real heads intact, and carries itself with the quiet dignity you are only just beginning to deserve.", Price: 6000},
		{Name: "Crown of the Fallen", Tier: 5, Description: "Crown of the Fallen. Every previous owner died in it. None of them died because of it. It will outlast you too.", Price: 25000},
	},
	SlotBoots: {
		{Name: "Knobby-Ass Boots", Tier: 0, Description: "The knobs are not a feature. Nobody knows what they are. Stop looking at them.", Price: 0},
		{Name: "Dead Man's Boots", Tier: 1, Description: "Taken off a corpse. The corpse didn't need them. You do. Don't think about it too hard.", Price: 75},
		{Name: "Boots of Mild Discomfort", Tier: 2, Description: "They've been places. Bad places. Places that did things to the leather you'd rather not examine. They'll hold together. Probably.", Price: 350},
		{Name: "Boots of Getting There Eventually", Tier: 3, Description: "Light enough. Grip is decent. Built for someone who moves with purpose, which you are in the process of becoming.", Price: 1200},
		{Name: "Ranger's Boots", Tier: 4, Description: "Ranger's boots. You move quieter. Faster. Longer. The ground cooperates. The forest notices. Something has shifted.", Price: 6000},
		{Name: "Boots of the Wind", Tier: 5, Description: "The wind doesn't slow you. Terrain offers suggestions you are free to decline. These boots are an affront to the concept of obstacles.", Price: 25000},
	},
	SlotTool: {
		{Name: "Rusted PoS Pickaxe", Tier: 0, Description: "Technically a pickaxe. That is the single nicest thing anyone can say about it.", Price: 0},
		{Name: "Dull Copper Pickaxe", Tier: 1, Description: "Copper. Soft. Gets the job done if you hit very hard and the ore is feeling cooperative.", Price: 100},
		{Name: "Chipped Iron Pickaxe", Tier: 2, Description: "Iron. Chipped to hell but bites the rock with something approaching intention. A pickaxe that exists and functions.", Price: 450},
		{Name: "Serviceable Steel Pickaxe", Tier: 3, Description: "Steel, properly weighted, properly edged. The mountain will acknowledge this pickaxe. Not respect it. Acknowledge it.", Price: 1500},
		{Name: "Mithril Pickaxe", Tier: 4, Description: "Mithril. Weighs nothing. Hits like consequence. Ores don't resist so much as rearrange themselves out of respect.", Price: 7500},
		{Name: "Diamond Pickaxe", Tier: 5, Description: "Diamond. Breaks anything short of fate and occasionally that too. The only limits left are your arm, your nerve, and the number of hours in a day.", Price: 30000},
	},
}

// tier0Equipment returns the name for a given slot at tier 0.
func tier0Equipment(slot EquipmentSlot) string {
	return equipmentTiers[slot][0].Name
}

// equipmentDefByTier returns the definition for a slot at a given tier.
func equipmentDefByTier(slot EquipmentSlot, tier int) EquipmentDef {
	defs := equipmentTiers[slot]
	if tier < 0 || tier >= len(defs) {
		return defs[0]
	}
	return defs[tier]
}

// ── Equipment Score ──────────────────────────────────────────────────────────

func advEquipmentScore(equip map[EquipmentSlot]*AdvEquipment) int {
	score := 0
	for _, slot := range allSlots {
		eq, ok := equip[slot]
		if !ok {
			continue
		}
		tierContrib := eq.Tier
		if slot == SlotWeapon {
			tierContrib *= 2
		}
		// Condition modifier: below 50 halves contribution
		if eq.Condition < 50 {
			tierContrib /= 2
		}
		score += tierContrib
	}
	return score
}

// ── XP & Level-Up ────────────────────────────────────────────────────────────

const maxAdvLevel = 50

// xpToNextLevel returns XP needed to advance from level to level+1.
func xpToNextLevel(level int) int {
	return 100 + (level * 3)
}

// checkAdvLevelUp checks if a character leveled up in the given skill and applies it.
// Returns whether a level-up occurred and the new level.
func checkAdvLevelUp(char *AdventureCharacter, skill string) (bool, int) {
	var xp *int
	var level *int
	switch skill {
	case "combat":
		xp = &char.CombatXP
		level = &char.CombatLevel
	case "mining":
		xp = &char.MiningXP
		level = &char.MiningSkill
	case "foraging":
		xp = &char.ForagingXP
		level = &char.ForagingSkill
	default:
		return false, 0
	}

	if *level >= maxAdvLevel {
		return false, *level
	}

	leveled := false
	for *level < maxAdvLevel {
		needed := xpToNextLevel(*level)
		if *xp < needed {
			break
		}
		*xp -= needed
		*level++
		leveled = true
	}
	return leveled, *level
}

// ── DB CRUD ──────────────────────────────────────────────────────────────────

func loadAdvCharacter(userID id.UserID) (*AdventureCharacter, error) {
	d := db.Get()
	c := &AdventureCharacter{}
	var alive, actionTaken, holidayTaken int
	var deadUntil sql.NullTime

	err := d.QueryRow(`
		SELECT user_id, display_name,
		       combat_level, mining_skill, foraging_skill, fishing_skill,
		       combat_xp, mining_xp, foraging_xp, fishing_xp,
		       alive, dead_until, action_taken_today, holiday_action_taken,
		       arena_wins, arena_losses, invasion_score, title,
		       current_streak, best_streak, last_action_date, grudge_location,
		       created_at, last_active_at
		FROM adventure_characters WHERE user_id = ?`, string(userID)).Scan(
		&c.UserID, &c.DisplayName,
		&c.CombatLevel, &c.MiningSkill, &c.ForagingSkill, &c.FishingSkill,
		&c.CombatXP, &c.MiningXP, &c.ForagingXP, &c.FishingXP,
		&alive, &deadUntil, &actionTaken, &holidayTaken,
		&c.ArenaWins, &c.ArenaLosses, &c.InvasionScore, &c.Title,
		&c.CurrentStreak, &c.BestStreak, &c.LastActionDate, &c.GrudgeLocation,
		&c.CreatedAt, &c.LastActiveAt,
	)
	if err != nil {
		return nil, err
	}
	c.Alive = alive == 1
	c.ActionTakenToday = actionTaken == 1
	c.HolidayActionTaken = holidayTaken == 1
	if deadUntil.Valid {
		c.DeadUntil = &deadUntil.Time
	}
	return c, nil
}

func createAdvCharacter(userID id.UserID, displayName string) error {
	d := db.Get()
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO adventure_characters (user_id, display_name)
		VALUES (?, ?)`, string(userID), displayName)
	if err != nil {
		return err
	}

	// Create tier-0 equipment in all slots
	for _, slot := range allSlots {
		def := equipmentTiers[slot][0]
		_, err = tx.Exec(`
			INSERT INTO adventure_equipment (user_id, slot, tier, condition, name, actions_used)
			VALUES (?, ?, 0, 100, ?, 0)`, string(userID), string(slot), def.Name)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func saveAdvCharacter(char *AdventureCharacter) error {
	d := db.Get()
	alive := 0
	if char.Alive {
		alive = 1
	}
	actionTaken := 0
	if char.ActionTakenToday {
		actionTaken = 1
	}
	holidayTaken := 0
	if char.HolidayActionTaken {
		holidayTaken = 1
	}

	_, err := d.Exec(`
		UPDATE adventure_characters SET
			display_name = ?, combat_level = ?, mining_skill = ?, foraging_skill = ?, fishing_skill = ?,
			combat_xp = ?, mining_xp = ?, foraging_xp = ?, fishing_xp = ?,
			alive = ?, dead_until = ?, action_taken_today = ?, holiday_action_taken = ?,
			arena_wins = ?, arena_losses = ?, invasion_score = ?, title = ?,
			current_streak = ?, best_streak = ?, last_action_date = ?, grudge_location = ?,
			last_active_at = CURRENT_TIMESTAMP
		WHERE user_id = ?`,
		char.DisplayName, char.CombatLevel, char.MiningSkill, char.ForagingSkill, char.FishingSkill,
		char.CombatXP, char.MiningXP, char.ForagingXP, char.FishingXP,
		alive, char.DeadUntil, actionTaken, holidayTaken,
		char.ArenaWins, char.ArenaLosses, char.InvasionScore, char.Title,
		char.CurrentStreak, char.BestStreak, char.LastActionDate, char.GrudgeLocation,
		string(char.UserID),
	)
	return err
}

func loadAdvEquipment(userID id.UserID) (map[EquipmentSlot]*AdvEquipment, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT slot, tier, condition, name, actions_used
		FROM adventure_equipment WHERE user_id = ?`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	equip := make(map[EquipmentSlot]*AdvEquipment)
	for rows.Next() {
		e := &AdvEquipment{}
		var slot string
		if err := rows.Scan(&slot, &e.Tier, &e.Condition, &e.Name, &e.ActionsUsed); err != nil {
			return nil, err
		}
		e.Slot = EquipmentSlot(slot)
		equip[e.Slot] = e
	}
	return equip, rows.Err()
}

func saveAdvEquipment(userID id.UserID, eq *AdvEquipment) error {
	d := db.Get()
	_, err := d.Exec(`
		UPDATE adventure_equipment
		SET tier = ?, condition = ?, name = ?, actions_used = ?
		WHERE user_id = ? AND slot = ?`,
		eq.Tier, eq.Condition, eq.Name, eq.ActionsUsed,
		string(userID), string(eq.Slot))
	return err
}

func loadAdvInventory(userID id.UserID) ([]AdvItem, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT id, name, item_type, tier, value
		FROM adventure_inventory WHERE user_id = ?
		ORDER BY tier DESC, value DESC`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AdvItem
	for rows.Next() {
		var it AdvItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Type, &it.Tier, &it.Value); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func addAdvInventoryItem(userID id.UserID, item AdvItem) error {
	d := db.Get()
	_, err := d.Exec(`
		INSERT INTO adventure_inventory (user_id, name, item_type, tier, value)
		VALUES (?, ?, ?, ?, ?)`,
		string(userID), item.Name, item.Type, item.Tier, item.Value)
	return err
}

func removeAdvInventoryItem(itemID int64) error {
	d := db.Get()
	_, err := d.Exec(`DELETE FROM adventure_inventory WHERE id = ?`, itemID)
	return err
}

func clearAdvInventory(userID id.UserID) ([]AdvItem, error) {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return nil, err
	}
	d := db.Get()
	_, err = d.Exec(`DELETE FROM adventure_inventory WHERE user_id = ?`, string(userID))
	if err != nil {
		return nil, err
	}
	return items, nil
}

func advInventoryCount(userID id.UserID) int {
	d := db.Get()
	var count int
	_ = d.QueryRow(`SELECT COUNT(*) FROM adventure_inventory WHERE user_id = ?`, string(userID)).Scan(&count)
	return count
}

func loadAllAdvCharacters() ([]AdventureCharacter, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT user_id, display_name,
		       combat_level, mining_skill, foraging_skill, fishing_skill,
		       combat_xp, mining_xp, foraging_xp, fishing_xp,
		       alive, dead_until, action_taken_today, holiday_action_taken,
		       arena_wins, arena_losses, invasion_score, title,
		       current_streak, best_streak, last_action_date, grudge_location,
		       created_at, last_active_at
		FROM adventure_characters`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []AdventureCharacter
	for rows.Next() {
		c := AdventureCharacter{}
		var alive, actionTaken, holidayTaken int
		var deadUntil sql.NullTime
		if err := rows.Scan(
			&c.UserID, &c.DisplayName,
			&c.CombatLevel, &c.MiningSkill, &c.ForagingSkill, &c.FishingSkill,
			&c.CombatXP, &c.MiningXP, &c.ForagingXP, &c.FishingXP,
			&alive, &deadUntil, &actionTaken, &holidayTaken,
			&c.ArenaWins, &c.ArenaLosses, &c.InvasionScore, &c.Title,
			&c.CurrentStreak, &c.BestStreak, &c.LastActionDate, &c.GrudgeLocation,
			&c.CreatedAt, &c.LastActiveAt,
		); err != nil {
			return nil, err
		}
		c.Alive = alive == 1
		c.ActionTakenToday = actionTaken == 1
		c.HolidayActionTaken = holidayTaken == 1
		if deadUntil.Valid {
			c.DeadUntil = &deadUntil.Time
		}
		chars = append(chars, c)
	}
	return chars, rows.Err()
}

func resetAllAdvDailyActions() error {
	d := db.Get()
	// Only reset actions taken before today — protects against race if a player
	// resolves their action at exactly midnight.
	today := time.Now().UTC().Format("2006-01-02")
	_, err := d.Exec(`UPDATE adventure_characters SET action_taken_today = 0, holiday_action_taken = 0 WHERE last_action_date < ? OR last_action_date IS NULL`, today)
	return err
}

func logAdvActivity(userID id.UserID, activityType, location, outcome string, lootValue int64, xpGained int, flavorKey string) {
	db.Exec("adventure: log activity",
		`INSERT INTO adventure_activity_log (user_id, activity_type, location, outcome, loot_value, xp_gained, flavor_key)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(userID), activityType, location, outcome, lootValue, xpGained, flavorKey)
}

// ── Buff CRUD ────────────────────────────────────────────────────────────────

func loadAdvActiveBuffs(userID id.UserID) ([]AdvBuff, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT id, user_id, buff_type, buff_name, modifier, expires_at
		FROM adventure_buffs
		WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buffs []AdvBuff
	for rows.Next() {
		var b AdvBuff
		if err := rows.Scan(&b.ID, &b.UserID, &b.BuffType, &b.BuffName, &b.Modifier, &b.ExpiresAt); err != nil {
			return nil, err
		}
		buffs = append(buffs, b)
	}
	return buffs, rows.Err()
}

func addAdvBuff(userID id.UserID, buffType, buffName string, modifier float64, expiresAt time.Time) error {
	d := db.Get()
	_, err := d.Exec(`
		INSERT INTO adventure_buffs (user_id, buff_type, buff_name, modifier, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		string(userID), buffType, buffName, modifier, expiresAt)
	return err
}

func pruneAdvExpiredBuffs() error {
	d := db.Get()
	_, err := d.Exec(`DELETE FROM adventure_buffs WHERE expires_at < CURRENT_TIMESTAMP`)
	return err
}

// ── Today's Activity Log ─────────────────────────────────────────────────────

type AdvDayLog struct {
	UserID       id.UserID
	ActivityType string
	Location     string
	Outcome      string
	LootValue    int64
	XPGained     int
}

func loadAdvTodayLogs() ([]AdvDayLog, error) {
	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")
	rows, err := d.Query(`
		SELECT user_id, activity_type, COALESCE(location,''), outcome, loot_value, xp_gained
		FROM adventure_activity_log
		WHERE logged_at >= ? AND logged_at < DATE(?, '+1 day')
		ORDER BY logged_at`, today, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AdvDayLog
	for rows.Next() {
		var l AdvDayLog
		if err := rows.Scan(&l.UserID, &l.ActivityType, &l.Location, &l.Outcome, &l.LootValue, &l.XPGained); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
