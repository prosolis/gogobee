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
	"sort"
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
	// Synthetic players don't type !arm between fights; flip on the
	// in-combat auto-arm so Fighter Second Wind, Cleric Healing Word,
	// etc. fire the way a competent prod player would set them up.
	// Without this the sim under-counts class survival.
	simAutoArmEnabled = true
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

// stockSimConsumables drops a small tier-appropriate bundle of potions
// + a couple offensive items into the synthetic player's inventory so
// SelectConsumables / setupAutoHealFromInventory have something to fire
// during autoResolveCombat. Counts are deliberately modest — a real
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
	// YieldCount is the total number of material rows the synthetic
	// player banked across the expedition (one row per +1 of any
	// resource). YieldsByName breaks that total down by resource name
	// for tier/per-resource calibration. Both are read from
	// adventure_inventory at end-of-run.
	YieldCount    int
	YieldsByName  map[string]int
	// Combats holds a per-combat trace for every fight the synthetic
	// player entered during the expedition (boss + elites + patrols).
	// Used by post-hoc analysis to dig into class-survival walls
	// without re-running the matrix. Populated from combat_sessions
	// rows + their TurnLog at end-of-run.
	Combats []SimCombatSummary
	Log     []SimLogEntry
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
	Events []CombatEvent `json:",omitempty"`
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
	res.YieldCount, res.YieldsByName = simMaterialYields(uid)
	res.Combats = simCombatSummaries(uid)
	return res, nil
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
		out[len(out)-1].Events = lastEvents
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
		kind, arg := s.simPickCombatAction(ctx.Sender, cur)
		var dispatchErr error
		switch kind {
		case "consume":
			dispatchErr = s.P.handleConsumeCmd(ctx, arg)
		case "cast":
			dispatchErr = s.P.handleCombatCastCmd(ctx, arg)
		default:
			dispatchErr = s.P.handleAttackCmd(ctx)
		}
		if dispatchErr != nil {
			return false, fmt.Errorf("%s iter %d: %w", kind, i, dispatchErr)
		}
	}
	return false, fmt.Errorf("combat exceeded %d rounds", autoCombatRoundCap)
}

// simHealHPThresholdPct is the player-HP percentage below which the sim
// reaches for a heal consumable before its !attack/!cast. 40% mirrors
// the "competent player" prod assumption — heal early enough to absorb
// one more big hit, not so early that a 1-HP scratch burns a potion.
const simHealHPThresholdPct = 40

// simPickCombatAction is the sim's per-turn decision tree, mirroring
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
func (s *SimRunner) simPickCombatAction(uid id.UserID, sess *CombatSession) (kind, arg string) {
	c, _ := LoadDnDCharacter(uid)
	if c == nil || sess == nil {
		return "attack", ""
	}
	lowHP := sess.PlayerHPMax > 0 && sess.PlayerHP*100 < sess.PlayerHPMax*simHealHPThresholdPct
	if lowHP {
		inv := s.P.loadConsumableInventory(uid)
		for _, it := range inv {
			if it.Def.Effect == EffectHeal {
				return "consume", it.Def.Name
			}
		}
	}
	if isSpellcaster(c) && !simMartialFirstClass(c.Class) {
		if id := simPickSpell(c, uid); id != "" {
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

// simPickSpell returns the spell ID a competent player would cast this
// turn, or "" when no usable spell is available (forcing a !attack).
// Selection rules:
//   - Only damage-effect spells (damage_attack / damage_save / damage_auto).
//     Control/buff/heal are out (J2c sweep showed control scoring at
//     either 22 or 5 nets ≤±2pp at L12 and regresses warlock at 22 —
//     no headroom worth the complexity). Healing is handled by the
//     consumable-first branch in simPickCombatAction.
//   - Reaction-cast spells are excluded (engine rejects them).
//   - Non-cantrips require an available slot at their native level (no
//     upcasting — preserves high slots for high-level spells).
//   - Among feasible candidates, prefer higher slot level; tie-break on
//     expected damage from the dice string.
//
// Returns the spell ID for handleCombatCastCmd.
func simPickSpell(c *DnDCharacter, uid id.UserID) string {
	known, err := listKnownSpells(uid)
	if err != nil || len(known) == 0 {
		return ""
	}
	slots, _ := getSpellSlots(uid)
	type cand struct {
		id     string
		level  int
		expDmg float64
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
		if sp.Level > 0 {
			pair, ok := slots[sp.Level]
			if !ok || pair[0]-pair[1] <= 0 {
				continue
			}
		}
		cands = append(cands, cand{id: sp.ID, level: sp.Level, expDmg: spellExpectedDamage(sp, sp.Level, c.Level)})
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].level != cands[j].level {
			return cands[i].level > cands[j].level
		}
		return cands[i].expDmg > cands[j].expDmg
	})
	return cands[0].id
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
