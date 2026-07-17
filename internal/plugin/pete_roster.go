package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// The live adventurer board pushed to Pete (gogobee_boredom_plan.md's sibling —
// see the roster section of the Pete plan).
//
// Everything else we send Pete is an accomplishment: a death, a clear, a
// milestone. Those are clippings — they read as archive the moment they land, no
// matter how fast we deliver them. The board is the other kind of thing: state
// that is *currently true*, which is the only thing that can make a page feel
// alive. So it is a snapshot, pushed whole, replacing whatever Pete had.
//
// It is also, by design, a target list. The plan is to let people who aren't
// even playing hire assassins and mobs against adventurers who are out in the
// world right now — so the board carries a stable per-player token and real zone
// depth, not just a pretty display string, and it shows the zone *live* while
// they're still in it.
const (
	// rosterTickInterval — how often we push. Pete's staleness window is several
	// times this, so a missed push or two is invisible; a real outage isn't.
	rosterTickInterval = 2 * time.Minute

	// rosterPushTimeout — the push is dropped on failure, never retried (a stale
	// snapshot is a lie, and the next tick carries the truth), so it must not be
	// able to pile up.
	rosterPushTimeout = 15 * time.Second
)

// peteRosterTicker pushes the board to Pete forever.
func (p *AdventurePlugin) peteRosterTicker() {
	if !peteclient.Enabled() {
		return
	}
	ticker := time.NewTicker(rosterTickInterval)
	defer ticker.Stop()
	for range ticker.C {
		if !newsEmissionOn() {
			continue // master switch off: the board goes stale on Pete and says so
		}
		p.pushRoster()
		p.pushDetails()
	}
}

// rosterPushOK tracks the last push's outcome so we can log the transitions and
// nothing else. A push every 2 minutes forever is far too noisy to log at INFO,
// but total silence is worse: a ticker that is succeeding quietly looks exactly
// like one that never started, and that ambiguity already cost an operator a
// wrong-turn debug during the first deploy. So: say something the first time it
// works, say something when it breaks, say something when it recovers.
var rosterPushOK bool

func (p *AdventurePlugin) pushRoster() {
	snap, err := buildRosterSnapshot(time.Now().UTC(), p.euro)
	if err != nil {
		slog.Error("roster: build snapshot failed", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rosterPushTimeout)
	defer cancel()

	if err := peteclient.PushRoster(ctx, snap); err != nil {
		// The failure itself is not alarming — the next tick retries by simply
		// being a fresher snapshot, and if we stay down Pete's board correctly
		// stops claiming to be live. Only the *transition* is worth a line.
		if rosterPushOK {
			slog.Warn("roster: push failed, board will go stale on Pete", "err", err)
		} else {
			slog.Debug("roster: push failed, dropping snapshot", "err", err)
		}
		rosterPushOK = false
		return
	}

	if !rosterPushOK {
		slog.Info("roster: board accepted by Pete — live adventurer board is publishing",
			"adventurers", len(snap.Adventurers))
		rosterPushOK = true
	}
}

// detailPushOK mirrors rosterPushOK for the private self-detail push: log the
// transitions and nothing else.
var detailPushOK bool

func (p *AdventurePlugin) pushDetails() {
	snap, err := buildDetailSnapshot(time.Now().UTC())
	if err != nil {
		slog.Error("roster: build detail snapshot failed", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rosterPushTimeout)
	defer cancel()

	if err := peteclient.PushDetails(ctx, snap); err != nil {
		if detailPushOK {
			slog.Warn("roster: detail push failed, self-view will go stale on Pete", "err", err)
		} else {
			slog.Debug("roster: detail push failed, dropping snapshot", "err", err)
		}
		detailPushOK = false
		return
	}

	if !detailPushOK {
		slog.Info("roster: private self-detail accepted by Pete", "players", len(snap.Players))
		detailPushOK = true
	}
}

// rosterDetail assembles the public detail sheet for one adventurer: the combat
// stats every visitor may see, plus equipped gear. Expedition context (supplies,
// threat, room) is layered on by the caller when a run is active.
func rosterDetail(uid id.UserID, c *DnDCharacter) *peteclient.RosterDetail {
	d := &peteclient.RosterDetail{
		HPCurrent:  c.HPCurrent,
		HPMax:      c.HPMax,
		TempHP:     c.TempHP,
		ArmorClass: c.ArmorClass,
		Abilities:  [6]int{c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA},
		Modifiers:  c.Modifiers(),
	}
	if equip, err := loadAdvEquipment(uid); err == nil {
		for _, slot := range allSlots {
			e, ok := equip[slot]
			if !ok || e == nil {
				continue
			}
			d.Gear = append(d.Gear, peteclient.GearItem{
				Slot:       string(slot),
				Name:       e.Name,
				Tier:       e.Tier,
				Condition:  e.Condition,
				Masterwork: e.Masterwork,
			})
		}
	}
	return d
}

// buildDetailSnapshot assembles the private, owner-only detail set — inventory,
// vault, house, and pets for every alive player, keyed by localpart. It does NOT
// skip opted-out players: the news opt-out governs the *public* board, but this
// data is never shown there — Pete only ever serves it back to the one signed-in
// owner it belongs to, so hiding a player from it would only deny them their own
// sheet. The board token rides along so Pete can match owner↔page without ever
// reversing the one-way token.
func buildDetailSnapshot(now time.Time) (peteclient.DetailSnapshot, error) {
	snap := peteclient.DetailSnapshot{SnapshotAt: now.Unix()}
	rows, err := db.Get().Query(`SELECT user_id FROM player_meta WHERE alive = 1`)
	if err != nil {
		return snap, err
	}
	var uids []id.UserID
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return snap, err
		}
		uids = append(uids, id.UserID(uid))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return snap, err
	}

	for _, uid := range uids {
		lp := localpartOf(uid)
		if lp == "" {
			continue
		}
		adv, err := loadAdvCharacter(uid)
		if err != nil || adv == nil {
			continue
		}
		pd := peteclient.PlayerDetail{
			Localpart: lp,
			Token:     eventToken(uid, "roster"),
			House: peteclient.HouseView{
				Tier:        adv.HouseTier,
				LoanBalance: adv.HouseLoanBalance,
				Autopay:     adv.HouseAutopay,
				Rate:        adv.HouseCurrentRate,
			},
			Pets: petViews(adv),
		}
		if items, err := loadAdvInventory(uid); err == nil {
			pd.Inventory = itemViews(items)
		}
		if items, err := loadAdvVault(uid); err == nil {
			pd.Vault = itemViews(items)
		}
		pd.Equipped = equippedViews(uid)
		snap.Players = append(snap.Players, pd)
	}
	return snap, nil
}

