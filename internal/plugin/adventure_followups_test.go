package plugin

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Equipment Mastery ──────────────────────────────────────────────────────

func TestAdvEquipmentMasteryBonus(t *testing.T) {
	cases := []struct {
		name string
		used map[EquipmentSlot]int
		want float64
	}{
		{"empty equip", nil, 0},
		{"all sub-50", map[EquipmentSlot]int{SlotWeapon: 49, SlotArmor: 1}, 0},
		{"one slot crossed first threshold", map[EquipmentSlot]int{SlotWeapon: 50}, 1},
		{"one slot at all three thresholds", map[EquipmentSlot]int{SlotWeapon: 250}, 3},
		{"mixed: 100 + 50", map[EquipmentSlot]int{SlotWeapon: 100, SlotArmor: 50}, 3},
		{"all 5 slots maxed = 15 raw, capped at 10", map[EquipmentSlot]int{
			SlotWeapon: 999, SlotArmor: 999, SlotHelmet: 999, SlotBoots: 999, SlotTool: 999,
		}, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equip := map[EquipmentSlot]*AdvEquipment{}
			for slot, used := range tc.used {
				equip[slot] = &AdvEquipment{ActionsUsed: used}
			}
			got := advEquipmentMasteryBonus(equip)
			if got != tc.want {
				t.Errorf("got %.1f, want %.1f", got, tc.want)
			}
		})
	}
}

