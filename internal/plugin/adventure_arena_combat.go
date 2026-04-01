package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// ── Combat Log Types ───────────────────────────────────────────────────────

type ArenaCombatLog struct {
	Rounds    []ArenaCombatRound
	PlayerHP  int
	EnemyHP   int
	PlayerWon bool
}

type ArenaCombatRound struct {
	Number         int
	Text           string // action description with damage filled in
	Type           string // "player_hit", "enemy_hit", "block", "environmental"
	DamageToPlayer int
	DamageToEnemy  int
	PlayerHP       int // HP after this round
	EnemyHP        int // HP after this round
}

// ── Combat Log Generation ──────────────────────────────────────────────────

// generateArenaCombatLog assembles a turn-by-turn narrative for a fight whose
// outcome is already determined. The log is cosmetic — the roll already happened.
// closeness is 0.0 (decisive) to 1.0 (razor-thin margin).
func generateArenaCombatLog(playerWon bool, closeness float64) *ArenaCombatLog {
	// Pick HP pools
	playerHP := 60 + rand.IntN(41) // 60-100
	enemyHP := 60 + rand.IntN(41)  // 60-100

	// Determine round count: decisive=3-4, close=5-6
	numRounds := 3
	if closeness > 0.7 {
		numRounds = 5 + rand.IntN(2) // 5-6
	} else if closeness > 0.4 {
		numRounds = 4 + rand.IntN(2) // 4-5
	} else {
		numRounds = 3 + rand.IntN(2) // 3-4
	}

	// Assign round types
	types := assignRoundTypes(numRounds, playerWon)

	// Calculate damage distribution
	picker := newActionPicker()
	rounds := distributeDamage(types, playerHP, enemyHP, playerWon, picker)

	return &ArenaCombatLog{
		Rounds:    rounds,
		PlayerHP:  playerHP,
		EnemyHP:   enemyHP,
		PlayerWon: playerWon,
	}
}

// assignRoundTypes determines what happens each round.
// Final round is always winner hitting — this is enforced.
// Guarantees at least 1 hit round per side.
func assignRoundTypes(numRounds int, playerWon bool) []string {
	types := make([]string, numRounds)

	// Final round: winner lands the killing blow
	if playerWon {
		types[numRounds-1] = "player_hit"
	} else {
		types[numRounds-1] = "enemy_hit"
	}

	// Fill remaining rounds
	for i := 0; i < numRounds-1; i++ {
		roll := rand.Float64()
		switch {
		case roll < 0.15:
			types[i] = "environmental"
		case roll < 0.35:
			types[i] = "block"
		case roll < 0.65:
			if i%2 == 0 {
				types[i] = "enemy_hit"
			} else {
				types[i] = "player_hit"
			}
		default:
			if i%2 == 0 {
				types[i] = "player_hit"
			} else {
				types[i] = "enemy_hit"
			}
		}
	}

	// Guarantee at least 1 hit round per side (besides the final round).
	hasPlayerHit := false
	hasEnemyHit := false
	for _, t := range types {
		if t == "player_hit" {
			hasPlayerHit = true
		}
		if t == "enemy_hit" || t == "environmental" {
			hasEnemyHit = true
		}
	}
	// If missing a side, convert the first block round (or first non-final round).
	if !hasPlayerHit {
		for i := 0; i < numRounds-1; i++ {
			if types[i] == "block" || types[i] == "enemy_hit" || types[i] == "environmental" {
				types[i] = "player_hit"
				break
			}
		}
	}
	if !hasEnemyHit {
		for i := 0; i < numRounds-1; i++ {
			if types[i] == "block" || types[i] == "player_hit" {
				types[i] = "enemy_hit"
				break
			}
		}
	}

	return types
}

