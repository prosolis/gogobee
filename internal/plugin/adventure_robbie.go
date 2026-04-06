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

// ── Robbie the Friendly Bandit ───────────────────────────────────────────────
//
// Robbie is an automated NPC who visits players at a random hour each day,
// takes sub-tier inventory items, leaves €50 per item, donates everything
// to the community pot, and occasionally drops a Get Out of Medical Debt
// Free card when collecting Masterwork gear.

// In-memory target hour — picked fresh each day, regenerated on restart.
var (
	robbieTargetHour int    = -1
	robbieTargetDay  string // "2006-01-02"
)

// ── Ticker ───────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) robbieTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		dateKey := now.Format("2006-01-02")

		// At midnight (or first tick of the day), pick today's target hour.
		if robbieTargetDay != dateKey {
			robbieTargetHour = 8 + rand.IntN(14) // 8–21 inclusive
			robbieTargetDay = dateKey
			slog.Info("adventure: robbie target hour set", "hour", robbieTargetHour, "date", dateKey)
		}

		if now.Hour() != robbieTargetHour || now.Minute() != 0 {
			continue
		}

		jobName := "adventure_robbie"
		if db.JobCompleted(jobName, dateKey) {
			continue
		}

		slog.Info("adventure: robbie sweep starting")
		p.robbieVisitAll()
		db.MarkJobCompleted(jobName, dateKey)
	}
}

// ── Visit All Players ────────────────────────────────────────────────────────

func (p *AdventurePlugin) robbieVisitAll() {
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("adventure: robbie: failed to load characters", "err", err)
		return
	}

	rand.Shuffle(len(chars), func(i, j int) { chars[i], chars[j] = chars[j], chars[i] })

	for i, char := range chars {
		if !char.Alive {
			continue
		}

		// Jitter between players to avoid Matrix rate limits.
		if i > 0 {
			time.Sleep(time.Duration(1000+rand.IntN(2000)) * time.Millisecond)
		}

		p.robbieVisitPlayer(char.UserID, char.DisplayName)
	}
}

// ── Single Player Visit ──────────────────────────────────────────────────────

func (p *AdventurePlugin) robbieVisitPlayer(userID id.UserID, displayName string) {
	mu := p.advUserLock(userID)
	mu.Lock()
	defer mu.Unlock()

	// Load inventory + equipped gear
	inv, err := loadAdvInventory(userID)
	if err != nil {
		return
	}
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		return
	}

	// Find qualifying items
	qualifying := robbieQualifyingItems(inv, equip)
	if len(qualifying) == 0 {
		return
	}

	// 40% chance of visiting
	if rand.Float64() >= 0.40 {
		return
	}

	// Execute the visit — collect items
	var totalPayout int64
	var communityTotal int64
	var masterworkTaken bool
	var takenItems []AdvItem

	for _, item := range qualifying {
		if err := removeAdvInventoryItem(item.ID); err != nil {
			slog.Error("adventure: robbie: failed to remove item", "item_id", item.ID, "err", err)
			continue
		}
		takenItems = append(takenItems, item)
		totalPayout += 50
		communityTotal += item.Value
		if item.Type == "MasterworkGear" {
			masterworkTaken = true
		}
	}

	if len(takenItems) == 0 {
		return
	}

	// Credit player
	p.euro.Credit(userID, float64(totalPayout), "robbie_handling_fee")

	// Donate to community pot
	if communityTotal > 0 {
		communityPotAdd(int(communityTotal))
	}

	// Handle Get Out of Medical Debt Free card
	hasCard := robbiePlayerHasCard(userID)
	gaveCard := false
	if masterworkTaken && !hasCard {
		_ = addAdvInventoryItem(userID, AdvItem{
			Name:  "Get Out of Medical Debt Free",
			Type:  "card",
			Tier:  0,
			Value: 0,
		})
		gaveCard = true
	}

	// Update visit count
	char, err := loadAdvCharacter(userID)
	if err == nil {
		char.RobbieVisitCount++
		_ = saveAdvCharacter(char)
	}

	// Send DM
	dm := renderRobbieDM(userID, takenItems, totalPayout, masterworkTaken, gaveCard)
	if err := p.SendDM(userID, dm); err != nil {
		slog.Error("adventure: robbie: failed to send DM", "user", userID, "err", err)
	}

	// Room announcement
	gr := gamesRoom()
	if gr != "" {
		announcement := renderRobbieRoomAnnouncement(displayName, len(takenItems), totalPayout, masterworkTaken, gaveCard)
		p.SendMessage(gr, announcement)
	}
}

