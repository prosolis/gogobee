package plugin

import (
	"math/rand/v2"
	"sort"
)

// ── N-body auto-resolve ──────────────────────────────────────────────────────
//
// The auto-resolve engine seats a roster, exactly as the turn-based engine has
// since P3. `SimulateCombat` is the one-seat case and nothing more: for a solo
// roster every short-circuit below collapses to the pre-roster code path and
// the engine draws from the RNG in precisely the pre-roster order. That is what
// keeps `TestCombatCharacterization` byte-identical and the d8prereq_corpus
// baselines comparable. If the golden moves, solo balance moved — stop.
//
// The invariants the solo path rests on, all of them mirrored from P3:
//
//   - enemyTargetSeat draws nothing for a one-seat roster (there is only one
//     target), so the enemy's choice costs no randomness.
//   - the initiative loop draws one player roll then one enemy roll, which is
//     the pre-roster order; ties go to the player, as `playerInit >= enemyInit`
//     always did.
//   - the per-seat loops run exactly once.
//
// Per-actor state (poison, charges, rage, wards) follows the `combatState`
// cursor, so the resolution primitives need no changes: they already read
// `st.c` — the turn engine has called them that way since P3.

// PartyCombatResult is one fight, seen from every seat at once. The fight-scoped
// fields (the enemy, the round count, the event log) are shared; `Seats` holds
// the per-character view, and `Seats[i]` is a complete `CombatResult` so a solo
// caller can take `Seats[0]` and be handed exactly what `SimulateCombat` always
// returned.
type PartyCombatResult struct {
	PlayerWon   bool
	TimedOut    bool
	TotalRounds int
	Events      []CombatEvent

	EnemyStartHP int
	EnemyEntryHP int
	EnemyEndHP   int

	Seats []CombatResult
}

// AnySurvivor reports whether at least one seat is still standing. A party can
// win a fight it did not all walk away from.
func (r PartyCombatResult) AnySurvivor() bool {
	for i := range r.Seats {
		if r.Seats[i].PlayerEndHP > 0 {
			return true
		}
	}
	return false
}

// simulateParty auto-resolves one enemy against a roster of N player characters.
// Production auto-resolve passes a nil rng (package global); the sim harness and
// the characterization test seed it.
func simulateParty(players []Combatant, enemy Combatant, phases []CombatPhase) PartyCombatResult {
	return simulatePartyWithRNG(players, enemy, phases, nil)
}

