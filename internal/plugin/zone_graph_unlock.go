package plugin

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Thieves' tools — the counterweight to a failed skill check.
//
// A Perception / stat-check lock rolls once per (run, edge) and the roll is
// seeded, deliberately, so re-reading the fork can't reroll it (plan §G5). That
// half shipped; the other half never did, so a bad roll simply deleted a branch
// of the graph for the rest of the run with no recourse at all — worst for a
// solo low-WIS character, who quietly loses routes they never learn existed.
//
// Tools are that recourse: a consumable that answers the check instead of the
// character. They are deliberately NOT a skeleton key. A key lock is a quest
// token, a level-min lock is progression, and a region-clear lock is structure —
// none of those are "you rolled badly", so none of them are pickable. Tools open
// exactly the locks that luck closed.

// thievesToolsName is the inventory item name. Matching is case-folded so a
// player typing it back at the shop doesn't have to find the apostrophe.
const thievesToolsName = "Thieves' Tools"

// thievesToolsItemType keeps them out of the combat consumable scan. They are a
// utility item like the medical-debt card, not something the fight engine may
// spend on the player's behalf.
const thievesToolsItemType = "tool"

// thievesToolsPrice is what Luigi charges. Priced against a T1 consumable so
// carrying a couple is a routine purchase rather than a considered one — the
// point is that no run is ever hard-walled, not to open a money sink.
const thievesToolsPrice int64 = 600

// pickableLock reports whether tools can answer this lock. Only the two
// dice-driven kinds qualify; see the file comment for why the rest don't.
func pickableLock(kind string) bool {
	return kind == string(LockPerception) || kind == string(LockStatCheck)
}

// isThievesToolsReply matches a shop reply against the tools.
//
// Deliberately not "is the reply a substring of the name": that predicate is
// true for a bare "s", for "to", and for an empty message body — and this branch
// sits ahead of the consumable list, so a stray keystroke in the Supplies view
// would silently bill the player €600. Match on the item's own words instead.
func isThievesToolsReply(reply string) bool {
	return containsFold(reply, "thieves") || containsFold(reply, "thief") ||
		containsFold(reply, "tools")
}

// thievesToolsHeld returns the inventory row IDs of every set the player is
// carrying. One read answers both "have they got any" and "how many are left",
// which the unlock flow would otherwise ask the inventory table three times.
func thievesToolsHeld(userID id.UserID) []int64 {
	inv, err := loadAdvInventory(userID)
	if err != nil {
		return nil
	}
	var ids []int64
	for _, it := range inv {
		if strings.EqualFold(it.Name, thievesToolsName) {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

// findThievesTools returns the inventory row ID of one set of tools, or ok=false
// if the player is carrying none.
func findThievesTools(userID id.UserID) (int64, bool) {
	held := thievesToolsHeld(userID)
	if len(held) == 0 {
		return 0, false
	}
	return held[0], true
}

// countThievesTools reports how many sets the player carries, for the "N left"
// line after a use.
func countThievesTools(userID id.UserID) int {
	return len(thievesToolsHeld(userID))
}

// zoneCmdUnlock handles `!zone unlock <n>`: spend one set of thieves' tools to
// open a locked fork option, then commit the move exactly as `!zone go <n>`
// would. Every guard zoneCmdGo applies applies here too — same run, same leader
// rule, same mid-fight refusal — because this is that command with a different
// admission price.
func (p *AdventurePlugin) zoneCmdUnlock(ctx MessageContext, rest string) error {
	run, isLeader, err := activeZoneRunFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	if !isLeader {
		return p.SendDM(ctx.Sender, msgLeaderPicksPath)
	}
	if cs, _ := activeCombatSessionFor(ctx.Sender); cs != nil {
		return p.SendDM(ctx.Sender, "⚔️ Finish your fight first — `!attack` or `!flee`.")
	}
	pf, derr := decodePendingFork(run.NodeChoices)
	if derr != nil {
		return p.SendDM(ctx.Sender, "Couldn't decode pending fork: "+derr.Error())
	}
	if pf == nil {
		return p.SendDM(ctx.Sender, "No fork pending. Use "+continueHint(ctx.Sender))
	}

	held := thievesToolsHeld(ctx.Sender)

	rest = strings.TrimSpace(rest)
	if rest == "" {
		zone := zoneOrFallback(run.ZoneID)
		return p.SendDM(ctx.Sender, "**Which one?**\n\n"+renderForkPrompt(zone, *pf)+
			fmt.Sprintf("\n\n_`!zone unlock <n>` — spends one set of %s. You carry %d._",
				thievesToolsName, len(held)))
	}
	choice := atoiSafe(rest)
	if choice < 1 || choice > len(pf.Options) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Choice must be a number from the menu (1–%d).", len(pf.Options)))
	}
	chosen := pf.Options[choice-1]

	if chosen.Unlocked {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"**%s** is already open — no need to spend tools. `!zone go %d`.", chosen.Label, choice))
	}
	if !pickableLock(chosen.Lock) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"🔒 Tools won't help here. %s\n\n_Thieves' tools answer a failed check, not a locked gate._",
			lockRefusalFor(chosen)))
	}
	if len(held) == 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"🔒 **%s** needs %s and you're carrying none.\n\nLuigi stocks them under `!shop` → Supplies.",
			chosen.Label, thievesToolsName))
	}
	if rerr := removeAdvInventoryItem(held[0]); rerr != nil {
		return p.SendDM(ctx.Sender, "Couldn't spend the tools: "+rerr.Error())
	}

	// The tools answered the check, so the option is open from here on. Commit
	// it back to the pending fork before advancing: if the advance fails, the
	// player has paid and must not be told the door is shut again.
	pf.Options[choice-1].Unlocked = true
	pf.Options[choice-1].Reason = "opened with " + thievesToolsName
	_ = writePendingFork(run.RunID, *pf)

	header := fmt.Sprintf("🔓 **%s** — picked. _(%s used, %d left)_\n\n",
		chosen.Label, thievesToolsName, len(held)-1)
	return p.commitForkChoice(ctx, run, pf.Options[choice-1], header)
}

// lockRefusalFor phrases why a non-pickable lock stays shut, preferring the
// evaluator's own reason so the player sees the same wording the menu gave.
func lockRefusalFor(c pendingChoice) string {
	if c.Reason != "" {
		return c.Reason + "."
	}
	switch c.Lock {
	case string(LockKey):
		return "That door wants a key, and a key is a thing you find, not a thing you force."
	case string(LockLevelMin):
		return "That way is beyond you yet. Come back stronger."
	case string(LockRegionClear):
		return "Somewhere else has to fall first."
	}
	return "That one isn't going to open."
}
