package plugin

// Mischief Makers (M1) — paid monster hits on live expeditions.
//
// A buyer spends euros to send a monster at an adventurer who is out in a
// dungeon right now. The games room hears the contract is out; a window later
// the monster is delivered mid-run. Surviving it pays the target and unseals
// the buyer in Pete's bulletin. See gogobee_mischief_plan.md.
//
// Three properties the design leans on, all of them enforced here rather than
// trusted:
//
//   - The monster is priced against the TARGET, not the dungeon they happen to
//     be standing in. It comes from the zone pool of the target's own level
//     bracket (mischiefBracketZones), which is the corpus the difficulty pass is
//     calibrated on. The arena ladder is deliberately NOT the source: its
//     BaseLethality is a legacy flat death chance and arena T5 one-shots a level
//     20, which would collapse every tier into an execution.
//
//   - The payout basis is the BASE fee, never what the buyer actually paid. The
//     +25% sign surcharge is a pure sink. With the per-tier percentages below
//     capped at 75%, a survival purse is always strictly less than the buyer's
//     outlay — so two players colluding to move money this way lose to
//     `!baltransfer`, which is free. That cap is the whole anti-collusion story;
//     no danger multiplier is needed.
//
//   - Mischief maims, it does not murder. The delivery close-out floors every
//     seat at 1 HP and force-extracts the expedition. A purchased attack that
//     lands while the victim is asleep must not be able to delete their
//     character — see mischiefResolveDowned.

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

const (
	// mischiefWindow — how long the games room has to react before the monster
	// is delivered. Long enough for a bless/escalate scramble (M2), short
	// enough that a contract resolves inside one sitting.
	mischiefWindow = 60 * time.Minute

	// mischiefMinLevel — a level 1-2 adventurer is nobody's idea of a fair
	// target, and the bracket-1 pool is the only thing we could send anyway.
	mischiefMinLevel = 3

	// mischiefTargetCooldown — a target gets a breather after one resolves, so
	// a buyer with money can't chain-maim someone out of the game.
	mischiefTargetCooldown = 24 * time.Hour

	// mischiefBuyerDailyCap — contracts one buyer may place per rolling day.
	mischiefBuyerDailyCap = 2

	// mischiefBossPerTargetWindow — boss tier is "end this expedition" against a
	// caster (0-14% survival in the M0 sweep). One per target per week, on top of
	// the cooldown and the buyer cap.
	mischiefBossPerTargetWindow = 7 * 24 * time.Hour

	// mischiefSignSurcharge — the buyer pays this much on top to put their name
	// on the contract from the start. Pure sink; see the payout-basis note above.
	mischiefSignSurcharge = 0.25

	// Fizzle: the expedition ended before the monster could find them.
	mischiefFizzleRefund = 0.90

	// mischiefBlessingFee — what the room pays to put a ward on the target while
	// the contract is open. Cheap on purpose: helping has to be an impulse, or
	// nobody bothers and the window is just a countdown.
	mischiefBlessingFee = 25

	// mischiefBlessingCap — how many blessings one contract can carry. Three at
	// +10% MaxHP each is a third of a health bar: enough to swing an elite
	// coin-flip, not enough to make a boss survivable on its own.
	mischiefBlessingCap = 3

	// mischiefBlessingPct — temp HP per blessing, as a fraction of the target's
	// MaxHP. Same cushion mechanism as a well-rested long rest (applyDnDHPScaling),
	// and like it, evaporates when the fight's HP is persisted back.
	mischiefBlessingPct = 0.10
)

// Contract statuses. `delivering` is the claimed-but-unresolved state a CAS
// moves through, so a ticker double-fire finds nothing left to claim.
const (
	mischiefStatusOpen       = "open"
	mischiefStatusDelivering = "delivering"
	mischiefStatusDelivered  = "delivered"
	mischiefStatusFizzled    = "fizzled"
)

const (
	mischiefOutcomeSurvived = "survived"
	mischiefOutcomeDowned   = "downed"
)

// ── Tiers ────────────────────────────────────────────────────────────────────

// mischiefTier is one rung of the grunt→boss ladder. Fee and PayoutPct come
// from the M0 pricing sweep (n=2000/cell against the real combat engine) read
// against the live prod economy — see the tier table in the plan doc.
type mischiefTier struct {
	Key       string
	Display   string
	Fee       int     // base fee; also the payout basis
	PayoutPct float64 // survival purse as a fraction of Fee. Capped at 0.75.
	Blurb     string
}

var mischiefTiers = []mischiefTier{
	{Key: "grunt", Display: "Grunt", Fee: 40, PayoutPct: 0.40,
		Blurb: "One of the local nasties. Mostly theatre — it'll cost them blood, not the run."},
	{Key: "mob", Display: "Mob", Fee: 100, PayoutPct: 0.50,
		Blurb: "Three of them, back to back. Attrition, and a real dent in the supplies."},
	{Key: "elite", Display: "Elite", Fee: 350, PayoutPct: 0.65,
		Blurb: "Something that hunts on purpose, and it opens with a free swing. A coin-flip at their level."},
	{Key: "boss", Display: "Boss", Fee: 1200, PayoutPct: 0.75,
		Blurb: "The worst thing in their bracket, with the drop on them. This ends expeditions."},
}

func mischiefTierByKey(key string) (mischiefTier, bool) {
	for _, t := range mischiefTiers {
		if t.Key == strings.ToLower(strings.TrimSpace(key)) {
			return t, true
		}
	}
	return mischiefTier{}, false
}