func simulatePartyWithRNG(players []Combatant, enemy Combatant, phases []CombatPhase, rng *rand.Rand) PartyCombatResult {
	// Party-only: bump the enemy's max HP so the fight lasts long enough for the
	// scaled action economy to actually threaten each member. Solo is unchanged
	// (scale 1.0), so SimulateCombat and the golden do not move.
	if len(players) > 1 {
		enemy.Stats.MaxHP = scaledEnemyMaxHP(enemy.Stats.MaxHP, len(players))
	}
	enemyStart := enemy.Stats.MaxHP
	if enemy.Stats.StartHP > 0 && enemy.Stats.StartHP < enemy.Stats.MaxHP {
		enemyStart = enemy.Stats.StartHP
	}

	actors := make([]*actor, len(players))
	seats := make([]CombatResult, len(players))
	for i := range players {
		actors[i] = newActor(&players[i])
		seats[i] = CombatResult{
			PlayerStartHP: players[i].Stats.MaxHP,
			PlayerEntryHP: actors[i].playerHP,
			EnemyStartHP:  enemy.Stats.MaxHP,
			EnemyEntryHP:  enemyStart,
		}
	}

	st := &combatState{
		actor:   actors[0],
		actors:  actors,
		enemyHP: enemyStart,
		rng:     rng,
	}
	// Holding the enemy holds it for everyone, so the control effect is
	// fight-scoped: any caster who queued one arms it.
	for i := range players {
		if players[i].Mods.SpellEnemySkipFirst {
			st.enemySkipFirst = true
		}
	}

	// Pre-combat one-shots, grouped by kind rather than by seat. A one-seat
	// roster walks these in the pre-roster order.
	for i := range actors {
		st.seat(i)
		if st.c.Mods.SniperKillProc > 0 && st.randFloat() < st.c.Mods.SniperKillProc {
			st.enemyHP = 0
			st.events = append(st.events, CombatEvent{
				Round: 0, Phase: "pre_combat", Actor: "npc", Action: "sniper_kill",
				Damage: enemy.Stats.MaxHP, PlayerHP: st.playerHP, EnemyHP: 0,
				Seat: i, Desc: "Arina",
			})
			seats[i].SniperKilled = true
			return finalizeParty(seats, st, players, enemy)
		}
	}

	for i := range actors {
		st.seat(i)
		if st.c.Mods.FlatDmgStart > 0 {
			dmg := st.c.Mods.FlatDmgStart
			st.enemyHP = max(0, st.enemyHP-dmg)
			st.events = append(st.events, CombatEvent{
				Round: 0, Phase: "pre_combat", Actor: "consumable", Action: "flat_damage",
				Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Seat: i,
			})
			if st.enemyHP <= 0 {
				return finalizeParty(seats, st, players, enemy)
			}
		}
	}

	// Queued spells. Resolved by applyPendingCast() before this runs — the
	// modifiers carry the resolved damage and narrative hook.
	for i := range actors {
		st.seat(i)
		if st.c.Mods.SpellPreDamageDesc == "" {
			continue
		}
		dmg := st.c.Mods.SpellPreDamage
		resisted := dmg > 0 && enemyResistsSpells(&enemy, st)
		if resisted {
			dmg = max(1, dmg/2)
		}
		if dmg > 0 {
			st.enemyHP = max(0, st.enemyHP-dmg)
		}
		st.events = append(st.events, CombatEvent{
			Round: 0, Phase: "pre_combat", Actor: "player", Action: "spell_cast",
			Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Seat: i,
			Desc: st.c.Mods.SpellPreDamageDesc,
		})
		if resisted {
			st.events = append(st.events, CombatEvent{
				Round: 0, Phase: "pre_combat", Actor: "enemy", Action: "spell_fizzle",
				Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Seat: i,
			})
		}
		if st.enemyHP <= 0 {
			return finalizeParty(seats, st, players, enemy)
		}
	}

	// Pre-combat: Misty gourmet is now a per-round heal, no pre-combat damage

	for _, phase := range phases {
		roundsThisPhase := phase.Rounds
		// Add slight variance: ±1 round for non-Decisive phases
		if phase.Name != "Decisive" && roundsThisPhase > 1 {
			roundsThisPhase += st.roll(2) // 0 or +1
		}
		for r := 0; r < roundsThisPhase; r++ {
			st.round++
			if simulatePartyRound(st, &enemy, &phase, seats) {
				return finalizeParty(seats, st, players, enemy)
			}
		}
	}

	// Exhausted the phase clock without a kill. Tiebreak on HP percentage to
	// decide the outcome — but DO NOT zero out HP on the loser. Timeout =
	// retreat, not a lethal blow, so no character-death side effects fire.
	//
	// The party reads its fraction off the pooled roster, which for one seat is
	// that seat's own fraction, i.e. the pre-roster comparison. Slight bias to
	// the players on exact ties (frac >=).
	var partyHP, partyMax int
	for i := range st.actors {
		partyHP += st.actors[i].playerHP
		partyMax += players[i].Stats.MaxHP
	}
	playerFrac := float64(partyHP) / float64(max(1, partyMax))
	enemyFrac := float64(st.enemyHP) / float64(max(1, enemy.Stats.MaxHP))
	playerWonTiebreak := playerFrac >= enemyFrac
	st.seat(0)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: "exhaust", Actor: "system", Action: "timeout",
		PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})
	for i := range seats {
		seats[i].TimedOut = true
	}
	out := finalizeParty(seats, st, players, enemy)
	out.TimedOut = true
	out.PlayerWon = playerWonTiebreak
	for i := range out.Seats {
		out.Seats[i].PlayerWon = playerWonTiebreak
	}
	return out
}

