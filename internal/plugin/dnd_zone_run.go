package plugin

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 11 D1b — DungeonRun state machine. Implements `gogobee_dungeon_zones.md`
// §4 (Dungeon Structure) and the DungeonRun model in §7. Persists a single
// zone run per player to dnd_zone_run.
//
// Room sequencing follows the design doc fixed pattern:
//   Entry → Exploration ×N₁ → Trap → Exploration ×N₂ → Elite → Boss
// where N₁ + N₂ scales with zone tier so total rooms lands in the
// zone's [MinRooms, MaxRooms] window. Boss is always last.
//
// State machine:
//   start → advance × R → boss → complete
//                ↓
//             abandon (manual or 24h-idle, D1c)
//
// D1b ships persistence and pure logic only. !zone enter/advance/etc
// commands wire to this in D1c; combat resolution per room is wired in
// D1e onwards. Trap/elite/boss room *behavior* is not implemented yet —
// advancing through them currently just records them as cleared.

// RoomType enumerates the room categories per design doc §4.2.
type RoomType string

const (
	RoomEntry       RoomType = "entry"
	RoomExploration RoomType = "exploration"
	RoomTrap        RoomType = "trap"
	RoomElite       RoomType = "elite"
	RoomBoss        RoomType = "boss"
)

// DungeonRun is the in-memory shape of a dnd_zone_run row.
//
// Phase G4 adds CurrentNode / VisitedNodes / NodeChoices alongside the
// legacy CurrentRoom / RoomSeq fields. The graph columns dual-write
// during the migration; readers prefer CurrentNode but fall back to
// deriving it from CurrentRoom + RoomSeq (compileRunGraph) when a row
// predates the migration. The legacy fields retire in G9.
type DungeonRun struct {
	RunID         string
	UserID        string
	ZoneID        ZoneID
	CurrentRoom   int
	TotalRooms    int
	RoomSeq       []RoomType
	RoomsCleared  []int
	BossDefeated  bool
	Abandoned     bool
	LootCollected []string
	DMMood        int
	StartedAt     time.Time
	LastActionAt  time.Time
	CompletedAt   *time.Time

	// Phase G4 — branching zone graph run state. CurrentNode is the
	// authoritative position once the graph rollout is complete; until
	// then it dual-writes with CurrentRoom. VisitedNodes is the ordered
	// path of node_ids the player has resolved. NodeChoices stores
	// pending fork-prompt state (G5 surface) — populated when the player
	// arrives at a fork with 2+ unlocked outgoing edges.
	CurrentNode  string
	VisitedNodes []string
	NodeChoices  map[string]any

	// Revisit R1 — monotonic count of node entries, including the entry
	// room. Distinct from CurrentRoom: CurrentRoom answers "where am I on
	// the path" and moves backwards when the player backtracks, while
	// RoomsTraversed answers "how much walking has this run cost" and only
	// ever climbs. Forward-only navigation keeps them locked together at
	// RoomsTraversed == CurrentRoom+1; revisit is what pulls them apart.
	RoomsTraversed int
}

// IsActive reports whether this run is still ongoing (not boss-defeated,
// not abandoned, not otherwise completed).
func (r *DungeonRun) IsActive() bool {
	return !r.BossDefeated && !r.Abandoned && r.CompletedAt == nil
}

// CurrentRoomType returns the type of the room the player is currently
// standing in. The live CurrentNode's kind is authoritative — that's
// the only way side paths (e.g. the Crypt of Valdris secret chamber)
// resolve to the right room-type when the player diverges from the
// canonical RoomSeq. The legacy RoomSeq fallback covers in-flight runs
// from before the G4 dual-write deploy that lack a CurrentNode entry.
// Returns "" if no resolution is possible.
func (r *DungeonRun) CurrentRoomType() RoomType {
	if r.CurrentNode != "" {
		if g, ok := loadZoneGraph(r.ZoneID); ok {
			if n, exists := g.Nodes[r.CurrentNode]; exists {
				return nodeKindToRoomType(n.Kind)
			}
		}
	}
	if r.CurrentRoom < 0 || r.CurrentRoom >= len(r.RoomSeq) {
		return ""
	}
	return r.RoomSeq[r.CurrentRoom]
}

// RoomIsCleared reports whether the player has already resolved the room at
// the given path index. Sticky: once cleared, a room stays cleared for the
// life of the run, however many times the player walks back through it.
func (r *DungeonRun) RoomIsCleared(idx int) bool {
	for _, c := range r.RoomsCleared {
		if c == idx {
			return true
		}
	}
	return false
}

