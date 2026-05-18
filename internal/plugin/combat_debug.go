package plugin

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// dumpCombatEventsIfDebug logs every event in a CombatResult to slog.Info
// when GOGOBEE_COMBAT_DEBUG=1 is set. Renders one event per line in a
// compact form: "[round phase actor.action] hp=P/E roll=R vsAC=V dmg=D desc".
//
// Used to diagnose ghost damage / dropped events: the renderer silently
// returns "" for unknown actions, which makes damage that hit the engine
// state but never reached the chat invisible. This dump shows everything.
func dumpCombatEventsIfDebug(label string, result CombatResult) {
	if os.Getenv("GOGOBEE_COMBAT_DEBUG") != "1" {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n=== combat dump: %s ===\n", label))
	b.WriteString(fmt.Sprintf("PlayerStartHP=%d PlayerEntryHP=%d EnemyStartHP=%d EnemyEntryHP=%d PlayerEndHP=%d EnemyEndHP=%d won=%v\n",
		result.PlayerStartHP, result.PlayerEntryHP, result.EnemyStartHP, result.EnemyEntryHP,
		result.PlayerEndHP, result.EnemyEndHP, result.PlayerWon))
	prevP := result.PlayerEntryHP
	if prevP == 0 {
		prevP = result.PlayerStartHP
	}
	prevE := result.EnemyEntryHP
	if prevE == 0 {
		prevE = result.EnemyStartHP
	}
	for i, e := range result.Events {
		dp := e.PlayerHP - prevP
		de := e.EnemyHP - prevE
		b.WriteString(fmt.Sprintf("  %3d [r%d %s %s.%s] hp=%d/%d (Δp=%+d Δe=%+d) roll=%d vsAC=%d dmg=%d desc=%q\n",
			i, e.Round, e.Phase, e.Actor, e.Action, e.PlayerHP, e.EnemyHP, dp, de,
			e.Roll, e.RollAgainst, e.Damage, e.Desc))
		prevP, prevE = e.PlayerHP, e.EnemyHP
	}
	b.WriteString("=== end dump ===\n")
	slog.Info("combat debug dump", "log", b.String())
}
