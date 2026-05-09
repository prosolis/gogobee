# Gogobee — Branching Zones (Option A: Greenfield Zone Graph)

**Status:** plan, not yet implemented.
**Branch convention:** carve a new branch off `adv-2.0` (e.g. `branching-zones` or `phase-G`).
**Prerequisite:** none — Phase L is deployed; nothing else blocks this.
**Owner:** future session.

---

## 1. Goal & non-goals

### Goal

Replace the linear room model (`room_seq_json` flat array + `current_room INTEGER` index) with a **graph model**: zones are directed graphs of nodes, players navigate by choosing outgoing edges, edges can be gated by Perception checks / keys / level / cleared regions. Enables:

- Slay-the-Spire-style "pick your next room" forks.
- Optional side rooms (secret chambers behind Perception walls).
- Shortcuts that skip rooms.
- Multiple paths converging on a shared boss antechamber.
- Loop-back edges (off by default; opt-in per zone).

### Non-goals

- Rewriting combat. The combat engine (`combat_engine.go` / `combat_stats.go`) is shared infra per `gogobee_legacy_migration.md` §7.4 and stays untouched.
- Changing zone *contents* (enemy rosters, loot tables, boss stat blocks). Only the topology of how rooms connect changes.
- Reworking the multi-region system. Regions remain the coarse-grain layer; the graph lives **inside** a region. A zone like Underdark has 4 regions and each region has its own subgraph.

---

## 2. Schema

Two new tables. **Naming**: avoid the `dnd_` prefix per the project feedback note about trademark exposure — use `zone_node` / `zone_edge`. Existing `dnd_zone_run` is grandfathered.

### `zone_node`

```sql
CREATE TABLE zone_node (
    node_id        TEXT PRIMARY KEY,           -- "crypt_valdris.entry", globally unique
    zone_id        TEXT NOT NULL,              -- FK-ish: matches ZoneID
    region_id      TEXT NOT NULL DEFAULT '',   -- empty for single-region zones
    kind           TEXT NOT NULL,              -- entry|exploration|trap|elite|boss|harvest|rest_camp|secret|fork|merge
    label          TEXT NOT NULL DEFAULT '',   -- human-readable for !zone map ("Side Chapel")
    is_entry       INTEGER NOT NULL DEFAULT 0, -- exactly one per zone (or per region for multi-region)
    is_boss        INTEGER NOT NULL DEFAULT 0, -- exactly one boss reachable per zone
    pos_x          INTEGER NOT NULL DEFAULT 0, -- ASCII map layout hint
    pos_y          INTEGER NOT NULL DEFAULT 0,
    content_json   TEXT NOT NULL DEFAULT '{}'  -- opaque per-node payload (loot bias, encounter override, harvest ref)
);
CREATE INDEX idx_zone_node_zone ON zone_node(zone_id);
```

### `zone_edge`

```sql
CREATE TABLE zone_edge (
    edge_id        INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id        TEXT NOT NULL,
    from_node      TEXT NOT NULL,              -- FK-ish: zone_node.node_id
    to_node        TEXT NOT NULL,              -- FK-ish: zone_node.node_id
    lock_kind      TEXT NOT NULL DEFAULT 'none', -- none|perception_check|key_required|level_min|region_clear|stat_check
    lock_data_json TEXT NOT NULL DEFAULT '{}', -- {"dc": 12} | {"key_id": "skull_key"} | {"min_level": 5} | etc
    hint           TEXT NOT NULL DEFAULT '',   -- player-visible hint when lock active ("A faint breeze suggests a passage")
    weight         INTEGER NOT NULL DEFAULT 1  -- for ordering choices in the prompt
);
CREATE INDEX idx_zone_edge_from ON zone_edge(zone_id, from_node);
```

### Run-state additions on `dnd_zone_run`

```sql
ALTER TABLE dnd_zone_run ADD COLUMN current_node TEXT NOT NULL DEFAULT '';
ALTER TABLE dnd_zone_run ADD COLUMN visited_nodes TEXT NOT NULL DEFAULT '[]'; -- JSON array of node_ids
ALTER TABLE dnd_zone_run ADD COLUMN node_choices TEXT NOT NULL DEFAULT '{}';  -- JSON: pending fork prompt state
```

