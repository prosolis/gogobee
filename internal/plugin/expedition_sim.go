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
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// simInlineBossCombat is a D8-e DIAGNOSTIC toggle. When GOGOBEE_SIM_INLINE_BOSS=1
// the sim routes boss/elite doorways through the inline SimulateCombat path
// (production autopilot behavior) instead of the turn-based !fight engine.
// Used to A/B the martial T4/T5 regression. NOT for production.
func simInlineBossCombat() bool { return os.Getenv("GOGOBEE_SIM_INLINE_BOSS") == "1" }

// SimRunner owns a temp-DB AdventurePlugin + EuroPlugin pair and exposes
// just enough surface to drive synthetic players end-to-end.
type SimRunner struct {
	P    *AdventurePlugin
	Euro *EuroPlugin
	// Companion hires Pete into the party on Day 1: "" = none, "auto" = fill the
	// missing role, or an explicit class id. He takes a combat seat and no loot,
	// which is exactly the thing the sweep is here to measure — whether an extra
	// below-median body lifts the trailing case without carrying it.
	Companion string
}

// NewSimRunner initializes a fresh sqlite DB in dataDir and constructs
// the plugin pair. dataDir should already exist and be writable; the
// caller is responsible for cleanup. Re-init after a Close+new dataDir.
func NewSimRunner(dataDir string) (*SimRunner, error) {
	db.Close()
	if err := db.Init(dataDir); err != nil {
		return nil, fmt.Errorf("db init: %w", err)
	}
	// Synthetic players don't type !arm between fights; flip on the
	// in-combat auto-arm so Fighter Second Wind, Cleric Healing Word,
	// etc. fire the way a competent prod player would set them up.
	// Without this the sim under-counts class survival.
	simAutoArmEnabled = true
	// N7/B3 — neutralize the weekly Omen so corpus results don't depend on which
	// wall-clock week the sweep runs in (the sim's synthetic clock never reaches
	// activeOmen's time.Now()).
	simOmenDisabled = true
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
	// An unknown class is not an error anywhere downstream: applyClassBaselineStats
	// and computeMaxHP both fall through to a default, and the run walks off with a
	// 1-HP character that dies on turn one. A typo in -class or -party-classes then
	// reports a normal outcome for a character nobody asked to simulate.
	if _, ok := classInfo(class); !ok {
		return nil, fmt.Errorf("unknown class %q", class)
	}
	if err := createAdvCharacter(uid, "sim_"+string(class)); err != nil {
		return nil, fmt.Errorf("createAdvCharacter: %w", err)
	}
	if simPetLevel > 0 {
		if err := attachSimPet(uid, simPetLevel); err != nil {
			return nil, fmt.Errorf("attachSimPet: %w", err)
		}
	}
	c := &DnDCharacter{
		UserID: uid,
		Race:   RaceHuman,
		Class:  class,
		Level:  level,
	}
	applyClassBaselineStats(c)
	// Match prod char-creation: race mods on top of the class array.
	// BuildCharacter has historically defaulted to Human (+1 to all);
	// without this step the synthetic character ran a full point under
	// every real player at the same level.
	scores := applyRaceMods(c.Race, [6]int{c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA})
	c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = scores[0], scores[1], scores[2], scores[3], scores[4], scores[5]
	conMod := abilityModifier(c.CON)
	c.HPMax = computeMaxHP(c.Class, conMod, level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))
	c.ShortRestCharges = level
	if err := SaveDnDCharacter(c); err != nil {
		return nil, fmt.Errorf("SaveDnDCharacter: %w", err)
	}
	// Class resource pool (stamina/favor/focus/spell_slot). !setup writes
	// this for prod players; the sim has to do it explicitly so Second
	// Wind / Healing Word / etc. have a pool to draw from.
	if err := initResources(uid, class); err != nil {
		return nil, fmt.Errorf("initResources: %w", err)
	}
	if pool := slotsForClassLevel(class, level); len(pool) > 0 {
		if err := setSpellSlotsForLevel(uid, class, level); err != nil {
			return nil, fmt.Errorf("setSpellSlotsForLevel: %w", err)
		}
	}
	// Populate the known+prepared spell list. ensureSpellsForCharacter is
	// the canonical seeder used by !setup and the auto-migrate path; prod
	// players hit it on character create. The sim has to call it
	// explicitly so simPickSpell sees a real spellbook. No-op for martials.
	if err := ensureSpellsForCharacter(c); err != nil {
		return nil, fmt.Errorf("ensureSpellsForCharacter: %w", err)
	}
	if err := outfitSimCharacter(uid, level); err != nil {
		return nil, fmt.Errorf("outfitSimCharacter: %w", err)
	}
	if err := stockSimConsumables(uid, level); err != nil {
		return nil, fmt.Errorf("stockSimConsumables: %w", err)
	}
	return c, nil
}

// SeedPostgameUnlock makes the synthetic user satisfy postgameUnlocked's
// T5-clear gate by writing two completed, boss-defeated dnd_expedition rows
// (dragons_lair + abyss_portal) into the sim's throwaway DB. Without this the
// P1 gate in startZoneRun rejects every T6 zone ("did not persist after start")
// and the sim can't reach post-game content. Sim/test-only — the live gate is
// unchanged. Idempotent enough for a fresh per-run DB; call once after
// BuildCharacter/PrepareRealCharacter when the target zone IsPostgameZone.
func (s *SimRunner) SeedPostgameUnlock(uid id.UserID) error {
	for i, z := range []ZoneID{ZoneDragonsLair, ZoneAbyssPortal} {
		if _, err := db.Get().Exec(
			`INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, status, boss_defeated)
			 VALUES (?, ?, ?, ?, 1)`,
			fmt.Sprintf("sim-unlock-%s-%d", uid, i), string(uid), string(z), ExpeditionStatusComplete,
		); err != nil {
			return fmt.Errorf("seed T5 clear %s: %w", z, err)
		}
	}
	return nil
}

// IsPostgameZone exports the T6 predicate so the sim binary (a separate
// package) can decide whether to call SeedPostgameUnlock before a run.
func IsPostgameZone(id ZoneID) bool { return isPostgameZone(id) }

// PrepareRealCharacter readies an already-persisted character (loaded from a
// copy of the prod DB) for a headless run. Unlike BuildCharacter it fabricates
// nothing: the character keeps its real race/class/subclass/level, ability
// scores, equipment, spellbook and inventory. It only (a) heals to full — a
// player rests before heading out, so we don't handicap a wounded prod snapshot
// — and (b) tops the bankroll up to `bank` so the "heavy" supply preset always
// affords itself regardless of the player's live coin balance. Returns the
// loaded character. The DB copy is disposable; the live prod file is untouched.
func (s *SimRunner) PrepareRealCharacter(uid id.UserID, bank float64) (*DnDCharacter, error) {
	c, err := LoadDnDCharacter(uid)
	if err != nil {
		return nil, fmt.Errorf("LoadDnDCharacter: %w", err)
	}
	if c == nil {
		return nil, fmt.Errorf("no character for %s in db copy", uid)
	}
	if c.Class == "" {
		return nil, fmt.Errorf("%s has no class (setup incomplete) — nothing to simulate", uid)
	}
	c.HPCurrent = c.HPMax
	c.TempHP = 0
	c.Exhaustion = 0
	c.ShortRestCharges = c.Level
	if err := SaveDnDCharacter(c); err != nil {
		return nil, fmt.Errorf("SaveDnDCharacter: %w", err)
	}
	if bal := s.Euro.GetBalance(uid); bal < bank {
		s.Euro.Credit(uid, bank-bal, "expedition-sim real-char bankroll top-up")
	}
	return c, nil
}

