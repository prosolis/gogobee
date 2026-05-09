package plugin

import "testing"

// Phase R6 — zone-condition harvest interactions (§3 zone notes).

func TestZoneConditionHarvestBlock_Underforge(t *testing.T) {
	cases := []struct {
		stacks int
		action HarvestAction
		want   bool
	}{
		{9, HarvestMine, false},
		{10, HarvestMine, true},
		{10, HarvestScavenge, false},
		{0, HarvestMine, false},
	}
	for _, c := range cases {
		exp := &Expedition{ZoneID: ZoneUnderforge, TemporalStack: c.stacks}
		got := zoneConditionHarvestBlock(exp, c.action) != ""
		if got != c.want {
			t.Errorf("Underforge stacks=%d action=%s: got block=%v want=%v", c.stacks, c.action, got, c.want)
		}
	}
}

func TestZoneConditionHarvestBlock_DragonsLair(t *testing.T) {
	exp := &Expedition{ZoneID: ZoneDragonsLair, RegionState: map[string]any{}}
	if zoneConditionHarvestBlock(exp, HarvestMine) != "" {
		t.Errorf("Dragon's Lair pre-awake should not block")
	}
	exp.RegionState["infernax_awake"] = true
	if zoneConditionHarvestBlock(exp, HarvestMine) == "" {
		t.Errorf("Dragon's Lair post-awake should block")
	}
}

func TestZoneConditionHarvestBlock_Abyss(t *testing.T) {
	cases := []struct {
		stack int
		want  bool
	}{
		{60, false},
		{80, false},
		{81, true},
		{99, true},
	}
	for _, c := range cases {
		exp := &Expedition{ZoneID: ZoneAbyssPortal, TemporalStack: c.stack}
		got := zoneConditionHarvestBlock(exp, HarvestEssence) != ""
		if got != c.want {
			t.Errorf("Abyss stack=%d: got block=%v want=%v", c.stack, got, c.want)
		}
	}
}

func TestZoneConditionHarvestDCMod_Underforge(t *testing.T) {
	cases := []struct {
		stacks int
		action HarvestAction
		want   int
	}{
		{6, HarvestMine, 0},
		{7, HarvestMine, 3},
		{9, HarvestMine, 3},
		{7, HarvestScavenge, 0},
	}
	for _, c := range cases {
		exp := &Expedition{ZoneID: ZoneUnderforge, TemporalStack: c.stacks}
		got, _ := zoneConditionHarvestDCMod(exp, c.action)
		if got != c.want {
			t.Errorf("Underforge stacks=%d action=%s: got delta=%d want=%d", c.stacks, c.action, got, c.want)
		}
	}
}

func TestZoneConditionHarvestDCMod_DragonsLair_Awareness(t *testing.T) {
	cases := []struct {
		day  int
		want int
	}{
		{8, 0},
		{9, 4},
		{12, 4},
	}
	for _, c := range cases {
		exp := &Expedition{ZoneID: ZoneDragonsLair, CurrentDay: c.day, RegionState: map[string]any{}}
		got, _ := zoneConditionHarvestDCMod(exp, HarvestMine)
		if got != c.want {
			t.Errorf("Dragon's Lair day=%d: got delta=%d want=%d", c.day, got, c.want)
		}
	}
}

func TestZoneConditionHarvestDCMod_Abyss(t *testing.T) {
	cases := []struct {
		stack int
		want  int
	}{
		{60, 0},
		{61, 5},
		{80, 5},
		{81, 0}, // 81+ is the block branch; DC mod stops contributing.
	}
	for _, c := range cases {
		exp := &Expedition{ZoneID: ZoneAbyssPortal, TemporalStack: c.stack}
		got, _ := zoneConditionHarvestDCMod(exp, HarvestMine)
		if got != c.want {
			t.Errorf("Abyss stack=%d: got delta=%d want=%d", c.stack, got, c.want)
		}
	}
}

func TestZoneConditionHarvestDCMod_FeywildDoubleDay(t *testing.T) {
	exp := &Expedition{ZoneID: ZoneFeywildCrossing, RegionState: map[string]any{
		"feywild_today": string(FeywildDistortionDouble),
	}}
	if got, _ := zoneConditionHarvestDCMod(exp, HarvestForage); got != -3 {
		t.Errorf("Feywild double-day forage: got delta=%d want=-3", got)
	}
	if got, _ := zoneConditionHarvestDCMod(exp, HarvestMine); got != 0 {
		t.Errorf("Feywild double-day mine: should not bonus, got delta=%d", got)
	}
	exp.RegionState["feywild_today"] = string(FeywildDistortionLoop)
	if got, _ := zoneConditionHarvestDCMod(exp, HarvestForage); got != 0 {
		t.Errorf("Feywild loop should not bonus forage DC, got delta=%d", got)
	}
}

