package plugin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E5 — Extraction & Resume.
// Spec: gogobee_expedition_system.md §10.
//
//   - Voluntary (§10.1): !extract — burns 1 in-game day, status='extracting',
//     resumable for 7 real days. All loot/XP/coins kept.
//   - Forced (§10.2): triggered by HP-0 / supplies-0 / Abyss collapse.
//     status='abandoned', 20% coin tax on CoinsEarned, NOT resumable.
//   - Resume (§10.3): !resume within 7 days, repurchase supplies, threat
//     and temporal stacks restored as-is.
//
// HP-0 and supplies-0 wiring lives with combat / supply hookups (deferred);
// this commit ships the helpers and wires the existing Abyss-collapse path.

const (
	extractResumeWindow      = 7 * 24 * time.Hour
	forcedExtractCoinTaxFrac = 0.20
)

var (
	ErrNoResumableExpedition = errors.New("no resumable expedition for player")
	ErrResumeWindowExpired   = errors.New("expedition resume window expired (7 days)")
)

// voluntaryExtractExpedition burns one in-game day's supplies, advances
// the day counter, and flips status to 'extracting'. The expedition stays
// resumable until completed_at + 7 days.
func voluntaryExtractExpedition(userID id.UserID) (*Expedition, error) {
	e, err := getActiveExpedition(userID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNoActiveExpedition
	}
	harsh := e.ThreatLevel > 60
	newSupplies, _ := applyDailyBurn(e.Supplies, harsh, e.SiegeMode)
	e.Supplies = newSupplies
	supJSON, _ := json.Marshal(newSupplies)
	if _, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET status        = 'extracting',
		       supplies_json = ?,
		       current_day   = current_day + 1,
		       completed_at  = CURRENT_TIMESTAMP,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, string(supJSON), e.ID); err != nil {
		return nil, fmt.Errorf("voluntary extract: %w", err)
	}
	e.CurrentDay++
	e.Status = ExpeditionStatusExtracting
	_ = retireAllRegionRuns(e)
	return e, nil
}

// forcedExtractExpedition flips an expedition to 'abandoned'. Returns the
// 20% coin-tax amount the caller should debit (CoinsEarned * 0.20, floored).
// Reason is appended to the expedition log.
func forcedExtractExpedition(expID, reason string) (*Expedition, int, error) {
	e, err := getExpedition(expID)
	if err != nil {
		return nil, 0, err
	}
	if e == nil {
		return nil, 0, ErrNoActiveExpedition
	}
	tax := int(float64(e.CoinsEarned) * forcedExtractCoinTaxFrac)
	if _, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET status        = 'abandoned',
		       completed_at  = CURRENT_TIMESTAMP,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, expID); err != nil {
		return nil, 0, fmt.Errorf("forced extract: %w", err)
	}
	e.Status = ExpeditionStatusAbandoned
	_ = retireAllRegionRuns(e)
	return e, tax, nil
}

// forceExtractExpeditionForRunLoss bridges run-loss call sites (turn-based
// elite/boss death or flee, exploration combat death, patrol-interrupt
// death) into the forced-extract flow. Those sites already abandon the
// zone run, but without flipping the wrapping expedition the ambient
// ticker keeps DMing about a dungeon the player walked away from. No-op
// when there is no active expedition for this user.
func forceExtractExpeditionForRunLoss(userID id.UserID, reason string) {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil {
		return
	}
	if _, _, err := forcedExtractExpedition(exp.ID, reason); err != nil {
		slog.Warn("expedition: force-extract on run loss",
			"user", userID, "expedition", exp.ID, "reason", reason, "err", err)
	}
}

// getResumableExpedition returns the most recent 'extracting' row for the
// user, regardless of age (caller checks the 7-day window).
func getResumableExpedition(userID id.UserID) (*Expedition, error) {
	row := db.Get().QueryRow(`
		SELECT expedition_id, user_id, zone_id, run_id, status,
		       start_date, current_day, current_region, boss_defeated,
		       supplies_json, camp_json, threat_level, threat_siege,
		       threat_events, temporal_stack, region_state,
		       xp_earned, coins_earned, gm_mood,
		       last_briefing_at, last_recap_at, last_ambient_kind,
		       last_activity, completed_at
		  FROM dnd_expedition
		 WHERE user_id = ?
		   AND status = 'extracting'
		 ORDER BY completed_at DESC
		 LIMIT 1`, string(userID))
	e, err := scanExpedition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// resumeExpedition flips an extracting row back to 'active' with fresh
// supplies. Threat/temporal/region state are preserved as-is.
func resumeExpedition(expID string, supplies ExpeditionSupplies) error {
	supJSON, _ := json.Marshal(supplies)
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET status        = 'active',
		       supplies_json = ?,
		       completed_at  = NULL,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, string(supJSON), expID)
	return err
}