// distributeDamage creates rounds with damage values that sum correctly.
func distributeDamage(types []string, playerHP, enemyHP int, playerWon bool, picker *actionPicker) []ArenaCombatRound {
	numRounds := len(types)

	// Total damage dealt: winner kills the loser (deals their full HP).
	// Loser deals some but not all of winner's HP.
	var totalDmgToEnemy, totalDmgToPlayer int
	if playerWon {
		totalDmgToEnemy = enemyHP
		totalDmgToPlayer = int(float64(playerHP) * (0.3 + rand.Float64()*0.5)) // 30-80% of player HP
	} else {
		totalDmgToPlayer = playerHP
		totalDmgToEnemy = int(float64(enemyHP) * (0.3 + rand.Float64()*0.5))
	}

	// Count damage rounds for each side
	var playerHitRounds, enemyHitRounds []int
	for i, t := range types {
		switch t {
		case "player_hit":
			playerHitRounds = append(playerHitRounds, i)
		case "enemy_hit":
			enemyHitRounds = append(enemyHitRounds, i)
		case "environmental":
			enemyHitRounds = append(enemyHitRounds, i) // environmental damages player
		}
	}

	// Distribute damage to enemy across player_hit rounds
	enemyDmgPerRound := splitDamage(totalDmgToEnemy, len(playerHitRounds))
	// Distribute damage to player across enemy_hit + environmental rounds
	playerDmgPerRound := splitDamage(totalDmgToPlayer, len(enemyHitRounds))

	// Build rounds
	rounds := make([]ArenaCombatRound, numRounds)
	currentPlayerHP := playerHP
	currentEnemyHP := enemyHP
	playerDmgIdx := 0
	enemyDmgIdx := 0

	for i, t := range types {
		r := ArenaCombatRound{
			Number: i + 1,
			Type:   t,
		}

		switch t {
		case "player_hit":
			dmg := 0
			if enemyDmgIdx < len(enemyDmgPerRound) {
				dmg = enemyDmgPerRound[enemyDmgIdx]
				enemyDmgIdx++
			}
			r.DamageToEnemy = dmg
			currentEnemyHP -= dmg
			if currentEnemyHP < 0 {
				currentEnemyHP = 0
			}
			r.Text = pickFrom(arenaPlayerHitActions, picker.player, dmg)

		case "enemy_hit":
			dmg := 0
			if playerDmgIdx < len(playerDmgPerRound) {
				dmg = playerDmgPerRound[playerDmgIdx]
				playerDmgIdx++
			}
			r.DamageToPlayer = dmg
			currentPlayerHP -= dmg
			if currentPlayerHP < 0 {
				currentPlayerHP = 0
			}
			// Mix in player-miss actions (~30% of enemy_hit rounds)
			if rand.IntN(100) < 30 {
				r.Text = pickFrom(arenaPlayerMissActions, picker.playerMiss, dmg)
			} else {
				r.Text = pickFrom(arenaEnemyActions, picker.enemy, dmg)
			}

		case "block":
			r.Text = pickFromNoFmt(arenaBlockActions, picker.block)

		case "environmental":
			dmg := 0
			if playerDmgIdx < len(playerDmgPerRound) {
				dmg = playerDmgPerRound[playerDmgIdx]
				playerDmgIdx++
			}
			r.DamageToPlayer = dmg
			currentPlayerHP -= dmg
			if currentPlayerHP < 0 {
				currentPlayerHP = 0
			}
			r.Text = pickFrom(arenaEnvironmentalActions, picker.environment, dmg)
		}

		r.PlayerHP = currentPlayerHP
		r.EnemyHP = currentEnemyHP
		rounds[i] = r
	}

	// Ensure final round ends at exactly 0 for the loser
	last := &rounds[numRounds-1]
	if playerWon {
		last.EnemyHP = 0
	} else {
		last.PlayerHP = 0
	}

	return rounds
}

// splitDamage distributes total damage across n rounds with some variance.
// Each round gets at least 1 damage. If total < n, excess rounds get 0.
func splitDamage(total, n int) []int {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []int{total}
	}

	result := make([]int, n)

	// If total < n, give 1 to the first `total` rounds, 0 to the rest.
	if total <= n {
		for i := 0; i < total && i < n; i++ {
			result[i] = 1
		}
		return result
	}

	remaining := total

	for i := 0; i < n-1; i++ {
		avg := remaining / (n - i)
		if avg <= 0 {
			avg = 1
		}
		// Variance: 50%-150% of average
		lo := avg / 2
		if lo < 1 {
			lo = 1
		}
		hi := avg + avg/2
		if hi < lo {
			hi = lo
		}
		dmg := lo + rand.IntN(hi-lo+1)
		// Reserve at least 1 per remaining round
		maxThisRound := remaining - (n - 1 - i)
		if maxThisRound < 1 {
			maxThisRound = 1
		}
		if dmg > maxThisRound {
			dmg = maxThisRound
		}
		if dmg < 1 {
			dmg = 1
		}
		result[i] = dmg
		remaining -= dmg
	}
	result[n-1] = remaining
	if result[n-1] < 1 {
		result[n-1] = 1
	}

	return result
}