`current_room` and `room_seq_json` stay during the migration; retired in Phase G9.

### Runtime (Go) shapes

```go
// internal/plugin/zone_graph.go (new file)

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
    NodeKindFork        ZoneNodeKind = "fork"   // pure choice node, no encounter
    NodeKindMerge       ZoneNodeKind = "merge"  // paths converge, no encounter
)

type ZoneNode struct {
    NodeID    string       // "crypt_valdris.entry"
    ZoneID    ZoneID
    RegionID  string       // "" for single-region
    Kind      ZoneNodeKind
    Label     string       // "Side Chapel"
    IsEntry   bool
    IsBoss    bool
    PosX, PosY int
    Content   ZoneNodeContent // typed wrapper around content_json
}

type ZoneNodeContent struct {
    EncounterOverride string  // bestiary_id; if "", roll from zone roster
    HarvestRef        string  // harvest table id
    LootBias          float64 // multiplier for end-of-room loot roll
    Narration         string  // optional bespoke entry text
}

type ZoneEdgeLockKind string

const (
    LockNone        ZoneEdgeLockKind = "none"
    LockPerception  ZoneEdgeLockKind = "perception_check"
    LockKey         ZoneEdgeLockKind = "key_required"
    LockLevelMin    ZoneEdgeLockKind = "level_min"
    LockRegionClear ZoneEdgeLockKind = "region_clear"
    LockStatCheck   ZoneEdgeLockKind = "stat_check"  // {"stat":"str","dc":14}
)

type ZoneEdge struct {
    From     string
    To       string
    Lock     ZoneEdgeLockKind
    LockData map[string]any
    Hint     string
    Weight   int
}

type ZoneGraph struct {
    ZoneID ZoneID
    Nodes  map[string]ZoneNode  // keyed by NodeID
    Edges  map[string][]ZoneEdge // keyed by from_node
    Entry  string               // entry NodeID
    Boss   string               // boss NodeID (must be reachable)
}
```

---

## 3. Authoring model

Zones author their graph as Go literals, registered alongside the existing `ZoneDefinition`. Two convenience builders:

```go
// BuildLinearGraph compiles a flat room sequence (the existing model) into
// a graph with N-1 unconditional edges. Used for legacy compat and for
// zones that intentionally stay linear.
func BuildLinearGraph(zoneID ZoneID, seq []ZoneNodeKind) ZoneGraph

// BuildGraph is the explicit authoring path: pass nodes + edges directly.
// Validates: exactly one entry, exactly one boss, boss reachable from
// entry, no orphan nodes, no self-loops unless explicitly opted in.
func BuildGraph(zoneID ZoneID, nodes []ZoneNode, edges []ZoneEdge) ZoneGraph
```

Authoring example for a branched Crypt of Valdris (POC):

