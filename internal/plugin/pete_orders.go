package plugin

// The web action queue's game-side loop — the equip queue's sibling, and the
// first one where the web plays the game rather than dressing the character.
//
// An owner, signed in on Pete, asks for something: pull out of a run, take
// today's swing at the Siege, set out for a zone, walk back into the run they
// extracted from, hire the pet sitter. Pete records the intent; we poll for it,
// run the real command path (the same one `!extract`, `!expedition start`,
// `!resume` and the rest run — not a second implementation of it), and file a
// verdict Pete shows them.
//
// Same non-idempotency problem as equip, with higher stakes: replaying an
// extraction would end a run the player had already resumed, and replaying a bout
// would spend a day's swing they never got back. So before touching anything we
// check the adv_applied_orders ledger — if this order's guid is there, the
// mutation landed on an earlier tick and we only lost the verdict-ack, so we
// re-file the stored verdict and mutate nothing.
//
// The poll is faster than equip's (15s vs 30s) for one reason: an extraction is
// the answer to something the player is *watching* go wrong on the who page. A
// minute of silence there reads as a button that didn't work.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"
	"maunium.net/go/mautrix/id"
)

const (
	advOrderPollInterval = 15 * time.Second
	// A Siege bout runs a full combat and streams its narration to Matrix before
	// takeSiegeBout returns, so this budget is minutes, not seconds — the equip
	// path's 20s would abandon a fight that was going fine.
	advOrderPollTimeout = 5 * time.Minute
)

// peteAdvOrderTicker polls Pete for web actions and fulfils them. Started
// alongside the other adventure tickers; exits on stopCh.
func (p *AdventurePlugin) peteAdvOrderTicker() {
	if !peteclient.Enabled() {
		return // no Pete wire configured; the action queue is simply off
	}
	ticker := time.NewTicker(advOrderPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.pollAdvOrders()
		}
	}
}

func (p *AdventurePlugin) pollAdvOrders() {
	ctx, cancel := context.WithTimeout(context.Background(), advOrderPollTimeout)
	defer cancel()

	orders, err := peteclient.PendingOrders(ctx)
	if err != nil {
		// A Pete predating the queue answers 404; a wire blip looks the same. Quiet
		// on purpose — this must not spam while Pete hasn't shipped the endpoint.
		slog.Debug("orders: poll failed", "err", err)
		return
	}
	for _, order := range orders {
		p.fulfilAdvOrder(ctx, order)
	}
}

// fulfilAdvOrder applies one action and files its verdict. A transient failure is
// left pending for the next poll (no verdict); a permanent one gets a specific
// rejection. The guid ledger makes a re-offer after a lost ack a no-op that
// simply re-files the verdict.
func (p *AdventurePlugin) fulfilAdvOrder(ctx context.Context, order peteclient.AdvOrder) {
	// Already applied on an earlier tick? Re-file the stored verdict, mutate nothing.
	if status, detail, ok := advOrderAlreadyApplied(order.GUID); ok {
		if err := peteclient.VerdictOrder(ctx, order.GUID, status, detail); err != nil {
			slog.Warn("orders: re-file verdict push failed, will retry next poll",
				"order", order.GUID, "status", status, "err", err)
		}
		return
	}

	owner, ok := p.equipOwnerMXID(order.OwnerLocalpart)
	if !ok {
		// The client isn't up (tests) or the localpart is empty. Not our order to
		// fail permanently — leave it pending and try again once we can name the owner.
		slog.Debug("orders: cannot resolve owner, leaving pending", "order", order.GUID, "owner", order.OwnerLocalpart)
		return
	}

	status, detail, retry := p.applyAdvOrder(owner, order)
	if retry {
		return // transient; leave pending for the next tick
	}

	// Record the verdict BEFORE pushing it, so a crash after the mutation still
	// short-circuits next tick and re-files rather than re-applying. Same ordering
	// and the same reasoning as the equip poller.
	if err := recordAdvApplied(order.GUID, status, detail); err != nil {
		slog.Warn("orders: failed to record applied order, leaving pending",
			"order", order.GUID, "status", status, "err", err)
		return
	}
	if err := peteclient.VerdictOrder(ctx, order.GUID, status, detail); err != nil {
		slog.Warn("orders: verdict push failed, will re-file next poll",
			"order", order.GUID, "status", status, "err", err)
		return
	}
	slog.Info("orders: web action fulfilled", "order", order.GUID, "action", order.Action, "status", status)
}

