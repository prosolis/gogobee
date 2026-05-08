package plugin

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Phase 11 D1e — combat / trap / boss resolution per zone room.
//
// D1c shipped the !zone command surface and D1d wired TwinBee narration
// + mood. D1e fills in the "what actually happens when you !zone advance
// into a fight" — spawning enemies from the zone roster, running them
// through the existing combat engine with the player's full D&D layer,
// awarding XP per kill and zone loot on boss defeat, and ending the run
// (mood penalty + GMPlayerDeath narration) if the player goes down.
//
// Per-room behavior:
//   Entry        → no combat (already narrated on enter)
//   Exploration  → SpawnWeight-biased pick from non-elite roster, combat
//   Trap         → flat % MaxHP nick + trap_detected/trap_tripped flavor
//   Elite        → pick from elite-only roster (falls back to non-elite),
//                  combat with +tier scaling on stats
//   Boss         → zone.Boss bestiary entry, combat + zone Loot table on win
//
// Boss fights remain one-shot SimulateCombat in D1e — the per-turn
// !attack/!cast/!dodge interface from gogobee_dungeon_zones.md §4 is
// scoped to a later sub-phase. The roster, stat blocks, and mood/loot
// scaffolding land here so the run loop is end-to-end playable.

// ── Enemy selection ─────────────────────────────────────────────────────────

// pickZoneEnemy picks one enemy from the zone roster for the given room
// kind. Selection is deterministic on (run, room) so re-running the same
// room (idempotent reads of !zone advance after it's already resolved)
// yields the same line of fate. isElite filters to entries flagged elite,
// falling back to the full roster if no elite entries exist.
//
// SpawnWeight (1..10) biases the cumulative wheel; weight 0 falls back to 5.
func pickZoneEnemy(zone ZoneDefinition, runID string, roomIdx int, isElite bool) (DnDMonsterTemplate, bool) {
	pool := make([]ZoneEnemy, 0, len(zone.Enemies))
	if isElite {
		for _, e := range zone.Enemies {
			if e.IsElite {
				pool = append(pool, e)
			}
		}
	}
	if len(pool) == 0 {
		for _, e := range zone.Enemies {
			if !isElite && e.IsElite {
				continue
			}
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 {
		return DnDMonsterTemplate{}, false
	}

	total := 0
	for _, e := range pool {
		w := e.SpawnWeight
		if w <= 0 {
			w = 5
		}
		total += w
	}
	roll := int(zoneSelectorHash(runID, roomIdx) % uint64(total))
	cum := 0
	var chosen ZoneEnemy
	for _, e := range pool {
		w := e.SpawnWeight
		if w <= 0 {
			w = 5
		}
		cum += w
		if roll < cum {
			chosen = e
			break
		}
	}
	tmpl, ok := dndBestiary[chosen.BestiaryID]
	return tmpl, ok
}

// zoneSelectorHash — same family as pickLineDeterministic; kept separate
// so a roster pick and a narration pick on the same (run, room) don't
// collide on the same hash output.
func zoneSelectorHash(runID string, roomIdx int) uint64 {
	h := fnv.New64a()
	h.Write([]byte("enemy:"))
	h.Write([]byte(runID))
	var sb [8]byte
	for i := 0; i < 8; i++ {
		sb[i] = byte(roomIdx >> (8 * i))
	}
	h.Write(sb[:])
	return h.Sum64()
}

// ── Combat runner ───────────────────────────────────────────────────────────

// runZoneCombat sets up a Combatant pair from the player's full D&D
// layer + a bestiary monster, runs SimulateCombat with dungeonCombatPhases,
// and persists post-combat side effects (HP, subclass state, XP).
//
// Returns the engine result so the caller can branch on PlayerWon and
// fold combat events into the per-room narration.
func (p *AdventurePlugin) runZoneCombat(
	userID id.UserID,
	monster DnDMonsterTemplate,
	tier int,
) (CombatResult, error) {
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return CombatResult{}, fmt.Errorf("load adv character: %w", err)
	}
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		return CombatResult{}, fmt.Errorf("load equipment: %w", err)
	}
	bonuses := p.loadCombatBonuses(userID, char)
	chatLvl := p.chatLevel(userID)

	playerStats, playerMods := DerivePlayerStats(char, equip, bonuses, chatLvl, char.CurrentStreak, false)

	dndChar, freshMigrate, err := ensureDnDCharacterForCombat(userID, char)
	if err != nil {
		return CombatResult{}, fmt.Errorf("ensure dnd character: %w", err)
	}
	if freshMigrate {
		p.maybeSendDnDOnboarding(userID, char, dndChar)
	}
	applyDnDPlayerLayer(&playerStats, dndChar)
	applyDnDEquipmentLayer(&playerStats, dndChar, equip)
	applyDnDHPScaling(&playerStats, dndChar)
	applyClassPassives(&playerStats, &playerMods, dndChar)
	applyRacePassives(&playerStats, &playerMods, dndChar)
	applySubclassPassives(&playerStats, &playerMods, dndChar)
	if firedName, fired := applyArmedAbility(dndChar, &playerMods); fired {
		slog.Info("dnd: armed ability fired (zone)", "user", userID, "ability", firedName)
	}

	enemyStats, enemyMods := monster.toCombatStats()
	// Tier scaling matches applyDnDDungeonMonsterLayer's intent: keep AC
	// floor reasonable for higher-tier zones, but only bump if the bestiary
	// stat block is below the floor — boss/elite stat blocks already encode
	// their challenge, so we don't double-scale them.
	if tier > 1 {
		floorAC := dndDungeonACBase + tier
		if enemyStats.AC < floorAC {
			enemyStats.AC = floorAC
		}
		floorAB := dndDungeonAtkBase + tier
		if enemyStats.AttackBonus < floorAB {
			enemyStats.AttackBonus = floorAB
		}
	}
	applyPendingCast(userID, dndChar, &playerStats, &playerMods, &enemyStats)

	player := Combatant{
		Name:     char.DisplayName,
		Stats:    playerStats,
		Mods:     playerMods,
		IsPlayer: true,
	}
	enemy := Combatant{
		Name:    monster.Name,
		Stats:   enemyStats,
		Mods:    enemyMods,
		Ability: monster.Ability,
	}

	result := SimulateCombat(player, enemy, dungeonCombatPhases)

	p.grantCombatAchievements(userID, result)
	persistDnDHPAfterCombat(userID, result.PlayerStartHP, result.PlayerEndHP)
	if err := persistDnDPostCombatSubclass(dndChar, playerMods.BerserkerRage, result, playerMods); err != nil {
		slog.Error("dnd: post-combat subclass persist (zone)", "user", userID, "err", err)
	}

	if xp := zoneCombatXP(result, monster.CR, tier); xp > 0 {
		if _, err := p.grantDnDXP(userID, xp); err != nil {
			slog.Error("dnd: grantDnDXP zone", "user", userID, "err", err)
		}
	}

	return result, nil
}

