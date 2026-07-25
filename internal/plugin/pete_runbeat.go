package plugin

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// Run beats — the room-by-room texture of an expedition, on its way to Pete.
//
// Until now Pete learned that an expedition happened only when it ended: a
// zone_clear, a retreat, a death. The run itself — the fight that nearly went
// wrong, the trap, the haul — was narrated to one Matrix DM and then discarded.
// This records the shape of each moment as it happens so Pete can retell it.
//
// Three rules hold the design together:
//
//   - **Facts, not prose.** A beat carries nouns and numbers; the engine's
//     narration stays in Matrix. Pete owns the words, the same split every Fact
//     already respects.
//   - **Its own channel.** Beats never touch pete_emit_queue. They are
//     high-volume and low-stakes, and a chatty run must not be able to spend the
//     retry budget a death dispatch depends on.
//   - **Never block, never fail the game.** recordRunBeat swallows its errors to
//     a log line. A liveblog is a nice-to-have; the walk it is watching is not.
//
// Delivery rides the roster ticker (one extra request per 2 minutes, not one per
// room) but unlike the roster it is retried, because a dropped beat is a hole in
// a story rather than a stale number the next snapshot corrects.

// runBeatBatch bounds one push. A busy realm mid-evening might produce a few
// hundred beats between ticks; this keeps any single request small and lets the
// backlog drain over a few ticks instead of one enormous POST.
const runBeatBatch = 200

// recordRunBeat appends a beat to the outbound buffer. Never returns an error:
// every caller is on the walk's hot path and none of them can do anything useful
// with a failure to log a story.
//
// Seq is assigned by the INSERT itself (MAX+1 within the statement), so two
// concurrent writers on the same run can't collide on a number — SQLite
// serialises the statement, and the primary key would reject the loser anyway.
func recordRunBeat(runID string, b peteclient.RunBeat) {
	if runID == "" || !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	b.RunID = runID
	if b.OccurredAt == 0 {
		b.OccurredAt = nowUnix()
	}
	// Seq and RunID live in columns; the rest of the beat is the payload, so
	// adding a field later needs no migration.
	kind, occurred := b.Kind, b.OccurredAt
	b.Seq = 0
	payload, err := json.Marshal(b)
	if err != nil {
		slog.Debug("runbeat: marshal failed", "run", runID, "kind", kind, "err", err)
		return
	}
	if _, err := db.Get().Exec(`
		INSERT INTO pete_run_beat (run_id, seq, kind, occurred_at, payload)
		SELECT ?, COALESCE(MAX(seq), 0) + 1, ?, ?, ?
		  FROM pete_run_beat WHERE run_id = ?`,
		runID, kind, occurred, string(payload), runID); err != nil {
		slog.Debug("runbeat: record failed", "run", runID, "kind", kind, "err", err)
	}
}

