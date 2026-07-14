package plugin

// Mischief Makers (M1) — delivery.
//
// The sibling of the ambient ticker: once a contract's window closes, the
// monster has to actually find the target mid-run. runMischiefInterrupt is
// modelled line-for-line on runHarvestInterrupt (pick enemy → ambush nick →
// runZoneCombatRoster → close out), with two deliberate departures:
//
//   - It never calls recordZoneKill and never advances zone state. The fight is
//     extrinsic to the dungeon: somebody bought it, the dungeon didn't send it.
//     Crediting it as a zone kill would let a buyer unlock a target's
//     RequiresKill resource gates for them.
//
//   - It never marks anybody dead. runHarvestInterrupt's loss path perma-kills;
//     a *purchased* attack that lands while its victim is asleep must not be
//     able to delete their character. Mischief maims: floor at 1 HP,
//     force-extract, take a bite out of the un-banked coins.

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// mischiefTickInterval — how often we look for contracts whose window has
// closed. The window itself is the pacing; the tick just has to be fine-grained
// enough that "about an hour" isn't a lie.
const mischiefTickInterval = time.Minute

// mischiefDeliveryGrace — how long past its window a contract may sit in
// `delivering` before the sweep calls it stranded. Comfortably longer than any
// chain of auto-resolved fights takes, so a slow delivery is never swept out
// from under itself (and the CAS in abandonStaleMischief is the backstop if it
// somehow is).
const mischiefDeliveryGrace = 15 * time.Minute

// mischiefTreasureWeight / mischiefRenownXP — the survival extras. Deliberately
// modest: the purse is the reward, this is the garnish.
const (
	mischiefTreasureWeight = 1.0
	mischiefEliteRenownXP  = 40
	mischiefBossRenownXP   = 120
	mischiefBossCacheSize  = 2
)

func (p *AdventurePlugin) mischiefTicker() {
	ticker := time.NewTicker(mischiefTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.fireMischiefDeliveries(time.Now().UTC())
		}
	}
}

// fireMischiefDeliveries walks contracts whose window has closed and either
// delivers the monster or fizzles the contract.
//
// The gates mirror the ambient ticker's, for the same reason: a mid-run event
// that talks over a live turn-based fight, or lands on top of the 06:00
// briefing, reads as a bug even when it isn't. A blocked contract is NOT
// fizzled — it simply waits for the next tick. The only thing that fizzles a
// contract is the expedition ending, which is what the buyer was actually
// betting against.
func (p *AdventurePlugin) fireMischiefDeliveries(now time.Time) {
	if inAmbientQuietWindow(now) {
		return
	}
	p.sweepStaleMischief(now)
	for _, c := range loadMischiefDue(now) {
		contract := c
		p.fireOneMischiefDelivery(&contract)
	}
}