// mischiefNextTier is the rung an escalation buys. Boss is the top: there is
// nothing worse in the bracket to send.
func mischiefNextTier(key string) (mischiefTier, bool) {
	for i, t := range mischiefTiers {
		if t.Key == key && i+1 < len(mischiefTiers) {
			return mischiefTiers[i+1], true
		}
	}
	return mischiefTier{}, false
}

// mischiefBlessingTempHP is the cushion a contract's blessings are worth to a
// target of this MaxHP. At least 1 per blessing, so a low-HP character still
// feels the ward they were bought.
func mischiefBlessingTempHP(hpMax, blessings int) int {
	if blessings <= 0 || hpMax <= 0 {
		return 0
	}
	if blessings > mischiefBlessingCap {
		blessings = mischiefBlessingCap
	}
	per := int(float64(hpMax) * mischiefBlessingPct)
	if per < 1 {
		per = 1
	}
	return per * blessings
}

// mischiefSignedFee is what a buyer pays to put their name on the contract up
// front. The extra is a sink — mischiefPurse never sees it.
func mischiefSignedFee(fee int) int {
	return int(math.Round(float64(fee) * (1 + mischiefSignSurcharge)))
}

// mischiefPurse is the target's cut for surviving: a percentage of the BASE fee
// (which escalations raise, and the sign surcharge does not). Hard-capped at 75%
// so no combination of tier + escalation can pay out more than three-quarters of
// what was spent.
func mischiefPurse(fee int, pct float64) int {
	if pct > 0.75 {
		pct = 0.75
	}
	return int(math.Round(float64(fee) * pct))
}

// ── Monster selection ────────────────────────────────────────────────────────

// mischiefBracketZones — the zone whose enemy pool a target of each level
// bracket draws its attacker from. The target's *current* dungeon is not used:
// a level 20 slumming in a tier 1 zone should still be sent something that can
// find them, and a level 4 who wandered into a tier 4 zone should not be sent
// its boss.
var mischiefBracketZones = map[int]ZoneID{
	1: ZoneGoblinWarrens,
	2: ZoneSunkenTemple,
	3: ZoneManorBlackspire,
	4: ZoneUnderdark,
	5: ZoneDragonsLair,
}

func mischiefBracketForLevel(level int) int {
	switch {
	case level <= 3:
		return 1
	case level <= 7:
		return 2
	case level <= 12:
		return 3
	case level <= 17:
		return 4
	}
	return 5
}

// mischiefBracketZone resolves the zone a target's attacker is drawn from.
func mischiefBracketZone(level int) ZoneDefinition {
	zone, _ := getZone(mischiefBracketZones[mischiefBracketForLevel(level)])
	return zone
}

