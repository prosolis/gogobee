package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func townTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// TestTownLoaders_ExerciseRealSchema seeds each source table and runs every
// loader against a real DB, catching column typos / GROUP BY errors that the
// pure render tests can't.
func TestTownLoaders_ExerciseRealSchema(t *testing.T) {
	townTestDB(t)

	alice := id.UserID("@alice:test.invalid")
	bob := id.UserID("@bob:test.invalid")

	if err := upsertPlayerMetaDisplayName(alice, "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := upsertPlayerMetaDisplayName(bob, "Bob"); err != nil {
		t.Fatal(err)
	}

	// Civic pride.
	trackTaxPaid(alice, 5000)
	trackTaxPaid(bob, 100)
	civic, err := loadCivicPrideBoard(townCivicBoardLimit)
	if err != nil {
		t.Fatalf("loadCivicPrideBoard: %v", err)
	}
	if len(civic) != 2 || civic[0].Name != "Alice" || civic[0].TotalPaid != 5000 {
		t.Fatalf("civic board wrong: %+v", civic)
	}

	// Housing street.
	if err := upsertPlayerMetaHouseState(alice, HouseState{Tier: 4}); err != nil {
		t.Fatal(err)
	}
	if err := upsertPlayerMetaHouseState(bob, HouseState{Tier: 1}); err != nil {
		t.Fatal(err)
	}
	houses, err := loadHousingStreet(townListCap)
	if err != nil {
		t.Fatalf("loadHousingStreet: %v", err)
	}
	if len(houses) != 2 || houses[0].Tier != 4 {
		t.Fatalf("housing street wrong: %+v", houses)
	}

	// Pet showcase — only arrived, non-chased pets appear.
	if err := upsertPlayerMetaPetState(alice, PetState{Type: "dog", Name: "Rex", Level: 5, ArmorTier: 2, Arrived: true}); err != nil {
		t.Fatal(err)
	}
	if err := upsertPlayerMetaPetState(bob, PetState{Type: "cat", Name: "Ghost", Level: 3, Arrived: true, ChasedAway: true}); err != nil {
		t.Fatal(err)
	}
	pets, err := loadPetShowcase(townListCap)
	if err != nil {
		t.Fatalf("loadPetShowcase: %v", err)
	}
	if len(pets) != 1 || !strings.Contains(pets[0].Name, "Rex") || pets[0].ArmorName != "Chain Dog Barding" {
		t.Fatalf("pet showcase wrong (chased-away pet should be hidden): %+v", pets)
	}

	// Graveyard.
	deadUntil := time.Now().Add(6 * time.Hour).UTC()
	if err := upsertPlayerMetaDeathState(alice, DeathState{
		Alive: false, DeadUntil: &deadUntil, LastDeathDate: "2026-07-10",
		DeathSource: "arena", DeathLocation: "the Colosseum",
	}); err != nil {
		t.Fatal(err)
	}
	graves, err := loadGraveyard(graveyardLimit)
	if err != nil {
		t.Fatalf("loadGraveyard: %v", err)
	}
	if len(graves) != 1 || graves[0].Name != "Alice" || !graves[0].StillDown || graves[0].Source != "arena" {
		t.Fatalf("graveyard wrong: %+v", graves)
	}

	// Rivals board — directed pair ledger summed per user.
	upsertRivalRecord(alice, bob, true)
	upsertRivalRecord(alice, bob, true)
	upsertRivalRecord(bob, alice, false)
	board, err := loadRivalsBoard(rivalsBoardLimit)
	if err != nil {
		t.Fatalf("loadRivalsBoard: %v", err)
	}
	if len(board) != 2 || board[0].Name != "Alice" || board[0].Wins != 2 {
		t.Fatalf("rivals board wrong: %+v", board)
	}
}