// applyAdvOrder runs the real action. It returns the terminal status and a human
// note for Pete, or retry=true for a transient fault that should leave the order
// pending. It records nothing and pushes nothing — the caller does both.
//
// Note what is NOT here: a per-user lock. Almost every verb takes it inside its
// own shared helper (performExtraction, takeSiegeBout, performResume,
// performBabysitPurchase), which is what serialises them against the Matrix
// commands running the very same code. Taking it here too would deadlock on a
// non-reentrant mutex.
//
// The exceptions are the three twins whose Matrix caller already holds the lock
// across the whole `!expedition` switch and so cannot take it themselves —
// performExpeditionStart, performExpeditionAbandon and performExpeditionLeave.
// Their apply wrappers below take it instead. That asymmetry is written down in
// both places because getting it wrong does not fail loudly: it wedges the
// player's lock forever and every later adventure command from them hangs. The
// rule for a new verb is not "web wrappers take the lock" — it is "look at what
// the Matrix caller does", and the two babysit verbs go the other way.
func (p *AdventurePlugin) applyAdvOrder(owner id.UserID, order peteclient.AdvOrder) (status, detail string, retry bool) {
	switch order.Action {
	case peteclient.AdvOrderExtract:
		out, err := p.performExtraction(owner)
		switch {
		case errors.Is(err, errExtractNoRun):
			return "rejected_not_running", "You weren't on an expedition.", false
		case errors.Is(err, errExtractNotLeader):
			return "rejected_not_leader", "Only the party leader can call the extraction.", false
		case err != nil:
			// Every remaining failure here is a DB fault. Leave it pending: nothing
			// has been written, so the next tick retries cleanly.
			slog.Warn("orders: extraction failed", "order", order.GUID, "user", owner, "err", err)
			return "", "", true
		}
		return "applied", fmt.Sprintf(
			"Out of %s on day %d. Loot, XP and coins kept. Say !resume within 7 days to go back in.",
			out.Zone, out.Day), false

	case peteclient.AdvOrderSiegeJoin:
		bout, boss, err := p.takeSiegeBout(owner)
		switch {
		case errors.Is(err, errSiegeNoBoss):
			return "rejected_no_siege", "No Siege is camped outside town right now.", false
		case errors.Is(err, errSiegeNoCharacter):
			return "rejected_unavailable", "You don't have an adventurer yet.", false
		case errors.Is(err, errSiegeDead):
			return "rejected_unavailable", "You're dead. The Siege will have to wait.", false
		case errors.Is(err, errSiegeAlreadyFought):
			return "rejected_already_fought", "You've already taken your bout today. One fight per day.", false
		case err != nil:
			// A combat that errored persisted nothing terminal, but it may have
			// written HP. Retry is still right: the once-per-day gate is stamped by
			// the contribution row, which only lands on a bout that completed.
			slog.Warn("orders: siege bout failed", "order", order.GUID, "user", owner, "err", err)
			return "", "", true
		}
		// The blow-by-blow went to Matrix; the web gets the same one-line result the
		// narration closed with, minus its markdown.
		return "applied", advOrderPlainText(siegeBoutFooter(bout, boss)), false

	case peteclient.AdvOrderExpedition:
		return p.applyWebExpeditionStart(owner, order)

	case peteclient.AdvOrderResume:
		return p.applyWebResume(owner, order)

	case peteclient.AdvOrderBabysit:
		return p.applyWebBabysit(owner, order)

	case peteclient.AdvOrderAbandon:
		return p.applyWebAbandon(owner)

	case peteclient.AdvOrderLeave:
		return p.applyWebLeave(owner)

	case peteclient.AdvOrderBabysitCancel:
		return p.applyWebBabysitCancel(owner)

	default:
		// Pete validates the action before it ever queues an order, so this is a
		// contract breach, not a user mistake. Reject permanently rather than spin.
		return "rejected_unavailable", "Unknown action.", false
	}
}