func TestRollManorCursedTrinket(t *testing.T) {
	// Wrong zone or wrong action: never fires.
	if rollManorCursedTrinket(ZoneCryptValdris, HarvestScavenge, func(int) int { return 0 }) {
		t.Errorf("non-Manor zone should never fire")
	}
	if rollManorCursedTrinket(ZoneManorBlackspire, HarvestForage, func(int) int { return 0 }) {
		t.Errorf("non-scavenge action should never fire")
	}
	// 10% threshold: roll 9 fires, roll 10 does not.
	if !rollManorCursedTrinket(ZoneManorBlackspire, HarvestScavenge, func(int) int { return 9 }) {
		t.Errorf("roll=9 should fire (under 10%%)")
	}
	if rollManorCursedTrinket(ZoneManorBlackspire, HarvestScavenge, func(int) int { return 10 }) {
		t.Errorf("roll=10 should not fire (boundary)")
	}
}

func TestRestoreHarvestNodesInRoom(t *testing.T) {
	exp := &Expedition{
		ZoneID:        ZoneFeywildCrossing,
		CurrentRegion: "",
		RegionState: map[string]any{
			regionStateHarvestKey: map[string]map[string][]HarvestNode{
				"": {
					"0": {
						{ResourceID: "fey_dust", CurrentCharges: 0, MaxCharges: 2},
						{ResourceID: "enchanted_petal", CurrentCharges: 1, MaxCharges: 2},
					},
				},
			},
		},
	}
	if !restoreHarvestNodesInRoom(exp, 0) {
		t.Fatalf("restore should report changes")
	}
	table := loadHarvestTable(exp)
	got := table[""]["0"]
	for _, n := range got {
		if n.CurrentCharges != n.MaxCharges {
			t.Errorf("node %s: charges=%d max=%d (expected restored)", n.ResourceID, n.CurrentCharges, n.MaxCharges)
		}
	}
}

func TestRestoreHarvestNodesInRoom_NoOpEmpty(t *testing.T) {
	exp := &Expedition{ZoneID: ZoneFeywildCrossing, RegionState: map[string]any{}}
	if got := restoreHarvestNodesInRoom(exp, -1); got {
		t.Errorf("negative roomIdx should no-op")
	}
}

func TestIsBladeWeapon(t *testing.T) {
	cases := []struct {
		it   AdvItem
		want bool
	}{
		{AdvItem{Name: "Mithral Dagger", Type: "weapon"}, true},
		{AdvItem{Name: "Iron Longsword", Type: "weapon"}, true},
		{AdvItem{Name: "Battle Scimitar", Type: "weapon"}, true},
		{AdvItem{Name: "Heavy Mace", Type: "weapon"}, false},
		{AdvItem{Name: "Iron Dagger", Type: "consumable"}, false},
		{AdvItem{Name: "Drow Poison (diluted)", Type: "material"}, false},
	}
	for _, c := range cases {
		if got := isBladeWeapon(c.it); got != c.want {
			t.Errorf("%s (type=%s): got %v want %v", c.it.Name, c.it.Type, got, c.want)
		}
	}
}

func TestPickBladeWeapons_Coverage(t *testing.T) {
	items := []AdvItem{
		{ID: 1, Name: "Mithral Dagger", Type: "weapon"},
		{ID: 2, Name: "Iron Mace", Type: "weapon"},
		{ID: 3, Name: "Iron Longsword", Type: "weapon"},
		{ID: 4, Name: "Drow Poison (diluted)", Type: "material"},
	}
	picked, short := pickBladeWeapons(items, 2, nil)
	if short != 0 {
		t.Errorf("expected fully covered, got short=%d", short)
	}
	if len(picked) != 2 {
		t.Errorf("expected 2 picks, got %d", len(picked))
	}
	for _, id := range picked {
		if id == 2 || id == 4 {
			t.Errorf("picked non-blade id=%d", id)
		}
	}
}

func TestPickBladeWeapons_Short(t *testing.T) {
	items := []AdvItem{{ID: 1, Name: "Mithral Dagger", Type: "weapon"}}
	picked, short := pickBladeWeapons(items, 2, nil)
	if short != 1 {
		t.Errorf("expected short=1, got %d", short)
	}
	if len(picked) != 1 {
		t.Errorf("expected 1 pick, got %d", len(picked))
	}
}

func TestPickBladeWeapons_ExcludesAlreadyConsumed(t *testing.T) {
	items := []AdvItem{
		{ID: 1, Name: "Iron Dagger", Type: "weapon"},
		{ID: 2, Name: "Iron Sword", Type: "weapon"},
	}
	picked, short := pickBladeWeapons(items, 1, []int64{1})
	if short != 0 || len(picked) != 1 || picked[0] != 2 {
		t.Errorf("expected pick=[2] short=0; got pick=%v short=%d", picked, short)
	}
}

func TestThomCraftRecipes_DrowPoisonBlade_Defined(t *testing.T) {
	r, ok := findRecipeByID("drow_poison_blade")
	if !ok {
		t.Fatalf("drow_poison_blade recipe not registered")
	}
	if r.BladeWeaponCount != 1 {
		t.Errorf("expected BladeWeaponCount=1, got %d", r.BladeWeaponCount)
	}
	if _, ok := r.Ingredients["Drow Poison (diluted)"]; !ok {
		t.Errorf("expected Drow Poison ingredient")
	}
	if r.OutputType != "weapon" {
		t.Errorf("expected weapon output type, got %s", r.OutputType)
	}
}