// generateRoomSequence builds the deterministic-but-seeded room layout
// for a run of the given zone. The boss room is always last; one Entry
// is always first; one Trap and one Elite room sit between explorations.
//
// Total length tracks the zone's *graph* longest entry→boss path so the
// "Room X/Y" display lines up with what the player actually walks. If
// the graph hasn't been authored / can't be loaded, we fall back to a
// dice roll within [zone.MinRooms, zone.MaxRooms] (the pre-graph shape).
func generateRoomSequence(zone ZoneDefinition, rng *rand.Rand) []RoomType {
	total := 0
	if g, ok := loadZoneGraph(zone.ID); ok {
		total = graphLongestPath(g)
	}
	if total == 0 {
		total = zone.MinRooms
		if zone.MaxRooms > zone.MinRooms {
			total += rng.IntN(zone.MaxRooms - zone.MinRooms + 1)
		}
	}
	// Fixed slots: Entry + Trap + Elite + Boss = 4. Remaining = explorations.
	const fixed = 4
	if total < fixed+2 {
		total = fixed + 2 // at least 2 exploration rooms
	}
	explorations := total - fixed
	// Split exploration rooms before/after the trap. Bias toward 1–2 on each side.
	preTrap := 1
	if explorations >= 3 {
		preTrap = 1 + rng.IntN(explorations-1)
	} else if explorations == 2 {
		preTrap = 1
	}
	postTrap := explorations - preTrap

	seq := make([]RoomType, 0, total)
	seq = append(seq, RoomEntry)
	for i := 0; i < preTrap; i++ {
		seq = append(seq, RoomExploration)
	}
	seq = append(seq, RoomTrap)
	for i := 0; i < postTrap; i++ {
		seq = append(seq, RoomExploration)
	}
	seq = append(seq, RoomElite)
	seq = append(seq, RoomBoss)
	return seq
}

// newRunID — 16-char hex token. Crypto-random; collision-resistant.
func newRunID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// Fall back to math/rand if /dev/urandom is unavailable.
		// Run IDs are not security-sensitive, just unique.
		v := rng2.Uint64()
		for i := range b {
			b[i] = byte(v >> (8 * i))
		}
	}
	return hex.EncodeToString(b[:])
}

// rng2 is a seeded math/rand fallback for newRunID. Test isolation isn't
// needed — collisions are still vanishingly unlikely.
var rng2 = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0xC0FFEE))

// ---- Persistence ------------------------------------------------------------

var (
	// ErrRunAlreadyActive — player tried to start a run while another is in flight.
	ErrRunAlreadyActive = errors.New("zone run already active for player")
	// ErrNoActiveRun — player tried to advance/retreat with no run in flight.
	ErrNoActiveRun = errors.New("no active zone run for player")
	// ErrUnknownZone — start called with an unregistered ZoneID.
	ErrUnknownZone = errors.New("unknown zone")
	// ErrZoneTierLocked — player level too low for the zone.
	ErrZoneTierLocked = errors.New("zone tier above player ceiling")
)

