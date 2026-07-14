package plugin

// M0 pricing sweep for gogobee_mischief_plan.md — single-fight survival per
// mischief tier, measured against the class-balance harness builds. Skip-gated:
// it is a pricing instrument, not a regression test.
//
// It drives the SAME production selection code a live delivery does
// (mischiefBracketZone + mischiefMonsters), so the fee table can't silently
// drift away from the fight it was priced against. What it does NOT share is the
// delivery path itself: this measures a full-HP build in a single chain, which
// is optimistic — a real target is wounded mid-run, and may have a party. The
// plan's M1 close-out item is to re-run this through runMischiefInterrupt.
//
// Run: MISCHIEF_SWEEP=1 go test ./internal/plugin -run TestMischiefPricingSweep -v

import (
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
)

// one trial: full-HP build fights the tier's chain; survival = won every fight.
func mischiefTrial(p classBalanceProfile, tierKey string, rng *rand.Rand) (survived bool, endHPPct float64) {
	c := buildHarnessCharacter(p)
	zone := mischiefBracketZone(p.Level)
	chain, ambush := mischiefMonsters(tierKey, zone, rng)
	hp := c.HPMax
	for i, m := range chain {
		if ambush && i == 0 {
			hp -= clampSurpriseNick(surpriseRoundNick(m, int(zone.Tier)), hp, c.HPMax)
		}
		player := buildHarnessPlayer(c)
		if hp < c.HPMax {
			player.Stats.StartHP = hp
		}
		enemy := buildHarnessZoneEnemy(m, int(zone.Tier))
		if spell, slot, ok := pickBestDamageSpell(c); ok {
			applyHarnessSpellCast(c, spell, slot, &player.Stats, &player.Mods, &enemy.Stats)
		}
		res := SimulateCombat(player, enemy, dungeonCombatPhases)
		if !res.PlayerWon {
			return false, 0
		}
		hp = res.PlayerEndHP
	}
	return true, float64(hp) / float64(c.HPMax)
}

func TestMischiefPricingSweep(t *testing.T) {
	if os.Getenv("MISCHIEF_SWEEP") == "" {
		t.Skip("set MISCHIEF_SWEEP=1 to run")
	}
	const trials = 2000
	rng := rand.New(rand.NewPCG(20260713, 0x6D69736368696566))
	profiles := []classBalanceProfile{
		{Class: ClassPaladin, Level: 1},
		{Class: ClassMage, Level: 4},
		{Class: ClassMage, Level: 8, Subclass: SubclassEvocation},
		{Class: ClassFighter, Level: 12, Subclass: SubclassChampion},
		{Class: ClassRogue, Level: 20, Subclass: SubclassArcaneTrickster},
		{Class: ClassCleric, Level: 20, Subclass: SubclassLifeDomain},
	}
	fmt.Printf("%-28s %-8s %8s %10s\n", "profile", "tier", "survive%", "avgEndHP%")
	for _, p := range profiles {
		for _, tier := range mischiefTiers {
			wins := 0
			hpSum := 0.0
			for i := 0; i < trials; i++ {
				ok, hpPct := mischiefTrial(p, tier.Key, rng)
				if ok {
					wins++
					hpSum += hpPct
				}
			}
			avgHP := 0.0
			if wins > 0 {
				avgHP = hpSum / float64(wins) * 100
			}
			fmt.Printf("%-28s %-8s %7.1f%% %9.1f%%\n",
				fmt.Sprintf("%s L%d %s", p.Class, p.Level, p.Subclass), tier.Key,
				float64(wins)/trials*100, avgHP)
		}
	}
}
