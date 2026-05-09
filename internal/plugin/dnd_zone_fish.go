package plugin

import (
	"math/rand/v2"

	"gogobee/internal/flavor"
)

// Resource & Combat Integration §6 — Fishing System Integration.
//
// Fishing reuses the harvest pipeline (HarvestFish action). This file
// adds the §6 zone gate, the existing FishingSkill rank bonus, the
// Ranger +20% rare-catch rule, and the Feywild Time Distortion hook.

// fishingZones is the §6.1 allow-list — only these zones expose !fish.
var fishingZones = map[ZoneID]bool{
	ZoneForestShadows:   true,
	ZoneSunkenTemple:    true,
	ZoneUnderdark:       true,
	ZoneFeywildCrossing: true,
}

// isFishingZone returns true if !fish is permitted in zoneID.
func isFishingZone(zoneID ZoneID) bool { return fishingZones[zoneID] }

// fishingSkillBonus folds the legacy FishingSkill (0–10) into a small
// roll bonus so existing rank progression still pays off inside the
// expedition pipeline. +1 per 5 levels, capped at +2.
func fishingSkillBonus(fishingSkill int) int {
	if fishingSkill <= 0 {
		return 0
	}
	bonus := fishingSkill / 5
	if bonus > 2 {
		bonus = 2
	}
	return bonus
}

// rangerRareCatchUpgrade implements §6.2 — Ranger +20% rare catch rate.
// When the harvest outcome is Common or Standard for a fishing attempt
// by a Ranger, 20% chance to upgrade to Rich. Returns the (possibly
// upgraded) outcome.
func rangerRareCatchUpgrade(class DnDClass, action HarvestAction, outcome HarvestOutcome, rng *rand.Rand) HarvestOutcome {
	if class != ClassRanger || action != HarvestFish {
		return outcome
	}
	if outcome != OutcomeCommon && outcome != OutcomeStandard {
		return outcome
	}
	if rngFloat(rng) < 0.20 {
		return OutcomeRich
	}
	return outcome
}

// feywildFishDistortion narrates a Time Distortion flicker on a fishing
// attempt in Feywild Crossing. 15% chance per attempt; pure flavor for
// R4b (mechanical Time Loop event lives in the temporal-events system
// and is not duplicated here).
func feywildFishDistortion(zoneID ZoneID, action HarvestAction, rng *rand.Rand) string {
	if zoneID != ZoneFeywildCrossing || action != HarvestFish {
		return ""
	}
	if rngFloat(rng) >= 0.15 {
		return ""
	}
	line := flavor.Pick(flavor.FeywildTimeDistortionHalf)
	if line == "" {
		line = flavor.Pick(flavor.FeywildTimeLoop)
	}
	if line == "" {
		return ""
	}
	return "_⏳ " + line + "_\n\n"
}
