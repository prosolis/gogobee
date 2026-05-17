package plugin

// Expedition sim runner — headless driver that walks expeditions through
// the production plugin code paths against a temp sqlite DB. Used by
// cmd/expedition-sim to generate batched corpora for H3/H4 analysis.
//
// SendDM is a no-op when AdventurePlugin.Client is nil, so the runner
// reads ground truth from the DB rather than parsing transcripts. The
// real-time day cycle (06:00/21:00 cron) is bypassed; the runner drives
// it directly via TickDay, which calls deliverRecap + deliverBriefing
// with a synthetic clock so multi-day expeditions are reachable without
// real-time waits.

import (
	"fmt"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// SimRunner owns a temp-DB AdventurePlugin + EuroPlugin pair and exposes
// just enough surface to drive synthetic players end-to-end.
type SimRunner struct {
	P    *AdventurePlugin
	Euro *EuroPlugin
}

// NewSimRunner initializes a fresh sqlite DB in dataDir and constructs
// the plugin pair. dataDir should already exist and be writable; the
// caller is responsible for cleanup. Re-init after a Close+new dataDir.
func NewSimRunner(dataDir string) (*SimRunner, error) {
	db.Close()
	if err := db.Init(dataDir); err != nil {
		return nil, fmt.Errorf("db init: %w", err)
	}
	euro := &EuroPlugin{}
	p := &AdventurePlugin{euro: euro}
	return &SimRunner{P: p, Euro: euro}, nil
}

// Close releases the shared sqlite handle. Safe to call multiple times.
func (s *SimRunner) Close() {
	db.Close()
}

// BuildCharacter persists a synthetic character at (class, level) under
// uid, with adventure_character + dnd_character + spell-slot rows wired
// up so handleDnDExpeditionCmd accepts them. Stats are class-flavored
// but otherwise vanilla (no race/subclass perks, no equipment).
func (s *SimRunner) BuildCharacter(uid id.UserID, class DnDClass, level int) (*DnDCharacter, error) {
	if err := createAdvCharacter(uid, "sim_"+string(class)); err != nil {
		return nil, fmt.Errorf("createAdvCharacter: %w", err)
	}
	c := &DnDCharacter{
		UserID: uid,
		Race:   RaceHuman,
		Class:  class,
		Level:  level,
	}
	applyClassBaselineStats(c)
	conMod := abilityModifier(c.CON)
	c.HPMax = computeMaxHP(c.Class, conMod, level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))
	c.ShortRestCharges = level
	if err := SaveDnDCharacter(c); err != nil {
		return nil, fmt.Errorf("SaveDnDCharacter: %w", err)
	}
	if pool := slotsForClassLevel(class, level); len(pool) > 0 {
		if err := setSpellSlotsForLevel(uid, class, level); err != nil {
			return nil, fmt.Errorf("setSpellSlotsForLevel: %w", err)
		}
	}
	return c, nil
}

// applyClassBaselineStats sets ability scores that put each class in
// its expected role band — martials prioritize STR/CON, casters their
// casting stat. Values mirror the array choices in the class-balance
// harness so sim numbers stay comparable.
func applyClassBaselineStats(c *DnDCharacter) {
	switch c.Class {
	case ClassFighter, ClassPaladin:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 16, 13, 15, 8, 12, 10
	case ClassRogue, ClassRanger:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 10, 16, 14, 12, 13, 8
	case ClassMage, ClassSorcerer:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 8, 14, 13, 16, 12, 10
	case ClassCleric, ClassDruid:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 12, 10, 14, 8, 16, 13
	case ClassBard, ClassWarlock:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 8, 13, 14, 10, 12, 16
	default:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 12, 12, 12, 12, 12, 12
	}
}

// SimResult is the per-expedition summary the runner emits.
type SimResult struct {
	UserID    string
	Class     string
	Level     int
	Zone      string
	Outcome   string // "extracted" | "cleared" | "tpk" | "boss_doorway" | "fork" | "halted" | "ongoing"
	StopCode  string // sim-stopReason label of the last autopilot walk
	Rooms     int    // total rooms walked across all autopilot bursts
	Walks     int    // autopilot walk invocations (each = up to autopilotRoomCap rooms)
	DayTicks  int    // synthetic day rollovers fired by TickDay
	StartHP   int
	EndHP     int
	StartCoin float64
	EndCoin   float64
	SUStart   float32
	SUEnd     float32
	DaysAtEnd int
	Threat    int
	Log       []SimLogEntry
}

// SimLogEntry is a JSONL-friendly projection of one dnd_expedition_log
// row. We expose just the fields a post-hoc analyzer needs without
// pulling the full ExpeditionEntry type.
type SimLogEntry struct {
	Day     int
	Type    string
	Summary string
	Flavor  string
	TS      time.Time
}