func mischiefPickEnemy(zone ZoneDefinition, elite bool, rng *rand.Rand) DnDMonsterTemplate {
	var pool []ZoneEnemy
	for _, e := range zone.Enemies {
		if e.IsElite == elite {
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 {
		pool = zone.Enemies
	}
	if len(pool) == 0 {
		return DnDMonsterTemplate{}
	}
	return dndBestiary[pool[rng.IntN(len(pool))].BestiaryID]
}

// mischiefMonsters returns the fight chain a tier sends, and whether the first
// monster gets the ambush (the attacker's privilege — it was lying in wait).
// This IS the shape the M0 sweep priced; changing it invalidates the fee table.
//
//	grunt = one non-elite zone enemy, no ambush
//	mob   = three non-elite zone enemies back to back (HP carries), no ambush
//	elite = one elite zone enemy, ambush
//	boss  = the zone boss, ambush
func mischiefMonsters(tierKey string, zone ZoneDefinition, rng *rand.Rand) (chain []DnDMonsterTemplate, ambush bool) {
	switch tierKey {
	case "grunt":
		return []DnDMonsterTemplate{mischiefPickEnemy(zone, false, rng)}, false
	case "mob":
		return []DnDMonsterTemplate{
			mischiefPickEnemy(zone, false, rng),
			mischiefPickEnemy(zone, false, rng),
			mischiefPickEnemy(zone, false, rng),
		}, false
	case "elite":
		return []DnDMonsterTemplate{mischiefPickEnemy(zone, true, rng)}, true
	case "boss":
		return []DnDMonsterTemplate{dndBestiary[zone.Boss.BestiaryID]}, true
	}
	return nil, false
}

// ── Model + persistence ──────────────────────────────────────────────────────

type mischiefContract struct {
	ID              string
	BuyerID         id.UserID
	TargetID        id.UserID
	Tier            string
	Fee             int // payout basis (base fee + escalations)
	Paid            int // what the buyer actually parted with
	Status          string
	Signed          bool
	EscalationCount int
	EscalatedBy     id.UserID // "" unless somebody paid to bump the tier
	BlessingCount   int
	Source          string
	Outcome         string
	CreatedAt       time.Time
	WindowEndsAt    time.Time
}

const mischiefCols = `contract_id, buyer_id, target_id, tier, fee, paid, status, signed,
	escalation_count, COALESCE(escalated_by, ''), blessing_count, source, COALESCE(outcome, ''),
	created_at, window_ends_at`

func scanMischief(row interface{ Scan(...any) error }) (*mischiefContract, error) {
	c := &mischiefContract{}
	err := row.Scan(&c.ID, &c.BuyerID, &c.TargetID, &c.Tier, &c.Fee, &c.Paid, &c.Status,
		&c.Signed, &c.EscalationCount, &c.EscalatedBy, &c.BlessingCount, &c.Source, &c.Outcome,
		&c.CreatedAt, &c.WindowEndsAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// insertMischiefContract writes the contract, and FAILS if one is already live
// against this target (idx_mischief_one_live_per_target). That error is the
// serialization point for two buyers racing at the same victim — the caller must
// refund on it, not ignore it.
//
// Every timestamp is bound as a Go time.Time rather than left to
// CURRENT_TIMESTAMP: created_at and window_ends_at are both compared against
// bound Go times later (the daily cap, the due sweep), and mixing SQLite's own
// text stamp with the driver's encoding is how you get a comparison that is
// lexicographic-by-accident.
func insertMischiefContract(c *mischiefContract) error {
	_, err := db.Get().Exec(
		`INSERT INTO mischief_contracts
		 (contract_id, buyer_id, target_id, tier, fee, paid, status, signed, source,
		  created_at, window_ends_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, string(c.BuyerID), string(c.TargetID), c.Tier, c.Fee, c.Paid,
		c.Status, c.Signed, c.Source, c.CreatedAt, c.WindowEndsAt)
	return err
}

// blessMischiefContract adds one ward to an open contract and reports whether it
// took. The cap and the still-open check are in the WHERE clause, not in the
// caller: the window is a scramble, and a room that piles four blessings in the
// same second must still land exactly three. A caller that loses this race has
// already been debited and must refund.
func blessMischiefContract(contractID string) bool {
	res := db.ExecResult("mischief: bless contract",
		`UPDATE mischief_contracts SET blessing_count = blessing_count + 1
		  WHERE contract_id = ? AND status = ? AND blessing_count < ?`,
		contractID, mischiefStatusOpen, mischiefBlessingCap)
	if res == nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// escalateMischiefContract bumps an open contract one rung: the tier, the payout
// basis (fee) and the buyer-side outlay (paid) all move together, so the purse
// stays a percentage of what was actually spent and the 75% cap survives an
// escalation untouched.
//
// The escalation_count = 0 and tier = <from> guards are what make it one step,
// once, even if two people hit `!mischief escalate` on the same contract at the
// same moment. The loser is refunded.
func escalateMischiefContract(contractID, fromTier string, next mischiefTier, delta int, by id.UserID) bool {
	res := db.ExecResult("mischief: escalate contract",
		`UPDATE mischief_contracts
		    SET tier = ?, fee = ?, paid = paid + ?, escalation_count = 1, escalated_by = ?
		  WHERE contract_id = ? AND status = ? AND tier = ? AND escalation_count = 0`,
		next.Key, next.Fee, delta, string(by), contractID, mischiefStatusOpen, fromTier)
	if res == nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// claimMischiefForDelivery moves open → delivering and reports whether THIS
// caller won the race. Exactly one delivery per contract, whatever the ticker
// does across a restart.
func claimMischiefForDelivery(contractID string) bool {
	res := db.ExecResult("mischief: claim delivery",
		`UPDATE mischief_contracts SET status = ?
		 WHERE contract_id = ? AND status = ?`,
		mischiefStatusDelivering, contractID, mischiefStatusOpen)
	if res == nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// resolveMischiefContract stamps the terminal state. Called only by the
// delivery path, which already holds the claim.
func resolveMischiefContract(contractID, status, outcome string) {
	db.Exec("mischief: resolve contract",
		`UPDATE mischief_contracts
		    SET status = ?, outcome = ?, resolved_at = ?
		  WHERE contract_id = ?`,
		status, outcome, time.Now().UTC(), contractID)
}

// fizzleMischiefContract claims an open contract straight to fizzled — the
// target's expedition ended before the monster found them. Same CAS discipline
// as delivery, so a fizzle and a delivery can never both fire.
func fizzleMischiefContract(contractID string) bool {
	return casMischiefStatus("mischief: fizzle contract", contractID,
		mischiefStatusOpen, mischiefStatusFizzled, mischiefStatusFizzled)
}

// abandonStaleMischief releases a stuck delivery. CAS off `delivering`, so a
// delivery that is merely slow (a long fight) cannot be swept out from under
// itself and double-refunded.
func abandonStaleMischief(contractID string) bool {
	return casMischiefStatus("mischief: abandon stale delivery", contractID,
		mischiefStatusDelivering, mischiefStatusFizzled, mischiefStatusFizzled)
}

// casMischiefStatus is the one status transition every terminal path takes: move
// from → to only if the row is still in `from`, and report whether THIS caller
// won. Every transition is conditional, which is what makes a ticker double-fire
// or a restart mid-delivery unable to pay (or refund) twice.
func casMischiefStatus(label, contractID, from, to, outcome string) bool {
	res := db.ExecResult(label,
		`UPDATE mischief_contracts
		    SET status = ?, outcome = ?, resolved_at = ?
		  WHERE contract_id = ? AND status = ?`,
		to, outcome, time.Now().UTC(), contractID, from)
	if res == nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// loadStaleMischiefDeliveries returns contracts stuck in `delivering` — claimed
// by a delivery that then crashed, or was killed mid-fight by a restart. Without
// this sweep the row lives forever: the target can never be targeted again (the
// live-contract check counts `delivering`) and the buyer's money is simply gone.
func loadStaleMischiefDeliveries(before time.Time) []mischiefContract {
	return loadMischiefByStatus(mischiefStatusDelivering, before)
}

// loadMischiefDue returns open contracts whose window has closed.
func loadMischiefDue(now time.Time) []mischiefContract {
	return loadMischiefByStatus(mischiefStatusOpen, now)
}

func loadMischiefByStatus(status string, dueBy time.Time) []mischiefContract {
	rows, err := db.Get().Query(
		`SELECT `+mischiefCols+` FROM mischief_contracts
		  WHERE status = ? AND window_ends_at <= ?`,
		status, dueBy)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []mischiefContract
	for rows.Next() {
		if c, err := scanMischief(rows); err == nil {
			out = append(out, *c)
		}
	}
	return out
}

// liveMischiefForTarget returns the contract already in flight against a target,
// or nil. One at a time: two monsters converging on the same person turns a
// mischief into a mugging.
func liveMischiefForTarget(targetID id.UserID) *mischiefContract {
	row := db.Get().QueryRow(
		`SELECT `+mischiefCols+` FROM mischief_contracts
		  WHERE target_id = ? AND status IN (?, ?)
		  ORDER BY created_at DESC LIMIT 1`,
		string(targetID), mischiefStatusOpen, mischiefStatusDelivering)
	c, err := scanMischief(row)
	if err != nil {
		return nil
	}
	return c
}

// mischiefTargetCoolingDown reports whether the target survived (or was floored
// by) a delivered contract too recently to be sent another.
//
// Only a DELIVERED contract cools a target down. A fizzle never reached them,
// and a crash-swept stranding never happened at all — counting either would sell
// cheap immunity: a target whose friend places a grunt and who then extracts pays
// 10% of a grunt fee for a full day of being untargetable.
func mischiefTargetCoolingDown(targetID id.UserID, now time.Time) (bool, time.Duration) {
	var resolved time.Time
	err := db.Get().QueryRow(
		`SELECT resolved_at FROM mischief_contracts
		  WHERE target_id = ? AND status = ? AND resolved_at IS NOT NULL
		  ORDER BY resolved_at DESC LIMIT 1`,
		string(targetID), mischiefStatusDelivered).Scan(&resolved)
	if err != nil {
		return false, 0 // no row (or unreadable stamp) == not cooling down
	}
	if wait := mischiefTargetCooldown - now.Sub(resolved.UTC()); wait > 0 {
		return true, wait
	}
	return false, 0
}

// mischiefBuyerCountSince is how many contracts this buyer has placed in the
// window — the anti-spam cap. Fizzled ones count: the limit is on how often you
// may point a monster at someone, not on how often it connects.
func mischiefBuyerCountSince(buyerID id.UserID, since time.Time) int {
	var n int
	_ = db.Get().QueryRow(
		`SELECT COUNT(*) FROM mischief_contracts WHERE buyer_id = ? AND created_at > ?`,
		string(buyerID), since).Scan(&n)
	return n
}

// mischiefBossCountSince counts boss-tier contracts aimed at a target inside the
// window, from anyone. Boss is the "end their expedition" button; one a week.
//
// Fizzled ones do NOT count, and that asymmetry with the buyer cap is deliberate:
// the buyer cap limits how often you may POINT a monster at someone, but this cap
// protects the target from being boss'd, and a boss that never found them is not
// something they need protecting from. Counting it would let a friendly buyer burn
// the target's weekly boss slot for the price of a fizzle rake.
func mischiefBossCountSince(targetID id.UserID, since time.Time) int {
	var n int
	_ = db.Get().QueryRow(
		`SELECT COUNT(*) FROM mischief_contracts
		  WHERE target_id = ? AND tier = 'boss' AND created_at > ? AND status != ?`,
		string(targetID), since, mischiefStatusFizzled).Scan(&n)
	return n
}

// ── Eligibility ──────────────────────────────────────────────────────────────

// mischiefTargetable reports why a target can't be sent a monster, or "" if they
// can. The active expedition is the whole premise: the monster has to have
// somewhere to walk in from.
func (p *AdventurePlugin) mischiefTargetable(targetID id.UserID, tier mischiefTier, now time.Time) string {
	dndChar, err := LoadDnDCharacter(targetID)
	if err != nil || dndChar == nil {
		return fmt.Sprintf("%s has no adventurer.", p.DisplayName(targetID))
	}
	// A dead character can still own an 'active' expedition row — that drift is
	// exactly what the ambient ticker keeps a run-liveness net for. Don't sell a
	// hit on a corpse.
	if advChar, err := loadAdvCharacter(targetID); err != nil || advChar == nil || !advChar.Alive {
		return fmt.Sprintf("%s is in no state to be attacked. Leave them be.", p.DisplayName(targetID))
	}
	if dndChar.Level < mischiefMinLevel {
		return fmt.Sprintf("%s is only level %d — nobody's putting a price on a rookie's head (level %d+).",
			p.DisplayName(targetID), dndChar.Level, mischiefMinLevel)
	}
	exp, err := getActiveExpedition(targetID)
	if err != nil || exp == nil {
		return fmt.Sprintf("%s isn't out on an expedition. Monsters don't do house calls.",
			p.DisplayName(targetID))
	}
	if c := liveMischiefForTarget(targetID); c != nil {
		return fmt.Sprintf("Somebody already has coin on %s. Wait for it to land.", p.DisplayName(targetID))
	}
	if cooling, wait := mischiefTargetCoolingDown(targetID, now); cooling {
		return fmt.Sprintf("%s has only just shaken one off. Try again %s.",
			p.DisplayName(targetID), formatDuration(wait))
	}
	if tier.Key == "boss" && mischiefBossCountSince(targetID, now.Add(-mischiefBossPerTargetWindow)) > 0 {
		return fmt.Sprintf("%s has already had a boss sent after them this week. Once is plenty.",
			p.DisplayName(targetID))
	}
	return ""
}

// ── Command surface ──────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleMischiefCmd(ctx MessageContext, args string) error {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, p.mischiefBoard())
	}
	switch strings.ToLower(fields[0]) {
	case "board", "tiers", "help":
		return p.SendReply(ctx.RoomID, ctx.EventID, p.mischiefBoard())
	case "status":
		return p.mischiefStatusCmd(ctx)
	case "send":
		return p.mischiefSendCmd(ctx, fields[1:])
	case "bless":
		return p.mischiefBlessCmd(ctx, fields[1:])
	case "escalate":
		return p.mischiefEscalateCmd(ctx, fields[1:])
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Unknown option. `!mischief` for the board · `!mischief send <tier> @user` · "+
				"`!mischief bless @user` · `!mischief escalate @user` · `!mischief status`")
	}
}

func (p *AdventurePlugin) mischiefBoard() string {
	var b strings.Builder
	b.WriteString("😈 **Mischief Makers** — put coin on an adventurer's head while they're out in a dungeon.\n")
	b.WriteString("The monster comes from their own bracket, so it's always a fight worth watching. ")
	b.WriteString("If they survive it, they keep a cut of your money — and the town finds out it was you.\n\n")
	for _, t := range mischiefTiers {
		b.WriteString(fmt.Sprintf("· **%s** — %s (signed: %s) — survivor keeps %s\n   _%s_\n",
			t.Display, fmtEuro(t.Fee), fmtEuro(mischiefSignedFee(t.Fee)),
			fmtEuro(mischiefPurse(t.Fee, t.PayoutPct)), t.Blurb))
	}
	b.WriteString("\n`!mischief send <tier> @user` — anonymous · add `signed` to put your name on it up front.\n")
	b.WriteString(fmt.Sprintf(
		"While a contract is open, the rest of us get a say:\n"+
			"· `!mischief bless @user` — %s, up to %d — a ward for the fight. Help them.\n"+
			"· `!mischief escalate @user` — pay the difference, send something worse. \"Help\" them. "+
			"It raises their purse too, if they live.\n",
		fmtEuro(mischiefBlessingFee), mischiefBlessingCap))
	b.WriteString(fmt.Sprintf("_They have to be out on an expedition, level %d+. It lands about %s later._",
		mischiefMinLevel, formatDuration(mischiefWindow)))
	return b.String()
}

func (p *AdventurePlugin) mischiefStatusCmd(ctx MessageContext) error {
	if c := liveMischiefForTarget(ctx.Sender); c != nil {
		t, _ := mischiefTierByKey(c.Tier)
		who := "Someone"
		if c.Signed {
			who = "**" + p.DisplayName(c.BuyerID) + "**"
		}
		wards := ""
		if c.BlessingCount > 0 {
			wards = fmt.Sprintf("\n🕯️ %d of the town's wards are on you.", c.BlessingCount)
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"😈 %s has paid for a **%s** to find you. It arrives %s. Survive it and you keep %s.%s",
			who, strings.ToLower(t.Display), formatDuration(time.Until(c.WindowEndsAt)),
			fmtEuro(mischiefPurse(c.Fee, t.PayoutPct)), wards))
	}
	rows, err := db.Get().Query(
		`SELECT `+mischiefCols+` FROM mischief_contracts
		  WHERE buyer_id = ? AND status IN (?, ?) ORDER BY created_at DESC`,
		string(ctx.Sender), mischiefStatusOpen, mischiefStatusDelivering)
	if err == nil {
		defer rows.Close()
		var lines []string
		for rows.Next() {
			if c, err := scanMischief(rows); err == nil {
				lines = append(lines, fmt.Sprintf("· a **%s** for **%s**, landing %s",
					c.Tier, p.DisplayName(c.TargetID), formatDuration(time.Until(c.WindowEndsAt))))
			}
		}
		if len(lines) > 0 {
			return p.SendReply(ctx.RoomID, ctx.EventID,
				"😈 **Your contracts in flight**\n"+strings.Join(lines, "\n"))
		}
	}
	return p.SendReply(ctx.RoomID, ctx.EventID,
		"Nothing out on you, and nothing out from you. `!mischief` for the board.")
}

func (p *AdventurePlugin) mischiefSendCmd(ctx MessageContext, fields []string) error {
	if len(fields) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!mischief send <grunt|mob|elite|boss> @user [signed]`")
	}
	tier, ok := mischiefTierByKey(fields[0])
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"That's not a tier. Try `grunt`, `mob`, `elite` or `boss` — `!mischief` for the board.")
	}
	targetID, ok := p.ResolveUser(fields[1], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Could not resolve that user. Usage: `!mischief send <tier> @user`")
	}
	if targetID == ctx.Sender {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"You can't put a price on your own head. Get help elsewhere.")
	}
	signed := len(fields) > 2 && strings.EqualFold(fields[2], "signed")

	// Serialise this buyer's placement so a double-submit can't pass the daily
	// cap twice and double-debit — same discipline as the duel escrow.
	lock := p.advUserLock(ctx.Sender)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now().UTC()
	if n := mischiefBuyerCountSince(ctx.Sender, now.Add(-24*time.Hour)); n >= mischiefBuyerDailyCap {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"You've already sent %d monsters today. Even mischief keeps office hours.", n))
	}
	if reason := p.mischiefTargetable(targetID, tier, now); reason != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, reason)
	}

	price := tier.Fee
	if signed {
		price = mischiefSignedFee(tier.Fee)
	}
	if bal := p.euro.GetBalance(ctx.Sender); int(bal) < price {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"A %s runs %s — you have %s.", strings.ToLower(tier.Display), fmtEuro(price), fmtEuro(int(bal))))
	}
	if !p.euro.Debit(ctx.Sender, float64(price), "mischief_contract") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't take your money. Try again.")
	}

	c := &mischiefContract{
		ID:           uuid.NewString(),
		BuyerID:      ctx.Sender,
		TargetID:     targetID,
		Tier:         tier.Key,
		Fee:          tier.Fee,
		Paid:         price,
		Status:       mischiefStatusOpen,
		Signed:       signed,
		Source:       "matrix",
		CreatedAt:    now,
		WindowEndsAt: now.Add(mischiefWindow),
	}
	// The one-live-contract-per-target index is the real guard: the check above
	// ran under this buyer's lock, and another buyer could have slipped a contract
	// onto the same target since. Losing that race costs the buyer nothing.
	if err := insertMischiefContract(c); err != nil {
		p.euro.Credit(ctx.Sender, float64(price), "mischief_refund_contested")
		slog.Warn("mischief: contract insert rejected",
			"buyer", ctx.Sender, "target", targetID, "err", err)
		// Refund either way, but don't tell a buyer a rival outbid them when the
		// write simply failed — they'd stop retrying and believe in a contract that
		// does not exist. The index is the only thing that legitimately rejects here.
		if liveMischiefForTarget(targetID) != nil {
			return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
				"Somebody beat you to **%s** — there's already a monster on the way. You've been refunded.",
				p.DisplayName(targetID)))
		}
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"The contract didn't take — something went wrong on our end. You've been refunded; try again.")
	}

	p.announceMischiefContract(c, tier)
	p.dmMischiefVictim(c, tier)
	emitMischiefContractNews(c, tier)

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
		"😈 Done. A **%s** is on its way to **%s** — it finds them in about %s.\n%s",
		strings.ToLower(tier.Display), p.DisplayName(targetID), formatDuration(mischiefWindow),
		mischiefBuyerNote(signed)))
}

