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
	extractionSweepInterval  = time.Hour
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
	newSupplies, _ := applyExpeditionDailyBurn(e, harsh, e.SiegeMode)
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
	releaseParty(e.ID)
	return e, tax, nil
}

// The reasons a run can end badly. They were bare strings at four call sites;
// they are constants now because the news seam has to tell them apart — a
// retreat is a story and a death is a different story, and an idle reap is
// neither.
const (
	lossCombatDeath   = "combat death"
	lossCombatRetreat = "combat retreat" // solo: ran out the phase clock and withdrew
	lossCombatFlee    = "combat flee"    // party: the turn engine broke off
	lossIdleTimeout   = "run idle-timeout (§4.3 stale-run reap)"
)

// forceExtractExpeditionForRunLoss bridges run-loss call sites (turn-based
// elite/boss death or flee, exploration combat death, patrol-interrupt
// death) into the forced-extract flow. Those sites already abandon the
// zone run, but without flipping the wrapping expedition the ambient
// ticker keeps DMing about a dungeon the player walked away from. No-op
// when there is no active expedition for this user.
//
// It is also the one chokepoint every bad ending passes through, which makes it
// where the retreat dispatch is filed. Read the expedition BEFORE the extract:
// forcedExtractExpedition stamps it 'abandoned' and zeroes the live fields, so
// afterwards there is no day count left to report.
func forceExtractExpeditionForRunLoss(userID id.UserID, reason string) {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil {
		return
	}
	day, zoneID := exp.CurrentDay, exp.ZoneID
	if _, _, err := forcedExtractExpedition(exp.ID, reason); err != nil {
		slog.Warn("expedition: force-extract on run loss",
			"user", userID, "expedition", exp.ID, "reason", reason, "err", err)
		return
	}
	emitRetreatNews(userID, reason, zoneID, day)
}

// finalizeExpeditionOnZoneClear bridges the run-complete seam (boss down,
// no outgoing edges) into expedition completion — the success-path twin of
// forceExtractExpeditionForRunLoss. When the cleared run is the active
// expedition's current run AND the clear finishes the whole zone (a
// single-region zone, or the zone-boss region of a multi-region zone), it
// flips the expedition to 'complete', records boss_defeated, and awards
// completion milestones. Returns the rendered milestone lines for the caller
// to append to the run-complete message.
//
// No-op (nil) for standalone runs and for mid-zone region clears, which
// leave the expedition active so inter-region travel can continue. Without
// this, a cleared zone leaves the expedition 'active' forever and the
// ambient ticker keeps DMing about a dungeon the player already finished.
func (p *AdventurePlugin) finalizeExpeditionOnZoneClear(userID id.UserID, runID string) []string {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil {
		return nil
	}
	if exp.RunID != runID {
		return nil // the completed run isn't this expedition's current run
	}

	if IsMultiRegionZone(exp.ZoneID) {
		region, ok := CurrentRegion(exp)
		if !ok {
			return nil
		}
		if _, err := MarkRegionBossDefeated(exp, region.ID); err != nil {
			slog.Warn("expedition: mark region boss defeated",
				"user", userID, "expedition", exp.ID, "region", region.ID, "err", err)
		}
		if !region.IsZoneBoss {
			return nil // region cleared; expedition continues to the next region
		}
	} else {
		// Single-region zone: there's no region registry to flip through
		// MarkRegionBossDefeated, so set the zone-level flag directly.
		exp.BossDefeated = true
		if _, err := db.Get().Exec(`
			UPDATE dnd_expedition
			   SET boss_defeated = 1,
			       last_activity = CURRENT_TIMESTAMP
			 WHERE expedition_id = ?`, exp.ID); err != nil {
			slog.Warn("expedition: set boss defeated on zone clear",
				"user", userID, "expedition", exp.ID, "err", err)
		}
	}

	// completeExpedition must run before AwardCompletionMilestones — the
	// latter gates on status == 'complete'.
	if err := completeExpedition(exp.ID, ExpeditionStatusComplete); err != nil {
		slog.Warn("expedition: complete on zone clear",
			"user", userID, "expedition", exp.ID, "err", err)
		return nil
	}
	exp.Status = ExpeditionStatusComplete
	_ = retireAllRegionRuns(exp)
	p.rollZoneTreasure(userID, exp.ZoneID, advTreasureWeightZoneClear)
	lines := p.AwardCompletionMilestones(exp, false)
	// N6/D3: the Shadow's payoff for this zone — a crow (player was first) or a
	// waiting journal page (the Shadow cleared it first). Appended after the
	// milestone lines so the campaign beat reads last.
	if sl := p.shadowOnPlayerZoneClear(userID, exp.ZoneID); sl != "" {
		lines = append(lines, sl)
	}
	// News: boss down means the zone is cleared. Realm-first → PRIORITY,
	// repeat → BULLETIN. No-op unless the Pete seam is enabled.
	emitZoneClearNews(userID, exp)
	return lines
}