// zoneCombatXP — CR-weighted XP per kill, with a tier-based floor so
// low-CR roster fillers in a high-tier zone still feel worthwhile.
// Loss grants the standard dndLossXPFraction.
func zoneCombatXP(result CombatResult, cr float32, tier int) int {
	if tier < 1 {
		tier = 1
	}
	// CR 0.25 → 25, CR 1 → 100, CR 3 → 300, CR 5 → 500. Caps at CR 20.
	base := int(cr * 100)
	floor := dndDungeonXPPerTier * tier // T1=15, T5=75
	if base < floor {
		base = floor
	}
	if !result.PlayerWon {
		return int(float64(base) * dndLossXPFraction)
	}
	if result.NearDeath {
		return int(float64(base) * dndNearDeathXPBonus)
	}
	return base
}

// ── Mood event scanning ─────────────────────────────────────────────────────

// scanMoodEventsFromCombat walks combat events and applies §3.2 mood
// triggers for nat-20s and nat-1s. Returns the count of each so the
// caller can include the deltas in the rendered status line.
func scanMoodEventsFromCombat(runID string, result CombatResult) (nat20s, nat1s int) {
	for _, ev := range result.Events {
		if ev.Roll == 20 && ev.Actor == "player" {
			nat20s++
		}
		if ev.Roll == 1 && ev.Actor == "player" {
			nat1s++
		}
	}
	for i := 0; i < nat20s; i++ {
		if _, err := applyMoodEvent(runID, MoodEventNat20); err != nil {
			slog.Error("zone: applyMoodEvent nat20", "run", runID, "err", err)
			break
		}
	}
	for i := 0; i < nat1s; i++ {
		if _, err := applyMoodEvent(runID, MoodEventNat1); err != nil {
			slog.Error("zone: applyMoodEvent nat1", "run", runID, "err", err)
			break
		}
	}
	return nat20s, nat1s
}

// ── Trap rooms ──────────────────────────────────────────────────────────────

