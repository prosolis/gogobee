package plugin

// The web action queue's game-side loop — the equip queue's sibling, and the
// first one where the web plays the game rather than dressing the character.
//
// An owner, signed in on Pete, asks to pull out of a run or to take today's swing
// at the Siege. Pete records the intent; we poll for it, run the real command
// path (the same one `!extract` and `!adventure worldboss fight` run — not a
// second implementation of it), and file a verdict Pete shows them.
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
// Note what is NOT here: a per-user lock. Both verbs take it inside their own
// shared helper (performExtraction, takeSiegeBout), which is what serialises them
// against the Matrix commands running the very same code. Taking it here too
// would deadlock on a non-reentrant mutex.
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

	default:
		// Pete validates the action before it ever queues an order, so this is a
		// contract breach, not a user mistake. Reject permanently rather than spin.
		return "rejected_unavailable", "Unknown action.", false
	}
}

// advOrderPlainText strips the Matrix markdown out of a line reused as a web
// verdict. Pete renders the detail as text, so asterisks would show up literally.
func advOrderPlainText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '*':
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