// runHasEndBeat reports whether this run's story has already been closed. Cheap
// enough to ask at every end site because a run only ends once; see beatRunEnd
// for why the first answer is the one that must stick.
func runHasEndBeat(runID string) bool {
	if runID == "" || !peteclient.Enabled() {
		return false
	}
	var n int
	err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM pete_run_beat WHERE run_id = ? AND kind = 'end'`, runID).Scan(&n)
	return err == nil && n > 0
}

// latestRunIDForNews is the run a just-filed dispatch is about, or "" when there
// isn't one to point at.
//
// The three dispatches that end an expedition — a clear, a retreat, a death —
// are all emitted *after* the run they concluded has been closed, and two of
// them from call sites several frames away from the run row. So rather than
// thread a run id through five signatures and hope the lifetimes line up, this
// asks the question that is actually true at that moment: what is the last run
// this player started. A player has one run at a time and a dispatch about their
// expedition ending is about that one. Multi-region is the case worth stating:
// each region gets its own run, and the last one started is the one they were
// standing in when it ended, which is the log the dispatch should open.
//
// Two clauses do the real work and neither is optional:
//
//   - The `pete_run_beat` check. Runs exist with no beats behind them — from
//     before the liveblog shipped, or with the seam off — and handing Pete a run
//     id it has nothing for would mint a dispatch link to a 404.
//   - The recency window. Not every death happens in a dungeon: the campaign
//     path kills people at the Empty Throne, and without this a death that had
//     nothing to do with any expedition would link to whatever run that player
//     last walked, possibly days ago. An expedition-ending dispatch is filed
//     seconds after its run closes, so "still open, or closed just now" is the
//     honest test for "this dispatch is about that run".
func latestRunIDForNews(userID id.UserID) string {
	if userID == "" || !peteclient.Enabled() {
		return ""
	}
	var runID string
	err := db.Get().QueryRow(`
		SELECT r.run_id
		  FROM dnd_zone_run r
		 WHERE r.user_id = ?
		   AND (r.completed_at IS NULL OR r.completed_at >= datetime('now', '-10 minutes'))
		   AND EXISTS (SELECT 1 FROM pete_run_beat b WHERE b.run_id = r.run_id)
		 ORDER BY r.started_at DESC, r.rowid DESC
		 LIMIT 1`, string(userID)).Scan(&runID)
	if err != nil {
		return ""
	}
	return runID
}

// runBeatPushOK mirrors rosterPushOK: log the transitions, stay quiet otherwise.
var runBeatPushOK bool

// pushRunBeats drains the unsent buffer to Pete. Called from the roster ticker.
func (p *AdventurePlugin) pushRunBeats() {
	beats, drop, err := loadRunBeatBatch(runBeatBatch)
	if err != nil {
		slog.Error("runbeat: load batch failed", "err", err)
		return
	}
	// Beats belonging to an opted-out player are retired locally without ever
	// going out. Marking them sent (rather than deleting) keeps one code path for
	// "this row is done with" and lets the retention sweep reap them on its own
	// clock. Opting back in mid-run loses the earlier beats, which is the right
	// way round to be wrong.
	if len(drop) > 0 {
		if err := markRunBeatsSent(drop); err != nil {
			slog.Warn("runbeat: retire opted-out beats", "err", err)
		}
	}
	if len(beats) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), rosterPushTimeout)
	defer cancel()
	if err := peteclient.PushRunBeats(ctx, beats); err != nil {
		if runBeatPushOK {
			slog.Warn("runbeat: push failed, liveblog will lag on Pete", "err", err, "beats", len(beats))
		} else {
			slog.Debug("runbeat: push failed, will retry next tick", "err", err)
		}
		runBeatPushOK = false
		return // rows stay unsent: this is the one push that retries
	}
	if err := markRunBeatsSent(beats); err != nil {
		// Delivered but not marked. Pete is idempotent on (run_id, seq), so the
		// re-send next tick is a no-op there — better than dropping the row.
		slog.Warn("runbeat: mark sent failed, beats will re-send", "err", err)
	}
	if !runBeatPushOK {
		slog.Info("runbeat: liveblog accepted by Pete", "beats", len(beats))
		runBeatPushOK = true
	}
}

// loadRunBeatBatch reads up to limit unsent beats in (run, seq) order and splits
// them into the ones to send and the ones to retire unsent.
//
// It drains the cursor completely before resolving a single owner, and that is
// not a style preference. The pool is one connection wide, so a query issued
// while these rows are still open waits for a connection that this loop is
// holding and will not release until the loop ends — a deadlock that the roster
// ticker would hit on its very first tick with any beat in the buffer.
//
// The opt-out check is then per *run*, resolved once and cached for the batch: a
// run belongs to exactly one player, and re-asking per beat would turn a 200-row
// batch into 200 lookups of an answer that cannot change inside one tick.
func loadRunBeatBatch(limit int) (send []peteclient.RunBeat, drop []peteclient.RunBeat, err error) {
	type row struct {
		runID   string
		seq     int64
		payload string
	}

	rows, qerr := db.Get().Query(`
		SELECT run_id, seq, payload
		  FROM pete_run_beat
		 WHERE sent_at IS NULL
		 ORDER BY run_id ASC, seq ASC
		 LIMIT ?`, limit)
	if qerr != nil {
		return nil, nil, qerr
	}
	var raw []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.runID, &r.seq, &r.payload); err != nil {
			rows.Close()
			return nil, nil, err
		}
		raw = append(raw, r)
	}
	rerr := rows.Err()
	rows.Close()
	if rerr != nil {
		return nil, nil, rerr
	}

	allowed := map[string]bool{}
	for _, r := range raw {
		var b peteclient.RunBeat
		if err := json.Unmarshal([]byte(r.payload), &b); err != nil {
			// An undecodable row is dead weight forever; retire it rather than
			// letting it head the queue and block every beat behind it.
			slog.Warn("runbeat: undecodable payload, retiring", "run", r.runID, "seq", r.seq, "err", err)
			drop = append(drop, peteclient.RunBeat{RunID: r.runID, Seq: r.seq})
			continue
		}
		b.RunID, b.Seq = r.runID, r.seq

		ok, known := allowed[r.runID]
		if !known {
			ok = runBeatAllowed(r.runID)
			allowed[r.runID] = ok
		}
		if ok {
			send = append(send, b)
		} else {
			drop = append(drop, b)
		}
	}
	return send, drop, nil
}

// runBeatAllowed reports whether this run's beats may leave the box. A run whose
// owner can't be resolved is refused: the liveblog is a public surface, and
// "don't know who this is" is not a safe basis for publishing where they are.
func runBeatAllowed(runID string) bool {
	run, err := getZoneRun(runID)
	if err != nil || run == nil || run.UserID == "" {
		return false
	}
	return !isNewsOptedOut(id.UserID(run.UserID))
}

// markRunBeatsSent stamps a batch delivered, in one transaction so a crash
// mid-mark can't leave half a run looking unsent and re-send it.
func markRunBeatsSent(beats []peteclient.RunBeat) error {
	if len(beats) == 0 {
		return nil
	}
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE pete_run_beat SET sent_at = ? WHERE run_id = ? AND seq = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Unix()
	for _, b := range beats {
		if _, err := stmt.Exec(now, b.RunID, b.Seq); err != nil {
			return err
		}
	}
	return tx.Commit()
}
