package plugin

import (
	"math"
	"sort"
)

// Weighted race-balance model. Drives the race-mod tuning in dnd.go
// (see gogobee_dnd_design_doc and the project memory).
//
// Flat net ability mods are NOT equal *effective* power: a +1 in a stat a
// build actually uses is worth far more than a +1 in a dump stat. This
// model scores each race by weighting its mods against a blend of (a) the
// combat stat priorities of each class and (b) a class-independent
// "utility" weight for how much each stat does *outside* combat — zone
// locks, expedition harvest, skill checks, haggling. See classStatWeights
// and the combatUtilityBlend constant.
//
// Every weight block is normalized to sum to 6.0 — so a perfectly flat race
// (+1 to every stat) scores exactly its net mod total (6.0) under ANY
// class. A spiky race scores above baseline with a class that wants its
// high stats, below with one that doesn't. The best-fit score is therefore
// the race's realistic effective-power ceiling.

// ability score indices into the [6]int Mods block.
const (
	statSTR = 0
	statDEX = 1
	statCON = 2
	statINT = 3
	statWIS = 4
	statCHA = 5
)

func statIndex(name string) int {
	switch name {
	case "STR":
		return statSTR
	case "DEX":
		return statDEX
	case "CON":
		return statCON
	case "INT":
		return statINT
	case "WIS":
		return statWIS
	case "CHA":
		return statCHA
	}
	return -1
}

// normalizeWeights scales a raw weight block so it sums to 6.0 — the point
// where a flat +1-to-all race scores its net mod total (6.0) under it.
func normalizeWeights(w [6]float64) [6]float64 {
	var sum float64
	for _, v := range w {
		sum += v
	}
	if sum == 0 {
		return w
	}
	for i := range w {
		w[i] = w[i] * 6 / sum
	}
	return w
}

// combatStatWeights returns the per-class *combat* weight block. Raw role
// weights, before normalization:
//
//	primary attack stat (PrimaryA) ... 4
//	secondary stat (PrimaryB) ........ 2
//	CON (survivability) .............. 3
//	everything else (saves/skills) ... 1
//
// CON is weighted highly for every class — HP keeps you in the fight
// regardless of build. Where a class's PrimaryA/PrimaryB *is* CON, the
// higher weight wins (max, not sum).
func combatStatWeights(c DnDClassInfo) [6]float64 {
	var w [6]float64
	for i := range w {
		w[i] = 1 // floor: saves, skills, utility
	}
	if w[statCON] < 3 {
		w[statCON] = 3 // survivability matters for every class
	}
	if i := statIndex(c.PrimaryB); i >= 0 && w[i] < 2 {
		w[i] = 2
	}
	if i := statIndex(c.PrimaryA); i >= 0 && w[i] < 4 {
		w[i] = 4
	}
	return normalizeWeights(w)
}

// utilityStatWeights is the class-independent *non-combat* weight block:
// how much each stat does outside a fight. Derived from a codebase audit
// of where ability mods drive non-combat outcomes — zone-navigation stat
// locks (zone_graph_nav.go), expedition harvest yields (dnd_expedition_
// harvest.go), night/transit checks, the 11-skill system with its NPC
// cost-refund hooks (dnd_skills.go), and CHA hospital-bill haggling
// (adventure_hospital.go). Raw weights, before normalization:
//
//	STR ... 2.0  (one zone lock, mining harvest)
//	DEX ... 3.0  (two zone locks, fishing harvest, AC)
//	CON ... 5.0  (two locks, short-rest healing, level-up HP, night checks)
//	INT ... 3.5  (one lock, Arcana/Investigation, scavenge + essence harvest)
//	WIS ... 5.0  (one lock, Perception/Insight, healing, forage + commune, night)
//	CHA ... 4.0  (three locks, Persuasion/Intimidation/Deception, haggling)
func utilityStatWeights() [6]float64 {
	return normalizeWeights([6]float64{2.0, 3.0, 5.0, 3.5, 5.0, 4.0})
}

// combatUtilityBlend is how much a character's effective power comes from
// combat vs. everything else (zones, expeditions, harvest, skills,
// haggling). gogobee is combat-forward but not combat-only.
const combatUtilityBlend = 0.6

// classStatWeights is the blended per-class weight block used for scoring:
// combatUtilityBlend of the class's combat weights plus the remainder of
// the class-independent utility weights. Both components are normalized to
// sum 6.0, so the blend is too — a flat +1-to-all race still scores exactly
// 6.0 under every class.
func classStatWeights(c DnDClassInfo) [6]float64 {
	combat := combatStatWeights(c)
	utility := utilityStatWeights()
	var w [6]float64
	for i := range w {
		w[i] = combatUtilityBlend*combat[i] + (1-combatUtilityBlend)*utility[i]
	}
	return w
}

// raceScore is the weighted effective-power of a race under one class:
// Σ mod[i] * weight[i].
func raceScore(mods [6]int, w [6]float64) float64 {
	var s float64
	for i := 0; i < 6; i++ {
		s += float64(mods[i]) * w[i]
	}
	return s
}

// raceBalanceBaseline is the Standard Human score under every class:
// +1 to all six stats × weights summing to 6.0.
const raceBalanceBaseline = 6.0

// raceBalance is one race's result in the weighted balance pass.
type raceBalance struct {
	Race       DnDRace
	BestClass  DnDClass
	BestScore  float64 // effective-power ceiling (best-fit class)
	WorstClass DnDClass
	WorstScore float64 // effective-power floor (worst-fit class)
	AvgScore   float64 // mean across all playable classes
}

// Delta is the race's best-fit score minus the Standard Human baseline.
// Positive = over-performs a flat +1-to-all race at its optimal build.
func (rb raceBalance) Delta() float64 { return rb.BestScore - raceBalanceBaseline }

// computeRaceBalance scores every race's mods against every playable class
// and reports its best-fit, worst-fit, and average effective power.
// Results are sorted by Delta descending — biggest over-performers first.
func computeRaceBalance() []raceBalance {
	out := make([]raceBalance, 0, len(dndRaces))
	for _, ri := range dndRaces {
		rb := raceBalance{Race: ri.Key, WorstScore: math.Inf(1)}
		var total float64
		var n int
		first := true
		for _, ci := range dndClasses {
			if !ci.Playable {
				continue
			}
			s := raceScore(ri.Mods, classStatWeights(ci))
			if first || s > rb.BestScore {
				rb.BestScore, rb.BestClass = s, ci.Key
			}
			if s < rb.WorstScore {
				rb.WorstScore, rb.WorstClass = s, ci.Key
			}
			total += s
			n++
			first = false
		}
		if n > 0 {
			rb.AvgScore = total / float64(n)
		}
		out = append(out, rb)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Delta() > out[j].Delta()
	})
	return out
}