// mischiefOpenContractFor resolves "@user" to the contract currently open against
// them, or explains why the room can't act on it. A contract already claimed for
// delivery is deliberately out of reach: the monster is in the room with them and
// a ward bought now would have to reach back into a fight that has started.
func (p *AdventurePlugin) mischiefOpenContractFor(ctx MessageContext, fields []string, verb string) (*mischiefContract, id.UserID, string) {
	if len(fields) < 1 {
		return nil, "", fmt.Sprintf("Usage: `!mischief %s @user`", verb)
	}
	targetID, ok := p.ResolveUser(fields[0], ctx.RoomID)
	if !ok {
		return nil, "", fmt.Sprintf("Could not resolve that user. Usage: `!mischief %s @user`", verb)
	}
	c := liveMischiefForTarget(targetID)
	if c == nil {
		return nil, targetID, fmt.Sprintf("Nobody has coin on **%s** right now.", p.DisplayName(targetID))
	}
	if c.Status != mischiefStatusOpen {
		return nil, targetID, fmt.Sprintf(
			"Too late — the thing has already found **%s**.", p.DisplayName(targetID))
	}
	return c, targetID, ""
}

// mischiefBlessCmd — the helping half of the window. The fee is a pure sink (it
// goes to the community pot, never to the purse), so a blessing can't be used to
// route money to the target: it buys them HP, not euros.
func (p *AdventurePlugin) mischiefBlessCmd(ctx MessageContext, fields []string) error {
	lock := p.advUserLock(ctx.Sender)
	lock.Lock()
	defer lock.Unlock()

	c, targetID, reason := p.mischiefOpenContractFor(ctx, fields, "bless")
	if c == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, reason)
	}
	if c.BlessingCount >= mischiefBlessingCap {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"**%s** is already carrying every ward the town has to give (%d). The rest is up to them.",
			p.DisplayName(targetID), mischiefBlessingCap))
	}
	if bal := p.euro.GetBalance(ctx.Sender); int(bal) < mischiefBlessingFee {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"A ward costs %s — you have %s.", fmtEuro(mischiefBlessingFee), fmtEuro(int(bal))))
	}
	if !p.euro.Debit(ctx.Sender, float64(mischiefBlessingFee), "mischief_blessing") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't take your money. Try again.")
	}
	// The cap is enforced by the UPDATE, not by the read above — two people can
	// buy the third ward in the same second, and only one of them may have it.
	if !blessMischiefContract(c.ID) {
		p.euro.Credit(ctx.Sender, float64(mischiefBlessingFee), "mischief_blessing_refund")
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Somebody got there first — the ward wasn't needed. You've been refunded.")
	}
	communityPotAdd(mischiefBlessingFee)
	trackTaxPaid(ctx.Sender, mischiefBlessingFee)

	p.announceMischiefBlessing(c, ctx.Sender, c.BlessingCount+1)
	p.SendDM(targetID, fmt.Sprintf(
		"🕯️ **%s** has paid for a ward on you. Whatever's coming, I've made you harder to put down.",
		p.DisplayName(ctx.Sender)))
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
		"🕯️ Done — **%s** goes into that fight tougher than they would have. (%d/%d wards)",
		p.DisplayName(targetID), c.BlessingCount+1, mischiefBlessingCap))
}