// midZoneRegionClear reports whether the just-completed run (runID) is the
// active expedition's current run AND finishes a non-boss region of a
// multi-region zone — i.e. a region clear that leaves the expedition active
// with a next region to cross into. Returns the cleared region, the next
// region, and true in that case; zero-values + false for a full zone clear,
// a standalone run, or any read error. Used to word the run-complete message
// (region-cleared vs zone-cleared) without re-deriving region state inline.
func midZoneRegionClear(userID id.UserID, runID string) (cur, next ExpeditionRegion, ok bool) {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil || exp.RunID != runID || !IsMultiRegionZone(exp.ZoneID) {
		return ExpeditionRegion{}, ExpeditionRegion{}, false
	}
	region, found := CurrentRegion(exp)
	if !found || region.IsZoneBoss {
		return ExpeditionRegion{}, ExpeditionRegion{}, false
	}
	nxt, found := nextRegion(exp.ZoneID, region.ID)
	if !found {
		return ExpeditionRegion{}, ExpeditionRegion{}, false
	}
	return region, nxt, true
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

// extractionLapsed reports whether an 'extracting' row is past its resume
// window and should be reaped. A NULL completed_at counts as lapsed: the column
// is the only clock the window has, and a row without one can never expire.
func extractionLapsed(e *Expedition, now time.Time) bool {
	return e.CompletedAt == nil || now.Sub(*e.CompletedAt) > extractResumeWindow
}

// loadLapsedExtractions returns every extracted expedition whose resume window
// has closed, regardless of owner.
func loadLapsedExtractions(now time.Time) ([]*Expedition, error) {
	rows, err := db.Get().Query(`
		SELECT`+expeditionSelectCols+`
		  FROM dnd_expedition e
		 WHERE e.status = 'extracting'
		   AND (e.completed_at IS NULL OR e.completed_at < ?)`,
		now.Add(-extractResumeWindow))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExpeditionRows(rows)
}

// sweepLapsedExtractions closes out every extraction whose seven-day window has
// run out, which releases its roster.
//
// Without this the window is a promise the code never keeps. `handleResumeCmd`
// expires a lapsed row lazily, but only the *leader* can call `!resume` — so a
// leader who quits, forgets, or simply starts a different expedition leaves the
// row 'extracting' forever, and releaseParty (which fires only on a terminal
// status) never runs. Every member stays seated, and assertNotAdventuring keeps
// refusing them a run of their own. `!expedition leave` is their escape, but a
// player should not have to find it.
func (p *AdventurePlugin) sweepLapsedExtractions(now time.Time) {
	exps, err := loadLapsedExtractions(now)
	if err != nil {
		slog.Error("expedition: load lapsed extractions", "err", err)
		return
	}
	for _, e := range exps {
		if err := p.reapLapsedExtraction(e); err != nil {
			slog.Warn("expedition: expire lapsed extraction", "expedition", e.ID, "err", err)
			continue
		}
	}
}

// reapLapsedExtraction closes one lapsed extraction and tells its roster the
// way back has shut. It is the single reap-with-notify path: the hourly
// sweeper loops it, and expeditionCmdStart calls it when a player starts a new
// expedition on top of one whose window has already closed — so a member is
// never silently unseated by whichever path happens to reach the row first.
//
// The audience is read before the close-out: completeExpedition disbands the
// roster, and expeditionAudience reads that roster.
func (p *AdventurePlugin) reapLapsedExtraction(e *Expedition) error {
	audience := expeditionAudience(e)
	if err := completeExpedition(e.ID, ExpeditionStatusFailed); err != nil {
		return err
	}
	zone, _ := getZone(e.ZoneID)
	body := fmt.Sprintf(
		"🕯 **The way back has closed — %s**\n\nYour extracted expedition sat past its seven-day resume window, and the dungeon reshaped without you. Day %d is where it ends.\n\nThe loot, XP, and coins you carried out are still yours. `!expedition list` when you're ready for the next one.",
		zone.Display, e.CurrentDay)
	for _, uid := range audience {
		if err := p.SendDM(uid, body); err != nil {
			slog.Warn("expedition: lapsed-extraction DM failed", "user", uid, "expedition", e.ID, "err", err)
		}
	}
	return nil
}

// expeditionExtractionSweepTicker reaps lapsed extractions hourly. The window is
// seven days, so the cadence only bounds how long a freed member waits to hear
// about it.
func (p *AdventurePlugin) expeditionExtractionSweepTicker() {
	ticker := time.NewTicker(extractionSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.sweepLapsedExtractions(time.Now().UTC())
		}
	}
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

// Sentinels for the two ways an extraction can be refused. They exist so the
// headless caller (the web action queue, pete_orders.go) can turn a refusal into
// its own verdict without parsing a DM. `!extract` maps them straight back to the
// prose it always sent.
var (
	errExtractNoRun     = errors.New("extract: no active expedition")
	errExtractNotLeader = errors.New("extract: not the party leader")
)

// extractOutcome is what an extraction did, for a caller that has to describe it
// somewhere other than a DM.
type extractOutcome struct {
	Zone string // display name
	Day  int
}

// performExtraction is the whole of `!extract` minus the command framing: the
// per-user lock, the leader check, the state flip, the log line, the party
// fan-out and the emergence pet roll. It is shared with the web action queue so
// that pulling out from a phone is the *same* extraction, not a second
// implementation of one — the party still gets DM'd, the log still gets its
// line, and the resume window is the same window.
func (p *AdventurePlugin) performExtraction(uid id.UserID) (extractOutcome, error) {
	userMu := p.advUserLock(uid)
	userMu.Lock()
	defer userMu.Unlock()

	exp, isLeader, err := activeExpeditionFor(uid)
	if err != nil {
		return extractOutcome{}, fmt.Errorf("reading expedition state: %w", err)
	}
	if exp == nil {
		return extractOutcome{}, errExtractNoRun
	}
	if !isLeader {
		// Extraction ends the expedition for the whole roster, so it is the
		// leader's call — the same reasoning that makes `!flee` leader-only.
		return extractOutcome{}, errExtractNotLeader
	}
	zone, _ := getZone(exp.ZoneID)
	updated, err := voluntaryExtractExpedition(uid)
	if err != nil {
		return extractOutcome{}, err
	}
	line := flavor.Pick(flavor.ExtractionVoluntary)
	_ = appendExpeditionLog(updated.ID, updated.CurrentDay, "narrative",
		"voluntary extraction", line)
	markActedToday(uid)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🚪 **Extraction — %s, Day %d**\n\n",
		zone.Display, updated.CurrentDay))
	if line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
	}

	// The extraction ends the day for everyone standing in the dungeon, and the
	// roster outlives a voluntary extract — `extracting` is a resumable limbo, so
	// members are still seated and must hear about it. Only the leader can call
	// `!resume`, so the closing line differs per reader.
	expires := (time.Now().UTC().Add(extractResumeWindow)).Format("2006-01-02 15:04 MST")
	p.fanOutExpeditionDM(updated, b.String(), func(uid id.UserID, body string) string {
		if uid == id.UserID(updated.UserID) {
			return body + fmt.Sprintf("Loot, XP, and coins are kept. The dungeon stays where you left it — `!resume` within 7 days to come back. After %s the expedition expires.", expires)
		}
		return body + fmt.Sprintf("Loot, XP, and coins are kept. Your leader can `!resume` within 7 days, and you'll walk back in with them. After %s the expedition expires.", expires)
	})

	// Emergence seam: surfacing from a run is when an animal may have moved
	// into the empty house. Every member surfaced, so every member rolls.
	for _, member := range expeditionAudience(updated) {
		p.maybeRollPetArrivalOnEmerge(member)
	}
	return extractOutcome{Zone: zone.Display, Day: updated.CurrentDay}, nil
}

