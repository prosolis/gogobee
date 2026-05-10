package plugin

// Phase G5 — branching-zone navigation surface.
//
// Wires the graph types from G2/G3/G4 into the !zone advance flow:
// when the player clears a room with 2+ outgoing edges, write a pending
// fork prompt to dnd_zone_run.node_choices and DM the menu. !zone go <n>
// consumes the choice, validates the chosen edge is unlocked, and
// advances the run state to the chosen node.
//
// G9a retired the GOGOBEE_BRANCHING_ZONES POC gate: graph mode is the
// only runtime path now that all 9 zones have hand-authored graphs.

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"gogobee/internal/db"
)

// pendingFork is the typed shape of dnd_zone_run.node_choices when the
// player is paused at a fork. Persisted as JSON inside the
// map[string]any NodeChoices column; helpers below round-trip via JSON
// so the column stays tolerant of hand-authored / future shapes.
type pendingFork struct {
	PendingAt string          `json:"pending_at"`
	Options   []pendingChoice `json:"options"`
}

type pendingChoice struct {
	Index    int    `json:"index"`
	To       string `json:"to"`
	Label    string `json:"label"`
	Unlocked bool   `json:"unlocked"`
	Hint     string `json:"hint"`
	Lock     string `json:"lock"`
	Reason   string `json:"reason,omitempty"`
}

