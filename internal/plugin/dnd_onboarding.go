package plugin

import (
	"fmt"
	"log/slog"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// One-shot onboarding DM fired the first time a legacy player encounters
// the new D&D layer. Triggered from ensureDnDCharacterForCombat at the
// moment of fresh auto-migration; suppressed for genuinely new players
// (combat_level <= 1) since the message frames around "previous players".

// dndLegacyMinLevel — players with combat_level at or above this are
// considered "legacy players" who deserve the onboarding spiel. Anyone
// at L1 is brand new and gets the standard !setup nudge instead.
const dndLegacyMinLevel = 2

// maybeSendDnDOnboarding sends the welcome DM iff the player has visible
// legacy adventure progress AND hasn't been onboarded before. The
// OnboardingSent flag survives draft cancellation, !respec, and any other
// state mutation — once a player has seen the welcome, they never see it
// again. Logs and continues on send failure (DM is best-effort, never blocks combat).
func (p *AdventurePlugin) maybeSendDnDOnboarding(userID id.UserID, advChar *AdventureCharacter, dnd *DnDCharacter) {
	if advChar == nil || advChar.CombatLevel < dndLegacyMinLevel {
		return
	}
	if dnd == nil || dnd.OnboardingSent {
		return
	}
	// Skip entirely if there's no Matrix client — tests construct empty
	// AdventurePlugin{} and we shouldn't write to the DB on a nil-send.
	if p == nil || p.Client == nil {
		return
	}
	msg := dndOnboardingText(advChar.CombatLevel, dnd.Level)
	if err := p.SendDM(userID, msg); err != nil {
		slog.Error("dnd: onboarding DM failed", "user", userID, "err", err)
		// Don't mark as sent if delivery failed — they should get a chance
		// on their next combat. Send failures here are typically transient.
		return
	}
	dnd.OnboardingSent = true
	if err := SaveDnDCharacter(dnd); err != nil {
		slog.Error("dnd: persist onboarding flag", "user", userID, "err", err)
	}
}

func dndOnboardingText(oldLevel, newLevel int) string {
	prelude := ""
	if line := flavor.Pick(flavor.ExpeditionStart); line != "" {
		prelude = "_" + line + "_\n\n"
	}
	return prelude + fmt.Sprintf(`Hi there! Welcome to the new Adventure game!

We shamelessly cribbed.. aimed for feature parity with our competitors.
And the result is this..! and adventure game with most of the best Dungeons & Drag-

"AHEM!" *TwinBee glances at the Pinkerton agent in the corner of the room.

..d20 System mechanics ready for you to explore!

As a result of these amazing and entirely necessary changes that weren't done at the whim of a bored engineer.. the level system has changed. But no worries! We spent hours coming up with an algorithm that would ensure each player arrives at a level that is fully representative of their level under the previous system (..by dividing your previous level by five).

Your previous level **%d** is now Adv 2.0 level **%d**.

Enjoy!

Type !setup to get your character situated under this hot new and legally distinct system.`,
		oldLevel, newLevel)
}
