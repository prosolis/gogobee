package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Masterwork Skill Drop Definitions ──────────────────────────────────────

type MasterworkDef struct {
	Slot        EquipmentSlot
	Activity    AdvActivityType
	SkillSource string  // "mining", "fishing", "foraging"
	Tier        int     // 1-5, matches location tier
	Name        string
	Description string  // character sheet / trade listing
	DropRate    float64 // per-tier: 0.05, 0.04, 0.03, 0.02, 0.015
}

var masterworkDefs = []MasterworkDef{
	// ── Mining → Weapon (sword) ────────────────────────────────────────────
	{
		Slot: SlotWeapon, Activity: AdvActivityMining, SkillSource: "mining", Tier: 1,
		Name: "The Ore You Know", DropRate: 0.05,
		Description: "Found wedged in a copper seam. Pre-sharpened by geological pressure over approximately four million years. Somehow better than what the shop sells for twice the effort.",
	},
	{
		Slot: SlotWeapon, Activity: AdvActivityMining, SkillSource: "mining", Tier: 2,
		Name: "Shaft Splitter", DropRate: 0.04,
		Description: "This came out of an iron vein looking exactly like this. You didn't forge it. You found it. The iron vein did all the work. You're going to take full credit.",
	},
	{
		Slot: SlotWeapon, Activity: AdvActivityMining, SkillSource: "mining", Tier: 3,
		Name: "The Vein Attempt", DropRate: 0.03,
		Description: "Named after the desperate prayer you said when your pickaxe hit something that wasn't ore. It wasn't a god that answered. It was a sword. Close enough.",
	},
	{
		Slot: SlotWeapon, Activity: AdvActivityMining, SkillSource: "mining", Tier: 4,
		Name: "Lode Bearer", DropRate: 0.02,
		Description: "A serious blade pulled from a serious depth. The edge was formed under pressure that would have killed you. Something down there was planning ahead.",
	},
	{
		Slot: SlotWeapon, Activity: AdvActivityMining, SkillSource: "mining", Tier: 5,
		Name: "The Motherload", DropRate: 0.015,
		Description: "There is no good explanation for why this was in the rock. The rock isn't talking. You're not asking. You've decided to stop thinking about it and just use it.",
	},

	// ── Fishing → Armor (chest) ────────────────────────────────────────────
	{
		Slot: SlotArmor, Activity: AdvActivityFishing, SkillSource: "fishing", Tier: 1,
		Name: "Mudscale Plate", DropRate: 0.05,
		Description: "Assembled from scales shed by something that lives in the Muddy Pond and apparently grew very large doing it. Smells like the pond. Always will.",
	},
	{
		Slot: SlotArmor, Activity: AdvActivityFishing, SkillSource: "fishing", Tier: 2,
		Name: "Riverglass Mail", DropRate: 0.04,
		Description: "The links are formed from some kind of calcified river growth — not metal, exactly, but harder than it has any right to be. It came up on your line. You're choosing not to investigate further.",
	},
	{
		Slot: SlotArmor, Activity: AdvActivityFishing, SkillSource: "fishing", Tier: 3,
		Name: "Tidal Cuirass", DropRate: 0.03,
		Description: "Whatever this came from, it lived in tidal zones and learned to take a beating. The surface is worn smooth from years of wave impact. It fits like it was meant for something your size. It wasn't.",
	},
	{
		Slot: SlotArmor, Activity: AdvActivityFishing, SkillSource: "fishing", Tier: 4,
		Name: "Pelagic Plate", DropRate: 0.02,
		Description: "Deep sea armor that ended up in Sable Cove via a process you would rather not reconstruct. The plating is layered like fish scales but heavier. Cold to the touch even in direct sun. You've stopped noticing.",
	},
	{
		Slot: SlotArmor, Activity: AdvActivityFishing, SkillSource: "fishing", Tier: 5,
		Name: "The Deepforged Carapace", DropRate: 0.015,
		Description: "This came up on the line from the Abyssal Trench and the line almost didn't hold. It is not from a fish. It is not from anything you have seen or want to see. It fits perfectly. You find this more unsettling than the alternative.",
	},

	// ── Foraging �� Boots ───────────────────────────────────────────────────
	{
		Slot: SlotBoots, Activity: AdvActivityForaging, SkillSource: "foraging", Tier: 1,
		Name: "Bark Treads", DropRate: 0.05,
		Description: "Woven from bark that was evidently done with being a tree. Lightweight, grippy, and somehow waterproof. Found under a log in the Sunlit Meadow where they had no business being. Better than the shop's offering, which tells you something about the shop.",
	},
	{
		Slot: SlotBoots, Activity: AdvActivityForaging, SkillSource: "foraging", Tier: 2,
		Name: "Thornweave Boots", DropRate: 0.04,
		Description: "Made entirely from briars that have been worked into something wearable, which should not be possible. They don't scratch. They don't catch. They move like they know where you're going before you do.",
	},
	{
		Slot: SlotBoots, Activity: AdvActivityForaging, SkillSource: "foraging", Tier: 3,
		Name: "The Wandering Sole", DropRate: 0.03,
		Description: "These have been places. The sole is worn in the pattern of a specific path you've never walked. Whoever wore them before you covered serious ground. They handed off the work to you without being asked.",
	},
	{
		Slot: SlotBoots, Activity: AdvActivityForaging, SkillSource: "foraging", Tier: 4,
		Name: "Deepwood Striders", DropRate: 0.02,
		Description: "The forest didn't make these. The forest let these happen. There's a difference and you feel it when you put them on — like being tolerated by something very old that has decided you're not worth stopping.",
	},
	{
		Slot: SlotBoots, Activity: AdvActivityForaging, SkillSource: "foraging", Tier: 5,
		Name: "The Last Step", DropRate: 0.015,
		Description: "Found at the edge of the Fungal Dark where the light stops. You almost didn't go that far. Something about wearing them suggests you will always go that far, from now on. This is either good news or the other kind.",
	},
}

