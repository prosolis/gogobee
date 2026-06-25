package plugin

import (
	"strings"
	"testing"
)

// TestRenderEvent_StatefulDrainsFormatMagnitude is a regression test for the
// leaked "%d" in stateful enemy-effect narration. stat_drain, debuff and
// ally_buff carry their magnitude in CombatEvent.Damage and their flavor
// pools contain a %d placeholder; renderEvent must fmt.Sprintf them rather
// than emit the template raw (which surfaced "-%d damage" to players).
func TestRenderEvent_StatefulDrainsFormatMagnitude(t *testing.T) {
	for _, action := range []string{"stat_drain", "debuff", "ally_buff"} {
		e := CombatEvent{Actor: "enemy", Action: action, Damage: 4}
		out := renderEvent(e, "Rurina", "Shadow", CombatResult{}, newActionPicker())
		if strings.Contains(out, "%") {
			t.Errorf("%s: leaked format placeholder in output: %q", action, out)
		}
		if !strings.Contains(out, "4") {
			t.Errorf("%s: expected magnitude 4 in output: %q", action, out)
		}
	}
}
