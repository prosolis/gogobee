package plugin

import (
	"strings"
	"testing"
)

// ── formatNumber (xp.go) ───────────────────────────────────────────────────

func TestFormatNumber_Small(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
	}
	for _, tc := range cases {
		if got := formatNumber(tc.in); got != tc.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatNumber_WithCommas(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{1000, "1,000"},
		{10000, "10,000"},
		{1000000, "1,000,000"},
		{1234567, "1,234,567"},
	}
	for _, tc := range cases {
		if got := formatNumber(tc.in); got != tc.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatNumber_Negative(t *testing.T) {
	got := formatNumber(-1234)
	if got != "-1,234" {
		t.Errorf("formatNumber(-1234) = %q, want %q", got, "-1,234")
	}
}

// ── formatNumberInt64 (tools.go) ───────────────────────────────────────────

func TestFormatNumberInt64_Small(t *testing.T) {
	if got := formatNumberInt64(42); got != "42" {
		t.Errorf("formatNumberInt64(42) = %q, want %q", got, "42")
	}
}

func TestFormatNumberInt64_Large(t *testing.T) {
	if got := formatNumberInt64(1234567890); got != "1,234,567,890" {
		t.Errorf("formatNumberInt64(1234567890) = %q, want %q", got, "1,234,567,890")
	}
}

func TestFormatNumberInt64_Negative(t *testing.T) {
	if got := formatNumberInt64(-999999); got != "-999,999" {
		t.Errorf("formatNumberInt64(-999999) = %q, want %q", got, "-999,999")
	}
}

// ── normalizeExpression (tools.go) ─────────────────────────────────────────

func TestNormalizeExpression_NaturalLanguage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"5 times 3", "5 * 3"},
		{"10 divided by 2", "10 / 2"},
		{"3 plus 4", "3 + 4"},
		{"10 minus 3", "10 - 3"},
		{"2 to the power of 8", "2 ** 8"},
	}
	for _, tc := range cases {
		if got := normalizeExpression(tc.in); got != tc.want {
			t.Errorf("normalizeExpression(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeExpression_StripPrefix(t *testing.T) {
	cases := []struct{ in, wantContains string }{
		{"what is 2+2", "2+2"},
		{"calculate 5*3", "5*3"},
		{"What's 10/2", "10/2"},
	}
	for _, tc := range cases {
		got := normalizeExpression(tc.in)
		if !strings.Contains(got, tc.wantContains) {
			t.Errorf("normalizeExpression(%q) = %q, want it to contain %q", tc.in, got, tc.wantContains)
		}
	}
}

func TestNormalizeExpression_CommaNumbers(t *testing.T) {
	got := normalizeExpression("40,000 + 1,234")
	if strings.Contains(got, ",") {
		t.Errorf("normalizeExpression should strip commas from numbers, got %q", got)
	}
}

func TestNormalizeExpression_PercentOf(t *testing.T) {
	got := normalizeExpression("8% of 40000")
	// Should transform to (8 / 100) * 40000
	if !strings.Contains(got, "/ 100") || !strings.Contains(got, "* 40000") {
		t.Errorf("normalizeExpression(\"8%% of 40000\") = %q, expected percent-of transformation", got)
	}
}

// ── formatCalcResult (tools.go) ────────────────────────────────────────────

func TestFormatCalcResult_Int(t *testing.T) {
	if got := formatCalcResult(42); got != "42" {
		t.Errorf("formatCalcResult(42) = %q, want %q", got, "42")
	}
}

func TestFormatCalcResult_LargeInt(t *testing.T) {
	if got := formatCalcResult(1000000); got != "1,000,000" {
		t.Errorf("formatCalcResult(1000000) = %q, want %q", got, "1,000,000")
	}
}

func TestFormatCalcResult_WholeFloat(t *testing.T) {
	// Whole number floats should format without decimals
	got := formatCalcResult(1000.0)
	if got != "1,000" {
		t.Errorf("formatCalcResult(1000.0) = %q, want %q", got, "1,000")
	}
}

func TestFormatCalcResult_DecimalFloat(t *testing.T) {
	got := formatCalcResult(3.14)
	if got != "3.14" {
		t.Errorf("formatCalcResult(3.14) = %q, want %q", got, "3.14")
	}
}

func TestFormatCalcResult_Int64(t *testing.T) {
	got := formatCalcResult(int64(9876543))
	if got != "9,876,543" {
		t.Errorf("formatCalcResult(int64(9876543)) = %q, want %q", got, "9,876,543")
	}
}

func TestFormatCalcResult_String(t *testing.T) {
	got := formatCalcResult("hello")
	if got != "hello" {
		t.Errorf("formatCalcResult(\"hello\") = %q, want %q", got, "hello")
	}
}

// ── countMatches (lottery.go) ──────────────────────────────────────────────

func TestCountMatches_AllMatch(t *testing.T) {
	if got := countMatches([]int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5}); got != 5 {
		t.Errorf("all match = %d, want 5", got)
	}
}

func TestCountMatches_NoneMatch(t *testing.T) {
	if got := countMatches([]int{1, 2, 3, 4, 5}, []int{6, 7, 8, 9, 10}); got != 0 {
		t.Errorf("none match = %d, want 0", got)
	}
}

func TestCountMatches_PartialMatch(t *testing.T) {
	if got := countMatches([]int{1, 2, 3, 4, 5}, []int{3, 5, 7, 9, 11}); got != 2 {
		t.Errorf("partial match = %d, want 2", got)
	}
}

// ── formatLotteryNumbers (lottery.go) ──────────────────────────────────────

func TestFormatLotteryNumbers(t *testing.T) {
	got := formatLotteryNumbers([]int{3, 12, 17, 24, 29})
	if !strings.Contains(got, "3") || !strings.Contains(got, "29") {
		t.Errorf("formatLotteryNumbers missing numbers: %q", got)
	}
	// Should use middle dot separator
	if !strings.Contains(got, "\u00b7") {
		t.Errorf("formatLotteryNumbers should use middle dot separator: %q", got)
	}
}

// ── generateLotteryNumbers (lottery.go) ────────────────────────────────────

func TestGenerateLotteryNumbers_Count(t *testing.T) {
	nums := generateLotteryNumbers()
	if len(nums) != 5 {
		t.Fatalf("generateLotteryNumbers returned %d numbers, want 5", len(nums))
	}
}

func TestGenerateLotteryNumbers_Range(t *testing.T) {
	for i := 0; i < 100; i++ {
		nums := generateLotteryNumbers()
		for _, n := range nums {
			if n < 1 || n > 30 {
				t.Fatalf("number %d out of range [1,30]", n)
			}
		}
	}
}

func TestGenerateLotteryNumbers_Sorted(t *testing.T) {
	for i := 0; i < 50; i++ {
		nums := generateLotteryNumbers()
		for j := 1; j < len(nums); j++ {
			if nums[j] <= nums[j-1] {
				t.Fatalf("numbers not sorted: %v", nums)
			}
		}
	}
}

func TestGenerateLotteryNumbers_Unique(t *testing.T) {
	for i := 0; i < 50; i++ {
		nums := generateLotteryNumbers()
		seen := make(map[int]bool)
		for _, n := range nums {
			if seen[n] {
				t.Fatalf("duplicate number %d in %v", n, nums)
			}
			seen[n] = true
		}
	}
}

// ── joinNames (lottery_draw.go) ────────────────────────────────────────────

func TestJoinNames_One(t *testing.T) {
	if got := joinNames([]string{"Alice"}); got != "Alice" {
		t.Errorf("joinNames([Alice]) = %q, want %q", got, "Alice")
	}
}

func TestJoinNames_Two(t *testing.T) {
	if got := joinNames([]string{"Alice", "Bob"}); got != "Alice and Bob" {
		t.Errorf("joinNames([Alice,Bob]) = %q, want %q", got, "Alice and Bob")
	}
}

func TestJoinNames_Three(t *testing.T) {
	got := joinNames([]string{"Alice", "Bob", "Charlie"})
	want := "Alice, Bob, and Charlie"
	if got != want {
		t.Errorf("joinNames([Alice,Bob,Charlie]) = %q, want %q", got, want)
	}
}

// ── chatLevelXPBonus (adventure_chat_perks.go) ─────────────────────────────

func TestChatLevelXPBonus(t *testing.T) {
	cases := []struct {
		level int
		want  float64
	}{
		{0, 0.0},
		{5, 0.0},
		{10, 0.05},
		{19, 0.05},
		{20, 0.10},
		{50, 0.25},
		{100, 0.25}, // capped
	}
	for _, tc := range cases {
		if got := chatLevelXPBonus(tc.level); got != tc.want {
			t.Errorf("chatLevelXPBonus(%d) = %f, want %f", tc.level, got, tc.want)
		}
	}
}

// ── chatLevelRareBonus (adventure_chat_perks.go) ───────────────────────────

func TestChatLevelRareBonus(t *testing.T) {
	cases := []struct {
		level int
		want  float64
	}{
		{0, 0.0},
		{9, 0.0},
		{10, 0.005},
		{30, 0.015},
		{50, 0.025},
		{99, 0.025}, // capped
	}
	for _, tc := range cases {
		if got := chatLevelRareBonus(tc.level); got != tc.want {
			t.Errorf("chatLevelRareBonus(%d) = %f, want %f", tc.level, got, tc.want)
		}
	}
}

// ── shopGreeting (adventure_chat_perks.go) ─────────────────────────────────

func TestShopGreeting_Tiers(t *testing.T) {
	cases := []struct {
		level       int
		wantContain string
	}{
		{0, "What do you need"},
		{10, "You again"},
		{30, "keeping your usual"},
		{50, "saw you coming"},
	}
	for _, tc := range cases {
		got := shopGreeting(tc.level)
		if !strings.Contains(got, tc.wantContain) {
			t.Errorf("shopGreeting(%d) = %q, want it to contain %q", tc.level, got, tc.wantContain)
		}
	}
}
