package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/flavor"

	"maunium.net/go/mautrix/id"
)

// ── NPC Constants ──────────────────────────────────────────────────────────

const (
	mistyCost          = 100
	arinaCost          = 5000
	npcCooldownDays    = 7
	npcEncounterChance = 0.075 // 7.5%
	npcBuffDuration    = 7 * 24 * time.Hour

	// Arena effect chances per round
	mistyEffectChance = 0.20 // 20%
	arinaEffectChance = 0.08 // 8%

	// Misty crowd revenge damage range
	crowdRevengeDmgMin = 3
	crowdRevengeDmgMax = 8

	// Misty gourmet food — enemy damage
	gourmetEnemyDmg = 5
	// Misty gourmet food — equipment condition repair
	gourmetConditionRepair = 5
)

// ── Message Count Tracking ─────────────────────────────────────────────────

// npcTrackMessage is called on every room message from an adventure player.
// It increments their daily message count and fires NPC encounter rolls
// when thresholds are hit.
func (p *AdventurePlugin) npcTrackMessage(userID id.UserID) {
	// Acquire user lock to prevent lost updates on message count and
	// double encounter triggers from rapid messages.
	userMu := p.advUserLock(userID)
	userMu.Lock()

	char, err := loadAdvCharacter(userID)
	if err != nil || !char.Alive {
		userMu.Unlock()
		return
	}

	today := time.Now().UTC().Format("2006-01-02")

	// Reset count if it's a new day
	if char.NPCMsgCountDate != today {
		char.NPCMsgCount = 0
		char.NPCMsgCountDate = today
		char.MistyRollTarget = 5 + rand.IntN(6) // 5–10
		char.ArinaRollTarget = 5 + rand.IntN(6) // 5–10
	}

	char.NPCMsgCount++

	var fireNPC string

	// Check Misty threshold
	if char.MistyRollTarget > 0 && char.NPCMsgCount == char.MistyRollTarget {
		if p.npcShouldEncounter(char, "misty") {
			char.MistyRollTarget = 0
			fireNPC = "misty"
		}
	}

	// Check Arina threshold (only if Misty didn't fire — one encounter per message)
	if fireNPC == "" && char.ArinaRollTarget > 0 && char.NPCMsgCount == char.ArinaRollTarget {
		if p.npcShouldEncounter(char, "arina") {
			char.ArinaRollTarget = 0
			fireNPC = "arina"
		}
	}

	if err := saveAdvCharacter(char); err != nil {
		slog.Error("npc: failed to save msg count", "user", userID, "err", err)
	}
	if err := upsertPlayerMetaNPCState(userID, npcStateFromAdvChar(char)); err != nil {
		slog.Error("player_meta: npc msg count dual-write failed", "user", userID, "err", err)
	}

	// Release lock BEFORE firing encounter (npcFireEncounter re-acquires it)
	userMu.Unlock()

	if fireNPC != "" {
		safeGo("npc-"+fireNPC+"-encounter", func() {
			p.npcFireEncounter(userID, fireNPC)
		})
	}
}

// npcShouldEncounter checks cooldown and rolls the 7.5% chance.
func (p *AdventurePlugin) npcShouldEncounter(char *AdventureCharacter, npc string) bool {
	now := time.Now().UTC()
	cooldown := npcCooldownDays * 24 * time.Hour

	switch npc {
	case "misty":
		if char.MistyLastSeen != nil && now.Sub(*char.MistyLastSeen) < cooldown {
			return false
		}
	case "arina":
		if char.ArinaLastSeen != nil && now.Sub(*char.ArinaLastSeen) < cooldown {
			return false
		}
	}

	return rand.Float64() < npcEncounterChance
}

