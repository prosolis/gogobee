package plugin

import (
	"fmt"
	"log/slog"
	"strings"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E6b — Expedition milestones (§13).
//
// Daily milestones fire from the morning briefing once their day-condition
// is met (e.CurrentDay reflects the new day after rollover):
//   - First Night : day 2  (+50 XP)
//   - Week One    : day 8  (+200 XP, Thom Krooke discount flag)
//   - Two Weeks   : day 15 (+500 XP, supply restock + consumable cache)
//
// Completion milestones fire from the boss-kill path (combat-link hook):
//   - Patient Zero : status=complete && max_threat_seen ≤ 50 (+10% of XPEarned)
//   - Survivalist  : status=complete && tier ≥ 3 && never forced-extracted (title)
//   - Long Game    : status=complete && tier == 5 (guaranteed legendary item)
//
// Two Weeks pays out in supplies rather than a stat bump: combat MaxHP is
// built from gear/arena/housing in combat_stats.go with no expedition in
// scope, and threading one through would leak an expedition-only buff into
// the sim's balance corpus. A cache self-expires when the run ends.
//
// Cartographer (search every room) is fully deferred — needs combat-link
// search-hook data not yet plumbed.
//
// Awarded keys persist as RegionState["milestones"] = []string. Max threat
// observed persists as RegionState["max_threat_seen"] = float64 (sqlite JSON).

const (
	MilestoneKeyFirstNight   = "first_night"
	MilestoneKeyWeekOne      = "week_one"
	MilestoneKeyTwoWeeks     = "two_weeks"
	MilestoneKeyPatientZero  = "patient_zero"
	MilestoneKeySurvivalist  = "survivalist"
	MilestoneKeyLongGame     = "long_game"
	MilestoneKeyCartographer = "cartographer"

	regionStateMilestonesKey = "milestones"
	regionStateMaxThreatKey  = "max_threat_seen"
)

// HasMilestone reports whether `key` has already been awarded for `e`.
func HasMilestone(e *Expedition, key string) bool {
	return regionListContains(regionListFromState(e, regionStateMilestonesKey), key)
}

// awardMilestone is idempotent — returns false if already awarded.
func awardMilestone(e *Expedition, key string) (bool, error) {
	return addRegionListEntry(e, regionStateMilestonesKey, key)
}

// recordMaxThreat updates RegionState["max_threat_seen"] if the current
// threat level is higher than the stored value. Persists.
func recordMaxThreat(e *Expedition) error {
	if e == nil {
		return nil
	}
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	prev, _ := e.RegionState[regionStateMaxThreatKey].(float64)
	cur := float64(e.ThreatLevel)
	if cur <= prev {
		return nil
	}
	e.RegionState[regionStateMaxThreatKey] = cur
	return persistRegionState(e)
}

// maxThreatSeen returns the highest threat ever observed during the
// expedition (post-rollover, daily samples).
func maxThreatSeen(e *Expedition) int {
	if e == nil || e.RegionState == nil {
		return 0
	}
	v, _ := e.RegionState[regionStateMaxThreatKey].(float64)
	return int(v)
}

// milestoneAward bundles what gets returned to the briefing/recap renderer
// so it can splice the line into its DM body.
type milestoneAward struct {
	Key    string
	Title  string
	XP     int
	Flavor string
	Notes  string // optional extra ("Thom Krooke discount flag set", etc.)
	Extra  string // grant narration (loot lines, cache contents)
}

func (m milestoneAward) Render() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🏆 **Milestone — %s** (+%d XP)\n", m.Title, m.XP))
	if m.Flavor != "" {
		b.WriteString(m.Flavor)
		b.WriteString("\n")
	}
	if m.Notes != "" {
		b.WriteString("_")
		b.WriteString(m.Notes)
		b.WriteString("_\n")
	}
	if m.Extra != "" {
		b.WriteString(m.Extra)
		b.WriteString("\n")
	}
	return b.String()
}

