package plugin

import (
	"math/rand/v2"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Treasure Bonus Type (DB row) ─────────────────────────────────────────────

type AdvTreasureBonus struct {
	TreasureKey string
	Name        string
	Tier        int
	BonusType   string
	BonusValue  float64
}

// ── Treasure Definition ──────────────────────────────────────────────────────

type AdvTreasureDef struct {
	Key           string
	Name          string
	Tier          int
	Bonuses       []advTreasureBonusDef
	InventoryDesc string
	RoomAnnounce  string // non-empty for tier 5 and special items
}

type advTreasureBonusDef struct {
	Type  string
	Value float64
}

type AdvTreasureDrop struct {
	Def *AdvTreasureDef
}

// ── Drop Rates ───────────────────────────────────────────────────────────────

var advTreasureDropRates = map[int]float64{
	1: 0.015,
	2: 0.012,
	3: 0.008,
	4: 0.004,
	5: 0.0015,
}

const advMaxTreasures = 3

// ── Treasure Definitions ─────────────────────────────────────────────────────

var advAllTreasures = map[int][]AdvTreasureDef{
	1: {
		{
			Key: "soggy_cellar_crown_jewel", Name: "The Soggy Cellar Crown Jewel", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "foraging_skill", Value: 1}},
			InventoryDesc: "The Soggy Cellar Crown Jewel. A button. +1 Foraging.",
		},
		{
			Key: "1up_mushroom_expired", Name: "The 1-Up Mushroom (Expired)", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "special_respawn_halve", Value: 1}}, // v2: not active in v1
			InventoryDesc: "The 1-Up Mushroom (Expired). Slightly grey. Once weekly: 12hr respawn.",
		},
		{
			Key: "rat_king_lucky_foot", Name: "The Rat King's Lucky Foot", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "death_chance", Value: -2}},
			InventoryDesc: "The Rat King's Lucky Foot. Slightly damp. -2% death chance.",
		},
		{
			Key: "ancient_cellar_medallion", Name: "Ancient Cellar Medallion (Probably)", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "xp_multiplier", Value: 5}},
			InventoryDesc: "Ancient Cellar Medallion (Probably). Illegible engravings. +5% dungeon XP.",
		},
		{
			Key: "bent_copper_coin", Name: "The Considerably Bent Copper Coin of Fortune", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "loot_quality", Value: 2}},
			InventoryDesc: "The Considerably Bent Copper Coin of Fortune. Very bent. +2% loot quality.",
		},
		{
			Key: "wooden_sword", Name: "The Wooden Sword (It Was Dangerous to Go Alone)", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "combat_level", Value: 1}},
			InventoryDesc: "The Wooden Sword. From an old man. +1 Combat. It was dangerous to go alone.",
		},
		{
			Key: "suspicious_mushroom", Name: "Suspicious Mushroom (Do Not Ask Where It's Been)", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "exceptional_chance", Value: 2}},
			InventoryDesc: "Suspicious Mushroom. Glowing. Humming. Do not ask. +2% exceptional chance.",
		},
		{
			Key: "power_pellet", Name: "Power Pellet (Slightly Past Its Best)", Tier: 1,
			Bonuses:       []advTreasureBonusDef{{Type: "death_chance", Value: -3}},
			InventoryDesc: "Power Pellet (Vintage). -3% death chance. The monsters remember.",
		},
		{
			Key: "konami_scroll", Name: "The Konami Scroll", Tier: 1,
			Bonuses: []advTreasureBonusDef{
				{Type: "combat_level", Value: 2},
				{Type: "mining_skill", Value: 2},
				{Type: "foraging_skill", Value: 2},
			},
			InventoryDesc: "The Konami Scroll. ↑↑↓↓←→←→. +2 all skills. You know what it means.",
		},
	},
	2: {
		{
			Key: "goblin_warchief_ring", Name: "Goblin Warchief's Signet Ring", Tier: 2,
			Bonuses:       []advTreasureBonusDef{{Type: "combat_level", Value: 2}},
			InventoryDesc: "Goblin Warchief's Signet Ring. Thumb-sized. +2 Combat.",
		},
		{
			Key: "kobold_compass", Name: "The Kobold Cartographer's Compass", Tier: 2,
			Bonuses:       []advTreasureBonusDef{{Type: "mining_skill", Value: 3}},
			InventoryDesc: "Kobold Cartographer's Compass. Fish bone needle. +3 Mining.",
		},
		{
			Key: "spread_gun", Name: "The Spread Gun", Tier: 2,
			Bonuses: []advTreasureBonusDef{
				{Type: "combat_level", Value: 4},
				{Type: "success_chance", Value: 8},
			},
			InventoryDesc: "The Spread Gun. You know. +4 Combat, +8% dungeon success.",
		},
		{
			Key: "e_tank", Name: "E-Tank (Partially Full, Handle With Care)", Tier: 2,
			Bonuses:       []advTreasureBonusDef{{Type: "special_monthly_respawn", Value: 1}}, // v2: not active in v1
			InventoryDesc: "E-Tank (Half Full). 12hr respawn on 2nd monthly death. Handle with care.",
		},
		{
			Key: "stone_of_jordan", Name: "The Stone of Jordan", Tier: 2,
			Bonuses: []advTreasureBonusDef{
				{Type: "mining_skill", Value: 3},
				{Type: "foraging_skill", Value: 3},
			},
			InventoryDesc: "The Stone of Jordan. Fits perfectly. Always. +3 Mining/Foraging.",
		},
	},
	3: {
		{
			Key: "draugr_memory_stone", Name: "The Draugr's Memory Stone", Tier: 3,
			Bonuses: []advTreasureBonusDef{
				{Type: "combat_level", Value: 5},
				{Type: "special_weekly_death_pass", Value: 1}, // v2
			},
			InventoryDesc: "The Draugr's Memory Stone. Warm. +5 Combat, weekly death pass.",
		},
		{
			Key: "ghost_touched_lantern", Name: "Ghost-Touched Lantern", Tier: 3,
			Bonuses: []advTreasureBonusDef{
				{Type: "foraging_skill", Value: 4},
				{Type: "mining_skill", Value: 4},
			},
			InventoryDesc: "Ghost-Touched Lantern. Glows without fuel. +4 Foraging, +4 Mining.",
		},
		{
			Key: "estus_flask", Name: "The Estus Flask (Cracked But Functional)", Tier: 3,
			Bonuses:       []advTreasureBonusDef{{Type: "death_chance", Value: -6}},
			InventoryDesc: "Estus Flask (Cracked). Warm. Always warm. -6% death, daily death mitigation.",
		},
		{
			Key: "twinbee_bell", Name: "TwinBee's Bell (Genuine Article)", Tier: 3,
			Bonuses: []advTreasureBonusDef{
				{Type: "all_skills", Value: 6},
				{Type: "xp_multiplier", Value: 5},
			},
			InventoryDesc: "TwinBee's Bell. Rings on its own. Approves of you. +6 all skills, +5% XP.",
		},
	},
	4: {
		{
			Key: "hollow_crown", Name: "The Hollow Crown", Tier: 4,
			Bonuses: []advTreasureBonusDef{
				{Type: "combat_level", Value: 10},
				{Type: "exceptional_chance", Value: 5},
			},
			InventoryDesc: "The Hollow Crown. Sits crooked. +10 Combat, +5% exceptional dungeon chance.",
		},
		{
			Key: "wardens_last_key", Name: "Warden's Last Key", Tier: 4,
			Bonuses: []advTreasureBonusDef{
				{Type: "mining_skill", Value: 8},
				{Type: "combat_level", Value: 8},
				{Type: "special_sealed_vault", Value: 1}, // v2
			},
			InventoryDesc: "Warden's Last Key. Cold to the touch. +8 Mining/Combat. One use.",
		},
		{
			Key: "thunderfury", Name: "[THUNDERFURY, BLESSED BLADE OF THE WINDSEEKER]", Tier: 4,
			Bonuses: []advTreasureBonusDef{
				{Type: "combat_level", Value: 12},
				{Type: "success_chance", Value: 10},
			},
			InventoryDesc: "[THUNDERFURY, BLESSED BLADE OF THE WINDSEEKER]. Yes you got it. +12 Combat.",
			RoomAnnounce:  "⚡ Did {name} get Thunderfury? {name} got Thunderfury. [THUNDERFURY, BLESSED BLADE OF THE WINDSEEKER] has been found in {location}.",
		},
		{
			Key: "ocarina", Name: "The Ocarina (Cracked, Still Plays)", Tier: 4,
			Bonuses:       []advTreasureBonusDef{{Type: "all_skills", Value: 10}},
			InventoryDesc: "The Ocarina (Cracked). Three songs. +10 all skills. Do not play the third one.",
		},
	},
	5: {
		{
			Key: "shard_of_unnamed", Name: "Shard of the Unnamed", Tier: 5,
			Bonuses: []advTreasureBonusDef{
				{Type: "combat_level", Value: 15},
				{Type: "xp_multiplier", Value: 10},
				{Type: "death_chance", Value: -5},
			},
			InventoryDesc: "Shard of the Unnamed. +15 Combat, +10% XP, -5% death.",
			RoomAnnounce:  "🔴 {name} has recovered the Shard of the Unnamed from the Abyssal Maw. The server feels different.",
		},
		{
			Key: "cartographers_final_map", Name: "The Cartographer's Final Map", Tier: 5,
			Bonuses:       []advTreasureBonusDef{{Type: "all_skills", Value: 12}},
			InventoryDesc: "The Cartographer's Final Map. Updates on its own. +12 all skills, full map.",
			RoomAnnounce:  "🔴 {name} has found the Cartographer's Final Map in the Abyssal Maw. It has their name on it. It always did.",
		},
		{
			Key: "triforce_shard", Name: "The Triforce Shard (One Third of Something Larger)", Tier: 5,
			Bonuses: []advTreasureBonusDef{
				{Type: "all_skills", Value: 5},
				{Type: "death_chance", Value: -8},
				// Note: +15 to chosen skill is v2 interactive
			},
			InventoryDesc: "Triforce Shard (×1/3). Warm. Waiting. +5 all skills, -8% death.",
			RoomAnnounce:  "🔺 {name} has recovered a Triforce Shard from the Abyssal Maw. One third of something. The other two thirds are somewhere. Probably.",
		},
		{
			Key: "the_corridor", Name: "The Corridor (You Know the One)", Tier: 5,
			Bonuses: []advTreasureBonusDef{
				{Type: "all_skills", Value: 12},
				{Type: "special_monthly_death_bypass", Value: 1}, // v2
			},
			InventoryDesc: "The Corridor. Folded. Don't look back. +12 all skills, monthly death bypass.",
			RoomAnnounce:  "🔴 {name} found The Corridor in the Abyssal Maw. They know the one. So does it.",
		},
	},
}