// ── Encounter Fire ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) npcFireEncounter(userID id.UserID, npc string) {
	userMu := p.advUserLock(userID)
	userMu.Lock()
	defer userMu.Unlock()

	char, err := loadAdvCharacter(userID)
	if err != nil || !char.Alive {
		return
	}

	// Don't consume the encounter — or its one-shot arc beat — if the player is
	// already mid-interaction (shop, treasure, another NPC). Bail before
	// touching MistyEncounterCount so a contended slot defers the whole
	// encounter to a later fire instead of durably advancing the counter past
	// a 5/15/30 threshold and losing that beat forever.
	if _, occupied := p.pending.Load(string(userID)); occupied {
		return
	}

	now := time.Now().UTC()

	var opening, prompt string
	switch npc {
	case "misty":
		char.MistyLastSeen = &now
		char.MistyEncounterCount++
		// Pool union: legacy mistyOpenings + D&D MistyGreeting flavor.
		mistyPool := dndMistyGreetingPool()
		opening = mistyPool[rand.IntN(len(mistyPool))]
		// D2 arc: a deepening beat lands the one time the counter hits a
		// threshold (5/15/30). MistyEncounterCount only ever increments by one
		// above, so each beat is delivered exactly once.
		if beat := mistyArcBeat(char.MistyEncounterCount); beat != "" {
			opening = beat + "\n\n" + opening
		}
		prompt = fmt.Sprintf("👤 A woman approaches you.\n\n_%s_\n\n"+
			"Reply `yes` to give €%d, or `no` to walk away.", opening, mistyCost)
	case "arina":
		char.ArinaLastSeen = &now
		arinaPool := dndArinaGreetingPool()
		opening = arinaPool[rand.IntN(len(arinaPool))]
		prompt = fmt.Sprintf("💎 A woman in expensive clothing stops you.\n\n_%s_\n\n"+
			"Reply `yes` to pay €%d, or `no` to decline.", opening, arinaCost)
	}

	if err := saveAdvCharacter(char); err != nil {
		slog.Error("npc: failed to save last_seen", "user", userID, "npc", npc, "err", err)
		return
	}
	if err := upsertPlayerMetaNPCState(userID, npcStateFromAdvChar(char)); err != nil {
		slog.Error("player_meta: npc last_seen dual-write failed", "user", userID, "npc", npc, "err", err)
	}

	// Set pending interaction — NPC encounters stay valid until end of UTC day
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "npc_encounter",
		Data:      npc,
		ExpiresAt: endOfDay,
	})

	if err := p.SendDM(userID, prompt); err != nil {
		slog.Error("npc: failed to send encounter DM", "user", userID, "npc", npc, "err", err)
		p.pending.Delete(string(userID))
	}
}

// ── Accept / Decline Resolution ────────────────────────────────────────────

func (p *AdventurePlugin) resolveNPCEncounter(ctx MessageContext, interaction *advPendingInteraction) error {
	npc := interaction.Data.(string)
	body := strings.ToLower(strings.TrimSpace(ctx.Body))

	accepted := body == "yes" || body == "y" || body == "pay"
	declined := body == "no" || body == "n" || body == "decline"

	if !accepted && !declined {
		// Re-store the pending interaction — they typed something else
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Reply `yes` or `no`.")
	}

	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Something went wrong loading your character.")
	}

	now := time.Now().UTC()
	expires := now.Add(npcBuffDuration)

	switch npc {
	case "misty":
		return p.resolveMisty(ctx, char, accepted, &expires)
	case "arina":
		return p.resolveArina(ctx, char, accepted, &expires)
	}
	return nil
}

