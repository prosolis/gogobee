package plugin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gogobee/internal/db"
)

// E3 — Surfacing the buried social data. Three read-only registries that
// render data the game already stores: !town (civic pride + housing + pets),
// !graveyard (recent deaths), and !rivals board (room-wide duel standings).
//
// Leak-check (gogobee_engagement_plan.md §E3): the civic-pride board ranks
// tax_ledger.total_paid, which is dominated by gambling/shop/arena rake and
// carries ZERO Misty/Arina donation signal — those debits never touch
// tax_ledger (adventure_npcs.go routes them through euro.Debit with their own
// reason strings, counters live in player_meta.misty_*/arina_* columns). So no
// board here can let a player correlate donations with the hidden NPC buffs.

const (
	// townCivicBoardLimit caps the civic-pride board.
	townCivicBoardLimit = 10
	// townListCap caps the housing/pet showcase lists.
	townListCap = 15
	// graveyardLimit caps the graveyard.
	graveyardLimit = 12
	// rivalsBoardLimit caps the room-wide rivalry standings.
	rivalsBoardLimit = 10
)

// ── Civic pride ──────────────────────────────────────────────────────────────

type civicEntry struct {
	Name      string
	TotalPaid int64
}

// loadCivicPrideBoard returns the top contributors to the community pot by
// lifetime tax paid. Safe to rank — see the leak-check note above.
func loadCivicPrideBoard(limit int) ([]civicEntry, error) {
	rows, err := db.Get().Query(
		`SELECT COALESCE(NULLIF(c.display_name, ''), t.user_id), t.total_paid
		   FROM tax_ledger t
		   LEFT JOIN player_meta c ON c.user_id = t.user_id
		  WHERE t.total_paid > 0
		  ORDER BY t.total_paid DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []civicEntry
	for rows.Next() {
		var e civicEntry
		if err := rows.Scan(&e.Name, &e.TotalPaid); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Housing street ───────────────────────────────────────────────────────────

type housingEntry struct {
	Name string
	Tier int
}

// loadHousingStreet returns everyone who owns a house, best homes first.
func loadHousingStreet(limit int) ([]housingEntry, error) {
	rows, err := db.Get().Query(
		`SELECT COALESCE(NULLIF(display_name, ''), user_id), house_tier
		   FROM player_meta
		  WHERE house_tier > 0
		  ORDER BY house_tier DESC, display_name
		  LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []housingEntry
	for rows.Next() {
		var e housingEntry
		if err := rows.Scan(&e.Name, &e.Tier); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Pet showcase ─────────────────────────────────────────────────────────────

type petShowcaseEntry struct {
	Name      string
	Type      string
	Level     int
	ArmorName string
}

// loadPetShowcase returns every active pet — both slots — across all players,
// highest level first. The arrived/chased flags live in the *_flags_json
// columns, so they're decoded in Go rather than in SQL.
func loadPetShowcase(limit int) ([]petShowcaseEntry, error) {
	rows, err := db.Get().Query(
		`SELECT COALESCE(NULLIF(display_name, ''), user_id),
		        pet_name, pet_type, pet_level, pet_armor_tier, pet_flags_json,
		        pet2_name, pet2_type, pet2_level, pet2_armor_tier, pet2_flags_json
		   FROM player_meta
		  WHERE pet_type != '' OR pet2_type != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []petShowcaseEntry
	for rows.Next() {
		var (
			owner                         string
			petName, petType, flagsRaw    string
			level, armorTier              int
			pet2Name, pet2Type, flags2Raw string
			pet2Level, pet2Armor          int
		)
		if err := rows.Scan(&owner, &petName, &petType, &level, &armorTier, &flagsRaw,
			&pet2Name, &pet2Type, &pet2Level, &pet2Armor, &flags2Raw); err != nil {
			return nil, err
		}
		if e, ok := showcaseEntryFor(owner, petName, petType, level, armorTier, flagsRaw); ok {
			out = append(out, e)
		}
		if e, ok := showcaseEntryFor(owner, pet2Name, pet2Type, pet2Level, pet2Armor, flags2Raw); ok {
			out = append(out, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level > out[j].Level
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// showcaseEntryFor builds a showcase entry for one pet slot, returning ok=false
// if the slot is empty or its pet was chased away.
func showcaseEntryFor(owner, petName, petType string, level, armorTier int, flagsRaw string) (petShowcaseEntry, bool) {
	if petType == "" {
		return petShowcaseEntry{}, false
	}
	var flags petFlagsJSON
	if flagsRaw != "" {
		_ = json.Unmarshal([]byte(flagsRaw), &flags)
	}
	if !flags.Arrived || flags.ChasedAway {
		return petShowcaseEntry{}, false
	}
	return petShowcaseEntry{
		Name:      firstNonEmpty(petName, "an unnamed companion") + " (" + owner + ")",
		Type:      petType,
		Level:     level,
		ArmorName: petArmorName(petType, armorTier),
	}, true
}

// petArmorName maps a pet's armor tier to its barding name, or "" for none.
func petArmorName(petType string, tier int) string {
	if tier <= 0 {
		return ""
	}
	for _, a := range petArmorDefs(petType) {
		if a.Tier == tier {
			return a.Name
		}
	}
	return ""
}

// ── Graveyard ────────────────────────────────────────────────────────────────

type graveEntry struct {
	Name          string
	Source        string // "adventure" | "arena" | ""
	Location      string
	LastDeathDate string // "2006-01-02"
	StillDown     bool   // alive == 0 (inside the 6h respawn window)
}

// loadGraveyard returns the most recent deaths across all players. Deaths are
// day-granularity (last_death_date), so ordering falls back to dead_until for
// the still-in-the-ground; a revived player keeps their last_death_date but
// clears dead_until.
func loadGraveyard(limit int) ([]graveEntry, error) {
	rows, err := db.Get().Query(
		`SELECT COALESCE(NULLIF(display_name, ''), user_id),
		        alive, death_source, death_location, last_death_date, dead_until
		   FROM player_meta
		  WHERE last_death_date != ''
		  ORDER BY last_death_date DESC, dead_until DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []graveEntry
	for rows.Next() {
		var (
			e         graveEntry
			aliveInt  int
			deadUntil sql.NullTime
		)
		if err := rows.Scan(&e.Name, &aliveInt, &e.Source, &e.Location, &e.LastDeathDate, &deadUntil); err != nil {
			return nil, err
		}
		e.StillDown = aliveInt == 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Rivals board ─────────────────────────────────────────────────────────────

type rivalBoardEntry struct {
	Name       string
	Wins       int
	Losses     int
	LastDuelAt *time.Time
}

// loadRivalsBoard aggregates adventure_rival_records into room-wide standings.
// The table is a directed per-pair ledger written from both duellists' sides,
// so summing a user_id's rows gives that player's total record.
func loadRivalsBoard(limit int) ([]rivalBoardEntry, error) {
	rows, err := db.Get().Query(
		`SELECT COALESCE(NULLIF(c.display_name, ''), r.user_id),
		        SUM(r.wins), SUM(r.losses), MAX(r.last_duel_at)
		   FROM adventure_rival_records r
		   LEFT JOIN player_meta c ON c.user_id = r.user_id
		  GROUP BY r.user_id
		  ORDER BY SUM(r.wins) DESC, SUM(r.losses) ASC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []rivalBoardEntry
	for rows.Next() {
		var (
			e        rivalBoardEntry
			lastDuel sql.NullString
		)
		if err := rows.Scan(&e.Name, &e.Wins, &e.Losses, &lastDuel); err != nil {
			return nil, err
		}
		// MAX() over a DATETIME column loses SQLite's type affinity and comes
		// back as text, so parse it by hand rather than scanning a NullTime.
		if lastDuel.Valid {
			if t, ok := parseSQLiteTime(lastDuel.String); ok {
				e.LastDuelAt = &t
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Rendering (client-free, unit-testable) ───────────────────────────────────

func renderTownRegistry(civic []civicEntry, houses []housingEntry, pets []petShowcaseEntry) string {
	var sb strings.Builder
	sb.WriteString("🏛️ **The Town Registry**\n\n")

	sb.WriteString("**Civic Pride** — the guild's most generous coffers-fillers\n")
	if len(civic) == 0 {
		sb.WriteString("  _The collection plate is empty. Tragic._\n")
	} else {
		for i, e := range civic {
			sb.WriteString(fmt.Sprintf("  %2d. %-18s %s\n", i+1, truncName(e.Name, 18), fmtEuro(e.TotalPaid)))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("**Housing Street** — who's put down roots\n")
	if len(houses) == 0 {
		sb.WriteString("  _Not a single deed on file. A town of drifters._\n")
	} else {
		for _, e := range houses {
			tierName := "a house"
			if def := houseTierByTier(e.Tier); def != nil {
				tierName = def.Name
			}
			sb.WriteString(fmt.Sprintf("  %-18s  %s\n", truncName(e.Name, 18), tierName))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("**Pet Showcase** — the good companions\n")
	if len(pets) == 0 {
		sb.WriteString("  _No pets in town. Somebody adopt something._\n")
	} else {
		for _, e := range pets {
			line := fmt.Sprintf("  %-30s  L%d %s", truncName(e.Name, 30), e.Level, e.Type)
			if e.ArmorName != "" {
				line += " · " + e.ArmorName
			}
			sb.WriteString(line + "\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func renderGraveyard(deaths []graveEntry, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("⚰️ **St. Guildmore's Memorial Garden**\n")
	sb.WriteString("_The groundskeeper tips his hat. \"They gave their all. Well — they gave enough.\"_\n\n")

	if len(deaths) == 0 {
		sb.WriteString("_Nobody's died lately. The garden's quiet. Enjoy it while it lasts._")
		return sb.String()
	}

	for _, e := range deaths {
		place := e.Location
		if place == "" {
			place = "parts unknown"
		}
		verb := "fell"
		if e.Source == "arena" {
			verb = "fell in the arena"
		}
		when := humanizeDeathDate(e.LastDeathDate, now)
		line := fmt.Sprintf("  ⚰️ **%s** — %s at %s (%s)", truncName(e.Name, 24), verb, place, when)
		if e.StillDown {
			line += " · _still resting_"
		} else {
			line += " · _back on their feet_"
		}
		sb.WriteString(line + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func renderRivalsBoard(board []rivalBoardEntry, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("⚔️ **Rivalry Standings** — duels across the guild\n\n")

	if len(board) == 0 {
		sb.WriteString("_No duels on record. A suspiciously peaceful town._")
		return sb.String()
	}

	for i, e := range board {
		last := "—"
		if e.LastDuelAt != nil {
			last = humanizeDaysAgo(*e.LastDuelAt, now)
		}
		sb.WriteString(fmt.Sprintf("  %2d. %-18s %dW - %dL   last duel: %s\n",
			i+1, truncName(e.Name, 18), e.Wins, e.Losses, last))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// ── Small render helpers ─────────────────────────────────────────────────────

func truncName(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// parseSQLiteTime parses a SQLite CURRENT_TIMESTAMP string ("2006-01-02
// 15:04:05", UTC), tolerating an RFC3339 variant. Aggregates like MAX() strip
// the column's type affinity so the driver hands back text.
func parseSQLiteTime(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// humanizeDaysAgo renders a duel/event time as "today" / "1 day ago" / "N days
// ago", matching the inline style in handleRivalsCmd.
func humanizeDaysAgo(t, now time.Time) string {
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "1 day ago"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

// humanizeDeathDate renders a "2006-01-02" death date relative to now. Falls
// back to the raw string if it doesn't parse.
func humanizeDeathDate(dateStr string, now time.Time) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		if dateStr == "" {
			return "some time ago"
		}
		return dateStr
	}
	days := int(now.UTC().Truncate(24*time.Hour).Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleTownCmd(ctx MessageContext) error {
	civic, err := loadCivicPrideBoard(townCivicBoardLimit)
	if err != nil {
		return err
	}
	houses, err := loadHousingStreet(townListCap)
	if err != nil {
		return err
	}
	pets, err := loadPetShowcase(townListCap)
	if err != nil {
		return err
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, renderTownRegistry(civic, houses, pets))
}

func (p *AdventurePlugin) handleGraveyardCmd(ctx MessageContext) error {
	deaths, err := loadGraveyard(graveyardLimit)
	if err != nil {
		return err
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, renderGraveyard(deaths, time.Now().UTC()))
}

// handleRivalsTopCmd routes the top-level !rivals command: "!rivals board" is
// the room-wide standings; anything else defers to the per-user record view.
func (p *AdventurePlugin) handleRivalsTopCmd(ctx MessageContext, args string) error {
	if strings.EqualFold(strings.TrimSpace(args), "board") {
		board, err := loadRivalsBoard(rivalsBoardLimit)
		if err != nil {
			return err
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, renderRivalsBoard(board, time.Now().UTC()))
	}
	return p.handleRivalsCmd(ctx)
}