// combatOver reads the fight's terminal condition off HP rather than off a
// primitive's bool. resolvePlayerAttack documents why: a retaliate aura can
// drop the swinger with the enemy still standing, so `true` means "something
// decisive happened", not "the players won". For a one-seat roster this is the
// same answer the old `return true` gave.
func combatOver(st *combatState) bool {
	return st.enemyHP <= 0 || !st.anyAlive()
}

// enemyTargetSeat picks who the enemy swings at. A one-seat roster draws no
// randomness — there is only one target — which is what keeps the solo RNG
// stream identical. Shared with the turn engine.
func enemyTargetSeat(st *combatState) (int, bool) {
	if len(st.actors) == 1 {
		return 0, st.actors[0].playerHP > 0
	}
	standing := make([]int, 0, len(st.actors))
	for i, a := range st.actors {
		if a.playerHP > 0 {
			standing = append(standing, i)
		}
	}
	if len(standing) == 0 {
		return 0, false
	}
	return standing[st.roll(len(standing))], true
}

// eventsForSeat is the sub-log a single character is responsible for. Events the
// engine never stamped — the enemy regenerating, the phase clock running out —
// carry seat 0, so they read as the leader's. Only ever called for a party: a
// solo seat is handed the whole log by identity.
func eventsForSeat(events []CombatEvent, seat int) []CombatEvent {
	out := make([]CombatEvent, 0, len(events))
	for _, e := range events {
		if e.Seat == seat {
			out = append(out, e)
		}
	}
	return out
}

// stampEventSeats attributes every event appended since `mark` to a seat, so
// party narration can say who did what. Seat 0 stamps a zero, which is
// `omitempty` — a solo fight's event log is byte-identical.
func stampEventSeats(st *combatState, mark, seat int) {
	for i := mark; i < len(st.events); i++ {
		st.events[i].Seat = seat
	}
}

// roundInitiative rolls the round's turn order: every seat, then the enemy, in
// seating order — the pre-roster draw order when there is one seat. Ties favour
// the players, then the lower seat.
func roundInitiative(st *combatState, enemy *Combatant, phase *CombatPhase) []int {
	type entry struct {
		seat int
		init float64
	}
	entries := make([]entry, 0, len(st.actors)+1)
	for i, a := range st.actors {
		speed := float64(a.c.Stats.Speed) * phase.SpeedWeight
		entries = append(entries, entry{i, speed + st.randFloat()*10 + a.c.Mods.InitiativeBias})
	}
	enemySpeed := float64(enemy.Stats.Speed) * phase.SpeedWeight
	entries = append(entries, entry{enemySeat, enemySpeed + st.randFloat()*10})

	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.init != b.init {
			return a.init > b.init
		}
		if (a.seat == enemySeat) != (b.seat == enemySeat) {
			return b.seat == enemySeat
		}
		return a.seat < b.seat
	})
	order := make([]int, len(entries))
	for i, e := range entries {
		order[i] = e.seat
	}
	return order
}