// masterworkDefFor returns the masterwork definition for a given activity and tier.
func masterworkDefFor(activity AdvActivityType, tier int) *MasterworkDef {
	for i := range masterworkDefs {
		if masterworkDefs[i].Activity == activity && masterworkDefs[i].Tier == tier {
			return &masterworkDefs[i]
		}
	}
	return nil
}

// ── Drop Flavor Text (DM) ─────────────────────────────────────────────────

func masterworkDropFlavorText(activity AdvActivityType, tier int) string {
	switch activity {
	case AdvActivityMining:
		switch {
		case tier <= 2:
			return "Your pickaxe hits something that rings differently. Metallic but not ore. You dig around it. It is a sword. It was in the rock. You decide this is fine and put it in your bag."
		case tier == 3:
			return "The vein opens up and there it is. A blade, fully formed, embedded in silver ore like it grew there. Your mining handbook does not cover this. You take it anyway."
		case tier == 4:
			return "Deep in the forge level, your pickaxe sparks against something that sparks back. You spend twenty minutes clearing the rock around it. When you pull it free it catches the lamplight in a way that makes the other miners stop working. You pretend not to notice."
		default:
			return "The deepest seam in the Abyssal Mine gives up something it was keeping. No drama. No earthquake. The rock just opens and there it is. A blade that looks like it was waiting. You take it. You don't ask how long it's been there. You don't want to know."
		}
	case AdvActivityFishing:
		switch {
		case tier <= 2:
			return "You reel in something heavy that turns out not to be a fish. It is armor, or something shaped like armor, and it is in better condition than anything at the shop. You rinse it off in the water and put it on. The pond smells follow it everywhere."
		case tier == 3:
			return "The line goes dead-weight and you pull up a cuirass that was clearly not in the catch plan. It's heavier than it looks and colder than the water should account for. You add it to your haul. The fish you didn't catch would have been worth less anyway."
		case tier == 4:
			return "Something takes your bait and doesn't run. It just sinks. You reel against it for ten minutes before the line comes up with armor attached and nothing else. No creature. No explanation. The armor is exceptional. You decide this is the important part."
		default:
			return "The Abyssal Trench gives up the carapace without a fight, which is somehow worse than if it had fought. You didn't hook it. It was just there on the line when you pulled up. It fits. You start fishing again immediately because thinking about it isn't an option you're entertaining."
		}
	case AdvActivityForaging:
		switch {
		case tier <= 2:
			return "Under a root cluster in the foraging area, half-buried in moss: a pair of boots. They fit. You don't know what to do with that information so you put them on and keep foraging. They are noticeably better than what you were wearing."
		case tier == 3:
			return "You push through a dense section of the Ancient Forest and find a clearing that wasn't on any route you've taken before. In the center, on a flat rock, a pair of boots. No tracks around the rock. No sign anyone left them. They're your size. You take them and don't mention the clearing to anyone."
		case tier == 4:
			return "The Verdant Depths foraging run turns up the usual haul plus one thing that isn't usual: boots, hanging from a branch at eye level, laced and ready. The branch is forty feet up. The boots are at eye level. You decide to focus on the boots and not the branch."
		default:
			return "The Fungal Dark produces your haul and then, at the very last moment before you turn back, a pair of boots you didn't put there. They are better than anything in the shop. The fungi around them are growing in an outward pattern, like they were avoiding something. You put the boots on. The fungi don't react. You don't find this reassuring."
		}
	}
	return ""
}

