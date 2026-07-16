package plugin

// Tier 6 "Mythic" post-game zone definitions + region registries (Phase P3).
//
// Five hand-authored endgame zones, each four regions deep, gated behind
// postgameUnlocked. Standards are demoted T4/T5 monsters (the tier
// statement); elites and bosses are the bespoke stat blocks in
// postgame_bestiary.go. Loot entries here are declared but their drop
// mechanics (boss-only signature roll, magicItemEffectOverlay, pity
// recipes) are wired in Phase P5.
//
// Registration runs from this file's init(), which executes after
// dnd_zone.go's init() (filename order), so T6 zones append to zoneOrder
// after T1–T5. Hooks/Atmosphere are TwinBee first-person voice.

func init() {
	registerZone(zoneOssuaryAscendant())
	registerZone(zoneFirstHoard())
	registerZone(zoneUnplace())
	registerZone(zoneDrownedStar())
	registerZone(zoneLastMeridian())

	regionsByZone[ZoneOssuaryAscendant] = []ExpeditionRegion{
		{ID: "ossuary_bonefall_steps", ZoneID: ZoneOssuaryAscendant, Order: 1,
			Name: "Bonefall Steps", BaseCampSite: false,
			RegionBoss:  "The Chorister",
			EnemySubset: []string{"wight", "revenant", "the_chorister"}},
		{ID: "ossuary_cathedral_marrow", ZoneID: ZoneOssuaryAscendant, Order: 2,
			Name: "Cathedral of Marrow", BaseCampSite: true,
			RegionBoss:  "Grave Cardinal",
			EnemySubset: []string{"banshee", "wraith", "grave_cardinal"}},
		{ID: "ossuary_reliquary_vaults", ZoneID: ZoneOssuaryAscendant, Order: 3,
			Name: "Reliquary Vaults", BaseCampSite: true,
			RegionBoss:  "Reliquary Knight",
			EnemySubset: []string{"wraith", "revenant", "reliquary_knight"}},
		{ID: "ossuary_apotheosis_engine", ZoneID: ZoneOssuaryAscendant, Order: 4,
			Name: "The Apotheosis Engine", BaseCampSite: false,
			RegionBoss: "Valdris, At Last (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"wight", "banshee"}},
	}
	regionsByZone[ZoneFirstHoard] = []ExpeditionRegion{
		{ID: "hoard_cooling_throne", ZoneID: ZoneFirstHoard, Order: 1,
			Name: "The Cooling Throne", BaseCampSite: false,
			RegionBoss:  "Choir Drake Matriarch",
			EnemySubset: []string{"salamander", "helmed_horror", "choir_drake_matriarch"}},
		{ID: "hoard_gilded_gullet", ZoneID: ZoneFirstHoard, Order: 2,
			Name: "Gilded Gullet", BaseCampSite: true,
			RegionBoss:  "Coinborn Simulacrum",
			EnemySubset: []string{"helmed_horror", "young_red_dragon", "coinborn_simulacrum"}},
		{ID: "hoard_clutch_vaults", ZoneID: ZoneFirstHoard, Order: 3,
			Name: "The Clutch Vaults", BaseCampSite: true,
			RegionBoss:  "Wyrm-Sworn Executor",
			EnemySubset: []string{"young_red_dragon", "salamander", "wyrm_sworn_executor"}},
		{ID: "hoard_cradle_first_flame", ZoneID: ZoneFirstHoard, Order: 4,
			Name: "Cradle of the First Flame", BaseCampSite: false,
			RegionBoss: "Aurvandryx (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"young_red_dragon"}},
	}
	regionsByZone[ZoneUnplace] = []ExpeditionRegion{
		{ID: "unplace_the_fray", ZoneID: ZoneUnplace, Order: 1,
			Name: "The Fray", BaseCampSite: false,
			RegionBoss:  "Angleworn Horror",
			EnemySubset: []string{"hezrou", "nalfeshnee", "angleworn_horror"}},
		{ID: "unplace_eschers_debt", ZoneID: ZoneUnplace, Order: 2,
			Name: "Escher's Debt", BaseCampSite: true,
			RegionBoss:  "The Unnumbered",
			EnemySubset: []string{"nalfeshnee", "marilith", "the_unnumbered"}},
		{ID: "unplace_stitchworks", ZoneID: ZoneUnplace, Order: 3,
			Name: "The Stitchworks", BaseCampSite: true,
			RegionBoss:  "Echo of Belaxath",
			EnemySubset: []string{"marilith", "hezrou", "echo_of_belaxath"}},
		{ID: "unplace_the_seam", ZoneID: ZoneUnplace, Order: 4,
			Name: "The Seam", BaseCampSite: false,
			RegionBoss: "The Seamstress (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"marilith"}},
	}
	regionsByZone[ZoneDrownedStar] = []ExpeditionRegion{
		{ID: "star_long_sink", ZoneID: ZoneDrownedStar, Order: 1,
			Name: "The Long Sink", BaseCampSite: false,
			RegionBoss:  "Choir of Static",
			EnemySubset: []string{"merrow", "water_elemental", "choir_of_static"}},
		{ID: "star_pilgrim_trench", ZoneID: ZoneDrownedStar, Order: 2,
			Name: "Pilgrim Trench", BaseCampSite: true,
			RegionBoss:  "Lantern Warden",
			EnemySubset: []string{"aboleth_thrall", "water_elemental", "lantern_warden"}},
		{ID: "star_radiant_wreck", ZoneID: ZoneDrownedStar, Order: 3,
			Name: "The Radiant Wreck", BaseCampSite: true,
			RegionBoss:  "Star-Gorged Leviathanling",
			EnemySubset: []string{"aboleth_thrall", "merrow", "star_gorged_leviathanling"}},
		{ID: "star_heart_chapel", ZoneID: ZoneDrownedStar, Order: 4,
			Name: "The Heart Chapel", BaseCampSite: false,
			RegionBoss: "Seraphel (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"water_elemental"}},
	}
	regionsByZone[ZoneLastMeridian] = []ExpeditionRegion{
		{ID: "meridian_dusk_colonnade", ZoneID: ZoneLastMeridian, Order: 1,
			Name: "The Dusk Colonnade", BaseCampSite: false,
			RegionBoss:  "The Hour Thief",
			EnemySubset: []string{"specter", "wraith", "the_hour_thief"}},
		{ID: "meridian_gallery_spent_hours", ZoneID: ZoneLastMeridian, Order: 2,
			Name: "Gallery of Spent Hours", BaseCampSite: true,
			RegionBoss:  "Wardens-in-Waiting",
			EnemySubset: []string{"wraith", "helmed_horror", "wardens_in_waiting"}},
		{ID: "meridian_escapement", ZoneID: ZoneLastMeridian, Order: 3,
			Name: "The Escapement", BaseCampSite: true,
			RegionBoss:  "Pendulum Colossus",
			EnemySubset: []string{"helmed_horror", "specter", "pendulum_colossus"}},
		{ID: "meridian_one_minute", ZoneID: ZoneLastMeridian, Order: 4,
			Name: "One Minute To Midnight", BaseCampSite: false,
			RegionBoss: "The Custodian (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"helmed_horror", "wraith"}},
	}
}

// ---- Tier 6 zone factories --------------------------------------------------

func zoneOssuaryAscendant() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneOssuaryAscendant,
		Display:    "The Ossuary Ascendant",
		Tier:       ZoneTierMythic,
		LevelMin:   18,
		LevelMax:   20,
		Faction:    "Undead, Lich-ascendant",
		Atmosphere: "An inverted bone cathedral hanging over the old Crypt; marrow-light, choral static, the smell of a very patient plan finally paying off.",
		Hook:       "Valdris made it. Dying in the Crypt was step one all along — the phylactery shard you looted was bait. He has rebuilt himself into a true lich, one vertebra at a time, and hung his cathedral upside-down over his own grave. Your oldest quest item has come home. I do not like it here.",
		MinRooms:   44,
		MaxRooms:   52,
		Enemies: []ZoneEnemy{
			{BestiaryID: "wight", SpawnWeight: 6},
			{BestiaryID: "revenant", SpawnWeight: 5},
			{BestiaryID: "banshee", SpawnWeight: 4},
			{BestiaryID: "wraith", SpawnWeight: 4},
			{BestiaryID: "the_chorister", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "grave_cardinal", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "reliquary_knight", SpawnWeight: 2, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_valdris_ascendant",
			Name:        "Valdris, At Last",
			CR:          26,
			HP:          470,
			AC:          21,
			PhaseTwoAt:  0.50,
			Description: "The lich-aspirant from the Crypt, ascended. He respects a good plan. He has been running one on you for years.",
			Abilities: []string{
				"Apotheosis Nova: decisive AoE, armor-piercing radiant-necrotic burst",
				"Phase 2 (<50% HP): flies; nova recharges faster",
				"Phylactery Verses (Layer 2): three secret Verses in the zone each strip one Legendary Resistance / rebirth proc if found first",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "crown_of_the_patient_king", DropChance: 0.04, BossOnly: true, Note: "head, Legendary: +3 INT & WIS; 1/run survive a killing blow at 1 HP"},
			{ItemID: "valdris_quill", DropChance: 0.04, BossOnly: true, Note: "caster weapon, Legendary: spell attacks +3; on spell kill refund the slot (1/combat)"},
			{ItemID: "verse_bound_folio", UniqueAlways: true, Note: "T6 crafting anchor"},
			{ItemID: "coins_60d10x120", DropChance: 1.0, Note: "60d10 × 120 coins"},
		},
		FlavorFile: "zone_ossuary_ascendant_flavor.go",
	}
}

func zoneFirstHoard() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneFirstHoard,
		Display:    "The First Hoard",
		Tier:       ZoneTierMythic,
		LevelMin:   18,
		LevelMax:   20,
		Faction:    "Dragons, Wyrm-cult, the Hoard itself",
		Atmosphere: "Beneath Infernus Peak, still warm. Rivers of gilded scale, a floor of minted antibodies, and under it all the slow breathing of something that was old when fire was new.",
		Hook:       "Infernax is dead and the mountain is still warm — because he was never the owner. He was the guard dog. Beneath the peak sleeps Aurvandryx, the Ember Before Fire, whose shed scales became every dragon you have ever met. The hoard isn't treasure. It's her clutch, gilded over. I suggest we not wake the mother.",
		MinRooms:   44,
		MaxRooms:   52,
		Enemies: []ZoneEnemy{
			{BestiaryID: "salamander", SpawnWeight: 6},
			{BestiaryID: "helmed_horror", SpawnWeight: 5},
			{BestiaryID: "young_red_dragon", SpawnWeight: 3},
			{BestiaryID: "choir_drake_matriarch", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "coinborn_simulacrum", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "wyrm_sworn_executor", SpawnWeight: 2, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_aurvandryx",
			Name:        "Aurvandryx, the Ember Before Fire",
			CR:          28,
			HP:          540,
			AC:          22,
			PhaseTwoAt:  0.40,
			Description: "The progenitor wyrm. Fire that predates fire resistance. She is not defending treasure; she is defending eggs.",
			Abilities: []string{
				"First Flame: decisive fire AoE that ignores resistance (phase-2 narration notes it failing)",
				"Phase 2 (<40% HP): grows; First Flame recharges faster",
				"Greed Tax (Layer 2): her Attack scales +1 per N loot drops picked up during the run",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "aegis_of_the_first_scale", DropChance: 0.05, BossOnly: true, Note: "armor, Legendary (best in game): AC +3, fire immunity, 1/run ignore a killing blow over half max HP"},
			{ItemID: "emberheart_greatblade", DropChance: 0.04, BossOnly: true, Note: "weapon, Legendary: +3, +3d6 fire, crits strip target BlockRate next round"},
			{ItemID: "cooling_ember_of_aurvandryx", UniqueAlways: true, Note: "T6 crafting anchor — the legendary-fire capstone thyraks_core has been waiting for"},
			{ItemID: "coins_60d10x120", DropChance: 1.0, Note: "60d10 × 120 coins"},
		},
		FlavorFile: "zone_first_hoard_flavor.go",
	}
}

func zoneUnplace() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneUnplace,
		Display:    "The Unplace",
		Tier:       ZoneTierMythic,
		LevelMin:   18,
		LevelMax:   20,
		Faction:    "Demons, geometry itself, a corrupted celestial",
		Atmosphere: "A wound in space that geometry gave up on. Rooms that are also other rooms. A horizon indoors. Something is stitching it shut from the inside, and it is not doing it to help.",
		Hook:       "Belaxath tore the portal open over thirty years; killing him didn't close it — it left the tear unattended. The wound has scarred into a region where a corner can be three rooms away and a straight line arrives before it leaves. I have stopped trusting the map. I advise you stop trusting the walls.",
		MinRooms:   44,
		MaxRooms:   52,
		Enemies: []ZoneEnemy{
			{BestiaryID: "hezrou", SpawnWeight: 6},
			{BestiaryID: "nalfeshnee", SpawnWeight: 4},
			{BestiaryID: "marilith", SpawnWeight: 3},
			{BestiaryID: "angleworn_horror", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "the_unnumbered", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "echo_of_belaxath", SpawnWeight: 2, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_seamstress",
			Name:        "The Seamstress",
			CR:          27,
			HP:          480,
			AC:          21,
			PhaseTwoAt:  0.35,
			Description: "A corrupted celestial who volunteered to close the tear centuries early and has been sewing herself into it ever since. Half of her is on the other side, and the far half does most of the talking.",
			Abilities: []string{
				"Needle Rain: decisive AoE of driven needles",
				"Phase 2 (<35% HP): the room sews inside-out",
				"Inversion Stitch (Layer 2): healing and damage swap direction on her in telegraphed 2-round pulses",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "needle_of_the_seam", DropChance: 0.04, BossOnly: true, Note: "dagger, Legendary (best caster weapon): +3; spell crit range +1; on crit next spell is free"},
			{ItemID: "mantle_of_elsewhere", DropChance: 0.05, BossOnly: true, Note: "cloak: +2 AC; 1/combat auto-evade one hit"},
			{ItemID: "spool_of_undone_geometry", UniqueAlways: true, Note: "T6 crafting anchor"},
			{ItemID: "coins_60d10x120", DropChance: 1.0, Note: "60d10 × 120 coins"},
		},
		FlavorFile: "zone_unplace_flavor.go",
	}
}

func zoneDrownedStar() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneDrownedStar,
		Display:    "The Drowned Star",
		Tier:       ZoneTierMythic,
		LevelMin:   18,
		LevelMax:   20,
		Faction:    "Aboleth-touched, abyssal pilgrims, a fallen angel",
		Atmosphere: "An abyssal trench lit from below by a dying star, and by the angel who rode it down. The pilgrimage route glows. Both the star and its guardian have gone strange in ten thousand years of dark.",
		Hook:       "The Dreaming Aboleth has been dreaming of one specific thing this whole time: a star that fell into the trench before the surface had names, with its angel still strapped to it. Seraphel rode her charge down rather than abandon it, and has spent ten thousand years keeping a dying star alive with radiance meant for healing. Both of them have gone strange. I would too.",
		MinRooms:   44,
		MaxRooms:   52,
		Enemies: []ZoneEnemy{
			{BestiaryID: "merrow", SpawnWeight: 6},
			{BestiaryID: "water_elemental", SpawnWeight: 5},
			{BestiaryID: "aboleth_thrall", SpawnWeight: 3},
			{BestiaryID: "choir_of_static", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "lantern_warden", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "star_gorged_leviathanling", SpawnWeight: 2, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_seraphel",
			Name:        "Seraphel, the Light That Sank",
			CR:          27,
			HP:          460,
			AC:          21,
			PhaseTwoAt:  0.50,
			Description: "An angel who would not let go. Ten thousand years of shielding a dying star with healing light have made her something that grieves in radiant bursts.",
			Abilities: []string{
				"Sanctified Undertow: decisive radiant AoE",
				"Phase 2 (<50% HP): grieving — faster, sloppier",
				"Two Hearts (Layer 2): a second ~150 HP Star-Heart pool; burn it and she enrages, spare it and she never does (hidden mercy loot bias)",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "halo_of_the_sunken_dawn", DropChance: 0.04, BossOnly: true, Note: "head, Legendary: +2 all saves; ally-heals you cast +50%"},
			{ItemID: "tideglass_ward", DropChance: 0.05, BossOnly: true, Note: "shield: +3 AC contribution; 1/combat negate an AoE proc entirely"},
			{ItemID: "ember_of_the_drowned_star", UniqueAlways: true, Note: "T6 crafting anchor"},
			{ItemID: "coins_60d10x120", DropChance: 1.0, Note: "60d10 × 120 coins"},
		},
		FlavorFile: "zone_drowned_star_flavor.go",
	}
}

func zoneLastMeridian() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneLastMeridian,
		Display:    "The Last Meridian",
		Tier:       ZoneTierMythic,
		LevelMin:   18,
		LevelMax:   20,
		Faction:    "Clock-constructs, horologist ghosts, the Custodian",
		Atmosphere: "A necropolis-observatory where timekeeping was invented and is now being quietly decommissioned. Rooms you visited at dusk are midnight when you return. The final region plays in the second the clock stops.",
		Hook:       "This is where the world's hours were invented — and where they are being taken apart. The Custodian has decided its contract is complete and is dismantling the hours one at a time on its way out. It is unfailingly polite about it. It will apologize before each swing. I find that worse, somehow.",
		MinRooms:   44,
		MaxRooms:   52,
		Enemies: []ZoneEnemy{
			{BestiaryID: "specter", SpawnWeight: 6},
			{BestiaryID: "wraith", SpawnWeight: 5},
			{BestiaryID: "helmed_horror", SpawnWeight: 3},
			{BestiaryID: "the_hour_thief", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "wardens_in_waiting", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "pendulum_colossus", SpawnWeight: 2, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_custodian",
			Name:        "The Custodian of the Last Hour",
			CR:          27,
			HP:          500,
			AC:          21,
			PhaseTwoAt:  0.45,
			Description: "A titanic clock-golem of verdigris and orrery rings, closing out its contract. It apologizes, on schedule, before it kills you.",
			Abilities: []string{
				"Chime: decisive AoE on the hour",
				"Phase 2 (<45% HP): rewinds",
				"Amendment (Layer 2): once, rewinds itself to its round-3 HP snapshot; soft midnight timer adds +2 Atk/round past round 20",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "the_second_hand", DropChance: 0.04, BossOnly: true, Note: "weapon, Legendary (best martial sidearm): +3; 1/combat immediate extra attack; on kill refund the use"},
			{ItemID: "escapement_plate", DropChance: 0.05, BossOnly: true, Note: "armor: +3 AC; first stun each combat is negated"},
			{ItemID: "unspent_hour", UniqueAlways: true, Note: "T6 crafting anchor"},
			{ItemID: "coins_60d10x120", DropChance: 1.0, Note: "60d10 × 120 coins"},
		},
		FlavorFile: "zone_last_meridian_flavor.go",
	}
}