// fireOneMischiefDelivery resolves a single due contract under the TARGET's lock.
//
// The lock is not optional. hasActiveCombatSession only sees the turn-based
// engine; the target's own `!explore` / autopilot walk resolves its fights
// inline (runHarvestInterrupt → runZoneCombatRoster) under advUserLock and
// reports no session. Without taking that same lock, a delivery can run a second
// combat against the same character sheet concurrently — two LoadDnDCharacter →
// mutate → SaveDnDCharacter writers, last write wins, and a whole fight's HP cost
// vanishes. The boredom ticker takes this lock for exactly this reason.
//
// Everything below the lock runs on the auto-resolve path, which takes no user
// locks of its own, so there is nothing here to deadlock against.
func (p *AdventurePlugin) fireOneMischiefDelivery(contract *mischiefContract) {
	target := contract.TargetID

	lock := p.advUserLock(target)
	lock.Lock()
	defer lock.Unlock()

	// Re-read under the lock: whatever the due-sweep saw, the expedition may have
	// ended while we were queuing behind the target's own command.
	exp, err := getActiveExpedition(target)
	if err != nil || exp == nil || exp.Status != ExpeditionStatusActive {
		p.fizzleMischief(contract)
		return
	}
	// Same safety net the ambient ticker keeps: the expedition row can still
	// say 'active' after the player has functionally left the dungeon.
	if exp.RunID != "" {
		if run, _ := getZoneRun(exp.RunID); run == nil || !run.IsActive() {
			p.fizzleMischief(contract)
			return
		}
	}
	if hasActiveCombatSession(target) {
		return // they're mid-fight; the monster can wait a minute
	}
	if !claimMischiefForDelivery(contract.ID) {
		return // another tick (or another process) already has it
	}
	// Re-read AFTER the claim, never before it. The row we were handed came from
	// the due-sweep, and the window's commands (bless, escalate) keep writing to an
	// open contract right up to the moment the claim closes it. A ward bought in
	// that gap would be paid for and never applied; an escalation would be paid for
	// and deliver the *old* tier's monster. The claim is the fence — everything the
	// fight is built from has to be read on this side of it.
	if fresh, err := mischiefContractByID(contract.ID); err == nil && fresh != nil {
		contract = fresh
	}
	if err := p.deliverMischief(contract, exp); err != nil {
		slog.Error("mischief: delivery failed",
			"contract", contract.ID, "target", target, "err", err)
	}
}

// sweepStaleMischief releases contracts stranded in `delivering` — the process
// died between the claim and the close-out (a restart mid-fight, a panic). The
// row would otherwise be immortal: the target can never be targeted again, and
// the buyer's money never comes back.
//
// Everyone who paid is made whole in full rather than rake-charged. A stranded
// delivery is our crash, not a bet they lost — and the escalator paid too, so the
// refund splits on mischiefOutlay.
//
// The ward comes back off the sheet here as well. The crash is exactly the case
// runMischiefInterrupt's `defer clearMischiefBlessings` cannot cover, and a ward
// left behind is a permanent free cushion on the target until something else
// happens to reset TempHP.
func (p *AdventurePlugin) sweepStaleMischief(now time.Time) {
	for _, c := range loadStaleMischiefDeliveries(now.Add(-mischiefDeliveryGrace)) {
		contract := c
		if !abandonStaleMischief(contract.ID) {
			continue
		}
		p.clearStrandedMischiefWard(&contract)

		buyerPaid, escPaid := mischiefOutlay(&contract)
		p.euro.Credit(contract.BuyerID, float64(buyerPaid), "mischief_refund_stranded")
		p.SendDM(contract.BuyerID, fmt.Sprintf(
			"😾 Your **%s** never reached **%s** — something went wrong on our end. Fully refunded: %s.",
			contract.Tier, p.DisplayName(contract.TargetID), fmtEuro(buyerPaid)))
		if escPaid > 0 {
			p.euro.Credit(contract.EscalatedBy, float64(escPaid), "mischief_refund_stranded")
			p.SendDM(contract.EscalatedBy, fmt.Sprintf(
				"😾 The **%s** you paid to upgrade never reached **%s** — something went wrong on our end. "+
					"Fully refunded: %s.",
				contract.Tier, p.DisplayName(contract.TargetID), fmtEuro(escPaid)))
		}
		slog.Warn("mischief: swept stranded delivery",
			"contract", contract.ID, "target", contract.TargetID)
	}
}

// clearStrandedMischiefWard takes back a ward whose delivery died before it could
// clear its own. The amount is re-derived exactly as applyMischiefBlessings
// derived it, off the target's current MaxHP.
//
// If the crash landed BEFORE the ward was ever applied, this over-subtracts into
// a well-rested cushion the target had of their own. That window is a handful of
// DB reads wide against a whole fight, clearMischiefBlessings floors at 0, and the
// alternative — leaving a stranded ward on the sheet forever — is the strictly
// worse failure.
func (p *AdventurePlugin) clearStrandedMischiefWard(c *mischiefContract) {
	if c.BlessingCount <= 0 {
		return
	}
	dc, err := LoadDnDCharacter(c.TargetID)
	if err != nil || dc == nil {
		return
	}
	if ward := mischiefBlessingTempHP(dc.HPMax, c.BlessingCount); ward > 0 {
		p.clearMischiefBlessings(c.TargetID, ward)
	}
}

