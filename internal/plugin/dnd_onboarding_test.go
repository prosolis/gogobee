package plugin

import (
	"strings"
	"testing"
)

func TestDnDOnboardingText_ContainsLevelMapping(t *testing.T) {
	msg := dndOnboardingText(49, 9)
	if !strings.Contains(msg, "**49**") {
		t.Error("onboarding text doesn't include old level")
	}
	if !strings.Contains(msg, "**9**") {
		t.Error("onboarding text doesn't include new level")
	}
	// Spot-check signature lines from the user's brief.
	for _, snippet := range []string{
		"Welcome to the new Adventure game",
		"Pinkerton agent",
		"dividing your previous level by five",
		"!setup",
		"legally distinct",
	} {
		if !strings.Contains(msg, snippet) {
			t.Errorf("onboarding missing expected snippet: %q", snippet)
		}
	}
}

// TestMaybeSendDnDOnboarding_LegacyThreshold: only fires for combat_level >= 2.
// The DM call itself is no-op in tests (Base.Client is nil and SendDM guards),
// so we can't assert on send — but we can assert the function doesn't panic
// or error for either branch.
func TestMaybeSendDnDOnboarding_DoesNotPanic(t *testing.T) {
	p := &AdventurePlugin{}
	dnd := &DnDCharacter{Level: 4}

	// Brand-new player (combat_level=1): should be a no-op.
	p.maybeSendDnDOnboarding("@new:x", &AdventureCharacter{CombatLevel: 1}, dnd)

	// Legacy player (combat_level=20): should attempt to send (no-op'd by nil Client).
	p.maybeSendDnDOnboarding("@legacy:x", &AdventureCharacter{CombatLevel: 20}, dnd)

	// nil advChar: should be a no-op.
	p.maybeSendDnDOnboarding("@nil:x", nil, dnd)
}

func TestDnDLegacyMinLevel(t *testing.T) {
	if dndLegacyMinLevel < 1 {
		t.Errorf("dndLegacyMinLevel = %d, must be ≥ 1", dndLegacyMinLevel)
	}
}