// actionPicker tracks used indices per pool to avoid repeats within a fight.
type actionPicker struct {
	enemy       map[int]bool
	player      map[int]bool
	playerMiss  map[int]bool
	block       map[int]bool
	environment map[int]bool
}

func newActionPicker() *actionPicker {
	return &actionPicker{
		enemy:       make(map[int]bool),
		player:      make(map[int]bool),
		playerMiss:  make(map[int]bool),
		block:       make(map[int]bool),
		environment: make(map[int]bool),
	}
}

// pickFrom selects a random unused entry from pool, formats it with damage, and marks it used.
// Resets if pool is exhausted.
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

// ── Render ─────────────────────────────────────────────────────────────────

func renderArenaCombatLog(log *ArenaCombatLog, monster *ArenaMonster, won bool, reward int64, xp int, closerLine string) string {
	var sb strings.Builder

	for _, r := range log.Rounds {
		sb.WriteString(r.Text + "\n")

		// Compact HP status line — damage is already in the action text via %d
		switch r.Type {
		case "player_hit":
			sb.WriteString(fmt.Sprintf("  [You: %d/%d | Enemy: %d/%d]\n", r.PlayerHP, log.PlayerHP, r.EnemyHP, log.EnemyHP))
		case "enemy_hit", "environmental":
			sb.WriteString(fmt.Sprintf("  [You: %d/%d | Enemy: %d/%d]\n", r.PlayerHP, log.PlayerHP, r.EnemyHP, log.EnemyHP))
		}
		// Blocks: no HP line (no damage happened)
	}

	sb.WriteString("\n")
	if won {
		sb.WriteString(fmt.Sprintf("💀 %s has been defeated.\n", monster.Name))
		sb.WriteString(closerLine + "\n")
		sb.WriteString(fmt.Sprintf("🏆 +%d XP | €%d earned\n", xp, reward))
	} else {
		sb.WriteString("The healers are already moving.\n")
		sb.WriteString("💀 Defeated.\n")
		sb.WriteString(closerLine + "\n")
		sb.WriteString(fmt.Sprintf("+%d XP (participation) | Back tomorrow.\n", arenaParticipationXP))
	}

	return sb.String()
}

const arenaParticipationXP = 60

// ── Closer Lines ───────────────────────────────────────────────────────────

func arenaWinCloser(loserName string, lastRound int) string {
	closers := []string{
		"%s fought. It counts.",
		"%s will be back. The arena keeps score.",
		fmt.Sprintf("%%s has until tomorrow to think about round %d.", lastRound),
		"%s gave you more trouble than you'd like to admit. They don't need to know that.",
		"%s loses this one. The next one is an open question.",
		"%s came here to fight and did. The result is a separate matter.",
		"%s is already planning the rematch. You can feel it.",
	}
	return fmt.Sprintf(closers[rand.IntN(len(closers))], loserName)
}

func arenaLoseCloser(winnerName string, lastRound int) string {
	closers := []string{
		"You fought. It counts.",
		"You'll be back. The arena keeps score.",
		fmt.Sprintf("You have until tomorrow to think about round %d.", lastRound),
		fmt.Sprintf("You gave %s more trouble than they'd like to admit. Small comfort. Still comfort.", winnerName),
		"You lose this one. The next one is an open question.",
		"You came here to fight and did. The result is a separate matter.",
		fmt.Sprintf("%s won this one. You're already planning the rematch.", winnerName),
	}
	return closers[rand.IntN(len(closers))]
}

// ── Action Pools ───────────────────────────────────────────────────────────

