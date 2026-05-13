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
	"_The exchange settles into a rhythm. A few rounds of nothing meaningful — TwinBee skips to the next thing that matters._",
	"_TwinBee narrates the next few seconds in a single sigh. Both sides keep swinging. Both sides keep missing._",
	"_And the battle goes on. Yadda yadda yadda. TwinBee will narrate the next part where something actually lands._",
	"_The grind continues. TwinBee uses the time to mentally redesign the dungeon's lighting._",
	"_A flurry of misses on both sides. Nobody is impressing anybody. TwinBee waits for the next hit._",
	"_TwinBee politely declines to narrate this stretch. It was, in TwinBee's professional opinion, a waste of everyone's time._",
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
