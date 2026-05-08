package plugin

import "testing"

func TestSupplyDailyBurn_AllTiers(t *testing.T) {
	cases := []struct {
		tier ZoneTier
		want float32
	}{
		{ZoneTierBeginner, 1.0},
		{ZoneTierApprentice, 1.5},
		{ZoneTierJourneyman, 2.0},
		{ZoneTierVeteran, 3.0},
		{ZoneTierLegendary, 4.0},
	}
	for _, c := range cases {
		if got := supplyDailyBurn(c.tier); got != c.want {
			t.Errorf("tier %d: got %v, want %v", c.tier, got, c.want)
		}
	}
}

func TestSupplyHarshMultiplier_AllTiers(t *testing.T) {
	cases := []struct {
		tier ZoneTier
		want float32
	}{
		{ZoneTierBeginner, 1.0},
		{ZoneTierApprentice, 1.5},
		{ZoneTierJourneyman, 2.0},
		{ZoneTierVeteran, 2.5},
		{ZoneTierLegendary, 3.0},
	}
	for _, c := range cases {
		if got := supplyHarshMultiplier(c.tier); got != c.want {
			t.Errorf("tier %d: got %v, want %v", c.tier, got, c.want)
		}
	}
}

func TestSupplyDepletion_Brackets(t *testing.T) {
	cases := []struct {
		current float32
		max     float32
		want    SupplyDepletionState
	}{
		{10, 10, SupplyNormal},        // 100%
		{3, 10, SupplyNormal},         // 30%
		{2.5, 10, SupplyNormal},       // 25% — boundary stays normal
		{2, 10, SupplyRationing},      // 20%
		{1, 10, SupplyRationing},      // 10% — boundary stays rationing
		{0.9, 10, SupplySevereRationing},
		{0.1, 10, SupplySevereRationing},
		{0, 10, SupplyStarvation},
	}
	for _, c := range cases {
		s := ExpeditionSupplies{Current: c.current, Max: c.max}
		if got := supplyDepletion(s); got != c.want {
			t.Errorf("%v/%v: got %d, want %d", c.current, c.max, got, c.want)
		}
	}
}

func TestSupplyRollModifier(t *testing.T) {
	cases := map[SupplyDepletionState]int{
		SupplyNormal:          0,
		SupplyRationing:       -1,
		SupplySevereRationing: -2,
		SupplyStarvation:      -2,
	}
	for state, want := range cases {
		if got := supplyRollModifier(state); got != want {
			t.Errorf("state %d: got %d, want %d", state, got, want)
		}
	}
}

func TestSupplyAllowsLongRest(t *testing.T) {
	cases := map[SupplyDepletionState]bool{
		SupplyNormal:          true,
		SupplyRationing:       true,
		SupplySevereRationing: false,
		SupplyStarvation:      false,
	}
	for state, want := range cases {
		if got := supplyAllowsLongRest(state); got != want {
			t.Errorf("state %d: got %v, want %v", state, got, want)
		}
	}
}

func TestSupplyPurchase_Validate(t *testing.T) {
	cases := []struct {
		p       SupplyPurchase
		wantErr bool
	}{
		{SupplyPurchase{StandardPacks: 1}, false},
		{SupplyPurchase{StandardPacks: 3, DeluxePacks: 1}, false}, // max
		{SupplyPurchase{StandardPacks: 4}, true},                  // over standard cap
		{SupplyPurchase{DeluxePacks: 2}, true},                    // over deluxe cap
		{SupplyPurchase{StandardPacks: -1}, true},
		{SupplyPurchase{}, true}, // none purchased
	}
	for _, c := range cases {
		err := c.p.Validate()
		if (err != nil) != c.wantErr {
			t.Errorf("%+v: err = %v, wantErr=%v", c.p, err, c.wantErr)
		}
	}
}

func TestSupplyPurchase_TotalAndCost(t *testing.T) {
	p := SupplyPurchase{StandardPacks: 3, DeluxePacks: 1}
	if p.Total() != 50 {
		t.Errorf("total = %v, want 50", p.Total())
	}
	if p.Cost() != 240 {
		t.Errorf("cost = %d, want 240", p.Cost())
	}
}

func TestMakeSupplies_FillsFromTier(t *testing.T) {
	p := SupplyPurchase{StandardPacks: 2}
	s := makeSupplies(ZoneTierJourneyman, p)
	if s.Max != 20 || s.Current != 20 {
		t.Errorf("supplies: %+v", s)
	}
	if s.DailyBurn != 2.0 {
		t.Errorf("daily burn = %v, want 2", s.DailyBurn)
	}
	if s.PacksStandard != 2 {
		t.Errorf("packs std = %d", s.PacksStandard)
	}
}

func TestApplyDailyBurn_DrainsAndClamps(t *testing.T) {
	s := ExpeditionSupplies{Current: 5, Max: 10, DailyBurn: 2, HarshMod: 1.5, ForagedToday: true}
	s, burn := applyDailyBurn(s, false)
	if burn != 2 {
		t.Errorf("burn = %v, want 2", burn)
	}
	if s.Current != 3 {
		t.Errorf("current = %v, want 3", s.Current)
	}
	if s.ForagedToday {
		t.Error("forage flag should reset on day rollover")
	}
	// Harsh active doubles via mult.
	s, burn = applyDailyBurn(s, true)
	if burn != 3 { // 2 × 1.5
		t.Errorf("harsh burn = %v, want 3", burn)
	}
	// Drain to floor.
	for i := 0; i < 5; i++ {
		s, _ = applyDailyBurn(s, false)
	}
	if s.Current != 0 {
		t.Errorf("expected current clamped to 0, got %v", s.Current)
	}
}