func (p *AdventurePlugin) resolveMisty(ctx MessageContext, char *AdventureCharacter, accepted bool, expires *time.Time) error {
	if accepted {
		balance := p.euro.GetBalance(ctx.Sender)
		if balance < float64(mistyCost) {
			// Can't afford it — treat as decline but with a kinder message
			char.MistyDebuffExpires = expires
			if err := saveAdvCharacter(char); err != nil {
				slog.Error("npc: failed to save misty debuff", "user", ctx.Sender, "err", err)
			}
			_ = upsertPlayerMetaNPCState(char.UserID, npcStateFromAdvChar(char))
			return p.SendDM(ctx.Sender, "You reach for your gold but there isn't enough. She sees this. She doesn't say anything.\n\n_"+mistyDeclineLine+"_")
		}

		if !p.euro.Debit(ctx.Sender, float64(mistyCost), "misty_donation") {
			char.MistyDebuffExpires = expires
			if err := saveAdvCharacter(char); err != nil {
				slog.Error("npc: failed to save misty debuff", "user", ctx.Sender, "err", err)
			}
			_ = upsertPlayerMetaNPCState(char.UserID, npcStateFromAdvChar(char))
			return p.SendDM(ctx.Sender, "You reach for your gold but there isn't enough. She sees this. She doesn't say anything.\n\n_"+mistyDeclineLine+"_")
		}
		// D&D Insight check — passing refunds the donation. Renders flavor on
		// either outcome (success or fail) when a check was attempted.
		insightCheck := dndNPCInsightRefund(ctx.Sender, p.euro, mistyCost)
		char.MistyBuffExpires = expires
		char.MistyDonatedCount++

		// Pet reactivation: donating to Misty after chasing pet away
		mistyPet, _ := loadPetState(char.UserID)
		if mistyReactivatePet(mistyPet) {
			char.PetReactivated = true
		}

		if err := saveAdvCharacter(char); err != nil {
			slog.Error("npc: failed to save misty buff", "user", ctx.Sender, "err", err)
		}
		_ = upsertPlayerMetaPetState(char.UserID, petStateFromAdvChar(char))
		_ = upsertPlayerMetaNPCState(char.UserID, npcStateFromAdvChar(char))

		reply := mistyAcceptLines[rand.IntN(len(mistyAcceptLines))]

		// Housing hint (fires once after 2+ encounters)
		mistyHouse, _ := loadHouseState(char.UserID)
		hint := mistyHousingHint(char.MistyEncounterCount, char.MistyDonatedCount, mistyHouse)
		if hint != "" {
			reply += "\n\n_" + hint + "_"
		}

		// Flavor for the D&D check outcome (when applicable).
		if insightCheck.Attempted {
			if insightCheck.Succeeded {
				if line := flavor.Pick(flavor.MistyInsightSuccess); line != "" {
					reply += "\n\n_" + line + "_"
				}
				reply += "\n\n_(Insight DC 12 passed — donation refunded.)_"
			} else {
				if line := flavor.Pick(flavor.MistySkillFail); line != "" {
					reply += "\n\n_" + line + "_"
				}
			}
		}

		return p.SendDM(ctx.Sender, fmt.Sprintf("_%s_", reply))
	}

	// Declined
	char.MistyDebuffExpires = expires
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("npc: failed to save misty debuff", "user", ctx.Sender, "err", err)
	}
	_ = upsertPlayerMetaNPCState(char.UserID, npcStateFromAdvChar(char))
	return p.SendDM(ctx.Sender, fmt.Sprintf("_%s_", mistyDeclineLine))
}

func (p *AdventurePlugin) resolveArina(ctx MessageContext, char *AdventureCharacter, accepted bool, expires *time.Time) error {
	if accepted {
		balance := p.euro.GetBalance(ctx.Sender)
		if balance < float64(arinaCost) {
			reply := arinaDeclineLines[rand.IntN(len(arinaDeclineLines))]
			return p.SendDM(ctx.Sender, fmt.Sprintf("You can't afford it. She noticed before you did.\n\n_%s_", reply))
		}

		if !p.euro.Debit(ctx.Sender, float64(arinaCost), "arina_investment") {
			reply := arinaDeclineLines[rand.IntN(len(arinaDeclineLines))]
			return p.SendDM(ctx.Sender, fmt.Sprintf("You can't afford it. She noticed before you did.\n\n_%s_", reply))
		}
		// D&D Arcana check — flavor for either outcome when a check was attempted.
		arcanaCheck := dndNPCArcanaRefund(ctx.Sender, p.euro, arinaCost)
		char.ArinaBuffExpires = expires
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("npc: failed to save arina buff", "user", ctx.Sender, "err", err)
		}
		_ = upsertPlayerMetaNPCState(char.UserID, npcStateFromAdvChar(char))
		extra := ""
		if arcanaCheck.Attempted {
			if arcanaCheck.Succeeded {
				if line := flavor.Pick(flavor.ArinaArcanaSuccess); line != "" {
					extra = "\n\n_" + line + "_"
				}
				extra += "\n\n_(Arcana DC 14 passed — investment refunded.)_"
			} else {
				if line := flavor.Pick(flavor.ArinaSkillFail); line != "" {
					extra = "\n\n_" + line + "_"
				}
			}
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"_She snatches the money from you, almost rudely, then looks at you expectantly._\n\n\"%s\"\n\n_She walks away just as quickly as she came._%s",
			arinaAcceptLine, extra))
	}

	// Declined — no mechanical effect, just insults
	reply := arinaDeclineLines[rand.IntN(len(arinaDeclineLines))]
	return p.SendDM(ctx.Sender, fmt.Sprintf("_%s_", reply))
}