// stockSimConsumables drops a small tier-appropriate bundle of potions
// + a couple offensive items into the synthetic player's inventory so
// SelectConsumables / setupAutoHealFromInventory have something to fire
// during autoDriveCombat. Counts are deliberately modest — a real
// L7+ player typically carries 3-6 heals plus a couple of buffs; we
// mirror that band rather than max-stocking, which would mask class
// power gaps.
func stockSimConsumables(uid id.UserID, level int) error {
	tier := simGearTierForLevel(level)
	bundle := simConsumableBundle(tier)
	for name, qty := range bundle {
		def := consumableDefByName(name)
		if def == nil {
			continue
		}
		for i := 0; i < qty; i++ {
			item := AdvItem{
				Name:  def.Name,
				Type:  "consumable",
				Tier:  def.Tier,
				Value: int64(def.Price),
			}
			if err := addAdvInventoryItem(uid, item); err != nil {
				return err
			}
		}
	}
	return nil
}

// simConsumableBundle returns the name→qty bundle for a given gear tier.
// Counts are deliberately lean — real players don't haul unlimited
// potions, and over-stocking would mask class-power gaps by letting the
// sim heal through any combat. The picker is heal-biased; the offensive
// item gives one boost cycle, not a sustained buff. Inventory rows are
// consumed in-combat by SelectConsumables, so a long expedition will
// run dry on its own.
func simConsumableBundle(tier int) map[string]int {
	switch tier {
	case 1:
		return map[string]int{"Berry Poultice": 2}
	case 2:
		return map[string]int{"Herb Salve": 2, "Coal Bomb": 1}
	case 3:
		return map[string]int{"Herb Salve": 2, "Goblin Grease": 1}
	case 4:
		return map[string]int{"Spirit Tonic": 2, "Sapphire Elixir": 1}
	default: // 5
		return map[string]int{"Spirit Tonic": 3, "Ancient Artifact Oil": 1, "Voidstone Shard": 1}
	}
}

// outfitSimCharacter promotes every adventure_equipment row for uid to
// a tier matched to character level, with full condition. createAdvCharacter
// seeds tier-0 ("Basic Ass Sword" etc) which leaves the sim character
// effectively unarmed; without this step the synthetic player can't beat
// anything above goblin_warrens regardless of stats.
//
// Tier mapping mirrors the zone-tier curve (L1-3 → T1, L4-6 → T2,
// L7-9 → T3, L10-12 → T4, L13+ → T5). It's a "kitted-out at expected
// difficulty" baseline, not a min-max — players past the appropriate
// shop visit should be at or above this band.
// attachSimPet stamps a base housing pet (Massive Dog, no armor) at the
// given level onto the synthetic character via the normal adv-char save
// path, so combat's DerivePlayerStats sees HasPet()==true. Dog vs cat is
// numerically identical today, so type is arbitrary; armor tier stays 0 to
// model the plain "base pet" rather than a kitted one.
func attachSimPet(uid id.UserID, level int) error {
	char, err := loadAdvCharacter(uid)
	if err != nil {
		return err
	}
	char.PetType = "dog"
	char.PetName = "SimDog"
	char.PetLevel = level
	char.PetArmorTier = 0
	char.PetArrived = true
	char.PetChasedAway = false
	char.PetXP = 0
	return saveAdvCharacter(char)
}

func outfitSimCharacter(uid id.UserID, level int) error {
	tier := simGearTierForLevel(level)
	equip, err := loadAdvEquipment(uid)
	if err != nil {
		return err
	}
	for slot, eq := range equip {
		def := equipmentDefByTier(slot, tier)
		eq.Tier = tier
		eq.Condition = 100
		eq.Name = def.Name
		eq.ActionsUsed = 0
		if err := saveAdvEquipment(uid, eq); err != nil {
			return err
		}
	}
	return nil
}

func simGearTierForLevel(level int) int {
	switch {
	case level >= 13:
		return 5
	case level >= 10:
		return 4
	case level >= 7:
		return 3
	case level >= 4:
		return 2
	default:
		return 1
	}
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
	case ClassMage:
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 8, 14, 13, 16, 12, 10
	case ClassSorcerer:
		// Sorcerer is a CHA caster (spellcastingMod → CHA), so its 16 goes in
		// CHA, not INT. Previously it shared the Mage's INT-heavy array, which
		// left the synthetic sorcerer casting at CHA mod 0 — every CHA-scaled
		// ability (cantrip, Innate Sorcery, spell DCs) ran crippled and sorc
		// trailed the field in every sweep. Prod players always placed 16 in
		// CHA; this makes the sim's sorcerer match a real one.
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 8, 14, 13, 10, 12, 16
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
	Outcome   string // "extracted" | "cleared" | "tpk" | "leader_down" | "fled" | "boss_doorway" | "fork" | "halted" | "ongoing"
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
	// YieldCount is the total number of material rows the synthetic
	// player banked across the expedition (one row per +1 of any
	// resource). YieldsByName breaks that total down by resource name
	// for tier/per-resource calibration. Both are read from
	// adventure_inventory at end-of-run.
	YieldCount   int
	YieldsByName map[string]int
	// Combats holds a per-combat trace for every fight the synthetic
	// player entered during the expedition (boss + elites + patrols).
	// Used by post-hoc analysis to dig into class-survival walls
	// without re-running the matrix. Populated from combat_sessions
	// rows + their TurnLog at end-of-run.
	Combats []SimCombatSummary
	// PartySize is the seated roster (1 for a solo run). Members carries a
	// per-seat projection for the followers; the leader's own numbers stay
	// on the top-level fields so a solo row is byte-shaped as it always was.
	PartySize int
	// LeaderAlive is read off the AdventureCharacter death flag, not HP — the
	// close-out after a loss leaves HP in no particular state.
	LeaderAlive bool
	Members     []SimMemberStat `json:",omitempty"`
	// DaySnapshots traces HP/SU/threat/rooms at every day rollover
	// (Night camp) plus the start (Day 0) and the final state. Used by
	// D7-c long-expedition baselining to see how the trajectory bends
	// across multi-day runs without scrubbing the log.
	DaySnapshots []SimDaySnapshot
	Log          []SimLogEntry
}

// SimDaySnapshot is a point-in-time projection of the sim state at a
// day-rollover boundary. Day 0 is captured at expedition start; every
// subsequent entry lands right after a Night camp lands (CurrentDay
// already incremented). A final entry is appended at end-of-run.
type SimDaySnapshot struct {
	Day       int
	HPCurrent int
	HPMax     int
	Supplies  float32
	Threat    int
	Rooms     int // cumulative autopilot rooms walked at snapshot time
}

// SimMemberStat is one follower's slice of a party run. The leader is not
// listed here — their numbers are the SimResult's own top-level fields.
// Alive is what a T5 band reading actually turns on: a party "clears" when
// the boss dies, but a member who died on the way still paid for it.
type SimMemberStat struct {
	UserID  string
	Class   string
	Level   int
	StartHP int
	EndHP   int
	Alive   bool
}

// SimCombatSummary is a compact per-fight trace: the entry stats, the
// per-round damage dealt by each side, and the outcome. Lets J-phase
// analysis ask "did Fighter L12 hit the manor boss often enough?"
// without re-running the matrix.
type SimCombatSummary struct {
	SessionID    string
	EncounterID  string
	EnemyID      string
	Status       string // active/won/lost/fled/expired
	Rounds       int
	PlayerHPMax  int
	PlayerHPEnd  int
	EnemyHPMax   int
	EnemyHPEnd   int
	PlayerDamage int // total damage dealt by player across the fight
	EnemyDamage  int // total damage dealt by enemy across the fight
	PlayerHits   int // d20 rolls by player that landed (>= enemy AC)
	PlayerMisses int
	EnemyHits    int
	EnemyMisses  int
	PlayerAC     int // inferred from RollAgainst on enemy attack events
	EnemyAC      int // inferred from RollAgainst on player attack events
	// Events is the raw per-round TurnLog. Populated only when
	// SetSimIncludeTrace(true) has been called, and only on the LAST
	// combat per expedition (the boss room) to keep JSONL size bounded.
	// Used by J2 caster-survival analysis.
	Events []simTraceEvent `json:",omitempty"`
}

