package plugin

// Phase G5 — graph-mode dispatch glue. Bridges the navigation helpers
// in zone_graph_nav.go into the !zone advance / !zone go command flows.

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// advanceTransitionGraph is the graph-mode replacement for
// markRoomCleared inside zoneCmdAdvance. It records that the current
// room is cleared, then decides what comes next based on the player's
// outgoing edges in the registered (or legacy-compiled) zone graph:
//
//   - 0 outgoing edges → run completes. complete=true. Caller emits
//     the run-complete narration. boss_defeated is set when the
//     current node is the graph's boss.
//   - 1 unlocked outgoing edge → auto-advance to that node. next is
//     the resolved next-room type for the teaser block.
//   - 2+ outgoing edges OR 1 locked edge → fork prompt is persisted
//     and forkMessage is the rendered menu. Caller emits it instead
//     of the next-room teaser. The player commits via !zone go <n>.
//
// A single locked edge still surfaces as a fork prompt: the player
// would otherwise be stuck with no signal that progression is gated.
func (p *AdventurePlugin) advanceTransitionGraph(run *DungeonRun, zone ZoneDefinition, c *DnDCharacter) (next RoomType, forkMessage string, complete bool, err error) {
	g, ok := loadZoneGraph(zone.ID)
	if !ok {
		err = fmt.Errorf("no graph for zone %q", zone.ID)
		return
	}
	currentNode := run.CurrentNode
	if currentNode == "" {
		// Should not happen post-G4 dual-write; fall back to derived id
		// so we never crash on an old row that snuck through.
		currentNode = deriveLegacyNodeID(run.ZoneID, run.CurrentRoom)
	}

	if _, rerr := recordRoomCleared(run.RunID); rerr != nil {
		err = rerr
		return
	}

	ctx := buildUnlockCtx(c, run.RunID, currentNode)
	choices := evaluateForkEdges(g, currentNode, ctx)
	if len(choices) == 0 {
		// Dead-end or boss node. Boss completes with boss_defeated=1;
		// non-boss dead-ends just terminate the run quietly.
		isBoss := g.Nodes[currentNode].IsBoss
		if cerr := completeRunAtNode(run.RunID, isBoss); cerr != nil {
			err = cerr
			return
		}
		complete = true
		return
	}

	unlocked := unlockedChoices(choices)
	if len(choices) == 1 && len(unlocked) == 1 {
		// Plain auto-advance — no fork to prompt for.
		chosen := choices[0]
		if aerr := advanceZoneRunNode(run.RunID, chosen.To); aerr != nil {
			err = aerr
			return
		}
		// G6d — region boundary hook. Multi-region branching graphs
		// can cross a region edge mid-run; emit the existing transit
		// flavor + log so the player gets the same "you've crossed
		// into X" beat the !region travel command produces. Doesn't
		// retire the run (the graph is the authoritative state) — it
		// just narrates the crossing.
		fireGraphRegionTransition(run.UserID, g.Nodes[currentNode], g.Nodes[chosen.To])
		next = nodeKindToRoomType(g.Nodes[chosen.To].Kind)
		return
	}

	pf := pendingFork{
		PendingAt: currentNode,
		Options:   choices,
	}
	if werr := writePendingFork(run.RunID, pf); werr != nil {
		err = werr
		return
	}
	forkMessage = renderForkPrompt(zone, pf)
	return
}

// fireGraphRegionTransition compares the from/to graph node region ids
// and, when they differ, mirrors the existing !region travel surface:
// updates Expedition.CurrentRegion + visited list and writes a transit
// log entry. Unlike full !region travel, it does NOT burn supplies,
// advance the day, or retire/spawn DungeonRuns — the graph is the
// authoritative run state, and the player crossed via the graph edge,
// not a separate transit day. Standalone (no expedition) is a no-op.
func fireGraphRegionTransition(userID string, from, to ZoneNode) {
	if from.RegionID == to.RegionID {
		return
	}
	if to.RegionID == "" {
		return
	}
	exp, err := getActiveExpedition(id.UserID(userID))
	if err != nil || exp == nil {
		return
	}
	if exp.CurrentRegion == to.RegionID {
		return
	}
	if err := setCurrentRegion(exp, to.RegionID); err != nil {
		return
	}
	_, _ = MarkRegionVisited(exp, to.RegionID)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
		fmt.Sprintf("graph region transit: %s → %s (node %s → %s)",
			from.RegionID, to.RegionID, from.NodeID, to.NodeID), "")
}