// ── Treasure Drop Logic ──────────────────────────────────────────────────────

func rollAdvTreasureDrop(tier int, userID id.UserID) *AdvTreasureDrop {
	rate, ok := advTreasureDropRates[tier]
	if !ok {
		return nil
	}

	if rand.Float64() >= rate {
		return nil
	}

	pool, ok := advAllTreasures[tier]
	if !ok || len(pool) == 0 {
		return nil
	}

	// Pick random treasure
	def := &pool[rand.IntN(len(pool))]

	// Duplicate check
	owns, err := advUserOwnsTreasure(userID, def.Key)
	if err != nil || owns {
		// Reroll once
		def = &pool[rand.IntN(len(pool))]
		owns, err = advUserOwnsTreasure(userID, def.Key)
		if err != nil || owns {
			return nil // both rolls duplicated
		}
	}

	return &AdvTreasureDrop{Def: def}
}

// ── Treasure DB Operations ───────────────────────────────────────────────────

func advSaveTreasure(userID id.UserID, def *AdvTreasureDef) error {
	d := db.Get()
	tx, err := d.Begin()
	if err != nil {
		return err
	}

	for _, bonus := range def.Bonuses {
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO adventure_treasures (user_id, treasure_key, name, tier, bonus_type, bonus_value)
			VALUES (?, ?, ?, ?, ?, ?)`,
			string(userID), def.Key, def.Name, def.Tier, bonus.Type, bonus.Value)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func advDiscardTreasure(userID id.UserID, treasureKey string) error {
	d := db.Get()
	_, err := d.Exec(`DELETE FROM adventure_treasures WHERE user_id = ? AND treasure_key = ?`,
		string(userID), treasureKey)
	return err
}

func advCountTreasures(userID id.UserID) (int, error) {
	d := db.Get()
	var count int
	err := d.QueryRow(`
		SELECT COUNT(DISTINCT treasure_key) FROM adventure_treasures WHERE user_id = ?`,
		string(userID)).Scan(&count)
	return count, err
}

func advUserOwnsTreasure(userID id.UserID, treasureKey string) (bool, error) {
	d := db.Get()
	var count int
	err := d.QueryRow(`
		SELECT COUNT(*) FROM adventure_treasures WHERE user_id = ? AND treasure_key = ?`,
		string(userID), treasureKey).Scan(&count)
	return count > 0, err
}

func loadAdvTreasureBonuses(userID id.UserID) ([]AdvTreasureBonus, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT treasure_key, name, tier, bonus_type, bonus_value
		FROM adventure_treasures WHERE user_id = ?`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bonuses []AdvTreasureBonus
	for rows.Next() {
		var b AdvTreasureBonus
		if err := rows.Scan(&b.TreasureKey, &b.Name, &b.Tier, &b.BonusType, &b.BonusValue); err != nil {
			return nil, err
		}
		bonuses = append(bonuses, b)
	}
	return bonuses, rows.Err()
}

// advUserTreasures returns the distinct treasures a user owns (for display/discard prompts).
func advUserTreasures(userID id.UserID) ([]AdvTreasureDef, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT DISTINCT treasure_key, name, tier
		FROM adventure_treasures WHERE user_id = ?
		ORDER BY tier, treasure_key`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var treasures []AdvTreasureDef
	for rows.Next() {
		var t AdvTreasureDef
		if err := rows.Scan(&t.Key, &t.Name, &t.Tier); err != nil {
			return nil, err
		}
		// Look up full definition
		for tier, defs := range advAllTreasures {
			for _, def := range defs {
				if def.Key == t.Key {
					t = def
					t.Tier = tier
					break
				}
			}
		}
		treasures = append(treasures, t)
	}
	return treasures, rows.Err()
}
