package plugin

// M1 close-out sweep for gogobee_mischief_plan.md — survival per tier through the
// REAL delivery path (runMischiefInterrupt → runZoneCombatRoster), not through
// SimulateCombat.
//
// The M0 sweep that priced the fee table measured a full-HP build in a single
// chain. Two things it could not see, and both of them are why this exists:
//
//   - A real target is WOUNDED. The monster arrives mid-run, after the dungeon has
//     already taken a bite out of them. The entryHP arm is the control-vs-treatment
//     axis: 100% reproduces M0 through the new path (so a divergence there is a
//     path bug, not a difficulty finding), 70% and 40% are what a live delivery
//     actually lands on.
//   - The M2 ward. `blessings` drives the same temp-HP cushion the room buys, so
//     we can price what €75 of charity is actually worth against an elite.
//
// Run (heavy — see the plan's sim rules, n≥750 per cell for a real signal):
//
//	MISCHIEF_SWEEP=1 go test ./internal/plugin -run TestMischiefDeliverySweep -v -timeout 4h

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// mischiefDeliveryTrial seeds a real adventurer on a real expedition at the given
// fraction of their max HP, sends them the tier's monster through the production
// delivery path, and reports whether they were still standing.
//
// Survival is read the way the delivery reads it (HP > 0), not as "won every
// fight" — an engine timeout with HP left is a target who held the thing off.
func mischiefDeliveryTrial(
	t *testing.T, p *AdventurePlugin, prof classBalanceProfile,
	tierKey string, entryHPPct float64, blessings int, seq int, rng *rand.Rand,
) (survived bool, endHPPct float64) {
	t.Helper()
	uid := id.UserID(fmt.Sprintf("@sweep%d:sim", seq))

	c := buildHarnessCharacter(prof)
	c.UserID = uid
	c.HPCurrent = int(float64(c.HPMax) * entryHPPct)
	if c.HPCurrent < 1 {
		c.HPCurrent = 1
	}
	if err := createAdvCharacter(uid, "sweep"+strconv.Itoa(seq)); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}

	run, err := startZoneRun(uid, ZoneGoblinWarrens, prof.Level, rng)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	expID := "exp-sweep-" + strconv.Itoa(seq)
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, run_id, status, start_date, coins_earned)
		VALUES (?, ?, 'goblin_warrens', ?, 'active', ?, 0)`,
		expID, string(uid), run.RunID, time.Now().UTC()); err != nil {
		t.Fatalf("seed expedition: %v", err)
	}
	exp, err := getActiveExpedition(uid)
	if err != nil || exp == nil {
		t.Fatalf("getActiveExpedition: %v", err)
	}

	bracket := mischiefBracketZone(prof.Level)
	chain, ambush := mischiefMonsters(tierKey, bracket, rng)

	_, _, survived = p.runMischiefInterrupt(uid, exp, bracket, chain, ambush, blessings)
	hp, max := dndHPSnapshot(uid)
	if max <= 0 {
		max = 1
	}
	return survived, float64(hp) / float64(max)
}

func TestMischiefDeliverySweep(t *testing.T) {
	if os.Getenv("MISCHIEF_SWEEP") == "" {
		t.Skip("set MISCHIEF_SWEEP=1 to run")
	}
	trials := 250
	if n := os.Getenv("MISCHIEF_TRIALS"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			trials = v
		}
	}
	newMischiefTestDB(t)
	p := &AdventurePlugin{euro: NewEuroPlugin(nil)}
	p.Sink = &captureSink{}

	rng := rand.New(rand.NewPCG(20260713, 0x64656C6976657279))
	profiles := []classBalanceProfile{
		{Class: ClassPaladin, Level: 3},
		{Class: ClassMage, Level: 4},
		{Class: ClassMage, Level: 8, Subclass: SubclassEvocation},
		{Class: ClassFighter, Level: 12, Subclass: SubclassChampion},
		{Class: ClassRogue, Level: 20, Subclass: SubclassArcaneTrickster},
		{Class: ClassCleric, Level: 20, Subclass: SubclassLifeDomain},
	}
	// entryHP 1.0 is the control arm: it should reproduce the M0 table through the
	// real path. The wounded arms are the finding.
	entryHPs := []float64{1.0, 0.7, 0.4}

	seq := 0
	fmt.Printf("\n%-26s %-6s %-7s %-6s %8s %10s\n",
		"profile", "tier", "entryHP", "wards", "survive%", "avgEndHP%")
	for _, prof := range profiles {
		for _, tier := range mischiefTiers {
			for _, entry := range entryHPs {
				// The ward arm only where a ward could decide anything: the tiers the
				// M0 sweep did not already call theatre.
				wardArms := []int{0}
				if tier.Key == "elite" || tier.Key == "boss" {
					wardArms = []int{0, mischiefBlessingCap}
				}
				for _, wards := range wardArms {
					wins, hpSum := 0, 0.0
					for i := 0; i < trials; i++ {
						seq++
						ok, hpPct := mischiefDeliveryTrial(t, p, prof, tier.Key, entry, wards, seq, rng)
						if ok {
							wins++
							hpSum += hpPct
						}
					}
					avgHP := 0.0
					if wins > 0 {
						avgHP = hpSum / float64(wins) * 100
					}
					fmt.Printf("%-26s %-6s %6.0f%% %6d %7.1f%% %9.1f%%\n",
						fmt.Sprintf("%s L%d %s", prof.Class, prof.Level, prof.Subclass),
						tier.Key, entry*100, wards, float64(wins)/float64(trials)*100, avgHP)
				}
			}
		}
	}
}
