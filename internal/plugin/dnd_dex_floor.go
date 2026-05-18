package plugin

import (
	"log/slog"

	"gogobee/internal/db"
)

// bumpDexFloorForExistingCharacters raises the DEX score of any
// dnd_character row below 14 to 14, then recomputes armor_class from the
// new DEX modifier. One-shot bootstrap to align prod-DB characters with
// the 2026-05-10 fun-balance change to classStatPriority (cleric/mage
// DEX baselines moved up so armor tier upgrades feel like upgrades).
//
// Idempotent: the UPDATE only touches rows where dex_score < 14. After
// the first run, it's a no-op until someone manually drops a character's
// DEX below the floor.
//
// Trade-off: this departs from canonical D&D standard-array purity. The
// game prioritizes "armor and AC matter" over "your stat dump should
// punish you forever." If we ever want to back this out, the migration
// is a single SQL UPDATE — but the canonicality gain isn't worth the
// fun loss for an asynchronous-combat game.
func bumpDexFloorForExistingCharacters() {
	const dexFloor = 14
	d := db.Get()
	res, err := d.Exec(`UPDATE dnd_character SET dex_score = ? WHERE dex_score < ?`, dexFloor, dexFloor)
	if err != nil {
		slog.Error("dnd: dex floor migration failed", "err", err)
		return
	}
	bumped, _ := res.RowsAffected()
	if bumped == 0 {
		return
	}

	// Recompute armor_class for everyone we touched. Combat re-derives AC
	// each fight via applyDnDEquipmentLayer, so this is mostly cosmetic
	// (sheet display, !stats), but stale ArmorClass on the character row
	// would mislead players reading their sheet.
	rows, err := d.Query(`SELECT user_id, class, dex_score FROM dnd_character WHERE dex_score = ?`, dexFloor)
	if err != nil {
		slog.Error("dnd: dex floor — load post-bump rows failed", "err", err)
		return
	}
	defer rows.Close()
	type touched struct {
		userID string
		class  DnDClass
		dex    int
	}
	var batch []touched
	for rows.Next() {
		var t touched
		var classStr string
		if err := rows.Scan(&t.userID, &classStr, &t.dex); err != nil {
			slog.Warn("dnd: dex floor scan failed", "err", err)
			continue
		}
		t.class = DnDClass(classStr)
		batch = append(batch, t)
	}
	for _, t := range batch {
		ac := computeAC(t.class, abilityModifier(t.dex))
		if _, err := d.Exec(`UPDATE dnd_character SET armor_class = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
			ac, t.userID); err != nil {
			slog.Warn("dnd: dex floor armor_class update failed", "user", t.userID, "err", err)
		}
	}
	slog.Info("dnd: applied DEX floor 14 to existing characters",
		"bumped", bumped, "ac_recomputed", len(batch))
}

// seedShortRestChargesForExistingCharacters fills short_rest_charges =
// dnd_level for any character whose charges are still 0 (the schema
// default for new columns on existing rows). One-shot, idempotent —
// after the first run only newly-created chars need seeding, and those
// already get charges = level via autoBuildCharacter / setup confirm.
func seedShortRestChargesForExistingCharacters() {
	d := db.Get()
	res, err := d.Exec(`UPDATE dnd_character SET short_rest_charges = dnd_level WHERE short_rest_charges = 0 AND dnd_level > 0`)
	if err != nil {
		slog.Error("dnd: short rest charge seed failed", "err", err)
		return
	}
	if seeded, _ := res.RowsAffected(); seeded > 0 {
		slog.Info("dnd: seeded short rest charges = level for existing characters", "seeded", seeded)
	}
}
