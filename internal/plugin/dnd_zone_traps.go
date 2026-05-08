package plugin

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strings"
)

// Phase 11 D2a — Tier 1 trap catalog. Implements `gogobee_dungeon_zones.md`
// §6 (Environmental Traps) for tier 1, replacing the flat-percent HP nick
// shipped in D1e with proper trap kinds.
//
// Each trap defines a detect skill + DC, optional damage dice, and an
// "alerts" flag for tripwire-style hazards that don't deal damage. Higher
// tiers add their own trap pools in later sub-phases (D3a/D4a).
//
// Selection is deterministic on (runID, roomIdx, "trap") so re-reading
// !zone status after a trap room resolves yields the same trap.

type ZoneTrapKind string

const (
	TrapPit            ZoneTrapKind = "pit"
	TrapTripwireAlarm  ZoneTrapKind = "tripwire_alarm"
	TrapPoisonDart     ZoneTrapKind = "poison_dart"
	TrapFlameJet       ZoneTrapKind = "flame_jet"
	TrapCollapsingCeil ZoneTrapKind = "collapsing_ceiling"
	TrapGlyphOfWarding ZoneTrapKind = "glyph_of_warding"
	TrapAntimagicField ZoneTrapKind = "antimagic_field"
	TrapNecroticMiasma ZoneTrapKind = "necrotic_miasma"
	TrapSymbolOfDeath  ZoneTrapKind = "symbol_of_death"
	TrapPlanarRift     ZoneTrapKind = "planar_rift"
)

// zoneTrapDef — static definition of a zone trap. Damage dice are in
// (count, sides) form; AlertsEnemies flags traps that have no damage
// but raise the alarm. Weight biases the room-level random pick.
type zoneTrapDef struct {
	Kind          ZoneTrapKind
	Display       string
	Trigger       string  // short flavor descriptor for the trip narration
	DetectSkill   DnDSkill
	DetectDC      int
	DamageDiceN   int     // 0 if no direct damage
	DamageDiceD   int     // sides
	DamageType      string // "bludgeoning", "piercing", "poison", ...
	AlertsEnemies   bool   // tripwire-class: 0 damage, raises alarm
	SuppressesMagic bool   // antimagic-class: 0 damage, narrative suppression
	Tier            ZoneTier
	Weight          int    // 1..10, default 5
}

// tier1TrapCatalog — design doc §6 Tier 1 table.
var tier1TrapCatalog = []zoneTrapDef{
	{
		Kind:        TrapPit,
		Display:     "Pit Trap",
		Trigger:     "the floor gives way",
		DetectSkill: SkillPerception,
		DetectDC:    12,
		DamageDiceN: 2,
		DamageDiceD: 6,
		DamageType:  "bludgeoning",
		Tier:        ZoneTierBeginner,
		Weight:      5,
	},
	{
		Kind:          TrapTripwireAlarm,
		Display:       "Tripwire Alarm",
		Trigger:       "a wire snags your ankle and a clatter of bells erupts",
		DetectSkill:   SkillPerception,
		DetectDC:      10,
		AlertsEnemies: true,
		Tier:          ZoneTierBeginner,
		Weight:        4,
	},
	{
		Kind:        TrapPoisonDart,
		Display:     "Poison Dart",
		Trigger:     "a dart hisses out of a hole you didn't see",
		DetectSkill: SkillInvestigation,
		DetectDC:    12,
		DamageDiceN: 2, // 1d4 piercing + 1d4 simulated poison flattened to 2d4
		DamageDiceD: 4,
		DamageType:  "piercing+poison",
		Tier:        ZoneTierBeginner,
		Weight:      3,
	},
}

