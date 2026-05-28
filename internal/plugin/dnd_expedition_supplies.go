package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Phase 12 E1b — Supply procurement, daily burn, and depletion effects.
// Spec: gogobee_expedition_system.md §4.

// Pack pricing/yield. §4.2.
const (
	SupplyPackStandardSU    = 10
	SupplyPackStandardCoins = 50

	SupplyPackDeluxeSU    = 20
	SupplyPackDeluxeCoins = 90

	SupplyForageMaxSU = 4 // 1d4 cap (Ranger, WIS DC 12) — §4.2
)

// supplyPackCaps returns the per-tier maximum standard and deluxe pack
// counts a player can buy for an expedition. D5-a: caps now scale by
// zone tier so a T5 loadout actually clears DailyBurn(raw) × intended
// days × harsh-multiplier — see gogobee_long_expedition_plan.md §D5.
// Intended-day anchors come from the §2 target table (T1=2 → T5=7).
func supplyPackCaps(tier ZoneTier) (standard, deluxe int) {
	switch tier {
	case ZoneTierBeginner:
		return 2, 1
	case ZoneTierApprentice:
		return 2, 1
	case ZoneTierJourneyman:
		return 3, 1
	case ZoneTierVeteran:
		return 5, 1
	case ZoneTierLegendary:
		return 7, 2
	}
	return 3, 1
}

// SupplyLoadout names a tier-scaled preset purchase. D5-b: empty-arg
// `!expedition start <zone>` now prompts the player to pick one of these
// rather than silently defaulting to 1 standard pack. Raw `Ns Md` syntax
// remains the advanced override.
type SupplyLoadout int

const (
	LoadoutLean SupplyLoadout = iota
	LoadoutBalanced
	LoadoutHeavy
)

// loadoutPurchase returns the SupplyPurchase for a preset at a tier.
// Lean: covers intended days at raw daily burn (cheap, no harsh buffer).
// Balanced: ~harsh-ready for an intended-length run (recommended).
// Heavy: equals supplyPackCaps — the harsh×3 ceiling D5-a sized for.
func loadoutPurchase(tier ZoneTier, l SupplyLoadout) SupplyPurchase {
	switch l {
	case LoadoutHeavy:
		std, dlx := supplyPackCaps(tier)
		return SupplyPurchase{StandardPacks: std, DeluxePacks: dlx}
	case LoadoutBalanced:
		switch tier {
		case ZoneTierBeginner, ZoneTierApprentice:
			return SupplyPurchase{StandardPacks: 2}
		case ZoneTierJourneyman:
			return SupplyPurchase{StandardPacks: 3}
		case ZoneTierVeteran:
			return SupplyPurchase{StandardPacks: 5}
		case ZoneTierLegendary:
			return SupplyPurchase{StandardPacks: 5, DeluxePacks: 1}
		}
		return SupplyPurchase{StandardPacks: 2}
	default: // LoadoutLean
		switch tier {
		case ZoneTierBeginner, ZoneTierApprentice:
			return SupplyPurchase{StandardPacks: 1}
		case ZoneTierJourneyman:
			return SupplyPurchase{StandardPacks: 2}
		case ZoneTierVeteran:
			return SupplyPurchase{StandardPacks: 3}
		case ZoneTierLegendary:
			return SupplyPurchase{StandardPacks: 5}
		}
		return SupplyPurchase{StandardPacks: 1}
	}
}

// parseLoadoutToken returns the preset selected by tok, if any. Accepts
// short forms (l/b/h) and a couple of synonyms. Empty/unknown → false.
func parseLoadoutToken(tok string) (SupplyLoadout, bool) {
	switch strings.ToLower(strings.TrimSpace(tok)) {
	case "lean", "l", "light":
		return LoadoutLean, true
	case "balanced", "b", "bal", "standard":
		return LoadoutBalanced, true
	case "heavy", "h", "max", "full":
		return LoadoutHeavy, true
	}
	return 0, false
}

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

// applyRangerForage grants the Ranger's daily 1d4-SU forage yield (§4.2,
// wired in D5-c). Returns the SU added — 0 for non-Rangers, when the
// player has already foraged today, or when supplies are already at Max.
// The grant is headroom-capped so a Heavy loadout doesn't overflow its
// purchased ceiling. rng is the test-injectable [0,n) source; nil falls
// back to math/rand. Caller owns the persistence.
//
// Sizing rationale: with D5-a caps so generous (Lean T5 = 50 SU vs ~14 SU
// burned over a 7-day intended run at phase5B×0.5), this perk operates
// as a quiet Lean-loadout cushion, not a Heavy multiplier — average +2.5
// SU/day off a Ranger is roughly one extra day of late-stage T5 burn
// across a full expedition.
func applyRangerForage(e *Expedition, c *DnDCharacter, rng func(int) int) float32 {
	if e == nil || c == nil || c.Class != ClassRanger {
		return 0
	}
	if e.Supplies.ForagedToday {
		return 0
	}
	headroom := e.Supplies.Max - e.Supplies.Current
	if headroom <= 0 {
		e.Supplies.ForagedToday = true
		return 0
	}
	var roll int
	if rng != nil {
		roll = rng(SupplyForageMaxSU) + 1
	} else {
		roll = rand.IntN(SupplyForageMaxSU) + 1
	}
	gain := float32(roll)
	if gain > headroom {
		gain = headroom
	}
	e.Supplies.Current += gain
	e.Supplies.ForagedToday = true
	return gain
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

// Validate enforces §4.2 caps (no negatives, at least one pack
// purchased — an expedition without supplies is not a legal start) and
// the per-tier maximums from supplyPackCaps.
func (p SupplyPurchase) Validate(tier ZoneTier) error {
	if p.StandardPacks < 0 || p.DeluxePacks < 0 {
		return fmt.Errorf("supply pack counts must be non-negative")
	}
	stdCap, dlxCap := supplyPackCaps(tier)
	if p.StandardPacks > stdCap {
		return fmt.Errorf("standard packs capped at %d for T%d (got %d)", stdCap, int(tier), p.StandardPacks)
	}
	if p.DeluxePacks > dlxCap {
		return fmt.Errorf("deluxe packs capped at %d for T%d (got %d)", dlxCap, int(tier), p.DeluxePacks)
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
// phase5BDailyBurnRatePct is the shipped daily-burn multiplier from
// Phase 3-B's sweep + Phase 5-B's post-buff re-validation. 50 means
// "half live burn" — needed because the Phase 5-B player power floor
// keeps T4/T5 expeditioners alive long enough that the original 100%
// burn rate starves them out before extraction. See
// gogobee_expedition_difficulty.md Phase 3-B (sweep) and Phase 5-B
// (re-validated under HP buff, shipped).
const phase5BDailyBurnRatePct = 50

func applyDailyBurn(s ExpeditionSupplies, harshActive, siege bool) (ExpeditionSupplies, float32) {
	return applyDailyBurnP(s, harshActive, siege, phase5BDailyBurnRatePct)
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