// enemyActionsThisRound is how many attack-actions the enemy takes this round
// against the seated roster. Solo returns 1 without drawing from the RNG, so both
// combat engines collapse to their pre-party behaviour and the characterization
// golden and d8prereq corpus do not move.
//
// A party's action budget is a *fractional expectation* (partyActionExpectation),
// realised as floor(exp) actions plus one more with probability frac(exp). The
// fraction matters because the action lever is coarse: against a party of two, an
// integer 2 actions is a 100% faceroll and 3 is brutal (~45%), with nothing
// between — a per-round coin-flip for the extra action is the only way to land
// the band in the gap. The single draw is taken only for a party, so the solo RNG
// stream is untouched.
// §2(b): the budget counts the seats still STANDING, re-derived every round —
// not the seats that walked in.
//
// It used to read len(st.actors), which includes the dead. So a party that lost a
// member kept paying for them: the survivor faced a boss still swinging at
// two-player cadence, alone. That is a death spiral with the arrow pointing the
// wrong way — the moment a party is losing, the engine hits it harder — and it is
// the single nastiest thing the companion sweep turned up, because it is not a
// companion bug at all. It has been live for every human party since N3.
//
// A corpse does not threaten anybody, and the enemy has no reason to keep spending
// actions on one.
func enemyActionsThisRound(st *combatState) int {
	n := livingActors(st)
	if n < 2 {
		return 1
	}
	exp := partyActionExpectation(n)
	base := int(exp)
	if frac := exp - float64(base); frac > 0 && st.randFloat() < frac {
		base++
	}
	return base
}

// livingActors counts the seats still standing.
func livingActors(st *combatState) int {
	n := 0
	for _, a := range st.actors {
		if a.playerHP > 0 {
			n++
		}
	}
	return n
}

// partyActionExpectation is the expected number of enemy attack-actions per round
// against a party of n. A single enemy swing (the pre-P8 behaviour) let each
// member absorb ~1/N² of the solo incoming and cleared 100% of T5; the sim band
// landed the martial-leader target only near ~2N actions, and HP alone can't touch
// a martial (fighter stays at 100% even at enemy HP ×1.6). So the enemy's action
// economy is the load-bearing lever, and it is fractional so a two-person party
// lands between soloing and a trio instead of snapping to one of two integers
// (against a duo, an integer 2 actions is a ~91% faceroll and 3 is a ~45% meat
// grinder — 2.4 sits fighter+cleric at ~75%, between solo 70% and trio 90%).
//
// N≥3 follows 2N−1, the point that gave fighter ~90% / cleric-led ~72% at
// HP ×1.15. The whole curve is monotonic by party size and never drops a
// composition below its solo clear rate — bringing a friend is never a penalty.
func partyActionExpectation(n int) float64 {
	switch {
	case n < 2:
		return 1
	case n == 2:
		return 2.4
	default:
		return float64(2*n - 1)
	}
}

// partyEnemyHPScale is the party-only multiplier on the enemy's max HP. Solo
// (roster < 2) returns 1.0, so the solo path — and the characterization golden
// and the d8prereq corpus — is byte-for-byte untouched. With the action economy
// fixed, each member now takes ~1 swing a round, so a longer fight is more
// cumulative exposure per member: HP became a live difficulty lever exactly the
// way P6e predicted it would once the enemy stopped swinging only once.
//
// 1.15 is where the P8 sim sweep landed the band: with 2N−1 actions, a party of
// three clears the T5 martial-leader band at ~89% (fighter) / ~67% (cleric-led),
// clearly safer than soloing (70% / 27%) but with real TPKs and member deaths —
// not the 100% faceroll a single enemy swing produced.
func partyEnemyHPScale(rosterSize int) float64 {
	if rosterSize < 2 {
		return 1.0
	}
	return 1.15
}

// scaledEnemyMaxHP applies the party HP scalar to a base max-HP with one rounding
// rule, so every call site (the auto-resolve engine, the party session's initial
// persist, and the per-turn rebuild) agrees on the same number.
func scaledEnemyMaxHP(baseMaxHP, rosterSize int) int {
	return int(float64(baseMaxHP) * partyEnemyHPScale(rosterSize))
}

