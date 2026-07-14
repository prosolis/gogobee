package plugin

import (
	"context"
	"log/slog"
	"time"

	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// The casino's money loop.
//
// Pete runs the games and holds the chips; we hold the euros and are the only
// one who can move them. A player who buys in or cashes out on games.parodia.dev
// opens an escrow row over there, and this loop is what turns it into a real
// balance change over here.
//
// It has to be a poll because Pete cannot call us — there is no inbound API on
// this box and there isn't going to be one. That constraint is what shaped the
// whole design: money crosses the border twice per *session*, not twice per
// hand, so a few seconds of poll latency lands on "buying chips…" and never on
// "deal me in".
//
// Every step is idempotent on the escrow guid, because every step can be
// interrupted in the worst possible place. If we die after DebitIdem and before
// the verdict is queued, Pete re-offers the row, we claim it again, DebitIdem
// replays as a no-op and reports the same answer, and the verdict goes out. The
// player is charged once. That is the only property here that really matters.
const (
	// escrowPollInterval — a player is watching a spinner, so this is seconds,
	// not the 30s the mischief seam gets away with.
	escrowPollInterval = 3 * time.Second

	// escrowPollTimeout — one full poll-claim-settle pass. Generous: the
	// alternative to finishing is a claimed row nobody answers.
	escrowPollTimeout = 30 * time.Second

	reasonInsufficientFunds = "insufficient_funds"
)

// StartPeteEscrowLoop runs the border forever. Safe to call when the Pete seam
// is off — it simply never starts.
func StartPeteEscrowLoop(ctx context.Context, euro *EuroPlugin) {
	if !peteclient.Enabled() || euro == nil {
		return
	}
	slog.Info("pete games: escrow loop started", "interval", escrowPollInterval)
	go func() {
		t := time.NewTicker(escrowPollInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pollEscrowOnce(ctx, euro)
			}
		}
	}()
}

// escrowPollOK tracks the poll's health so we log transitions and not a line
// every three seconds. Pete being down is normal during its redeploys and is not
// worth shouting about; Pete being down for an hour is.
var escrowPollOK = true

func pollEscrowOnce(ctx context.Context, euro *EuroPlugin) {
	ctx, cancel := context.WithTimeout(ctx, escrowPollTimeout)
	defer cancel()

	pending, err := peteclient.PendingEscrow(ctx)
	if err != nil {
		if escrowPollOK {
			slog.Warn("pete games: escrow poll failed — buy-ins and cash-outs are stalled", "err", err)
			escrowPollOK = false
		}
		return
	}
	if !escrowPollOK {
		slog.Info("pete games: escrow poll recovered")
		escrowPollOK = true
	}
	if len(pending) == 0 {
		return
	}

	for _, e := range pending {
		if ctx.Err() != nil {
			return
		}
		settleEscrow(ctx, euro, e)
	}
	// The verdicts are on the durable queue now. Send them at once rather than
	// waiting up to a sender tick — there is a person at the other end of this.
	peteclient.Flush(ctx)
}

// settleEscrow moves the euros for one crossing and queues the answer.
func settleEscrow(ctx context.Context, euro *EuroPlugin, e peteclient.Escrow) {
	// Claim first. The claim is what fixes the amount and the player: the poll's
	// copy of the row is already a few milliseconds stale, and this is money.
	claimed, err := peteclient.ClaimEscrow(ctx, e.GUID)
	if err != nil {
		slog.Warn("pete games: claim failed, will retry", "guid", e.GUID, "err", err)
		return
	}
	// Pete has already decided this one — a stale re-offer that raced our own
	// earlier verdict. Nothing to do, and nothing went wrong.
	if claimed.State != "claimed" {
		slog.Debug("pete games: escrow already decided", "guid", e.GUID, "state", claimed.State)
		return
	}
	if claimed.Amount <= 0 {
		slog.Error("pete games: escrow with a non-positive amount, refusing",
			"guid", claimed.GUID, "amount", claimed.Amount)
		return
	}

	user := id.UserID(claimed.MatrixUser)
	amount := float64(claimed.Amount)

	var ok bool
	var balance float64
	switch claimed.Kind {
	case "buyin":
		ok, balance, err = euro.DebitIdem(user, amount, "games_buyin", claimed.GUID)
	case "cashout":
		ok, balance, err = euro.CreditIdem(user, amount, "games_cashout", claimed.GUID)
	default:
		// Not a transient failure and not something a retry fixes. Say no, so the
		// row reaches a terminal state on Pete instead of being re-offered forever.
		slog.Error("pete games: unknown escrow kind", "guid", claimed.GUID, "kind", claimed.Kind)
		peteclient.EmitEscrowVerdict(peteclient.EscrowVerdict{
			GUID: claimed.GUID, OK: false, Reason: "unknown_kind",
		})
		return
	}

	if err != nil {
		// The money did not move and we don't know that it didn't — so say
		// nothing. Pete re-offers the row once the claim goes stale, and the guid
		// makes the retry safe whichever way this actually landed.
		slog.Error("pete games: euro move failed, leaving the row for a retry",
			"guid", claimed.GUID, "kind", claimed.Kind, "err", err)
		return
	}

	reason := ""
	if !ok {
		// The only way DebitIdem says no: the player cannot cover it. A cash-out
		// has no floor to breach, so this is always a buy-in.
		reason = reasonInsufficientFunds
	}

	peteclient.EmitEscrowVerdict(peteclient.EscrowVerdict{
		GUID:         claimed.GUID,
		OK:           ok,
		Reason:       reason,
		BalanceAfter: balance,
	})
	slog.Info("pete games: escrow moved", "guid", claimed.GUID, "user", claimed.MatrixUser,
		"kind", claimed.Kind, "amount", claimed.Amount, "ok", ok, "balance_after", balance)
}