// tier2TrapCatalog — design doc §6 Tier 2 table. Save mechanics ("DEX DC X
// half") are not modeled at this layer; full dice resolve on a failed
// detect, matching the Tier 1 convention.
var tier2TrapCatalog = []zoneTrapDef{
	{
		Kind:        TrapFlameJet,
		Display:     "Flame Jet",
		Trigger:     "a pressure plate sinks under your boot and gouts of flame roar up",
		DetectSkill: SkillPerception,
		DetectDC:    14,
		DamageDiceN: 3,
		DamageDiceD: 6,
		DamageType:  "fire",
		Tier:        ZoneTierApprentice,
		Weight:      5,
	},
	{
		Kind:        TrapCollapsingCeil,
		Display:     "Collapsing Ceiling",
		Trigger:     "a rigged support snaps and a slab of ceiling crashes down",
		DetectSkill: SkillInvestigation,
		DetectDC:    14,
		DamageDiceN: 4,
		DamageDiceD: 6,
		DamageType:  "bludgeoning",
		Tier:        ZoneTierApprentice,
		Weight:      4,
	},
	{
		Kind:        TrapGlyphOfWarding,
		Display:     "Glyph of Warding",
		Trigger:     "a sigil flares on the doorframe and arcs of lightning lash out",
		DetectSkill: SkillArcana,
		DetectDC:    14,
		DamageDiceN: 5,
		DamageDiceD: 8,
		DamageType:  "lightning",
		Tier:        ZoneTierApprentice,
		Weight:      3,
	},
}

// tier3TrapCatalog — design doc §6 Tier 3–5 table. The doc lumps all
// high-tier traps into a single list, so tiers 3, 4, and 5 share this
// pool. Two of the four entries (Antimagic Field, Symbol of Death) have
// effects that don't fit the dice-only damage model:
//
//   - Antimagic Field: no damage; SuppressesMagic flag flips the
//     narration to a non-damaging suppression beat. The actual
//     spell-suppression mechanic is left to a later phase — for now
//     the line tells the player magic is choked in this room.
//   - Necrotic Miasma: design doc says "6d6 + max HP -10 until long
//     rest". The max-HP debuff isn't modeled here; trap delivers the
//     6d6 necrotic damage only.
//   - Symbol of Death: doc reads "INT DC 20 or drop to 0 HP". We
//     approximate with 8d10 psychic; the existing near-KO cap in
//     applyTrapEffectWithDetect prevents an outright kill.
//   - Planar Rift: 4d10 psychic. The teleport-to-random-room mechanic
//     isn't modeled — trap just deals psychic damage for now.
var tier3TrapCatalog = []zoneTrapDef{
	{
		Kind:            TrapAntimagicField,
		Display:         "Antimagic Field",
		Trigger:         "the air thickens and every charm at your belt goes cold",
		DetectSkill:     SkillArcana,
		DetectDC:        18,
		SuppressesMagic: true,
		Tier:            ZoneTierJourneyman,
		Weight:          3,
	},
	{
		Kind:        TrapNecroticMiasma,
		Display:     "Necrotic Miasma",
		Trigger:     "the sarcophagus lid shifts and a green-black fume pours out",
		DetectSkill: SkillPerception,
		DetectDC:    16,
		DamageDiceN: 6,
		DamageDiceD: 6,
		DamageType:  "necrotic",
		Tier:        ZoneTierJourneyman,
		Weight:      5,
	},
	{
		Kind:        TrapSymbolOfDeath,
		Display:     "Symbol of Death",
		Trigger:     "an inscribed sigil flares and your vision blanks to white",
		DetectSkill: SkillInvestigation,
		DetectDC:    20,
		DamageDiceN: 8,
		DamageDiceD: 10,
		DamageType:  "psychic",
		Tier:        ZoneTierJourneyman,
		Weight:      2,
	},
	{
		Kind:        TrapPlanarRift,
		Display:     "Planar Rift",
		Trigger:     "the floor folds wrong and a tear of starless dark grasps at you",
		DetectSkill: SkillArcana,
		DetectDC:    20,
		DamageDiceN: 4,
		DamageDiceD: 10,
		DamageType:  "psychic",
		Tier:        ZoneTierJourneyman,
		Weight:      3,
	},
}