// enemyActionPlan is the shared action budget both combat engines resolve an
// enemy turn against: how many attack-actions the enemy takes, and whether the
// first one reuses the round's already-picked target (and its already-rolled
// procs). A cleave/lifesteal ability already spent the first action, so one fewer
// follows and none of them reuse the initial target. Both engines call this so
// the load-bearing count stays in lockstep even though their per-action bodies
// differ.
func enemyActionPlan(st *combatState, abilityDealtDamage bool) (count int, reuseFirst bool) {
	count = enemyActionsThisRound(st)
	reuseFirst = !abilityDealtDamage
	if abilityDealtDamage {
		count--
	}
	return count, reuseFirst
}

// enemyRoundSwings resolves the enemy's attacks for one round of the auto-resolve
// engine. A solo roster takes exactly one swing at its single seat, reusing the
// round's already-rolled target and pet procs, so its RNG stream and event log are
// byte-for-byte the pre-P8 engine's. A party faces enemyActionsThisRound() swings,
// each re-targeted at a random standing seat with that seat's own pet procs, so the
// damage is spread across the roster instead of pinning the round's first target.
//
// abilityDealtDamage means a cleave/lifesteal already spent the enemy's first
// action this round, so one fewer swing follows — for solo that collapses to the
// old "the ability stands in for the attack" skip. Returns true if the fight is
// decided.
func enemyRoundSwings(st *combatState, enemy *Combatant, phase *CombatPhase, seats []CombatResult,
	target int, abilityDealtDamage, petWhiff, petDeflect, sporeMiss bool) bool {

	swings, reuseFirst := enemyActionPlan(st, abilityDealtDamage)
	for k := 0; k < swings; k++ {
		tgt := target
		sw, sd, sp := petWhiff, petDeflect, sporeMiss
		if !(reuseFirst && k == 0) {
			// A fresh target for every swing past the first: re-pick among the
			// standing seats and roll that seat's own pet procs. This branch never
			// runs for a solo roster, so it adds no draws to the solo stream.
			var alive bool
			if tgt, alive = enemyTargetSeat(st); !alive {
				return combatOver(st)
			}
			st.seat(tgt)
			sw = st.c.Mods.PetWhiffProc > 0 && st.randFloat() < st.c.Mods.PetWhiffProc
			sd = st.c.Mods.PetDeflectProc > 0 && st.randFloat() < st.c.Mods.PetDeflectProc
			if sd {
				seats[tgt].PetDeflected = true
			}
			sp = st.sporeRounds > 0 && st.randFloat() < 0.15
		}
		st.seat(tgt)
		mark := len(st.events)
		over := resolveEnemyAttack(st, st.c, enemy, phase, &seats[tgt], sw, sd, sp)
		stampEventSeats(st, mark, tgt)
		if over && combatOver(st) {
			return true
		}
	}
	return false
}