// checkDailyMilestones inspects the expedition after a day rollover and
// awards any newly-reached daily milestones. Awarded milestones are
// persisted, XP is granted via p.xp, and the rendered lines are returned
// for the briefing renderer to append.
func (p *AdventurePlugin) checkDailyMilestones(e *Expedition) []string {
	if e == nil {
		return nil
	}
	var out []string
	award := func(key, title string, xp int, line, notes string, grant func() string) {
		if HasMilestone(e, key) {
			return
		}
		if _, err := awardMilestone(e, key); err != nil {
			return
		}
		if xp > 0 && p.xp != nil {
			p.xp.GrantXP(id.UserID(e.UserID), xp, "expedition milestone: "+title)
		}
		var extra string
		if grant != nil {
			extra = grant()
		}
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "milestone",
			"milestone awarded: "+title, line)
		out = append(out, milestoneAward{
			Key: key, Title: title, XP: xp, Flavor: line, Notes: notes, Extra: extra,
		}.Render())
	}

	// First Night — survived day 1, now waking on day 2.
	if e.CurrentDay >= 2 {
		award(MilestoneKeyFirstNight, "First Night", 50,
			flavor.Pick(flavor.MilestoneFirstNight), "", nil)
	}
	// Week One — survived day 7, now waking on day 8.
	if e.CurrentDay >= 8 {
		award(MilestoneKeyWeekOne, "Week One", 200,
			flavor.Pick(flavor.MilestoneWeekOne),
			"Thom Krooke discount flag set on next visit.", nil)
	}
	// Two Weeks — survived day 14, now waking on day 15.
	if e.CurrentDay >= 15 {
		award(MilestoneKeyTwoWeeks, "Two Weeks", 500,
			flavor.Pick(flavor.MilestoneTwoWeeks), "",
			func() string { return p.grantTwoWeeksCache(e) })
	}
	return out
}

// twoWeeksRestockDays — days of rations the Two Weeks cache puts back in the
// pack. Never overfills past what the purchased packs can hold.
const twoWeeksRestockDays = 3

// twoWeeksCacheSize — consumables granted alongside the restock.
const twoWeeksCacheSize = 3

// grantTwoWeeksCache restocks supplies and drops a small consumable cache at
// the zone's tier. Returns the narration block for the briefing DM.
func (p *AdventurePlugin) grantTwoWeeksCache(e *Expedition) string {
	var lines []string

	if s := e.Supplies; s.DailyBurn > 0 {
		s.Current += twoWeeksRestockDays * s.DailyBurn
		if s.Max > 0 && s.Current > s.Max {
			s.Current = s.Max
		}
		if s.Current > e.Supplies.Current {
			if err := updateSupplies(e.ID, s); err != nil {
				slog.Error("milestone: two weeks restock failed",
					"expedition", e.ID, "err", err)
			} else {
				lines = append(lines, fmt.Sprintf(
					"📦 Supplies restocked — %.1f of %.1f.", s.Current, s.Max))
				e.Supplies = s
			}
		}
	}

	zone, _ := getZone(e.ZoneID)
	userID := id.UserID(e.UserID)
	counts := map[string]int{}
	var order []string
	for _, item := range consumableCache(int(zone.Tier), twoWeeksCacheSize) {
		if err := addAdvInventoryItem(userID, item); err != nil {
			slog.Error("milestone: two weeks cache grant failed",
				"user", userID, "item", item.Name, "err", err)
			continue
		}
		if counts[item.Name] == 0 {
			order = append(order, item.Name)
		}
		counts[item.Name]++
	}
	if len(order) > 0 {
		var parts []string
		for _, name := range order {
			if n := counts[name]; n > 1 {
				parts = append(parts, fmt.Sprintf("%s ×%d", name, n))
			} else {
				parts = append(parts, name)
			}
		}
		lines = append(lines, "🧪 "+strings.Join(parts, ", "))
	}
	return strings.Join(lines, "\n")
}