// simTraceEvent is CombatEvent with the seat ALWAYS emitted.
//
// CombatEvent.Seat is `omitempty` for wire-compat reasons that are correct for
// persistence and wrong for a trace: seat 0 and "no seat" serialize identically,
// so a party trace silently reads as a solo fight. That is not hypothetical — it
// is what made a hired companion who never took a turn look, in the JSON, like a
// fight that only ever had one seat, and it cost an hour of chasing the wrong bug.
// A diagnostic that cannot tell you who acted is not a diagnostic.
type simTraceEvent struct {
	Round       int
	Seat        int // always present, even when 0
	Phase       string
	Actor       string
	Action      string
	Damage      int
	PlayerHP    int
	EnemyHP     int
	Roll        int
	RollAgainst int
	Desc        string `json:",omitempty"`
}

// simTraceEvents converts a TurnLog for the trace.
func simTraceEvents(events []CombatEvent) []simTraceEvent {
	out := make([]simTraceEvent, len(events))
	for i, e := range events {
		out[i] = simTraceEvent{
			Round: e.Round, Seat: e.Seat, Phase: e.Phase, Actor: e.Actor, Action: e.Action,
			Damage: e.Damage, PlayerHP: e.PlayerHP, EnemyHP: e.EnemyHP,
			Roll: e.Roll, RollAgainst: e.RollAgainst, Desc: e.Desc,
		}
	}
	return out
}

// simIncludeTrace gates per-round event capture on SimCombatSummary.
// Off by default — matrix runs already emit megabytes of summary rows
// and the raw turn log multiplies that. SetSimIncludeTrace flips it on
// for targeted J2-style diagnostic sweeps.
var simIncludeTrace = false

// SetSimIncludeTrace toggles inclusion of the raw per-round CombatEvent
// stream on the LAST SimCombatSummary of each expedition (the boss
// room). Callers should flip this on before BuildCharacter / RunExpedition.
func SetSimIncludeTrace(on bool) { simIncludeTrace = on }

// simPetLevel, when > 0, attaches a base housing pet (Massive Dog, no
// armor) at that level to every synthetic character. 0 (the default) leaves
// the character petless, matching prod char-creation. Pets are otherwise
// unreachable in the sim — synthetic chars never trigger the arrival flow —
// so this is the only way to exercise the per-round pet attack / deflect /
// whiff path for balance measurement. Combat reads pet stats off the
// AdventureCharacter (see DerivePlayerStats), so BuildCharacter stamps the
// fields there, not on the DnDCharacter.
var simPetLevel = 0

// SetSimPetLevel attaches a base pet at the given level (1-10) to sim
// characters. 0 disables. Flip before BuildCharacter.
func SetSimPetLevel(level int) { simPetLevel = level }

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

// simWalkInterval — how much synthetic time each autopilot walk
// represents. Matches the production autorun cooldown so the
// nightCampWindow check inside decideAutopilotCamp lands on the same
// real-time cadence: ~8 walks ≈ 16h ≈ one Night-camp rollover.
const simWalkInterval = autoRunCooldown

// RunExpedition starts an expedition for uid in zoneID and loops the
// autopilot walk (compact mode, so elite rooms auto-resolve inline)
// until a hard stop fires. Between walks it advances a synthetic clock
// (simWalkInterval per walk) and calls into the same maybeAutoCamp /
// pitchBossSafetyCamp scheduler the production autorun ticker uses, so
// HP-low mid-day rests + Night-camp rollovers fire under the sim.
//
// walkCap bounds the number of autopilot bursts as a safety net. Each
// burst walks up to autopilotRoomCap rooms.
//
// maxDays, when > 0, stops the run once res.DayTicks reaches that count
// — used by D7-c long-expedition baselining to bound multi-day runs by
// synthetic day count rather than walk count. 0 leaves the run
// unbounded by days (the walkCap safety net still applies).
//
// Pre-state: uid must own a synthetic character via BuildCharacter and
// have a coin balance sufficient for outfitting (caller's responsibility).
func (s *SimRunner) RunExpedition(uid id.UserID, zoneID ZoneID, walkCap, maxDays int) (*SimResult, error) {
	return s.RunPartyExpedition(uid, nil, zoneID, walkCap, maxDays)
}