// fizzleMischief refunds everyone who put money on the contract (minus the rake)
// when the monster arrives to an empty dungeon. The CAS is what makes the refund
// safe: a contract that was simultaneously claimed for delivery cannot also be
// refunded here.
//
// The buyer and the escalator are refunded SEPARATELY, off mischiefOutlay. They
// are two people who each parted with their own money, and c.Paid is the sum.
func (p *AdventurePlugin) fizzleMischief(c *mischiefContract) {
	if !fizzleMischiefContract(c.ID) {
		return
	}
	buyerPaid, escPaid := mischiefOutlay(c)

	refund, rake := p.rakedMischiefRefund(c.BuyerID, buyerPaid, "mischief_fizzle_refund")
	p.SendDM(c.BuyerID, fmt.Sprintf(
		"😾 Your **%s** got to the dungeon and found nobody home — **%s** was already out of there.\n"+
			"You're refunded %s. The other %s stays with the town, for its trouble.",
		c.Tier, p.DisplayName(c.TargetID), fmtEuro(refund), fmtEuro(rake)))

	if escPaid > 0 {
		escRefund, escRake := p.rakedMischiefRefund(c.EscalatedBy, escPaid, "mischief_fizzle_refund")
		p.SendDM(c.EscalatedBy, fmt.Sprintf(
			"😾 The **%s** you paid to upgrade found nobody home — **%s** was already out of there.\n"+
				"You're refunded %s. The other %s stays with the town, for its trouble.",
			c.Tier, p.DisplayName(c.TargetID), fmtEuro(escRefund), fmtEuro(escRake)))
		refund += escRefund
	}
	emitMischiefFizzledNews(c, refund)
}

// rakedMischiefRefund gives one stakeholder their stake back minus the fizzle
// rake, and sinks the rake against THEIR name. Returns what they got back and
// what the town kept.
func (p *AdventurePlugin) rakedMischiefRefund(who id.UserID, paid int, reason string) (refund, rake int) {
	if paid <= 0 {
		return 0, 0
	}
	refund = int(float64(paid) * mischiefFizzleRefund)
	rake = paid - refund
	if refund > 0 {
		p.euro.Credit(who, float64(refund), reason)
	}
	if rake > 0 {
		communityPotAdd(rake)
		trackTaxPaid(who, rake)
	}
	return refund, rake
}

// deliverMischief runs the purchased fight and closes it out. By the time this
// is called the contract is claimed, so it resolves exactly once.
func (p *AdventurePlugin) deliverMischief(c *mischiefContract, exp *Expedition) error {
	tier, ok := mischiefTierByKey(c.Tier)
	if !ok {
		return fmt.Errorf("unknown mischief tier %q", c.Tier)
	}
	target := c.TargetID

	dndChar, err := LoadDnDCharacter(target)
	if err != nil || dndChar == nil {
		// The target evaporated between claim and delivery. Nothing to attack —
		// treat it as a fizzle so the buyer isn't charged for a no-show.
		p.refundClaimedMischief(c, "target has no character")
		return nil
	}

	// The attacker comes from the target's own bracket, not the dungeon they are
	// standing in. See mischiefBracketZones.
	bracket := mischiefBracketZone(dndChar.Level)
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	chain, ambush := mischiefMonsters(tier.Key, bracket, rng)
	// Every link, not just the first: a zone-pool entry with no bestiary row
	// yields a zero-value template, and a nameless 0-HP monster as fight 2 of a
	// mob would read as the attack inexplicably giving up halfway.
	if len(chain) == 0 {
		p.refundClaimedMischief(c, "no monster available")
		return nil
	}
	for _, m := range chain {
		if m.Name == "" {
			p.refundClaimedMischief(c, "bestiary miss in the "+bracket.Display+" pool")
			return nil
		}
	}

	narration, monsterName, survived := p.runMischiefInterrupt(
		target, exp, bracket, chain, ambush, c.BlessingCount)

	if survived {
		p.resolveMischiefSurvived(c, tier, exp, bracket, monsterName, narration)
	} else {
		p.resolveMischiefDowned(c, exp, monsterName, narration)
	}
	return nil
}

