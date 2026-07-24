package plugin

import (
	"fmt"
	"strings"

	"gogobee/internal/db"
)

// Revisit R2 (gogobee_revisit_plan.md §R2) — `!revisit <N>` walks one graph
// edge back toward a room the player has already been to. Forward-only
// navigation was the rule since Phase G; this retires it deliberately.
//
// Built on R1's split: CurrentRoom is the *position* (the first-entry index
// of CurrentNode) and moves backwards with the player, while RoomsTraversed
// is the *effort* and only climbs. The room keeps its original number, so a
// revisited room resolves to the same enemies, traps and harvest nodes it
// had on the way in.

// revisitThreatCost is what one backtrack edge adds to the threat clock.
//
// The revisit plan originally specced this as a *discount* on a +2/-1.0
// per-room cost. That cost does not exist: verified at HEAD, movement charges
// no threat and no supplies (supplies burn per day; threat comes from combat,
// drift, harvest noise and ambient). Backtracking through cleared rooms fires
// no combat, so without this it would be entirely free.
//
// +1 mirrors the harvest-noise bump: doubling back stirs up attention but
// costs less than a fight (+5) or an elite (+8). The real pressure on a
// detour remains the day clock, which a long one burns anyway.
const revisitThreatCost = 1

// revisitSiegeGuard refuses a voluntary backtrack that would trip Siege Mode.
// applyThreatDelta flips siege at 100 and siege is one-way sticky, so a
// player at 99 must not be able to strand their own expedition on a move
// they made to go pick up a herb.
const revisitSiegeGuard = 100 - revisitThreatCost

// adjacentNodes returns every node joined to `node` by a graph edge, in
// either direction. Zone graph edges are directed (they encode the forward
// route), but a corridor you walked down is a corridor you can walk back up.
func adjacentNodes(g ZoneGraph, node string) map[string]bool {
	adj := make(map[string]bool)
	for _, e := range g.outgoingEdges(node) {
		adj[e.To] = true
	}
	for from, outs := range g.Edges {
		for _, e := range outs {
			if e.To == node {
				adj[from] = true
			}
		}
	}
	return adj
}

// revisitZoneRun moves the run back to targetNode. VisitedNodes is untouched
// (it is an ordered set — see appendVisited), so the target keeps the room
// number the player read off `!map`. rooms_traversed still climbs: the walk
// happened, and narration cadence rides it.
//
// Returns the target's path index.
func revisitZoneRun(runID, targetNode string, visited []string) (int, error) {
	if _, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET current_node = ?,
		       rooms_traversed = rooms_traversed + 1,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, targetNode, runID); err != nil {
		return 0, err
	}
	idx := pathIndexOf(visited, targetNode)
	if run, _ := getZoneRun(runID); run != nil {
		beatRoom(run, targetNode, idx, "doubled back")
	}
	return idx, nil
}

