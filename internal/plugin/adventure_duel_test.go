package plugin

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func newDuelTestDB(t *testing.T) {
	t.Helper()
	db.Close()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// duelSeed builds a deterministic RNG so the two-orientation resolver is
// reproducible across runs — the production path passes nil (global RNG).
func duelSeed(i uint64) *rand.Rand { return rand.New(rand.NewPCG(0x1d4e11, i)) }

func TestDuelPayout(t *testing.T) {
	// Both escrow 2000 → pool 4000 → winner 70% (2800), pot 30% (1200).
	w, pot := duelPayout(2000)
	if w != 2800 || pot != 1200 {
		t.Fatalf("duelPayout(2000) = (%d, %d), want (2800, 1200)", w, pot)
	}
	// The pot always gets exactly what the winner doesn't; nothing is minted
	// or lost against the pooled 2×stake.
	for _, stake := range []int{100, 137, 999, 6000} {
		w, pot := duelPayout(stake)
		if w+pot != stake*2 {
			t.Errorf("duelPayout(%d): winner+pot = %d, want pool %d", stake, w+pot, stake*2)
		}
		if w <= pot {
			t.Errorf("duelPayout(%d): winner %d should exceed pot %d", stake, w, pot)
		}
	}
}

func TestDuelMaxStake(t *testing.T) {
	if got := duelMaxStake(5); got != 2500 {
		t.Fatalf("duelMaxStake(5) = %d, want 2500", got)
	}
}

func TestDuelPerHitDamage(t *testing.T) {
	// 1d8 (avg 4.5) + ability mod 3 = 7.5 base, + Divine Strike 2 = 9.5,
	// × (1 + 0.25 damage bonus) = 11.875.
	pc := Combatant{
		Stats: CombatStats{
			Weapon:              &WeaponProfile{DamageCount: 1, DamageSides: 8},
			AbilityModForDamage: 3,
		},
		Mods: CombatModifiers{DivineStrikePerHit: 2, DamageBonus: 0.25},
	}
	got := duelPerHitDamage(pc)
	if math.Abs(got-11.875) > 1e-9 {
		t.Fatalf("duelPerHitDamage = %v, want 11.875", got)
	}

	// Weaponless build falls back to the flat Attack stat.
	bare := Combatant{Stats: CombatStats{Attack: 9}}
	if got := duelPerHitDamage(bare); got != 9 {
		t.Fatalf("weaponless duelPerHitDamage = %v, want 9", got)
	}

	// A build that computes to <1 is floored to 1 (a fight always deals damage).
	weak := Combatant{Stats: CombatStats{Attack: 0}}
	if got := duelPerHitDamage(weak); got != 1 {
		t.Fatalf("floored duelPerHitDamage = %v, want 1", got)
	}
}

func TestSynthDuelEnemy(t *testing.T) {
	pc := Combatant{
		Name:     "Aria",
		IsPlayer: true,
		Stats: CombatStats{
			MaxHP:               123,
			AC:                  17,
			AttackBonus:         8,
			Defense:             5,
			Weapon:              &WeaponProfile{DamageCount: 1, DamageSides: 8},
			AbilityModForDamage: 3,
		},
		Mods: CombatModifiers{ExtraAttacks: 1}, // two swings/round
	}
	e := synthDuelEnemy(pc)

	if e.IsPlayer {
		t.Error("synthesised enemy must not be flagged IsPlayer")
	}
	if e.Ability != nil {
		t.Error("v1 synth enemy carries no MonsterAbility")
	}
	// HP / AC / to-hit carry over verbatim: to-hit stays faithful both ways.
	if e.Stats.MaxHP != 123 || e.Stats.AC != 17 || e.Stats.AttackBonus != 8 {
		t.Errorf("stat carryover wrong: got HP=%d AC=%d AB=%d", e.Stats.MaxHP, e.Stats.AC, e.Stats.AttackBonus)
	}
	// Damage: per-hit (7.5) × swings (2) folded into one flat Attack = 15.
	if e.Stats.Attack != 15 {
		t.Errorf("folded Attack = %d, want 15 (7.5 × 2 swings)", e.Stats.Attack)
	}
}

// TestResolveDuel_StrongerWinsRegardlessOfRole pins that a clearly superior
// build wins whether it issues or receives the challenge — the two-orientation
// resolver must not hand the win to the attacker seat.
func TestResolveDuel_StrongerWinsRegardlessOfRole(t *testing.T) {
	strong := Combatant{
		Name: "Champion", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 220, AC: 19, AttackBonus: 12,
			Weapon: &WeaponProfile{DamageCount: 2, DamageSides: 8}, AbilityModForDamage: 5, WeaponProficient: true,
		},
	}
	weak := Combatant{
		Name: "Novice", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 45, AC: 11, AttackBonus: 3,
			Weapon: &WeaponProfile{DamageCount: 1, DamageSides: 4}, AbilityModForDamage: 0, WeaponProficient: true,
		},
	}

	for i := uint64(0); i < 50; i++ {
		if r := resolveDuel(strong, weak, duelSeed(i)); r.draw || !r.challengerWins {
			t.Fatalf("seed %d: strong-as-challenger should win, got %+v", i, r)
		}
		if r := resolveDuel(weak, strong, duelSeed(i)); r.draw || r.challengerWins {
			t.Fatalf("seed %d: strong-as-challenged should win, got %+v", i, r)
		}
	}
}