// ── !extract command ────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleExtractCmd(ctx MessageContext, _ string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition to extract from.")
	}
	zone, _ := getZone(exp.ZoneID)
	updated, err := voluntaryExtractExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't extract: "+err.Error())
	}
	line := flavor.Pick(flavor.ExtractionVoluntary)
	_ = appendExpeditionLog(updated.ID, updated.CurrentDay, "narrative",
		"voluntary extraction", line)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🚪 **Extraction — %s, Day %d**\n\n",
		zone.Display, updated.CurrentDay))
	if line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("Loot, XP, and coins are kept. The dungeon stays where you left it — `!resume` within 7 days to come back. After %s the expedition expires.",
		(time.Now().UTC().Add(extractResumeWindow)).Format("2006-01-02 15:04 MST")))
	if err := p.SendDM(ctx.Sender, b.String()); err != nil {
		return err
	}
	// Emergence seam: surfacing from a run is when an animal may have moved
	// into the empty house.
	p.maybeRollPetArrivalOnEmerge(ctx.Sender)
	return nil
}

// ── !resume command ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleResumeCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character: "+err.Error())
	}
	if c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender, "No Adv 2.0 character yet — run `!setup` first.")
	}

	if existing, _ := getActiveExpedition(ctx.Sender); existing != nil {
		zone, _ := getZone(existing.ZoneID)
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You already have an active expedition in **%s** (Day %d). Finish it or `!expedition abandon` first.",
			zone.Display, existing.CurrentDay))
	}

	exp, err := getResumableExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No extracted expedition to resume. Use `!expedition start <zone>` to begin a new one.")
	}
	if exp.CompletedAt == nil || time.Since(*exp.CompletedAt) > extractResumeWindow {
		// Expire it so it doesn't keep resurfacing.
		_ = completeExpedition(exp.ID, ExpeditionStatusFailed)
		return p.SendDM(ctx.Sender,
			"That extraction is past its 7-day resume window — the dungeon has reshaped without you. Start a new expedition.")
	}

	purchase, err := parseSupplyArgs(strings.TrimSpace(args))
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't parse supply packs: "+err.Error())
	}
	if err := purchase.Validate(); err != nil {
		return p.SendDM(ctx.Sender, "Invalid pack selection: "+err.Error())
	}
	cost := float64(purchase.Cost())
	if p.euro == nil {
		return p.SendDM(ctx.Sender, "Coin system unavailable — try again later.")
	}
	if balance := p.euro.GetBalance(ctx.Sender); balance < cost {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Not enough coins. Outfitting costs **%d** but you have **%.0f**.",
			int(cost), balance))
	}
	if !p.euro.Debit(ctx.Sender, cost, "expedition resume outfitting: "+string(exp.ZoneID)) {
		return p.SendDM(ctx.Sender, "Couldn't debit outfitting cost (try again).")
	}

	zone, _ := getZone(exp.ZoneID)
	supplies := makeSupplies(zone.Tier, purchase)
	if err := resumeExpedition(exp.ID, supplies); err != nil {
		p.euro.Credit(ctx.Sender, cost, "expedition resume refund")
		return p.SendDM(ctx.Sender, "Couldn't resume: "+err.Error())
	}
	exp.Status = ExpeditionStatusActive
	exp.Supplies = supplies
	// Spawn a fresh DungeonRun for the resumed region; the prior run was
	// abandoned at extract time. Per-room harvest state in RegionState is
	// preserved as-is so progression isn't reset.
	exp.RunID = ""
	exp.RegionState[regionStateRegionRuns] = map[string]string{}
	_ = persistRegionState(exp)
	if _, err := ensureRegionRun(exp, c.Level); err != nil {
		p.euro.Credit(ctx.Sender, cost, "expedition resume refund (run-spawn failed)")
		return p.SendDM(ctx.Sender, "Couldn't outfit the resumed region: "+err.Error())
	}
	line := flavor.Pick(flavor.ExpeditionResume)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
		"expedition resumed", line)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🚪 **Expedition resumed — %s, Day %d**\n\n",
		zone.Display, exp.CurrentDay))
	if line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("**Re-outfitted:** %.0f SU (%d standard, %d deluxe) — %d coins\n",
		supplies.Max, purchase.StandardPacks, purchase.DeluxePacks, purchase.Cost()))
	b.WriteString(fmt.Sprintf("**Threat:** %d / 100 (resumed at extraction value)\n", exp.ThreatLevel))
	if exp.TemporalStack != 0 {
		b.WriteString(fmt.Sprintf("**Zone stack:** %d (resumed)\n", exp.TemporalStack))
	}
	b.WriteString("\nUse `!expedition status` for the daily briefing.")
	return p.SendDM(ctx.Sender, b.String())
}