// refundClaimedMischief unwinds a contract that was already claimed for delivery
// but couldn't be staged. Everyone who paid gets everything back — this is our
// fault, not a fizzle they gambled on. The escalator is a payer too; see
// mischiefOutlay.
func (p *AdventurePlugin) refundClaimedMischief(c *mischiefContract, reason string) {
	resolveMischiefContract(c.ID, mischiefStatusFizzled, mischiefStatusFizzled)
	buyerPaid, escPaid := mischiefOutlay(c)
	p.euro.Credit(c.BuyerID, float64(buyerPaid), "mischief_refund_unstageable")
	p.SendDM(c.BuyerID, fmt.Sprintf(
		"😾 Your **%s** never made it out of the gate. Fully refunded: %s.", c.Tier, fmtEuro(buyerPaid)))
	if escPaid > 0 {
		p.euro.Credit(c.EscalatedBy, float64(escPaid), "mischief_refund_unstageable")
		p.SendDM(c.EscalatedBy, fmt.Sprintf(
			"😾 The **%s** you paid to upgrade never made it out of the gate. Fully refunded: %s.",
			c.Tier, fmtEuro(escPaid)))
	}
	slog.Warn("mischief: contract unstageable", "contract", c.ID, "reason", reason)
}

// runMischiefInterrupt fights the whole chain and reports whether the target was
// still standing at the end.
//
// Survival is read off the target's HP, not off PlayerWon. The engine's timeout
// is a retreat, not a lethal blow — a target who ran out the clock with HP left
// held the thing off, and a bought monster that merely *outlasted* them has not
// earned a maiming. The chain continues while they stand: a mob is three
// attackers, and beating the first two doesn't mean the third goes home.
func (p *AdventurePlugin) runMischiefInterrupt(
	target id.UserID,
	exp *Expedition,
	bracket ZoneDefinition,
	chain []DnDMonsterTemplate,
	ambush bool,
	blessings int,
) (narration, monsterName string, survived bool) {
	var b strings.Builder
	last := chain[len(chain)-1].Name

	if ward := p.applyMischiefBlessings(target, blessings); ward > 0 {
		b.WriteString(fmt.Sprintf(
			"_The town's wards hold — %d of them, worth %d HP of cushion._\n", blessings, ward))
		defer p.clearMischiefBlessings(target, ward)
	}

	for i, monster := range chain {
		// The ambush is the attacker's privilege — it was lying in wait, and the
		// target had no reason to expect it. Same one-free-swing approximation the
		// harvest interrupt uses, with the same wounded-entrant clamp so it can't
		// pre-empt the fight outright.
		if ambush && i == 0 {
			if dc, _ := LoadDnDCharacter(target); dc != nil {
				nick := clampSurpriseNick(
					surpriseRoundNick(monster, int(bracket.Tier)), dc.HPCurrent, dc.HPMax)
				if nick > 0 {
					dc.HPCurrent -= nick
					_ = SaveDnDCharacter(dc)
					b.WriteString(fmt.Sprintf(
						"_It was waiting for you. **%s** opens with a free swing — %d HP._\n",
						monster.Name, nick))
				}
			}
		}

		preHP, _ := dndHPSnapshot(target)
		pres, _, err := p.runZoneCombatRoster(
			fightRoster(target), monster, int(bracket.Tier), dungeonCombatPhases, exp.DMMood)
		if err != nil {
			// Mid-chain build failure. The target keeps whatever HP they had; treat
			// them as having survived rather than maiming them on our bug.
			slog.Error("mischief: combat error", "target", target, "err", err)
			b.WriteString(fmt.Sprintf("_(The %s never quite arrives.)_\n", monster.Name))
			return b.String(), last, true
		}
		postHP, maxHP := dndHPSnapshot(target)

		b.WriteString(fmt.Sprintf("⚔️ **%s** (HP %d, AC %d)\n", monster.Name, monster.HP, monster.AC))
		switch {
		case pres.Seats[0].PlayerWon:
			b.WriteString(fmt.Sprintf("✅ **%s** down — %d→%d / %d HP.\n",
				monster.Name, preHP, postHP, maxHP))
		case postHP > 0:
			// The engine's timeout: they held it off without killing it. That still
			// pays — but say so plainly, or a stalemate reads as a clean kill and the
			// player wonders why the thing is still following them.
			b.WriteString(fmt.Sprintf("⏳ **%s** breaks off, still breathing. You're at %d / %d HP.\n",
				monster.Name, postHP, maxHP))
		}
		if postHP <= 0 {
			return b.String(), monster.Name, false
		}
		if i < len(chain)-1 {
			b.WriteString("_And it isn't alone._\n")
		}
	}
	return b.String(), last, true
}