func TestRenderTownRegistry_AllThreeBoards(t *testing.T) {
	civic := []civicEntry{{Name: "Alice", TotalPaid: 1234567}, {Name: "Bob", TotalPaid: 500}}
	houses := []housingEntry{{Name: "Alice", Tier: 4}, {Name: "Bob", Tier: 1}}
	pets := []petShowcaseEntry{{Name: "Rex (Alice)", Type: "dog", Level: 7, ArmorName: "Plate Dog Barding"}}

	out := renderTownRegistry(civic, houses, pets)

	for _, want := range []string{
		"Civic Pride", "€1,234,567", "Alice",
		"Housing Street", "Established", "Base House",
		"Pet Showcase", "Rex (Alice)", "L7 dog", "Plate Dog Barding",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("town registry missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderTownRegistry_EmptyBoardsDontPanic(t *testing.T) {
	out := renderTownRegistry(nil, nil, nil)
	for _, want := range []string{"Civic Pride", "Housing Street", "Pet Showcase"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty registry missing header %q", want)
		}
	}
}

func TestPetArmorName(t *testing.T) {
	if got := petArmorName("dog", 0); got != "" {
		t.Errorf("tier 0 should be no armor, got %q", got)
	}
	if got := petArmorName("dog", 3); got != "Plate Dog Barding" {
		t.Errorf("dog tier 3 = %q, want Plate Dog Barding", got)
	}
	if got := petArmorName("cat", 5); got != "Dragonscale Cat Armor" {
		t.Errorf("cat tier 5 = %q, want Dragonscale Cat Armor", got)
	}
	if got := petArmorName("dog", 99); got != "" {
		t.Errorf("out-of-range tier should be empty, got %q", got)
	}
}

func TestRenderGraveyard_StillDownVsRecovered(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	deaths := []graveEntry{
		{Name: "Alice", Source: "arena", Location: "the Colosseum", LastDeathDate: "2026-07-10", StillDown: true},
		{Name: "Bob", Source: "adventure", Location: "", LastDeathDate: "2026-07-08", StillDown: false},
	}
	out := renderGraveyard(deaths, now)

	if !strings.Contains(out, "fell in the arena") {
		t.Errorf("arena death should read distinctly\n%s", out)
	}
	if !strings.Contains(out, "parts unknown") {
		t.Errorf("blank location should fall back\n%s", out)
	}
	if !strings.Contains(out, "still resting") {
		t.Errorf("Alice is still down\n%s", out)
	}
	if !strings.Contains(out, "back on their feet") {
		t.Errorf("Bob has recovered\n%s", out)
	}
	if !strings.Contains(out, "today") || !strings.Contains(out, "2 days ago") {
		t.Errorf("relative dates missing\n%s", out)
	}
}

func TestRenderGraveyard_Empty(t *testing.T) {
	out := renderGraveyard(nil, time.Now())
	if !strings.Contains(out, "garden's quiet") {
		t.Errorf("empty graveyard should be flavored\n%s", out)
	}
}

func TestRenderRivalsBoard_OrdersAndFormats(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	yesterday := now.Add(-24 * time.Hour)
	board := []rivalBoardEntry{
		{Name: "Alice", Wins: 9, Losses: 2, LastDuelAt: &yesterday},
		{Name: "Bob", Wins: 3, Losses: 5, LastDuelAt: nil},
	}
	out := renderRivalsBoard(board, now)

	if !strings.Contains(out, "9W - 2L") {
		t.Errorf("Alice's record missing\n%s", out)
	}
	if !strings.Contains(out, "1 day ago") {
		t.Errorf("relative last-duel missing\n%s", out)
	}
	if !strings.Contains(out, "last duel: —") {
		t.Errorf("Bob has no recorded duel time\n%s", out)
	}
	// Alice ranks above Bob.
	if strings.Index(out, "Alice") > strings.Index(out, "Bob") {
		t.Errorf("board should be ordered by wins\n%s", out)
	}
}

func TestHumanizeDeathDate(t *testing.T) {
	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"2026-07-10": "today",
		"2026-07-09": "yesterday",
		"2026-07-05": "5 days ago",
		"":           "some time ago",
		"garbage":    "garbage",
	}
	for in, want := range cases {
		if got := humanizeDeathDate(in, now); got != want {
			t.Errorf("humanizeDeathDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncName(t *testing.T) {
	if got := truncName("short", 18); got != "short" {
		t.Errorf("short name unchanged, got %q", got)
	}
	long := truncName("this-is-a-very-long-display-name", 10)
	if len([]rune(long)) != 10 {
		t.Errorf("truncated to %d runes, want 10: %q", len([]rune(long)), long)
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("truncated name should end with ellipsis: %q", long)
	}
}
