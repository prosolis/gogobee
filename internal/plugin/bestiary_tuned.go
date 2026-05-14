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
// Caveat — abilities are NOT wired. Every generated entry has a nil Ability;
// its Notes field carries the SRD multiattack/trait text as raw material for
// the follow-up ability-wiring pass. Until that pass lands, a tuned-only
// creature fights with stats but no special mechanic.
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