// RunPartyExpedition is RunExpedition with followers. uid leads; every id in
// members is invited and accepts on Day 1, through the same
// `!expedition invite` / `!expedition accept` commands a player types — so the
// sim measures the seating rules (tier gate, busy guard, supply pooling) rather
// than a hand-built roster that cannot fail. Each member must already own a
// character and a bankroll.
//
// A nil members list is the solo path, unchanged and bit-identical: no invite is
// sent, no roster row is written, and `sess.IsParty()` stays false all the way
// down into the turn engine. That is what keeps the d8prereq_corpus baselines
// replayable.
func (s *SimRunner) RunPartyExpedition(uid id.UserID, members []id.UserID, zoneID ZoneID, walkCap, maxDays int) (*SimResult, error) {
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
	// D5-b made a bare "start <zone>" return the loadout prompt without
	// outfitting. Force the tier-max "heavy" preset so multi-day runs
	// have enough supplies to actually exercise [[project-sim-event-anchored-broken]] rollovers.
	if err := s.P.handleDnDExpeditionCmd(ctx, "start "+string(zoneID)+" heavy"); err != nil {
		return res, fmt.Errorf("expedition start: %w", err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp == nil {
		res.Outcome = "halted"
		return res, fmt.Errorf("expedition did not persist after start")
	}
	// Seat the followers before the first walk — the invite window is Day 1,
	// and their packs have to reach the pool before SUStart is read.
	if len(members) > 0 {
		if err := s.seatParty(uid, members); err != nil {
			res.Outcome = "halted"
			res.StopCode = "party:" + err.Error()
			return res, err
		}
		if exp, _ = getActiveExpedition(uid); exp == nil {
			res.Outcome = "halted"
			return res, fmt.Errorf("expedition vanished while seating the party")
		}
	}
	// Hire the companion on Day 1, after the humans are seated so the role fill
	// sees the party it is filling a hole in. He is not in `members` — he buys no
	// packs and takes no seat in the human roster — but fightRoster reads
	// expeditionSeats, so he shows up in every fight from here on.
	if s.Companion != "" {
		class, ok := parseCompanionClass(s.Companion)
		if !ok {
			class = companionRoleFill(companionPartyClasses(exp.ID))
		}
		if err := hireCompanion(exp.ID, class, companionPartyLevel(exp.ID)); err != nil {
			res.Outcome = "halted"
			res.StopCode = "companion:" + err.Error()
			return res, err
		}
	}

	roster := append([]id.UserID{uid}, members...)
	res.PartySize = len(roster)
	for _, m := range members {
		stat := SimMemberStat{UserID: string(m)}
		if mc, _ := LoadDnDCharacter(m); mc != nil {
			stat.Class, stat.Level, stat.StartHP = string(mc.Class), mc.Level, mc.HPCurrent
		}
		res.Members = append(res.Members, stat)
	}
	res.SUStart = exp.Supplies.Current
	// Day-0 baseline so the snapshot stream always opens with a known
	// starting state, even if the run halts before the first rollover.
	s.captureDaySnapshot(res, exp, uid)

	// Synthetic clock — anchored on the expedition's real start_date so
	// nightCampWindow / nightSafetyNet comparisons against LastBriefingAt
	// stay coherent with the rows the autopilot scheduler reads.
	simNow := exp.StartDate

	for i := 0; i < walkCap; i++ {
		simNow = simNow.Add(simWalkInterval)
		walk := s.P.runAutopilotWalk(ctx, autopilotRoomCap, true, simInlineBossCombat())
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
			// stopEnded means "the run is over", which is not the same as
			// "everyone died": resolveCombatRoom ends the run on a *timeout*
			// loss too, and deliberately does not mark that player dead (it is
			// mechanically a retreat). Ask the death flags rather than assume.
			res.Outcome = simRunEndOutcome(roster)
			i = walkCap // exit
		case stopComplete:
			// A stopComplete that reaches the sim is normally a full zone
			// clear (the walk auto-advances mid-zone region boundaries
			// internally). But a mid-zone region clear can still surface
			// here — e.g. the walk's auto-advance hit a transit error and
			// returned stopComplete. Mirror runAutopilotWalk: if the
			// expedition is still active in a multi-region zone with a
			// next region, cross into it and keep simulating instead of
			// scoring a premature "cleared".
			if fresh, ferr := getActiveExpedition(uid); ferr == nil && fresh != nil &&
				IsMultiRegionZone(fresh.ZoneID) {
				if cur, ok := CurrentRegion(fresh); ok {
					if next, ok := nextRegion(fresh.ZoneID, cur.ID); ok {
						if _, terr := s.P.advanceToNextRegion(uid, fresh, cur, next); terr != nil {
							res.Outcome = "halted"
							res.StopCode = "region_transit:" + terr.Error()
							i = walkCap
							break
						}
						exp = fresh
						break // continue the outer walk loop in the next region
					}
				}
			}
			res.Outcome = "cleared"
			i = walkCap
		case stopBoss, stopElite:
			// Auto-resolve the encounter: !fight to open, then !attack
			// per round until the session resolves (won / lost / fled).
			killed, err := s.P.autoDriveCombat(ctx)
			if err != nil {
				res.Outcome = "halted"
				res.StopCode = "combat:" + err.Error()
				i = walkCap
				break
			}
			if !killed {
				// The party lost or fled — autopilot can't continue past the
				// gate.
				res.Outcome = simRunEndOutcome(roster)
				i = walkCap
			} else {
				// Combat won — a competent prod player would short-rest
				// between fights when wounded or down on slots (H5 added
				// the partial slot refresh). Without this the sim
				// under-counts caster mid-expedition staying power. Every
				// seat rests: charges and slots are character-scoped, and a
				// member who never rested would drag the party's numbers. A seat
				// that died in the fight the party just won is skipped:
				// handleDnDShortRest does not gate on the death flag, so resting
				// a corpse heals it back above 0 and the casualty disappears
				// from res.Members[i].EndHP.
				for _, m := range roster {
					if !simSeatAlive(m) {
						continue
					}
					s.maybeShortRest(MessageContext{Sender: m}, m)
				}
			}
			// Boss kill closes the run via combat resolution → continue
			// the loop so the next walk picks up the cleared-run state.
		case stopFork:
			// Deterministic sim policy: take the first UNLOCKED path. The
			// old blind "go 1" stalled forever on all-skill-check forks
			// (feywild fork1) — resolveForkChoice rejects a locked edge but
			// zoneCmdGo swallows it as a sent-DM with a nil return, so the
			// run never advanced and burned every walk at the same node. A
			// real player reads the menu and picks a passable path; mirror
			// that. choice==0 means every edge is locked (a graph soft-lock
			// the author must fix) — halt loudly rather than spin.
			choice := s.firstUnlockedForkChoice(ctx.Sender)
			if choice == 0 {
				res.Outcome = "halted"
				res.StopCode = "fork_all_locked"
				i = walkCap
				break
			}
			if err := s.P.handleDnDExpeditionCmd(ctx, fmt.Sprintf("go %d", choice)); err != nil {
				res.Outcome = "halted"
				res.StopCode = "fork:" + err.Error()
				i = walkCap
				break
			}
		case stopBlocked:
			res.Outcome = "blocked"
			i = walkCap
		case stopBossSafety:
			// Compact autopilot bailed before the boss (HP/SU gate). Mirror
			// the production autorun: force-pitch a rest camp, dwell, then
			// break it so the next walk re-engages the boss.
			fresh, _ := getActiveExpedition(uid)
			if fresh == nil {
				res.Outcome = "extracted"
				i = walkCap
				break
			}
			advanced, err := s.applyAutoCampBossSafety(fresh, &simNow)
			if err != nil {
				res.Outcome = "halted"
				res.StopCode = "boss_safety_camp:" + err.Error()
				i = walkCap
				break
			}
			if advanced {
				res.DayTicks++
				if fresh2, _ := getActiveExpedition(uid); fresh2 != nil {
					s.captureDaySnapshot(res, fresh2, uid)
				}
			}
			if exp, _ = getActiveExpedition(uid); exp == nil {
				res.Outcome = "extracted"
				i = walkCap
			}
			if maxDays > 0 && res.DayTicks >= maxDays {
				if res.Outcome == "" {
					res.Outcome = "day_capped"
				}
				i = walkCap
			}
		default:
			// stopOK / stopPreflight / stopHarvestCombat — soft stops.
			// Run the same camp scheduler the production autorun fires
			// after every walk. With simNow past nightCampWindow the
			// pitch is a Night camp and drives the day rollover; below
			// that, an HP-low rest may fire mid-day.
			fresh, _ := getActiveExpedition(uid)
			if fresh == nil {
				res.Outcome = "extracted"
				i = walkCap
				break
			}
			advanced, err := s.applyAutoCamp(fresh, &simNow)
			if err != nil {
				res.Outcome = "halted"
				res.StopCode = "auto_camp:" + err.Error()
				i = walkCap
				break
			}
			if advanced {
				res.DayTicks++
				if fresh2, _ := getActiveExpedition(uid); fresh2 != nil {
					s.captureDaySnapshot(res, fresh2, uid)
				}
			}
			// maybeAutoCamp's drift step may have force-extracted (starvation).
			if exp, _ = getActiveExpedition(uid); exp == nil {
				res.Outcome = "extracted"
				i = walkCap
			}
			if maxDays > 0 && res.DayTicks >= maxDays {
				if res.Outcome == "" {
					res.Outcome = "day_capped"
				}
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
	for i, m := range members {
		if mc, _ := LoadDnDCharacter(m); mc != nil {
			res.Members[i].EndHP = mc.HPCurrent
		}
		res.Members[i].Alive = simSeatAlive(m)
	}
	res.LeaderAlive = simSeatAlive(uid)
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
	res.YieldCount, res.YieldsByName = simMaterialYields(uid)
	res.Combats = simCombatSummaries(uid)
	// Final snapshot. Re-read the expedition so closed-run state is
	// visible (extracted runs return nil from getActiveExpedition; the
	// row is still on disk via mostRecentExpeditionID). If the
	// expedition row is gone we synthesize from the cached SU/threat
	// already on res so the snapshot stream always closes.
	if exp2, _ := getActiveExpedition(uid); exp2 != nil {
		s.captureDaySnapshot(res, exp2, uid)
	} else if past := mostRecentExpeditionID(uid); past != "" {
		if exp3, _ := getExpedition(past); exp3 != nil {
			s.captureDaySnapshot(res, exp3, uid)
		}
	}
	return res, nil
}

// seatParty walks each follower through `!expedition invite` → `!expedition
// accept heavy`, as the leader and then as themselves. Both commands answer a
// refusal with a DM and a nil error — a no-op against the sim's nil client — so
// the seat is verified against the roster afterwards rather than the return
// value. A follower who cannot sit down (tier gate, empty purse, a run of their
// own) is a halted run, not a quietly smaller party: a two-player T5 reading
// taken from a solo walk is worse than no reading.
func (s *SimRunner) seatParty(leader id.UserID, members []id.UserID) error {
	exp, err := getActiveExpedition(leader)
	if err != nil || exp == nil {
		return fmt.Errorf("no expedition to seat: %w", err)
	}
	for _, m := range members {
		if err := s.P.handleDnDExpeditionCmd(MessageContext{Sender: leader},
			"invite "+string(m)); err != nil {
			return fmt.Errorf("invite %s: %w", m, err)
		}
		if err := s.P.handleDnDExpeditionCmd(MessageContext{Sender: m},
			"accept "+simPartyLoadout); err != nil {
			return fmt.Errorf("accept %s: %w", m, err)
		}
	}
	size, err := partySize(exp.ID)
	if err != nil {
		return fmt.Errorf("read roster: %w", err)
	}
	if want := len(members) + 1; size != want {
		return fmt.Errorf("roster seated %d of %d — a follower was refused", size, want)
	}
	return nil
}

// simPartyLoadout mirrors the leader's own "heavy" purchase in RunExpedition:
// each member buys the tier-max pack, and the pool is their sum. Anything less
// and the party starves before the boss, which would read as a difficulty
// finding rather than the supply-budget artifact it is.
const simPartyLoadout = "heavy"

// simSeatAlive reports whether a seat is still standing. Death is recorded on
// the AdventureCharacter (markAdventureDead → Kill), not as HPCurrent <= 0: the
// close-out that follows a loss can leave HP anywhere, and a player who merely
// fled at 0 HP is not dead. Reading the death flag is the only honest answer.
func simSeatAlive(uid id.UserID) bool {
	c, err := loadAdvCharacter(uid)
	if err != nil || c == nil {
		return false
	}
	return c.Alive
}

// simRunEndOutcome names how a run ended once the walk says it is over. The
// three cases are distinct and the distinction is the whole point of a party
// reading:
//
//	tpk         — every seat is dead. The turn engine only reports Lost once
//	              anyAlive() goes false, so a real party loss marks the roster.
//	leader_down — the leader died and members are still standing. Only the
//	              leader owns the run and expedition rows, so their death tears
//	              both down; inline room/patrol combat is fought by the leader
//	              alone, which is how a party reaches this state with untouched
//	              members (N3/P7 finding).
//	fled        — the run ended with the leader alive: a timeout loss (a
//	              mechanical retreat, deliberately not a death) or a forced
//	              extract.
//
// For a solo roster leader-dead and all-dead are the same predicate, so a solo
// run still scores exactly "tpk" or "fled" as it always did.
func simRunEndOutcome(roster []id.UserID) string {
	leaderAlive := simSeatAlive(roster[0])
	if leaderAlive {
		return "fled"
	}
	for _, m := range roster[1:] {
		if simSeatAlive(m) {
			return "leader_down"
		}
	}
	return "tpk"
}

// firstUnlockedForkChoice returns the 1-based index of the first
// traversable option at the pending fork, or 0 if every edge is locked
// (a graph soft-lock — see feywild fork1, which had no LockNone exit).
func (s *SimRunner) firstUnlockedForkChoice(uid id.UserID) int {
	run, err := getActiveZoneRun(uid)
	if err != nil || run == nil {
		return 1
	}
	pf, err := decodePendingFork(run.NodeChoices)
	if err != nil || pf == nil {
		return 1
	}
	for _, o := range pf.Options {
		if o.Unlocked {
			return o.Index
		}
	}
	return 0
}

// captureDaySnapshot appends a SimDaySnapshot reflecting current state.
// HP is read from the live character row; SU/threat/day from the live
// expedition. Rooms is the running res.Rooms counter.
func (s *SimRunner) captureDaySnapshot(res *SimResult, exp *Expedition, uid id.UserID) {
	snap := SimDaySnapshot{
		Day:      exp.CurrentDay,
		Supplies: exp.Supplies.Current,
		Threat:   exp.ThreatLevel,
		Rooms:    res.Rooms,
	}
	if c, _ := LoadDnDCharacter(uid); c != nil {
		snap.HPCurrent = c.HPCurrent
		snap.HPMax = c.HPMax
	}
	res.DaySnapshots = append(res.DaySnapshots, snap)
}

// simCombatSummaries pulls every combat_sessions row for uid and folds
// its TurnLog into a SimCombatSummary. AC values are inferred from the
// RollAgainst column on attack events (the engine writes the defender's
// AC there). Rows are ordered by started_at so the boss fight is last.
func simCombatSummaries(uid id.UserID) []SimCombatSummary {
	rows, err := db.Get().Query(`
		SELECT session_id, encounter_id, enemy_id, status, round,
		       player_hp, player_hp_max, enemy_hp, enemy_hp_max,
		       turn_log_json
		  FROM combat_session
		 WHERE user_id = ?
		 ORDER BY started_at ASC`, string(uid))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []SimCombatSummary
	var lastEvents []CombatEvent
	for rows.Next() {
		var s SimCombatSummary
		var turnLogJSON string
		if err := rows.Scan(
			&s.SessionID, &s.EncounterID, &s.EnemyID, &s.Status, &s.Rounds,
			&s.PlayerHPEnd, &s.PlayerHPMax, &s.EnemyHPEnd, &s.EnemyHPMax,
			&turnLogJSON); err != nil {
			continue
		}
		var events []CombatEvent
		if turnLogJSON != "" {
			_ = json.Unmarshal([]byte(turnLogJSON), &events)
		}
		lastEvents = events
		for _, ev := range events {
			switch ev.Actor {
			case "player":
				if ev.Roll > 0 {
					if ev.Damage > 0 {
						s.PlayerHits++
					} else {
						s.PlayerMisses++
					}
					if ev.RollAgainst > s.EnemyAC {
						s.EnemyAC = ev.RollAgainst
					}
				}
				if ev.Damage > 0 {
					s.PlayerDamage += ev.Damage
				}
			case "enemy":
				if ev.Roll > 0 {
					if ev.Damage > 0 {
						s.EnemyHits++
					} else {
						s.EnemyMisses++
					}
					if ev.RollAgainst > s.PlayerAC {
						s.PlayerAC = ev.RollAgainst
					}
				}
				if ev.Damage > 0 {
					s.EnemyDamage += ev.Damage
				}
			}
		}
		out = append(out, s)
	}
	if simIncludeTrace && len(out) > 0 {
		out[len(out)-1].Events = simTraceEvents(lastEvents)
	}
	return out
}

// simMaterialYields tallies the material rows the synthetic player
// banked in adventure_inventory. One row per +1, so the count is the
// raw resource-unit total. Map breakdown is keyed by resource name.
func simMaterialYields(uid id.UserID) (int, map[string]int) {
	rows, err := db.Get().Query(`
		SELECT name, COUNT(*)
		  FROM adventure_inventory
		 WHERE user_id = ? AND item_type = 'material'
		 GROUP BY name`, string(uid))
	if err != nil {
		return 0, nil
	}
	defer rows.Close()
	out := make(map[string]int)
	total := 0
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return total, out
		}
		out[name] = n
		total += n
	}
	return total, out
}

// autoDriveCombat dispatches !fight at the current elite/boss gate,
// then loops !attack/!cast/!consume until the combat session resolves.
// Returns true when the player won (enemy dead, room cleared), false when
// the player lost or fled. autoCombatRoundCap is a safety cap against
// pathological stalemates (shouldn't trigger in practice — combat is
// strictly monotone in HP).
//
// Shared by the headless sim and the production background autopilot
// (long-expedition D8-f): a silent ctx (ctx.Silent) suppresses the
// per-round DM narration so the autorun digest can summarize the fight
// without spamming the player a message per round. Driving the real
// !fight/!attack engine here is what gives autopilot bosses true parity
// with manual `!fight` — including enemy multiattack, which the old
// inline SimulateCombat path ignored.
const autoCombatRoundCap = 200

func (p *AdventurePlugin) autoDriveCombat(ctx MessageContext) (bool, error) {
	if err := p.handleFightCmd(ctx); err != nil {
		return false, fmt.Errorf("fight: %w", err)
	}
	sess, err := activeCombatSessionFor(ctx.Sender)
	if err != nil {
		return false, fmt.Errorf("getActiveCombatSession: %w", err)
	}
	if sess == nil {
		// No session opened (bestiary miss, doorway already cleared).
		return false, nil
	}
	// The cap counts dispatches, and a party spends one per seat per round. Scale
	// it by the roster so a three-player boss fight gets the same 200 rounds of
	// headroom a solo one does; solo multiplies by 1 and is unchanged.
	sessionID := sess.SessionID
	dispatchCap := autoCombatRoundCap * sess.RosterSize()
	for i := 0; i < dispatchCap; i++ {
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

		// Every combat command is refused to anyone but the seat on the clock, so
		// a party fight has to be driven as whoever that is. Solo skips the lookup
		// entirely: seat 0 is the only seat, and the sender is already sitting in
		// it — the bit-identical path the balance corpus rests on.
		turn := ctx
		seat := 0
		if cur.IsParty() {
			seat, err = p.actingSeatForAutopilot(cur)
			if err != nil {
				return false, fmt.Errorf("iter %d: %w", i, err)
			}
			turn.Sender = id.UserID(cur.seatUserID(seat))
		}

		// An engine-driven seat is never dispatched as a chat command. There is no
		// character for the handlers to load, and a command arriving from that
		// seat's id reads to beginCombatTurn as a player returning to the keyboard
		// — which is how the driver used to clear the very latch that was moving
		// the seat. Drive it directly instead.
		if cur.seatIsEngineDriven(seat) {
			if err := p.driveEngineSeat(cur, seat); err != nil {
				return false, fmt.Errorf("engine seat %d: %w", seat, err)
			}
			continue
		}

		kind, arg := p.pickAutoCombatActionForSeat(turn.Sender, cur, seat)
		var dispatchErr error
		switch kind {
		case "consume":
			dispatchErr = p.handleConsumeCmd(turn, arg)
		case "cast":
			dispatchErr = p.handleCombatCastCmd(turn, arg)
		default:
			dispatchErr = p.handleAttackCmd(turn)
		}
		if dispatchErr != nil {
			return false, fmt.Errorf("%s iter %d: %w", kind, i, dispatchErr)
		}
	}
	return false, fmt.Errorf("combat exceeded %d dispatches (%d seats × %d rounds)",
		dispatchCap, sess.RosterSize(), autoCombatRoundCap)
}

// actingSeatForAutopilot names the seat autoDriveCombat must act as next. The
// session it reads has already been settled by the command that parked it here,
// so it is sitting on a live player's turn; a session that is not is a bug in
// the engine, not a state the autopilot should paper over by spinning to the
// round cap.
func (p *AdventurePlugin) actingSeatForAutopilot(sess *CombatSession) (int, error) {
	players, enemy, err := p.partyCombatantsForSession(sess)
	if err != nil {
		return 0, fmt.Errorf("rebuild party fight: %w", err)
	}
	seat, waiting := actingSeat(sess, players, enemy)
	if !waiting {
		return 0, fmt.Errorf("combat session %s: active but waiting on nobody (phase %q)",
			sess.SessionID, sess.Phase)
	}
	return seat, nil
}

// simShortRestHPPct is the HP-percentage threshold below which the sim
// pulls the short-rest lever after a combat. 60% mirrors the
// "competent prod player" model: rest when you've taken real damage and
// have charges to spend, but don't burn charges on scratches.
const simShortRestHPPct = 60

// maybeShortRest fires a short rest after a won combat when the
// character has charges and either (a) HP is below simShortRestHPPct or
// (b) any spell slots have been used. Mirrors what a prod player would
// type between rooms; H5 (commit 1cd53eb) made this slot-recovering for
// casters. Errors are swallowed — short rest is best-effort QoL and a
// failure should not abort the sim run.
func (s *SimRunner) maybeShortRest(ctx MessageContext, uid id.UserID) {
	c, _ := LoadDnDCharacter(uid)
	if c == nil || c.ShortRestCharges <= 0 {
		return
	}
	needsHP := c.HPMax > 0 && c.HPCurrent*100 < c.HPMax*simShortRestHPPct
	hasUsedSlots, _ := casterHasUsedSlots(uid)
	if !needsHP && !hasUsedSlots {
		return
	}
	_ = s.P.handleDnDShortRest(ctx)
	// handleDnDShortRest sets RestingUntil = now+1h. The autopilot walk
	// doesn't gate on that, but TickDay's overnight camp / future zone
	// transitions might — clear it so the sim isn't accidentally
	// locked out of state it would otherwise reach.
	if c2, _ := LoadDnDCharacter(uid); c2 != nil && c2.RestingUntil != nil {
		c2.RestingUntil = nil
		_ = SaveDnDCharacter(c2)
	}
}

// simHealHPThresholdPct is the player-HP percentage below which the sim
// reaches for a heal consumable before its !attack/!cast. 40% mirrors
// the "competent player" prod assumption — heal early enough to absorb
// one more big hit, not so early that a 1-HP scratch burns a potion.
const simHealHPThresholdPct = 40

// pickAutoCombatAction is the per-turn decision tree, mirroring
// what a competent prod player would type:
//
//  1. If HP is below simHealHPThresholdPct and the inventory has a heal
//     consumable, !consume <heal>. (Slot-spell heals were tried in J2c
//     but regressed druid by 10pp — slot heals like cure_wounds heal
//     ~10HP vs 40HP from a tier-4 Spirit Tonic, and burned a slot the
//     caster needed for damage spells. Consumable-first wins.)
//  2. Else if the character is a non-martial-first spellcaster with a
//     usable damage spell + slot (or a damaging cantrip), !cast it.
//     Higher-slot damage outranks lower-slot damage. (J2c also tried
//     scoring control spells; net ±0 vs J2b in the n=100 sweep, and
//     a higher control weight regressed warlock by 6.6pp — control
//     scoring was reverted. Bard/cleric trailing remains a class-pool
//     issue, not a picker issue.)
//  3. Else !attack.
//
// Pre-J2a the sim looped !attack only, which underweighted every caster
// class — see sim_results/j2_findings.md for the trace evidence.
func (p *AdventurePlugin) pickAutoCombatAction(uid id.UserID, sess *CombatSession) (kind, arg string) {
	return p.pickAutoCombatActionForSeat(uid, sess, 0)
}

// pickAutoCombatActionForSeat is pickAutoCombatAction for an arbitrary seat.
// The decision tree reads HP and the running concentration aura, and both are
// per-character — before N3/P5 they were read straight off the session row,
// which is seat 0. Driving a party member's turn off the leader's HP would heal
// the wrong person and re-arm the wrong aura.
func (p *AdventurePlugin) pickAutoCombatActionForSeat(uid id.UserID, sess *CombatSession, seat int) (kind, arg string) {
	if sess == nil {
		return "attack", ""
	}
	// seatPickSheet, not LoadDnDCharacter: the hired companion has no character row
	// and never will, so the raw load returned nil for him and this function bailed
	// to "attack" on its first line — every turn, for the whole fight. That one line
	// is why a hired Cleric swung a mace while the party died. He fights off the
	// same synthetic sheet the rest of his seat is built from.
	c := seatPickSheet(sess, uid)
	if c == nil {
		return "attack", ""
	}
	st := sess.actorStatusesForSeat(seat)
	hp, hpMax := sess.seatHP(seat), sess.seatHPMax(seat)
	lowHP := hpMax > 0 && hp*100 < hpMax*simHealHPThresholdPct
	if lowHP {
		inv := p.loadConsumableInventory(uid)
		for _, it := range inv {
			if it.Def.Effect == EffectHeal {
				return "consume", it.Def.Name
			}
		}
	}
	// §1 — a healer with a hurt friend heals the friend. This is what a competent
	// player does, and until the engine could target another seat it was not an
	// option the picker even had. It runs before the damage picks: a party member
	// about to die is a more urgent use of a slot than another Fireball.
	// A healer heals whoever is worst off — the friend bleeding out beside them, or
	// themselves. An empty target is a self-cast, and `!cast <spell>` with no
	// @mention is exactly how a player writes that.
	if id, target := simPickHeal(c, uid, sess, seat); id != "" {
		if target == "" {
			return "cast", id
		}
		return "cast", id + " @" + target
	}

	if isSpellcaster(c) && !simMartialFirstClass(c.Class) {
		// Cleric: Spiritual Weapon is a BuffSelf that fires a spectral
		// bonus-action attack each round via SpiritWeaponProc/Dmg mods —
		// simPickSpell skips BuffSelf entries by design, so a cleric
		// otherwise never spends an L2 slot on it. Force the pick once
		// per fight (BuffSpiritProc==0) so the picker doesn't pretend
		// it's not a damage option.
		if id := simPickSpiritualWeapon(c, sess, seat, uid, st); id != "" {
			return "cast", id
		}
		// Once a concentration aura is up, a competent caster maintains it and
		// attacks (or casts a non-concentration spell) rather than burning a
		// slot to re-arm the same aura — so the picker excludes concentration
		// spells while one is active.
		auraActive := st.ConcentrationDmg > 0
		if id := simPickSpell(c, sess, seat, uid, auraActive); id != "" {
			return "cast", id
		}
	}
	return "attack", ""
}

// simMartialFirstClass marks the half-casters whose damage identity is
// weapon-attack + ExtraAttack rather than slot spells. Picker skips
// !cast for these — Ranger's weapon swing with Hunter's Mark + Extra
// Attack out-damages a single Lightning Arrow at L12, and the J2b
// re-baseline measured a 35pp regression when the picker burned slots
// on damage_save spells for them. Paladin currently has no damage-
// effect spells in defaults (kit is bless/cure/buff), so the picker is
// already a no-op there; listed here for intent.
func simMartialFirstClass(class DnDClass) bool {
	switch class {
	case ClassRanger, ClassPaladin:
		return true
	}
	return false
}

// simPickSpiritualWeapon returns a !cast argument for Spiritual Weapon
// when a cleric should open the fight with it: the buff is not already
// active on the session, the spell is prepared, and some slot ≥ L2 is
// available. Returns "" otherwise. The buff path in combat_cmd.go folds
// into BuffSpiritProc/Dmg via applyBuffDelta, which the turn engine fires
// each round via spiritWeaponStrike — so one cast is worth more than a
// single L2 damage spell across a multi-round fight.
//
// Slot pick: lowest available slot ≥ 2. Upcasting is +1d8 per 2 slots
// above 2nd, so spending a precious L5 to add a single d8 to the proc is
// not worth burning the bigger slot's damage potential elsewhere; sim
// behaves like a competent player and saves the high slot.
func simPickSpiritualWeapon(c *DnDCharacter, sess *CombatSession, seat int, uid id.UserID, st ActorStatuses) string {
	if c == nil || c.Class != ClassCleric {
		return ""
	}
	if st.BuffSpiritProc > 0 {
		return ""
	}
	known, err := seatKnownSpells(sess, seat, uid)
	if err != nil {
		return ""
	}
	prepared := false
	for _, k := range known {
		if k.SpellID == "spiritual_weapon" && k.Prepared {
			prepared = true
			break
		}
	}
	if !prepared {
		return ""
	}
	slots, _ := seatSpellSlots(sess, seat, uid)
	const simMaxSlot = 5
	for sl := 2; sl <= simMaxSlot; sl++ {
		pair, ok := slots[sl]
		if !ok || pair[0]-pair[1] <= 0 {
			continue
		}
		if sl == 2 {
			return "spiritual_weapon"
		}
		return fmt.Sprintf("spiritual_weapon --upcast %d", sl)
	}
	return ""
}

// simPickSpell returns the spell argument a competent player would pass
// to !cast this turn, or "" when no usable spell is available (forcing a
// !attack). The return value is fed straight to handleCombatCastCmd, so
// upcast picks come back as `"<spell_id> --upcast N"`.
// Selection rules:
//   - Only damage-effect spells (damage_attack / damage_save / damage_auto).
//     Control/buff/heal are out (J2c sweep showed control scoring at
//     either 22 or 5 nets ≤±2pp at L12 and regresses warlock at 22 —
//     no headroom worth the complexity). Healing is handled by the
//     consumable-first branch in simPickCombatAction.
//   - Reaction-cast spells are excluded (engine rejects them).
//   - For each prepared leveled spell, enumerate one candidate per
//     available slot at level ≥ native (D8-b, aggressive upcasting).
//     spellExpectedDamage handles the +1-die-per-slot-above-native
//     scaling. Cantrips contribute one slot-0 candidate.
//   - Among feasible candidates, prefer higher slot level (preserves
//     high-slot supremacy and burns the big slots first); tie-break on
//     expected damage from the dice string.
//
// simHealAllyThresholdPct is how hurt a friend has to be before a healer spends a
// slot on them rather than on damage. Deliberately lower than the self-heal
// threshold: a heal is a wasted turn if the target was going to live anyway, and
// the party's damage output is what ends the fight.
const simHealAllyThresholdPct = 45

// simPickHeal returns the heal spell a competent healer would cast this turn, and
// the seat to put it on — which may be the healer's own. An empty spell id means
// nobody needs healing, or this character cannot heal.
//
// Returning target "" means "cast it on yourself": the caller drops the @mention,
// and the engine's ordinary self-heal path takes it.
//
// It considers the caster's own seat, and it runs for a solo fight, and BOTH of
// those are recent. Until now the rule skipped `i == seat` and bailed on
// `!sess.IsParty()`, which together meant something nobody had said out loud: **no
// autopiloted caster in this game had ever cast a heal on themselves.** A solo
// cleric carried cure_wounds for a whole 30-room run and used it exactly never,
// while dying with a full pool of level-1 slots. That is a strong candidate for
// why the class corpus has cleric at 46–56% against fighter's 82%
// ([[project_d8prereq_baseline]], §6 of the combat-engine plan) — the picker was
// not playing the class.
//
// The healer heals whoever is worst off. Usually that is the friend bleeding out
// next to them. Sometimes it is them.
func simPickHeal(c *DnDCharacter, uid id.UserID, sess *CombatSession, seat int) (spellID, target string) {
	if sess == nil || c == nil || !isSpellcaster(c) {
		return "", ""
	}

	// Who is worst off? Only living seats — the engine will not raise the downed,
	// and a heal aimed at a corpse is a lost turn.
	worst, worstPct := -1, 101
	for i := range sess.RosterSize() {
		hp, hpMax := sess.seatHP(i), sess.seatHPMax(i)
		if hp <= 0 || hpMax <= 0 {
			continue
		}
		if pct := hp * 100 / hpMax; pct < worstPct {
			worst, worstPct = i, pct
		}
	}
	if worst < 0 || worstPct >= simHealAllyThresholdPct {
		return "", ""
	}
	// Healing yourself is not an ally-target at all — it is the plain self-heal the
	// engine has always had. Hand the caller no target and let it say so.
	if worst == seat {
		return simPickHealSpell(c, uid, sess, seat), ""
	}

	best := simPickHealSpell(c, uid, sess, seat)
	if best == "" {
		return "", ""
	}
	// Target by the seat's own Matrix localpart: splitCastTarget resolves against
	// the seats in this fight, so this round-trips without a room lookup.
	return best, id.UserID(sess.seatUserID(worst)).Localpart()
}

// simPickHealSpell is the cheapest heal this caster can actually cast right now,
// or "". Burning a 5th-level slot to top somebody up is not what a competent
// player does, so it takes the lowest castable slot.
func simPickHealSpell(c *DnDCharacter, uid id.UserID, sess *CombatSession, seat int) string {
	known, err := seatKnownSpells(sess, seat, uid)
	if err != nil {
		return ""
	}
	slots, _ := seatSpellSlots(sess, seat, uid)
	best, bestLevel := "", 99
	for _, k := range known {
		if !k.Prepared {
			continue
		}
		sp, ok := lookupSpell(k.SpellID)
		if !ok || sp.Effect != EffectSpellHeal || sp.CastTime == CastReaction {
			continue
		}
		onList := false
		for _, cl := range sp.Classes {
			if cl == c.Class {
				onList = true
				break
			}
		}
		if !onList || sp.Level == 0 {
			continue // heals are slot spells; a level-0 "heal" is not a thing we cast
		}
		if pair, ok := slots[sp.Level]; !ok || pair[0]-pair[1] <= 0 {
			continue
		}
		if sp.Level < bestLevel {
			best, bestLevel = sp.ID, sp.Level
		}
	}
	return best
}

func simPickSpell(c *DnDCharacter, sess *CombatSession, seat int, uid id.UserID, auraActive bool) string {
	known, err := seatKnownSpells(sess, seat, uid)
	if err != nil || len(known) == 0 {
		return ""
	}
	slots, _ := seatSpellSlots(sess, seat, uid)
	type cand struct {
		id          string
		slot        int
		nativeLevel int
		expDmg      float64
	}
	var cands []cand
	for _, k := range known {
		if !k.Prepared {
			continue
		}
		sp, ok := lookupSpell(k.SpellID)
		if !ok {
			continue
		}
		switch sp.Effect {
		case EffectDamageAttack, EffectDamageSave, EffectDamageAuto:
		default:
			continue
		}
		if sp.CastTime == CastReaction {
			continue
		}
		// An aura is already ticking — don't re-arm it; prefer attacks or a
		// non-concentration spell this turn.
		if auraActive && sp.Concentration {
			continue
		}
		onList := false
		for _, cl := range sp.Classes {
			if cl == c.Class {
				onList = true
				break
			}
		}
		if !onList {
			continue
		}
		if sp.Level == 0 {
			cands = append(cands, cand{id: sp.ID, slot: 0, nativeLevel: 0, expDmg: spellExpectedDamage(sp, 0, c.Level)})
			continue
		}
		// simMaxSlot mirrors parseCombatCast's slot-level cap; anything
		// above it would be rejected by the cast handler anyway.
		const simMaxSlot = 5
		for sl := sp.Level; sl <= simMaxSlot; sl++ {
			pair, ok := slots[sl]
			if !ok || pair[0]-pair[1] <= 0 {
				continue
			}
			cands = append(cands, cand{
				id:          sp.ID,
				slot:        sl,
				nativeLevel: sp.Level,
				expDmg:      spellExpectedDamage(sp, sl, c.Level),
			})
		}
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].slot != cands[j].slot {
			return cands[i].slot > cands[j].slot
		}
		return cands[i].expDmg > cands[j].expDmg
	})
	best := cands[0]
	if best.slot > best.nativeLevel && best.nativeLevel > 0 {
		return fmt.Sprintf("%s --upcast %d", best.id, best.slot)
	}
	return best.id
}