// mischiefEscalateCmd — the "helping" half. Anyone but the target can pay the
// difference to send something worse. The money joins the payout basis, so piling
// on raises the jackpot the target walks away with if they live: the room's cruelty
// is also its generosity, which is the joke.
func (p *AdventurePlugin) mischiefEscalateCmd(ctx MessageContext, fields []string) error {
	lock := p.advUserLock(ctx.Sender)
	lock.Lock()
	defer lock.Unlock()

	c, targetID, reason := p.mischiefOpenContractFor(ctx, fields, "escalate")
	if c == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, reason)
	}
	if targetID == ctx.Sender {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"You don't get to raise the price on your own head. Have some dignity.")
	}
	cur, _ := mischiefTierByKey(c.Tier)
	next, ok := mischiefNextTier(c.Tier)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"It's already the worst thing in their bracket. There is nothing bigger to send.")
	}
	if c.EscalationCount > 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"Somebody has already made it worse — it's a **%s** now. Once is the limit.",
			strings.ToLower(cur.Display)))
	}
	// An escalation into boss tier is still a boss landing on this person, so it
	// answers to the same weekly limit a bought boss does. (This contract is not a
	// boss yet, so it cannot count itself.)
	now := time.Now().UTC()
	if next.Key == "boss" && mischiefBossCountSince(targetID, now.Add(-mischiefBossPerTargetWindow)) > 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"**%s** has already had a boss sent after them this week. Let them keep this one.",
			p.DisplayName(targetID)))
	}
	delta := next.Fee - cur.Fee
	if bal := p.euro.GetBalance(ctx.Sender); int(bal) < delta {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"Bumping a %s up to a %s costs the difference: %s. You have %s.",
			strings.ToLower(cur.Display), strings.ToLower(next.Display), fmtEuro(delta), fmtEuro(int(bal))))
	}
	if !p.euro.Debit(ctx.Sender, float64(delta), "mischief_escalation") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't take your money. Try again.")
	}
	if !escalateMischiefContract(c.ID, cur.Key, next, delta, ctx.Sender) {
		p.euro.Credit(ctx.Sender, float64(delta), "mischief_escalation_refund")
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Somebody beat you to it, or the thing already left. You've been refunded.")
	}

	purse := mischiefPurse(next.Fee, next.PayoutPct)
	p.announceMischiefEscalation(c, next, purse)
	p.SendDM(targetID, fmt.Sprintf(
		"😈 It got worse. What's coming for you is a **%s** now — somebody in town paid to upgrade it.\n"+
			"Their money's in the pot with the rest: walk away from it and you keep %s.",
		strings.ToLower(next.Display), fmtEuro(purse)))
	if c.BuyerID != ctx.Sender {
		p.SendDM(c.BuyerID, fmt.Sprintf(
			"😈 Somebody liked your idea and paid to make it worse: **%s** is getting a **%s** now.",
			p.DisplayName(targetID), strings.ToLower(next.Display)))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
		"😈 Paid. It's a **%s** now. If **%s** walks away from it they keep %s — you helped with that too.\n"+
			"_Nobody knows it was you — unless they survive._",
		strings.ToLower(next.Display), p.DisplayName(targetID), fmtEuro(purse)))
}

