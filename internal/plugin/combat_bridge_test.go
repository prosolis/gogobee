package plugin

import (
	"testing"
	"time"
)

func TestCheckDeathSaveEvent_Found(t *testing.T) {
	events := []CombatEvent{
		{Action: "hit"}, {Action: "crit"}, {Action: "death_save"}, {Action: "hit"},
	}
	if !checkDeathSaveEvent(events) {
		t.Error("should find death_save event")
	}
}

func TestCheckDeathSaveEvent_NotFound(t *testing.T) {
	events := []CombatEvent{
		{Action: "hit"}, {Action: "crit"}, {Action: "block"},
	}
	if checkDeathSaveEvent(events) {
		t.Error("should not find death_save event")
	}
}

func TestCheckDeathSaveEvent_Empty(t *testing.T) {
	if checkDeathSaveEvent(nil) {
		t.Error("nil events should return false")
	}
	if checkDeathSaveEvent([]CombatEvent{}) {
		t.Error("empty events should return false")
	}
}

func TestApplyXPBonuses_AllNeutral(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{BaseXP: 100, OverlevelMult: 1.0})
	if r.Total != 100 {
		t.Errorf("neutral bonuses: got %d, want 100", r.Total)
	}
	if r.Breakdown != "" {
		t.Errorf("neutral breakdown should be empty, got %q", r.Breakdown)
	}
}

func TestApplyXPBonuses_NearDeath(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{BaseXP: 100, NearDeath: true, OverlevelMult: 1.0})
	if r.Total != 114 {
		t.Errorf("near-death: got %d, want 114", r.Total)
	}
	if r.Breakdown != "+15% near-death" {
		t.Errorf("breakdown = %q", r.Breakdown)
	}
}

func TestApplyXPBonuses_BonusMult(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{BaseXP: 100, BonusMult: 20, OverlevelMult: 1.0})
	if r.Total != 120 {
		t.Errorf("bonus mult 20%%: got %d, want 120", r.Total)
	}
}

func TestApplyXPBonuses_Ironclad(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{BaseXP: 100, Ironclad: true, OverlevelMult: 1.0})
	if r.Total != 105 {
		t.Errorf("ironclad: got %d, want 105", r.Total)
	}
}

func TestApplyXPBonuses_OverlevelPenalty(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{BaseXP: 100, OverlevelMult: 0.5})
	if r.Total != 50 {
		t.Errorf("overlevel 0.5x: got %d, want 50", r.Total)
	}
}

func TestApplyXPBonuses_OverlevelFloor(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{BaseXP: 1, OverlevelMult: 0.01})
	if r.Total < 1 {
		t.Errorf("overlevel floor: got %d, want >= 1", r.Total)
	}
}

func TestApplyXPBonuses_AllStacked(t *testing.T) {
	r := applyXPBonuses(XPBonusParams{
		BaseXP: 100, NearDeath: true, BonusMult: 20, Ironclad: true, OverlevelMult: 0.8,
	})
	if r.Total != 113 {
		t.Errorf("all stacked: got %d, want 113", r.Total)
	}
	if r.Breakdown == "" {
		t.Error("stacked breakdown should not be empty")
	}
}

func TestTransitionDeath_PardonFires(t *testing.T) {
	_ = transitionDeath(DeathTransitionParams{
		Char:        &AdventureCharacter{Alive: true},
		ChatLevel:   25,
		Location:    "Dark Cave",
		AllowPardon: true,
	})
	// Pardon is 33% chance — run enough times to confirm it can fire
	pardoned := 0
	for i := 0; i < 300; i++ {
		c := &AdventureCharacter{Alive: true}
		res := transitionDeath(DeathTransitionParams{
			Char: c, ChatLevel: 25, Location: "Dark Cave", AllowPardon: true,
		})
		if res.Pardoned {
			pardoned++
			if c.LastPardonUsed == nil {
				t.Fatal("pardon should set LastPardonUsed")
			}
			if c.GrudgeLocation != "Dark Cave" {
				t.Fatalf("pardon should set GrudgeLocation, got %q", c.GrudgeLocation)
			}
			if res.Died || res.Reprieved {
				t.Fatal("pardoned should not also be died/reprieved")
			}
		}
	}
	if pardoned == 0 {
		t.Error("pardon should fire at least once in 300 trials (p≈33%)")
	}
}

func TestTransitionDeath_PardonCooldown(t *testing.T) {
	recent := time.Now().UTC().Add(-1 * time.Hour)
	char := &AdventureCharacter{Alive: true, LastPardonUsed: &recent}
	r := transitionDeath(DeathTransitionParams{
		Char: char, ChatLevel: 25, Location: "Cave", AllowPardon: true,
	})
	if r.Pardoned {
		t.Error("pardon should not fire during cooldown")
	}
	if !r.Died {
		t.Error("should die when pardon on cooldown and no sovereign")
	}
}

