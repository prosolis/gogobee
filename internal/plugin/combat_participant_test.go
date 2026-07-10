package plugin

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// setupEmptyTestDB opens a fresh schema in a temp dir. Unlike setupZoneRunTestDB
// it copies nothing from data/gogobee.db, so it runs everywhere instead of
// skipping when the prod db is absent — the session/participant/party tables are
// the only ones these tests touch, and none of them need seeded characters.
func setupEmptyTestDB(t *testing.T) {
	t.Helper()
	db.Close()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// ── The wire format ─────────────────────────────────────────────────────────

// The ActorStatuses split moved ~25 fields behind an anonymous embed. That is a
// no-op on the wire only because the embed is untagged: encoding/json flattens
// it into the parent object. If someone ever gives it a json tag, every
// in-flight prod session silently loses its poison, its charges, and its
// once-per-fight one-shots on the next resume. Pin the flattening.
func TestCombatStatuses_JSONStaysFlat(t *testing.T) {
	s := CombatStatuses{
		ActorStatuses: ActorStatuses{PoisonTicks: 2, PoisonDmg: 5, WardCharges: 3},
		Enraged:       true,
		EnemyRegen:    4,
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"poison_ticks", "poison_dmg", "ward_charges", "enraged", "enemy_regen"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("key %q missing — ActorStatuses is nested, not flattened: %s", key, raw)
		}
	}
	if _, nested := obj["ActorStatuses"]; nested {
		t.Errorf("ActorStatuses serialized as a nested object: %s", raw)
	}
}

// A statuses_json blob written before the split must decode unchanged.
func TestCombatStatuses_DecodesPreSplitRow(t *testing.T) {
	const preSplit = `{"poison_ticks":3,"poison_dmg":6,"enraged":true,` +
		`"ward_charges":2,"lucky_used":true,"enemy_retaliate_frac":0.25,` +
		`"buff_ac_bonus":4,"max_hp_drain":7}`
	var s CombatStatuses
	if err := json.Unmarshal([]byte(preSplit), &s); err != nil {
		t.Fatal(err)
	}
	if s.PoisonTicks != 3 || s.PoisonDmg != 6 || s.WardCharges != 2 ||
		s.LuckyUsed != true || s.BuffACBonus != 4 || s.MaxHPDrain != 7 {
		t.Errorf("per-actor fields lost: %+v", s.ActorStatuses)
	}
	if !s.Enraged || s.EnemyRetaliateFrac != 0.25 {
		t.Errorf("fight-scoped fields lost: %+v", s)
	}
}

// ── Solo stays solo ─────────────────────────────────────────────────────────