// Enemy actions — hit the player. %d is damage.
var arenaEnemyActions = []string{
	"The enemy insults your clothing choices. Spot-on. Hits you for %d emotional damage. They weren't wrong about the boots. They do not go with that top on this planet nor any other.",
	"The enemy puts their weapon away, walks up to you, and Will Smiths you across the face. The audacity of the move hurts more than the hit itself. %d damage.",
	"The enemy questions your life choices. You pause to genuinely reflect. They hit you during the pause. %d damage.",
	"The enemy delivers a full monologue. You listen to the whole thing. It was actually pretty good. %d damage from the time lost.",
	"The enemy compliments you unexpectedly. You thank them. They snicker because you actually believed them and revealed to everyone that you're somehow a bigger buffoon than previously known. %d damage.",
	"The enemy points at something behind you. You don't fall for it. They throw a projectile which bounces off the wall and hits you in the back of the head. What an amazing trick shot. %d damage. The crowd roars in laughter at the spectacle. But mostly at you.",
	"The enemy pulls out their phone and starts filming. You perform for the camera. This was a mistake. %d damage.",
	"The enemy sneezes directly in your face. You lose your turn being disgusted. %d damage while you process this.",
	"The enemy whispers something. You lean in to hear it. %d damage. There was nothing worth hearing.",
	"The enemy trips. Recovers. Hits you anyway. %d damage. You were rooting for them for a second there.",
	"The enemy takes a phone call, hits you one-handed, and continues the call. %d damage. You were barely a distraction.",
	"The enemy critiques your fighting stance in detail. You correct it instinctively. Your corrected stance is worse. %d damage.",
	"The enemy yawns mid-fight. Not performatively. Genuinely. %d damage while you process the disrespect.",
	"The enemy pauses to stretch before attacking. You wait. You don't know why you waited. %d damage when they finish.",
	"The enemy hits you with the flat of their blade. A choice. A message. %d damage. The message is received.",
	"The enemy stares at you for an uncomfortably long time before attacking. You break eye contact first. This was the plan. %d damage.",
	"The enemy sighs before hitting you. Like they had somewhere better to be. %d damage.",
	"The enemy recounts a mildly interesting story mid-fight. You get drawn in. %d damage before the ending, which was not worth it.",
	"The enemy raises one eyebrow at you and then attacks. The eyebrow did more damage than the hit. %d damage total.",
	"The enemy adjusts their grip, rolls their shoulders, and hits you with what is technically the bare minimum of effort. %d damage. You gave it everything. They did not.",
}

// Player actions — hit the enemy. %d is damage to enemy.
var arenaPlayerHitActions = []string{
	"You make a joke using a painfully dated reference. While the enemy stands there pondering what on earth you could possibly be referring to, you seize the opportunity and land a critical hit. %d damage. Your jokes are always great at leaving people dazed and confused.",
	"You attempt a battle cry. It comes out as a question. The enemy is briefly confused. You hit them for %d damage before they recover.",
	"You wind up for a big hit and connect for %d damage. You pulled something. The enemy doesn't know this yet.",
	"You hit the enemy for %d damage. They seem fine. You are less fine about this than they are.",
	"You connect cleanly for %d damage and immediately look at your hand like you're surprised it worked. You were.",
	"You score a clean hit for %d damage and immediately start explaining to no one in particular how you did that. Nobody asked. The fight is still happening.",
	"You land a hit for %d damage and follow up with a second strike that connects with nothing. You style it out. Nobody is convinced.",
}

// Player actions — player's turn goes wrong. %d is damage to player.
var arenaPlayerMissActions = []string{
	"You reach for your weapon and grab the wrong item. You are holding a receipt. The enemy hits you for %d damage. You find this receipt later and it's actually useful.",
	"You make prolonged eye contact with a spectator. It goes on too long. The enemy hits you for %d damage. The spectator looks away first.",
	"Your shoelace comes untied. You are wearing boots. You address this. The enemy does not wait. %d damage.",
	"You sneeze at a critical moment. The enemy respectfully waits. Then hits you for %d damage. There was no respect involved actually.",
	"You perform a move you saw in a film once. It does not work like in the film. %d damage. The physics were always wrong in that film.",
	"You get distracted by a food vendor passing the arena perimeter. So does the enemy. You recover second. %d damage.",
	"You attempt to intimidate the enemy. They laugh. Genuinely. This is worse than if they hadn't. You take %d damage from the experience.",
	"You slip on something. There is nothing to slip on. %d damage. The arena floor is flat and dry. You will be thinking about this.",
	"You decide mid-swing to do something different. The original plan was better. %d damage.",
	"You attempt a combo you've been mentally rehearsing for weeks. It goes fine until the third move. %d damage.",
	"You feint left. The enemy doesn't move. You feint right. They still don't move. You just stand there feinting at someone who is not playing along. The enemy hits you. %d damage.",
	"You remember reading something about fighting once. You implement it. It was about chess. %d damage.",
	"You close your eyes for the strike because it feels more dramatic. You miss. The enemy doesn't. %d damage.",
	"You decide this is the moment for something new. It is not the moment for something new. %d damage. File this under lessons.",
}

