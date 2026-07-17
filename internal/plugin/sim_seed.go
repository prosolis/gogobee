package plugin

import (
	"encoding/hex"
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Deterministic sim seeding — OFF by default, so the production binary is
// byte-identical in behaviour. The expedition-sim harness calls SeedSim once at
// subprocess startup to make a run reproducible: the three nondeterminism seams
// that dominate outcome variance — the zone-layout RNG, the run id (traps hash
// off it), and each combat SessionID (the whole turn engine hashes from it) —
// all derive from a single base seed via a process-local counter.
//
// This does NOT touch the peripheral top-level math/rand/v2 procs (pardon rolls,
// minor damage jitter, ambient events); those stay random. Seeding the three
// dominant seams collapses the batch-to-batch drift (identical dungeons + combat
// dice across arms) so an A/B passive change reads at ~0 noise. If a residual
// proc ever proves load-bearing, seed it too — but validate empirically first.
//
// Ordering contract: within one subprocess an expedition is resolved on a single
// goroutine, so nextSimSeed() is drawn in a deterministic order (zone rng, run
// id, then one per combat session in creation order). Two arms sharing a base
// seed draw identical values up to the point a passive change diverges them —
// and SessionIDs are assigned at session *creation*, before a fight resolves, so
// the Nth combat pairs regardless of how the fight plays out.
var (
	simSeedActive atomic.Bool
	simSeedBase   uint64
	simSeedCtr    atomic.Uint64
)

// simCombatRand is an INDEPENDENT seeded stream for the outcome-decisive
// in-combat rolls that the three dominant seams don't cover: the spell
// attack/save/damage d20s (dnd_spell_combat.go), the 33% pardon death-cheat
// (combat_bridge.go), and the short-rest heal die (dnd_rest.go). These fire
// disproportionately in caster / borderline-boss runs — exactly the population
// being certified — and leaving them on the global generator was the main
// source of the per-cell residual (the fighter+ranger repro never exercised
// them, so it read ~0 noise while bard swung 8pp on identical code).
//
// It is a SEPARATE stream from nextSimSeed()'s counter so it never perturbs the
// zone-layout / run-id / SessionID draw order the martial repro validated.
// Within a subprocess an expedition runs on one goroutine; the mutex is belt-
// and-braces so a stray concurrent draw can't race, not a correctness crutch.
var (
	simCombatMu   sync.Mutex
	simCombatRand *rand.Rand
)

// SeedSim activates deterministic seeding for this process. A negative seed
// disables it (the default). Call once at startup, before any expedition runs —
// it is not safe to toggle while a run is in flight.
func SeedSim(seed int64) {
	if seed < 0 {
		simSeedActive.Store(false)
		simCombatRand = nil
		return
	}
	simSeedBase = uint64(seed)
	simSeedCtr.Store(0)
	// 0x5EED5 gives the peripheral-combat stream a distinct sub-stream from the
	// seam counter so the two never correlate or share draws.
	simCombatRand = rand.New(rand.NewPCG(uint64(seed), 0x5EED5))
	simSeedActive.Store(true)
}

func simSeedOn() bool { return simSeedActive.Load() }

// nextSimSeed returns the next counter-mixed seed. Golden-ratio odd multiplier
// decorrelates successive draws.
func nextSimSeed() uint64 {
	n := simSeedCtr.Add(1)
	return simSeedBase ^ (n * 0x9E3779B97F4A7C15)
}

// simHexToken renders one seeded draw as the same 16-char hex shape the
// crypto-random id helpers produce.
func simHexToken() string {
	v := nextSimSeed()
	var b [8]byte
	for i := range b {
		b[i] = byte(v >> (8 * i))
	}
	return hex.EncodeToString(b[:])
}

// simZoneRNG returns a deterministic generator for one zone layout.
func simZoneRNG() *rand.Rand {
	return rand.New(rand.NewPCG(nextSimSeed(), 0xC0FFEE))
}

// simIntN / simFloat64 draw an outcome-decisive in-combat roll from the seeded
// peripheral stream when seeding is active; otherwise they fall through to the
// global generator so the production binary stays byte-identical (prod never
// calls SeedSim, so simSeedOn() is always false there).
func simIntN(n int) int {
	if !simSeedOn() {
		return rand.IntN(n)
	}
	simCombatMu.Lock()
	defer simCombatMu.Unlock()
	return simCombatRand.IntN(n)
}

func simFloat64() float64 {
	if !simSeedOn() {
		return rand.Float64()
	}
	simCombatMu.Lock()
	defer simCombatMu.Unlock()
	return simCombatRand.Float64()
}