// applyMischiefBlessings turns the room's wards into temp HP on the target's
// sheet for the duration of the delivery, and returns the cushion granted.
//
// TempHP is the well-rested mechanism (applyDnDHPScaling layers it above MaxHP
// for the fight, and persistDnDHPAfterCombat clamps back down to MaxHP after), so
// a ward is a damage sponge and nothing more — it can't leave the target at more
// HP than they started with.
//
// It ADDS to whatever TempHP is already there rather than overwriting: a target
// who long-rested at a T3 home before setting out is carrying a cushion of their
// own, and a ward bought to help them must not silently delete it. clearMischiefBlessings
// subtracts exactly what we added, which is why this returns the amount.
//
// The cushion refreshes for each link of a chain, because TempHP is a sheet field
// and applyDnDHPScaling re-reads it per fight. That only ever matters for the mob
// tier (the sole multi-fight chain), which the M0 sweep priced at 98-100% survival
// as pure theatre anyway. The tiers where a ward decides anything — elite and boss —
// are single fights, so what the room buys there is exactly one sponge.
func (p *AdventurePlugin) applyMischiefBlessings(target id.UserID, blessings int) int {
	if blessings <= 0 {
		return 0
	}
	c, err := LoadDnDCharacter(target)
	if err != nil || c == nil {
		return 0
	}
	ward := mischiefBlessingTempHP(c.HPMax, blessings)
	if ward <= 0 {
		return 0
	}
	c.TempHP += ward
	if err := SaveDnDCharacter(c); err != nil {
		return 0
	}
	return ward
}

// clearMischiefBlessings takes the ward back off the sheet once the delivery is
// over — the blessing was bought for THIS fight, not for the rest of the run.
// Subtracts what we added, so a pre-existing well-rested cushion survives intact.
func (p *AdventurePlugin) clearMischiefBlessings(target id.UserID, ward int) {
	c, err := LoadDnDCharacter(target)
	if err != nil || c == nil {
		return
	}
	if c.TempHP -= ward; c.TempHP < 0 {
		c.TempHP = 0
	}
	_ = SaveDnDCharacter(c)
}

