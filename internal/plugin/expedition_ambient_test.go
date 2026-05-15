package plugin

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

// Pure-logic tests for Phase 12 E7 (ambient ticker). The full
// deliverAmbient path mutates the prod DB; those gates already live
// behind setupAuditTestDB in other expedition tests. The behavior tested
// here — eligibility, weighted pick, quiet-window math, DM render —
// covers the resolution layer in isolation.

func TestInAmbientQuietWindow(t *testing.T) {
	day := func(h, m int) time.Time {
		return time.Date(2026, 5, 14, h, m, 0, 0, time.UTC)
	}
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"on briefing", day(6, 0), true},
		{"30 min before briefing", day(5, 30), true},
		{"59 min before briefing", day(5, 1), true},
		{"61 min before briefing", day(4, 59), false},
		{"mid-afternoon", day(14, 17), false},
		{"on recap", day(21, 0), true},
		{"30 min after recap", day(21, 30), true},
		{"61 min after recap", day(22, 1), false},
		{"deep night", day(3, 0), false},
	}
	for _, c := range cases {
		if got := inAmbientQuietWindow(c.t); got != c.want {
			t.Errorf("%s: inAmbientQuietWindow(%v) = %v, want %v", c.name, c.t, got, c.want)
		}
	}
}

func TestPickAmbientEvent_AlwaysReturnsEligible(t *testing.T) {
	// Brand-new expedition: no camp, no threat. Only "always" events
	// should ever come back. We sample many times with different seeds
	// to brute-force the weighted pick.
	e := &Expedition{Supplies: ExpeditionSupplies{Current: 5}}
	disallowed := map[string]bool{
		"camp_visitor":    true,
		"faction_whisper": true,
	}
	for i := 0; i < 200; i++ {
		rng := rand.New(rand.NewPCG(uint64(i), uint64(i*7+1)))
		ev := pickAmbientEvent(e, rng)
		if disallowed[ev.Kind] {
			t.Fatalf("seed %d picked ineligible event %q", i, ev.Kind)
		}
	}
}

func TestPickAmbientEvent_CampVisitorRequiresCamp(t *testing.T) {
	e := &Expedition{
		Supplies: ExpeditionSupplies{Current: 5},
		Camp:     &CampState{Active: true},
	}
	// With camp active, camp_visitor should appear at least once in 500 spins.
	sawVisitor := false
	for i := 0; i < 500; i++ {
		rng := rand.New(rand.NewPCG(uint64(i+1), uint64(i*13+2)))
		if pickAmbientEvent(e, rng).Kind == "camp_visitor" {
			sawVisitor = true
			break
		}
	}
	if !sawVisitor {
		t.Errorf("expected camp_visitor to be reachable when camped")
	}
}

func TestPickAmbientEvent_PackRatNeedsSupplies(t *testing.T) {
	// Starving expedition: pack_rat should never fire — there's nothing
	// for the rat to take, and a "−0.0 SU" DM would be silly.
	e := &Expedition{Supplies: ExpeditionSupplies{Current: 0}}
	for i := 0; i < 200; i++ {
		rng := rand.New(rand.NewPCG(uint64(i+3), uint64(i*5+7)))
		if pickAmbientEvent(e, rng).Kind == "pack_rat" {
			t.Fatalf("seed %d: pack_rat fired with no supplies", i)
		}
	}
}

func TestPickAmbientEvent_WhisperNeedsThreat(t *testing.T) {
	low := &Expedition{Supplies: ExpeditionSupplies{Current: 5}, ThreatLevel: 10}
	for i := 0; i < 200; i++ {
		rng := rand.New(rand.NewPCG(uint64(i+5), uint64(i*11+3)))
		if pickAmbientEvent(low, rng).Kind == "faction_whisper" {
			t.Fatalf("seed %d: whisper fired below threshold", i)
		}
	}
	hot := &Expedition{Supplies: ExpeditionSupplies{Current: 5}, ThreatLevel: 60}
	saw := false
	for i := 0; i < 500; i++ {
		rng := rand.New(rand.NewPCG(uint64(i+9), uint64(i*17+5)))
		if pickAmbientEvent(hot, rng).Kind == "faction_whisper" {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected faction_whisper to be reachable at threat 60")
	}
}

func TestPickAmbientEvent_SiegeSuppressesWhisper(t *testing.T) {
	// In siege mode the faction has stopped whispering and started
	// kicking the door. The ambient whisper would undersell the moment.
	e := &Expedition{
		Supplies:    ExpeditionSupplies{Current: 5},
		ThreatLevel: 100,
		SiegeMode:   true,
	}
	for i := 0; i < 200; i++ {
		rng := rand.New(rand.NewPCG(uint64(i+13), uint64(i*19+11)))
		if pickAmbientEvent(e, rng).Kind == "faction_whisper" {
			t.Fatalf("seed %d: whisper fired during siege", i)
		}
	}
}

func TestRenderAmbientDM_NoFooterPureFlavor(t *testing.T) {
	e := &Expedition{CurrentDay: 4}
	ev := ambientEventMonologue()
	body := renderAmbientDM(e, ev, "A pebble blinks.", "")
	if !strings.Contains(body, "Day 4") {
		t.Errorf("missing day header: %q", body)
	}
	if !strings.Contains(body, "A pebble blinks.") {
		t.Errorf("missing flavor line: %q", body)
	}
	if strings.Contains(body, "_") && strings.Count(body, "_") >= 2 {
		t.Errorf("unexpected italic footer in pure-flavor DM: %q", body)
	}
}

func TestRenderAmbientDM_WithFooter(t *testing.T) {
	e := &Expedition{CurrentDay: 7}
	body := renderAmbientDM(e, ambientEventMonologue(), "A door slams.", "Threat +1")
	if !strings.Contains(body, "Threat +1") {
		t.Errorf("missing footer: %q", body)
	}
}