func mischiefBuyerNote(signed bool) string {
	if signed {
		return "_Your name is on it. Everyone already knows._"
	}
	return "_Nobody knows it was you — unless they walk away from it._"
}

// ── Announcements ────────────────────────────────────────────────────────────

// announceMischiefContract tells the games room a hit is out. The buyer is named
// only if they paid to sign it; otherwise this is the whole point of the window —
// the room knows someone did it and doesn't know who.
func (p *AdventurePlugin) announceMischiefContract(c *mischiefContract, tier mischiefTier) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	who := "**Someone in town**"
	if c.Signed {
		who = fmt.Sprintf("**%s**", p.DisplayName(c.BuyerID))
	}
	p.SendMessage(gr, fmt.Sprintf(
		"😈 %s has put %s on **%s**'s head — a **%s** is already looking for them out there.\n"+
			"It finds them in about %s. If they walk away from it, they keep %s of that money — and we all find out who paid.",
		who, fmtEuro(c.Paid), p.DisplayName(c.TargetID), strings.ToLower(tier.Display),
		formatDuration(time.Until(c.WindowEndsAt)), fmtEuro(mischiefPurse(c.Fee, tier.PayoutPct))))
}

// dmMischiefVictim is TwinBee telling the target that somebody wants them dead.
// Pure flavor — the expedition is autonomous and they cannot act on it directly —
// but it is what lets them ask the room for wards, which is the whole window.
func (p *AdventurePlugin) dmMischiefVictim(c *mischiefContract, tier mischiefTier) {
	who := "Somebody in town"
	if c.Signed {
		who = "**" + p.DisplayName(c.BuyerID) + "**"
	}
	p.SendDM(c.TargetID, fmt.Sprintf(
		"😈 **Word's reached me, and you're not going to like it.**\n"+
			"%s has paid to have a **%s** find you out there. It's already looking. I make it about %s.\n\n"+
			"I can't call it off, and I can't come out there. What the town *can* do is ward you — "+
			"anyone who likes you can `!mischief bless @%s` (%s each, up to %d).\n"+
			"Walk away from it and their money is yours: **%s**. And we all find out who paid.",
		who, strings.ToLower(tier.Display), formatDuration(time.Until(c.WindowEndsAt)),
		p.DisplayName(c.TargetID), fmtEuro(mischiefBlessingFee), mischiefBlessingCap,
		fmtEuro(mischiefPurse(c.Fee, tier.PayoutPct))))
}

