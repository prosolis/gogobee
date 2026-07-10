package plugin

import (
	"math/rand/v2"
	"testing"
)

// Misty's two round-end procs on the turn-based surface.
//
// Both were built onto every turn-based combatant by DerivePlayerStats and then
// never read: stepRoundEnd had no counterpart to endOfRoundForSeat. The buff was
// simply lost. The debuff was an exploit — a player who declined Misty escaped
// her crowd entirely by fighting with !attack instead of letting the room
// auto-resolve, which needed no discovery at all.
//
// seatEndOfRound is the shared hook. These pin that it fires, that it cannot be
// dodged, and that a character with no Misty history is not touched at all —
// the property that keeps the sim corpus and the golden file still.

// mistyState seats one combatant at hp, cursor armed, with a seeded RNG.
func mistyState(t *testing.T, player *Combatant, hp int) *combatState {
	t.Helper()
	st := testCombatState(hp, 100, 1, rand.New(rand.NewPCG(7, 7)))
	st.actor.c = player
	st.actor.hpMax = player.Stats.MaxHP
	return st
}

// The two procs at certainty, so no test depends on a roll.
func mistyCursed(maxHP, dmg int) *Combatant {
	return &Combatant{
		Name:  "Cursed",
		Stats: CombatStats{MaxHP: maxHP, AC: 10},
		Mods:  CombatModifiers{CrowdRevengeProc: 1.0, CrowdRevengeDmg: dmg},
	}
}

func mistyBuffed(maxHP, heal int) *Combatant {
	return &Combatant{
		Name:  "Buffed",
		Stats: CombatStats{MaxHP: maxHP, AC: 10},
		Mods:  CombatModifiers{MistyHealProc: 1.0, MistyHealAmt: heal},
	}
}

func hasAction(events []CombatEvent, action string) bool {
	for _, e := range events {
		if e.Action == action {
			return true
		}
	}
	return false
}

// ── the exploit ──────────────────────────────────────────────────────────────

// The debuff must land at round end in a manual fight, exactly as it does when
// the room auto-resolves. Before seatEndOfRound, !attack was a clean escape.
func TestSeatEndOfRound_CrowdRevengeCannotBeDodgedByFightingManually(t *testing.T) {
	st := mistyState(t, mistyCursed(40, 6), 40)
	over := seatEndOfRound(st, st.c, CombatPhaseRoundEnd, &CombatResult{})

	if over {
		t.Fatal("a 6-damage swing against 40 HP should not end the fight")
	}
	if st.playerHP != 34 {
		t.Errorf("playerHP = %d, want 34 — Misty's crowd took its swing", st.playerHP)
	}
	if !hasAction(st.events, "crowd_revenge") {
		t.Error("no crowd_revenge event on the log")
	}
}

// The debuff can be lethal, and a lethal one with nobody else standing ends the
// fight rather than leaving a corpse to take the next round's turn.
func TestSeatEndOfRound_CrowdRevengeCanEndASoloFight(t *testing.T) {
	st := mistyState(t, mistyCursed(40, 30), 3)
	over := seatEndOfRound(st, st.c, CombatPhaseRoundEnd, &CombatResult{})

	if st.playerHP != 0 {
		t.Fatalf("playerHP = %d, want 0", st.playerHP)
	}
	if !over {
		t.Error("a solo character dropped by the crowd should end the fight")
	}
}

// ── the buff ─────────────────────────────────────────────────────────────────

// The heal fires, caps at max HP, and marks the result so the achievement can
// be granted.
func TestSeatEndOfRound_MistyHealFiresAndCaps(t *testing.T) {
	st := mistyState(t, mistyBuffed(40, 12), 34)
	var res CombatResult
	seatEndOfRound(st, st.c, CombatPhaseRoundEnd, &res)

	if st.playerHP != 40 {
		t.Errorf("playerHP = %d, want 40 — the heal caps at max HP", st.playerHP)
	}
	if !res.MistyHealed {
		t.Error("MistyHealed flag not set")
	}
	if !hasAction(st.events, "misty_heal") {
		t.Error("no misty_heal event on the log")
	}
}

// A character the crowd just dropped is not then healed back up by the same
// hook. The heal is for the living.
func TestSeatEndOfRound_NoHealForACharacterTheCrowdJustDropped(t *testing.T) {
	c := mistyCursed(40, 40)
	c.Mods.MistyHealProc = 1.0
	c.Mods.MistyHealAmt = 20

	st := mistyState(t, c, 10)
	st.actors = append(st.actors, &actor{playerHP: 5}) // an ally keeps the fight alive
	over := seatEndOfRound(st, st.c, CombatPhaseRoundEnd, &CombatResult{})

	if over {
		t.Fatal("an ally is still standing — the fight is not over")
	}
	if st.playerHP != 0 {
		t.Errorf("playerHP = %d, want 0 — the heal must not resurrect them", st.playerHP)
	}
	if hasAction(st.events, "misty_heal") {
		t.Error("misty_heal fired on a downed character")
	}
}

// ── the property that protects the corpus ────────────────────────────────────

// A character with no Misty history is untouched: no HP change, no events, and
// — because both procs short-circuit before st.randFloat() — no dice drawn. If
// the hook drew even one float, every simulated fight would diverge and
// combat_characterization.golden would move.
func TestSeatEndOfRound_UnbuffedCharacterIsUntouchedAndDrawsNoDice(t *testing.T) {
	plain := &Combatant{Name: "Plain", Stats: CombatStats{MaxHP: 40, AC: 10}}

	st := mistyState(t, plain, 40)
	control := mistyState(t, plain, 40) // same seed, never passed to the hook

	if over := seatEndOfRound(st, st.c, CombatPhaseRoundEnd, &CombatResult{}); over {
		t.Fatal("an unbuffed character cannot be dropped by a hook that does nothing")
	}
	if st.playerHP != 40 || len(st.events) != 0 {
		t.Errorf("unbuffed character was touched: hp=%d events=%d", st.playerHP, len(st.events))
	}
	// The RNG streams must still be in lockstep: the hook consumed nothing.
	if got, want := st.randFloat(), control.randFloat(); got != want {
		t.Errorf("seatEndOfRound consumed RNG: next float %v, want %v", got, want)
	}
}

// ── the wiring ───────────────────────────────────────────────────────────────

// The unit tests above call seatEndOfRound directly, which proves the hook is
// correct but not that stepRoundEnd calls it. This drives the real session API
// through a round_end step. Comment the call out of stepRoundEnd and this test
// reports PlayerHP=40 — the exploit, exactly as it shipped.
func TestStepRoundEnd_WiresSeatEndOfRound(t *testing.T) {
	setupEmptyTestDB(t)
	player := mistyCursed(40, 6)
	enemy := &Combatant{Name: "Target", Stats: CombatStats{MaxHP: 100, AC: 10}}
	sess := &CombatSession{
		SessionID: "wiring", UserID: "@u:example.org", Status: CombatStatusActive,
		PlayerHP: 40, PlayerHPMax: 40, EnemyHP: 100,
		Phase: CombatPhaseRoundEnd, Round: 1,
	}
	if _, err := advanceCombatSession(sess, player, enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.PlayerHP != 34 {
		t.Errorf("PlayerHP = %d, want 34 — stepRoundEnd never ran the hook", sess.PlayerHP)
	}
	if !hasAction(sess.TurnLog, "crowd_revenge") {
		t.Error("no crowd_revenge event persisted to the session log")
	}
}
