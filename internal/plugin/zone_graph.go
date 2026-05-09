package plugin

// Phase G2 — branching zone graph types + validator.
//
// See gogobee_branching_zones_plan.md §2-§3. Zones author their topology
// as a directed graph of ZoneNode + ZoneEdge. The graph is registered
// alongside ZoneDefinition; runtime navigation lives in later phases.
//
// G2 is infra only: types, builders, validator, registry. Nothing reads
// these graphs yet.

import (
	"errors"
	"fmt"
)

type ZoneNodeKind string

const (
	NodeKindEntry       ZoneNodeKind = "entry"
	NodeKindExploration ZoneNodeKind = "exploration"
	NodeKindTrap        ZoneNodeKind = "trap"
	NodeKindElite       ZoneNodeKind = "elite"
	NodeKindBoss        ZoneNodeKind = "boss"
	NodeKindHarvest     ZoneNodeKind = "harvest"
	NodeKindRestCamp    ZoneNodeKind = "rest_camp"
	NodeKindSecret      ZoneNodeKind = "secret"
	NodeKindFork        ZoneNodeKind = "fork"
	NodeKindMerge       ZoneNodeKind = "merge"
)

// ZoneNodeContent — typed wrapper around the persisted content_json blob.
// EncounterOverride pins a specific bestiary id; if empty, the zone's
// roster roll is used. HarvestRef points at a harvest table id (G6
// rewires dnd_expedition_harvest off room_idx onto node_id). LootBias
// multiplies the end-of-room loot roll (Secret rooms typically ≥ 1.5).
// Narration is bespoke entry text that overrides the zone-level pool.
// AllowSelfLoop opts out of the validator's self-loop ban.
type ZoneNodeContent struct {
	EncounterOverride string
	HarvestRef        string
	LootBias          float64
	Narration         string
	AllowSelfLoop     bool
}

type ZoneNode struct {
	NodeID   string
	ZoneID   ZoneID
	RegionID string
	Kind     ZoneNodeKind
	Label    string
	IsEntry  bool
	IsBoss   bool
	PosX     int
	PosY     int
	Content  ZoneNodeContent
}

type ZoneEdgeLockKind string

const (
	LockNone        ZoneEdgeLockKind = "none"
	LockPerception  ZoneEdgeLockKind = "perception_check"
	LockKey         ZoneEdgeLockKind = "key_required"
	LockLevelMin    ZoneEdgeLockKind = "level_min"
	LockRegionClear ZoneEdgeLockKind = "region_clear"
	LockStatCheck   ZoneEdgeLockKind = "stat_check"
)

type ZoneEdge struct {
	From     string
	To       string
	Lock     ZoneEdgeLockKind
	LockData map[string]any
	Hint     string
	Weight   int
}

// ZoneGraph — fully validated graph for a zone. Nodes is keyed by
// NodeID; Edges is keyed by from-node so outgoing-edge lookup is O(1).
// Entry/Boss are the canonical node IDs; the validator guarantees both
// exist and that Boss is reachable from Entry.
type ZoneGraph struct {
	ZoneID ZoneID
	Nodes  map[string]ZoneNode
	Edges  map[string][]ZoneEdge
	Entry  string
	Boss   string
}

// BuildLinearGraph compiles a flat room sequence (the existing model)
// into a graph with N-1 unconditional edges. Used by the legacy zone
// compiler (G3) and by zones that intentionally stay linear. Node IDs
// are zoneID-prefixed and 1-indexed: "<zone>.r1", "<zone>.r2", ...
// First node is entry, last node is boss; if seq is empty or has no
// boss-typed final entry, the caller is expected to coerce.
func BuildLinearGraph(zoneID ZoneID, seq []ZoneNodeKind) ZoneGraph {
	g := ZoneGraph{
		ZoneID: zoneID,
		Nodes:  map[string]ZoneNode{},
		Edges:  map[string][]ZoneEdge{},
	}
	if len(seq) == 0 {
		return g
	}
	ids := make([]string, len(seq))
	for i, kind := range seq {
		id := fmt.Sprintf("%s.r%d", zoneID, i+1)
		ids[i] = id
		n := ZoneNode{
			NodeID: id,
			ZoneID: zoneID,
			Kind:   kind,
			PosX:   i,
		}
		if i == 0 {
			n.IsEntry = true
			n.Kind = NodeKindEntry
		}
		if i == len(seq)-1 {
			n.IsBoss = true
			n.Kind = NodeKindBoss
		}
		g.Nodes[id] = n
	}
	for i := 0; i < len(ids)-1; i++ {
		g.Edges[ids[i]] = []ZoneEdge{{From: ids[i], To: ids[i+1], Lock: LockNone, Weight: 1}}
	}
	g.Entry = ids[0]
	g.Boss = ids[len(ids)-1]
	return g
}