// simulatePartyRound runs one round for the whole roster. Returns true if the
// fight is over.
func simulatePartyRound(st *combatState, enemy *Combatant, phase *CombatPhase, seats []CombatResult) bool {
	phaseName := phase.Name

	// Whoever the enemy is looking at this round. Chosen before the ability
	// fires, because the ability lands on its target. Costs no RNG when solo.
	target, alive := enemyTargetSeat(st)
	if !alive {
		return true
	}
	st.seat(target)

	// Phase 9 spell: control effect (Hold Person, Sleep, Command) skips the
	// enemy's attack for one round. Fight-scoped — holding it holds it for all.
	enemyHeldThisRound := false
	if st.enemySkipFirst {
		st.enemySkipFirst = false
		if enemyImmuneToControl(enemy, st) {
			// fear_immune: the control spell can't take hold — the enemy acts
			// as normal this round.
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "fear_resist",
				PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		} else {
			enemyHeldThisRound = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "spell_held",
				PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}
	}

	// Monster ability: check at round start. It resolves against the enemy's
	// target, which the cursor already points at.
	abilityDealtDamage := enemyHeldThisRound
	if enemy.Ability != nil {
		if abilityFires(enemy.Ability, phaseName, st) {
			mark := len(st.events)
			over := applyAbility(st, st.c, enemy, phase, &seats[target])
			stampEventSeats(st, mark, target)
			if over && combatOver(st) {
				return true
			}
			// Cleave and lifesteal deal damage — skip normal enemy attack this round
			switch enemy.Ability.Effect {
			case "cleave", "lifesteal":
				abilityDealtDamage = true
			}
		}
	}

	// Poison tick from previous round. Stacked per character, so every seat
	// carrying it bleeds.
	for i := range st.actors {
		st.seat(i)
		if st.poisonTicks <= 0 {
			continue
		}
		st.playerHP = max(0, st.playerHP-st.poisonDmg)
		st.poisonTicks--
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "poison_tick",
			Damage: st.poisonDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Seat: i,
		})
		if st.playerHP <= 0 && !trySave(st, st.c, phaseName) && !st.anyAlive() {
			return true
		}
	}

	// The poison may have dropped the seat the enemy had picked. Re-target
	// before the swing procs, which read the target's own modifiers.
	if st.actors[target].playerHP <= 0 {
		if target, alive = enemyTargetSeat(st); !alive {
			return true
		}
	}
	st.seat(target)

	// Pet whiff: if proc'd, enemy's attack this round is a guaranteed miss
	petWhiff := st.c.Mods.PetWhiffProc > 0 && st.randFloat() < st.c.Mods.PetWhiffProc
	// Pet deflect: halves incoming damage to the target this round
	petDeflect := st.c.Mods.PetDeflectProc > 0 && st.randFloat() < st.c.Mods.PetDeflectProc
	if petDeflect {
		seats[target].PetDeflected = true
	}
	// Spore cloud: enemy has 15% miss chance (decremented when enemy actually attacks)
	sporeMiss := st.sporeRounds > 0 && st.randFloat() < 0.15

	// Determine initiative. DM mood (Effusive/Hostile) biases a player's roll
	// via Mods.InitiativeBias — +X means they go first more often.
	for _, s := range roundInitiative(st, enemy, phase) {
		if s == enemySeat {
			if enemyRoundSwings(st, enemy, phase, seats, target, abilityDealtDamage, petWhiff, petDeflect, sporeMiss) {
				return true
			}
			continue
		}
		if st.actors[s].playerHP <= 0 {
			// A seat that went down earlier this round forfeits its swing.
			continue
		}
		st.seat(s)
		mark := len(st.events)
		over := resolvePlayerSwings(st, st.c, enemy, phase, &seats[s])
		stampEventSeats(st, mark, s)
		if over && combatOver(st) {
			return true
		}
	}

	// End-of-round, per surviving character.
	for i := range st.actors {
		st.seat(i)
		if st.playerHP <= 0 {
			continue
		}
		mark := len(st.events)
		if over := endOfRoundForSeat(st, phase, &seats[i]); over {
			stampEventSeats(st, mark, i)
			return true
		}
		stampEventSeats(st, mark, i)
	}

	// Regenerate (monster ability): the enemy knits its wounds at the close of
	// every round once the ability has armed st.enemyRegen. Fight-scoped.
	if st.enemyRegen > 0 && st.enemyHP > 0 && st.enemyHP < enemy.Stats.MaxHP {
		st.enemyHP = min(enemy.Stats.MaxHP, st.enemyHP+st.enemyRegen)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "regen_tick",
			Damage: st.enemyRegen, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	// End-of-round Orc Rage backstop. The primary trigger sits at the top of
	// resolvePlayerAttack so rage fires same-round when the character swings
	// after taking the threshold-crossing hit. But if the enemy goes first this
	// round AND next, they can be two-shot without ever getting back to that
	// check. Re-checking here ensures the rage event always fires while HP > 0
	// and below 50%, even if the buff goes unused.
	for i := range st.actors {
		st.seat(i)
		mark := len(st.events)
		maybeTriggerOrcRage(st, st.c, phaseName)
		stampEventSeats(st, mark, i)
	}

	return false
}

