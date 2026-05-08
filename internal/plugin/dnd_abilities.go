package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 6 — active abilities via pre-arm model.
//
// The combat engine is one-shot, so player-activated abilities use a pre-arm
// pattern: !arm <ability> reserves it (consumes one resource immediately)
// and the next combat auto-fires it via existing CombatModifiers fields.
// The armed flag is cleared on combat completion regardless of outcome.
//
// Resources refresh on long rest (Phase 6 simplification — short-rest classes
// also use long-rest refresh until we split short/long resource pools).

// ── Ability definitions ──────────────────────────────────────────────────────

type DnDAbility struct {
	ID          string
	Name        string
	Class       DnDClass
	// Subclass: if non-empty, ability is only available when the player's
	// subclass matches. Phase 10 adds the first such ability (Berserker
	// rage). Used by parseAbility/arm gating and by classActiveAbilities
	// when listing for !arm / !abilities.
	Subclass    DnDSubclass
	Resource    string // "stamina", "spell_slot", "favor", etc.
	Description string
	// Effect — applied to mods at combat start when armed
	Apply func(c *DnDCharacter, mods *CombatModifiers)
}

var dndActiveAbilities = map[string]DnDAbility{
	"second_wind": {
		ID:          "second_wind",
		Name:        "Second Wind",
		Class:       ClassFighter,
		Resource:    "stamina",
		Description: "Restore HP at low health: 1d10 + level (consumes 1 stamina).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// HealItem fires once when player drops below 50% HP.
			// Avg of 1d10 + level: 5.5 + level.
			mods.HealItem += 5 + c.Level
		},
	},
	"magic_missile": {
		ID:          "magic_missile",
		Name:        "Magic Missile",
		Class:       ClassMage,
		Resource:    "spell_slot",
		Description: "Three darts strike unerringly at the start of combat: 3 × (1d4+1) force damage.",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// 3 darts × avg 3.5 = ~10. Auto-hit pre-combat damage.
			mods.FlatDmgStart += 9 + abilityModifier(c.INT)
		},
	},
	"healing_word": {
		ID:          "healing_word",
		Name:        "Healing Word",
		Class:       ClassCleric,
		Resource:    "favor",
		Description: "Restore 1d4 + WIS HP at low health (consumes 1 divine favor).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.HealItem += 3 + abilityModifier(c.WIS)
		},
	},
}

func parseAbility(s string) (DnDAbility, bool) {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "_")))
	s = strings.ReplaceAll(s, "-", "_")
	if a, ok := dndActiveAbilities[s]; ok {
		return a, true
	}
	for _, a := range dndActiveAbilities {
		if strings.EqualFold(a.Name, s) {
			return a, true
		}
	}
	return DnDAbility{}, false
}

// classActiveAbilities returns the active abilities a class can know,
// excluding subclass-gated entries (those are listed by characterActiveAbilities).
func classActiveAbilities(class DnDClass) []DnDAbility {
	var out []DnDAbility
	for _, a := range dndActiveAbilities {
		if a.Class == class && a.Subclass == "" {
			out = append(out, a)
		}
	}
	return out
}