// ---- the three verbs that take arguments and spend coins ------------------------
//
// Everything below re-resolves its own arguments against the game's own tables.
// Pete only ever offers what gogobee quoted it (see pete_offers.go), but a quote
// is up to two minutes stale and is not a permission — so the zone is looked up
// again in availableZonesFor, the loadout is priced again at the real tier, and
// the fee is read again at the real level. A forged param buys nothing.
//
// All three are money moves on a retrying wire, so each hands the order guid down
// as the idempotency key. Nothing here refunds-then-retries: once a refund has
// happened the guid-keyed debit will not charge again, so a retry would hand over
// the goods for free. Every failure past the debit is therefore permanent.

// applyWebExpeditionStart sends the owner's adventurer out of town.
func (p *AdventurePlugin) applyWebExpeditionStart(owner id.UserID, order peteclient.AdvOrder) (status, detail string, retry bool) {
	if order.Params == nil || order.Params.Zone == "" {
		return "rejected_unavailable", "That order didn't say where to.", false
	}
	// performExpeditionStart is the one headless twin that does NOT take the
	// per-user lock (its Matrix caller already holds it across the whole
	// `!expedition` switch), so this is the one order case that has to.
	userMu := p.advUserLock(owner)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(owner)
	if err != nil {
		return "", "", true // a DB fault; nothing written, so the next tick retries cleanly
	}
	if c == nil || c.PendingSetup {
		return "rejected_unavailable", "You don't have an adventurer yet.", false
	}
	zoneID, ok := resolveZoneInput(order.Params.Zone, availableZonesFor(owner, c.Level))
	if !ok {
		if reason := postgameLockReason(order.Params.Zone, owner, c.Level); reason != "" {
			return "rejected_zone_locked", advOrderPlainText(reason), false
		}
		return "rejected_zone_locked", "That zone isn't open to you right now.", false
	}
	zone, _ := getZone(zoneID)
	// An unknown loadout is refused rather than defaulted. A default here would
	// spend coins on a pack size the player never picked.
	loadout, ok := parseLoadoutToken(order.Params.Loadout)
	if !ok {
		return "rejected_unavailable", "That isn't a loadout I sell.", false
	}
	out, err := p.performExpeditionStart(owner, c, zone, loadoutPurchase(zone.Tier, loadout), order.GUID)
	if err != nil {
		status := "rejected_unavailable"
		switch {
		case errors.Is(err, errExpStartZoneLocked):
			status = "rejected_zone_locked"
		case errors.Is(err, errExpStartBusy):
			status = "rejected_busy"
		case errors.Is(err, errExpStartBroke):
			status = "rejected_insufficient_funds"
		}
		// Everything else — still resting, a bad pack count, a start that tore
		// itself down and refunded — is rejected_unavailable, and the detail line
		// carries the specifics.
		return status, advOrderPlainText(err.Error()), false
	}
	return "applied", fmt.Sprintf(
		"Out of town, bound for %s, with the %s loadout: %d coins, about %d days of provisions.",
		out.Zone.Display, loadoutName(loadout), out.Cost, out.Days), false
}