// trapsByTier returns all traps registered at the given tier. Tiers 3,
// 4, and 5 share the Tier 3 catalog per the design doc's combined
// "Tier 3–5 Traps" table; later phases may split them.
func trapsByTier(t ZoneTier) []zoneTrapDef {
	switch t {
	case ZoneTierBeginner:
		return tier1TrapCatalog
	case ZoneTierApprentice:
		return tier2TrapCatalog
	case ZoneTierJourneyman, ZoneTierVeteran, ZoneTierLegendary:
		return tier3TrapCatalog
	}
	return nil
}

// pickZoneTrap selects a trap deterministically from the zone's tier
// catalog based on (runID, roomIdx). Returns ok=false if no traps are
// registered for the tier (defensive — Tier 1 always has entries).
func pickZoneTrap(zone ZoneDefinition, runID string, roomIdx int) (zoneTrapDef, bool) {
	pool := trapsByTier(zone.Tier)
	if len(pool) == 0 {
		return zoneTrapDef{}, false
	}
	total := 0
	for _, tr := range pool {
		w := tr.Weight
		if w <= 0 {
			w = 5
		}
		total += w
	}
	roll := int(zoneTrapSelectorHash(runID, roomIdx) % uint64(total))
	cum := 0
	for _, tr := range pool {
		w := tr.Weight
		if w <= 0 {
			w = 5
		}
		cum += w
		if roll < cum {
			return tr, true
		}
	}
	return pool[len(pool)-1], true
}

// zoneTrapSelectorHash — separate namespace from enemy/loot hashes so a
// trap pick on the same (run, room) doesn't collide with their seeds.
func zoneTrapSelectorHash(runID string, roomIdx int) uint64 {
	h := fnv.New64a()
	h.Write([]byte("trap:"))
	h.Write([]byte(runID))
	var sb [8]byte
	for i := 0; i < 8; i++ {
		sb[i] = byte(roomIdx >> (8 * i))
	}
	h.Write(sb[:])
	return h.Sum64()
}

// rollTrapDamage rolls a trap's damage dice with a seeded RNG so the
// same (run, room) yields the same number each read. Returns 0 for
// alert-only traps (Tripwire). DamageType "piercing+poison" is treated
// as a single combined dice pool — we don't model the Poisoned condition
// here; the design-doc CON DC 11 save is folded into the same dice pool
// as a flattened average.
func rollTrapDamage(tr zoneTrapDef, runID string, roomIdx int) int {
	if tr.AlertsEnemies || tr.SuppressesMagic || tr.DamageDiceN == 0 || tr.DamageDiceD == 0 {
		return 0
	}
	rng := rand.New(rand.NewPCG(zoneTrapSelectorHash(runID, roomIdx), 0x712D2A))
	total := 0
	for i := 0; i < tr.DamageDiceN; i++ {
		total += rng.IntN(tr.DamageDiceD) + 1
	}
	return total
}

// trapDamageHeader builds the user-facing one-liner for a damaging trap
// trip — "💢 Pit Trap (2d6 bludgeoning) — 9 HP". Pure formatter.
func trapDamageHeader(tr zoneTrapDef, dmg int, hpCur, hpMax int) string {
	if tr.AlertsEnemies {
		return fmt.Sprintf("🔔 **%s** — %s. The next room knows you're here.", tr.Display, tr.Trigger)
	}
	if tr.SuppressesMagic {
		return fmt.Sprintf("🚫 **%s** — %s. Spells fizzle and enchanted gear sleeps until you leave the room.", tr.Display, tr.Trigger)
	}
	dice := fmt.Sprintf("%dd%d", tr.DamageDiceN, tr.DamageDiceD)
	return fmt.Sprintf("💢 **%s** (%s %s) — **%d HP** (%d/%d remaining).",
		tr.Display, dice, tr.DamageType, dmg, hpCur, hpMax)
}

// trapSpottedHeader — narration for a successful detection roll. No
// damage; the player gets a beat of competence flavor.
func trapSpottedHeader(tr zoneTrapDef, res SkillCheckResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("👁️ **%s spotted** — ", tr.Display))
	info, _ := skillInfo(tr.DetectSkill)
	b.WriteString(fmt.Sprintf("%s check %d vs DC %d.", info.Display, res.Total, tr.DetectDC))
	return b.String()
}
