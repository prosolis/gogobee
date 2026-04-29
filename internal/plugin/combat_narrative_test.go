package plugin

import (
	"strings"
	"testing"
)

func TestRenderCombatLog_ProducesMessages(t *testing.T) {
	player := Combatant{
		Name:     "Hero",
		Stats:    CombatStats{MaxHP: 100, Attack: 20, Defense: 10, Speed: 8, CritRate: 0.05},
		Mods:     CombatModifiers{DamageReduct: 1.0},
		IsPlayer: true,
	}
	enemy := Combatant{
		Name:  "Goblin",
		Stats: CombatStats{MaxHP: 60, Attack: 12, Defense: 5, Speed: 6, CritRate: 0.03},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}

	result := SimulateCombat(player, enemy, defaultCombatPhases)
	msgs := RenderCombatLog(result, "Hero", "Goblin")

	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 phase message, got %d", len(msgs))
	}
}

func TestRenderCombatLog_PhaseHeaders(t *testing.T) {
	player := Combatant{
		Name:     "Hero",
		Stats:    CombatStats{MaxHP: 100, Attack: 20, Defense: 10, Speed: 8, CritRate: 0.05},
		Mods:     CombatModifiers{DamageReduct: 1.0},
		IsPlayer: true,
	}
	enemy := Combatant{
		Name:  "Rat",
		Stats: CombatStats{MaxHP: 40, Attack: 8, Defense: 3, Speed: 4},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}

	result := SimulateCombat(player, enemy, defaultCombatPhases)
	msgs := RenderCombatLog(result, "Hero", "Rat")

	hasOpening := false
	for _, m := range msgs {
		if strings.Contains(m, "Opening") {
			hasOpening = true
		}
	}
	if !hasOpening {
		t.Error("should have an Opening phase header")
	}
}

func TestRenderCombatLog_WinPhaseMessages(t *testing.T) {
	result := CombatResult{
		PlayerWon:     true,
		PlayerStartHP: 100,
		EnemyStartHP:  50,
		PlayerEndHP:   80,
		EnemyEndHP:    0,
		TotalRounds:   3,
		Closeness:     0.2,
		Events: []CombatEvent{
			{Round: 1, Phase: "opening", Actor: "player", Action: "hit", Damage: 25, EnemyHP: 25, PlayerHP: 100},
			{Round: 2, Phase: "clash", Actor: "player", Action: "hit", Damage: 25, EnemyHP: 0, PlayerHP: 80},
		},
	}

	msgs := RenderCombatLog(result, "Hero", "Slime")

	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 phase message, got %d", len(msgs))
	}
	// Phase messages should contain damage numbers
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "25") {
			found = true
		}
	}
	if !found {
		t.Error("phase messages should contain damage values")
	}
}

func TestRenderCombatLog_LossPhaseMessages(t *testing.T) {
	result := CombatResult{
		PlayerWon:     false,
		PlayerStartHP: 100,
		EnemyStartHP:  80,
		PlayerEndHP:   0,
		EnemyEndHP:    40,
		TotalRounds:   4,
		Closeness:     0.5,
		Events: []CombatEvent{
			{Round: 1, Phase: "opening", Actor: "enemy", Action: "hit", Damage: 50, PlayerHP: 50, EnemyHP: 80},
			{Round: 2, Phase: "clash", Actor: "enemy", Action: "hit", Damage: 50, PlayerHP: 0, EnemyHP: 80},
		},
	}

	msgs := RenderCombatLog(result, "Hero", "Dragon")
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 phase message, got %d", len(msgs))
	}
}

func TestRenderCombatLogArena_ProducesPhaseMessages(t *testing.T) {
	result := CombatResult{
		PlayerWon:     true,
		PlayerStartHP: 100,
		EnemyStartHP:  60,
		PlayerEndHP:   70,
		EnemyEndHP:    0,
		TotalRounds:   3,
		Closeness:     0.3,
		Events: []CombatEvent{
			{Round: 1, Phase: "opening", Actor: "player", Action: "hit", Damage: 30, EnemyHP: 30, PlayerHP: 100},
			{Round: 2, Phase: "clash", Actor: "player", Action: "hit", Damage: 30, EnemyHP: 0, PlayerHP: 70},
		},
	}

	msgs := RenderCombatLogArena(result, "Hero", "Ratticus")
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 phase message, got %d", len(msgs))
	}
}

func TestRenderCombatLog_ConsumableEvents(t *testing.T) {
	result := CombatResult{
		PlayerWon:     true,
		PlayerStartHP: 100,
		EnemyStartHP:  60,
		PlayerEndHP:   80,
		EnemyEndHP:    0,
		Events: []CombatEvent{
			{Round: 0, Phase: "pre_combat", Actor: "consumable", Action: "use_consumable", Desc: "Goblin Grease"},
			{Round: 0, Phase: "pre_combat", Actor: "consumable", Action: "flat_damage", Damage: 8, PlayerHP: 100, EnemyHP: 52},
			{Round: 1, Phase: "opening", Actor: "player", Action: "hit", Damage: 30, PlayerHP: 100, EnemyHP: 22},
			{Round: 2, Phase: "clash", Actor: "player", Action: "hit", Damage: 22, PlayerHP: 80, EnemyHP: 0},
		},
	}

	msgs := RenderCombatLog(result, "Hero", "Imp")

	found := false
	for _, m := range msgs {
		if strings.Contains(m, "Goblin Grease") {
			found = true
		}
	}
	if !found {
		t.Error("should render consumable use event mentioning item name")
	}
}

func TestRenderCombatLog_MonsterAbilityEvents(t *testing.T) {
	result := CombatResult{
		PlayerWon:     true,
		PlayerStartHP: 100,
		EnemyStartHP:  60,
		PlayerEndHP:   50,
		EnemyEndHP:    0,
		Events: []CombatEvent{
			{Round: 1, Phase: "opening", Actor: "enemy", Action: "armor_break", Damage: 0, PlayerHP: 100, EnemyHP: 60},
			{Round: 1, Phase: "opening", Actor: "player", Action: "hit", Damage: 30, PlayerHP: 100, EnemyHP: 30},
			{Round: 2, Phase: "clash", Actor: "enemy", Action: "hit", Damage: 25, PlayerHP: 75, EnemyHP: 30},
			{Round: 2, Phase: "clash", Actor: "enemy", Action: "poison_tick", Damage: 5, PlayerHP: 70, EnemyHP: 30},
			{Round: 3, Phase: "decisive", Actor: "player", Action: "crit", Damage: 30, PlayerHP: 50, EnemyHP: 0},
		},
	}

	msgs := RenderCombatLog(result, "Hero", "Wyrm")

	hasArmorBreak := false
	hasPoison := false
	for _, m := range msgs {
		if strings.Contains(m, "armor") || strings.Contains(m, "Armor") || strings.Contains(m, "defense") || strings.Contains(m, "Defense") {
			hasArmorBreak = true
		}
		if strings.Contains(m, "poison") || strings.Contains(m, "Poison") || strings.Contains(m, "toxin") || strings.Contains(m, "Venom") || strings.Contains(m, "venom") {
			hasPoison = true
		}
	}
	if !hasArmorBreak {
		t.Error("should render armor_break event")
	}
	if !hasPoison {
		t.Error("should render poison_tick event")
	}
}
