package plugin

// Expedition sim runner — headless driver that walks expeditions through
// the production plugin code paths against a temp sqlite DB. Used by
// cmd/expedition-sim to generate batched corpora for H3/H4 analysis.
//
// SendDM is a no-op when AdventurePlugin.Client is nil, so the runner
// reads ground truth from the DB rather than parsing transcripts. The
// real-time day cycle (06:00/21:00 cron) is bypassed; expeditions are
// driven to completion via repeated !expedition run cycles until the
// autopilot stops or supplies drain.

import (
	"fmt"

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

// SimResult is the per-expedition summary the runner emits. The
// per-room log lives in the dnd_expedition_log table for now; later
// phases can extract a structured slice here.
type SimResult struct {
	UserID    string
	Class     string
	Level     int
	Zone      string
	Outcome   string // "extracted" | "abandoned" | "tpk" | "halted" | "ongoing"
	Rooms     int
	StartHP   int
	EndHP     int
	StartCoin float64
	EndCoin   float64
	SUStart   float32
	SUEnd     float32
	DaysAtEnd int
}

// RunExpedition starts an expedition for uid in zoneID and loops
// !expedition run until the autopilot stops advancing rooms. runCap
// bounds the total !expedition run invocations as a safety net against
// runaway loops (each invocation walks up to autopilotRoomCap rooms).
//
// Pre-state: uid must own a synthetic character via BuildCharacter and
// have a coin balance sufficient for outfitting (caller's responsibility).
func (s *SimRunner) RunExpedition(uid id.UserID, zoneID ZoneID, runCap int) (*SimResult, error) {
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

	priorRoom := -1
	stalls := 0
	for i := 0; i < runCap; i++ {
		if err := s.P.handleDnDExpeditionCmd(ctx, "run"); err != nil {
			return res, fmt.Errorf("expedition run iter %d: %w", i, err)
		}
		cur, _ := getActiveExpedition(uid)
		if cur == nil {
			// Expedition closed (extracted, TPK, abandoned).
			res.Outcome = "extracted"
			break
		}
		run, _ := getActiveZoneRun(uid)
		room := -1
		if run != nil {
			room = run.CurrentRoom
		}
		if room == priorRoom {
			stalls++
			if stalls >= 2 {
				// Two consecutive runs with no room advance — autopilot
				// has stopped (HP/SU/threshold). Sim mode bails so we
				// can record the halt rather than loop forever.
				res.Outcome = "halted"
				break
			}
		} else {
			stalls = 0
			priorRoom = room
		}
		res.Rooms++
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
	}
	return res, nil
}