```go
//   entry → corridor → FORK
//                       ├─[lockNone]── main_hall ──┐
//                       │                          ├── antechamber → boss
//                       └─[perception DC 12]──── side_chapel ──┘ (also: secret edge to crypt_loot)
//   secret_chamber dangles off side_chapel via perception DC 15

func zoneCryptValdrisGraph() ZoneGraph {
    nodes := []ZoneNode{
        {NodeID: "crypt_valdris.entry",          Kind: NodeKindEntry,       IsEntry: true, Label: "Crypt Threshold"},
        {NodeID: "crypt_valdris.corridor",       Kind: NodeKindExploration, Label: "Bone Corridor"},
        {NodeID: "crypt_valdris.fork",           Kind: NodeKindFork,        Label: "Branching Passage"},
        {NodeID: "crypt_valdris.main_hall",      Kind: NodeKindElite,       Label: "Main Hall"},
        {NodeID: "crypt_valdris.side_chapel",    Kind: NodeKindExploration, Label: "Side Chapel"},
        {NodeID: "crypt_valdris.secret_chamber", Kind: NodeKindSecret,      Label: "Hidden Reliquary",
         Content: ZoneNodeContent{LootBias: 2.0}},
        {NodeID: "crypt_valdris.antechamber",    Kind: NodeKindMerge,       Label: "Antechamber"},
        {NodeID: "crypt_valdris.boss",           Kind: NodeKindBoss,        IsBoss: true, Label: "Valdris's Sanctum"},
    }
    edges := []ZoneEdge{
        {From: "crypt_valdris.entry",       To: "crypt_valdris.corridor",       Lock: LockNone},
        {From: "crypt_valdris.corridor",    To: "crypt_valdris.fork",           Lock: LockNone},
        {From: "crypt_valdris.fork",        To: "crypt_valdris.main_hall",      Lock: LockNone, Weight: 1},
        {From: "crypt_valdris.fork",        To: "crypt_valdris.side_chapel",    Lock: LockPerception, LockData: map[string]any{"dc": 12}, Hint: "A faint draft from a crack in the wall.", Weight: 2},
        {From: "crypt_valdris.main_hall",   To: "crypt_valdris.antechamber",    Lock: LockNone},
        {From: "crypt_valdris.side_chapel", To: "crypt_valdris.antechamber",    Lock: LockNone},
        {From: "crypt_valdris.side_chapel", To: "crypt_valdris.secret_chamber", Lock: LockPerception, LockData: map[string]any{"dc": 15}, Hint: "A faint scratching behind the altar."},
        {From: "crypt_valdris.secret_chamber", To: "crypt_valdris.antechamber", Lock: LockNone},
        {From: "crypt_valdris.antechamber", To: "crypt_valdris.boss",           Lock: LockNone},
    }
    return BuildGraph(ZoneCryptValdris, nodes, edges)
}
```

Registration parallels existing `registerZone`:

```go
var zoneGraphRegistry = map[ZoneID]ZoneGraph{}

func registerZoneGraph(g ZoneGraph) {
    if _, dup := zoneGraphRegistry[g.ZoneID]; dup {
        panic("duplicate zone graph: " + g.ZoneID)
    }
    if err := validateZoneGraph(g); err != nil {
        panic("invalid zone graph for " + g.ZoneID + ": " + err.Error())
    }
    zoneGraphRegistry[g.ZoneID] = g
}
```

---

## 4. Migration phases

### G1 — Schema introduction *(infra; no behavior change)*

- Add `zone_node`, `zone_edge` CREATE statements to `internal/db/db.go` schema.
- Add `current_node` / `visited_nodes` / `node_choices` columns to `dnd_zone_run` via `columnMigrations`.
- No code reads from these yet.
- **Test:** `TestProdDBMigrations` passes against a fresh copy of `data/gogobee.db`.

### G2 — Graph types + validator *(infra)*

- New file `internal/plugin/zone_graph.go` defining the types in §2.
- Implement `BuildLinearGraph`, `BuildGraph`, `validateZoneGraph`.
- Validator checks:
  - Exactly one node has `IsEntry=true`.
  - Exactly one node has `IsBoss=true`.
  - Boss is reachable from entry via BFS.
  - No orphan nodes (every non-entry node has an incoming edge).
  - No self-loops unless `Content.AllowSelfLoop` flag is set (off by default).
- **Test:** unit tests with hand-rolled graphs, including failure cases.

### G3 — Legacy zone compiler + dual-mode loader *(infra; no behavior change)*

- For every existing `ZoneDefinition`, compile its `room_seq_json` template into a linear graph at registry init:

```go
func compileLegacyZoneGraph(z ZoneDefinition) ZoneGraph {
    // Read the room-type template that generateRoomSequence currently uses,
    // produce a linear chain of nodes with unconditional edges. Entry node
    // = first room, boss node = last room.
}
```

- `loadZoneGraph(zoneID)` returns the registered graph if present, else falls back to the legacy-compiled one.
- All zones now have a graph representation, but nothing reads it yet.
- **Test:** every existing zone compiles cleanly; node count = legacy room count.

### G4 — Run-state model + reader hot-swap *(behavior parity)*

