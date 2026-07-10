package plugin

import (
	"errors"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// N3/P6b — the player-facing party surface.
//
//	!expedition invite @user   (leader, Day 1)
//	!expedition accept [packs] (invitee — buys their own loadout)
//	!expedition decline
//	!expedition party          (roster + pool)
//	!expedition leave          (member; the leader ends it with !extract)
//
// The invitee pays for their own supplies, which is what makes a party fair
// rather than a free ride: the pool is the sum of what everyone carried in.

// zoneOpenToLevel reports whether a player at this level may enter the zone. The
// leader's expedition is no way around the tier gate — a party is not a taxi
// into content two tiers above your head.
func zoneOpenToLevel(zoneID ZoneID, level int) bool {
	for _, z := range zonesForLevel(level) {
		if z.ID == zoneID {
			return true
		}
	}
	return false
}

// expeditionCmdInvite asks a player to join the sender's expedition.
func (p *AdventurePlugin) expeditionCmdInvite(ctx MessageContext, rest string) error {
	target := strings.TrimSpace(rest)
	if target == "" {
		return p.SendDM(ctx.Sender, "`!expedition invite @user` — bring someone along. Day 1 only.")
	}

	exp, isLeader, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition. `!expedition start <zone>` first.")
	}
	if !isLeader {
		return p.SendDM(ctx.Sender, "Only the party leader can invite. You're along for the ride.")
	}
	if !inviteWindowOpen(exp) {
		return p.SendDM(ctx.Sender,
			"This expedition has already set off — companions join on Day 1 only.")
	}

	invitee, ok := p.ResolveUser(target, ctx.RoomID)
	if !ok {
		return p.SendDM(ctx.Sender, "Couldn't find that player.")
	}
	if invitee == ctx.Sender {
		return p.SendDM(ctx.Sender, "You're already here.")
	}
	if c, cerr := LoadDnDCharacter(invitee); cerr != nil || c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"**%s** hasn't made a character yet — they need `!setup` first.", p.DisplayName(invitee)))
	}

	zone := zoneOrFallback(exp.ZoneID)
	if err := inviteToParty(exp.ID, invitee, ctx.Sender); err != nil {
		return p.SendDM(ctx.Sender, inviteRefusalText(p.DisplayName(invitee), err))
	}

	if derr := p.SendDM(invitee, fmt.Sprintf(
		"🤝 **%s** wants you on an expedition into **%s**.\n\n"+
			"You'll buy your own supplies and they go into the party's pool. Loot and XP are yours alone — nothing is split.\n\n"+
			"`!expedition accept` to pick a supply pack, or `!expedition decline`.\n"+
			"_The offer stands for %s._",
		p.DisplayName(ctx.Sender), zone.Display, formatRespecDuration(expeditionInviteTTL))); derr != nil {
		// The invite row is committed; the DM is not. Tell the leader rather
		// than rolling back — the invitee can still `!expedition accept`.
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Invited **%s**, but couldn't DM them (%v). They can still `!expedition accept`.",
			p.DisplayName(invitee), derr))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"🤝 Asked **%s** to join you in **%s**. I'll hold the expedition here until they answer.",
		p.DisplayName(invitee), zone.Display))
}

// inviteRefusalText turns the invite layer's sentinel errors into player-facing
// copy. Anything unrecognised surfaces raw — a silent "couldn't invite" would
// hide a real bug.
func inviteRefusalText(name string, err error) string {
	switch {
	case errors.Is(err, ErrPartyFull):
		return fmt.Sprintf("Your party is full (%d, counting invites you haven't heard back on).", expeditionPartyMax)
	case errors.Is(err, ErrAlreadyInParty):
		return fmt.Sprintf("**%s** is already with you.", name)
	case errors.Is(err, ErrAlreadyInvited):
		return fmt.Sprintf("You've already asked **%s** — still waiting on them.", name)
	case errors.Is(err, ErrPlayerBusyElsewhere):
		return fmt.Sprintf("**%s** is already on an expedition of their own.", name)
	default:
		return "Couldn't send the invite: " + err.Error()
	}
}