// handleRevisitCmd implements `!revisit <N>` (also reachable as
// `!zone revisit <N>`). N is the 1-indexed room number from `!map`'s Path
// strip.
func (p *AdventurePlugin) handleRevisitCmd(ctx MessageContext, rest string) error {
	run, isLeader, err := activeZoneRunFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	// The fight outranks the path: a member swinging at a monster should hear
	// about the monster, not about who picks the route.
	if cs, _ := activeCombatSessionFor(ctx.Sender); cs != nil {
		return p.SendDM(ctx.Sender, "⚔️ Finish the fight first — `!attack` or `!flee`.")
	}
	if !isLeader {
		// Backtracking moves the whole party and costs the whole party a point
		// of threat. Same call as the fork: the leader's.
		return p.SendDM(ctx.Sender, msgLeaderPicksPath)
	}
	// A pending fork lives at the node the player is standing on, and both
	// `!zone advance` and `!zone go` resolve it without checking where that
	// is. Walking away would leave a prompt pointing at a room the player
	// has left. R3 makes revisit re-open forks properly; until then, commit.
	if pf, derr := decodePendingFork(run.NodeChoices); derr == nil && pf != nil {
		return p.SendDM(ctx.Sender, "Choose your path first — `!zone go <n>`.")
	}

	rest = strings.TrimSpace(rest)
	if rest == "" {
		return p.SendDM(ctx.Sender,
			"Revisit which room? Numbers come from the Path strip in `!map` — e.g. `!revisit 2`.")
	}
	n := atoiSafe(rest)
	if n <= 0 || n > len(run.VisitedNodes) {
		return p.SendDM(ctx.Sender, "You haven't been there yet. `!map` shows where you've walked.")
	}
	target := run.VisitedNodes[n-1]
	if target == run.CurrentNode {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You're already standing in Room %d.", n))
	}

	g, ok := loadZoneGraph(run.ZoneID)
	if !ok {
		return p.SendDM(ctx.Sender, "Couldn't read the zone layout.")
	}
	if !adjacentNodes(g, run.CurrentNode)[target] {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Room %d isn't directly connected — backtrack one step at a time.", n))
	}

	exp, _ := getActiveExpedition(ctx.Sender)
	if exp != nil && !exp.IsActive() {
		return p.SendDM(ctx.Sender, "Your expedition isn't underway.")
	}
	if exp != nil && exp.ThreatLevel >= revisitSiegeGuard {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"⏰ **Too hot to double back** — threat is %d/100. Doubling back would bring the siege down on you. Press on or `!extract`.",
			exp.ThreatLevel))
	}

	idx, rerr := revisitZoneRun(run.RunID, target, run.VisitedNodes)
	if rerr != nil {
		return p.SendDM(ctx.Sender, "Couldn't backtrack: "+rerr.Error())
	}
	markActedToday(ctx.Sender)

	var b strings.Builder
	if kind := autoBreakCampOnMove(ctx.Sender); kind != "" {
		b.WriteString(fmt.Sprintf("⛺ Camp struck (**%s**) — the party moved on.\n\n", kind))
	}
	roomType := nodeKindToRoomType(g.Nodes[target].Kind)
	b.WriteString(fmt.Sprintf("↩ You retrace your steps to **Room %d** — %s.\n\n",
		idx+1, prettyRoomType(roomType)))

	if exp != nil {
		// Standalone runs have no threat clock; only bill an expedition.
		_ = applyThreatDelta(exp.ID, revisitThreatCost, "retraced steps")
		grantAutorunGrace(exp.ID)
		b.WriteString(fmt.Sprintf("_Threat +%d — moving back through cleared ground is not quiet._\n\n",
			revisitThreatCost))
	}
	b.WriteString(fmt.Sprintf("**Room %d/%d — %s.** %s",
		idx+1, run.TotalRooms, prettyRoomType(roomType), continueHint(ctx.Sender)))
	return p.SendDM(ctx.Sender, b.String())
}

// grantAutorunGrace stamps the autorun CAS column so the background walker
// does not immediately march the player back out of the room they just
// walked into. Buys one full autoRunCooldown window.
//
// This is a stopgap, not the R5 autopilot guard: autopilot has no on/off
// switch (it is ticker-driven for every active expedition), so without it a
// revisit would be undone within the cooldown and the whole feature would
// read as broken. R5 decides the real policy — refuse to auto-walk from a
// non-frontier node, or walk back to the frontier first.
func grantAutorunGrace(expID string) {
	_, _ = db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_autorun_at = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, expID)
}

// revisitTargets lists the rooms adjacent to the player's position that they
// have already visited — the legal `!revisit` arguments. Rendered by `!map`
// so players don't have to guess which numbers are reachable.
func revisitTargets(g ZoneGraph, run *DungeonRun) []int {
	if run == nil {
		return nil
	}
	adj := adjacentNodes(g, run.CurrentNode)
	var out []int
	for i, node := range run.VisitedNodes {
		if node != run.CurrentNode && adj[node] {
			out = append(out, i+1)
		}
	}
	return out
}