// encodePendingFork turns a pendingFork into the map[string]any shape
// that DungeonRun.NodeChoices uses, so persisting just goes through
// the existing json marshaling in markRoomCleared / advanceZoneRunNode.
func encodePendingFork(pf pendingFork) (map[string]any, error) {
	raw, err := json.Marshal(pf)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decodePendingFork reads a pendingFork out of NodeChoices. Returns
// (nil, nil) when the column is empty or doesn't carry a fork prompt.
func decodePendingFork(m map[string]any) (*pendingFork, error) {
	if len(m) == 0 {
		return nil, nil
	}
	if _, ok := m["pending_at"]; !ok {
		return nil, nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var pf pendingFork
	if err := json.Unmarshal(raw, &pf); err != nil {
		return nil, err
	}
	return &pf, nil
}

// edgeUnlockCtx bundles everything the unlock evaluators need so we can
// test them without going through the live DB. Filled in by
// evaluateForkEdges from the live run + character.
type edgeUnlockCtx struct {
	RunID          string
	FromNode       string
	CharLevel      int
	AbilityMods    [6]int // STR, DEX, CON, INT, WIS, CHA — matches DnDCharacter.Modifiers()
	InventoryNames map[string]bool
	Expedition     *Expedition
}

// evaluateEdgeLock returns whether the player can take this edge right
// now, with a player-facing reason on failure. Per plan §G5: Perception
// rolls fire once at fork-arrival (deterministic seed) and the result
// is committed for the lifetime of the prompt. Re-renders show the
// same outcome so the player can't reload to retry.
func evaluateEdgeLock(e ZoneEdge, ctx edgeUnlockCtx) (unlocked bool, reason string) {
	switch e.Lock {
	case "", LockNone:
		return true, ""
	case LockPerception:
		dc := lockDataInt(e.LockData, "dc", 12)
		roll := perceptionRollForEdge(ctx.RunID, ctx.FromNode, e.To)
		total := roll + ctx.AbilityMods[4]
		if total >= dc {
			return true, ""
		}
		return false, fmt.Sprintf("Perception %d vs DC %d", total, dc)
	case LockKey:
		key := strings.ToLower(strings.TrimSpace(lockDataString(e.LockData, "key_id")))
		if key == "" {
			return false, "missing key (no key_id authored)"
		}
		if ctx.InventoryNames[key] {
			return true, ""
		}
		return false, "you don't have the key"
	case LockLevelMin:
		min := lockDataInt(e.LockData, "min_level", 1)
		if ctx.CharLevel >= min {
			return true, ""
		}
		return false, fmt.Sprintf("requires level %d (you are %d)", min, ctx.CharLevel)
	case LockRegionClear:
		region := lockDataString(e.LockData, "region_id")
		if region == "" {
			return false, "no region_id authored"
		}
		if ctx.Expedition != nil && IsRegionCleared(ctx.Expedition, region) {
			return true, ""
		}
		return false, "another region must be cleared first"
	case LockStatCheck:
		dc := lockDataInt(e.LockData, "dc", 12)
		stat := strings.ToUpper(lockDataString(e.LockData, "stat"))
		idx := abilityIndex(stat)
		if idx < 0 {
			return false, "invalid stat_check authoring"
		}
		roll := perceptionRollForEdge(ctx.RunID, ctx.FromNode, e.To)
		total := roll + ctx.AbilityMods[idx]
		if total >= dc {
			return true, ""
		}
		return false, fmt.Sprintf("%s %d vs DC %d", stat, total, dc)
	}
	return false, "unknown lock type"
}

// abilityIndex maps a stat short-name to the Modifiers() slot.
func abilityIndex(s string) int {
	switch strings.ToUpper(s) {
	case "STR":
		return 0
	case "DEX":
		return 1
	case "CON":
		return 2
	case "INT":
		return 3
	case "WIS":
		return 4
	case "CHA":
		return 5
	}
	return -1
}

func lockDataInt(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

func lockDataString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// perceptionRollForEdge synthesizes a stable 1d20 result for a given
// (run, from-node, to-node). SHA1 keeps the distribution clean and
// avoids math/rand state contention. Re-arrival at the same fork in
// the same run reproduces the same roll, so the player can't reload
// to retry a failed Perception (plan §G5).
func perceptionRollForEdge(runID, fromNode, toNode string) int {
	h := sha1.Sum([]byte(runID + "|" + fromNode + "|" + toNode))
	return int(binary.BigEndian.Uint16(h[:2])%20) + 1
}

// buildUnlockCtx assembles an edgeUnlockCtx from the live character +
// expedition state. Inventory item names are lower-cased for matching
// against lock_data.key_id.
func buildUnlockCtx(c *DnDCharacter, runID, fromNode string) edgeUnlockCtx {
	ctx := edgeUnlockCtx{
		RunID:          runID,
		FromNode:       fromNode,
		CharLevel:      c.Level,
		AbilityMods:    c.Modifiers(),
		InventoryNames: map[string]bool{},
	}
	if items, err := loadAdvInventory(c.UserID); err == nil {
		for _, it := range items {
			ctx.InventoryNames[strings.ToLower(it.Name)] = true
		}
	}
	if exp, err := getActiveExpedition(c.UserID); err == nil && exp != nil {
		ctx.Expedition = exp
	}
	return ctx
}

// evaluateForkEdges walks all outgoing edges of fromNode in the graph
// and produces a pending-choice list ready to be persisted. Locked
// edges that have a Hint stay in the menu (the player needs the
// teaser); locked edges without any hint at all are still listed but
// reasoned-out.
func evaluateForkEdges(g ZoneGraph, fromNode string, ctx edgeUnlockCtx) []pendingChoice {
	outs := g.outgoingEdges(fromNode)
	if len(outs) == 0 {
		return nil
	}
	choices := make([]pendingChoice, 0, len(outs))
	for i, e := range outs {
		unlocked, reason := evaluateEdgeLock(e, ctx)
		toNode := g.Nodes[e.To]
		label := toNode.Label
		if label == "" {
			label = prettyNodeKind(toNode.Kind)
		}
		choices = append(choices, pendingChoice{
			Index:    i + 1,
			To:       e.To,
			Label:    label,
			Unlocked: unlocked,
			Hint:     e.Hint,
			Lock:     string(e.Lock),
			Reason:   reason,
		})
	}
	return choices
}

func prettyNodeKind(k ZoneNodeKind) string {
	switch k {
	case NodeKindEntry:
		return "Entry"
	case NodeKindExploration:
		return "Exploration"
	case NodeKindTrap:
		return "Trap"
	case NodeKindElite:
		return "Elite"
	case NodeKindBoss:
		return "Boss"
	case NodeKindHarvest:
		return "Harvest"
	case NodeKindRestCamp:
		return "Rest Camp"
	case NodeKindSecret:
		return "Secret"
	case NodeKindFork:
		return "Fork"
	case NodeKindMerge:
		return "Merge"
	}
	return "Room"
}

// renderForkPrompt is the player-facing menu rendered from a pendingFork.
// Locked edges with a Hint show the hint as a teaser; locked edges
// without a hint just show "(locked)".
func renderForkPrompt(zone ZoneDefinition, pf pendingFork) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s — Path divides.** Choose with `!zone go <n>`.\n\n", zone.Display))
	for _, c := range pf.Options {
		switch {
		case c.Unlocked:
			b.WriteString(fmt.Sprintf("**%d.** %s\n", c.Index, c.Label))
		case c.Hint != "":
			b.WriteString(fmt.Sprintf("**%d.** %s _(locked — %s)_\n", c.Index, c.Label, c.Hint))
		default:
			b.WriteString(fmt.Sprintf("**%d.** %s _(locked)_\n", c.Index, c.Label))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// recordRoomCleared appends the current room to rooms_cleared and
// bumps last_action_at, without advancing current_room/current_node.
// Used by the graph-mode fork path: clearing the room is a separate
// step from choosing where to go next. Returns the updated DungeonRun
// snapshot reloaded post-write so callers see fresh fields.
func recordRoomCleared(runID string) (*DungeonRun, error) {
	r, err := getZoneRun(runID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrNoActiveRun
	}
	if !r.IsActive() {
		return nil, ErrNoActiveRun
	}
	cleared := append(r.RoomsCleared, r.CurrentRoom)
	clearedJSON, _ := json.Marshal(cleared)
	if _, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET rooms_cleared = ?,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, string(clearedJSON), runID); err != nil {
		return nil, err
	}
	r.RoomsCleared = cleared
	return r, nil
}

// writePendingFork persists a pendingFork into node_choices for the
// given run. Replaces any prior fork — there is only ever one pending
// at a time.
func writePendingFork(runID string, pf pendingFork) error {
	m, err := encodePendingFork(pf)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET node_choices = ?,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, string(raw), runID)
	return err
}

// clearPendingFork wipes node_choices. Called when the player commits a
// choice via !zone go and we transition to the chosen node.
func clearPendingFork(runID string) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET node_choices = '{}',
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, runID)
	return err
}

// completeRunAtNode marks the run finished at the current node. Used
// when graph-mode advance hits a 0-outgoing-edge boss or dead-end.
// boss=true sets boss_defeated; dead-ends leave it false.
func completeRunAtNode(runID string, boss bool) error {
	bossI := 0
	if boss {
		bossI = 1
	}
	_, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET boss_defeated = ?,
		       completed_at = CURRENT_TIMESTAMP,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, bossI, runID)
	return err
}