// floorMischiefRoster raises every seat the delivery put on the floor back to
// 1 HP. It runs on BOTH outcomes, and that is not belt-and-braces: the leader
// can win a fight their friend went down in, and a mischief delivery
// deliberately skips closeOutZoneWin/closeOutZoneLoss (which is what normally
// marks a 0-HP seat dead). Without this, a dropped party member is left alive at
// 0 HP — a state nothing else in the game produces, and one the gates that read
// `HPCurrent <= 0` (expedition autorun, for one) treat as broken rather than
// dead. Nobody dies for money; nobody gets stuck at zero for it either.
func (p *AdventurePlugin) floorMischiefRoster(target id.UserID) {
	for _, uid := range fightRoster(target) {
		if isCompanionSeat(uid) {
			continue // he takes no wounds home; he has no rows to floor
		}
		floorHPAtOne(uid)
	}
}

// resolveMischiefSurvived pays the target and unseals the buyer.
//
// The purse is a percentage of the base fee — never of what the buyer paid — so
// it is always strictly less than the buyer's outlay. See mischiefPurse.
func (p *AdventurePlugin) resolveMischiefSurvived(
	c *mischiefContract, tier mischiefTier, exp *Expedition, bracket ZoneDefinition,
	monsterName, narration string,
) {
	// The leader lived; a friend of theirs may not have. See floorMischiefRoster.
	p.floorMischiefRoster(c.TargetID)

	purse := mischiefPurse(c.Fee, tier.PayoutPct)
	p.euro.Credit(c.TargetID, float64(purse), "mischief_survived")

	// Everything spent on the contract that isn't the purse — the rake, plus the
	// whole sign surcharge — is a sink. That is what makes the payout cap bite. It
	// is booked against the buyer AND the escalator in proportion to what each of
	// them actually put in; c.Paid is the sum of the two.
	if rake := c.Paid - purse; rake > 0 {
		communityPotAdd(rake)
		trackMischiefSink(c, rake)
	}

	// Extras by tier. Treasure rolls against the zone they're actually in — the
	// loot is scavenged off a corpse in that dungeon, wherever the thing came from.
	var extras []string
	switch tier.Key {
	case "mob":
		p.rollZoneTreasure(c.TargetID, exp.ZoneID, mischiefTreasureWeight)
	case "elite":
		p.rollZoneTreasure(c.TargetID, exp.ZoneID, mischiefTreasureWeight)
		if before, after, err := addRenownXP(c.TargetID, mischiefEliteRenownXP); err == nil {
			p.announceRenown(c.TargetID, renownLevelFor(before), renownLevelFor(after))
			extras = append(extras, "The story of it is already going round town.")
		}
	case "boss":
		p.rollZoneTreasure(c.TargetID, exp.ZoneID, mischiefTreasureWeight)
		if before, after, err := addRenownXP(c.TargetID, mischiefBossRenownXP); err == nil {
			p.announceRenown(c.TargetID, renownLevelFor(before), renownLevelFor(after))
			extras = append(extras, "People are going to be telling this one for a while.")
		}
		for _, item := range consumableCache(int(bracket.Tier), mischiefBossCacheSize) {
			it := item
			p.grantZoneItem(c.TargetID, &it, "🧪")
		}
	}

	resolveMischiefContract(c.ID, mischiefStatusDelivered, mischiefOutcomeSurvived)

	var dm strings.Builder
	dm.WriteString("😈 **Somebody paid for that.**\n\n")
	dm.WriteString(narration)
	dm.WriteString(fmt.Sprintf(
		"\n🛡️ You're still standing, and the coin was never really theirs. **%s** is yours.\n",
		fmtEuro(purse)))
	dm.WriteString(fmt.Sprintf("It was **%s** who paid to have you killed. Do with that what you like.%s",
		p.DisplayName(c.BuyerID), p.mischiefEscalatorNote(c)))
	for _, e := range extras {
		dm.WriteString("\n_" + e + "_")
	}
	p.SendDM(c.TargetID, dm.String())

	p.SendDM(c.BuyerID, fmt.Sprintf(
		"🛡️ **%s walked away from your %s.** They keep %s of your money, and the town knows your name now.",
		p.DisplayName(c.TargetID), c.Tier, fmtEuro(purse)))
	if c.EscalatedBy != "" && c.EscalatedBy != c.BuyerID {
		p.SendDM(c.EscalatedBy, fmt.Sprintf(
			"🛡️ **%s** walked away from the **%s** you paid to upgrade — and your money went into their purse.\n"+
				"_The town knows you piled on. That was the deal._",
			p.DisplayName(c.TargetID), c.Tier))
	}

	p.announceMischiefSurvived(c, monsterName, purse)
	emitMischiefResolvedNews(c, monsterName, mischiefOutcomeSurvived, purse)
}

