package plugin

import (
	"fmt"
)

// Phase 12 E1b — Supply procurement, daily burn, and depletion effects.
// Spec: gogobee_expedition_system.md §4.

// Pack pricing/yield. §4.2.
const (
	SupplyPackStandardSU    = 10
	SupplyPackStandardCoins = 50
	SupplyPackStandardMax   = 3 // per expedition

	SupplyPackDeluxeSU    = 20
	SupplyPackDeluxeCoins = 90
	SupplyPackDeluxeMax   = 1 // per expedition

	SupplyForageMaxSU = 4 // 1d4 cap (Ranger, WIS DC 12) — §4.2
)

// supplyDailyBurn returns the base SU/day for a zone tier (§4.1).
// Tier 1: 1, Tier 2: 1.5, Tier 3: 2, Tier 4: 3, Tier 5: 4.
func supplyDailyBurn(tier ZoneTier) float32 {
	switch tier {
	case ZoneTierBeginner:
		return 1.0
	case ZoneTierApprentice:
		return 1.5
	case ZoneTierJourneyman:
		return 2.0
	case ZoneTierVeteran:
		return 3.0
	case ZoneTierLegendary:
		return 4.0
	}
	return 1.0
}

// supplyHarshMultiplier returns the harsh-conditions multiplier (§4.1).
// Tier 1×1, Tier 2×1.5, Tier 3×2, Tier 4×2.5, Tier 5×3.
func supplyHarshMultiplier(tier ZoneTier) float32 {
	switch tier {
	case ZoneTierBeginner:
		return 1.0
	case ZoneTierApprentice:
		return 1.5
	case ZoneTierJourneyman:
		return 2.0
	case ZoneTierVeteran:
		return 2.5
	case ZoneTierLegendary:
		return 3.0
	}
	return 1.0
}

// SupplyDepletionState classifies remaining supply ratio (§4.3).
type SupplyDepletionState int

const (
	SupplyNormal SupplyDepletionState = iota
	SupplyRationing
	SupplySevereRationing
	SupplyStarvation
)

// supplyDepletion returns the depletion state for a supplies snapshot.
// Brackets: >25% normal, 10–25% rationing, 1–9% severe, 0 starvation.
func supplyDepletion(s ExpeditionSupplies) SupplyDepletionState {
	if s.Max <= 0 {
		return SupplyNormal
	}
	if s.Current <= 0 {
		return SupplyStarvation
	}
	pct := (s.Current / s.Max) * 100
	switch {
	case pct >= 25:
		return SupplyNormal
	case pct >= 10:
		return SupplyRationing
	default:
		return SupplySevereRationing
	}
}

// supplyRollModifier maps depletion state to the universal roll penalty
// applied to attack/skill rolls (§4.3). Starvation has its own CON bleed
// handled elsewhere; the roll mod for starvation matches severe rationing
// floor so combat math doesn't divide by zero.
func supplyRollModifier(state SupplyDepletionState) int {
	switch state {
	case SupplyRationing:
		return -1
	case SupplySevereRationing, SupplyStarvation:
		return -2
	}
	return 0
}

// supplyAllowsLongRest returns false when severe rationing (§4.3) blocks
// long rests. Starvation also blocks (forced extraction is the next step).
func supplyAllowsLongRest(state SupplyDepletionState) bool {
	return state == SupplyNormal || state == SupplyRationing
}

// SupplyPurchase represents an expedition outfitting choice.
type SupplyPurchase struct {
	StandardPacks int
	DeluxePacks   int
}

// Total SU yield from a purchase.
func (p SupplyPurchase) Total() float32 {
	return float32(p.StandardPacks*SupplyPackStandardSU + p.DeluxePacks*SupplyPackDeluxeSU)
}

// Total cost in coins.
func (p SupplyPurchase) Cost() int {
	return p.StandardPacks*SupplyPackStandardCoins + p.DeluxePacks*SupplyPackDeluxeCoins
}

// Validate enforces §4.2 caps (max 3 standard, max 1 deluxe, no negatives,
// at least one pack purchased — an expedition without supplies is not a
// legal start).
func (p SupplyPurchase) Validate() error {
	if p.StandardPacks < 0 || p.DeluxePacks < 0 {
		return fmt.Errorf("supply pack counts must be non-negative")
	}
	if p.StandardPacks > SupplyPackStandardMax {
		return fmt.Errorf("standard packs capped at %d (got %d)", SupplyPackStandardMax, p.StandardPacks)
	}
	if p.DeluxePacks > SupplyPackDeluxeMax {
		return fmt.Errorf("deluxe packs capped at %d (got %d)", SupplyPackDeluxeMax, p.DeluxePacks)
	}
	if p.StandardPacks == 0 && p.DeluxePacks == 0 {
		return fmt.Errorf("expedition requires at least one supply pack")
	}
	return nil
}

// makeSupplies builds an ExpeditionSupplies from a purchase, sized for the
// given zone tier. Harsh-conditions multiplier defaults to 1 (off);
// upgraded by E2/E3 events.
func makeSupplies(tier ZoneTier, p SupplyPurchase) ExpeditionSupplies {
	total := p.Total()
	return ExpeditionSupplies{
		Current:       total,
		Max:           total,
		DailyBurn:     supplyDailyBurn(tier),
		HarshMod:      1.0,
		PacksStandard: p.StandardPacks,
		PacksDeluxe:   p.DeluxePacks,
	}
}

// applyDailyBurn deducts one day's supplies from the snapshot (caller
// persists). Returns the new snapshot and the SU consumed.
//
// Multiplier precedence (§4.1, §8.3):
//   - siege overrides everything with a hard 2× floor (even for tier 1
//     where HarshMod is 1×) — the dungeon is actively starving you out.
//   - otherwise, harshActive applies HarshMod (zone-tier scaled).
func applyDailyBurn(s ExpeditionSupplies, harshActive, siege bool) (ExpeditionSupplies, float32) {
	return applyDailyBurnP(s, harshActive, siege, 0)
}

// applyDailyBurnP is the rate-parameterized form used by the Phase 3-B
// sim harness lever sweep. burnRatePct == 0 means "use live" (100%);
// any positive value scales the final per-day burn by that percent
// (e.g. 50 = half burn). Live callers always go through applyDailyBurn.
// See gogobee_expedition_difficulty.md Phase 3-B.
func applyDailyBurnP(s ExpeditionSupplies, harshActive, siege bool, burnRatePct int) (ExpeditionSupplies, float32) {
	burn := s.DailyBurn
	switch {
	case siege:
		mult := s.HarshMod
		if mult < 2 {
			mult = 2
		}
		burn *= mult
	case harshActive:
		mult := s.HarshMod
		if mult <= 0 {
			mult = 1
		}
		burn *= mult
	}
	if burnRatePct > 0 {
		burn = burn * float32(burnRatePct) / 100
	}
	s.Current -= burn
	if s.Current < 0 {
		s.Current = 0
	}
	// Reset per-day forage flag.
	s.ForagedToday = false
	return s, burn
}