// applyAutoCamp drives the production camp scheduler under the sim's
// synthetic clock: call maybeAutoCamp with *simNow, then if a camp
// pitched, advance *simNow past minAutoCampDwell and break the camp so
// the next walk can proceed. Returns whether the pitch was a Night
// camp (i.e. a day rollover fired).
func (s *SimRunner) applyAutoCamp(exp *Expedition, simNow *time.Time) (bool, error) {
	_, d, ok := s.P.maybeAutoCamp(exp, *simNow)
	if !ok {
		return false, nil
	}
	return s.dwellAndBreakAutoCamp(exp, simNow, d.Night)
}

// applyAutoCampBossSafety mirrors applyAutoCamp for the stopBossSafety
// gate — the camp is force-pitched even when the regular HP threshold
// hasn't tripped (decideAutopilotCamp also blocks pitches at boss
// rooms, which is exactly where this one belongs).
func (s *SimRunner) applyAutoCampBossSafety(exp *Expedition, simNow *time.Time) (bool, error) {
	_, d, ok := s.P.pitchBossSafetyCamp(exp, *simNow)
	if !ok {
		return false, nil
	}
	return s.dwellAndBreakAutoCamp(exp, simNow, d.Night)
}

// dwellAndBreakAutoCamp advances *simNow past minAutoCampDwell and
// breaks the auto-pitched camp. Reloads exp from the DB first so the
// camp row reflects the just-applied pitch. Returns the night flag
// passed in (for the DayTicks counter).
func (s *SimRunner) dwellAndBreakAutoCamp(exp *Expedition, simNow *time.Time, night bool) (bool, error) {
	*simNow = simNow.Add(minAutoCampDwell)
	fresh, err := getExpedition(exp.ID)
	if err != nil {
		return night, err
	}
	if fresh != nil {
		_ = breakAutoCampIfDue(fresh, *simNow)
	}
	return night, nil
}