// itemViews renders inventory or vault rows for the private panel, resolving
// the display facts the row itself doesn't carry.
//
// Attuned is always false here and that is not an omission: equipping *moves*
// the row out of adventure_inventory into magic_item_equipped, so nothing in a
// backpack can hold a bond. Worn items come from equippedViews instead.
func itemViews(items []AdvItem) []peteclient.ItemView {
	if len(items) == 0 {
		return nil
	}
	out := make([]peteclient.ItemView, 0, len(items))
	for _, it := range items {
		v := peteclient.ItemView{
			Name:   it.Name,
			Type:   it.Type,
			Tier:   it.Tier,
			Value:  it.Value,
			Temper: it.Temper,
			Slot:   string(it.Slot),
		}
		// SkillSource is dual-use: a real skill name on masterwork gear, an
		// internal registry pointer on magic-item rows. Only the former is a
		// fact about the item; the latter is plumbing and stays home.
		if !strings.HasPrefix(it.SkillSource, "magic_item:") {
			v.SkillSource = it.SkillSource
		}
		if mi, ok := magicItemFromAdvItem(it); ok {
			eff := temperedItem(mi, it.Temper)
			v.Slot = string(eff.Slot)
			v.Desc = eff.Desc
			v.Effect = magicItemEffectSummary(eff)
			v.Attunement = eff.Attunement
			// The row id is the equip handle: a slotted magic item is the one thing
			// the web equip path can wear, so only it carries an id. Mundane gear (the
			// branch below) and unslotted curios get none, so no Equip button. This
			// runs for vault rows too — a vault magic item would carry an id — but Pete
			// offers the button on the backpack panel alone, so that's inert, not a leak.
			if eff.Slot != "" {
				v.ID = it.ID
			}
		} else if it.Slot != "" {
			// Shop equipment resolves by (slot, tier) — Name is decorative.
			v.Desc = equipmentDefByTier(it.Slot, it.Tier).Description
		}
		out = append(out, v)
	}
	return out
}

