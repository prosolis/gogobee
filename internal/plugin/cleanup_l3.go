package plugin

import (
	"log/slog"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// closeAndRefundLegacyCoopRuns is the Phase L3 one-shot. On every startup
// it scans for any coop_dungeon_runs still in 'open' or 'active' status,
// refunds member contributions and unsettled bets via the wallet, and
// flips the run to 'cancelled'.
//
// Idempotent: once a run is 'cancelled' it is skipped on subsequent runs.
// The cancel UPDATE is per-run with a status-guard, so two parallel calls
// (or a crashed-then-restarted process) will not double-credit a member
// or bettor — only the writer that flips status from 'open'/'active' to
// 'cancelled' performs the refund for that run.
//
// Tables are NOT dropped here. Historical rows remain for querying; a
// future GOGOBEE_COOP_PURGE pass will drop the schema.
func closeAndRefundLegacyCoopRuns(euro *EuroPlugin) {
	d := db.Get()

	rows, err := d.Query(`SELECT id FROM coop_dungeon_runs WHERE status IN ('open','active')`)
	if err != nil {
		slog.Error("coop L3 cleanup: list runs", "err", err)
		return
	}
	var runIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			slog.Error("coop L3 cleanup: scan run id", "err", err)
			continue
		}
		runIDs = append(runIDs, id)
	}
	rows.Close()

	if len(runIDs) == 0 {
		return
	}

	totalRuns, totalMembers, totalBets := 0, 0, 0
	for _, runID := range runIDs {
		res, err := d.Exec(`UPDATE coop_dungeon_runs
			SET status = 'cancelled', completed_at = CURRENT_TIMESTAMP
			WHERE id = ? AND status IN ('open','active')`, runID)
		if err != nil {
			slog.Error("coop L3 cleanup: cancel run", "run_id", runID, "err", err)
			continue
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			continue
		}
		totalRuns++

		mRows, err := d.Query(`SELECT user_id, total_contributed
			FROM coop_dungeon_members
			WHERE run_id = ? AND total_contributed > 0`, runID)
		if err != nil {
			slog.Error("coop L3 cleanup: load members", "run_id", runID, "err", err)
		} else {
			for mRows.Next() {
				var uid string
				var amt int
				if err := mRows.Scan(&uid, &amt); err != nil {
					slog.Error("coop L3 cleanup: scan member", "run_id", runID, "err", err)
					continue
				}
				if euro != nil && amt > 0 {
					euro.Credit(id.UserID(uid), float64(amt), "coop_l3_cancel_refund")
					totalMembers++
				}
			}
			mRows.Close()
		}

		bRows, err := d.Query(`SELECT player_id, amount
			FROM coop_dungeon_bets
			WHERE run_id = ? AND payout IS NULL`, runID)
		if err != nil {
			slog.Error("coop L3 cleanup: load bets", "run_id", runID, "err", err)
		} else {
			for bRows.Next() {
				var uid string
				var amt int
				if err := bRows.Scan(&uid, &amt); err != nil {
					slog.Error("coop L3 cleanup: scan bet", "run_id", runID, "err", err)
					continue
				}
				if euro != nil && amt > 0 {
					euro.Credit(id.UserID(uid), float64(amt), "coop_l3_cancel_bet_refund")
					totalBets++
				}
			}
			bRows.Close()
		}
	}

	if totalRuns > 0 {
		slog.Info("coop L3 cleanup: cancelled in-flight runs",
			"runs_cancelled", totalRuns,
			"member_refunds", totalMembers,
			"bet_refunds", totalBets)
	}
}
