package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── NPC Constants ──────────────────────────────────────────────────────────

const (
	mistyCost         = 100
	arinaCost         = 5000
	npcCooldownDays   = 7
	npcEncounterChance = 0.075 // 7.5%
	npcBuffDuration   = 7 * 24 * time.Hour

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

	now := time.Now().UTC()

	var opening, prompt string
	switch npc {
	case "misty":
		char.MistyLastSeen = &now
		opening = mistyOpenings[rand.IntN(len(mistyOpenings))]
		prompt = fmt.Sprintf("👤 A woman approaches you.\n\n_%s_\n\n"+
			"Reply `yes` to give €%d, or `no` to walk away.", opening, mistyCost)
	case "arina":
		char.ArinaLastSeen = &now
		opening = arinaOpenings[rand.IntN(len(arinaOpenings))]
		prompt = fmt.Sprintf("💎 A woman in expensive clothing stops you.\n\n_%s_\n\n"+
			"Reply `yes` to pay €%d, or `no` to decline.", opening, arinaCost)
	}

	if err := saveAdvCharacter(char); err != nil {
		slog.Error("npc: failed to save last_seen", "user", userID, "npc", npc, "err", err)
		return
	}

	// Don't overwrite an existing pending interaction (shop, treasure, etc.)
	if _, occupied := p.pending.Load(string(userID)); occupied {
		return
	}

	// Set pending interaction
	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "npc_encounter",
		Data:      npc,
		ExpiresAt: time.Now().Add(5 * time.Minute),
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
			return p.SendDM(ctx.Sender, "You reach for your gold but there isn't enough. She sees this. She doesn't say anything.\n\n_"+mistyDeclineLine+"_")
		}

		if !p.euro.Debit(ctx.Sender, float64(mistyCost), "misty_donation") {
			char.MistyDebuffExpires = expires
			if err := saveAdvCharacter(char); err != nil {
				slog.Error("npc: failed to save misty debuff", "user", ctx.Sender, "err", err)
			}
			return p.SendDM(ctx.Sender, "You reach for your gold but there isn't enough. She sees this. She doesn't say anything.\n\n_"+mistyDeclineLine+"_")
		}
		char.MistyBuffExpires = expires
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("npc: failed to save misty buff", "user", ctx.Sender, "err", err)
		}

		reply := mistyAcceptLines[rand.IntN(len(mistyAcceptLines))]
		return p.SendDM(ctx.Sender, fmt.Sprintf("_%s_", reply))
	}

	// Declined
	char.MistyDebuffExpires = expires
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("npc: failed to save misty debuff", "user", ctx.Sender, "err", err)
	}
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
		char.ArinaBuffExpires = expires
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("npc: failed to save arina buff", "user", ctx.Sender, "err", err)
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf("_%s_", arinaAcceptLine))
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
	EnemyDmg   int   // damage dealt to enemy
	PlayerDmg  int   // damage dealt to player
	SniperKill bool  // enemy instant kill
	CondRepair int   // equipment condition repair amount
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
	_, err := d.Exec(`
		UPDATE adventure_characters
		SET npc_msg_count = 0,
		    npc_msg_count_date = ?,
		    misty_roll_target = ABS(RANDOM()) % 6 + 5,
		    arina_roll_target = ABS(RANDOM()) % 6 + 5`,
		today)
	if err != nil {
		slog.Error("npc: midnight reset failed", "err", err)
	}
}