// TestResolveDuel_IdenticalFightersAreUnbiased is the fairness guarantee that
// justifies running both orientations: with two identical builds neither the
// challenger nor the challenged seat may have a systematic edge. A single-
// orientation model (attacker uses full kit, defender is a stat block) would
// skew hard toward the challenger; the both-orientations margin cancels it.
func TestResolveDuel_IdenticalFightersAreUnbiased(t *testing.T) {
	fighter := Combatant{
		Name: "Mirror", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 110, AC: 15, AttackBonus: 7,
			Weapon: &WeaponProfile{DamageCount: 1, DamageSides: 8}, AbilityModForDamage: 3, WeaponProficient: true,
		},
	}

	const n = 1200
	challengerWins, draws := 0, 0
	for i := uint64(0); i < n; i++ {
		r := resolveDuel(fighter, fighter, duelSeed(i))
		switch {
		case r.draw:
			draws++
		case r.challengerWins:
			challengerWins++
		}
	}
	decided := n - draws
	if decided == 0 {
		t.Fatal("all identical-fighter duels drew — resolver isn't discriminating")
	}
	// Unbiased by construction (challenger wins iff D1 > D2 for iid D); allow a
	// generous ±10% band so this stays a bias check, not a flaky coin-count.
	rate := float64(challengerWins) / float64(decided)
	if rate < 0.40 || rate > 0.60 {
		t.Fatalf("challenger win rate %.3f over %d decided duels — systematic bias (want ~0.5)", rate, decided)
	}
}

// TestClaimDuelChallenge pins the serialization primitive that guards every
// euro path: exactly one caller may claim a challenge, so accept/decline/expire
// can never both move money for the same row.
func TestClaimDuelChallenge(t *testing.T) {
	newDuelTestDB(t)
	ch := &advDuelChallenge{
		ChallengeID:  "duel-1",
		ChallengerID: id.UserID("@a:x"),
		ChallengedID: id.UserID("@b:x"),
		Stake:        1000,
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
	}
	insertDuelChallenge(ch)

	if pendingDuelForChallenged(id.UserID("@b:x")) == nil {
		t.Fatal("challenge should be pending for the challenged player before any claim")
	}
	if !duelInvolves(id.UserID("@a:x")) || !duelInvolves(id.UserID("@b:x")) {
		t.Fatal("duelInvolves should see both parties while the challenge is live")
	}

	if !claimDuelChallenge("duel-1") {
		t.Fatal("first claim must succeed")
	}
	if claimDuelChallenge("duel-1") {
		t.Fatal("second claim must fail — the row is gone, so no double refund/payout")
	}
	if pendingDuelForChallenged(id.UserID("@b:x")) != nil {
		t.Fatal("claimed challenge must no longer be pending")
	}
}

func TestLoadExpiredDuelChallenges(t *testing.T) {
	newDuelTestDB(t)
	insertDuelChallenge(&advDuelChallenge{
		ChallengeID: "live", ChallengerID: "@a:x", ChallengedID: "@b:x",
		Stake: 500, ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	insertDuelChallenge(&advDuelChallenge{
		ChallengeID: "stale", ChallengerID: "@c:x", ChallengedID: "@d:x",
		Stake: 500, ExpiresAt: time.Now().UTC().Add(-time.Hour),
	})
	got := loadExpiredDuelChallenges()
	if len(got) != 1 || got[0].ChallengeID != "stale" {
		t.Fatalf("loadExpiredDuelChallenges = %+v, want only the stale one", got)
	}
}

// TestSynthDuelEnemy_PortsDefenseAndProcs pins the fidelity carry-overs added
// after review: a tank's DamageReduct rides onto the synth enemy, and companion
// procs fold their expected damage into its round Attack.
func TestSynthDuelEnemy_PortsDefenseAndProcs(t *testing.T) {
	pc := Combatant{
		Stats: CombatStats{
			MaxHP: 100, AC: 15, AttackBonus: 6,
			Weapon: &WeaponProfile{DamageCount: 1, DamageSides: 8}, AbilityModForDamage: 2, // 6.5/hit
		},
		Mods: CombatModifiers{
			DamageReduct:     0.6,                     // Barbarian-style mitigation
			SpiritWeaponProc: 0.5, SpiritWeaponDmg: 4, // expected 0.5×(4+3)=3.5/round
		},
	}
	e := synthDuelEnemy(pc)
	if e.Mods.DamageReduct != 0.6 {
		t.Errorf("DamageReduct not ported: got %v, want 0.6", e.Mods.DamageReduct)
	}
	// One swing (6.5) + proc expectation (3.5) = 10 → rounded 10.
	if e.Stats.Attack != 10 {
		t.Errorf("folded Attack = %d, want 10 (6.5 weapon + 3.5 proc)", e.Stats.Attack)
	}
}

func TestParseDuelStake(t *testing.T) {
	p := &AdventurePlugin{}
	const level = 5 // cap = 2500

	// No stake → default to the cap.
	if got, err := p.parseDuelStake([]string{"@u"}, level); err != nil || got != 2500 {
		t.Fatalf("default stake = (%d, %v), want (2500, nil)", got, err)
	}
	// Explicit in-band stake, with and without the € prefix.
	if got, err := p.parseDuelStake([]string{"@u", "€1500"}, level); err != nil || got != 1500 {
		t.Fatalf("€1500 stake = (%d, %v), want (1500, nil)", got, err)
	}
	// Over the cap is rejected.
	if _, err := p.parseDuelStake([]string{"@u", "9999"}, level); err == nil {
		t.Error("stake above cap should error")
	}
	// Below the floor is rejected.
	if _, err := p.parseDuelStake([]string{"@u", "10"}, level); err == nil {
		t.Error("stake below the minimum should error")
	}
	// Garbage is rejected.
	if _, err := p.parseDuelStake([]string{"@u", "lots"}, level); err == nil {
		t.Error("non-numeric stake should error")
	}
}
