package plugin

import (
	"fmt"
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
//   - Two Weeks   : day 15 (+500 XP, +1 primary stat — deferred to char hookup)
//
// Completion milestones fire from the boss-kill path (combat-link hook):
//   - Patient Zero : status=complete && max_threat_seen ≤ 50 (+10% of XPEarned)
//   - Survivalist  : status=complete && tier ≥ 3 && never forced-extracted (title flag)
//   - Long Game    : status=complete && tier == 5 (legendary item — deferred)
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
	award := func(key, title string, xp int, line, notes string) {
		if HasMilestone(e, key) {
			return
		}
		if _, err := awardMilestone(e, key); err != nil {
			return
		}
		if xp > 0 && p.xp != nil {
			p.xp.GrantXP(id.UserID(e.UserID), xp, "expedition milestone: "+title)
		}
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "milestone",
			"milestone awarded: "+title, line)
		out = append(out, milestoneAward{
			Key: key, Title: title, XP: xp, Flavor: line, Notes: notes,
		}.Render())
	}

	// First Night — survived day 1, now waking on day 2.
	if e.CurrentDay >= 2 {
		award(MilestoneKeyFirstNight, "First Night", 50,
			flavor.Pick(flavor.MilestoneFirstNight), "")
	}
	// Week One — survived day 7, now waking on day 8.
	if e.CurrentDay >= 8 {
		award(MilestoneKeyWeekOne, "Week One", 200,
			flavor.Pick(flavor.MilestoneWeekOne),
			"Thom Krooke discount flag set on next visit.")
	}
	// Two Weeks — survived day 14, now waking on day 15.
	if e.CurrentDay >= 15 {
		award(MilestoneKeyTwoWeeks, "Two Weeks", 500,
			flavor.Pick(flavor.MilestoneTwoWeeks),
			"Permanent +1 primary stat (deferred — applied at character hookup).")
	}
	return out
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
	award := func(key, title string, xp int, line, notes string) {
		if HasMilestone(e, key) {
			return
		}
		if _, err := awardMilestone(e, key); err != nil {
			return
		}
		if xp > 0 && p.xp != nil {
			p.xp.GrantXP(id.UserID(e.UserID), xp, "expedition milestone: "+title)
		}
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "milestone",
			"milestone awarded: "+title, line)
		out = append(out, milestoneAward{
			Key: key, Title: title, XP: xp, Flavor: line, Notes: notes,
		}.Render())
	}

	if maxThreatSeen(e) <= 50 {
		bonus := e.XPEarned / 10
		award(MilestoneKeyPatientZero, "Patient Zero", bonus,
			flavor.Pick(flavor.MilestonePatientZero),
			"+10% XP bonus on the run.")
	}
	if int(zone.Tier) >= 3 && !forcedExtractedEver {
		award(MilestoneKeySurvivalist, "Survivalist", 0,
			"Survivalist title earned — completed a Tier "+fmt.Sprint(int(zone.Tier))+
				" expedition without a single forced extraction.",
			"Title flag recorded; cosmetic item deferred to item-grant hookup.")
	}
	if int(zone.Tier) == 5 {
		award(MilestoneKeyLongGame, "The Long Game", 0,
			flavor.Pick(flavor.MilestoneTheLongGame),
			"Guaranteed legendary item deferred to loot-grant hookup.")
	}
	return out
}