// applyWebResume walks the owner back into the run they extracted from.
func (p *AdventurePlugin) applyWebResume(owner id.UserID, order peteclient.AdvOrder) (status, detail string, retry bool) {
	// The loadout is required here, unlike in Matrix: an empty one asks
	// performResume for the pick-a-loadout prompt, which is a DM, not a verdict.
	if order.Params == nil || order.Params.Loadout == "" {
		return "rejected_unavailable", "That order didn't say what to pack.", false
	}
	if _, ok := parseLoadoutToken(order.Params.Loadout); !ok {
		return "rejected_unavailable", "That isn't a loadout I sell.", false
	}
	out, err := p.performResume(owner, order.Params.Loadout, order.GUID)
	if err != nil {
		var refusal advRefusal
		if !errors.As(err, &refusal) {
			// Not a refusal at all — a DB fault reading expedition state. Nothing
			// has been written, so leave it pending.
			slog.Warn("orders: resume failed", "order", order.GUID, "user", owner, "err", err)
			return "", "", true
		}
		status := "rejected_unavailable"
		switch {
		case errors.Is(err, errResumeBusy):
			status = "rejected_busy"
		case errors.Is(err, errResumeNothing), errors.Is(err, errResumeLapsed):
			status = "rejected_nothing_to_resume"
		case errors.Is(err, errResumeBroke):
			status = "rejected_insufficient_funds"
		}
		return status, advOrderPlainText(err.Error()), false
	}
	return "applied", fmt.Sprintf(
		"Back into %s on day %d, re-outfitted for %d coins.",
		out.Zone.Display, out.Day, out.Purchase.Cost()), false
}

// applyWebBabysit engages the pet sitter for a week or a month.
func (p *AdventurePlugin) applyWebBabysit(owner id.UserID, order peteclient.AdvOrder) (status, detail string, retry bool) {
	// The two durations the game sells. Anything else is a contract breach rather
	// than a user mistake, since Pete offers exactly these two.
	days := 0
	if order.Params != nil {
		days = order.Params.Days
	}
	if days != 7 && days != 30 {
		return "rejected_unavailable", "The sitter works by the week or by the month.", false
	}
	out, err := p.performBabysitPurchase(owner, days, order.GUID)
	if err != nil {
		status := "rejected_unavailable"
		switch {
		case errors.Is(err, errBabysitActive):
			status = "rejected_busy"
		case errors.Is(err, errBabysitBroke):
			status = "rejected_insufficient_funds"
		}
		return status, advOrderPlainText(err.Error()), false
	}
	label := "a week"
	if out.Days == 30 {
		label = "a month"
	}
	note := fmt.Sprintf("Sitter engaged for %s, %d coins.", label, out.Cost)
	if out.PetName != "" {
		note += fmt.Sprintf(" %s is in good hands.", out.PetName)
	}
	return "applied", note, false
}

// ---- the three verbs that take no arguments and spend nothing -------------------
//
// Each of these was already named inside a verdict the web shows: "!expedition
// abandon first", "!expedition leave to walk out alone", "cancel early (no
// refund)". A page that tells somebody to go and type a command it could have
// offered them is a page with a hole in it, and these three close it.
//
// None of them touches money, so none of them needs an idempotency key — the
// guid ledger in fulfilAdvOrder is the whole guard, and a replay it somehow got
// past would be refused honestly ("nothing to abandon") rather than charging
// anybody twice.

// applyWebAbandon closes the owner's expedition down for good.
func (p *AdventurePlugin) applyWebAbandon(owner id.UserID) (status, detail string, retry bool) {
	// performExpeditionAbandon does NOT take the per-user lock (its Matrix caller
	// holds it across the whole `!expedition` switch), so this has to. See the
	// note on applyAdvOrder.
	userMu := p.advUserLock(owner)
	userMu.Lock()
	defer userMu.Unlock()

	out, err := p.performExpeditionAbandon(owner)
	if err != nil {
		var refusal advRefusal
		if !errors.As(err, &refusal) {
			// A DB fault reading or writing expedition state. Nothing partial is
			// left behind that a retry would double up, so leave it pending.
			slog.Warn("orders: abandon failed", "user", owner, "err", err)
			return "", "", true
		}
		status := "rejected_unavailable"
		switch {
		case errors.Is(err, errAbandonNothing):
			status = "rejected_not_running"
		case errors.Is(err, errAbandonNotLeader):
			status = "rejected_not_leader"
		}
		return status, advOrderPlainText(err.Error()), false
	}
	// The extracted case keeps loot and XP, so saying "supplies are forfeit" there
	// would be a straight lie. Same split the DM makes.
	if out.Extracted {
		return "applied", fmt.Sprintf(
			"You let %s go on day %d. Loot, XP and coins are kept.", out.Zone.Display, out.Day), false
	}
	return "applied", fmt.Sprintf(
		"Expedition in %s abandoned on day %d. Supplies are forfeit.", out.Zone.Display, out.Day), false
}