// RunExpedition starts an expedition for uid in zoneID and loops the
// autopilot walk (compact mode, so elite rooms auto-resolve inline)
// until a hard stop fires. Between walks it fast-forwards the day
// cycle so multi-day expeditions complete without real-time waits.
//
// walkCap bounds the number of autopilot bursts as a safety net. Each
// burst walks up to autopilotRoomCap rooms.
//
// Pre-state: uid must own a synthetic character via BuildCharacter and
// have a coin balance sufficient for outfitting (caller's responsibility).
func (s *SimRunner) RunExpedition(uid id.UserID, zoneID ZoneID, walkCap int) (*SimResult, error) {
	c, err := LoadDnDCharacter(uid)
	if err != nil || c == nil {
		return nil, fmt.Errorf("LoadDnDCharacter: %w", err)
	}
	res := &SimResult{
		UserID:    string(uid),
		Class:     string(c.Class),
		Level:     c.Level,
		Zone:      string(zoneID),
		StartHP:   c.HPCurrent,
		StartCoin: s.Euro.GetBalance(uid),
	}

	ctx := MessageContext{Sender: uid}
	if err := s.P.handleDnDExpeditionCmd(ctx, "start "+string(zoneID)); err != nil {
		return res, fmt.Errorf("expedition start: %w", err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp == nil {
		res.Outcome = "halted"
		return res, fmt.Errorf("expedition did not persist after start")
	}
	res.SUStart = exp.Supplies.Current

	for i := 0; i < walkCap; i++ {
		walk := s.P.runAutopilotWalk(ctx, autopilotRoomCap, true)
		if walk.initErr != "" {
			res.Outcome = "halted"
			res.StopCode = "init:" + walk.initErr
			break
		}
		res.Walks++
		res.Rooms += walk.rooms
		res.StopCode = stopReasonLabel(walk.reason)

		// Hard exits: walk says we're done.
		switch walk.reason {
		case stopEnded:
			res.Outcome = "tpk"
			i = walkCap // exit
		case stopComplete:
			res.Outcome = "cleared"
			i = walkCap
		case stopBoss, stopElite:
			// Auto-resolve the encounter: !fight to open, then !attack
			// per round until the session resolves (won / lost / fled).
			killed, err := s.autoResolveCombat(ctx)
			if err != nil {
				res.Outcome = "halted"
				res.StopCode = "combat:" + err.Error()
				i = walkCap
				break
			}
			if !killed {
				// Player lost or fled — autopilot can't continue past the
				// gate. Record outcome and stop.
				if c2, _ := LoadDnDCharacter(uid); c2 == nil || c2.HPCurrent <= 0 {
					res.Outcome = "tpk"
				} else {
					res.Outcome = "fled"
				}
				i = walkCap
			}
			// Boss kill closes the run via combat resolution → continue
			// the loop so the next walk picks up the cleared-run state.
		case stopFork:
			// Deterministic sim policy: always take path 1. Real players
			// pick based on intent; the sim just needs to make progress.
			if err := s.P.handleDnDExpeditionCmd(ctx, "go 1"); err != nil {
				res.Outcome = "halted"
				res.StopCode = "fork:" + err.Error()
				i = walkCap
				break
			}
		case stopBlocked:
			res.Outcome = "blocked"
			i = walkCap
		default:
			// stopOK / stopPreflight / stopHarvestCombat — soft stops.
			// Fast-forward a day so the next walk has fresh supplies
			// budgeted, HP that overnight camp may have healed, and
			// threat drift recorded.
			if exp, _ = getActiveExpedition(uid); exp == nil {
				res.Outcome = "extracted"
				i = walkCap
				break
			}
			if err := s.TickDay(exp); err != nil {
				res.Outcome = "halted"
				res.StopCode = "tick:" + err.Error()
				i = walkCap
				break
			}
			res.DayTicks++
			// TickDay may have force-extracted (starvation). Re-check.
			if exp, _ = getActiveExpedition(uid); exp == nil {
				res.Outcome = "extracted"
				i = walkCap
			}
		}
	}
	if res.Outcome == "" {
		res.Outcome = "ongoing"
	}

	if c2, _ := LoadDnDCharacter(uid); c2 != nil {
		res.EndHP = c2.HPCurrent
	}
	res.EndCoin = s.Euro.GetBalance(uid)
	if exp2, _ := getActiveExpedition(uid); exp2 != nil {
		res.SUEnd = exp2.Supplies.Current
		res.DaysAtEnd = exp2.CurrentDay
		res.Threat = exp2.ThreatLevel
		// Pull the full log for the live expedition.
		res.Log, _ = simLogEntries(exp2.ID)
	} else {
		// Expedition closed — the row may still be queryable by the
		// most-recent expedition for the user.
		if past := mostRecentExpeditionID(uid); past != "" {
			res.Log, _ = simLogEntries(past)
		}
	}
	return res, nil
}

// autoResolveCombat dispatches !fight at the current elite/boss gate,
// then loops !attack until the combat session resolves. Returns true
// when the player won (enemy dead, room cleared), false when the
// player lost or fled. autoCombatRoundCap is a safety cap against
// pathological stalemates (shouldn't trigger in practice — combat is
// strictly monotone in HP).
const autoCombatRoundCap = 200

func (s *SimRunner) autoResolveCombat(ctx MessageContext) (bool, error) {
	if err := s.P.handleFightCmd(ctx); err != nil {
		return false, fmt.Errorf("fight: %w", err)
	}
	sess, err := getActiveCombatSession(ctx.Sender)
	if err != nil {
		return false, fmt.Errorf("getActiveCombatSession: %w", err)
	}
	if sess == nil {
		// No session opened (bestiary miss, doorway already cleared).
		return false, nil
	}
	sessionID := sess.SessionID
	for i := 0; i < autoCombatRoundCap; i++ {
		// Re-fetch by id so we can read Won/Lost/Fled — the "active"
		// filter on getActiveCombatSession returns nil once status flips.
		cur, err := getCombatSession(sessionID)
		if err != nil {
			return false, fmt.Errorf("getCombatSession: %w", err)
		}
		if cur == nil {
			return false, fmt.Errorf("combat session %s vanished mid-fight", sessionID)
		}
		switch cur.Status {
		case CombatStatusWon:
			return true, nil
		case CombatStatusLost, CombatStatusFled:
			return false, nil
		}
		if err := s.P.handleAttackCmd(ctx); err != nil {
			return false, fmt.Errorf("attack iter %d: %w", i, err)
		}
	}
	return false, fmt.Errorf("combat exceeded %d rounds", autoCombatRoundCap)
}

// TickDay drives one synthetic day rollover for exp: 21:00 recap of
// the current day, then 06:00 briefing of the next day. The briefing
// is what bumps current_day and applies supply burn / overnight camp /
// threat drift. exp is re-read from the DB after each step so callers
// see fresh state.
//
// The clock is anchored on exp.StartDate so repeat calls advance one
// real-day per call regardless of wall-clock time when the sim runs.
func (s *SimRunner) TickDay(exp *Expedition) error {
	if exp == nil {
		return fmt.Errorf("nil expedition")
	}
	// Anchor on start_date + current_day so each call lands on a fresh
	// (day, threshold) pair. CurrentDay starts at 1; we want the recap
	// of day D to fire before the briefing of day D+1.
	dayBase := time.Date(
		exp.StartDate.Year(), exp.StartDate.Month(), exp.StartDate.Day(),
		0, 0, 0, 0, time.UTC,
	).AddDate(0, 0, exp.CurrentDay-1)

	recapAt := dayBase.Add(time.Duration(expeditionRecapHour) * time.Hour).Add(30 * time.Second)
	if err := s.P.deliverRecap(exp, recapAt); err != nil {
		return fmt.Errorf("deliverRecap: %w", err)
	}
	if fresh, _ := getExpedition(exp.ID); fresh != nil {
		*exp = *fresh
	}

	briefAt := dayBase.AddDate(0, 0, 1).Add(time.Duration(expeditionBriefingHour) * time.Hour).Add(30 * time.Second)
	if err := s.P.deliverBriefing(exp, briefAt); err != nil {
		return fmt.Errorf("deliverBriefing: %w", err)
	}
	if fresh, _ := getExpedition(exp.ID); fresh != nil {
		*exp = *fresh
	}
	return nil
}

// simLogEntries returns every dnd_expedition_log row for expID, oldest
// first, projected into SimLogEntry.
func simLogEntries(expID string) ([]SimLogEntry, error) {
	rows, err := db.Get().Query(`
		SELECT day, timestamp, entry_type, summary, flavor
		  FROM dnd_expedition_log
		 WHERE expedition_id = ?
		 ORDER BY timestamp ASC, entry_id ASC`, expID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SimLogEntry
	for rows.Next() {
		var e SimLogEntry
		if err := rows.Scan(&e.Day, &e.TS, &e.Type, &e.Summary, &e.Flavor); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// mostRecentExpeditionID looks up the latest expedition row for uid —
// used after RunExpedition to recover the log even when the expedition
// has been extracted (and so is no longer "active").
func mostRecentExpeditionID(uid id.UserID) string {
	var id string
	_ = db.Get().QueryRow(`
		SELECT expedition_id
		  FROM dnd_expedition
		 WHERE user_id = ?
		 ORDER BY start_date DESC
		 LIMIT 1`, string(uid)).Scan(&id)
	return id
}

// stopReasonLabel maps the internal stopReason enum to a stable string
// for sim output (callers outside the package can't switch on the enum).
func stopReasonLabel(r stopReason) string {
	switch r {
	case stopOK:
		return "ok"
	case stopFork:
		return "fork"
	case stopElite:
		return "elite"
	case stopBoss:
		return "boss"
	case stopEnded:
		return "ended"
	case stopComplete:
		return "complete"
	case stopBlocked:
		return "blocked"
	case stopHarvestCombat:
		return "harvest_combat"
	case stopPreflight:
		return "preflight"
	default:
		return "unknown"
	}
}
