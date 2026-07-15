package plugin

// The mischief storefront's game-side loop.
//
// A buyer places a hit on Pete's web board; Pete records the intent and waits.
// We poll for those orders, do the real work against our own ledger — the money,
// the eligibility, the contract — and hand back a verdict Pete files against the
// order. Pete has no route into this box, which is why the traffic runs this way:
// we ask for work, we don't get told about it.
//
// The order guid is the idempotency key end to end (see placeWebMischief), so
// every step here is safe to repeat. That is the whole reason the loop can be
// this simple: a failed claim just leaves the order pending, and the next tick
// re-runs it as a no-op. The loop is its own retry, and needs no durable queue.

import (
	"context"
	"log/slog"
	"time"

	"gogobee/internal/peteclient"
)

const (
	mischiefPollInterval = 30 * time.Second
	mischiefPollTimeout  = 20 * time.Second
)

// peteMischiefTicker polls Pete for storefront orders and fulfils them. Started
// alongside the other adventure tickers; exits on stopCh.
func (p *AdventurePlugin) peteMischiefTicker() {
	if !peteclient.Enabled() {
		return // no Pete wire configured; the storefront half is simply off
	}
	ticker := time.NewTicker(mischiefPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.pollMischiefOrders()
		}
	}
}

// pollMischiefOrders fetches the pending orders and fulfils each in turn.
func (p *AdventurePlugin) pollMischiefOrders() {
	ctx, cancel := context.WithTimeout(context.Background(), mischiefPollTimeout)
	defer cancel()

	orders, err := peteclient.PendingMischief(ctx)
	if err != nil {
		// A Pete that predates the storefront answers 404 here; a wire blip is the
		// same. Either way there is nothing to do this tick, and the next one tries
		// again. Debug, not warn — this must be quiet when Pete simply hasn't
		// shipped the endpoint yet.
		slog.Debug("mischief: poll failed", "err", err)
		return
	}

	for _, order := range orders {
		p.fulfilMischiefOrder(ctx, order)
	}
}

// fulfilMischiefOrder places one order's contract and files the verdict. A
// transient failure (Retry) is left pending for the next poll; a real verdict is
// pushed back so the buyer sees why. A failed claim-push is not fatal — the order
// stays pending and the next poll re-runs it, which the guid makes a no-op.
func (p *AdventurePlugin) fulfilMischiefOrder(ctx context.Context, order peteclient.MischiefOrder) {
	res := p.placeWebMischief(order)
	if res.Retry {
		return // leave it pending; try again next tick
	}
	if err := peteclient.ClaimMischief(ctx, order.GUID, res.Status, res.Detail); err != nil {
		// The contract is placed (or the buyer refunded) on our side, but Pete
		// hasn't heard the verdict. It stays pending there and we'll re-offer it;
		// placeWebMischief will short-circuit on the stamped order guid and we'll
		// re-file. So this is a warn, not a lost order.
		slog.Warn("mischief: claim verdict push failed, will re-file next poll",
			"order", order.GUID, "status", res.Status, "err", err)
		return
	}
	slog.Info("mischief: web order fulfilled", "order", order.GUID, "status", res.Status)
}