// ── Misty's pair, shared by both engines ─────────────────────────────────────
//
// These two are the only round-end effects the turn engine did not have its own
// copy of. Everything else in endOfRoundForSeat either exists there already (the
// pet, the spiritual weapon — fired after a player action rather than at round
// end), is deliberately absent (the environmental hazard: turnCombatPhase is a
// single flat "Duel" phase with EnvironmentProc at 0), or is replaced by an
// explicit player command (the consumable auto-heal, which is `!consume`).
//
// So they are hoisted rather than re-implemented. A parallel sibling is what let
// the two win close-outs drift apart (deferred item D); this pair is now one
// list of effects with two callers, and cannot.
//
// Neither draws from the RNG unless its proc is armed, so a character with no
// Misty history rolls exactly the dice it rolled before — the sim corpus and
// combat_characterization.golden do not move.

// mistyCrowdRevenge is the debuff for declining Misty: her crowd takes a swing
// at the end of the round. Returns true when it decided the fight — that is,
// when it dropped this character, the death save failed, and nobody else is
// standing. A downed character in a still-live party returns false.
func mistyCrowdRevenge(st *combatState, player *Combatant, phaseName string) bool {
	if player.Mods.CrowdRevengeProc <= 0 || st.randFloat() >= player.Mods.CrowdRevengeProc {
		return false
	}
	dmg := player.Mods.CrowdRevengeDmg
	st.playerHP = max(0, st.playerHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "environment", Action: "crowd_revenge",
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		Desc: "Misty's crowd",
	})
	return st.playerHP <= 0 && !trySave(st, player, phaseName) && !st.anyAlive()
}

// mistyHeal is the buff's payout. It cannot decide the fight, so it returns
// nothing. result may be a scratch value the caller discards (the turn engine's
// is) — the durable record is the misty_heal event on the log, which is what
// seatCombatResult reads to award combat_misty_clutch.
func mistyHeal(st *combatState, player *Combatant, phaseName string, result *CombatResult) {
	if player.Mods.MistyHealProc <= 0 || st.randFloat() >= player.Mods.MistyHealProc {
		return
	}
	healAmt := player.Mods.MistyHealAmt
	st.playerHP = min(effPlayerMaxHP(player, st), st.playerHP+healAmt)
	result.MistyHealed = true
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "npc", Action: "misty_heal",
		Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		Desc: "Misty",
	})
}

// seatEndOfRound runs the round-end effects the turn engine owes one seat, in
// the order endOfRoundForSeat runs them: the crowd's swing, then — if the seat
// survived it — Misty's heal. Returns true when the fight is over.
//
// It exists because the turn engine's stepRoundEnd never ran either one. A
// player carrying Misty's buff lost it by fighting manually; worse, a player
// carrying her debuff *escaped* it by doing the same, which needed no discovery
// to exploit — just press !attack.
func seatEndOfRound(st *combatState, player *Combatant, phaseName string, result *CombatResult) bool {
	if over := mistyCrowdRevenge(st, player, phaseName); over {
		return true
	}
	if st.playerHP <= 0 {
		return false
	}
	mistyHeal(st, player, phaseName, result)
	return false
}

