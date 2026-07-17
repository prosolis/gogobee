package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// actionPicker tracks used indices per pool to avoid repeats within a fight.
type actionPicker struct {
	enemy       map[int]bool
	player      map[int]bool
	playerMiss  map[int]bool
	enemyMiss   map[int]bool
	block       map[int]bool
	environment map[int]bool
}

func newActionPicker() *actionPicker {
	return &actionPicker{
		enemy:       make(map[int]bool),
		player:      make(map[int]bool),
		playerMiss:  make(map[int]bool),
		enemyMiss:   make(map[int]bool),
		block:       make(map[int]bool),
		environment: make(map[int]bool),
	}
}

func pickFrom(pool []string, used map[int]bool, damage int) string {
	if len(used) >= len(pool) {
		for k := range used {
			delete(used, k)
		}
	}
	idx := rand.IntN(len(pool))
	for used[idx] {
		idx = (idx + 1) % len(pool)
	}
	used[idx] = true
	return fmt.Sprintf(pool[idx], damage)
}

func pickFromNoFmt(pool []string, used map[int]bool) string {
	if len(used) >= len(pool) {
		for k := range used {
			delete(used, k)
		}
	}
	idx := rand.IntN(len(pool))
	for used[idx] {
		idx = (idx + 1) % len(pool)
	}
	used[idx] = true
	return pool[idx]
}