- `dnd_zone_run` reader populates `current_node` from `current_room` if `current_node` is empty (one-time per-row warm-up).
- New helpers:
  - `loadZoneRunNode(runID) (currentNode string, visited []string, ...)`
  - `advanceZoneRunNode(runID, nextNode string)` — appends to `visited_nodes`, sets `current_node`, also bumps `current_room` for any legacy code still reading it.
- Run-state writes: every existing site that mutates `current_room` now also calls `advanceZoneRunNode` with the corresponding linear-graph node.
- **Test:** an in-progress run loaded post-deploy continues to advance correctly under the dual-write scheme.

### G5 — Navigation surface (the main UX change)

- `enterRoom` / room-advance code branches on `len(outgoingEdges(currentNode))`:
  - 1 edge: auto-advance (current behavior).
  - 0 edges: end-of-zone state (boss cleared OR dead-end secret).
  - 2+ edges: write a fork prompt to `node_choices`, DM the player a numbered menu of unlocked edges, plus hints for locked ones.
- New command `!zone go <n>` (also `!zone choose <n>`) consumes the pending fork choice. Validates the selection is unlocked given current state (Perception roll, key inventory, level, etc.).
- Edge unlock evaluation:
  - `LockNone` — always available.
  - `LockPerception` — auto-roll Perception when player reaches the fork; success unlocks the edge silently (still must be picked); failure shows the hint as a teaser ("you sense something but can't place it"). Re-roll never; choice is committed at fork-arrival.
  - `LockKey` — checks player inventory for `lock_data.key_id`.
  - `LockLevelMin` — `dnd_character.dnd_level >= lock_data.min_level`.
  - `LockRegionClear` — region progression (multi-region zones).
  - `LockStatCheck` — generic stat-vs-DC roll.
- `!zone status` reports current node label + outgoing edge previews.
- **Test:** end-to-end forks, locked-edge teasing, unlock paths, secret-chamber gating.

### G6 — Dependent surfaces

- **Harvest nodes** (`dnd_expedition_harvest.go`): currently keyed by `(region_id, room_idx)`. Re-key on `node_id`. Author harvest content into `ZoneNodeContent.HarvestRef`. Migration: legacy harvest_node rows get their `node_id` derived from `room_idx` via the linear-graph compiler.
- **Narration cadence** (`dnd_zone_narration.go`): mood asides, threat ticks, TwinBee interjections counted by `len(visited_nodes)` instead of `current_room`.
- **`!zone map` renderer**: rewrite to render the graph using `pos_x` / `pos_y` hints. Visited nodes filled, current node highlighted, locked edges shown dashed, secrets hidden until discovered.
- **Boss flow** unchanged in essence — entering the boss node still triggers `bossEncounter`. The path to it is what changed.
- **Region transitions** (`dnd_expedition_region.go`): when `nextNode.RegionID != currentNode.RegionID`, fire the existing region-transition flow.

### G7 — POC zone (Crypt of Valdris)

- Implement `zoneCryptValdrisGraph()` per §3 example.
- Register alongside the existing `zoneCryptValdris()` zone definition.
- Old linear template stays in place — only new runs use the graph form (gated by env var `GOGOBEE_BRANCHING_ZONES=1` for the POC week).
- Playtest: forks behave, secrets gate properly, boss reachable via both paths.

### G8 — Migrate remaining zones one at a time

- Each zone gets its own `zone<Name>Graph()` function.
- Authoring guidelines (drop into `gogobee_dungeon_zones.md` §X):
  - Aim for one fork per zone for T1, two forks for T2+.
  - Secret rooms should have meaningful loot (LootBias ≥ 1.5) since they cost a Perception roll.
  - Locked paths should always have a hint (cruel design otherwise).
  - Boss must be reachable from entry without relying on luck (no Perception walls on the only path).
- Per-zone PR pattern:
  - Add `zoneXxxGraph()`.
  - Test: validator passes, all paths reach boss, no orphan nodes.
  - Flip `GOGOBEE_BRANCHING_ZONES=1` for that zone.
- Remove env gate when all zones converted.

### G9 — Retire linear model