func TestTransitionDeath_SovereignReprieve(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Condition: 80, ArenaSet: "sovereign"},
		SlotArmor:  {Condition: 90, ArenaSet: "sovereign"},
		SlotHelmet: {Condition: 70, ArenaSet: "sovereign"},
		SlotBoots:  {Condition: 60, ArenaSet: "sovereign"},
		SlotTool:   {Condition: 50, ArenaSet: "sovereign"},
	}
	char := &AdventureCharacter{Alive: true}
	r := transitionDeath(DeathTransitionParams{
		Char: char, Equip: equip, Location: "Abyss",
		AllowSovereign: true,
	})
	if !r.Reprieved {
		t.Fatal("sovereign should reprieve")
	}
	if r.Died || r.Pardoned {
		t.Error("reprieved should not also be died/pardoned")
	}
	if char.GrudgeLocation != "Abyss" {
		t.Errorf("grudge location = %q, want Abyss", char.GrudgeLocation)
	}
	for slot, eq := range equip {
		if eq.Condition != 1 {
			t.Errorf("slot %s condition = %d, want 1", slot, eq.Condition)
		}
	}
}

func TestTransitionDeath_KillAndPetRecovery(t *testing.T) {
	deaths, petRecoveries := 0, 0
	for i := 0; i < 500; i++ {
		char := &AdventureCharacter{Alive: true}
		pet := PetState{Type: "cat", Arrived: true, Level: 5}
		r := transitionDeath(DeathTransitionParams{Char: char, Pet: pet, Location: "Mine"})
		if !r.Died {
			t.Fatal("should die with no saves available")
		}
		if !char.Alive == false {
			t.Fatal("character should not be alive")
		}
		if char.GrudgeLocation != "Mine" {
			t.Fatalf("grudge = %q, want Mine", char.GrudgeLocation)
		}
		deaths++
		if r.PetRecovered {
			petRecoveries++
			if char.DeadUntil == nil {
				t.Fatal("dead until should be set")
			}
		}
	}
	if petRecoveries == 0 {
		t.Error("pet recovery should fire at least once in 500 deaths")
	}
}

func TestTransitionDeath_EngineSaveBurnsCooldown(t *testing.T) {
	char := &AdventureCharacter{Alive: true}
	r := transitionDeath(DeathTransitionParams{
		Char: char, EngineSaved: true,
	})
	if !r.Died {
		t.Error("should die (no sovereign/pardon)")
	}
	if char.DeathReprieveLast == nil {
		t.Error("engine save should burn DeathReprieveLast")
	}
}

func TestTransitionDeath_NoLocation(t *testing.T) {
	char := &AdventureCharacter{Alive: true, GrudgeLocation: "old"}
	transitionDeath(DeathTransitionParams{Char: char})
	if char.GrudgeLocation != "old" {
		t.Errorf("empty location should not change grudge, got %q", char.GrudgeLocation)
	}
}

func TestApplyDegradationModifiers_Basic(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Condition: 100},
		SlotArmor:  {Condition: 100},
	}
	damage := map[EquipmentSlot]int{
		SlotWeapon: 10,
		SlotArmor:  20,
	}
	result := applyDegradationModifiers(damage, equip)
	if equip[SlotWeapon].Condition != 90 {
		t.Errorf("weapon condition = %d, want 90", equip[SlotWeapon].Condition)
	}
	if equip[SlotArmor].Condition != 80 {
		t.Errorf("armor condition = %d, want 80", equip[SlotArmor].Condition)
	}
	if result[SlotWeapon] != 10 || result[SlotArmor] != 20 {
		t.Errorf("returned damage should match applied: weapon=%d, armor=%d", result[SlotWeapon], result[SlotArmor])
	}
}

func TestApplyDegradationModifiers_Tempered(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Condition: 100, ArenaSet: "tempered"},
		SlotArmor:  {Condition: 100, ArenaSet: "tempered"},
		SlotHelmet: {Condition: 100, ArenaSet: "tempered"},
		SlotBoots:  {Condition: 100, ArenaSet: "tempered"},
		SlotTool:   {Condition: 100, ArenaSet: "tempered"},
	}
	damage := map[EquipmentSlot]int{SlotWeapon: 20}
	result := applyDegradationModifiers(damage, equip)
	if result[SlotWeapon] != 15 { // 20 * 0.75 = 15
		t.Errorf("tempered weapon damage = %d, want 15", result[SlotWeapon])
	}
}

func TestApplyDegradationModifiers_Mastery(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Condition: 100, ActionsUsed: 25},
	}
	damage := map[EquipmentSlot]int{SlotWeapon: 10}
	result := applyDegradationModifiers(damage, equip)
	if result[SlotWeapon] != 8 { // 10 * 0.8 = 8
		t.Errorf("mastery weapon damage = %d, want 8", result[SlotWeapon])
	}
}

func TestApplyDegradationModifiers_ConditionFloor(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Condition: 5},
	}
	damage := map[EquipmentSlot]int{SlotWeapon: 20}
	applyDegradationModifiers(damage, equip)
	if equip[SlotWeapon].Condition != 0 {
		t.Errorf("condition should floor at 0, got %d", equip[SlotWeapon].Condition)
	}
}