// RenderCombatLog converts a CombatResult into a slice of messages — one per
// phase plus a final outcome message. The caller sends them with 5-8 second
// delays between messages to create an unfolding fight feel.
func RenderCombatLog(result CombatResult, playerName, enemyName string) []string {
	picker := newActionPicker()
	phases := groupEventsByPhase(result.Events)

	var msgs []string
	for _, pg := range phases {
		msg := renderPhaseBlock(pg, playerName, enemyName, result, picker)
		if msg != "" {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

type phaseGroup struct {
	Name   string
	Events []CombatEvent
}

func groupEventsByPhase(events []CombatEvent) []phaseGroup {
	var groups []phaseGroup
	var current *phaseGroup

	for _, e := range events {
		if current == nil || current.Name != e.Phase {
			groups = append(groups, phaseGroup{Name: e.Phase})
			current = &groups[len(groups)-1]
		}
		current.Events = append(current.Events, e)
	}
	return groups
}

func renderPhaseBlock(pg phaseGroup, playerName, enemyName string, result CombatResult, picker *actionPicker) string {
	var sb strings.Builder

	header := phaseHeader(pg.Name)
	sb.WriteString(header + "\n")

	// Walk events with a 3+ consecutive-trivial collapse: long runs of
	// "miss / miss / miss" get one TwinBee yadda-yadda line instead of N
	// individual lines. Keeps boss fights (16 rounds) from drowning in
	// repetition while still rendering every impactful beat.
	i := 0
	for i < len(pg.Events) {
		e := pg.Events[i]
		if isTrivialEvent(e) {
			run := 1
			for i+run < len(pg.Events) && isTrivialEvent(pg.Events[i+run]) {
				run++
			}
			if run >= 3 {
				sb.WriteString(pickRand(narrativeFillerSlog) + "\n")
				i += run
				continue
			}
		}
		line := renderEvent(e, playerName, enemyName, result, picker)
		if line != "" {
			if roll := rollAnnotation(e); roll != "" {
				line = line + " " + roll
			}
			sb.WriteString(line + "\n")
		}
		i++
	}

	if len(pg.Events) == 0 {
		return ""
	}
	lastEvent := pg.Events[len(pg.Events)-1]
	sb.WriteString(fmt.Sprintf("  [%s: %d/%d | %s: %d/%d]",
		playerName, clampHP(lastEvent.PlayerHP), result.PlayerStartHP,
		enemyName, clampHP(lastEvent.EnemyHP), result.EnemyStartHP))

	return sb.String()
}

// isTrivialEvent — events that don't move HP or set up a tactical hook.
// A run of 3+ of these collapses into a TwinBee filler line.
func isTrivialEvent(e CombatEvent) bool {
	switch e.Action {
	case "miss", "spore_miss", "pet_whiff":
		return true
	}
	return false
}

// narrativeFillerSlog covers a run of consecutive trivial events
// (misses, whiffs) so long boss fights don't read as a wall of
// "you swing wide / they swing wide" lines.
var narrativeFillerSlog = []string{
	"_The exchange settles into a rhythm. A few rounds of nothing meaningful — I skip to the next thing that matters._",
	"_I narrate the next few seconds in a single sigh. Both sides keep swinging. Both sides keep missing._",
	"_And the battle goes on. Yadda yadda yadda. I will narrate the next part where something actually lands._",
	"_The grind continues. I use the time to mentally redesign the dungeon's lighting._",
	"_A flurry of misses on both sides. Nobody is impressing anybody. I wait for the next hit._",
	"_I politely decline to narrate this stretch. It was, in my professional opinion, a waste of everyone's time._",
}

func clampHP(hp int) int {
	if hp < 0 {
		return 0
	}
	return hp
}

// rollAnnotation returns a compact d20 tag for events that came from the
// d20-vs-AC resolution path. Empty for ability/heal/consumable events
// where Roll is unset. Format: "_(🎲 14 vs AC 13)_" — italicized so the
// dice fact reads as a sidebar to the narrative line.
func rollAnnotation(e CombatEvent) string {
	if e.Roll <= 0 {
		return ""
	}
	switch e.Action {
	case "hit", "crit", "miss", "block":
		// d20 contests — annotate with roll vs target AC.
	default:
		return ""
	}
	if e.RollAgainst > 0 {
		return fmt.Sprintf("_(🎲 %d vs AC %d)_", e.Roll, e.RollAgainst)
	}
	return fmt.Sprintf("_(🎲 %d)_", e.Roll)
}

func phaseHeader(name string) string {
	switch name {
	case "Opening", "opening":
		return pickRand(openingHeaders)
	case "Clash", "clash":
		return pickRand(clashHeaders)
	case "Decisive", "decisive":
		return pickRand(decisiveHeaders)
	case "pre_combat", "pre":
		return "⚔️ **The fight begins.**"
	case "Sudden Death", "sudden_death":
		return "⚔️ **Sudden Death — No Quarter**"
	case "exhaust":
		return "⚔️ **Exhaustion — Time Runs Out**"
	default:
		return "⚔️ **" + name + "**"
	}
}

func renderEvent(e CombatEvent, playerName, enemyName string, result CombatResult, picker *actionPicker) string {
	switch e.Action {
	case "hit":
		if e.Actor == "player" {
			return pickFrom(narrativePlayerHit, picker.player, e.Damage)
		}
		return pickFrom(narrativeEnemyHit, picker.enemy, e.Damage)

	case "crit":
		if e.Actor == "player" {
			if e.Desc == "auto_crit" {
				return pickFrom(narrativePlayerAutoCrit, picker.player, e.Damage)
			}
			return pickFrom(narrativePlayerCrit, picker.player, e.Damage)
		}
		return pickFrom(narrativeEnemyCrit, picker.enemy, e.Damage)

	case "block":
		if e.Actor == "player" {
			return fmt.Sprintf(pickRand(narrativeEnemyBlock), e.Damage)
		}
		return fmt.Sprintf(pickRand(narrativePlayerBlock), e.Damage)

	case "miss":
		if e.Actor == "player" {
			return pickFromNoFmt(narrativePlayerMiss, picker.playerMiss)
		}
		return pickFromNoFmt(narrativeEnemyMiss, picker.enemyMiss)

	case "pet_attack":
		return fmt.Sprintf(pickRand(narrativePetAttack), e.Damage)

	case "spirit_weapon_strike":
		return fmt.Sprintf(pickRand(narrativeSpiritWeapon), e.Damage)

	case "concentration_tick":
		return fmt.Sprintf(pickRand(narrativeConcentrationTick), e.Damage)

	case "cantrip":
		// e.Desc is the spell name (Fire Bolt / Eldritch Blast); e.Damage the hit.
		return fmt.Sprintf(pickRand(narrativeCantrip), e.Desc, e.Damage)

	case "pet_deflect":
		return pickRand(narrativePetDeflect)

	case "pet_whiff":
		return pickRand(narrativePetWhiff)

	case "sniper_kill":
		return pickRand(narrativeSniperKill)

	case "misty_heal":
		return fmt.Sprintf(pickRand(narrativeMistyHeal), e.Damage)

	case "death_save":
		return pickRand(narrativeDeathSave)

	case "environmental":
		return pickFrom(narrativeEnvironmental, picker.environment, e.Damage)

	case "use_consumable":
		return renderConsumableUse(e.Desc)

	case "consumable_skip":
		return "_Supplies conserved — threat assessed as manageable._"

	case "ward_absorb":
		return pickRand(narrativeWardAbsorb)

	case "spore_miss":
		return pickRand(narrativeSporeCloud)

	case "reflect_damage":
		return fmt.Sprintf(pickRand(narrativeReflect), e.Damage)

	case "crowd_revenge":
		return fmt.Sprintf(pickRand(narrativeCrowdRevenge), e.Damage)

	case "flat_damage":
		return fmt.Sprintf(pickRand(narrativeFlatDmg), e.Damage)

	case "heal_item":
		return fmt.Sprintf(pickRand(narrativeHealItem), e.Damage)

	// Monster abilities
	case "poison_tick":
		return fmt.Sprintf(pickRand(narrativePoisonTick), e.Damage)
	case "enrage":
		return pickRand(narrativeEnrage)
	case "armor_break":
		return pickRand(narrativeArmorBreak)
	case "stun":
		return pickRand(narrativeStun)
	case "stunned":
		return pickRand(narrativeStunned)
	case "poison":
		return pickRand(narrativePoisonApply)
	case "lifesteal":
		return fmt.Sprintf(pickRand(narrativeLifesteal), e.Damage)
	case "cleave":
		return fmt.Sprintf(pickRand(narrativeCleave), e.Damage)
	case "bonus_damage":
		return fmt.Sprintf(pickRand(narrativeBonusDamage), e.Damage)
	case "aoe":
		return fmt.Sprintf(pickRand(narrativeAoE), e.Damage)
	case "execute":
		return fmt.Sprintf(pickRand(narrativeExecute), e.Damage)
	case "self_heal":
		return fmt.Sprintf(pickRand(narrativeSelfHeal), e.Damage)
	case "ability_flavor":
		return pickRand(narrativeAbilityFlavor)

	// Monster abilities — slice 3 stateful effects
	case "evade":
		return pickRand(narrativeEvade)
	case "parry_stance":
		return pickRand(narrativeParryStance)
	case "advantage":
		return pickRand(narrativeAdvantage)
	case "retaliate_aura":
		return pickRand(narrativeRetaliateAura)
	case "retaliate":
		return fmt.Sprintf(pickRand(narrativeRetaliate), e.Damage)
	case "regenerate":
		return pickRand(narrativeRegenerate)
	case "regen_tick":
		return fmt.Sprintf(pickRand(narrativeRegenTick), e.Damage)
	case "survive_armed":
		return pickRand(narrativeSurviveArmed)
	case "survive_at_1":
		return pickRand(narrativeSurvive)
	case "phylactery_rebirth":
		return pickRand(narrativePhylacteryRebirth)
	case "amendment_rewind":
		return pickRand(narrativeAmendmentRewind)
	case "midnight_toll":
		return fmt.Sprintf(pickRand(narrativeMidnightToll), e.Damage)
	case "inversion_telegraph":
		return pickRand(narrativeInversionTelegraph)
	case "inversion_stitch":
		return pickRand(narrativeInversionStitch)
	case "heal_inverted":
		return fmt.Sprintf(pickRand(narrativeHealInverted), e.Damage)
	case "stat_drain":
		return fmt.Sprintf(pickRand(narrativeStatDrain), e.Damage)
	case "debuff":
		return fmt.Sprintf(pickRand(narrativeDebuff), e.Damage)
	case "max_hp_drain":
		return fmt.Sprintf(pickRand(narrativeMaxHPDrain), e.Damage)

	// Monster abilities — slice 4 (former flavor-only placeholders)
	case "spell_resist":
		return pickRand(narrativeSpellResist)
	case "spell_fizzle":
		return fmt.Sprintf(pickRand(narrativeSpellFizzle), e.Damage)
	case "reveal_armed":
		return pickRand(narrativeRevealArmed)
	case "revealed":
		return pickRand(narrativeRevealed)
	case "fear_immune":
		return pickRand(narrativeFearImmune)
	case "fear_resist":
		return pickRand(narrativeFearResist)
	case "ally_buff":
		return fmt.Sprintf(pickRand(narrativeAllyBuff), e.Damage)

	case "timeout":
		return pickRand(narrativeTimeout)

	default:
		return ""
	}
}

func renderConsumableUse(itemName string) string {
	templates := []string{
		"You crack open a **%s**. The fight just got interesting.",
		"**%s** consumed. Its effects take hold immediately.",
		"You down a **%s** before things get worse.",
		"The **%s** does its work. You feel the difference.",
	}
	return fmt.Sprintf(templates[rand.IntN(len(templates))], itemName)
}

func renderOutcome(result CombatResult, playerName, enemyName string, reward int64, xp int) string {
	var sb strings.Builder

	if result.PlayerWon {
		sb.WriteString(fmt.Sprintf("💀 **%s** has been defeated.\n", enemyName))
		if result.NearDeath {
			sb.WriteString(pickRand(narrativeNearDeathWin) + "\n")
		} else if result.Closeness < 0.3 {
			sb.WriteString(pickRand(narrativeDominantWin) + "\n")
		} else {
			sb.WriteString(pickRand(narrativeCleanWin) + "\n")
		}
		sb.WriteString(fmt.Sprintf("🏆 +%d XP | €%d earned", xp, reward))
	} else {
		sb.WriteString("💀 **Defeated.**\n")
		if result.NearDeath {
			sb.WriteString(pickRand(narrativeNearDeathLoss) + "\n")
		} else if result.Closeness < 0.3 {
			sb.WriteString(pickRand(narrativeDominantLoss) + "\n")
		} else {
			sb.WriteString(pickRand(narrativeCloseLoss) + "\n")
		}
		sb.WriteString(fmt.Sprintf("+%d XP (participation)", xp))
	}

	return sb.String()
}

func pickRand(pool []string) string {
	return pool[rand.IntN(len(pool))]
}

// ── Phase Headers ────────────────────────────────────────────────────────────

var openingHeaders = []string{
	"⚔️ **Opening — The First Exchange**",
	"⚔️ **Opening — Testing the Waters**",
	"⚔️ **Opening — Both Sides Size Each Other Up**",
	"⚔️ **Opening — The Fight Begins**",
}

var clashHeaders = []string{
	"⚔️ **Clash — The Real Fight**",
	"⚔️ **Clash — No More Warm-Ups**",
	"⚔️ **Clash — Steel Meets Steel**",
	"⚔️ **Clash — The Exchange Intensifies**",
}

var decisiveHeaders = []string{
	"⚔️ **Decisive — One of You Isn't Walking Away**",
	"⚔️ **Decisive — The Final Blow**",
	"⚔️ **Decisive — Everything on the Line**",
	"⚔️ **Decisive — This Ends Now**",
}

// ── Narrative Pools ──────────────────────────────────────────────────────────

var narrativePlayerHit = []string{
	"You connect cleanly. %d damage. Nothing fancy. Functional violence.",
	"A solid strike lands true. %d damage. The enemy reconsiders their career choices.",
	"You find an opening and take it. %d damage. The opening did not volunteer.",
	"Your weapon finds its mark. %d damage. Your weapon has better aim than you do most days.",
	"You press the advantage. %d damage. The advantage was already pressed. You pressed it further.",
	"A measured strike. %d damage. Nothing flashy. Just effective. The crowd is politely impressed.",
	"You swing and it connects where it matters. %d damage. Where it matters is the enemy's face.",
	"You hit them with a move you've been mentally rehearsing for weeks. %d damage. It worked. You're as surprised as they are.",
	"You land a clean hit for %d damage and immediately start explaining to no one how you did that.",
	"You score %d damage. The enemy seems fine. You are less fine about this than they are.",
}

var narrativeEnemyHit = []string{
	"The enemy strikes back. %d damage. They were not asking permission.",
	"A hit gets through your guard. %d damage. Your guard had one job.",
	"The enemy finds a gap in your defense. %d damage. The gap is now a feature.",
	"You take a hit. %d damage. It stings. Your pride stings more.",
	"The enemy's weapon connects with something important. %d damage. It was you. You were the important thing.",
	"A blow you didn't see coming. %d damage. In fairness, you weren't looking.",
	"The enemy is faster than expected. %d damage. Your expectations need recalibrating.",
	"The enemy questions your life choices mid-swing. You pause to reflect. %d damage during the pause.",
	"The enemy yawns before hitting you. Not performatively. Genuinely. %d damage while you process the disrespect.",
	"The enemy hits you with what is technically the bare minimum of effort. %d damage. You gave it everything. They did not.",
}

var narrativePlayerCrit = []string{
	"💥 **CRITICAL HIT.** You drive the blow home. %d damage. The crowd notices. The enemy notices more.",
	"💥 **CRIT!** Everything lined up perfectly. %d damage. You will never reproduce this.",
	"💥 A devastating strike. %d damage. You didn't know you had that in you. Neither did they.",
	"💥 You put everything behind this one. %d damage. It shows. It shows on their face specifically.",
	"💥 **CRIT!** %d damage. You look at your weapon like it just got a promotion.",
}

// narrativePlayerAutoCrit is used when a forced/auto crit fires *not* from a
// natural high roll — Rogue's first-strike instinct, a held/paralyzed enemy,
// or a consumable like Ancient Artifact Oil. The roll itself is unimpressive;
// the *setup* is what made the hit devastating.
var narrativePlayerAutoCrit = []string{
	"💥 **CRIT — opening exploit.** Years of looking for this exact gap pay off. %d damage. The roll didn't matter. You did.",
	"💥 Your training takes over before you do. The opening was there; instinct found it. %d damage.",
	"💥 The enemy left exactly the gap your subclass trained you to punish. %d damage. They did not get to protest.",
	"💥 **First-strike instinct fires.** You don't aim; you *know*. %d damage. The dice were a formality.",
	"💥 You read the seam between their guard and their breath. Slip in, slip out. %d damage. They notice on the way down.",
	"💥 The setup was perfect — the *attack* was just paperwork. %d damage.",
}

var narrativeEnemyCrit = []string{
	"💥 **CRITICAL HIT** from the enemy. %d damage. That one rearranged something.",
	"💥 The enemy finds a weak point you didn't know you had. %d damage. Now you know.",
	"💥 A brutal strike. %d damage. You feel that one in places you didn't know could feel things.",
	"💥 The enemy's eyes narrow before a devastating blow. %d damage. The narrowing was a warning. You missed it.",
	"💥 **CRIT.** %d damage. The enemy doesn't even look impressed with themselves. That's worse.",
}

var narrativeDodge = []string{
	"You sidestep at the last moment. Nothing lands. You style it out. Nobody is convinced.",
	"The attack passes through empty air. Clean dodge. You had no idea you could move like that.",
	"You read the movement and step aside. The enemy's weapon finds nothing but disappointment.",
	"Too slow. The strike finds nothing. The enemy retrieves their dignity from the floor.",
	"A dodge so clean it almost looks rehearsed. It was not. You were running away and it happened to work.",
	"You duck. The enemy's strike passes exactly where your head was. You both take a moment to appreciate how close that was.",
}

var narrativePlayerBlock = []string{
	"You brace and absorb the impact. Reduced to %d damage. Your arms disagree with this strategy.",
	"Your guard holds. Only %d damage gets through. The rest is your problem later.",
	"A solid block, but some force still connects. %d damage. Physics remains undefeated.",
	"You block it. %d damage still gets through. Blocking is more of a suggestion than a solution.",
}

var narrativeEnemyBlock = []string{
	"The enemy blocks your strike. Only %d damage penetrates. They seem unimpressed.",
	"A partial block. %d damage — less than you wanted. Less than they deserved.",
	"The enemy's defense absorbs most of it. %d damage. The enemy's forearm speaks to the pathetic nature of your striking.",
}

var narrativePlayerMiss = []string{
	"Your swing goes wide. Nothing connects. The air you hit had it coming.",
	"The enemy anticipated that one. Complete miss. They're already bored.",
	"You overcommit and hit nothing but air. The air files no complaint.",
	"A whiff. It happens. It happens to you more than average.",
	"You attempt a move you saw in a film once. It does not work like in the film.",
	"You close your eyes for the strike because it feels more dramatic. You miss. Everything about this was predictable.",
}

var narrativeEnemyMiss = []string{
	"The enemy lunges. Finds only air. The air takes the slight personally.",
	"Their strike misses by inches. They curse in a language you don't recognize.",
	"A wild swing, deflected by your stance. They expected better of themselves.",
	"The blow comes in too predictable. You're already gone. They commit. They miss.",
	"The enemy whiffs. The look on their face — what passes for one — is priceless.",
	"Their weapon meets your guard, slides past harmless. They've used this combo before. It worked then.",
	"The strike is fast. You are slightly faster. The math works out in your favor for once.",
}

var narrativePetAttack = []string{
	"🐾 Your pet lunges at the enemy from absolutely nowhere. %d damage. Your pet has been waiting for this.",
	"🐾 Your pet strikes from the side with zero hesitation. %d additional damage. The enemy was not monitoring the pet. Mistake.",
	"🐾 Your pet joins the fray with a well-timed attack. %d damage. The timing was suspicious. Your pet may be smarter than you.",
	"🐾 Your faithful companion lands a hit for %d damage. More faithful than accurate, but today both applied.",
}

var narrativeSpiritWeapon = []string{
	"✨ The spectral mace swings on its own and lands for %d damage. Floating menace, well-balanced.",
	"✨ Your spiritual weapon hovers, picks an angle, strikes — %d damage. No grip, all conviction.",
	"✨ A glowing weapon arcs in from beside you. %d damage. The enemy keeps trying to track it. Cannot.",
	"✨ The spectral blade flickers, then bites. %d damage. It does not tire. It does not blink.",
}

var narrativeConcentrationTick = []string{
	"🌀 The lingering aura grinds the enemy down — %d damage. Still humming. Still hungry.",
	"🌀 Your spell hasn't let go: the spirits sweep through again for %d damage.",
	"🌀 The radiant field pulses once more — %d damage. Concentration holds.",
	"🌀 The enemy steps wrong and the standing magic answers, %d damage. It does not move on.",
}

// narrativeCantrip fires each round an arcane blaster throws its at-will cantrip
// (Fire Bolt / Eldritch Blast) before the weapon swing. %s is the spell name,
// %d the damage — a caster's sustained floor, so it lands every round.
var narrativeCantrip = []string{
	"✨ %s streaks out and burns home — %d damage. The at-will floor never stops.",
	"✨ A bolt of %s answers before the staff even moves, scorching for %d.",
	"✨ %s lances the enemy for %d. No incantation, no wind-up — just the steady arcane drum.",
}

var narrativePetDeflect = []string{
	"🐾 Your pet intercepts the blow. Damage halved. Your pet is now your best piece of equipment.",
	"🐾 Your pet pushes you aside at the last second. Impact reduced. You did not ask to be pushed. Results speak for themselves.",
	"🐾 Your companion takes part of the hit for you. This is more loyalty than you deserve.",
}

var narrativePetWhiff = []string{
	"🐾 Your pet distracts the enemy completely. Their attack goes wide. Your pet is not sorry.",
	"🐾 Your pet startles the enemy mid-swing. Complete miss. The enemy glares at the pet. The pet does not care.",
	"🐾 The enemy flinches at your pet's sudden movement. Missed entirely. Your pet sits back down like nothing happened.",
}

var narrativeSniperKill = []string{
	"🎯 A shot rings out from the shadows. The enemy drops before the fight even starts. Arina collects her fee. She was leaving anyway.",
	"🎯 Arina's shot finds its mark. The enemy never knew what happened. Arina is already gone.",
	"🎯 One shot. One kill. Arina waves from somewhere you can't see. You wave back. She's already somewhere else.",
}

var narrativeMistyHeal = []string{
	"🌿 Misty's presence soothes your wounds. +%d HP. She makes it look effortless because for her it is.",
	"🌿 A warm glow surrounds you. Misty restores %d HP. She doesn't explain how. You don't ask.",
	"🌿 Misty whispers something. You feel %d HP return. Whatever she said, it worked.",
	"🌿 The air shimmers. Misty heals you for %d HP. The enemy watches this happen and seems annoyed about it.",
}

var narrativeDeathSave = []string{
	"👑 The Sovereign set flares with light. You refuse to fall. 1 HP remains. You should be dead. You are not. The set has opinions about your mortality.",
	"👑 Something keeps you standing. The Sovereign set burns bright. 1 HP. The enemy is visibly upset about this.",
	"👑 You should be dead. The Sovereign set disagrees. 1 HP. The disagreement is final.",
}

var narrativeEnvironmental = []string{
	"The arena floor shifts beneath you. %d damage. The floor has no allegiance.",
	"A loose stone catches you off-balance. %d damage. The arena's maintenance budget is your problem now.",
	"Something in the environment works against you. %d damage. The environment was never on your side.",
	"The terrain betrays you. %d damage. It was never loyal to begin with.",
	"A hazard you didn't notice. %d damage. In fairness, you were busy being hit by other things.",
	"A bird lands between you and the enemy. Both combatants stop. The bird leaves. The enemy recovers first. %d damage.",
	"Someone in the crowd drops their drink. The sound is startling. You both flinch. The enemy flinches smaller. %d damage.",
}

var narrativeWardAbsorb = []string{
	"✨ The Quartz Ward flares. The hit is absorbed completely. Money well spent.",
	"✨ Your ward shatters — but the damage doesn't reach you. One and done. Worth it.",
	"✨ The ward takes the hit. You don't. The ward has no opinions about this arrangement.",
}

var narrativeSporeCloud = []string{
	"🍄 The spore cloud confuses the enemy. They swing at nothing. The nothing is grateful.",
	"🍄 Spores fill the air. The enemy can't find you. You're right here. They still can't find you.",
	"🍄 The enemy chokes on spores and misses entirely. They will have questions about this later.",
}

var narrativeReflect = []string{
	"💎 The Voidstone Shard activates. %d damage reflected back. The enemy hit themselves, technically.",
	"💎 The enemy's own force is turned against them. %d reflected. They did this to themselves.",
	"💎 Half the blow bounces back. %d damage to the enemy. Karma. Immediate karma.",
}

var narrativeTimeout = []string{
	"The fight drags on. Neither side will yield. The judges decide on points.",
	"Exhaustion sets in. Both fighters are spent. The one still standing taller takes it.",
	"Time runs out. The crowd is restless. The decision falls to whoever bled less.",
}

var narrativeCrowdRevenge = []string{
	"🪨 The crowd throws something at you. %d damage. Misty's fans hold grudges.",
	"🪨 Someone in the stands got a clear shot. %d damage. You should have tipped better.",
	"🪨 The crowd is not on your side today. %d damage. Misty sends her regards.",
}

var narrativeFlatDmg = []string{
	"💣 The Coal Bomb detonates. %d damage before the fight even starts. The enemy was not ready. That was the point.",
	"💣 An explosion kicks things off. %d damage to the enemy. First impressions matter.",
}

var narrativeHealItem = []string{
	"🧪 You feel the potion take effect. +%d HP restored. Tastes terrible. Works perfectly.",
	"🧪 The salve kicks in. +%d HP. You don't know what's in it. You don't want to know.",
	"🧪 Your consumable heals you for %d HP mid-fight. The enemy watches you heal and seems personally offended.",
}

// Monster abilities
var narrativePoisonApply = []string{
	"☠️ The enemy's attack carries something extra. You feel it immediately. Poison.",
	"☠️ A venomous strike. The wound burns differently than it should. That's not good.",
	"☠️ Something coats the enemy's weapon. It's in you now. The burning starts.",
}

var narrativePoisonTick = []string{
	"☠️ Venom courses through you. %d poison damage. Your blood has opinions about this.",
	"☠️ The poison burns. %d damage. It's not getting better on its own.",
	"☠️ You feel the toxin working. %d damage this round. Time is not on your side.",
}

var narrativeEnrage = []string{
	"🔥 The enemy's eyes go red. Something has changed. They're stronger now. You preferred them before.",
	"🔥 **ENRAGE.** The enemy is wounded and furious. Attack increased. This was not the plan.",
	"🔥 Cornered and desperate, the enemy unleashes everything. Desperate enemies are the most dangerous kind.",
}

var narrativeArmorBreak = []string{
	"🛡️💥 Your armor cracks under the impact. Defense reduced. That sound was expensive.",
	"🛡️💥 The enemy shatters your guard. Your defense won't hold like it did. Neither will your confidence.",
	"🛡️💥 **Armor Break.** You feel exposed. Because you are.",
}

var narrativeStun = []string{
	"⚡ The enemy winds up something that looks bad. It is bad.",
	"⚡ The enemy prepares a stunning blow. Your immediate future just got complicated.",
	"⚡ The enemy's weapon crackles with intent. This is going to hurt differently.",
}

var narrativeStunned = []string{
	"⚡ You're stunned. Can't move. Can't attack. Can barely think.",
	"⚡ The blow leaves you reeling. You lose your turn. You lose some dignity too.",
	"⚡ Everything goes white for a moment. You skip your attack. The moment passes. The opportunity does not return.",
}

var narrativeLifesteal = []string{
	"🩸 The enemy drains your vitality. %d damage — and they heal. This is deeply unfair.",
	"🩸 You feel your strength being siphoned. %d damage, enemy heals. They're taking what's yours.",
	"🩸 **Lifesteal.** The enemy grows stronger as you weaken. %d damage. The math is going in the wrong direction.",
}

var narrativeCleave = []string{
	"⚔️⚔️ The enemy strikes twice in rapid succession. %d damage from the double blow. That should not be legal.",
	"⚔️⚔️ **Cleave!** Two strikes before you can react. %d damage total. One hit was enough. They did two.",
	"⚔️⚔️ A devastating combo. %d damage from both hits. The second one was personal.",
}

var narrativeBonusDamage = []string{
	"💢 The enemy finds an opening you didn't know you'd left. %d extra damage. Lesson noted.",
	"💢 A second strike slips past your guard before the first one finished. %d damage on top.",
	"💢 The enemy presses the advantage — %d more damage than the round had any right to deal.",
}

var narrativeAoE = []string{
	"💥 The attack erupts outward. %d damage, and your armor barely slows it. Nowhere to dodge.",
	"💥 A burst of force washes over you. %d damage — there was no good way to stand for that.",
	"💥 The enemy unleashes something wide and indiscriminate. %d damage. Cover would have been nice.",
}

var narrativeExecute = []string{
	"☠️ The enemy goes for the kill — %d damage aimed squarely at finishing you.",
	"☠️ A finishing blow. %d damage. The enemy can smell the end of this fight.",
	"☠️ The enemy commits everything to one last strike. %d damage. They want this over.",
}

var narrativeSelfHeal = []string{
	"✨ The enemy knits its wounds closed. +%d HP. That's going to make this longer.",
	"✨ The enemy mends itself before your eyes. +%d HP restored. Rude.",
	"✨ Something restorative passes over the enemy. +%d HP. The progress bar moved the wrong way.",
}

var narrativeAbilityFlavor = []string{
	"🌀 The enemy does *something* — you feel the shape of it more than the effect. Stay wary.",
	"🌀 The air shifts around the enemy. Whatever that was, it wasn't nothing.",
	"🌀 The enemy invokes a power you can't quite read. File it under 'concerning'.",
}

var narrativeEvade = []string{
	"💨 Your strike lands on nothing — the enemy was never quite where it looked.",
	"💨 The enemy slips the blow. Your weapon finds only the air it left behind.",
	"💨 A clean swing, a clean miss. The enemy ghosts aside at the last instant.",
}

var narrativeParryStance = []string{
	"🛡️ The enemy settles into a tight defensive guard. Hits are going to come harder now.",
	"🛡️ The enemy raises its guard and *means* it — every blow from here will have to earn it.",
	"🛡️ The enemy shifts its stance, weapon angled to turn your strikes aside.",
}

var narrativeAdvantage = []string{
	"🎯 The enemy reads your rhythm. Its strikes start coming with unsettling certainty.",
	"🎯 Something clicks for the enemy — it's anticipating you now, and it shows.",
	"🎯 The enemy presses an advantage you can't quite see. Its aim has sharpened.",
}

var narrativeRetaliateAura = []string{
	"🔥 The enemy flares with a punishing aura. Hitting it is about to cost you.",
	"🔥 A searing field wraps the enemy — every blow you land will bite back.",
	"🔥 The enemy wreathes itself in retaliation. Strike it and you share the pain.",
}

var narrativeRetaliate = []string{
	"🔥 The enemy's aura lashes back — %d damage for the privilege of hitting it.",
	"🔥 Your strike lands, and the recoil burns. %d damage bounces straight back into you.",
	"🔥 %d damage reflected. The enemy made you pay for that hit in real time.",
}

var narrativeRegenerate = []string{
	"♻️ The enemy's wounds begin to close on their own. This just got slower.",
	"♻️ Torn flesh knits and seals. The enemy has started regenerating.",
	"♻️ The enemy's body refuses to stay damaged — it's healing between blows now.",
}

var narrativeRegenTick = []string{
	"♻️ The enemy mends another %d HP. The damage you're doing keeps un-doing itself.",
	"♻️ +%d HP for the enemy as its wounds seal over. Frustrating.",
	"♻️ The enemy claws back %d HP. You'll have to out-pace the regeneration.",
}

var narrativeSurviveArmed = []string{
	"🕯️ The enemy refuses the idea of dying. Something keeps it on its feet past where it should fall.",
	"🕯️ A grim resilience settles over the enemy — it will not go down easy.",
	"🕯️ The enemy digs in. Whatever's holding it together, it isn't ready to let go.",
}

var narrativeSurvive = []string{
	"🕯️ The killing blow lands clean — and the enemy *stays standing* at 1 HP. Unbelievable.",
	"🕯️ That should have ended it. The enemy clings to a single point of HP through sheer spite.",
	"🕯️ The enemy by all rights should be down. It is, instead, very barely up.",
}

// narrativePhylacteryRebirth fires when Valdris burns a Verse the player left
// un-found: a bound rebirth spends and the lich reassembles. Each line reads as
// "you skipped one of these" so the mechanic teaches itself over a wipe.
var narrativePhylacteryRebirth = []string{
	"💀 The lich comes apart — and a Verse you never found sings him back together. He rises, unhurried.",
	"💀 Bone-dust swirls up off the floor and re-seats itself. A rebirth you didn't unbind just spent itself. He stands.",
	"💀 That should have been the end of him. A Verse still hums somewhere in the cathedral, and Valdris simply *begins again*.",
}

// narrativeAmendmentRewind fires when the Custodian rewinds itself to its round-3
// HP snapshot — the once-only Amendment. Each line reads as time being undone so
// the mechanic (front-loaded burst is partly refunded) teaches itself over a run.
var narrativeAmendmentRewind = []string{
	"🕰️ The Custodian raises a hand and *edits the last few minutes out of the record.* Wounds close in reverse; the clock-golem stands as it did rounds ago.",
	"🕰️ \"That entry is amended.\" The damage you dealt simply un-happens — the Custodian rewinds to where it was and resumes, unhurried.",
	"🕰️ Verdigris rings spin backward. Time you spent hurting it is refunded to the golem; it returns to its round-three self and keeps working.",
}

// narrativeMidnightToll fires past round 20 — the soft closing-time timer, the
// Custodian's Attack climbing each round a stalled fight refuses to end.
var narrativeMidnightToll = []string{
	"🔔 A bell tolls somewhere above. Closing time — the Custodian's swings come harder. (+%d attack)",
	"🔔 The hour is nearly spent, and so is its patience; each blow lands with more weight now. (+%d attack)",
}

// narrativeInversionTelegraph fires one round before an Inversion Stitch pulse —
// the Seamstress's tell. It reads as a warning so a player learns to hold heals.
var narrativeInversionTelegraph = []string{
	"🧵 The Seamstress draws a thread taut and the room *shivers* — walls flexing toward inside-out. Whatever you were about to mend, hold it. (next round, healing turns against you)",
	"🧵 A seam in the air puckers. The geometry is about to flip; a cure cast into it will run backward. (inversion incoming next round)",
}

// narrativeInversionStitch fires when a pulse activates — the room is sewn
// inside-out and healing now wounds for the pulse's duration.
var narrativeInversionStitch = []string{
	"🧵 The stitch pulls through. The room is inside-out now — for a moment, to heal is to hurt.",
	"🧵 Everything turns wrong-way-round. Mending and wounding have swapped ends of the needle.",
}

// narrativeHealInverted fires each time a heal lands during an active pulse: the
// cure runs backward and stings instead. Teaches the mechanic on the spot.
var narrativeHealInverted = []string{
	"🧵 The heal runs backward through the inverted room — the cure opens the wound it meant to close. (%d damage)",
	"🧵 Healing turns against its target in the sewn-inside-out air; the mend lands as a sting. (%d damage)",
}

var narrativeStatDrain = []string{
	"🩸 The enemy saps your strength — your swings feel heavier, weaker. (-%d hit damage)",
	"🩸 Something drains out of your limbs. Your hits won't bite as deep now. (-%d damage)",
	"🩸 The enemy leeches your vigour. -%d damage on every strike from here.",
}

var narrativeDebuff = []string{
	"😖 The enemy fouls your footing — your guard slips. (-%d AC)",
	"😖 A wave of something sickly rolls over you. Harder to defend now. (-%d AC)",
	"😖 The enemy's effect frays your defence. -%d AC, and their hits will notice.",
}

var narrativeMaxHPDrain = []string{
	"☠️ The enemy drains your life force — %d HP gone, and your maximum drops with it.",
	"☠️ Something cold pulls %d HP out of you and won't give it back. Your ceiling just fell.",
	"☠️ %d HP siphoned away, max and all. There's less of you to work with now.",
}

var narrativeSpellResist = []string{
	"🚫 The enemy's form shimmers with anti-magic. Spells are going to land soft from here.",
	"🚫 A warding sheen wraps the enemy — magic slides off it like rain off glass.",
	"🚫 The enemy shrugs the weave aside. Whatever you cast, it'll only half-stick.",
}

var narrativeSpellFizzle = []string{
	"🚫 Your spell breaks against the enemy's anti-magic — only %d damage bleeds through.",
	"🚫 The weave fizzles on contact. The enemy's resistance eats half of it; %d lands.",
	"🚫 Half your magic just evaporates. %d damage is all the enemy lets through.",
}

var narrativeRevealArmed = []string{
	"👁️ The enemy's eyes track your intent — it knows what you're about to do.",
	"👁️ The enemy reads you. Your next swing is telegraphed before you've thrown it.",
	"👁️ Something behind the enemy's gaze clicks. It's already seen your next move.",
}

var narrativeRevealed = []string{
	"👁️ The enemy saw it coming — your strike is forced wide before it begins.",
	"👁️ Telegraphed and countered. The enemy was already moving as you committed.",
	"👁️ Your swing meets a defence that read it a beat early. No clean angle left.",
}

var narrativeFearImmune = []string{
	"🐲 The enemy's resolve is iron — nothing you do is going to rattle it.",
	"🐲 Whatever fear you were counting on, the enemy doesn't have the receptors for it.",
	"🐲 The enemy stands utterly unshaken. Control magic is going to slide right off.",
}

var narrativeFearResist = []string{
	"🐲 Your control spell washes over the enemy and finds no purchase. It acts anyway.",
	"🐲 The enemy shrugs off the compulsion mid-cast and keeps coming.",
	"🐲 The spell should have held it. The enemy's will simply refuses the idea.",
}

var narrativeAllyBuff = []string{
	"📢 The enemy rallies itself with a roar — its blows are about to hit harder. (+%d attack)",
	"📢 Something emboldens the enemy. Its strikes pick up weight. (+%d attack)",
	"📢 The enemy steadies and surges. Expect heavier hits from here. (+%d attack)",
}

// Outcome flavor
var narrativeNearDeathWin = []string{
	"You survived by the skin of your teeth. Barely standing. The healers are on standby.",
	"That was closer than anyone should be comfortable with. But you won. Technically. Medically questionable.",
	"The healers rush over. You wave them off. Mostly because you can't lift your arm to do anything else.",
}

var narrativeDominantWin = []string{
	"Decisive. The enemy never had a chance. They knew it. You knew it. The crowd knew it.",
	"That wasn't a fight. That was a demonstration. The enemy was the visual aid.",
	"You barely broke a sweat. The enemy cannot say the same. The enemy cannot say much of anything right now.",
}

var narrativeCleanWin = []string{
	"A solid fight. You came out on top. The enemy came out on a stretcher.",
	"Well fought. The outcome was never really in doubt. Just in question.",
	"The enemy put up a fight. You put up a better one. The scoreboard reflects this.",
}

var narrativeNearDeathLoss = []string{
	"So close. One more hit and it would've gone the other way. One more hit didn't come.",
	"You almost had it. Almost doesn't count here. Almost doesn't count anywhere.",
	"A razor-thin margin. The wrong side of it. The margin doesn't care whose side it was.",
}

var narrativeDominantLoss = []string{
	"That was over before it started. The starting was a formality.",
	"Outclassed. Completely. There's no gentle way to say it so here it is.",
	"Some fights aren't fights. This was one of those. You were present. That's about all.",
}

var narrativeCloseLoss = []string{
	"A good fight. Not good enough. The scoreboard is indifferent to effort.",
	"You gave it everything. It wasn't quite sufficient. Sufficiency is the enemy's department today.",
	"The enemy earned that one. So did you. Only one of you gets paid.",
}

// ── Turn-based per-round narration ───────────────────────────────────────────
//
// RenderTurnRound narrates one resolved round of a manual elite/boss fight.
// Where RenderCombatLog (auto-resolve) groups a whole fight into phased,
// multi-message blocks, a turn-based round is one message: the events from
// player_turn → enemy_turn → round_end, in order. The shared hit/crit/miss/
// ability events the turn engine emits through the same primitives as
// auto-resolve fall through to renderEvent and reuse the full narrative pools,
// so TwinBee's voice is identical across both engines. Only the four
// turn-specific actions (flee, spell_cast, use_consumable, spell_held) get
// their own pools here.
//
// A fresh actionPicker per round is intentional: a round emits at most a
// handful of events, and across a long boss fight the picker resetting each
// round reads as variety rather than staleness.
func RenderTurnRound(events []CombatEvent, playerName, enemyName string) string {
	return RenderPartyTurnRound(events, []string{playerName}, enemyName, 0)
}

// RenderPartyTurnRound renders a round for one member of a party — the reader at
// viewerSeat.
//
// The turn narration pool is written in the second person: "You score 9 damage",
// "A hit gets through your guard". That is exactly right for the reader's own
// events and exactly wrong for everybody else's, so this splits on the event's
// Seat. The reader's own events go through the same flavor pool a solo fight
// uses, untouched; an ally's are summarised third-person by renderAllySeatEvent.
//
// A solo fight has one seat, every event belongs to it, and the reader is it —
// so it renders the same bytes it always has.
func RenderPartyTurnRound(events []CombatEvent, seatNames []string, enemyName string, viewerSeat int) string {
	if len(seatNames) == 0 {
		seatNames = []string{"You"}
	}
	seatName := func(seat int) string {
		if seat > 0 && seat < len(seatNames) {
			return seatNames[seat]
		}
		return seatNames[0]
	}

	picker := newActionPicker()
	var lines []string
	for _, e := range events {
		var line string
		if e.Seat == viewerSeat {
			line = renderTurnEvent(e, seatName(e.Seat), enemyName, picker)
			if roll := rollAnnotation(e); line != "" && roll != "" {
				line += " " + roll
			}
		} else {
			line = renderAllySeatEvent(e, seatName(e.Seat), enemyName)
		}
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "_The round passes without a clean blow landed._"
	}
	return strings.Join(lines, "\n")
}

// renderAllySeatEvent summarises one event belonging to somebody else's
// character, for a party member's DM. Terse on purpose: the reader wants the
// shape of the round, not three sentences of flavor about their friend's pet.
// Anything not worth a line in someone else's DM renders as "".
func renderAllySeatEvent(e CombatEvent, name, enemyName string) string {
	who := "**" + name + "**"
	down := func(line string) string {
		if e.PlayerHP <= 0 {
			return line + " " + who + " is down."
		}
		return line
	}
	switch e.Action {
	case "hit":
		if e.Actor == "player" {
			return fmt.Sprintf("%s hits %s for %d.", who, enemyName, e.Damage)
		}
		return down(fmt.Sprintf("%s hits %s for %d.", enemyName, who, e.Damage))
	case "crit":
		if e.Actor == "player" {
			return fmt.Sprintf("%s crits %s for %d!", who, enemyName, e.Damage)
		}
		return down(fmt.Sprintf("%s crits %s for %d!", enemyName, who, e.Damage))
	case "miss":
		if e.Actor == "player" {
			return fmt.Sprintf("%s misses.", who)
		}
		return fmt.Sprintf("%s misses %s.", enemyName, who)
	case "block":
		// renderEvent's convention: Actor "player" means the *enemy* blocked the
		// player's swing, and vice versa.
		if e.Actor == "player" {
			return fmt.Sprintf("%s blocks %s (%d).", enemyName, who, e.Damage)
		}
		return fmt.Sprintf("%s blocks (%d).", who, e.Damage)
	case "spell_cast":
		label := e.Desc
		if label == "" {
			label = "a spell"
		}
		return fmt.Sprintf("%s casts %s.", who, label)
	case "use_consumable":
		label := e.Desc
		if label == "" {
			label = "an item"
		}
		return fmt.Sprintf("%s uses %s.", who, label)
	case "pet_attack":
		return fmt.Sprintf("🐾 %s's pet strikes for %d.", who, e.Damage)
	case "spirit_weapon_strike":
		return fmt.Sprintf("%s's spirit weapon strikes for %d.", who, e.Damage)
	case "concentration_tick":
		return fmt.Sprintf("%s's aura burns %s for %d.", who, enemyName, e.Damage)
	case "poison_tick":
		return down(fmt.Sprintf("☠️ %s takes %d from poison.", who, e.Damage))
	case "environmental":
		return down(fmt.Sprintf("%s takes %d from the room.", who, e.Damage))
	case "crowd_revenge":
		// Deliberately unattributed. An ally sees the damage land — silence
		// would read as HP vanishing — but Misty's grudge is the owner's own
		// discovery, and naming her here would spoil it for the whole party.
		// Same reason misty_heal below reads as a plain recovery.
		return down(fmt.Sprintf("%s takes %d.", who, e.Damage))
	case "flat_damage":
		return fmt.Sprintf("%s deals %d.", who, e.Damage)
	case "heal_item", "misty_heal":
		return fmt.Sprintf("%s recovers %d.", who, e.Damage)
	case "death_save":
		return fmt.Sprintf("%s clings on.", who)
	case "stun", "stunned":
		return fmt.Sprintf("%s is stunned.", who)
	case "flee":
		return fmt.Sprintf("%s breaks off.", who)
	default:
		// Ward absorbs, spore misses, reflects, enrage cues: real, but noise in
		// somebody else's DM. The owner sees them in full in their own.
		return ""
	}
}

func renderTurnEvent(e CombatEvent, playerName, enemyName string, picker *actionPicker) string {
	switch e.Action {
	case "flee":
		return pickRand(narrativeTurnFlee)
	case "spell_held":
		return pickRand(narrativeTurnSpellHeld)
	case "spell_cast":
		label := e.Desc
		if label == "" {
			label = "a spell"
		}
		return fmt.Sprintf(pickRand(narrativeTurnSpellCast), label)
	case "use_consumable":
		label := e.Desc
		if label == "" {
			label = "an item"
		}
		return fmt.Sprintf(pickRand(narrativeTurnConsumable), label)
	}
	return renderEvent(e, playerName, enemyName, CombatResult{}, picker)
}

var narrativeTurnFlee = []string{
	"🏃 You break off and run. No shame in it — the dead don't get a sequel.",
	"🏃 You decide this fight isn't yours to win and make for the exit. I respect the math.",
	"🏃 You disengage and bolt. The enemy doesn't chase. The enemy didn't have to.",
}

var narrativeTurnSpellHeld = []string{
	"🌀 The enemy strains against the spell and gets nowhere. No attack this round.",
	"🌀 Held fast. The enemy's turn dissolves into furious, useless effort.",
	"🌀 The control holds. The enemy spends the round being a statue with opinions.",
}

var narrativeTurnSpellCast = []string{
	"✨ You loose the spell. **%s**.",
	"✨ The weave answers. **%s**.",
	"✨ Words, gesture, intent — **%s**.",
	"✨ You spend the magic where it counts. **%s**.",
}

var narrativeTurnConsumable = []string{
	"🧪 You crack it open mid-fight. **%s**.",
	"🧪 No time to be precious about it. **%s**.",
	"🧪 You burn a consumable on the spot. **%s**.",
}