// ── Drop Logic ─────────────────────────────────────────────────────────────

func (p *AdventurePlugin) checkMasterworkDrop(userID id.UserID, char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, loc *AdvLocation, outcome AdvOutcomeType) {
	// Only full-success outcomes drop masterwork (success or exceptional)
	if outcome != AdvOutcomeSuccess && outcome != AdvOutcomeExceptional {
		return
	}

	def := masterworkDefFor(loc.Activity, loc.Tier)
	if def == nil {
		return // no masterwork available for this activity+tier (e.g. dungeon)
	}

	// Roll for drop
	if rand.Float64() >= def.DropRate {
		return
	}

	// Check current equipment in target slot
	existing := equip[def.Slot]

	// Determine: auto-equip, inventory, or silent discard
	autoEquip := false
	if existing == nil {
		autoEquip = true
	} else if existing.Masterwork {
		if def.Tier > existing.Tier {
			autoEquip = true // upgrade over lower-tier masterwork
		} else {
			return // silent discard: same or higher tier masterwork already equipped
		}
	} else {
		autoEquip = true // any masterwork > shop gear
	}

	// First-drop detection
	isFirstDrop := char.MasterworkDropsReceived == 0
	char.MasterworkDropsReceived++
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("adventure: failed to save masterwork counter", "user", userID, "err", err)
	}

	if autoEquip {
		// Equip directly
		eq := equip[def.Slot]
		if eq == nil {
			eq = &AdvEquipment{Slot: def.Slot}
			equip[def.Slot] = eq
		}
		eq.Tier = def.Tier
		eq.Condition = 100
		eq.Name = def.Name
		eq.ActionsUsed = 0
		eq.ArenaTier = 0
		eq.ArenaSet = ""
		eq.Masterwork = true
		eq.SkillSource = def.SkillSource

		if err := saveAdvEquipment(userID, eq); err != nil {
			slog.Error("adventure: failed to save masterwork drop", "user", userID, "slot", def.Slot, "err", err)
			return
		}
	} else {
		// Send to inventory
		item := AdvItem{
			Name:        def.Name,
			Type:        "MasterworkGear",
			Tier:        def.Tier,
			Value:       0,
			Slot:        def.Slot,
			SkillSource: def.SkillSource,
		}
		if err := addAdvInventoryItem(userID, item); err != nil {
			slog.Error("adventure: failed to add masterwork to inventory", "user", userID, "err", err)
			return
		}
	}

	// Build DM text
	var sb strings.Builder

	if isFirstDrop {
		sb.WriteString("_This doesn't come from the shop._\n\n")
	}

	// Flavor text
	flavor := masterworkDropFlavorText(def.Activity, def.Tier)
	if flavor != "" {
		sb.WriteString(fmt.Sprintf("_%s_\n\n", flavor))
	}

	sb.WriteString(fmt.Sprintf("⭐ **Masterwork Drop: %s** (T%d)\n\n", def.Name, def.Tier))
	sb.WriteString(fmt.Sprintf("_%s_\n\n", def.Description))
	sb.WriteString(fmt.Sprintf("Masterwork %s — 1.25x effectiveness, +5%% %s success.\n", slotTitle(def.Slot), def.SkillSource))

	if autoEquip {
		sb.WriteString("Equipped automatically.")
	} else {
		sb.WriteString("Added to inventory. Use `!adventure equip` to equip it.")
	}

	p.SendDM(userID, sb.String())

	// Room announcement (tiered)
	p.postMasterworkAnnouncement(char.DisplayName, def, loc.Name)
}

