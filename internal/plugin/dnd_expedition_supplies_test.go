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
	// Phase 5-B (shipped): applyDailyBurn now scales by
	// phase5BDailyBurnRatePct = 50 by default — every expected value
	// here is halved relative to the pre-Phase-5-B baseline. The
	// math-pure shape (harsh-mult, siege-floor) is unchanged; only
	// the final multiplier is new. See applyDailyBurn doc for the
	// difficulty-pass motivation.
	s := ExpeditionSupplies{Current: 5, Max: 10, DailyBurn: 2, HarshMod: 1.5, ForagedToday: true}
	s, burn := applyDailyBurn(s, false, false)
	if burn != 1 { // 2 × 50%
		t.Errorf("burn = %v, want 1", burn)
	}
	if s.Current != 4 { // 5 - 1
		t.Errorf("current = %v, want 4", s.Current)
	}
	if s.ForagedToday {
		t.Error("forage flag should reset on day rollover")
	}
	// Harsh active applies HarshMod (1.5), then phase5B halves: 2*1.5*0.5 = 1.5.
	s, burn = applyDailyBurn(s, true, false)
	if burn != 1.5 {
		t.Errorf("harsh burn = %v, want 1.5", burn)
	}
	// Siege forces a 2× floor even when HarshMod is below 2 (tier 1);
	// phase5B halves: 1 × 2 × 0.5 = 1.
	t1 := ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1}
	_, sb := applyDailyBurn(t1, false, true)
	if sb != 1 {
		t.Errorf("siege tier1 burn = %v, want 1 (forced 2× floor × phase5B 50%%)", sb)
	}
	// Siege at higher tier still uses HarshMod when it exceeds 2:
	// 1 × 3 × 0.5 = 1.5.
	t5 := ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 3}
	_, sb5 := applyDailyBurn(t5, false, true)
	if sb5 != 1.5 {
		t.Errorf("siege tier5 burn = %v, want 1.5 (HarshMod × phase5B)", sb5)
	}
	// Drain to floor.
	for i := 0; i < 10; i++ {
		s, _ = applyDailyBurn(s, false, false)
	}
	if s.Current != 0 {
		t.Errorf("expected current clamped to 0, got %v", s.Current)
	}
}