// ── Qualifying Items ─────────────────────────────────────────────────────────

func robbieQualifyingItems(inv []AdvItem, equip map[EquipmentSlot]*AdvEquipment) []AdvItem {
	var result []AdvItem
	for _, item := range inv {
		// Only gear with a slot (skip ores, wood, fruit, gems, fish, etc.)
		if item.Slot == "" {
			continue
		}
		// Never touch Arena gear or cards
		if item.Type == "ArenaGear" || item.Type == "card" {
			continue
		}

		eq, hasSlot := equip[item.Slot]
		if !hasSlot {
			continue
		}

		if item.Type == "MasterworkGear" {
			// Take MW items only if equipped piece in same slot is also MW
			// and has effective tier >= this item's effective tier.
			if eq.Masterwork && advEffectiveTier(eq) >= float64(item.Tier)*1.25 {
				result = append(result, item)
			}
		} else {
			// Regular shop gear: take if item tier < equipped tier
			if item.Tier < eq.Tier {
				result = append(result, item)
			}
		}
	}
	return result
}

// ── Card Check ───────────────────────────────────────────────────────────────

func robbiePlayerHasCard(userID id.UserID) bool {
	inv, err := loadAdvInventory(userID)
	if err != nil {
		return false
	}
	for _, item := range inv {
		if item.Type == "card" && item.Name == "Get Out of Medical Debt Free" {
			return true
		}
	}
	return false
}

// ── DM Rendering ─────────────────────────────────────────────────────────────

func renderRobbieDM(userID id.UserID, items []AdvItem, total int64, mwTaken, gaveCard bool) string {
	var sb strings.Builder

	// Opening
	opening, _ := advPickFlavor(robbieOpenings, userID, "robbie_opening")
	if strings.Contains(opening, "%d") {
		opening = fmt.Sprintf(opening, total)
	}
	sb.WriteString(opening)
	sb.WriteString("\n\n")

	// Itemized list
	sb.WriteString("Items collected:\n\n")
	cardShownOnLine := false
	for _, item := range items {
		emoji := slotEmoji(item.Slot)
		if item.Type == "MasterworkGear" {
			sb.WriteString(fmt.Sprintf("  %s  %s (Masterwork T%d)   → €50", emoji, item.Name, item.Tier))
			if gaveCard && !cardShownOnLine {
				sb.WriteString("  + 🃏 Get Out of Medical Debt Free card")
				cardShownOnLine = true
			}
		} else {
			sb.WriteString(fmt.Sprintf("  %s  %s (T%d)   → €50", emoji, item.Name, item.Tier))
		}
		sb.WriteByte('\n')
	}

	sb.WriteString(fmt.Sprintf("\nTotal left for you: €%d\n", total))
	sb.WriteString("Everything else donated to the community pot. Good on ya.\n\n")

	// Context line
	if mwTaken {
		if gaveCard {
			sb.WriteString(robbieMasterworkGotCard)
		} else {
			sb.WriteString(robbieMasterworkAlreadyHas)
		}
	} else {
		sb.WriteString(fmt.Sprintf(robbieAllShopGear, total))
	}
	sb.WriteString("\n\n")

	// Closing
	closing, _ := advPickFlavor(robbieClosings, userID, "robbie_closing")
	sb.WriteString(closing)

	return sb.String()
}

// ── Room Announcement ────────────────────────────────────────────────────────

func renderRobbieRoomAnnouncement(name string, count int, total int64, mwTaken, gaveCard bool) string {
	if mwTaken && gaveCard {
		return fmt.Sprintf(robbieRoomMasterworkCard, name, total)
	}
	if mwTaken && !gaveCard {
		return fmt.Sprintf(robbieRoomMasterworkAlreadyHas, name, total)
	}
	return fmt.Sprintf(robbieRoomStandard, name, count, total)
}