func unlockedChoices(cs []pendingChoice) []pendingChoice {
	out := make([]pendingChoice, 0, len(cs))
	for _, c := range cs {
		if c.Unlocked {
			out = append(out, c)
		}
	}
	return out
}

// nodeKindToRoomType bridges the graph node kinds back to the legacy
// RoomType enum used by !zone advance's teaser. Stays in lockstep with
// roomTypeToNodeKind in zone_graph.go.
func nodeKindToRoomType(k ZoneNodeKind) RoomType {
	switch k {
	case NodeKindEntry:
		return RoomEntry
	case NodeKindExploration, NodeKindSecret, NodeKindHarvest, NodeKindRestCamp, NodeKindFork, NodeKindMerge:
		return RoomExploration
	case NodeKindTrap:
		return RoomTrap
	case NodeKindElite:
		return RoomElite
	case NodeKindBoss:
		return RoomBoss
	}
	return RoomExploration
}

// zoneCmdGo handles `!zone go <n>` (alias `!zone choose <n>`). Reads
// the pending fork from node_choices, validates the chosen edge is
// unlocked, advances the run state to the chosen node, and emits the
// arrival teaser. The player still has to !zone advance to resolve
// the new room — same cadence as auto-advance.
func (p *AdventurePlugin) zoneCmdGo(ctx MessageContext, rest string) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	pf, derr := decodePendingFork(run.NodeChoices)
	if derr != nil {
		return p.SendDM(ctx.Sender, "Couldn't decode pending fork: "+derr.Error())
	}
	if pf == nil {
		return p.SendDM(ctx.Sender, "No fork pending. Use `!zone advance` to continue.")
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		zone := zoneOrFallback(run.ZoneID)
		return p.SendDM(ctx.Sender, "**Pick a path:**\n\n"+renderForkPrompt(zone, *pf)+"\n\n_Use `!zone go <n>`._")
	}
	choice := atoiSafe(rest)
	if choice <= 0 {
		return p.SendDM(ctx.Sender, "Choice must be a number from the menu (`!zone go 1`, `!zone go 2`, …).")
	}
	chosen, cerr := resolveForkChoice(pf, choice)
	if cerr != nil {
		return p.SendDM(ctx.Sender, cerr.Error())
	}
	if aerr := advanceZoneRunNode(run.RunID, chosen.To); aerr != nil {
		return p.SendDM(ctx.Sender, "Couldn't advance: "+aerr.Error())
	}
	g, _ := loadZoneGraph(run.ZoneID)
	zone := zoneOrFallback(run.ZoneID)
	fromNode := g.Nodes[run.CurrentNode]
	nextNode := g.Nodes[chosen.To]
	fireGraphRegionTransition(run.UserID, fromNode, nextNode)
	nextRoom := nodeKindToRoomType(nextNode.Kind)
	nextIdx := run.CurrentRoom + 1
	var b strings.Builder
	b.WriteString(fmt.Sprintf("➡ You take the path: **%s**.\n\n", chosen.Label))
	if nextRoom == RoomBoss {
		if line := composeBossEntry(zone.ID, run.RunID, nextIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	} else {
		if line := twinBeeLine(zone.ID, DMRoomEntry, run.RunID, nextIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	}
	b.WriteString(fmt.Sprintf("**Room %d/%d — %s.** `!zone advance` to continue.",
		nextIdx+1, run.TotalRooms, prettyRoomType(nextRoom)))
	return p.SendDM(ctx.Sender, b.String())
}