// The whole balance corpus rides the solo path. It must write no participant
// rows, and its roster_size must stay at the DEFAULT so the loader never issues
// the second query (project_combat_session_cache_deferred: don't make it worse).
func TestSoloSessionWritesNoParticipantRows(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@solo-noparts:example.org")
	defer cleanupCombatSessions(uid)

	s, err := startCombatSession(uid, "run-solo", "node-1", "owlbear", 40, 40, 60, 60)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCombatSession(s); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM combat_participant WHERE session_id = ?`, s.SessionID,
	).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("solo fight wrote %d participant rows, want 0", rows)
	}

	var rosterSize int
	if err := db.Get().QueryRow(
		`SELECT roster_size FROM combat_session WHERE session_id = ?`, s.SessionID,
	).Scan(&rosterSize); err != nil {
		t.Fatal(err)
	}
	if rosterSize != 1 {
		t.Errorf("roster_size = %d, want 1", rosterSize)
	}

	got, err := getActiveCombatSession(uid)
	if err != nil || got == nil {
		t.Fatalf("getActiveCombatSession: %v / %v", got, err)
	}
	if got.IsParty() || got.RosterSize() != 1 || len(got.Participants) != 0 {
		t.Errorf("solo session reads back as a party: %+v", got.Participants)
	}
}

// ── Party seats ─────────────────────────────────────────────────────────────

func seatParty(t *testing.T, s *CombatSession, members ...CombatParticipant) {
	t.Helper()
	if err := insertCombatParticipants(s.SessionID, members); err != nil {
		t.Fatalf("insertCombatParticipants: %v", err)
	}
	s.Participants = members
}

func TestCombatParticipants_RoundTrip(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@party-lead:example.org")
	defer cleanupCombatSessions(uid)

	s, err := startCombatSession(uid, "run-party", "node-1", "owlbear", 40, 40, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	seatParty(t, s,
		CombatParticipant{Seat: 1, UserID: "@b:x", HP: 30, HPMax: 30,
			Statuses: ActorStatuses{WardCharges: 2}},
		CombatParticipant{Seat: 2, UserID: "@c:x", HP: 25, HPMax: 25,
			Statuses: ActorStatuses{ConcentrationDmg: 9, LuckyUsed: true}},
	)

	got, err := getActiveCombatSession(uid)
	if err != nil || got == nil {
		t.Fatalf("getActiveCombatSession: %v / %v", got, err)
	}
	if !got.IsParty() || got.RosterSize() != 3 {
		t.Fatalf("roster = %d, want 3", got.RosterSize())
	}
	if got.Participants[0].UserID != "@b:x" || got.Participants[0].Statuses.WardCharges != 2 {
		t.Errorf("seat 1 round-trip wrong: %+v", got.Participants[0])
	}
	if got.Participants[1].Statuses.ConcentrationDmg != 9 || !got.Participants[1].Statuses.LuckyUsed {
		t.Errorf("seat 2 round-trip wrong: %+v", got.Participants[1])
	}
	if want := []string{string(uid), "@b:x", "@c:x"}; !equalStrings(got.SeatUserIDs(), want) {
		t.Errorf("SeatUserIDs = %v, want %v", got.SeatUserIDs(), want)
	}

	// Mutable half writes back; hp_max and user_id do not move.
	got.Participants[0].HP = 11
	got.Participants[0].Statuses.WardCharges = 0
	got.Participants[0].Statuses.Raged = true
	if err := saveCombatSession(got); err != nil {
		t.Fatal(err)
	}
	again, err := getCombatSession(s.SessionID)
	if err != nil || again == nil {
		t.Fatalf("getCombatSession: %v / %v", again, err)
	}
	p := again.Participants[0]
	if p.HP != 11 || p.HPMax != 30 || p.Statuses.WardCharges != 0 || !p.Statuses.Raged {
		t.Errorf("seat 1 save round-trip wrong: %+v", p)
	}
}

// A gap in the seat sequence would shift every member down one index, because
// the engine addresses the roster positionally. Fail the load instead.
func TestLoadCombatParticipants_RejectsSeatGap(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@party-gap:example.org")
	defer cleanupCombatSessions(uid)

	s, err := startCombatSession(uid, "run-gap", "node-1", "owlbear", 40, 40, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	// Seat 2 with no seat 1.
	if err := insertCombatParticipants(s.SessionID, []CombatParticipant{
		{Seat: 2, UserID: "@c:x", HP: 25, HPMax: 25},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCombatParticipants(s.SessionID); err == nil {
		t.Error("loadCombatParticipants accepted a seat gap")
	}
}

// roster_size disagreeing with the persisted seats means a half-written fight.
// Reading it as solo would silently drop the party mid-combat.
func TestHydrateCombatParticipants_RejectsRosterMismatch(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@party-mismatch:example.org")
	defer cleanupCombatSessions(uid)

	s, err := startCombatSession(uid, "run-mm", "node-1", "owlbear", 40, 40, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	seatParty(t, s, CombatParticipant{Seat: 1, UserID: "@b:x", HP: 30, HPMax: 30})
	if _, err := db.Get().Exec(
		`UPDATE combat_session SET roster_size = 3 WHERE session_id = ?`, s.SessionID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := getActiveCombatSession(uid); err == nil {
		t.Error("hydrate accepted roster_size 3 with 2 seats persisted")
	}
}

func TestGetActiveCombatSessionForMember(t *testing.T) {
	setupEmptyTestDB(t)
	lead := id.UserID("@lead-lookup:example.org")
	member := id.UserID("@member-lookup:example.org")
	defer cleanupCombatSessions(lead)

	s, err := startCombatSession(lead, "run-lookup", "node-1", "owlbear", 40, 40, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	seatParty(t, s, CombatParticipant{Seat: 1, UserID: string(member), HP: 30, HPMax: 30})

	// The member owns no session row, so the seat-0 lookup must miss them...
	if got, err := getActiveCombatSession(member); err != nil || got != nil {
		t.Errorf("getActiveCombatSession found a row for a party member: %v / %v", got, err)
	}
	// ...and the member lookup must find the fight, fully hydrated.
	got, err := getActiveCombatSessionForMember(member)
	if err != nil || got == nil {
		t.Fatalf("getActiveCombatSessionForMember: %v / %v", got, err)
	}
	if got.SessionID != s.SessionID || got.RosterSize() != 2 {
		t.Errorf("wrong session for member: %+v", got)
	}
	// The leader is not a participant row, so the member lookup must miss them.
	if got, err := getActiveCombatSessionForMember(lead); err != nil || got != nil {
		t.Errorf("member lookup matched the leader: %v / %v", got, err)
	}
}

// ── The engine writes every seat, not just the cursor ───────────────────────

// P3 shipped with seats 1+ opening fresh from their Mods on every resume: a
// party member's once-per-fight one-shots rearmed every single engine step. P4
// is what fixes that, so pin it. A round_end step mutates no one-shot, so an
// exact round-trip is the assertion.
func TestTurnEngine_PartySeatStatusesSurviveAStep(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 50, 50)
	sess.Participants = []CombatParticipant{{
		Seat: 1, UserID: "@b:x", HP: 30, HPMax: 30,
		Statuses: ActorStatuses{
			Raged: true, LuckyUsed: true, DeathSaveUsed: true,
			WardCharges: 2, ArcaneWardHP: 8,
			BuffACBonus: 3, // command-layer owned; commit must not zero it
		},
	}}
	want := sess.Participants[0].Statuses

	a, b, enemy := basePlayer(), basePlayer(), baseEnemy()
	rng := rand.New(rand.NewPCG(42, phaseOrdinal(sess.Phase)))
	te := resumeTurnEngine(sess, []*Combatant{&a, &b}, &enemy, rng)
	if _, err := te.step(PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	te.commit()

	if got := sess.Participants[0].Statuses; got != want {
		t.Errorf("seat 1 statuses moved across a step:\n got %+v\nwant %+v", got, want)
	}
	if sess.Participants[0].HP != 30 {
		t.Errorf("seat 1 HP = %d, want 30", sess.Participants[0].HP)
	}
}

// commit reads each seat by index, never off the cursor — round_end parks the
// cursor at seat 0, and the enemy turn parks it on its target. A seat's damage
// must land on that seat's row.
func TestTurnEngine_CommitWritesSeatsByIndex(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 50, 50)
	sess.Participants = []CombatParticipant{
		{Seat: 1, UserID: "@b:x", HP: 30, HPMax: 30, Statuses: ActorStatuses{PoisonTicks: 1, PoisonDmg: 4}},
		{Seat: 2, UserID: "@c:x", HP: 25, HPMax: 25},
	}
	a, b, c, enemy := basePlayer(), basePlayer(), basePlayer(), baseEnemy()
	rng := rand.New(rand.NewPCG(42, phaseOrdinal(sess.Phase)))
	te := resumeTurnEngine(sess, []*Combatant{&a, &b, &c}, &enemy, rng)
	if _, err := te.step(PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	te.commit()

	// Seat 1's poison ticked on seat 1 alone.
	if sess.Participants[0].HP != 26 {
		t.Errorf("seat 1 HP = %d, want 26 (30 - 4 poison)", sess.Participants[0].HP)
	}
	if sess.Participants[0].Statuses.PoisonTicks != 0 {
		t.Errorf("seat 1 poison ticks = %d, want 0", sess.Participants[0].Statuses.PoisonTicks)
	}
	if sess.Participants[1].HP != 25 {
		t.Errorf("seat 2 HP = %d, want 25 — seat 1's poison leaked", sess.Participants[1].HP)
	}
	if sess.PlayerHP != 50 {
		t.Errorf("seat 0 HP = %d, want 50 — seat 1's poison leaked", sess.PlayerHP)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
