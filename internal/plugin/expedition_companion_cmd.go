package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// The player-facing surface for hiring Pete.
//
//	!expedition hire [class]   (leader, Day 1 — auto-fills the missing role)
//	!expedition dismiss        (leader — sends him home, no refund)
//
// He is hired on Day 1 like any other companion, because a body that materializes
// three rooms in is not a party member, it is a cheat code. See
// adventure_companion.go for why he is an NPC seat and not a player.

// expeditionCmdHire brings the correspondent along.
func (p *AdventurePlugin) expeditionCmdHire(ctx MessageContext, rest string) error {
	exp, isLeader, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition. `!expedition start <zone>` first.")
	}
	if !isLeader {
		return p.SendDM(ctx.Sender, "Only the party leader hires. You're along for the ride.")
	}
	if !inviteWindowOpen(exp) {
		return p.SendDM(ctx.Sender,
			"This expedition has already set off — Pete joins on Day 1 or not at all.")
	}
	if companionSeated(exp.ID) {
		return p.SendDM(ctx.Sender, "Pete's already with you. `!expedition party` to see the roster.")
	}
	if p.euro == nil {
		return p.SendDM(ctx.Sender, "Coin system unavailable — try again later.")
	}

	zone := zoneOrFallback(exp.ZoneID)
	level := companionPartyLevel(exp.ID)

	class, explicit := parseCompanionClass(rest)
	if !explicit {
		if arg := strings.TrimSpace(rest); arg != "" && !strings.EqualFold(arg, "auto") {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"Don't know the class %q. Name one, or just `!expedition hire` and he'll fill whatever you're missing.", arg))
		}
		class = companionRoleFill(companionPartyClasses(exp.ID))
	}

	cost := float64(companionHireCost(level, zone.Tier))
	if balance := p.euro.GetBalance(ctx.Sender); balance < cost {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Pete doesn't work for free. His fee for **%s** is **%d** — you have **%.0f**.",
			zone.Display, int(cost), balance))
	}
	if !p.euro.Debit(ctx.Sender, cost, "expedition: hired Pete") {
		return p.SendDM(ctx.Sender, "Couldn't debit Pete's fee (try again).")
	}

	if err := hireCompanion(exp.ID, class, level); err != nil {
		p.euro.Credit(ctx.Sender, cost, "expedition: Pete hire refund")
		return p.SendDM(ctx.Sender, companionRefusalText(err))
	}

	emitCompanionHireFact(ctx.Sender, class, level, zone)

	ci, _ := classInfo(class)
	filled := "You asked for a " + ci.Display + "."
	if !explicit {
		filled = "He's filling the hole in your roster — **" + ci.Display + "**."
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"📝 **Pete's coming with you into %s.** %s He's **level %d**, and his fee was **%d**.\n\n"+
			"He fights his own turns — you don't command him. He takes no loot and no XP; he's here for the story.\n"+
			"`!expedition dismiss` sends him home (no refund — he's already packed).",
		zone.Display, filled, level, int(cost)))
}

// expeditionCmdDismiss sends him home mid-run.
func (p *AdventurePlugin) expeditionCmdDismiss(ctx MessageContext) error {
	exp, isLeader, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition.")
	}
	if !isLeader {
		return p.SendDM(ctx.Sender, "Only the party leader can dismiss him.")
	}
	if err := dismissCompanion(exp.ID); err != nil {
		return p.SendDM(ctx.Sender, companionRefusalText(err))
	}
	return p.SendDM(ctx.Sender,
		"📝 **Pete heads back.** He got what he came for. The fee stays spent.")
}

// companionRefusalText turns the companion layer's sentinel errors into copy.
func companionRefusalText(err error) string {
	switch {
	case errors.Is(err, ErrCompanionAlreadyHired):
		return "Pete's already with you."
	case errors.Is(err, ErrCompanionOnAssignment):
		return "Pete's out on assignment with another party. He'll be back."
	case errors.Is(err, ErrCompanionNotHired):
		return "Pete isn't with you."
	case errors.Is(err, ErrPartyFull):
		return "No room — your party's full. Someone has to `!expedition leave` first."
	default:
		return "Couldn't hire Pete: " + err.Error()
	}
}