// resolveTrapRoom picks a trap from the zone's tier catalog (D2a),
// rolls a detection skill check (Perception/Investigation, per trap),
// and either narrates a clean spot or applies the trap's damage dice.
// Damage is capped so a single trap can't outright KO the player.
//
// Higher-tier zones currently fall through to the pre-D2a flat-percent
// nick — D3a/D4a will add Tier 2+ trap catalogs.
func (p *AdventurePlugin) resolveTrapRoom(userID id.UserID, run *DungeonRun, zone ZoneDefinition) (damage int, narration string) {
	dndChar, _ := LoadDnDCharacter(userID)
	if dndChar == nil {
		return 0, ""
	}
	trap, ok := pickZoneTrap(zone, run.RunID, run.CurrentRoom)
	if !ok {
		return p.resolveTrapRoomLegacy(userID, run, zone, dndChar)
	}
	return p.applyTrapEffect(userID, run, zone, dndChar, trap)
}

// applyTrapEffect runs the detect roll and damage application for a
// chosen trap. Split out from resolveTrapRoom so tests can target a
// specific trap kind without depending on the room-seed selector.
func (p *AdventurePlugin) applyTrapEffect(userID id.UserID, run *DungeonRun, zone ZoneDefinition, dndChar *DnDCharacter, trap zoneTrapDef) (int, string) {
	detect := performSkillCheck(dndChar, trap.DetectSkill, trap.DetectDC)
	return p.applyTrapEffectWithDetect(userID, run, zone, dndChar, trap, detect)
}

// applyTrapEffectWithDetect is the deterministic core of trap resolution:
// given a precomputed detection check result, it produces the narration
// and persists HP loss on a failed detect. Used by applyTrapEffect (which
// rolls the d20) and by tests (which inject a fixed result).
func (p *AdventurePlugin) applyTrapEffectWithDetect(
	userID id.UserID,
	run *DungeonRun,
	zone ZoneDefinition,
	dndChar *DnDCharacter,
	trap zoneTrapDef,
	detect SkillCheckResult,
) (int, string) {
	var b strings.Builder
	if line := twinBeeLine(zone.ID, GMTrapDetected, run.RunID, run.CurrentRoom); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if detect.Success {
		b.WriteString(trapSpottedHeader(trap, detect))
		return 0, b.String()
	}

	dmg := rollTrapDamage(trap, run.RunID, run.CurrentRoom)
	if dmg >= dndChar.HPCurrent {
		dmg = dndChar.HPCurrent - 1
		if dmg < 0 {
			dmg = 0
		}
	}
	if dmg > 0 {
		dndChar.HPCurrent -= dmg
		if err := SaveDnDCharacter(dndChar); err != nil {
			slog.Error("zone: trap HP persist", "user", userID, "err", err)
		}
	}
	if line := twinBeeLine(zone.ID, GMTrapTripped, run.RunID, run.CurrentRoom); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(trapDamageHeader(trap, dmg, dndChar.HPCurrent, dndChar.HPMax))
	return dmg, b.String()
}

// resolveTrapRoomLegacy is the pre-D2a flat-percent fallback used when a
// zone tier has no entries in the trap catalog yet (Tier 2+). Removed
// once D3a/D4a fill in the higher-tier catalogs.
func (p *AdventurePlugin) resolveTrapRoomLegacy(userID id.UserID, run *DungeonRun, zone ZoneDefinition, dndChar *DnDCharacter) (int, string) {
	tier := int(zone.Tier)
	pct := 0.08 + 0.03*float64(tier-1)
	dmg := int(float64(dndChar.HPMax) * pct)
	if dmg < 1 {
		dmg = 1
	}
	if dmg >= dndChar.HPCurrent {
		dmg = dndChar.HPCurrent - 1
		if dmg < 0 {
			dmg = 0
		}
	}
	dndChar.HPCurrent -= dmg
	if err := SaveDnDCharacter(dndChar); err != nil {
		slog.Error("zone: trap HP persist (legacy)", "user", userID, "err", err)
	}
	var b strings.Builder
	if line := twinBeeLine(zone.ID, GMTrapDetected, run.RunID, run.CurrentRoom); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if line := twinBeeLine(zone.ID, GMTrapTripped, run.RunID, run.CurrentRoom); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("💢 The trap takes **%d HP** (%d/%d remaining).", dmg, dndChar.HPCurrent, dndChar.HPMax))
	return dmg, b.String()
}