// TickDay drives one synthetic day rollover for exp: 21:00 recap of
// the current day, then 06:00 briefing of the next day. The briefing
// is what bumps current_day and applies supply burn / overnight camp /
// threat drift. exp is re-read from the DB after each step so callers
// see fresh state.
//
// The clock is anchored on exp.StartDate so repeat calls advance one
// real-day per call regardless of wall-clock time when the sim runs.
//
// Event-anchored expeditions (D2-b) own the rollover inside the
// autopilot's night-camp pitch — RunExpedition exercises that path via
// applyAutoCamp; TickDay is retained for tests and the legacy
// non-event-anchored fallback. The event-anchored branch here short-
// circuits to processNightCamp so callers that DO invoke TickDay on a
// post-cutoff expedition still see one-call-one-day semantics.
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
	if isEventAnchored(exp) {
		if err := s.tickEventAnchoredRollover(exp, briefAt); err != nil {
			return err
		}
	} else {
		if err := s.P.deliverBriefing(exp, briefAt); err != nil {
			return fmt.Errorf("deliverBriefing: %w", err)
		}
	}
	if fresh, _ := getExpedition(exp.ID); fresh != nil {
		*exp = *fresh
	}
	return nil
}

// tickEventAnchoredRollover mirrors pitchAutopilotCamp with Night=true
// for the sim: burn → optional Standard rest (if supplies cover it) →
// drift → stamp last_briefing_at. No DM, no camp row left active (rest
// is applied and immediately cleared the way pitchAutopilotCamp does
// via applyCampRest + the next walk-tick break). On the no-rest branch
// we still want burn/drift so supplies drain — matches a stalled
// autopilot which D2-b's safety net force-fires via processNightCamp.
func (s *SimRunner) tickEventAnchoredRollover(exp *Expedition, briefAt time.Time) error {
	burn, err := s.P.nightRolloverBurn(exp)
	if err != nil {
		return fmt.Errorf("nightRolloverBurn: %w", err)
	}
	// Pitch a Standard rest if affordable, else skip (autopilot would
	// also bail in low-SU; the burn already landed). Rough is the
	// fallback so the sim isn't stuck at zero healing on tight budgets.
	kind := CampTypeStandard
	if exp.Supplies.Current < campSupplyCost[kind] {
		kind = CampTypeRough
	}
	if exp.Supplies.Current >= campSupplyCost[kind] {
		exp.Supplies.Current -= campSupplyCost[kind]
		if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
			return fmt.Errorf("updateSupplies: %w", err)
		}
		applyCampRest(exp, kind)
	}
	drift := s.P.nightRolloverDrift(exp, briefAt)
	_ = burn
	_ = drift
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