// handleExtractCmd is `!extract`: the command framing around performExtraction.
// The extraction itself, including the DM everyone in the party gets, happens in
// there — so this only has to turn a refusal back into the prose it always sent.
func (p *AdventurePlugin) handleExtractCmd(ctx MessageContext, _ string) error {
	_, err := p.performExtraction(ctx.Sender)
	switch {
	case errors.Is(err, errExtractNoRun):
		return p.SendDM(ctx.Sender, "No active expedition to extract from.")
	case errors.Is(err, errExtractNotLeader):
		return p.SendDM(ctx.Sender, "Only your party leader can call the extraction. Ask them to `!extract`, or `!expedition leave` to walk out alone.")
	case err != nil:
		return p.SendDM(ctx.Sender, "Couldn't extract: "+err.Error())
	}
	return nil
}

// ── !resume command ─────────────────────────────────────────────────────────

// Sentinels for the ways going back in can be refused. Same contract as
// performExpeditionStart's: each one comes back inside an advRefusal that also
// carries the finished sentence, so `!resume` keeps its copy verbatim.
//
// errResumeNeedLoadout is the odd one out and deliberately so: it is not a
// refusal at all but the loadout prompt, returned as one so the whole decision
// stays inside the lock. The web never triggers it — it always names a loadout.
var (
	errResumeNeedLoadout = errors.New("resume: no loadout named")
	errResumeBusy        = errors.New("resume: already on an expedition")
	errResumeNothing     = errors.New("resume: no extracted expedition")
	errResumeLapsed      = errors.New("resume: past the 7-day window")
	errResumeBadPacks    = errors.New("resume: invalid pack selection")
	errResumeBroke       = errors.New("resume: cannot cover outfitting")
	errResumeFailed      = errors.New("resume: could not resume")
)