// ── Loot ─────────────────────────────────────────────────────────────────────

// rollZoneLoot rolls every entry in zone.Loot and returns the IDs that
// dropped. UniqueAlways entries always drop. Probabilistic entries roll
// independently. Coin-pattern IDs ("coins_<dice>x<mult>") are expanded
// to a single "coins" item with rolled value; everything else becomes a
// treasure-tier inventory item with a tier-derived placeholder value
// (zone equipment registry wiring is a later content phase).
func (p *AdventurePlugin) rollZoneLoot(userID id.UserID, run *DungeonRun, zone ZoneDefinition) []string {
	rng := rand.New(rand.NewPCG(uint64(zoneSelectorHash(run.RunID, run.CurrentRoom)), 0x100712))
	var granted []string
	for _, entry := range zone.Loot {
		if !entry.UniqueAlways && rng.Float64() > entry.DropChance {
			continue
		}
		item, ok := zoneLootToInventory(entry, zone, rng)
		if !ok {
			continue
		}
		if err := addAdvInventoryItem(userID, item); err != nil {
			slog.Error("zone: addAdvInventoryItem", "user", userID, "item", item.Name, "err", err)
			continue
		}
		if err := addLoot(run.RunID, entry.ItemID); err != nil {
			slog.Error("zone: addLoot audit", "user", userID, "item", entry.ItemID, "err", err)
		}
		granted = append(granted, entry.ItemID)
	}
	return granted
}

// zoneLootToInventory converts a ZoneLootEntry into an AdvItem. Coin
// entries get a rolled gold value; named items become treasure with a
// tier-scaled placeholder value. Returns ok=false for entries we should
// silently skip (none today, but the contract leaves room for future
// quest-only entries that don't materialize as inventory rows).
func zoneLootToInventory(entry ZoneLootEntry, zone ZoneDefinition, rng *rand.Rand) (AdvItem, bool) {
	if strings.HasPrefix(entry.ItemID, "coins_") {
		val := rollCoinPattern(entry.ItemID, rng)
		return AdvItem{
			Name:  "Coin Pouch",
			Type:  "treasure",
			Tier:  int(zone.Tier),
			Value: int64(val),
		}, true
	}
	tier := int(zone.Tier)
	displayName := titleCaseUnderscored(entry.ItemID)
	value := int64(50 * tier * tier) // T1=50, T2=200, T3=450, T4=800, T5=1250
	if entry.UniqueAlways {
		value = int64(100 * tier * tier)
	}
	return AdvItem{
		Name:  displayName,
		Type:  "treasure",
		Tier:  tier,
		Value: value,
	}, true
}

// rollCoinPattern parses "coins_<n>d<sides>x<mult>" — e.g.
// "coins_2d10x5" → 2d10 × 5. Bad input falls back to a tier-agnostic
// 25-coin floor so the player still gets *something* and the bug is
// loud in logs but not run-ending.
func rollCoinPattern(id string, rng *rand.Rand) int {
	body := strings.TrimPrefix(id, "coins_")
	xParts := strings.SplitN(body, "x", 2)
	if len(xParts) != 2 {
		slog.Warn("zone: bad coin pattern (no x)", "id", id)
		return 25
	}
	mult, err := atoi(xParts[1])
	if err != nil || mult <= 0 {
		slog.Warn("zone: bad coin pattern mult", "id", id)
		return 25
	}
	dice := strings.SplitN(xParts[0], "d", 2)
	if len(dice) != 2 {
		slog.Warn("zone: bad coin pattern dice", "id", id)
		return 25
	}
	n, err := atoi(dice[0])
	if err != nil || n <= 0 {
		slog.Warn("zone: bad coin pattern n", "id", id)
		return 25
	}
	sides, err := atoi(dice[1])
	if err != nil || sides <= 0 {
		slog.Warn("zone: bad coin pattern sides", "id", id)
		return 25
	}
	roll := 0
	for i := 0; i < n; i++ {
		roll += rng.IntN(sides) + 1
	}
	return roll * mult
}

// atoi — local strconv-free parser to avoid a single-call import.
func atoi(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit %q", r)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// titleCaseUnderscored renders "wpn_handaxe_+1" → "Wpn Handaxe +1".
// Pure ASCII; no Unicode handling needed for our zone-loot ID space.
func titleCaseUnderscored(id string) string {
	parts := strings.Split(id, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		c0 := p[0]
		if c0 >= 'a' && c0 <= 'z' {
			parts[i] = string(c0-32) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