// ── Arena Effect Checks ────────────────────────────────────────────────────

// npcArenaEffects is called during arena round resolution, after the normal
// combat roll but before the round log is written.
// Returns: extra text to append to round log, enemy HP modifier, player damage taken.
type npcArenaResult struct {
	Text       string
	EnemyDmg   int  // damage dealt to enemy
	PlayerDmg  int  // damage dealt to player
	SniperKill bool // enemy instant kill
	CondRepair int  // equipment condition repair amount
}

func npcCheckArenaEffects(char *AdventureCharacter, monsterName string) *npcArenaResult {
	now := time.Now().UTC()
	result := &npcArenaResult{}
	fired := false

	// Step 2: Misty debuff — crowd revenge (20%)
	if char.MistyDebuffExpires != nil && now.Before(*char.MistyDebuffExpires) {
		if rand.Float64() < mistyEffectChance {
			dmg := crowdRevengeDmgMin + rand.IntN(crowdRevengeDmgMax-crowdRevengeDmgMin+1)
			line := mistyCrowdRevengeLines[rand.IntN(len(mistyCrowdRevengeLines))]
			line = strings.ReplaceAll(line, "{damage}", fmt.Sprintf("%d", dmg))
			result.Text += "\n\n" + line
			result.PlayerDmg = dmg
			fired = true
		}
	}

	// Step 3: Misty buff — gourmet food (20%)
	if char.MistyBuffExpires != nil && now.Before(*char.MistyBuffExpires) {
		if rand.Float64() < mistyEffectChance {
			line := mistyGourmetFoodLines[rand.IntN(len(mistyGourmetFoodLines))]
			line = strings.ReplaceAll(line, "{enemy}", monsterName)
			result.Text += "\n\n" + line
			result.EnemyDmg = gourmetEnemyDmg
			result.CondRepair = gourmetConditionRepair
			fired = true
		}
	}

	// Step 4: Arina buff — sniper (8%)
	if char.ArinaBuffExpires != nil && now.Before(*char.ArinaBuffExpires) {
		if rand.Float64() < arinaEffectChance {
			line := arinaSniperLines[rand.IntN(len(arinaSniperLines))]
			line = strings.ReplaceAll(line, "{enemy}", monsterName)
			result.Text += "\n\n" + line
			result.SniperKill = true
			fired = true
		}
	}

	if !fired {
		return nil
	}
	return result
}

// ── Equipment Repair (Gourmet Food) ────────────────────────────────────────

// npcRepairMostDamaged heals condition on the player's most damaged equipment slot.
func npcRepairMostDamaged(userID id.UserID, equip map[EquipmentSlot]*AdvEquipment, amount int) {
	if len(equip) == 0 {
		return
	}

	// Find most damaged slot
	var worstSlot EquipmentSlot
	worstCondition := 101
	for slot, eq := range equip {
		if eq.Condition < worstCondition {
			worstCondition = eq.Condition
			worstSlot = slot
		}
	}

	if worstCondition >= 100 {
		return // nothing to repair
	}

	newCond := worstCondition + amount
	if newCond > 100 {
		newCond = 100
	}

	d := db.Get()
	_, err := d.Exec(`UPDATE adventure_equipment SET condition = ? WHERE user_id = ? AND slot = ?`,
		newCond, string(userID), string(worstSlot))
	if err != nil {
		slog.Error("npc: failed to repair equipment", "user", userID, "slot", worstSlot, "err", err)
	}
}

// ── Midnight Reset ─────────────────────────────────────────────────────────

// npcMidnightReset resets message counts and regenerates roll targets.
// Called from midnightReset. Uses a direct DB update to avoid loading/saving
// every character individually.
func npcMidnightReset() {
	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")
	if _, err := d.Exec(`
		UPDATE player_meta
		SET npc_msg_count = 0,
		    npc_msg_count_date = ?,
		    misty_roll_target = ABS(RANDOM()) % 6 + 5,
		    arina_roll_target = ABS(RANDOM()) % 6 + 5`,
		today); err != nil {
		slog.Error("npc: midnight reset failed", "err", err)
	}
}