// resumeOutcome is what going back in did, for a caller describing it somewhere
// other than a DM.
type resumeOutcome struct {
	Zone     ZoneDefinition
	Day      int
	Supplies ExpeditionSupplies
	Purchase SupplyPurchase
	Threat   int
	Stack    int
	Line     string
}

// performResume is `!resume` minus the command framing: the leader check, the
// lapse check, the re-outfitting purchase and the fresh region run. Shared with
// the web action queue so that walking back in from a phone is the same walk.
//
// loadoutTok is the raw `Ns Md` / preset token; empty asks for the prompt.
// idemKey, when set, is the web order's guid and moves the money onto the
// idempotent variants — see beginExpeditionIdem for why a refund then makes a
// retry unsafe, which is why every error here is permanent for the web caller.
func (p *AdventurePlugin) performResume(uid id.UserID, loadoutTok, idemKey string) (resumeOutcome, error) {
	userMu := p.advUserLock(uid)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(uid)
	if err != nil {
		return resumeOutcome{}, fmt.Errorf("Couldn't load your character: %s", err)
	}
	if c == nil || c.PendingSetup {
		return resumeOutcome{}, refuseAdv(errResumeNothing, "No Adv 2.0 character yet — run `!setup` first.")
	}

	if existing, isLeader, _ := activeExpeditionFor(uid); existing != nil {
		zone, _ := getZone(existing.ZoneID)
		if !isLeader {
			return resumeOutcome{}, refuseAdv(errResumeBusy,
				"You're riding a party expedition in **%s** (Day %d). Only its leader can `!resume`.",
				zone.Display, existing.CurrentDay)
		}
		// A web order that already paid and already resumed lands here on the
		// re-offer; the settled debit is what tells that apart from a player who
		// really is already out. Same tell as performExpeditionStart's.
		if idemKey != "" && p.euro != nil && p.euro.HasExternalTx(idemKey) {
			return resumeOutcome{Zone: zone, Day: existing.CurrentDay,
				Supplies: existing.Supplies, Threat: existing.ThreatLevel}, nil
		}
		return resumeOutcome{}, refuseAdv(errResumeBusy,
			"You already have an active expedition in **%s** (Day %d). Finish it or `!expedition abandon` first.",
			zone.Display, existing.CurrentDay)
	}

	exp, err := getResumableExpedition(uid)
	if err != nil {
		return resumeOutcome{}, fmt.Errorf("Couldn't read expedition state: %s", err)
	}
	if exp == nil {
		return resumeOutcome{}, refuseAdv(errResumeNothing,
			"No extracted expedition to resume. Use `!expedition start <zone>` to begin a new one.")
	}
	if extractionLapsed(exp, time.Now().UTC()) {
		// Expire it so it doesn't keep resurfacing. The hourly sweeper would get
		// here on its own; this keeps the refusal and the reap in one breath.
		_ = completeExpedition(exp.ID, ExpeditionStatusFailed)
		return resumeOutcome{}, refuseAdv(errResumeLapsed,
			"That extraction is past its 7-day resume window — the dungeon has reshaped without you. Start a new expedition.")
	}

	zone, _ := getZone(exp.ZoneID)
	// D5-b: prompt for a preset loadout on empty args.
	if strings.TrimSpace(loadoutTok) == "" {
		return resumeOutcome{}, refuseAdv(errResumeNeedLoadout, "%s", renderLoadoutPrompt(zone, "resume"))
	}
	purchase, err := resolveLoadoutOrParse(strings.TrimSpace(loadoutTok), zone.Tier)
	if err != nil {
		return resumeOutcome{}, refuseAdv(errResumeBadPacks, "Couldn't parse supply packs: %s", err.Error())
	}
	if err := purchase.Validate(zone.Tier); err != nil {
		return resumeOutcome{}, refuseAdv(errResumeBadPacks, "Invalid pack selection: %s", err.Error())
	}
	cost := float64(purchase.Cost())
	if p.euro == nil {
		return resumeOutcome{}, refuseAdv(errResumeFailed, "Coin system unavailable — try again later.")
	}
	paid := idemKey != "" && p.euro.HasExternalTx(idemKey)
	if !paid {
		if balance := p.euro.GetBalance(uid); balance < cost {
			return resumeOutcome{}, refuseAdv(errResumeBroke,
				"Not enough coins. Outfitting costs **%d** but you have **%.0f**.",
				int(cost), balance)
		}
	}
	debit := func(why string) bool { return p.euro.Debit(uid, cost, why) }
	refund := func(why, suffix string) { p.euro.Credit(uid, cost, why) }
	if idemKey != "" {
		debit = func(why string) bool {
			ok, _, err := p.euro.DebitIdem(uid, cost, why, idemKey)
			return err == nil && ok
		}
		refund = func(why, suffix string) {
			if _, _, err := p.euro.CreditIdem(uid, cost, why, idemKey+":refund"+suffix); err != nil {
				slog.Error("expedition: resume refund failed", "user", uid, "order", idemKey, "err", err)
			}
		}
	}
	if !debit("expedition resume outfitting: " + string(exp.ZoneID)) {
		return resumeOutcome{}, refuseAdv(errResumeFailed, "Couldn't debit outfitting cost (try again).")
	}

	supplies := makeSupplies(zone.Tier, purchase)
	if err := resumeExpedition(exp.ID, supplies); err != nil {
		refund("expedition resume refund", "")
		return resumeOutcome{}, refuseAdv(errResumeFailed, "Couldn't resume: %s", err.Error())
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
		refund("expedition resume refund (run-spawn failed)", ":region")
		return resumeOutcome{}, refuseAdv(errResumeFailed, "Couldn't outfit the resumed region: %s", err.Error())
	}
	line := flavor.Pick(flavor.ExpeditionResume)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
		"expedition resumed", line)

	return resumeOutcome{
		Zone: zone, Day: exp.CurrentDay, Supplies: supplies, Purchase: purchase,
		Threat: exp.ThreatLevel, Stack: exp.TemporalStack, Line: line,
	}, nil
}

// handleResumeCmd is `!resume`: the command framing around performResume.
func (p *AdventurePlugin) handleResumeCmd(ctx MessageContext, args string) error {
	out, err := p.performResume(ctx.Sender, args, "")
	if err != nil {
		// Every refusal — and the loadout prompt, which travels as one — arrives
		// as a finished sentence, so this only has to say it.
		return p.SendDM(ctx.Sender, err.Error())
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🚪 **Expedition resumed — %s, Day %d**\n\n",
		out.Zone.Display, out.Day))
	if out.Line != "" {
		b.WriteString(out.Line)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("**Re-outfitted:** %.0f SU (%d standard, %d deluxe) — %d coins\n",
		out.Supplies.Max, out.Purchase.StandardPacks, out.Purchase.DeluxePacks, out.Purchase.Cost()))
	b.WriteString(fmt.Sprintf("**Threat:** %d / 100 (resumed at extraction value)\n", out.Threat))
	if out.Stack != 0 {
		b.WriteString(fmt.Sprintf("**Zone stack:** %d (resumed)\n", out.Stack))
	}
	b.WriteString("\nUse `!expedition status` for the daily briefing.")
	return p.SendDM(ctx.Sender, b.String())
}