// advanceZoneRunNode moves a run to nextNode: appends to visited_nodes,
// sets current_node, bumps current_room (legacy dual-write), and
// clears any pending fork prompt. Caller is expected to have already
// called recordRoomCleared for the prior node.
func advanceZoneRunNode(runID, nextNode string) error {
	r, err := getZoneRun(runID)
	if err != nil {
		return err
	}
	if r == nil {
		return ErrNoActiveRun
	}
	visited := append(r.VisitedNodes, nextNode)
	visitedJSON, _ := json.Marshal(visited)
	nextRoom := r.CurrentRoom + 1
	_, err = db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET current_node  = ?,
		       visited_nodes = ?,
		       current_room  = ?,
		       node_choices  = '{}',
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`,
		nextNode, string(visitedJSON), nextRoom, runID)
	return err
}

// resolveForkChoice takes a 1-based choice index against a pending
// fork and returns the chosen pendingChoice if it's both present and
// unlocked. Errors are formatted for direct DM display.
func resolveForkChoice(pf *pendingFork, choice int) (pendingChoice, error) {
	if pf == nil || len(pf.Options) == 0 {
		return pendingChoice{}, fmt.Errorf("no fork pending")
	}
	if choice < 1 || choice > len(pf.Options) {
		return pendingChoice{}, fmt.Errorf("choice %d out of range (1–%d)", choice, len(pf.Options))
	}
	c := pf.Options[choice-1]
	if !c.Unlocked {
		if c.Reason != "" {
			return c, fmt.Errorf("path locked: %s", c.Reason)
		}
		return c, fmt.Errorf("path locked")
	}
	return c, nil
}

