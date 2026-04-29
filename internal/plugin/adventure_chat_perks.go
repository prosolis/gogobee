package plugin

// ── Chat Level Perks ────────────────────────────────────────────────────────
// Passive bonuses based on community chat level. All perks are automatic.

// chatLevelXPBonus returns the fractional XP bonus for the given chat level.
// +5% per 10 levels, capped at +25% at level 50+.
func chatLevelXPBonus(chatLevel int) float64 {
	tier := chatLevel / 10
	if tier > 5 {
		tier = 5
	}
	return float64(tier) * 0.05
}

// chatLevelRareBonus returns the additive rare drop rate bonus.
// +0.5% per 10 levels, capped at +2.5% at level 50+.
func chatLevelRareBonus(chatLevel int) float64 {
	tier := chatLevel / 10
	if tier > 5 {
		tier = 5
	}
	return float64(tier) * 0.005
}

// shopGreeting returns the shopkeeper's greeting based on chat level.
func shopGreeting(chatLevel int) string {
	switch {
	case chatLevel >= 50:
		return "I saw you coming from down the street. Your usual is already on the counter. The shop is open."
	case chatLevel >= 30:
		return "Back again. I've started keeping your usual in the back. The shop is open."
	case chatLevel >= 10:
		return "Ah. You again. The shop is open."
	default:
		return "The shop is open. What do you need?"
	}
}