// Block/dodge actions — no damage.
var arenaBlockActions = []string{
	"You swing with conviction. The enemy sidesteps it with the energy of someone who has somewhere else to be. Nothing happens. You both reset.",
	"The enemy lunges. You step aside. They continue past you for several feet and have to walk back. The pause is awkward for everyone.",
	"You block the incoming strike so cleanly that the enemy looks at their weapon like it betrayed them personally. You don't know their relationship so it probably did, but also you were faster.",
	"The enemy's attack grazes you but doesn't connect. They seem more annoyed by this than you are relieved.",
	"You duck. The enemy's strike passes exactly where your head was. You both take a moment to appreciate how close that was. Then the fight continues.",
	"The enemy deflects your attack with a move that was frankly unnecessary for the situation. It worked. You will be thinking about how unnecessary it was.",
	"You parry. The enemy's weapon skids off yours and they stumble slightly. They recover before you can do anything about it. It was still a good parry.",
	"The enemy blocks your hilariously poor-timed strike with their forearm. This speaks less about the strength of their forearm and much more so about the pathetic nature of your striking abilities.",
	"You dodge sideways into a pillar. It hurts but it doesn't count as a hit. The enemy didn't do that. The pillar gets no credit either.",
	"The enemy telegraphs the attack so clearly that you block it before they've finished committing to it. They look briefly embarrassed. They recover. The fight continues.",
	"You attempt a dodge and accidentally do something that looks extremely skilled. It was not intentional. The enemy hesitates, which was also not intentional. Nothing lands.",
	"The enemy's strike is deflected off your shoulder guard and disappears somewhere into the arena. They retrieve a backup weapon from somewhere. Nobody asks where they got it.",
	"You and the enemy swing at exactly the same moment. Both weapons meet in the middle. You stare at each other. Someone has to move first. It's them. The fight continues.",
	"The enemy's attack comes in low. You jump. Not gracefully. But adequately. Nothing connects. You land. The fight continues.",
	"You sidestep a strike that wasn't aimed at you. The enemy had already redirected. You both end up slightly confused about where the other one is. The round resolves without damage.",
	"A referee walks through the arena on the way to somewhere else. Eye contact is made with both fighters. They keep walking. There is a beat. The fight resumes.",
}

// Environmental actions — damage to player. %d is damage.
var arenaEnvironmentalActions = []string{
	"Your mother calls in the middle of battle asking when you're giving her grandchildren. The enemy hits you for %d damage while you work out how to answer that on speakerphone.",
	"A bird lands between you and the enemy. Both combatants stop. The bird leaves. The enemy recovers first. %d damage.",
	"A spectator in the front row is eating something that smells incredible. Both fighters lose focus. The enemy had less going on mentally. %d damage.",
	"The arena announcer mispronounces your name. You correct them mid-fight. The enemy hits you for %d damage. The announcer mispronounces it again.",
	"An old acquaintance you've been avoiding is in the crowd. You make brief eye contact. Mutual acknowledgment. The enemy hits you for %d damage during this social transaction.",
	"Someone in the crowd drops their drink. The sound is startling. You both flinch. The enemy flinches smaller. %d damage.",
	"A cloud passes in front of the sun at the wrong moment. %d damage. The cloud did not mean anything by it.",
	"The arena's background music cuts out unexpectedly. The silence is louder than the fight. The enemy hits you for %d damage in the disorientation.",
	"The arena PA crackles and announces something completely unrelated to your fight. You both look up. The enemy looks back down first. %d damage.",
	"Something falls from the spectator area. Nobody claims it. You both look at it. The enemy decides faster. %d damage.",
	"A dog wanders into the arena perimeter briefly. Both fighters stop. The dog is removed. You both needed that break more than you'd like to admit. The enemy uses the reset better. %d damage.",
	"The arena's scoreboard updates mid-fight and briefly shows wrong numbers. You spend a round trying to work out if that changes anything. It does not. %d damage while you calculate.",
	"The arena sells a limited merch item at exactly this moment. The announcement is enthusiastic. You are briefly curious. %d damage.",
	"The crowd goes quiet at an inopportune moment. You can hear everything. Including things you did not want to hear from the enemy's corner. %d damage.",
}