// applyWebLeave walks a party member out of somebody else's expedition.
func (p *AdventurePlugin) applyWebLeave(owner id.UserID) (status, detail string, retry bool) {
	// Same lock asymmetry as applyWebAbandon.
	userMu := p.advUserLock(owner)
	userMu.Lock()
	defer userMu.Unlock()

	if err := p.performExpeditionLeave(owner); err != nil {
		var refusal advRefusal
		if !errors.As(err, &refusal) {
			slog.Warn("orders: leave failed", "user", owner, "err", err)
			return "", "", true
		}
		status := "rejected_unavailable"
		switch {
		case errors.Is(err, errLeaveNothing):
			status = "rejected_not_running"
		case errors.Is(err, errLeaveIsLeader):
			status = "rejected_is_leader"
		}
		return status, advOrderPlainText(err.Error()), false
	}
	return "applied", "You turn back for town. Your supplies stay with the party.", false
}

// applyWebBabysitCancel dismisses the pet sitter early.
func (p *AdventurePlugin) applyWebBabysitCancel(owner id.UserID) (status, detail string, retry bool) {
	// No lock here, and that is not an oversight: performBabysitCancel takes it
	// itself, because ITS Matrix caller does not. The opposite of the two above.
	out, err := p.performBabysitCancel(owner)
	if err != nil {
		status := "rejected_unavailable"
		switch {
		case errors.Is(err, errBabysitNoSitter):
			status = "rejected_nothing_to_cancel"
		}
		return status, advOrderPlainText(err.Error()), false
	}
	// The DM prints the sitter's whole record of the stay; a verdict is one line
	// under a button, so the web gets the fact and the page keeps its shape.
	note := "Sitter dismissed. No refund — they were already here."
	if out.PetName != "" {
		note = fmt.Sprintf("Sitter dismissed. No refund. %s is back in your care.", out.PetName)
	}
	return "applied", note, false
}

// advOrderPlainText strips the Matrix markdown out of a line reused as a web
// verdict. Pete renders the detail as text, so asterisks and backticks would show
// up literally. The command hints inside those backticks stay — a verdict that
// says to type `!expedition abandon` is telling the truth about where the other
// door is, and the web has no button for it yet.
func advOrderPlainText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '*', '`':
			continue
		case '\n':
			out = append(out, ' ')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// ---- the applied-order ledger --------------------------------------------------

// advOrderAlreadyApplied reports the verdict we filed for an order, if we have
// already applied it. This is the short-circuit that keeps a re-offered order from
// re-running its non-idempotent mutation.
func advOrderAlreadyApplied(guid string) (status, detail string, ok bool) {
	err := db.Get().QueryRow(
		`SELECT status, detail FROM adv_applied_orders WHERE guid = ?`, guid,
	).Scan(&status, &detail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	if err != nil {
		// A read failure sends us down the mutation path and risks re-running an
		// extraction or a bout, so it is logged loudly. A transient one self-heals:
		// the mutation is guarded by this same table, so the next poll reads it.
		slog.Error("orders: applied-ledger read failed", "order", guid, "err", err)
		return "", "", false
	}
	return status, detail, true
}

// recordAdvApplied stamps an order as applied with the verdict we're about to
// file. OR IGNORE so a re-file that somehow reaches here can't error on the guid.
func recordAdvApplied(guid, status, detail string) error {
	_, err := db.Get().Exec(
		`INSERT OR IGNORE INTO adv_applied_orders (guid, status, detail) VALUES (?, ?, ?)`,
		guid, status, detail)
	return err
}