// BuildGraph is the explicit authoring path: pass nodes + edges
// directly. Validates: exactly one entry, exactly one boss, boss
// reachable from entry, no orphan nodes (every non-entry node has at
// least one incoming edge), no self-loops unless the node opts in via
// Content.AllowSelfLoop. Panics on invalid input — bad authoring
// should never reach a player.
func BuildGraph(zoneID ZoneID, nodes []ZoneNode, edges []ZoneEdge) ZoneGraph {
	g := ZoneGraph{
		ZoneID: zoneID,
		Nodes:  make(map[string]ZoneNode, len(nodes)),
		Edges:  map[string][]ZoneEdge{},
	}
	for _, n := range nodes {
		n.ZoneID = zoneID
		if _, dup := g.Nodes[n.NodeID]; dup {
			panic(fmt.Sprintf("duplicate node id %q in zone %q", n.NodeID, zoneID))
		}
		g.Nodes[n.NodeID] = n
		if n.IsEntry {
			g.Entry = n.NodeID
		}
		if n.IsBoss {
			g.Boss = n.NodeID
		}
	}
	for _, e := range edges {
		if e.Weight == 0 {
			e.Weight = 1
		}
		if e.Lock == "" {
			e.Lock = LockNone
		}
		g.Edges[e.From] = append(g.Edges[e.From], e)
	}
	if err := validateZoneGraph(g); err != nil {
		panic(fmt.Sprintf("invalid zone graph for %q: %v", zoneID, err))
	}
	return g
}

// validateZoneGraph enforces the structural invariants. Surfaced as a
// returned error so unit tests can exercise failure modes; BuildGraph
// converts the error into a panic at registration time.
func validateZoneGraph(g ZoneGraph) error {
	if len(g.Nodes) == 0 {
		return errors.New("graph has no nodes")
	}
	var entries, bosses int
	for _, n := range g.Nodes {
		if n.IsEntry {
			entries++
		}
		if n.IsBoss {
			bosses++
		}
	}
	if entries != 1 {
		return fmt.Errorf("expected exactly 1 entry node, got %d", entries)
	}
	if bosses != 1 {
		return fmt.Errorf("expected exactly 1 boss node, got %d", bosses)
	}
	// Edges must reference known nodes. Self-loops require opt-in.
	for from, outs := range g.Edges {
		if _, ok := g.Nodes[from]; !ok {
			return fmt.Errorf("edge from unknown node %q", from)
		}
		for _, e := range outs {
			if _, ok := g.Nodes[e.To]; !ok {
				return fmt.Errorf("edge %q→%q targets unknown node", from, e.To)
			}
			if e.From == e.To && !g.Nodes[from].Content.AllowSelfLoop {
				return fmt.Errorf("self-loop on node %q without AllowSelfLoop opt-in", from)
			}
		}
	}
	// No orphan nodes: every non-entry node must have at least one
	// incoming edge.
	incoming := map[string]int{}
	for _, outs := range g.Edges {
		for _, e := range outs {
			incoming[e.To]++
		}
	}
	for id, n := range g.Nodes {
		if n.IsEntry {
			continue
		}
		if incoming[id] == 0 {
			return fmt.Errorf("orphan node %q has no incoming edges", id)
		}
	}
	// Boss reachable from entry via BFS.
	if !reachable(g, g.Entry, g.Boss) {
		return fmt.Errorf("boss %q not reachable from entry %q", g.Boss, g.Entry)
	}
	return nil
}

func reachable(g ZoneGraph, from, to string) bool {
	if from == to {
		return true
	}
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range g.Edges[cur] {
			if e.To == to {
				return true
			}
			if !seen[e.To] {
				seen[e.To] = true
				queue = append(queue, e.To)
			}
		}
	}
	return false
}