// resolveMischiefDowned maims the target: HP floored at 1, expedition
// force-extracted, un-banked coins docked. Nobody dies for money.
//
// The whole fee goes to the community pot — never back to the buyer. A
// successful hit buys glory, not a rebate; refunding it would make repeat
// contracts free and turn the cap into a rounding error.
func (p *AdventurePlugin) resolveMischiefDowned(
	c *mischiefContract, exp *Expedition, monsterName, narration string,
) {
	// Every seat, not just the leader: a party that went down with them went down
	// to a bought monster too, and the no-perma-death rule is about the purchase,
	// not about who was holding the torch. Must run BEFORE the extract, which
	// releases the party and would leave us nobody to floor.
	p.floorMischiefRoster(c.TargetID)

	// forcedExtractExpedition retires the region runs and releases the party itself;
	// only the zone run has to be closed here.
	_ = abandonZoneRun(c.TargetID)
	_, tax, err := forcedExtractExpedition(exp.ID, "mischief contract")
	if err != nil {
		slog.Warn("mischief: force extract failed", "expedition", exp.ID, "err", err)
	}
	// The un-banked-coin loss. forcedExtractExpedition computes the 20% cut and
	// leaves the debit to the caller; for a maiming, this is the caller.
	if tax > 0 {
		p.euro.Debit(c.TargetID, float64(tax), "mischief_downed_loss")
	}

	communityPotAdd(c.Paid)
	trackMischiefSink(c, c.Paid)

	resolveMischiefContract(c.ID, mischiefStatusDelivered, mischiefOutcomeDowned)

	var dm strings.Builder
	dm.WriteString("😈 **Somebody paid for that.**\n\n")
	dm.WriteString(narration)
	dm.WriteString(fmt.Sprintf(
		"\n💀 **%s** put you down. You wake up on a cart headed home — alive, which is more than the contract asked for.\n",
		monsterName))
	dm.WriteString("Your expedition is over.")
	if tax > 0 {
		dm.WriteString(fmt.Sprintf(" Whatever you hadn't banked yet is gone — %s of it.", fmtEuro(tax)))
	}
	if c.Signed {
		dm.WriteString(fmt.Sprintf("\n\nThe contract was signed: **%s** paid for it.", p.DisplayName(c.BuyerID)))
	} else {
		dm.WriteString("\n\nNobody's saying who paid.")
	}
	p.SendDM(c.TargetID, dm.String())

	p.SendDM(c.BuyerID, fmt.Sprintf(
		"💀 Your **%s** found **%s** and put them on the floor. Their expedition is over.\n"+
			"_You get nothing back — %s went to the community pot. You did this for the story._",
		c.Tier, p.DisplayName(c.TargetID), fmtEuro(c.Paid)))
	if c.EscalatedBy != "" && c.EscalatedBy != c.BuyerID {
		// They stay anonymous: only a survival unseals. Their money is gone either
		// way — that is what makes the survival unseal a real risk to have taken.
		p.SendDM(c.EscalatedBy, fmt.Sprintf(
			"💀 The **%s** you paid to upgrade found **%s** and put them down. Nobody knows it was you.",
			c.Tier, p.DisplayName(c.TargetID)))
	}

	p.announceMischiefDowned(c, monsterName)
	emitMischiefResolvedNews(c, monsterName, mischiefOutcomeDowned, 0)
}