// AwardCompletionMilestones is called by the combat-link boss-kill path
// (deferred) once status flips to 'complete'. It checks Patient Zero,
// Survivalist, and Long Game and grants the relevant XP. Returns the
// rendered lines so the boss-kill DM can include them.
//
// `forcedExtractedEver` is whether any forced extraction occurred at any
// point (caller knows: status==abandoned in expedition history, or a
// forced-extract flag in RegionState the caller maintains).
func (p *AdventurePlugin) AwardCompletionMilestones(e *Expedition, forcedExtractedEver bool) []string {
	if e == nil || e.Status != ExpeditionStatusComplete {
		return nil
	}
	zone, _ := getZone(e.ZoneID)
	var out []string
	award := func(key, title string, xp int, line, notes string, grant func() string) {
		if HasMilestone(e, key) {
			return
		}
		if _, err := awardMilestone(e, key); err != nil {
			return
		}
		if xp > 0 && p.xp != nil {
			p.xp.GrantXP(id.UserID(e.UserID), xp, "expedition milestone: "+title)
		}
		var extra string
		if grant != nil {
			extra = grant()
		}
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "milestone",
			"milestone awarded: "+title, line)
		out = append(out, milestoneAward{
			Key: key, Title: title, XP: xp, Flavor: line, Notes: notes, Extra: extra,
		}.Render())
	}

	if maxThreatSeen(e) <= 50 {
		bonus := e.XPEarned / 10
		award(MilestoneKeyPatientZero, "Patient Zero", bonus,
			flavor.Pick(flavor.MilestonePatientZero),
			"+10% XP bonus on the run.", nil)
	}
	if int(zone.Tier) >= 3 && !forcedExtractedEver {
		line := flavor.Pick(flavor.MilestoneSurvivalist)
		if line == "" {
			line = "Survivalist title earned — completed a Tier " +
				fmt.Sprint(int(zone.Tier)) + " expedition without a single forced extraction."
		}
		award(MilestoneKeySurvivalist, "Survivalist", 0, line,
			"Title set — shows on your sheet.",
			func() string { return p.grantSurvivalistTitle(e, zone) })
	}
	if int(zone.Tier) == 5 {
		award(MilestoneKeyLongGame, "The Long Game", 0,
			flavor.Pick(flavor.MilestoneTheLongGame), "",
			func() string { return p.grantLongGameLegendary(e) })
	}
	return out
}

// survivalistTitle — the title written to player_meta.title on a clean T3+ clear.
const survivalistTitle = "Survivalist"

// grantSurvivalistTitle writes the title to the character and posts the
// games-room notice. Returns "" — the milestone's own Notes line already
// tells the player what happened.
func (p *AdventurePlugin) grantSurvivalistTitle(e *Expedition, zone ZoneDefinition) string {
	userID := id.UserID(e.UserID)
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		slog.Error("milestone: survivalist title load failed", "user", userID, "err", err)
		return ""
	}
	if char.Title != survivalistTitle {
		char.Title = survivalistTitle
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("milestone: survivalist title save failed", "user", userID, "err", err)
			return ""
		}
	}
	gr := gamesRoom()
	if gr == "" {
		return ""
	}
	displayName, _ := loadDisplayName(userID)
	p.SendMessage(id.RoomID(gr), fmt.Sprintf(
		"🎖️ **%s** walked out of %s on their own two feet — Tier %d, no extraction. **Survivalist.**",
		displayName, zone.Display, int(zone.Tier)))
	return ""
}

// grantLongGameLegendary hands out the guaranteed Legendary for a T5 clear.
func (p *AdventurePlugin) grantLongGameLegendary(e *Expedition) string {
	mi, ok := pickMagicItemForRarity(RarityLegendary, nil)
	if !ok {
		slog.Error("milestone: no legendary in registry", "expedition", e.ID)
		return ""
	}
	return p.dropMagicItemLoot(id.UserID(e.UserID), mi, LootTierLegendary)
}