// outgoingEdges returns the registered out-edges for a node in stable
// (Weight asc, then To asc) order, so fork prompts render
// deterministically. Consumers must apply lock evaluation themselves.
func (g ZoneGraph) outgoingEdges(from string) []ZoneEdge {
	src := g.Edges[from]
	out := make([]ZoneEdge, len(src))
	copy(out, src)
	// Stable sort: weight asc, then To asc. Avoids importing sort for
	// what is almost always a 1-3 element slice.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, b := out[j-1], out[j]
			less := a.Weight < b.Weight || (a.Weight == b.Weight && a.To < b.To)
			if less {
				break
			}
			out[j-1], out[j] = b, a
		}
	}
	return out
}

// zoneGraphRegistry — populated by registerZoneGraph at init (POC zones
// in G7 onward) and by the legacy compiler below (G3) for every other
// zone. loadZoneGraph returns the registered graph if present, else
// falls back to the linear graph synthesized from the ZoneDefinition.
var zoneGraphRegistry = map[ZoneID]ZoneGraph{}

func registerZoneGraph(g ZoneGraph) {
	if _, dup := zoneGraphRegistry[g.ZoneID]; dup {
		panic("duplicate zone graph: " + string(g.ZoneID))
	}
	if err := validateZoneGraph(g); err != nil {
		panic("invalid zone graph for " + string(g.ZoneID) + ": " + err.Error())
	}
	zoneGraphRegistry[g.ZoneID] = g
}

// compileLegacyZoneGraph synthesizes a linear graph from a
// ZoneDefinition that hasn't been hand-authored as a branching graph.
// Uses the canonical room pattern from generateRoomSequence
// (entry → exploration ×N₁ → trap → exploration ×N₂ → elite → boss)
// with N₁+N₂ chosen so total = MaxRooms. The exact per-run sequence
// still varies in length; this representative graph exists so every
// zone has a graph at boot for the validator and runtime fallbacks.
// G4's run-state hot-swap uses the per-run RoomSeq when it needs the
// authoritative shape.
func compileLegacyZoneGraph(z ZoneDefinition) ZoneGraph {
	total := z.MaxRooms
	const fixed = 4 // entry + trap + elite + boss
	if total < fixed+2 {
		total = fixed + 2
	}
	exps := total - fixed
	preTrap := exps / 2
	if preTrap < 1 {
		preTrap = 1
	}
	postTrap := exps - preTrap
	if postTrap < 1 {
		postTrap = 1
		preTrap = exps - postTrap
	}
	seq := make([]ZoneNodeKind, 0, total)
	seq = append(seq, NodeKindEntry)
	for i := 0; i < preTrap; i++ {
		seq = append(seq, NodeKindExploration)
	}
	seq = append(seq, NodeKindTrap)
	for i := 0; i < postTrap; i++ {
		seq = append(seq, NodeKindExploration)
	}
	seq = append(seq, NodeKindElite, NodeKindBoss)
	return BuildLinearGraph(z.ID, seq)
}

// compileRunGraph builds a linear graph that exactly mirrors a
// DungeonRun's RoomSeq. Used by the G4 hot-swap path to derive a
// node id from current_room when a row predates current_node. The
// returned graph is single-use (per run) and not registered.
func compileRunGraph(zoneID ZoneID, seq []RoomType) ZoneGraph {
	if len(seq) == 0 {
		return ZoneGraph{ZoneID: zoneID, Nodes: map[string]ZoneNode{}, Edges: map[string][]ZoneEdge{}}
	}
	kinds := make([]ZoneNodeKind, len(seq))
	for i, rt := range seq {
		kinds[i] = roomTypeToNodeKind(rt)
	}
	return BuildLinearGraph(zoneID, kinds)
}

func roomTypeToNodeKind(rt RoomType) ZoneNodeKind {
	switch rt {
	case RoomEntry:
		return NodeKindEntry
	case RoomExploration:
		return NodeKindExploration
	case RoomTrap:
		return NodeKindTrap
	case RoomElite:
		return NodeKindElite
	case RoomBoss:
		return NodeKindBoss
	}
	return NodeKindExploration
}

// loadZoneGraph returns the graph for a zone. Registered (hand-authored)
// graphs take precedence; otherwise the legacy linear compiler is used.
// Returns ok=false only for unknown zone IDs.
func loadZoneGraph(zoneID ZoneID) (ZoneGraph, bool) {
	if g, ok := zoneGraphRegistry[zoneID]; ok {
		return g, true
	}
	z, ok := getZone(zoneID)
	if !ok {
		return ZoneGraph{}, false
	}
	return compileLegacyZoneGraph(z), true
}