// startZoneRun creates a new run for the player. Fails if the player has
// an active run, the zone is unknown, or the player's level is too far
// below the zone's tier (per zonesForLevel's gate).
func startZoneRun(userID id.UserID, zoneID ZoneID, dndLevel int, rng *rand.Rand) (*DungeonRun, error) {
	zone, ok := getZone(zoneID)
	if !ok {
		return nil, ErrUnknownZone
	}
	allowed := false
	for _, z := range zonesForLevel(dndLevel) {
		if z.ID == zoneID {
			allowed = true
			break
		}
	}
	// Tier 6 zones are excluded from zonesForLevel by design; they're
	// admitted here only through the post-game unlock (both T5 bosses beaten
	// + level ≥ floor). This is the shared engine safety net — command sites
	// give the friendly reason before reaching here.
	if isPostgameZone(zoneID) {
		if ok, _ := postgameUnlocked(userID, dndLevel); ok {
			allowed = true
		}
	}
	if !allowed {
		return nil, ErrZoneTierLocked
	}
	existing, err := getActiveZoneRun(userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrRunAlreadyActive
	}

	if rng == nil {
		rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixMicro())))
	}
	seq := generateRoomSequence(zone, rng)

	startMood := 50
	if isHol, _ := isHolidayToday(); isHol {
		startMood = 55
	}
	run := &DungeonRun{
		RunID:        newRunID(),
		UserID:       string(userID),
		ZoneID:       zoneID,
		CurrentRoom:  0,
		TotalRooms:   len(seq),
		RoomSeq:      seq,
		RoomsCleared: []int{},
		DMMood:       startMood,
		StartedAt:    time.Now().UTC(),
		LastActionAt: time.Now().UTC(),
	}
	// G4 dual-write: persist the entry node id and seed visited_nodes
	// with it, so navigation surfaces in G5 can read graph state without
	// further migration. New runs always start at the registered graph's
	// Entry node; only zones that lack a registered graph (none, post-G8)
	// would fall back to the legacy linear `<zone>.r1` namespace.
	entryNode := deriveLegacyNodeID(zoneID, 0)
	if g, ok := zoneGraphRegistry[zoneID]; ok {
		entryNode = g.Entry
	}
	visitedJSON, _ := json.Marshal([]string{entryNode})
	run.CurrentNode = entryNode
	run.VisitedNodes = []string{entryNode}
	run.NodeChoices = map[string]any{}
	// Standing in the entry room is one traversal — the player walked in.
	// Starting at 1 also keeps `rooms_traversed = 0` free as the
	// "never backfilled" sentinel bootstrapRoomsTraversed keys on.
	run.RoomsTraversed = 1
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_zone_run
			(run_id, user_id, zone_id, total_rooms,
			 rooms_cleared, gm_mood,
			 current_node, visited_nodes, node_choices, rooms_traversed)
		VALUES (?, ?, ?, ?, '[]', ?, ?, ?, '{}', 1)`,
		run.RunID, run.UserID, string(zoneID), run.TotalRooms, startMood,
		entryNode, string(visitedJSON),
	); err != nil {
		return nil, fmt.Errorf("insert zone run: %w", err)
	}
	return run, nil
}

// zoneRunInactivityTimeout is the §4.3 stale-run threshold: a run that
// has gone untouched for this long is auto-abandoned the next time
// anyone looks at it.
const zoneRunInactivityTimeout = 24 * time.Hour

// getActiveZoneRun returns the player's in-flight run, or (nil, nil) if
// none. If the most-recent active run has been idle longer than
// zoneRunInactivityTimeout, it's auto-abandoned (§4.3) and the
// function returns (nil, nil) — the caller sees a clean slate.
func getActiveZoneRun(userID id.UserID) (*DungeonRun, error) {
	row := db.Get().QueryRow(`
		SELECT run_id, user_id, zone_id, total_rooms,
		       rooms_cleared, boss_defeated, abandoned,
		       loot_collected, gm_mood, started_at, last_action_at, completed_at,
		       current_node, visited_nodes, node_choices, rooms_traversed
		  FROM dnd_zone_run
		 WHERE user_id = ?
		   AND completed_at IS NULL
		   AND abandoned = 0
		   AND boss_defeated = 0
		 ORDER BY started_at DESC
		 LIMIT 1`,
		string(userID))
	r, err := scanZoneRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if time.Since(r.LastActionAt) > zoneRunInactivityTimeout {
		_ = abandonZoneRunByID(r.RunID)
		// A run reaped by the §4.3 idle timeout must also terminate the
		// wrapping active expedition. Without this, the expedition is left
		// status='active' pointing at a now-abandoned run: the autopilot's
		// runAutopilotWalk reads run==nil and bails, but the briefing/recap
		// ambient tickers keep firing — the player soft-locks at the last
		// fork, "stuck" with no way to route on. Mirror the run-loss seam,
		// but only when this run is the active expedition's current run so
		// a standalone (non-expedition) stale run still reaps cleanly.
		if exp, _ := getActiveExpedition(userID); exp != nil && exp.RunID == r.RunID {
			forceExtractExpeditionForRunLoss(userID, lossIdleTimeout)
		}
		return nil, nil
	}
	return r, nil
}

// getZoneRun fetches by RunID regardless of completion state. Test/admin use.
func getZoneRun(runID string) (*DungeonRun, error) {
	row := db.Get().QueryRow(`
		SELECT run_id, user_id, zone_id, total_rooms,
		       rooms_cleared, boss_defeated, abandoned,
		       loot_collected, gm_mood, started_at, last_action_at, completed_at,
		       current_node, visited_nodes, node_choices, rooms_traversed
		  FROM dnd_zone_run WHERE run_id = ?`, runID)
	r, err := scanZoneRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// scanZoneRun reads one row into a DungeonRun (used by both fetchers).
type scanner interface {
	Scan(dest ...any) error
}

func scanZoneRun(row scanner) (*DungeonRun, error) {
	var (
		r              DungeonRun
		zoneID         string
		clearedJSON    string
		lootJSON       string
		bossDefeatedI  int
		abandonedI     int
		completedAtRaw sql.NullTime
		currentNode    string
		visitedJSON    string
		choicesJSON    string
	)
	if err := row.Scan(
		&r.RunID, &r.UserID, &zoneID, &r.TotalRooms,
		&clearedJSON, &bossDefeatedI, &abandonedI,
		&lootJSON, &r.DMMood, &r.StartedAt, &r.LastActionAt, &completedAtRaw,
		&currentNode, &visitedJSON, &choicesJSON, &r.RoomsTraversed,
	); err != nil {
		return nil, err
	}
	r.ZoneID = ZoneID(zoneID)
	r.BossDefeated = bossDefeatedI != 0
	r.Abandoned = abandonedI != 0
	if completedAtRaw.Valid {
		t := completedAtRaw.Time
		r.CompletedAt = &t
	}
	if clearedJSON != "" {
		if err := json.Unmarshal([]byte(clearedJSON), &r.RoomsCleared); err != nil {
			return nil, fmt.Errorf("decode rooms_cleared: %w", err)
		}
	}
	if r.RoomsCleared == nil {
		r.RoomsCleared = []int{}
	}
	if lootJSON != "" {
		if err := json.Unmarshal([]byte(lootJSON), &r.LootCollected); err != nil {
			return nil, fmt.Errorf("decode loot_collected: %w", err)
		}
	}
	if r.LootCollected == nil {
		r.LootCollected = []string{}
	}
	// G4 graph run state. visited_nodes / node_choices come straight
	// from JSON; current_node is hot-swap-derived from current_room
	// when empty (rows that predate the migration).
	if visitedJSON != "" && visitedJSON != "[]" {
		if err := json.Unmarshal([]byte(visitedJSON), &r.VisitedNodes); err != nil {
			return nil, fmt.Errorf("decode visited_nodes: %w", err)
		}
	}
	if r.VisitedNodes == nil {
		r.VisitedNodes = []string{}
	}
	if choicesJSON != "" && choicesJSON != "{}" {
		if err := json.Unmarshal([]byte(choicesJSON), &r.NodeChoices); err != nil {
			return nil, fmt.Errorf("decode node_choices: %w", err)
		}
	}
	if r.NodeChoices == nil {
		r.NodeChoices = map[string]any{}
	}
	r.CurrentNode = currentNode
	// G9b: CurrentRoom is derived from VisitedNodes — no longer persisted
	// in current_room. Revisit R1 re-derives it as the *first-entry index
	// of CurrentNode* rather than len(VisitedNodes)-1. The two agree for
	// every forward-only run (each advance appends the node it moves to,
	// so the newest node is always the current one), but they diverge the
	// moment a player backtracks. Keying on the node — not the tail — is
	// what makes a revisited room resolve to its original identity: the
	// enemy/trap salts, harvest keys and encounter IDs downstream all
	// hash CurrentRoom, so room 3 must stay room 3 on the way back.
	r.CurrentRoom = pathIndexOf(r.VisitedNodes, r.CurrentNode)
	if r.CurrentNode == "" {
		// Defensive: a row from before the G4 dual-write deploy that
		// somehow survived 24h inactivity timeouts. Pin to the linear
		// entry node so resolveRoom doesn't crash.
		r.CurrentNode = deriveLegacyNodeID(r.ZoneID, r.CurrentRoom)
	}
	return &r, nil
}

// appendVisited records a node entry in first-entry order. A node already
// in the path is not re-appended: VisitedNodes is an ordered *set*, and the
// player's room numbers must not renumber themselves when they walk back
// through a room they've already seen. The step cost of that walk is
// carried by rooms_traversed, not by this slice.
func appendVisited(visited []string, node string) []string {
	for _, n := range visited {
		if n == node {
			return visited
		}
	}
	return append(visited, node)
}

// appendClearedRoom marks a room index resolved. Idempotent: walking out of
// a room a second time doesn't re-append. "Cleared" is a sticky property of
// the room, not a count of exits — re-appending would let a backtracking
// player inflate RoomsCleared past TotalRooms and skew every "N/M rooms"
// render that reads its length.
func appendClearedRoom(cleared []int, room int) []int {
	for _, r := range cleared {
		if r == room {
			return cleared
		}
	}
	return append(cleared, room)
}

// pathIndexOf returns the first-entry index of node within visited — the
// 0-based room number the player sees in `!map`'s Path strip. Revisits do
// not append, so a node's index is stable for the life of the run.
//
// A node missing from visited means a row whose current_node was hot-swapped
// in without a matching visit record (pre-G4 rows, defensive only). Fall back
// to the tail, which is what the pre-R1 derivation would have produced.
func pathIndexOf(visited []string, node string) int {
	for i, n := range visited {
		if n == node {
			return i
		}
	}
	if n := len(visited); n > 0 {
		return n - 1
	}
	return 0
}

// deriveLegacyNodeID returns the node id a linear graph would assign
// to position roomIdx (0-based) for the given zone. Mirrors
// BuildLinearGraph's "<zone>.r<n>" scheme so hot-swapped rows align
// with newly-started runs.
func deriveLegacyNodeID(zoneID ZoneID, roomIdx int) string {
	if roomIdx < 0 {
		roomIdx = 0
	}
	return fmt.Sprintf("%s.r%d", zoneID, roomIdx+1)
}

// markRoomCleared records that the current room has been resolved and
// advances the player along the first outgoing graph edge. Returns the
// new current room type (or "" if the run completed via boss kill /
// dead-end). Used by tests that drive a run end-to-end without going
// through the !zone advance command surface; the runtime command path
// uses advanceTransitionGraph directly so it can render fork prompts.
func markRoomCleared(runID string) (RoomType, error) {
	r, err := getZoneRun(runID)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", ErrNoActiveRun
	}
	if !r.IsActive() {
		return "", ErrNoActiveRun
	}
	g, ok := loadZoneGraph(r.ZoneID)
	if !ok {
		return "", fmt.Errorf("no graph for zone %q", r.ZoneID)
	}
	cleared := appendClearedRoom(r.RoomsCleared, r.CurrentRoom)
	clearedJSON, _ := json.Marshal(cleared)

	edges := g.outgoingEdges(r.CurrentNode)
	if len(edges) == 0 {
		// Dead-end or boss — run completes here.
		isBoss := g.Nodes[r.CurrentNode].IsBoss
		now := time.Now().UTC()
		if _, err := db.Get().Exec(`
			UPDATE dnd_zone_run
			   SET rooms_cleared = ?,
			       boss_defeated = ?,
			       completed_at  = ?,
			       last_action_at = ?
			 WHERE run_id = ?`,
			string(clearedJSON), boolToInt(isBoss), now, now, runID,
		); err != nil {
			return "", err
		}
		return "", nil
	}
	nextNode := edges[0].To
	visited := appendVisited(r.VisitedNodes, nextNode)
	visitedJSON, _ := json.Marshal(visited)
	if _, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET rooms_cleared = ?,
		       current_node  = ?,
		       visited_nodes = ?,
		       rooms_traversed = rooms_traversed + 1,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`,
		string(clearedJSON), nextNode, string(visitedJSON), runID,
	); err != nil {
		return "", err
	}
	r.CurrentNode = nextNode
	r.VisitedNodes = visited
	r.RoomsTraversed++
	r.CurrentRoom = pathIndexOf(visited, nextNode)
	return nodeKindToRoomType(g.Nodes[nextNode].Kind), nil
}