// characterActiveAbilities returns abilities available to a given character,
// including their subclass-gated abilities. Used for !arm listing so the
// Berserker sees `rage` once they've chosen the subclass.
func characterActiveAbilities(c *DnDCharacter) []DnDAbility {
	var out []DnDAbility
	for _, a := range dndActiveAbilities {
		if a.Class != c.Class {
			continue
		}
		if a.Subclass != "" && a.Subclass != c.Subclass {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ── Resource pool ────────────────────────────────────────────────────────────

// classResourceMax returns (resource_type, max_value) for a class.
// Phase 6: each class has one active resource pool.
func classResourceMax(class DnDClass) (string, int) {
	switch class {
	case ClassFighter:
		return "stamina", 3
	case ClassRogue:
		return "focus", 2
	case ClassMage:
		return "spell_slot", 1
	case ClassCleric:
		return "favor", 3
	case ClassRanger:
		return "focus", 2
	}
	return "", 0
}

// initResources writes a fresh resource row for the class. Idempotent —
// won't overwrite an existing pool. Called on !setup confirm and on
// auto-migration.
func initResources(userID id.UserID, class DnDClass) error {
	resType, max := classResourceMax(class)
	if resType == "" {
		return nil
	}
	_, err := db.Get().Exec(`
		INSERT OR IGNORE INTO dnd_resources (user_id, resource_type, current_value, max_value)
		VALUES (?, ?, ?, ?)`,
		string(userID), resType, max, max)
	return err
}

// getResource returns (current, max). Returns (0, 0, true) if no row exists.
func getResource(userID id.UserID, resType string) (int, int, error) {
	var cur, max int
	err := db.Get().QueryRow(
		`SELECT current_value, max_value FROM dnd_resources
		 WHERE user_id = ? AND resource_type = ?`,
		string(userID), resType,
	).Scan(&cur, &max)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return cur, max, err
}

// spendResource decrements current_value if at least amount is available.
// Returns true on success.
func spendResource(userID id.UserID, resType string, amount int) (bool, error) {
	res, err := db.Get().Exec(`
		UPDATE dnd_resources
		   SET current_value = current_value - ?,
		       last_reset_at = last_reset_at  -- no-op, keep timestamp
		 WHERE user_id = ? AND resource_type = ?
		   AND current_value >= ?`,
		amount, string(userID), resType, amount)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// refreshAllResources sets current_value = max_value for every resource of a user.
// Called on long rest.
func refreshAllResources(userID id.UserID) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_resources SET current_value = max_value, last_reset_at = CURRENT_TIMESTAMP
		WHERE user_id = ?`,
		string(userID))
	return err
}

// ── !arm command ─────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDArmCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	args = strings.TrimSpace(args)
	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character.")
	}

	if args == "" || strings.ToLower(args) == "list" {
		return p.SendDM(ctx.Sender, renderArmList(c))
	}
	if strings.ToLower(args) == "clear" {
		c.ArmedAbility = ""
		_ = SaveDnDCharacter(c)
		return p.SendDM(ctx.Sender, "Disarmed. Resource is not refunded.")
	}

	ab, ok := parseAbility(args)
	if !ok {
		return p.SendDM(ctx.Sender, "Unknown ability. Run `!arm` to see your options.")
	}
	if ab.Class != c.Class {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s is a %s ability — your class is %s.", ab.Name, titleClass(ab.Class), titleClass(c.Class)))
	}
	if ab.Subclass != "" && ab.Subclass != c.Subclass {
		needed, _ := subclassInfo(ab.Subclass)
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s is a %s subclass ability — choose that subclass via `!subclass`.",
			ab.Name, needed.Display))
	}

	if c.ArmedAbility != "" {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You already have **%s** armed. Run `!arm clear` to disarm first.",
			displayAbility(c.ArmedAbility)))
	}

	cur, _, err := getResource(ctx.Sender, ab.Resource)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load resources.")
	}
	if cur < 1 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"No %s remaining. Take a long rest to refresh.", ab.Resource))
	}

	// Audit fix C: save the armed-ability flag FIRST. If save fails, no
	// resource was spent. If save succeeds and spend fails (rare — single
	// writer SQLite makes it nearly impossible after a successful save in
	// the same connection), revert the armed flag.
	c.ArmedAbility = ab.ID
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save armed state: "+err.Error())
	}

	ok, err = spendResource(ctx.Sender, ab.Resource, 1)
	if err != nil || !ok {
		// Roll back the armed flag.
		c.ArmedAbility = ""
		if rerr := SaveDnDCharacter(c); rerr != nil {
			slog.Error("dnd: failed to revert armed flag after spend failure",
				"user", ctx.Sender, "err", rerr)
		}
		return p.SendDM(ctx.Sender, "Couldn't spend resource — armed state reverted.")
	}

	curAfter, max, _ := getResource(ctx.Sender, ab.Resource)
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"⚡ **%s** armed for next combat. (%s: %d/%d remaining)",
		ab.Name, ab.Resource, curAfter, max))
}

func renderArmList(c *DnDCharacter) string {
	abilities := characterActiveAbilities(c)
	var b strings.Builder
	b.WriteString("**Active Abilities** (pre-arm via `!arm <name>`)\n\n")

	if len(abilities) == 0 {
		b.WriteString("_No active abilities for your class yet._\n")
	} else {
		resType, _ := classResourceMax(c.Class)
		cur, max, _ := getResource(c.UserID, resType)
		b.WriteString(fmt.Sprintf("Resource: **%s** %d/%d\n\n", resType, cur, max))
		for _, a := range abilities {
			b.WriteString(fmt.Sprintf("  • **%s** (1 %s) — %s\n", a.Name, a.Resource, a.Description))
		}
	}
	if c.ArmedAbility != "" {
		b.WriteString(fmt.Sprintf("\n_Currently armed: **%s** (will fire on next combat)_\n", displayAbility(c.ArmedAbility)))
	}
	b.WriteString("\nResources refresh on long rest.")
	return b.String()
}

func displayAbility(id string) string {
	if a, ok := dndActiveAbilities[id]; ok {
		return a.Name
	}
	return id
}

// ── Combat hook ──────────────────────────────────────────────────────────────

// applyArmedAbility checks for a pre-armed ability on the character and
// applies its effect to the player's CombatModifiers, then clears the armed
// flag. Called from combat_bridge.go before SimulateCombat.
func applyArmedAbility(c *DnDCharacter, mods *CombatModifiers) (string, bool) {
	if c == nil || c.ArmedAbility == "" {
		return "", false
	}
	ab, ok := dndActiveAbilities[c.ArmedAbility]
	if !ok {
		c.ArmedAbility = ""
		_ = SaveDnDCharacter(c)
		return "", false
	}
	ab.Apply(c, mods)
	firedName := ab.Name
	c.ArmedAbility = ""
	_ = SaveDnDCharacter(c)
	return firedName, true
}