// endOfRoundForSeat runs the per-character close of a round against the seat the
// cursor already points at: environment, Misty's crowd, the pet, the spiritual
// weapon, Misty's heal, and the consumable auto-heal — in that order, which is
// the pre-roster order. Returns true if the fight is over.
func endOfRoundForSeat(st *combatState, phase *CombatPhase, result *CombatResult) bool {
	phaseName := phase.Name
	player := st.c

	// Environmental hazard
	if phase.EnvironmentProc > 0 && st.randFloat() < phase.EnvironmentProc {
		envDmg := 2 + st.roll(5)
		st.playerHP = max(0, st.playerHP-envDmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "environment", Action: "environmental",
			Damage: envDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 && !trySave(st, player, phaseName) && !st.anyAlive() {
			return true
		}
	}

	// Misty crowd revenge (debuff for declining Misty)
	if over := mistyCrowdRevenge(st, player, phaseName); over {
		return true
	}

	// A character the round just killed stops acting, but the fight goes on if
	// anyone else is standing.
	if st.playerHP <= 0 {
		return false
	}

	// Pet attack
	if player.Mods.PetAttackProc > 0 && st.randFloat() < player.Mods.PetAttackProc {
		petDmg := player.Mods.PetAttackDmg + st.roll(5)
		st.enemyHP = max(0, st.enemyHP-petDmg)
		result.PetAttacked = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "pet", Action: "pet_attack",
			Damage: petDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if enemyDown(st, phaseName) {
			return true
		}
	}

	// Spiritual Weapon strike
	if player.Mods.SpiritWeaponProc > 0 && st.randFloat() < player.Mods.SpiritWeaponProc {
		swDmg := player.Mods.SpiritWeaponDmg + st.roll(5)
		st.enemyHP = max(0, st.enemyHP-swDmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "spirit_weapon", Action: "spirit_weapon_strike",
			Damage: swDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if enemyDown(st, phaseName) {
			return true
		}
	}

	// Misty heal
	mistyHeal(st, player, phaseName, result)

	// Consumable heal: triggers when the character drops below 60% HP. Fires up
	// to HealItemCharges times per fight (1 = legacy one-shot; bosses can
	// stack from inventory).
	//
	// Threshold is 60% rather than 50% to give low-HP classes (cleric, mage)
	// more breathing room — at 50% a cleric was bleeding into the danger zone
	// before the heal fired.
	if st.healChargesLeft > 0 && player.Mods.HealItem > 0 &&
		st.playerHP > 0 && st.playerHP*5 < player.Stats.MaxHP*3 {
		st.healChargesLeft--
		healAmt := player.Mods.HealItem
		st.playerHP = min(effPlayerMaxHP(player, st), st.playerHP+healAmt)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "heal_item",
			Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	return false
}

// finalizeParty closes the fight, giving every seat its own view. The enemy's
// numbers and the event log are shared; HP, closeness and near-death are read
// per character. For one seat this is byte-for-byte the old `finalize`.
func finalizeParty(seats []CombatResult, st *combatState, players []Combatant, enemy Combatant) PartyCombatResult {
	won := st.enemyHP <= 0
	enemyMax := max(1, enemy.Stats.MaxHP)

	// A solo fight's seat holds the whole log, which is the same slice header the
	// pre-roster engine returned. A party's seats each hold their own events, so
	// the per-seat close-out (heal items burned, combat achievements) counts what
	// that character actually did rather than what the party did.
	solo := len(seats) == 1

	for i := range seats {
		a := st.actors[i]
		r := &seats[i]
		if solo {
			r.Events = st.events
		} else {
			r.Events = eventsForSeat(st.events, i)
		}
		r.PlayerEndHP = a.playerHP
		r.EnemyEndHP = st.enemyHP
		r.TotalRounds = st.round
		r.PlayerWon = won

		playerMax := max(1, players[i].Stats.MaxHP)
		if won && a.playerHP > 0 {
			r.NearDeath = float64(a.playerHP) < float64(playerMax)*0.15
			winnerRemaining := float64(a.playerHP) / float64(playerMax)
			r.Closeness = 1.0 - winnerRemaining
		} else if !won {
			enemyRemaining := float64(st.enemyHP) / float64(enemyMax)
			r.NearDeath = enemyRemaining < 0.15
			r.Closeness = 1.0 - enemyRemaining
		}
	}

	return PartyCombatResult{
		PlayerWon:    won,
		TotalRounds:  st.round,
		Events:       st.events,
		EnemyStartHP: enemy.Stats.MaxHP,
		EnemyEntryHP: seats[0].EnemyEntryHP,
		EnemyEndHP:   st.enemyHP,
		Seats:        seats,
	}
}
