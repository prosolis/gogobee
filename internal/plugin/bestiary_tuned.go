package plugin

// Tuned SRD bestiary — the engine-ready layer derived from the raw staging
// table.
//
// srdStagingBestiary holds verbatim SRD stat blocks, which one-shot gogobee's
// solo player and are not a drop-in roster (see bestiary_srd_staging.go).
// tunedBestiarySRD is what the codified tuning pass produces from it: a
// DnDMonsterTemplate per SRD monster, scaled to the engine's gameplay shape.
// The formula lives in cmd/open5e-import/tuned.go; regenerate the data with
// `go run ./cmd/open5e-import gen tuned`.
//
// Abilities — the generator classifies each creature's SRD trait names onto an
// engine MonsterAbility effect (see abilityFromTraits in tuned.go). A creature
// whose traits are all non-combat (Keen Smell, Amphibious, …) keeps a nil
// Ability. The Notes field records the raw SRD multiattack/trait text and which
// ability was wired, so a hand-tuning pass can refine the pick — and since the
// roster merge below lets hand-authored entries win, refining is just a matter
// of adding the creature to dndBestiary.
//
// File is intentionally NOT dnd_-prefixed (see memory: no new dnd_* identifiers).

var tunedBestiarySRD = buildTunedBestiarySRD()

// Merge the tuned templates into the canonical dndBestiary lookup. Hand-authored
// roster entries WIN: a tuned entry is only added for an ID the roster doesn't
// already define, so playtested numbers and wired abilities are never clobbered.
var _ = func() bool {
	for id, m := range tunedBestiarySRD {
		if _, ok := dndBestiary[id]; !ok {
			dndBestiary[id] = m
		}
	}
	return true
}()