// expeditionCmdAccept seats the sender on the expedition they were most recently
// asked to join, once they have bought their own supplies.
func (p *AdventurePlugin) expeditionCmdAccept(ctx MessageContext, c *DnDCharacter, rest string) error {
	if remaining := restingLockoutRemaining(c); remaining > 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"🛌 You're still resting — %s remaining. Pack up after.", formatRespecDuration(remaining)))
	}
	inv, err := latestInviteFor(ctx.Sender)
	if errors.Is(err, ErrInviteNotFound) {
		return p.SendDM(ctx.Sender, "Nobody has invited you anywhere.")
	}
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read your invites: "+err.Error())
	}

	exp, err := getExpedition(inv.ExpeditionID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read that expedition: "+err.Error())
	}
	if exp == nil || !inviteWindowOpen(exp) {
		_ = clearInvite(inv.ExpeditionID, ctx.Sender)
		return p.SendDM(ctx.Sender, "That expedition has already set off without you.")
	}
	// A standalone zone run isn't caught by assertNotAdventuring — it guards
	// expeditions and rosters, not bare runs. startExpedition rejects one; so
	// must this.
	if run, _ := getActiveZoneRun(ctx.Sender); run != nil {
		return p.SendDM(ctx.Sender,
			"You have an active zone run. Finish or `!zone abandon` before joining a party.")
	}

	zone := zoneOrFallback(exp.ZoneID)
	if !zoneOpenToLevel(exp.ZoneID, c.Level) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"**%s** is beyond you for now — come back at a higher level.", zone.Display))
	}

	packTok := strings.TrimSpace(rest)
	if packTok == "" {
		return p.SendDM(ctx.Sender, renderLoadoutPrompt(zone, "expedition accept"))
	}
	purchase, err := resolveLoadoutOrParse(packTok, zone.Tier)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't parse supply packs: "+err.Error())
	}
	if err := purchase.Validate(zone.Tier); err != nil {
		return p.SendDM(ctx.Sender, "Invalid pack selection: "+err.Error())
	}
	if p.euro == nil {
		return p.SendDM(ctx.Sender, "Coin system unavailable — try again later.")
	}
	cost := float64(purchase.Cost())
	if balance := p.euro.GetBalance(ctx.Sender); balance < cost {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Not enough coins. Outfitting costs **%d** but you have **%.0f**.", int(cost), balance))
	}

	if !p.euro.Debit(ctx.Sender, cost, "expedition outfitting: "+string(exp.ZoneID)) {
		return p.SendDM(ctx.Sender, "Couldn't debit outfitting cost (try again).")
	}
	if err := joinParty(exp.ID, ctx.Sender); err != nil {
		p.euro.Credit(ctx.Sender, cost, "expedition outfitting refund")
		return p.SendDM(ctx.Sender, inviteRefusalText(p.DisplayName(ctx.Sender), err))
	}
	_ = clearInvite(exp.ID, ctx.Sender)

	// Fold their packs into the pool. Do this after the seat is committed: a
	// member whose supplies landed but whose seat did not would have paid to
	// feed a party they aren't in.
	suppliesPurchase := purchase
	if isHol, _ := isHolidayToday(); isHol {
		suppliesPurchase.StandardPacks++
	}
	// updateSupplies overwrites supplies_json wholesale, so the pool has to be
	// re-read under the expedition's own lock: `exp` was fetched before the coin
	// debit, and a second invitee accepting — or the leader's day-burn tick —
	// may have rewritten the row since. Folding onto that stale snapshot would
	// silently drop their packs or resurrect spent SU.
	expMu := p.advExpeditionLock(exp.ID)
	expMu.Lock()
	fresh, err := getExpedition(exp.ID)
	if err != nil || fresh == nil {
		expMu.Unlock()
		return p.SendDM(ctx.Sender, "You're in, but your supplies didn't reach the pool.")
	}
	pooled := addSupplyPurchase(fresh.Supplies, suppliesPurchase)
	err = updateSupplies(exp.ID, pooled)
	expMu.Unlock()
	if err != nil {
		return p.SendDM(ctx.Sender, "You're in, but your supplies didn't reach the pool: "+err.Error())
	}

	leader := id.UserID(exp.UserID)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
		fmt.Sprintf("%s joined the party", p.DisplayName(ctx.Sender)), "")
	for _, uid := range expeditionAudience(exp) {
		body := fmt.Sprintf("🤝 **%s** joins the expedition into **%s**. Supplies pooled: **%.1f SU**.",
			p.DisplayName(ctx.Sender), zone.Display, pooled.Current)
		if uid == ctx.Sender {
			body = fmt.Sprintf("🤝 You join **%s** in **%s**. Party supplies: **%.1f SU**.\n\n"+
				"`!expedition party` for the roster. When a fight starts, you'll act on your own turn.",
				p.DisplayName(leader), zone.Display, pooled.Current)
		}
		if derr := p.SendDM(uid, body); derr != nil {
			return derr
		}
	}
	return nil
}

