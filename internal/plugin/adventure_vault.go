package plugin

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// N4/E1 — the Tier-4 "Established" home unlocks a vault: a fixed pool of slots
// that shelters items from `!sell all`, crafting, and combat consumption. It is
// pure storage — a vaulted item drops out of every play surface (loadAdvInventory
// filters it) until it is taken back — and the plumbing E2 gifting builds on.
const (
	vaultCapacity     = 10 // slots an Established home provides
	vaultMinHouseTier = 4  // Tier 4 "Established" gates the vault
)

// vaultUnlocked reports whether the character's house tier grants a vault, and a
// player-facing refusal line when it does not.
func vaultUnlocked(char *AdventureCharacter) (bool, string) {
	if char.HouseTier >= vaultMinHouseTier {
		return true, ""
	}
	return false, "🔒 A vault comes with an **Established** home (Tier 4). Upgrade your house at `!thom` to unlock " +
		fmt.Sprintf("%d slots of protected storage — vaulted items are safe from `!sell all` and never spent in combat.", vaultCapacity)
}

// handleVaultCmd routes `!adventure vault [store|take] <item>`. The reply-building
// core is client-free (renderVault / vaultStoreItem / vaultTakeItem) so it is
// unit-testable without a Matrix stub; the handler only loads the character and
// sends the result.
func (p *AdventurePlugin) handleVaultCmd(ctx MessageContext, args string) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character. Try `!adventure` to create one first.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. The vault stays locked.")
	}
	if ok, refusal := vaultUnlocked(char); !ok {
		return p.SendDM(ctx.Sender, refusal)
	}

	lower := strings.ToLower(strings.TrimSpace(args))
	var reply string
	switch {
	case lower == "" || lower == "list":
		reply = renderVault(ctx.Sender)
	case strings.HasPrefix(lower, "store "):
		reply = vaultStoreItem(ctx.Sender, strings.TrimSpace(args[len("store "):]))
	case strings.HasPrefix(lower, "take "):
		reply = vaultTakeItem(ctx.Sender, strings.TrimSpace(args[len("take "):]))
	default:
		reply = "Usage: `!adventure vault` to view · `!adventure vault store <item>` · `!adventure vault take <item>`."
	}
	return p.SendDM(ctx.Sender, reply)
}

// renderVault shows what is stowed and how many slots remain.
func renderVault(userID id.UserID) string {
	items, err := loadAdvVault(userID)
	if err != nil {
		return "The vault door won't budge. Try again in a moment."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏛️ **Your Vault** — %d / %d slots\n\n", len(items), vaultCapacity))
	if len(items) == 0 {
		sb.WriteString("Empty. `!adventure vault store <item>` to shelter something from sale or use.")
		return sb.String()
	}
	for _, it := range items {
		name := it.Name
		if it.Temper > 0 {
			name = fmt.Sprintf("%s +%d", name, it.Temper)
		}
		sb.WriteString(fmt.Sprintf("• %s — €%d\n", name, it.Value))
	}
	sb.WriteString("\n`!adventure vault take <item>` to bring one back into play.")
	return sb.String()
}

// vaultStoreItem moves one matching inventory item into the vault. Guards the
// capacity and reports the fresh slot count so the player never has to re-list.
func vaultStoreItem(userID id.UserID, query string) string {
	if query == "" {
		return "Store what? `!adventure vault store <item>`."
	}
	if advVaultCount(userID) >= vaultCapacity {
		return fmt.Sprintf("The vault is full (%d / %d). Take something out first with `!adventure vault take <item>`.", vaultCapacity, vaultCapacity)
	}
	items, err := loadAdvInventory(userID)
	if err != nil {
		return "Couldn't reach your inventory. Try again in a moment."
	}
	match := findInventoryMatch(items, query)
	if match == nil {
		return fmt.Sprintf("No inventory item matches %q. Check `!adventure inventory`.", query)
	}
	moved, err := setItemVaulted(userID, match.ID, true)
	if err != nil || !moved {
		return "Couldn't stow that item. Try again in a moment."
	}
	return fmt.Sprintf("🏛️ Stowed **%s** in the vault. %d / %d slots used.", match.Name, advVaultCount(userID), vaultCapacity)
}

// vaultTakeItem returns one matching vaulted item to the active inventory.
func vaultTakeItem(userID id.UserID, query string) string {
	if query == "" {
		return "Take what? `!adventure vault take <item>`."
	}
	items, err := loadAdvVault(userID)
	if err != nil {
		return "The vault door won't budge. Try again in a moment."
	}
	match := findInventoryMatch(items, query)
	if match == nil {
		return fmt.Sprintf("Nothing in the vault matches %q. Check `!adventure vault`.", query)
	}
	moved, err := setItemVaulted(userID, match.ID, false)
	if err != nil || !moved {
		return "Couldn't retrieve that item. Try again in a moment."
	}
	return fmt.Sprintf("Took **%s** out of the vault and back into your pack. %d / %d slots used.", match.Name, advVaultCount(userID), vaultCapacity)
}