func TestAdvSlotRelevantToActivity(t *testing.T) {
	cases := []struct {
		slot     EquipmentSlot
		activity AdvActivityType
		want     bool
	}{
		// Combat exercises the combat loadout.
		{SlotWeapon, AdvActivityDungeon, true},
		{SlotArmor, AdvActivityDungeon, true},
		{SlotHelmet, AdvActivityDungeon, true},
		{SlotBoots, AdvActivityDungeon, true},
		{SlotTool, AdvActivityDungeon, false}, // a dungeon shouldn't master a pickaxe

		// Each harvest activity exercises only the tool.
		{SlotTool, AdvActivityMining, true},
		{SlotTool, AdvActivityForaging, true},
		{SlotTool, AdvActivityFishing, true},
		{SlotWeapon, AdvActivityMining, false}, // foraging shouldn't master a sword
		{SlotArmor, AdvActivityForaging, false},
		{SlotHelmet, AdvActivityFishing, false},
		{SlotBoots, AdvActivityMining, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s_%s", tc.slot, tc.activity), func(t *testing.T) {
			if got := advSlotRelevantToActivity(tc.slot, tc.activity); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAdvMasteryRowSegment(t *testing.T) {
	cases := []struct {
		used int
		want string
	}{
		{0, ""},
		{1, "1/50"},
		{49, "49/50"},
		{50, "50/100 ✦"},
		{99, "99/100 ✦"},
		{100, "100/250 ✦✦"},
		{249, "249/250 ✦✦"},
		{250, "250 ✦✦✦"},
		{500, "500 ✦✦✦"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("used=%d", tc.used), func(t *testing.T) {
			if got := advMasteryRowSegment(tc.used); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAdvMasteryRollup(t *testing.T) {
	cases := []struct {
		name             string
		used             map[EquipmentSlot]int
		wantCrossed      int
		wantMaxed        int
		wantBonus        float64
		wantAnyProgress  bool
	}{
		{"empty", nil, 0, 0, 0, false},
		{"sub-50 only", map[EquipmentSlot]int{SlotWeapon: 30}, 0, 0, 0, true},
		{"one slot 1 threshold", map[EquipmentSlot]int{SlotWeapon: 50}, 1, 0, 1, true},
		{"one slot maxed", map[EquipmentSlot]int{SlotWeapon: 250}, 3, 1, 3, true},
		{"two slots, 5 thresholds", map[EquipmentSlot]int{SlotWeapon: 100, SlotArmor: 250}, 5, 1, 5, true},
		{"all maxed = 15 raw, capped at 10", map[EquipmentSlot]int{
			SlotWeapon: 999, SlotArmor: 999, SlotHelmet: 999, SlotBoots: 999, SlotTool: 999,
		}, 15, 5, 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equip := map[EquipmentSlot]*AdvEquipment{}
			for slot, used := range tc.used {
				equip[slot] = &AdvEquipment{ActionsUsed: used}
			}
			got := advMasteryRollup(equip)
			if got.ThresholdsCrossed != tc.wantCrossed {
				t.Errorf("ThresholdsCrossed: got %d, want %d", got.ThresholdsCrossed, tc.wantCrossed)
			}
			if got.MaxedSlots != tc.wantMaxed {
				t.Errorf("MaxedSlots: got %d, want %d", got.MaxedSlots, tc.wantMaxed)
			}
			if got.Bonus != tc.wantBonus {
				t.Errorf("Bonus: got %.1f, want %.1f", got.Bonus, tc.wantBonus)
			}
			if got.AnyProgress != tc.wantAnyProgress {
				t.Errorf("AnyProgress: got %v, want %v", got.AnyProgress, tc.wantAnyProgress)
			}
		})
	}
}

func TestRenderAdvMasteryAggregate(t *testing.T) {
	cases := []struct {
		name          string
		used          map[EquipmentSlot]int
		wantEmpty     bool
		wantSubstring []string
		wantNot       []string
	}{
		{"no progress → empty", nil, true, nil, nil},
		{"sub-threshold progress → building up", map[EquipmentSlot]int{SlotWeapon: 30},
			false, []string{"building up", "first threshold at 50"}, []string{"+0%", "loot quality"}},
		{"one threshold crossed → singular threshold", map[EquipmentSlot]int{SlotWeapon: 50},
			false, []string{"+1% loot quality", "1 threshold crossed"}, []string{"thresholds", "maxed", "cap reached"}},
		{"three thresholds, no maxed → plural threshold no maxed", map[EquipmentSlot]int{SlotWeapon: 100, SlotArmor: 50},
			false, []string{"+3% loot quality", "3 thresholds crossed"}, []string{"maxed", "cap reached"}},
		{"one maxed + extras → mention maxed slot", map[EquipmentSlot]int{SlotWeapon: 250, SlotArmor: 50},
			false, []string{"+4% loot quality", "4 thresholds crossed", "1 slot maxed"}, []string{"cap reached", "slots maxed"}},
		{"capped → flag cap reached", map[EquipmentSlot]int{
			SlotWeapon: 250, SlotArmor: 250, SlotHelmet: 250, SlotBoots: 250, SlotTool: 250,
		}, false, []string{"+10% loot quality", "15 thresholds crossed", "5 slots maxed", "cap reached"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equip := map[EquipmentSlot]*AdvEquipment{}
			for slot, used := range tc.used {
				equip[slot] = &AdvEquipment{ActionsUsed: used}
			}
			got := renderAdvMasteryAggregate(equip)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("expected non-empty, got empty")
			}
			for _, s := range tc.wantSubstring {
				if !strings.Contains(got, s) {
					t.Errorf("expected to contain %q, got %q", s, got)
				}
			}
			for _, s := range tc.wantNot {
				if strings.Contains(got, s) {
					t.Errorf("should not contain %q, got %q", s, got)
				}
			}
		})
	}
}

// ── Treasure Rank Ordering ─────────────────────────────────────────────────

func TestAdvTreasureRank_Ordering(t *testing.T) {
	t5 := &AdvTreasureDef{Key: "a", Tier: 5, Bonuses: []advTreasureBonusDef{{Type: "all_skills", Value: 5}}}
	t3multi := &AdvTreasureDef{Key: "b", Tier: 3, Bonuses: []advTreasureBonusDef{
		{Type: "combat_level", Value: 1}, {Type: "death_chance", Value: -1},
	}}
	t3single := &AdvTreasureDef{Key: "c", Tier: 3, Bonuses: []advTreasureBonusDef{{Type: "loot_quality", Value: 5}}}
	t3same := &AdvTreasureDef{Key: "d", Tier: 3, Bonuses: []advTreasureBonusDef{{Type: "loot_quality", Value: 5}}}

	if !advTreasureRankBetter(advTreasureRank(t5), advTreasureRank(t3multi)) {
		t.Error("T5 should beat T3 regardless of bonus count")
	}
	if !advTreasureRankBetter(advTreasureRank(t3multi), advTreasureRank(t3single)) {
		t.Error("equal tier: more bonuses should win")
	}
	if advTreasureRankBetter(advTreasureRank(t3single), advTreasureRank(t3same)) {
		t.Error("equal tier and bonus count: should NOT be strictly better (no churn)")
	}
	if advTreasureRankBetter(advTreasureRank(t3same), advTreasureRank(t3single)) {
		t.Error("equal tier and bonus count, reversed: also not strictly better")
	}
}

func TestAdvTreasureIrreplaceable(t *testing.T) {
	special := &AdvTreasureDef{Bonuses: []advTreasureBonusDef{{Type: "special_respawn_halve", Value: 1}}}
	mixed := &AdvTreasureDef{Bonuses: []advTreasureBonusDef{
		{Type: "all_skills", Value: 5}, {Type: "special_monthly_death_bypass", Value: 1},
	}}
	regular := &AdvTreasureDef{Bonuses: []advTreasureBonusDef{{Type: "loot_quality", Value: 10}}}
	none := &AdvTreasureDef{}

	if !advTreasureIrreplaceable(special) {
		t.Error("special_* bonus should mark irreplaceable")
	}
	if !advTreasureIrreplaceable(mixed) {
		t.Error("any special_* among bonuses should mark irreplaceable")
	}
	if advTreasureIrreplaceable(regular) {
		t.Error("regular bonuses should not be irreplaceable")
	}
	if advTreasureIrreplaceable(none) {
		t.Error("zero bonuses is not irreplaceable")
	}
	if advTreasureIrreplaceable(nil) {
		t.Error("nil should not be irreplaceable")
	}
}

// ── Crafting Teaser Boundaries ─────────────────────────────────────────────

func TestRenderCraftingTeaser_BracketBoundaries(t *testing.T) {
	cases := []struct {
		name             string
		foraging         int
		craftsSucceeded  int
		wantEmpty        bool
		wantContainsAny  []string // any of these (OR)
		wantNotContains  []string
	}{
		{"foraging 6 — out of pre-window", 6, 0, true, nil, nil},
		{"foraging 7 — pre-unlock with 3 levels left, plural", 7, 0, false, []string{"3 Foraging level"}, []string{"unlocks in 1 ", "unlocks in 0"}},
		{"foraging 9 — pre-unlock with 1 level, singular", 9, 0, false, []string{"1 Foraging level"}, []string{"levels"}},
		{"foraging 10, no crafts yet — unlocked nudge", 10, 0, false, []string{"Crafting is unlocked"}, []string{"unlocks in"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			char := &AdventureCharacter{
				ForagingSkill:   tc.foraging,
				CraftsSucceeded: tc.craftsSucceeded,
			}
			got := renderCraftingTeaser(char)
			if tc.wantEmpty {
				if got != "" {
					t.Errorf("expected empty, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("expected non-empty teaser, got empty")
			}
			for _, s := range tc.wantContainsAny {
				if strings.Contains(got, s) {
					goto matched
				}
			}
			t.Errorf("expected any of %v, got %q", tc.wantContainsAny, got)
		matched:
			for _, bad := range tc.wantNotContains {
				if strings.Contains(got, bad) {
					t.Errorf("output should not contain %q, got %q", bad, got)
				}
			}
		})
	}
}

func TestCraftingReminderWeekday_Spread(t *testing.T) {
	// Across many synthetic users in the same week, the per-player weekday
	// should land on most weekdays (uniform-ish hash). Asserting "≥ 4 of 7"
	// is a loose sanity check that catches a constant or pathological hash.
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	hits := map[int]bool{}
	for i := 0; i < 100; i++ {
		uid := id.UserID(strings.Repeat("a", i+1) + "@test:local")
		hits[craftingReminderWeekday(uid, now)] = true
	}
	if len(hits) < 5 {
		t.Errorf("expected weekday spread across most days, only hit %d distinct: %v", len(hits), hits)
	}
}

func TestCraftingReminderWeekday_StableWithinWeek(t *testing.T) {
	uid := id.UserID("@stable:test")
	mon := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC) // Monday
	sun := time.Date(2026, 5, 10, 23, 0, 0, 0, time.UTC) // Sunday same ISO week
	if craftingReminderWeekday(uid, mon) != craftingReminderWeekday(uid, sun) {
		t.Error("weekday should be stable within an ISO week")
	}
	// Following ISO week — should differ for at least *some* users; not
	// guaranteed for every user but we test the rotation property exists.
	nextWeek := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	differs := 0
	for i := 0; i < 50; i++ {
		u := id.UserID(strings.Repeat("u", i+1) + "@x")
		if craftingReminderWeekday(u, mon) != craftingReminderWeekday(u, nextWeek) {
			differs++
		}
	}
	if differs == 0 {
		t.Error("expected the per-week hash to rotate at least some users between weeks")
	}
}

// ── Location Risk/Reward Numbers ───────────────────────────────────────────

func TestCalculateAdvProbabilities_RiskReward_NormalizesAcrossConfigs(t *testing.T) {
	// Risk/reward render line depends on these probabilities. Lock in that
	// they normalize to 100 across a few realistic configurations beyond
	// the existing single-config sum test.
	cases := []struct {
		name string
		char *AdventureCharacter
		loc  *AdvLocation
	}{
		{"low-tier with low skill", &AdventureCharacter{ForagingSkill: 3},
			&AdvLocation{Activity: AdvActivityForaging, Tier: 1, BaseDeathPct: 5, EmptyPct: 30}},
		{"high-tier with high skill", &AdventureCharacter{ForagingSkill: 30},
			&AdvLocation{Activity: AdvActivityForaging, Tier: 4, BaseDeathPct: 25, EmptyPct: 20}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equip := map[EquipmentSlot]*AdvEquipment{
				SlotTool: {Tier: 3, Condition: 100},
			}
			probs := calculateAdvProbabilities(tc.char, equip, tc.loc, &AdvBonusSummary{}, false)
			total := probs.DeathPct + probs.EmptyPct + probs.SuccessPct + probs.ExceptionalPct
			if total < 99.99 || total > 100.01 {
				t.Errorf("probabilities should sum to 100, got %.2f (%+v)", total, probs)
			}
			if probs.DeathPct < 0 || probs.ExceptionalPct < 0 {
				t.Errorf("negative probability: %+v", probs)
			}
		})
	}
}