// equippedViews returns the magic items the player is actually wearing. This is
// the only place Attuned means anything: the bond lives on the equipped row, and
// with a cap of dndMagicItemAttuneLimit a worn item can be inert.
func equippedViews(uid id.UserID) []peteclient.ItemView {
	equipped, err := loadEquippedMagicItems(uid)
	if err != nil || len(equipped) == 0 {
		return nil
	}
	out := make([]peteclient.ItemView, 0, len(equipped))
	for _, e := range equipped {
		eff := e.Effective()
		out = append(out, peteclient.ItemView{
			Name:       eff.Name,
			Type:       string(eff.Kind),
			Value:      int64(eff.Value),
			Temper:     e.Temper,
			Slot:       string(e.Slot),
			Desc:       eff.Desc,
			Effect:     magicItemEffectSummary(eff),
			Attunement: eff.Attunement,
			Attuned:    e.Attuned,
		})
	}
	// Map iteration is random; the panel must not reshuffle every 60s poll.
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// petViews returns the player's live pet slots. A pet that was chased away is
// omitted — it isn't with them right now, and the self-view shows the present.
func petViews(adv *AdventureCharacter) []peteclient.PetView {
	var out []peteclient.PetView
	if adv.PetType != "" && !adv.PetChasedAway {
		out = append(out, peteclient.PetView{
			Type: adv.PetType, Name: adv.PetName, Level: adv.PetLevel,
			XP: adv.PetXP, ArmorTier: adv.PetArmorTier,
		})
	}
	if adv.Pet2Type != "" && !adv.Pet2ChasedAway {
		out = append(out, peteclient.PetView{
			Type: adv.Pet2Type, Name: adv.Pet2Name, Level: adv.Pet2Level,
			XP: adv.Pet2XP, ArmorTier: adv.Pet2ArmorTier,
		})
	}
	return out
}

// buildRosterSnapshot assembles the complete board.
//
// Complete is the contract: Pete *replaces* its board with this, so anyone we
// omit drops off the public page. That is exactly how the opt-out is enforced —
// an opted-out player is simply never in the payload, rather than being sent and
// anonymized. A standing row showing class + level + zone is trivially
// re-identifiable (there is one level-14 cleric), so "an adventurer" would have
// been a fig leaf; absence is the only honest option.
func buildRosterSnapshot(now time.Time, euro *EuroPlugin) (peteclient.RosterSnapshot, error) {
	snap := peteclient.RosterSnapshot{SnapshotAt: now.Unix(), Tiers: mischiefTierCatalog()}

	// Both DATETIME columns selected raw and folded in Go — NOT COALESCE()'d in
	// SQL. modernc.org/sqlite rebuilds a time.Time from the column's *declared*
	// type, and COALESCE() erases that affinity: the value comes back a string
	// and the Scan fails. Same trap playerIsIdle documents.
	rows, err := db.Get().Query(`
		SELECT user_id, last_player_action_at, created_at
		  FROM player_meta
		 WHERE alive = 1`)
	if err != nil {
		return snap, err
	}
	defer rows.Close()

	type player struct {
		uid        id.UserID
		lastAction *time.Time
	}
	var players []player
	for rows.Next() {
		var uid string
		var lastAction, created *time.Time
		if err := rows.Scan(&uid, &lastAction, &created); err != nil {
			return snap, err
		}
		if lastAction == nil {
			lastAction = created
		}
		players = append(players, player{id.UserID(uid), lastAction})
	}
	if err := rows.Err(); err != nil {
		return snap, err
	}

	for _, pl := range players {
		// The buyer's own advisory balance rides along in a separate keyspace,
		// keyed by localpart (their sign-in name), and is collected *before* the
		// opt-out skip: opt-out hides a player from the public board, but their own
		// balance is private on Pete — only ever read for the user asking about
		// themselves — so there is no reason to deny an opted-out player the
		// storefront's affordability hint. localpart is lowercase, matching how the
		// buyer signs in and how a web order's username is resolved back to an MXID.
		if euro != nil {
			if lp := localpartOf(pl.uid); lp != "" {
				snap.Balances = append(snap.Balances, peteclient.MischiefBalance{
					Username: lp,
					Euro:     euro.GetBalance(pl.uid),
				})
			}
		}

		if isNewsOptedOut(pl.uid) {
			continue
		}
		c, err := LoadDnDCharacter(pl.uid)
		if err != nil || c == nil || c.PendingSetup {
			continue // no character to show; a half-made one has no name yet
		}
		name := charName(pl.uid)
		if name == "" {
			continue // never fall back to a Matrix handle on a public page
		}

		e := peteclient.RosterEntry{
			// Stable per-player board token: salted, so it can't be recomputed
			// from the handle, and distinct from every event token, so the board
			// doesn't become the key that links a player's dispatches together.
			Token:     eventToken(pl.uid, "roster"),
			Name:      name,
			Level:     c.Level,
			ClassRace: classRaceLabel(c),
			Status:    "idle",
		}

		e.Detail = rosterDetail(pl.uid, c)

		if exp, _ := getActiveExpedition(pl.uid); exp != nil {
			zone := zoneOrFallback(exp.ZoneID)
			e.Status = "expedition"
			e.Zone = zone.Display
			e.Day = exp.CurrentDay
			if IsMultiRegionZone(exp.ZoneID) {
				if r, ok := CurrentRegion(exp); ok {
					e.Region = r.Name
				}
			}
			if e.Detail != nil {
				e.Detail.Supplies = int(exp.Supplies.Current)
				e.Detail.ThreatLevel = exp.ThreatLevel
				if exp.RunID != "" {
					if run, rerr := getZoneRun(exp.RunID); rerr == nil && run != nil && run.TotalRooms > 0 {
						e.Detail.Room = fmt.Sprintf("%d / %d", run.CurrentRoom+1, run.TotalRooms)
						e.Detail.Map = buildRosterMap(run)
					}
				}
			}
		} else if pl.lastAction != nil {
			if h := int(now.Sub(*pl.lastAction).Hours()); h > 0 {
				e.IdleHours = h
			}
		}
		snap.Adventurers = append(snap.Adventurers, e)
	}
	return snap, nil
}

// buildRosterMap computes the fog-of-war cut of a run's zone graph for the
// public roster. It sends every visited node with its true kind, plus the
// one-hop frontier — the destinations of edges leading out of visited nodes,
// with their kind withheld as "unknown". Edges are directed and stored by
// from-node, so "one hop out of a visited node" is exactly g.Edges[visited].
// A frontier node's edges are NOT walked, so nothing past the first closed
// door reaches the wire — "view source to find the boss room" is not fog of
// war. Node Label/Content never leave the game box.
//
// Output order is deterministic (visited-path order, then frontier in
// discovery order) so an unchanged run produces a byte-identical snapshot and
// the roster push does not churn.
func buildRosterMap(run *DungeonRun) *peteclient.RosterMap {
	g, ok := loadZoneGraph(run.ZoneID)
	if !ok || len(run.VisitedNodes) == 0 {
		return nil
	}

	// Unique visited nodes in path order — VisitedNodes repeats on backtrack.
	orderedVisited := make([]string, 0, len(run.VisitedNodes))
	seen := make(map[string]bool, len(run.VisitedNodes))
	for _, id := range run.VisitedNodes {
		if !seen[id] {
			seen[id] = true
			orderedVisited = append(orderedVisited, id)
		}
	}

	m := &peteclient.RosterMap{
		ZoneID:      string(run.ZoneID),
		CurrentNode: run.CurrentNode,
		Visited:     orderedVisited,
	}

	// Visited nodes first, in path order, with their real kind.
	emitted := make(map[string]bool, len(orderedVisited))
	for _, id := range orderedVisited {
		emitted[id] = true
		if n, ok := g.Nodes[id]; ok {
			m.Nodes = append(m.Nodes, peteclient.RosterMapNode{ID: id, Kind: string(n.Kind)})
		}
	}

	// Frontier: destinations of edges out of visited nodes, kind withheld.
	// Walk visited in path order so the frontier order is stable.
	for _, from := range orderedVisited {
		for _, edge := range g.Edges[from] {
			lock := edge.Lock
			if lock == LockNone {
				lock = "" // an open door needs no mark; omitempty drops it
			}
			m.Edges = append(m.Edges, peteclient.RosterMapEdge{
				From: edge.From,
				To:   edge.To,
				Lock: string(lock),
			})
			if !seen[edge.To] && !emitted[edge.To] {
				emitted[edge.To] = true
				m.Nodes = append(m.Nodes, peteclient.RosterMapNode{ID: edge.To, Kind: "unknown"})
			}
		}
	}
	return m
}

// resolveRosterToken maps a board token back to the adventurer it names. The
// token is a one-way HMAC (eventToken), so it can't be inverted — instead we
// recompute every live player's token and match. The salt is DB-persisted, so a
// token minted before a restart still resolves after one. O(players) per call,
// which is nothing at realm scale and only runs when a web order is being placed.
func resolveRosterToken(token string) (id.UserID, bool) {
	if token == "" {
		return "", false
	}
	rows, err := db.Get().Query(`SELECT user_id FROM player_meta WHERE alive = 1`)
	if err != nil {
		return "", false
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		if eventToken(id.UserID(uid), "roster") == token {
			return id.UserID(uid), true
		}
	}
	return "", false
}

// mischiefTierCatalog is the storefront price list, pushed on every tick so the
// web shop always renders gogobee's current prices — a fee retune here reaches
// Pete within a snapshot, and Pete never hardcodes a number of its own. The
// signed fee is the same +25% sink a Matrix buyer pays to put their name on it.
func mischiefTierCatalog() []peteclient.MischiefTier {
	out := make([]peteclient.MischiefTier, 0, len(mischiefTiers))
	for _, t := range mischiefTiers {
		out = append(out, peteclient.MischiefTier{
			Key:       t.Key,
			Display:   t.Display,
			Fee:       t.Fee,
			SignedFee: mischiefSignedFee(t.Fee),
			Blurb:     t.Blurb,
		})
	}
	return out
}