// announceMischiefBlessing — helping is public and immediate. The blesser is
// named on the spot: there is no reason to hide having done someone a kindness,
// and the room seeing wards go up is what pulls the next one out of somebody.
func (p *AdventurePlugin) announceMischiefBlessing(c *mischiefContract, blesser id.UserID, count int) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	p.SendMessage(gr, fmt.Sprintf(
		"🕯️ **%s** puts a ward on **%s** — %d of %d. Whatever's coming for them will have to work harder.",
		p.DisplayName(blesser), p.DisplayName(c.TargetID), count, mischiefBlessingCap))
}

// announceMischiefEscalation — "helping" is anonymous, exactly like the purchase
// it piles onto, and unsealed by the same survival.
func (p *AdventurePlugin) announceMischiefEscalation(c *mischiefContract, next mischiefTier, purse int) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	p.SendMessage(gr, fmt.Sprintf(
		"😈 **Somebody has made it worse.** What's hunting **%s** out there is a **%s** now.\n"+
			"They put their money in the pot to do it — so if **%s** walks away, the purse is up to %s.",
		p.DisplayName(c.TargetID), strings.ToLower(next.Display),
		p.DisplayName(c.TargetID), fmtEuro(purse)))
}

func (p *AdventurePlugin) announceMischiefSurvived(c *mischiefContract, monsterName string, purse int) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	// The unseal. Anonymity is the buyer's to lose, and losing it is what makes
	// a survival worth announcing. Whoever paid to make it worse loses theirs on
	// the same terms — piling on is a purchase, and it carries the same exposure.
	p.SendMessage(gr, fmt.Sprintf(
		"🛡️ **%s walked away from it.** The **%s** that came for them is dead, and %s is %s richer for the trouble.\n"+
			"The coin came from **%s**.%s",
		p.DisplayName(c.TargetID), monsterName, p.DisplayName(c.TargetID), fmtEuro(purse),
		p.DisplayName(c.BuyerID), p.mischiefEscalatorNote(c)))
}