- Drop `room_seq_json` and `current_room` columns once all zones have registered graphs and old runs are completed.
- Idempotent purge gate (mirror `purgeLegacyAdvCharacterTable` shape from L5):
```go
func purgeLegacyZoneRunColumns() {
    if os.Getenv("GOGOBEE_BRANCHING_PURGE") != "1" { return }
    // ALTER TABLE dnd_zone_run DROP COLUMN current_room
    // ALTER TABLE dnd_zone_run DROP COLUMN room_seq_json
}
```
- Wire into Init, gated, idempotent. Operator flips after final soak.

---

## 5. Testing strategy

- **Unit:** validator failure cases, edge unlock evaluation, fork prompt rendering, legacy-zone compiler shape.
- **Integration (in-process DB):** end-to-end run through a forked zone, including Perception-gated branch, secret room discovery, region transition.
- **Prod-DB simulation:** new test in `proddb_simulation_test.go` shape — load a real character, start a graph-mode zone run, exercise a fork choice, verify state persists across reload (overlay-layer parity).
- **Backward compat smoke:** test that a legacy zone (kept on the linear template) renders identically to pre-G7 output.

---

## 6. Risk inventory

| Risk | Mitigation |
|---|---|
| In-flight runs at deploy | G4 dual-write + warm-up populates `current_node` from `current_room` lazily. No run breakage. |
| Validator panics in prod | Run validator in CI; staging soak; `registerZoneGraph` panics at Init so bad authoring never reaches a player. |
| Fork-choice race (player sends `!zone go 1` while another DM is mid-fly) | Same lock mechanism existing zone advance uses (`p.advUserLock`). |
| `!zone map` render churn | Keep legacy renderer as fallback; switch by graph-presence flag during G6. |
| Player frustration with locked edges they can never see | Always emit `Hint` text on locked edges, even when the lock fails — "you sense something" teasers. |
| Regions integration drift | Keep `current_region` updates inside `advanceZoneRunNode`; zone-graph nodes carry `RegionID`, so transitions are derivable. |

---

## 7. Surface inventory (files likely to change)

- `internal/db/db.go` — schema, ALTER TABLEs (G1)
- `internal/plugin/zone_graph.go` — new (G2)
- `internal/plugin/dnd_zone.go` — register graphs alongside zones (G3+G7+G8)
- `internal/plugin/dnd_zone_run.go` (or wherever run state lives) — node-aware loaders/savers (G4)
- `internal/plugin/dnd_zone_cmd.go` — `!zone go`, fork rendering (G5)
- `internal/plugin/dnd_zone_combat.go` — entry-point shifts from room-idx to node-id (G5)
- `internal/plugin/dnd_zone_narration.go` — cadence reads from `visited_nodes` (G6)
- `internal/plugin/dnd_expedition_harvest.go` — harvest keying (G6)
- `internal/plugin/dnd_expedition_region.go` — region-on-node-transition (G6)
- `internal/plugin/dnd_zone_map.go` (or wherever `!zone map` renders) — graph render (G6)
- `gogobee_dungeon_zones.md` — authoring guidelines update (G8)

---

## 8. Open authoring questions (decide during G7)

1. **Backtracking** — opt-in per-edge or globally off? **Default proposed: off.**
2. **Secret-chamber discovery memory** — once found, persisted to character forever, or per-run? **Default proposed: per-run** (re-roll each entry, keeps replay tension).
3. **Fork timeout** — if player goes idle at a fork, default to first-listed edge after N hours? **Default proposed: yes, 6h, with a "TwinBee picked for you" DM.**
4. **Hidden node visibility on `!zone map`** — show as `?` glyph, hidden entirely, or revealed only after Perception success? **Default proposed: hidden until Perception success.**

---

## 9. First-session checklist

When the new session starts:

1. `git checkout -b branching-zones` off `adv-2.0`.
2. Read this file end-to-end.
3. Read `gogobee_dungeon_zones.md` (existing zone design) + `internal/plugin/dnd_zone.go` for current shape.
4. Run G1 (schema) + G2 (types) + G3 (legacy compiler) + G4 (run state). All four are infra; ship together with no behavior change.
5. Land G5 navigation behind `GOGOBEE_BRANCHING_ZONES=1` env gate.
6. POC G7 with Crypt of Valdris.
7. Pause for playtest before G8 mass migration.