// expeditionCmdDecline drops the sender's most recent invite.
func (p *AdventurePlugin) expeditionCmdDecline(ctx MessageContext) error {
	inv, err := latestInviteFor(ctx.Sender)
	if errors.Is(err, ErrInviteNotFound) {
		return p.SendDM(ctx.Sender, "Nobody has invited you anywhere.")
	}
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read your invites: "+err.Error())
	}
	if err := clearInvite(inv.ExpeditionID, ctx.Sender); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't decline: "+err.Error())
	}
	leader := id.UserID(inv.InvitedBy)
	_ = p.SendDM(leader, fmt.Sprintf("**%s** won't be joining you. The expedition is yours to walk.",
		p.DisplayName(ctx.Sender)))
	return p.SendDM(ctx.Sender, "Declined. Maybe next time.")
}

// expeditionCmdParty renders the roster, whoever asks for it.
func (p *AdventurePlugin) expeditionCmdParty(ctx MessageContext) error {
	exp, _, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition.")
	}
	members, err := partyMembers(exp.ID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read the roster: "+err.Error())
	}
	zone := zoneOrFallback(exp.ZoneID)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎒 **%s — Day %d**\n\n", zone.Display, exp.CurrentDay))
	if len(members) == 0 {
		b.WriteString(fmt.Sprintf("**%s** — alone.\n", p.DisplayName(id.UserID(exp.UserID))))
	}
	for _, m := range members {
		role := "member"
		if m.IsLeader() {
			role = "leader"
		}
		b.WriteString(fmt.Sprintf("**%s** _(%s)_\n", p.DisplayName(id.UserID(m.UserID)), role))
	}
	if invites, ierr := pendingInvites(exp.ID); ierr == nil {
		for _, inv := range invites {
			b.WriteString(fmt.Sprintf("**%s** _(asked, no answer yet)_\n", p.DisplayName(id.UserID(inv.UserID))))
		}
	}
	b.WriteString(fmt.Sprintf("\nSupplies: **%.1f / %.1f SU**", exp.Supplies.Current, exp.Supplies.Max))
	if inviteWindowOpen(exp) {
		b.WriteString("\n\n_`!expedition invite @user` while it's still Day 1._")
	}
	return p.SendDM(ctx.Sender, b.String())
}

// expeditionCmdLeave walks a member out. The leader cannot leave — their row is
// the expedition — so they are pointed at `!extract`, which ends it for all.
func (p *AdventurePlugin) expeditionCmdLeave(ctx MessageContext) error {
	// Resolve the seat the way the guards that trap them do. seatedExpeditionFor
	// spans `extracting`, which activeExpeditionFor does not: a leader who
	// extracts and never resumes would otherwise leave their members seated —
	// refused a new adventure by the guard, and told "no active expedition" by
	// the very command the guard points them at. The exit has to see every state
	// the gate sees. It already excludes leaders, so they fall through below.
	seated, err := seatedExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if seated != nil {
		return p.leaveSeatedParty(ctx, seated)
	}

	exp, isLeader, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition.")
	}
	if isLeader {
		return p.SendDM(ctx.Sender,
			"You're leading this one — `!extract` ends it for everyone, or `!expedition abandon` to walk away from it.")
	}
	return p.leaveSeatedParty(ctx, exp)
}

// leaveSeatedParty unseats a member and tells both ends. Shared by the two ways
// a member's seat resolves: the `extracting` limbo and the plain active party.
func (p *AdventurePlugin) leaveSeatedParty(ctx MessageContext, exp *Expedition) error {
	if err := leaveParty(exp.ID, ctx.Sender); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't leave: "+err.Error())
	}
	// Supplies stay in the pool. They were spent on the expedition, not lent to
	// it, and clawing them back would let a member starve the party on their way
	// out of the door.
	_ = p.SendDM(id.UserID(exp.UserID), fmt.Sprintf(
		"**%s** turned back. Their supplies stay with the party.", p.DisplayName(ctx.Sender)))
	return p.SendDM(ctx.Sender, "You turn back for town. Your supplies stay with the party.")
}
