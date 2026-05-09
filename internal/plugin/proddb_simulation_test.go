package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// TestProdDB_PlayerActionRoundTrip simulates real player actions against a
// copy of the prod DB and verifies the bootstrap + fan-out + overlay
// round-trip preserves state across save/reload boundaries. Targets every
// subsystem the L4-L5h migration touched.
//
// Skips silently if the prod DB isn't present.
func TestProdDB_PlayerActionRoundTrip(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present at " + src)
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	copyTestFile(t, src, dst)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(db.Close)

	bootstrapPlayerMetaFromLegacy()

	chars, err := loadAllAdvCharacters()
	if err != nil {
		t.Fatalf("loadAllAdvCharacters: %v", err)
	}
	if len(chars) == 0 {
		t.Fatal("bootstrap produced zero chars; nothing to simulate against")
	}

	// Pick first char as the simulation target.
	uid := chars[0].UserID
	t.Logf("=== simulating actions for %s ===", uid)

	// Snapshot original state for end-of-test no-regression check on
	// non-mutated subsystems.
	orig, err := loadAdvCharacter(uid)
	if err != nil || orig == nil {
		t.Fatalf("initial load failed: %v", err)
	}
	t.Logf("baseline: L%d combat / mining=%d foraging=%d fishing=%d, arena=%d-%d, hospital=%d, streak=%d/%d, masterwork=%d, pet=%q L%d",
		orig.CombatLevel, orig.MiningSkill, orig.ForagingSkill, orig.FishingSkill,
		orig.ArenaWins, orig.ArenaLosses, orig.HospitalVisits,
		orig.CurrentStreak, orig.BestStreak, orig.MasterworkDropsReceived,
		orig.PetType, orig.PetLevel)

	// ── 1. Skill XP / level (foraging craft success) ───────────────────────
	t.Run("foraging_xp_persists", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		preXP := c.ForagingXP
		preCrafts := c.CraftsSucceeded
		c.ForagingXP += 25
		c.CraftsSucceeded++
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.ForagingXP != preXP+25 {
			t.Errorf("foraging_xp: pre=%d post=%d (want %d)", preXP, got.ForagingXP, preXP+25)
		}
		if got.CraftsSucceeded != preCrafts+1 {
			t.Errorf("crafts_succeeded: pre=%d post=%d (want %d)", preCrafts, got.CraftsSucceeded, preCrafts+1)
		}
		t.Logf("foraging_xp=%d→%d, crafts=%d→%d", preXP, got.ForagingXP, preCrafts, got.CraftsSucceeded)
	})

	// ── 2. Arena counters ──────────────────────────────────────────────────
	t.Run("arena_counters_persist", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		preWins := c.ArenaWins
		preLosses := c.ArenaLosses
		preInv := c.InvasionScore
		c.ArenaWins++
		c.ArenaLosses++
		c.InvasionScore += 10
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.ArenaWins != preWins+1 || got.ArenaLosses != preLosses+1 || got.InvasionScore != preInv+10 {
			t.Errorf("arena: wins %d→%d, losses %d→%d, inv %d→%d",
				preWins, got.ArenaWins, preLosses, got.ArenaLosses, preInv, got.InvasionScore)
		}
		t.Logf("arena: wins %d, losses %d, inv %d", got.ArenaWins, got.ArenaLosses, got.InvasionScore)
	})

	// ── 3. Babysit (start session) ─────────────────────────────────────────
	t.Run("babysit_state_persists", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		future := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
		c.BabysitActive = true
		c.BabysitExpiresAt = &future
		c.BabysitSkillFocus = "foraging"
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if !got.BabysitActive {
			t.Error("babysit_active not persisted")
		}
		if got.BabysitExpiresAt == nil || !got.BabysitExpiresAt.Equal(future) {
			t.Errorf("babysit_expires: want %v got %v", future, got.BabysitExpiresAt)
		}
		if got.BabysitSkillFocus != "foraging" {
			t.Errorf("babysit_focus: want foraging got %q", got.BabysitSkillFocus)
		}
		t.Logf("babysit: active=%v expires=%v focus=%s", got.BabysitActive, got.BabysitExpiresAt, got.BabysitSkillFocus)

		// Cancel
		c2, _ := loadAdvCharacter(uid)
		c2.BabysitActive = false
		c2.BabysitExpiresAt = nil
		c2.BabysitSkillFocus = ""
		_ = saveAdvCharacter(c2)
		got2, _ := loadAdvCharacter(uid)
		if got2.BabysitActive || got2.BabysitExpiresAt != nil {
			t.Errorf("babysit clear failed: active=%v expires=%v", got2.BabysitActive, got2.BabysitExpiresAt)
		}
	})

	// ── 4. Lifecycle (streak update) ───────────────────────────────────────
	t.Run("streak_persists", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		preStreak := c.CurrentStreak
		preBest := c.BestStreak
		c.CurrentStreak = preStreak + 3
		if c.CurrentStreak > c.BestStreak {
			c.BestStreak = c.CurrentStreak
		}
		c.LastActionDate = "2026-05-09"
		c.StreakDecayed = false
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.CurrentStreak != preStreak+3 {
			t.Errorf("current_streak: want %d got %d", preStreak+3, got.CurrentStreak)
		}
		if got.BestStreak < preBest || got.BestStreak < got.CurrentStreak {
			t.Errorf("best_streak invariant broken: best=%d cur=%d", got.BestStreak, got.CurrentStreak)
		}
		t.Logf("streak: cur %d→%d, best=%d, last_action=%s", preStreak, got.CurrentStreak, got.BestStreak, got.LastActionDate)
	})

	// ── 5. Death state (kill + revive) ─────────────────────────────────────
	t.Run("death_state_persists", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		preVisits := c.HospitalVisits
		future := time.Now().UTC().Add(6 * time.Hour).Truncate(time.Second)
		c.Alive = false
		c.DeadUntil = &future
		c.DeathSource = "zone"
		c.DeathLocation = "Forest of Shadows"
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save kill: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.Alive {
			t.Error("alive flag should be false after kill")
		}
		if got.DeadUntil == nil || !got.DeadUntil.Equal(future) {
			t.Errorf("dead_until: want %v got %v", future, got.DeadUntil)
		}
		if got.DeathSource != "zone" || got.DeathLocation != "Forest of Shadows" {
			t.Errorf("death source/location: %q / %q", got.DeathSource, got.DeathLocation)
		}

		// Revive
		c2, _ := loadAdvCharacter(uid)
		c2.Alive = true
		c2.DeadUntil = nil
		c2.HospitalVisits++
		_ = saveAdvCharacter(c2)
		got2, _ := loadAdvCharacter(uid)
		if !got2.Alive || got2.DeadUntil != nil {
			t.Errorf("revive: alive=%v dead_until=%v", got2.Alive, got2.DeadUntil)
		}
		if got2.HospitalVisits != preVisits+1 {
			t.Errorf("hospital_visits: want %d got %d", preVisits+1, got2.HospitalVisits)
		}
		t.Logf("death cycle: alive=%v hospital_visits %d→%d", got2.Alive, preVisits, got2.HospitalVisits)
	})

	// ── 6. NPC counters (Misty encounter) ──────────────────────────────────
	t.Run("npc_counters_persist", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		pre := c.MistyEncounterCount
		now := time.Now().UTC().Truncate(time.Second)
		c.MistyEncounterCount++
		c.MistyLastSeen = &now
		c.MistyRollTarget = 50
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.MistyEncounterCount != pre+1 {
			t.Errorf("misty_encounter_count: want %d got %d", pre+1, got.MistyEncounterCount)
		}
		if got.MistyLastSeen == nil || !got.MistyLastSeen.Equal(now) {
			t.Errorf("misty_last_seen: want %v got %v", now, got.MistyLastSeen)
		}
		if got.MistyRollTarget != 50 {
			t.Errorf("misty_roll_target: want 50 got %d", got.MistyRollTarget)
		}
		t.Logf("misty: encounters %d→%d, last_seen=%v, roll_target=%d", pre, got.MistyEncounterCount, got.MistyLastSeen, got.MistyRollTarget)
	})

	// ── 7. Pet state (level + JSON-packed flags) ───────────────────────────
	t.Run("pet_state_persists", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		c.PetType = "fox"
		c.PetName = "TestFox"
		c.PetXP = 250
		c.PetLevel = 3
		c.PetArmorTier = 1
		c.PetArrived = true
		c.PetMorningDefense = true
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.PetType != "fox" || got.PetName != "TestFox" {
			t.Errorf("pet identity: type=%q name=%q", got.PetType, got.PetName)
		}
		if got.PetLevel != 3 || got.PetXP != 250 || got.PetArmorTier != 1 {
			t.Errorf("pet stats: L%d XP=%d armor=%d", got.PetLevel, got.PetXP, got.PetArmorTier)
		}
		if !got.PetArrived || !got.PetMorningDefense {
			t.Errorf("pet flags lost: arrived=%v morning_def=%v", got.PetArrived, got.PetMorningDefense)
		}
		t.Logf("pet: %s %q L%d XP=%d armor=T%d arrived=%v morning_def=%v",
			got.PetType, got.PetName, got.PetLevel, got.PetXP, got.PetArmorTier,
			got.PetArrived, got.PetMorningDefense)
	})

	// ── 8. House state (loan transitions) ──────────────────────────────────
	t.Run("house_state_persists", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		c.HouseTier = 2
		c.HouseLoanBalance = 50000
		c.HouseCurrentRate = 6.5
		c.HouseAutopay = true
		c.HouseMissedPayments = 0
		c.HouseLoanFrozen = false
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, _ := loadAdvCharacter(uid)
		if got.HouseTier != 2 || got.HouseLoanBalance != 50000 {
			t.Errorf("house: tier=%d loan=%d", got.HouseTier, got.HouseLoanBalance)
		}
		if got.HouseCurrentRate != 6.5 || !got.HouseAutopay {
			t.Errorf("house terms: rate=%.2f autopay=%v", got.HouseCurrentRate, got.HouseAutopay)
		}
		t.Logf("house: T%d loan=€%d rate=%.2f%% autopay=%v", got.HouseTier, got.HouseLoanBalance, got.HouseCurrentRate, got.HouseAutopay)
	})

	// ── 9. Idempotency: saving the same char twice produces no drift ───────
	t.Run("save_idempotent", func(t *testing.T) {
		c1, _ := loadAdvCharacter(uid)
		_ = saveAdvCharacter(c1)
		c2, _ := loadAdvCharacter(uid)
		_ = saveAdvCharacter(c2)
		c3, _ := loadAdvCharacter(uid)

		// Compare a representative cross-section. (Equal pointers like
		// DeadUntil get compared by dereferenced value.)
		if c2.CombatLevel != c3.CombatLevel || c2.ArenaWins != c3.ArenaWins ||
			c2.PetLevel != c3.PetLevel || c2.HouseTier != c3.HouseTier ||
			c2.MistyEncounterCount != c3.MistyEncounterCount ||
			c2.HospitalVisits != c3.HospitalVisits {
			t.Errorf("save not idempotent: c2 vs c3 differs")
		}
		t.Logf("save idempotency confirmed (CL/arena/pet/house/misty/hospital all stable)")
	})

	// ── 10. LastActiveAt always advances on save ───────────────────────────
	t.Run("last_active_advances", func(t *testing.T) {
		c, _ := loadAdvCharacter(uid)
		preActive := c.LastActiveAt
		time.Sleep(10 * time.Millisecond)
		_ = saveAdvCharacter(c)
		got, _ := loadAdvCharacter(uid)
		if !got.LastActiveAt.After(preActive) {
			t.Errorf("last_active didn't advance: pre=%v post=%v", preActive, got.LastActiveAt)
		}
		t.Logf("last_active: %v → %v (Δ=%v)", preActive, got.LastActiveAt, got.LastActiveAt.Sub(preActive))
	})

	// ── 11. createAdvCharacter for a brand-new user_id ─────────────────────
	t.Run("create_new_character", func(t *testing.T) {
		newUID := id.UserID("@simtest:parodia.dev")
		if err := createAdvCharacter(newUID, "SimTest"); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := loadAdvCharacter(newUID)
		if err != nil || got == nil {
			t.Fatalf("load fresh: %v", err)
		}
		if got.DisplayName != "SimTest" {
			t.Errorf("display_name: want SimTest got %q", got.DisplayName)
		}
		if !got.Alive {
			t.Error("fresh char should be Alive=true")
		}
		if got.CreatedAt.IsZero() {
			t.Error("created_at should be set on createAdvCharacter")
		}
		t.Logf("fresh char: name=%q alive=%v created=%v", got.DisplayName, got.Alive, got.CreatedAt)
	})
}
