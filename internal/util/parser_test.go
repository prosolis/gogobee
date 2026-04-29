package util

import (
	"testing"
)

// ── XPForLevel ─────────────────────────────────────────────────────────────

func TestXPForLevel_Zero(t *testing.T) {
	if got := XPForLevel(0); got != 0 {
		t.Errorf("XPForLevel(0) = %d, want 0", got)
	}
}

func TestXPForLevel_KnownValues(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 100},
		{2, 400},
		{5, 2500},
		{10, 10000},
		{20, 40000},
		{50, 250000},
	}
	for _, tc := range cases {
		if got := XPForLevel(tc.level); got != tc.want {
			t.Errorf("XPForLevel(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

// ── LevelFromXP ────────────────────────────────────────────────────────────

func TestLevelFromXP_Zero(t *testing.T) {
	if got := LevelFromXP(0); got != 0 {
		t.Errorf("LevelFromXP(0) = %d, want 0", got)
	}
}

func TestLevelFromXP_Negative(t *testing.T) {
	if got := LevelFromXP(-100); got != 0 {
		t.Errorf("LevelFromXP(-100) = %d, want 0", got)
	}
}

func TestLevelFromXP_ExactBoundaries(t *testing.T) {
	// At exactly the XP threshold for a level, you should be that level.
	cases := []struct {
		xp   int
		want int
	}{
		{99, 0},
		{100, 1},
		{399, 1},
		{400, 2},
		{2500, 5},
		{10000, 10},
	}
	for _, tc := range cases {
		if got := LevelFromXP(tc.xp); got != tc.want {
			t.Errorf("LevelFromXP(%d) = %d, want %d", tc.xp, got, tc.want)
		}
	}
}

func TestLevelFromXP_RoundTrip(t *testing.T) {
	for level := 0; level <= 100; level++ {
		xp := XPForLevel(level)
		got := LevelFromXP(xp)
		if got != level {
			t.Errorf("LevelFromXP(XPForLevel(%d)) = %d, want %d", level, got, level)
		}
	}
}

// ── ProgressBar ────────────────────────────────────────────────────────────

func TestProgressBar_Empty(t *testing.T) {
	bar := ProgressBar(0, 100, 10)
	want := "[----------] 0%"
	if bar != want {
		t.Errorf("ProgressBar(0,100,10) = %q, want %q", bar, want)
	}
}

func TestProgressBar_Full(t *testing.T) {
	bar := ProgressBar(100, 100, 10)
	want := "[##########] 100%"
	if bar != want {
		t.Errorf("ProgressBar(100,100,10) = %q, want %q", bar, want)
	}
}

func TestProgressBar_Half(t *testing.T) {
	bar := ProgressBar(50, 100, 10)
	want := "[#####-----] 50%"
	if bar != want {
		t.Errorf("ProgressBar(50,100,10) = %q, want %q", bar, want)
	}
}

func TestProgressBar_Overflow(t *testing.T) {
	bar := ProgressBar(200, 100, 10)
	want := "[##########] 100%"
	if bar != want {
		t.Errorf("ProgressBar(200,100,10) = %q, want %q", bar, want)
	}
}

func TestProgressBar_ZeroMax(t *testing.T) {
	bar := ProgressBar(50, 0, 10)
	want := "----------"
	if bar != want {
		t.Errorf("ProgressBar(50,0,10) = %q, want %q", bar, want)
	}
}

// ── IsCommand ──────────────────────────────────────────────────────────────

func TestIsCommand_ExactMatch(t *testing.T) {
	if !IsCommand("!rank", "!", "rank") {
		t.Error("IsCommand should match exact command")
	}
}

func TestIsCommand_WithArgs(t *testing.T) {
	if !IsCommand("!rank @someone", "!", "rank") {
		t.Error("IsCommand should match command with args")
	}
}

func TestIsCommand_CaseInsensitive(t *testing.T) {
	if !IsCommand("!RANK", "!", "rank") {
		t.Error("IsCommand should be case-insensitive")
	}
}

func TestIsCommand_NoMatch(t *testing.T) {
	if IsCommand("!leaderboard", "!", "rank") {
		t.Error("IsCommand should not match different command")
	}
}

func TestIsCommand_PartialNoMatch(t *testing.T) {
	if IsCommand("!rankings", "!", "rank") {
		t.Error("IsCommand should not match partial prefix without space")
	}
}

func TestIsCommand_LeadingWhitespace(t *testing.T) {
	if !IsCommand("  !rank", "!", "rank") {
		t.Error("IsCommand should handle leading whitespace")
	}
}

// ── GetArgs ────────────────────────────────────────────────────────────────

func TestGetArgs_NoArgs(t *testing.T) {
	if got := GetArgs("!rank", "!", "rank"); got != "" {
		t.Errorf("GetArgs should return empty for no args, got %q", got)
	}
}

func TestGetArgs_WithArgs(t *testing.T) {
	if got := GetArgs("!rank @someone", "!", "rank"); got != "@someone" {
		t.Errorf("GetArgs = %q, want %q", got, "@someone")
	}
}

func TestGetArgs_MultipleArgs(t *testing.T) {
	got := GetArgs("!calc 2 + 2", "!", "calc")
	if got != "2 + 2" {
		t.Errorf("GetArgs = %q, want %q", got, "2 + 2")
	}
}

func TestGetArgs_WrongCommand(t *testing.T) {
	if got := GetArgs("!other stuff", "!", "rank"); got != "" {
		t.Errorf("GetArgs for wrong command should return empty, got %q", got)
	}
}

// ── HasNonASCII ────────────────────────────────────────────────────────────

func TestHasNonASCII_ASCII(t *testing.T) {
	if HasNonASCII("hello world 123") {
		t.Error("pure ASCII should return false")
	}
}

func TestHasNonASCII_Emoji(t *testing.T) {
	if !HasNonASCII("hello 🌍") {
		t.Error("emoji should return true")
	}
}

func TestHasNonASCII_Unicode(t *testing.T) {
	if !HasNonASCII("café") {
		t.Error("accented chars should return true")
	}
}

func TestHasNonASCII_Empty(t *testing.T) {
	if HasNonASCII("") {
		t.Error("empty string should return false")
	}
}

// ── ParseMessage ───────────────────────────────────────────────────────────

func TestParseMessage_Plain(t *testing.T) {
	stats := ParseMessage("hello world")
	if stats.Words != 2 {
		t.Errorf("Words = %d, want 2", stats.Words)
	}
	if stats.Chars != 11 {
		t.Errorf("Chars = %d, want 11", stats.Chars)
	}
}

func TestParseMessage_WithLinks(t *testing.T) {
	stats := ParseMessage("check https://example.com and https://other.org")
	if stats.Links != 2 {
		t.Errorf("Links = %d, want 2", stats.Links)
	}
}

func TestParseMessage_WithImages(t *testing.T) {
	stats := ParseMessage("look https://img.example.com/cat.png here")
	if stats.Images != 1 {
		t.Errorf("Images = %d, want 1", stats.Images)
	}
	if stats.Links != 1 {
		t.Errorf("Links = %d, want 1", stats.Links)
	}
}

func TestParseMessage_Questions(t *testing.T) {
	stats := ParseMessage("what? really? yes!")
	if stats.Questions != 2 {
		t.Errorf("Questions = %d, want 2", stats.Questions)
	}
	if stats.Exclamations != 1 {
		t.Errorf("Exclamations = %d, want 1", stats.Exclamations)
	}
}

func TestParseMessage_Empty(t *testing.T) {
	stats := ParseMessage("")
	if stats.Words != 0 || stats.Chars != 0 {
		t.Errorf("empty message should have 0 words and 0 chars")
	}
}