// ── Room Announcements ─────────────────────────────────────────────────────

func (p *AdventurePlugin) postMasterworkAnnouncement(playerName string, def *MasterworkDef, locName string) {
	if def.Tier <= 2 {
		return // DM only, no room post
	}

	gr := gamesRoom()
	if gr == "" {
		return
	}

	verb := "working"
	switch def.Activity {
	case AdvActivityMining:
		verb = "mining"
	case AdvActivityFishing:
		verb = "fishing"
	case AdvActivityForaging:
		verb = "foraging"
	}

	var msg string
	switch {
	case def.Tier == 3:
		msg = fmt.Sprintf("%s found something unexpected while %s. The %s suggests they've been at this longer than most.",
			playerName, verb, def.Name)
	case def.Tier == 4:
		domain := "mine"
		switch def.Activity {
		case AdvActivityFishing:
			domain = "sea"
		case AdvActivityForaging:
			domain = "forest"
		}
		msg = fmt.Sprintf("%s pulled %s out of %s. ⭐ Masterwork %s. The shop doesn't sell this. The %s apparently does.",
			playerName, def.Name, locName, slotTitle(def.Slot), domain)
	case def.Tier == 5:
		msg = fmt.Sprintf("%s has %s. ⭐⭐ Tier 5 Masterwork %s, found in %s. Nobody else has this. The circumstances of its discovery have been described as \"unclear\" and left at that.",
			playerName, def.Name, slotTitle(def.Slot), locName)
	}

	if msg != "" {
		p.SendMessage(gr, msg)
	}
}

// ── Equip Command ──────────────────────────────────────────────────────────

type advPendingMasterworkEquip struct {
	Items []AdvItem
}

type advPendingMasterworkConfirm struct {
	Item AdvItem
}

func (p *AdventurePlugin) handleEquipCmd(ctx MessageContext) error {
	_, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found. Use `!adventure` to start.")
	}

	items, err := loadAdvInventory(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load inventory.")
	}

	// Filter to masterwork items only
	var mwItems []AdvItem
	for _, it := range items {
		if it.Type == "MasterworkGear" {
			mwItems = append(mwItems, it)
		}
	}

	if len(mwItems) == 0 {
		return p.SendDM(ctx.Sender, "You have no Masterwork gear waiting to be equipped. Go find some.")
	}

	equip, err := loadAdvEquipment(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load equipment.")
	}

	// Build listing
	var sb strings.Builder
	sb.WriteString("⭐ **Your unequipped Masterwork gear:**\n\n")

	for i, it := range mwItems {
		current := equip[it.Slot]
		currentDesc := "None"
		if current != nil {
			tag := fmt.Sprintf("T%d shop", current.Tier)
			if current.Masterwork {
				tag = fmt.Sprintf("T%d ⭐", current.Tier)
			} else if current.ArenaTier > 0 {
				tag = fmt.Sprintf("T%d ⚔️ arena", current.Tier)
			}
			currentDesc = fmt.Sprintf("%s (%s)", current.Name, tag)
		}
		sb.WriteString(fmt.Sprintf("%d. %s (T%d %s) — currently: %s\n",
			i+1, it.Name, it.Tier, slotTitle(it.Slot), currentDesc))
	}

	sb.WriteString("\nReply with a number to equip, or \"cancel\".")

	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "masterwork_equip",
		Data:      &advPendingMasterworkEquip{Items: mwItems},
		ExpiresAt: time.Now().Add(5 * time.Minute),
	})

	return p.SendDM(ctx.Sender, sb.String())
}

