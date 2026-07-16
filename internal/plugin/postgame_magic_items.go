package plugin

// Post-game (Tier 6) signature magic items — the ten hand-authored Legendary
// drops from the five Mythic zones. Two per zone; the third zone drop is a
// UniqueAlways crafting anchor (a treasure material, not an equippable), handled
// on the loot side.
//
// These are real magicItemRegistry entries so the loot path can grant them as
// equippable magic_item rows (see zoneLootToInventory) and the equip pipeline
// can resolve them. Their combat numbers live in postgameSignatureEffects, the
// magicItemEffectOverlay layer — bespoke, above the codified Legendary formula,
// because these are meant to be the best-in-slot chase items. Numbers are
// opening bids; the P7 sim sweep tunes them.
//
// The fancier one-shot riders each Note in postgame_zone_defs.go promises
// (survive-a-killing-blow, refund-the-slot, negate-a-stun, auto-evade, …) are
// Layer-2 boss/effect hooks deferred to P8 — encoded here is only what the
// magicItemEffect struct can already express.

// postgameSignatureMagicItems is wired into magicItemOverlay (magic_items.go)
// as a plain var reference, so it resolves before buildMagicItemRegistry() runs.
var postgameSignatureMagicItems = []MagicItem{
	// ── The Ossuary Ascendant ────────────────────────────────────────────────
	{
		ID: "crown_of_the_patient_king", Name: "Crown of the Patient King",
		Kind: MagicItemWondrous, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotHead, Value: 8000,
		Desc: "A thin band of black iron, warm as if just taken off. He respects a good plan; wear it and the fight waits for yours.",
	},
	{
		ID: "valdris_quill", Name: "Valdris's Quill",
		Kind: MagicItemWand, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotMainHand, Value: 8000,
		Desc: "The lich's own writing implement, still wet. Spells drawn with it land where they were aimed and then some.",
	},

	// ── The First Hoard ──────────────────────────────────────────────────────
	{
		ID: "aegis_of_the_first_scale", Name: "Aegis of the First Scale",
		Kind: MagicItemArmor, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotChest, Value: 9000,
		Desc: "A single shed scale of the Ember Before Fire, worked into a breastplate. Fire predates it and still cannot touch it.",
	},
	{
		ID: "emberheart_greatblade", Name: "Emberheart Greatblade",
		Kind: MagicItemWeapon, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotMainHand, Value: 8000,
		Desc: "Forged around a coal that has never cooled. Its edge opens wounds that go on burning after the swing.",
	},

	// ── The Unplace ──────────────────────────────────────────────────────────
	{
		ID: "needle_of_the_seam", Name: "Needle of the Seam",
		Kind: MagicItemWeapon, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotMainHand, Value: 8000,
		Desc: "The Seamstress's own needle, half here and half elsewhere. Spellwork slid along it finds the gaps in everything.",
	},
	{
		ID: "mantle_of_elsewhere", Name: "Mantle of Elsewhere",
		Kind: MagicItemWondrous, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotCloak, Value: 8000,
		Desc: "A cloak cut from a room that isn't. Now and then a blow passes through where you briefly were not.",
	},

	// ── The Drowned Star ─────────────────────────────────────────────────────
	{
		ID: "halo_of_the_sunken_dawn", Name: "Halo of the Sunken Dawn",
		Kind: MagicItemWondrous, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotHead, Value: 8000,
		Desc: "A ring of drowned light that never went out. Mercy carried through it lands heavier on those you mean to keep alive.",
	},
	{
		ID: "tideglass_ward", Name: "Tideglass Ward",
		Kind: MagicItemArmor, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotOffHand, Value: 8000,
		Desc: "A shield of pressure-fused abyssal glass. Once a fight it drinks a wave whole and gives you back the room.",
	},

	// ── The Last Meridian ────────────────────────────────────────────────────
	{
		ID: "the_second_hand", Name: "The Second Hand",
		Kind: MagicItemWeapon, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotMainHand, Value: 8000,
		Desc: "A slim blade that keeps its own time. Now and then it lends you a moment you did not have to swing again.",
	},
	{
		ID: "escapement_plate", Name: "Escapement Plate",
		Kind: MagicItemArmor, Rarity: RarityLegendary, Attunement: true,
		Slot: DnDSlotChest, Value: 8000,
		Desc: "Verdigris plate geared like a clock. The first blow that would stop you finds a moment already held in reserve.",
	},
}

// postgameSignatureEffects is wired into magicItemEffectOverlay
// (magic_items_gameplay.go). DamageReductMult is multiplicative (1.0 neutral,
// <1.0 reduces damage taken); the aggregator ignores a 0.0, so weapons/wondrous
// items that don't reduce damage simply leave it unset. Everything else is
// additive. Legendary codified baselines for reference: weapon DamageBonus 0.25,
// armor DamageReductMult 0.80, wand DamageBonus 0.15 + FlatDmgStart 5, wondrous
// InitiativeBias 2.5 + MaxHP 10 — these sit at or above those.
var postgameSignatureEffects = map[string]magicItemEffect{
	// Ossuary
	"crown_of_the_patient_king": {InitiativeBias: 3.0, MaxHP: 14},
	"valdris_quill":             {DamageBonus: 0.20, FlatDmgStart: 6},
	// First Hoard
	"aegis_of_the_first_scale": {DamageReductMult: 0.72, MaxHP: 10},
	"emberheart_greatblade":    {DamageBonus: 0.30, FlatDmgStart: 6},
	// Unplace
	"needle_of_the_seam":  {DamageBonus: 0.30, FlatDmgStart: 4},
	"mantle_of_elsewhere": {DamageReductMult: 0.88, InitiativeBias: 2.0},
	// Drowned Star
	"halo_of_the_sunken_dawn": {DamageReductMult: 0.92, InitiativeBias: 2.0, MaxHP: 12},
	"tideglass_ward":          {DamageReductMult: 0.80},
	// Last Meridian
	"the_second_hand":  {DamageBonus: 0.30, InitiativeBias: 3.0},
	"escapement_plate": {DamageReductMult: 0.74, MaxHP: 8},
}