// mischiefEscalatorNote names whoever paid to upgrade the contract, for the
// unseal. Empty when nobody did.
func (p *AdventurePlugin) mischiefEscalatorNote(c *mischiefContract) string {
	if c.EscalatedBy == "" {
		return ""
	}
	return fmt.Sprintf(" **%s** paid to make it worse.", p.DisplayName(c.EscalatedBy))
}

func (p *AdventurePlugin) announceMischiefDowned(c *mischiefContract, monsterName string) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	who := "Whoever paid for it"
	if c.Signed {
		who = fmt.Sprintf("**%s**, who paid for it,", p.DisplayName(c.BuyerID))
	}
	p.SendMessage(gr, fmt.Sprintf(
		"💀 **%s didn't walk away.** A **%s** found them mid-run and put them on the floor. "+
			"They're being carried home — expedition over, pockets lighter.\n%s keeps nothing: the money's gone to the community pot.",
		p.DisplayName(c.TargetID), monsterName, who))
}

// ── Pete news ────────────────────────────────────────────────────────────────
//
// Deploy Pete BEFORE gogobee whenever these types change: Pete 400s an unknown
// event_type, gogobee retries, and the bulletin parks forever.
//
// BULLETIN, never priority. Priority tier is the one thing that makes Pete post a
// live Matrix beat, and TwinBee already announces every one of these in the games
// room as it happens (announceMischiefContract / Survived / Downed) — a priority
// fact would just read Pete's version back to the people who watched it. Bulletin
// still carries the story to the site and the daily digest, where a roundup is
// what it is. Same call 1cbd68a made for death and zone_first.

func emitMischiefContractNews(c *mischiefContract, tier mischiefTier) {
	target := charName(c.TargetID)
	if target == "" {
		return
	}
	buyer := ""
	buyerUser := id.UserID("")
	if c.Signed {
		buyer = charName(c.BuyerID)
		buyerUser = c.BuyerID
	}
	ts := nowUnix()
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("mischief_contract:%s:%d", eventToken(c.TargetID, "mischief:"+c.ID), ts),
		EventType:  "mischief_contract",
		Tier:       "bulletin",
		Subject:    target,
		Opponent:   buyer,
		Boss:       tier.Display,
		Stakes:     fmtEuro(c.Paid),
		Level:      charLevel(c.TargetID),
		OccurredAt: ts,
	}, c.TargetID, buyerUser)
}

// emitMischiefResolvedNews files the survival or the maiming. On a survival the
// buyer is named whether or not they signed — that is the unseal, and it is the
// only brake on casual griefing. (`!news optout` still anonymizes them to "an
// adventurer", as it does everywhere else.)
func emitMischiefResolvedNews(c *mischiefContract, monsterName, outcome string, purse int) {
	target := charName(c.TargetID)
	if target == "" {
		return
	}
	eventType := "mischief_downed"
	buyer, buyerUser := "", id.UserID("")
	if outcome == mischiefOutcomeSurvived {
		eventType = "mischief_survived"
		buyer, buyerUser = charName(c.BuyerID), c.BuyerID
	} else if c.Signed {
		buyer, buyerUser = charName(c.BuyerID), c.BuyerID
	}
	ts := nowUnix()
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("%s:%s:%d", eventType, eventToken(c.TargetID, "mischief:"+c.ID), ts),
		EventType:  eventType,
		Tier:       "bulletin",
		Subject:    target,
		Opponent:   buyer,
		Boss:       monsterName,
		Stakes:     fmtEuro(purse),
		Level:      charLevel(c.TargetID),
		Outcome:    outcome,
		OccurredAt: ts,
	}, c.TargetID, buyerUser)
}

func emitMischiefFizzledNews(c *mischiefContract, refund int) {
	target := charName(c.TargetID)
	if target == "" {
		return
	}
	ts := nowUnix()
	emitFact(peteclient.Fact{
		GUID:       fmt.Sprintf("mischief_fizzled:%s:%d", eventToken(c.TargetID, "mischief:"+c.ID), ts),
		EventType:  "mischief_fizzled",
		Tier:       "bulletin",
		Subject:    target,
		Stakes:     fmtEuro(refund),
		Outcome:    "fizzled",
		OccurredAt: ts,
	}, c.TargetID, "")
}

// mischiefContractByID is the test/ops read path.
func mischiefContractByID(contractID string) (*mischiefContract, error) {
	row := db.Get().QueryRow(
		`SELECT `+mischiefCols+` FROM mischief_contracts WHERE contract_id = ?`, contractID)
	c, err := scanMischief(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}