// abandonZoneRun flags the active run as abandoned. Idempotent: returns
// ErrNoActiveRun if there is nothing to abandon.
func abandonZoneRun(userID id.UserID) error {
	r, err := getActiveZoneRun(userID)
	if err != nil {
		return err
	}
	if r == nil {
		return ErrNoActiveRun
	}
	_, err = db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET abandoned = 1,
		       completed_at = CURRENT_TIMESTAMP,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, r.RunID)
	return err
}

// abandonZoneRunByID abandons a specific run regardless of its active
// status. Idempotent — exits cleanly if the row is already terminal.
// Used by the expedition layer to retire a region's run when the player
// travels onward, since the user-keyed abandonZoneRun would refuse to
// fire when the run is no longer "active" (e.g. boss defeated).
func abandonZoneRunByID(runID string) error {
	if runID == "" {
		return nil
	}
	_, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET abandoned = 1,
		       completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, runID)
	return err
}

// adjustGMMood clamps mood to [0, 100] and persists. Used by D1d when
// nat-1/nat-20/zone-completion events fire. delta may be negative.
func adjustGMMood(runID string, delta int) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET gm_mood = MAX(0, MIN(100, gm_mood + ?)),
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, delta, runID)
	return err
}

// addLoot appends an item ID to the run's loot manifest. Caller is
// responsible for actually granting the item to the player's inventory;
// this is the audit trail of what dropped during the run.
func addLoot(runID string, itemID string) error {
	r, err := getZoneRun(runID)
	if err != nil {
		return err
	}
	if r == nil {
		return ErrNoActiveRun
	}
	loot := append(r.LootCollected, itemID)
	lootJSON, _ := json.Marshal(loot)
	_, err = db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET loot_collected = ?,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`, string(lootJSON), runID)
	return err
}