func (p *AdventurePlugin) handleMasterworkEquipReply(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingMasterworkEquip)
	reply := strings.TrimSpace(ctx.Body)

	if strings.EqualFold(reply, "cancel") {
		return p.SendDM(ctx.Sender, "Equip cancelled.")
	}

	// Parse number
	idx := 0
	for _, c := range reply {
		if c >= '0' && c <= '9' {
			idx = idx*10 + int(c-'0')
		} else {
			break
		}
	}
	idx-- // 1-indexed to 0-indexed

	if idx < 0 || idx >= len(data.Items) {
		return p.SendDM(ctx.Sender, "Invalid selection. Reply with a number from the list, or \"cancel\".")
	}

	selected := data.Items[idx]
	equip, err := loadAdvEquipment(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load equipment.")
	}

	current := equip[selected.Slot]

	// Warn if downgrading
	warning := ""
	if current != nil && current.ArenaTier > 0 {
		warning = fmt.Sprintf("\n⚠️ This will replace your **%s** (Arena gear). Are you sure?\n", current.Name)
	} else if current != nil && current.Masterwork && current.Tier > selected.Tier {
		warning = fmt.Sprintf("\n⚠️ %s is Tier %d. Your current %s is Tier %d. You'll be downgrading tier.\n",
			selected.Name, selected.Tier, current.Name, current.Tier)
	}

	currentName := "nothing"
	if current != nil {
		currentName = current.Name
	}

	// Store confirmation pending
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "masterwork_equip_confirm",
		Data:      &advPendingMasterworkConfirm{Item: selected},
		ExpiresAt: time.Now().Add(5 * time.Minute),
	})

	def := masterworkDefFor(slotToActivity(selected.Slot), selected.Tier)
	bonusDesc := ""
	if def != nil {
		bonusDesc = fmt.Sprintf("\n%s gives 1.25x %s effectiveness and +5%% %s success.\n", selected.Name, slotTitle(selected.Slot), def.SkillSource)
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("Equip **%s** (T%d ⭐ %s)?%s\nThis replaces your %s. The old item goes to inventory.%s\nReply \"yes\" to confirm.",
		selected.Name, selected.Tier, slotTitle(selected.Slot), warning, currentName, bonusDesc))
}

func (p *AdventurePlugin) handleMasterworkEquipConfirm(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingMasterworkConfirm)
	reply := strings.TrimSpace(strings.ToLower(ctx.Body))

	if reply != "yes" {
		return p.SendDM(ctx.Sender, "Equip cancelled.")
	}

	selected := data.Item

	equip, err := loadAdvEquipment(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load equipment.")
	}

	current := equip[selected.Slot]

	// If current equipment is special, move it to inventory
	if current != nil && (current.Masterwork || current.ArenaTier > 0) {
		oldItem := AdvItem{
			Name:        current.Name,
			Type:        "MasterworkGear",
			Tier:        current.Tier,
			Value:       0,
			Slot:        current.Slot,
			SkillSource: current.SkillSource,
		}
		if current.ArenaTier > 0 {
			oldItem.Type = "ArenaGear"
		}
		if err := addAdvInventoryItem(ctx.Sender, oldItem); err != nil {
			slog.Error("adventure: failed to move old equip to inventory", "user", ctx.Sender, "err", err)
		}
	}

	// Equip the selected item
	eq := equip[selected.Slot]
	if eq == nil {
		eq = &AdvEquipment{Slot: selected.Slot}
		equip[selected.Slot] = eq
	}
	eq.Tier = selected.Tier
	eq.Condition = 100
	eq.Name = selected.Name
	eq.ActionsUsed = 0
	eq.ArenaTier = 0
	eq.ArenaSet = ""
	eq.Masterwork = true
	eq.SkillSource = selected.SkillSource

	if err := saveAdvEquipment(ctx.Sender, eq); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save equipment. Try again.")
	}

	// Remove from inventory
	if err := removeAdvInventoryItem(selected.ID); err != nil {
		slog.Error("adventure: failed to remove equipped item from inventory", "user", ctx.Sender, "err", err)
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("**%s** equipped. ⭐ Masterwork %s, Tier %d.", selected.Name, slotTitle(selected.Slot), selected.Tier))
}

// slotToActivity converts an EquipmentSlot to the activity that drops masterwork for it.
func slotToActivity(s EquipmentSlot) AdvActivityType {
	switch s {
	case SlotWeapon:
		return AdvActivityMining
	case SlotArmor:
		return AdvActivityFishing
	case SlotBoots:
		return AdvActivityForaging
	}
	return ""
}
