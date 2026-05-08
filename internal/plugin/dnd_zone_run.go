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
	GMMood        int
	StartedAt     time.Time
	LastActionAt  time.Time
	CompletedAt   *time.Time
}

// IsActive reports whether this run is still ongoing (not boss-defeated,
// not abandoned, not otherwise completed).
func (r *DungeonRun) IsActive() bool {
	return !r.BossDefeated && !r.Abandoned && r.CompletedAt == nil
}

// CurrentRoomType returns the type of the room the player is currently
// standing in (CurrentRoom is 0-indexed). Returns "" if out of range.
func (r *DungeonRun) CurrentRoomType() RoomType {
	if r.CurrentRoom < 0 || r.CurrentRoom >= len(r.RoomSeq) {
		return ""
	}
	return r.RoomSeq[r.CurrentRoom]
}

// generateRoomSequence builds the deterministic-but-seeded room layout
// for a run of the given zone. The boss room is always last; one Entry
// is always first; one Trap and one Elite room sit between explorations.
// Total length is sampled in [zone.MinRooms, zone.MaxRooms].
func generateRoomSequence(zone ZoneDefinition, rng *rand.Rand) []RoomType {
	total := zone.MinRooms
	if zone.MaxRooms > zone.MinRooms {
		total += rng.IntN(zone.MaxRooms - zone.MinRooms + 1)
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
	seqJSON, _ := json.Marshal(seq)

	run := &DungeonRun{
		RunID:        newRunID(),
		UserID:       string(userID),
		ZoneID:       zoneID,
		CurrentRoom:  0,
		TotalRooms:   len(seq),
		RoomSeq:      seq,
		RoomsCleared: []int{},
		GMMood:       50,
		StartedAt:    time.Now().UTC(),
		LastActionAt: time.Now().UTC(),
	}
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_zone_run
			(run_id, user_id, zone_id, current_room, total_rooms,
			 room_seq_json, rooms_cleared, gm_mood)
		VALUES (?, ?, ?, 0, ?, ?, '[]', 50)`,
		run.RunID, run.UserID, string(zoneID), run.TotalRooms, string(seqJSON),
	); err != nil {
		return nil, fmt.Errorf("insert zone run: %w", err)
	}
	return run, nil
}

// getActiveZoneRun returns the player's in-flight run, or (nil, nil) if none.
func getActiveZoneRun(userID id.UserID) (*DungeonRun, error) {
	row := db.Get().QueryRow(`
		SELECT run_id, user_id, zone_id, current_room, total_rooms,
		       room_seq_json, rooms_cleared, boss_defeated, abandoned,
		       loot_collected, gm_mood, started_at, last_action_at, completed_at
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
	return r, err
}

// getZoneRun fetches by RunID regardless of completion state. Test/admin use.
func getZoneRun(runID string) (*DungeonRun, error) {
	row := db.Get().QueryRow(`
		SELECT run_id, user_id, zone_id, current_room, total_rooms,
		       room_seq_json, rooms_cleared, boss_defeated, abandoned,
		       loot_collected, gm_mood, started_at, last_action_at, completed_at
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
		seqJSON        string
		clearedJSON    string
		lootJSON       string
		bossDefeatedI  int
		abandonedI     int
		completedAtRaw sql.NullTime
	)
	if err := row.Scan(
		&r.RunID, &r.UserID, &zoneID, &r.CurrentRoom, &r.TotalRooms,
		&seqJSON, &clearedJSON, &bossDefeatedI, &abandonedI,
		&lootJSON, &r.GMMood, &r.StartedAt, &r.LastActionAt, &completedAtRaw,
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
	if err := json.Unmarshal([]byte(seqJSON), &r.RoomSeq); err != nil {
		return nil, fmt.Errorf("decode room_seq_json: %w", err)
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
	return &r, nil
}

// markRoomCleared records that the current room has been resolved and
// advances the player to the next room. Returns the new current room
// type (or "" if the run completed via boss kill / final room).
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
	cleared := append(r.RoomsCleared, r.CurrentRoom)
	clearedJSON, _ := json.Marshal(cleared)

	wasBossRoom := r.CurrentRoomType() == RoomBoss
	next := r.CurrentRoom + 1
	if next >= r.TotalRooms || wasBossRoom {
		// Boss room cleared = run complete via boss defeat.
		now := time.Now().UTC()
		if _, err := db.Get().Exec(`
			UPDATE dnd_zone_run
			   SET rooms_cleared = ?,
			       boss_defeated = ?,
			       completed_at  = ?,
			       last_action_at = ?
			 WHERE run_id = ?`,
			string(clearedJSON), boolToInt(wasBossRoom), now, now, runID,
		); err != nil {
			return "", err
		}
		return "", nil
	}
	if _, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET rooms_cleared = ?,
		       current_room  = ?,
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE run_id = ?`,
		string(clearedJSON), next, runID,
	); err != nil {
		return "", err
	}
	r.CurrentRoom = next
	return r.RoomSeq[next], nil
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

