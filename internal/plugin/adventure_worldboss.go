package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// World boss (N6/C3) — the monthly communal "Siege". A named boss camps outside
// town for a fixed window with a single shared HP pool. Every player gets one
// bout per day against it (an arena-style fight through runZoneCombat); the
// damage they deal is subtracted from the shared pool whether the individual
// bout is won or lost. The pool falling to zero defeats the boss; the window
// lapsing with the pool still up means the boss survives and loots the town.
//
// Economy (decisions locked 2026-07-10): a defeat pays every contributor from a
// freshly minted bounty scaled by fights fought (not damage — accessibility),
// plus a consumable cache and one low-rate treasure roll each; a survival debits
// the community pot as a tribute (a sink the economy wants). The bout itself
// costs real HP the way the arena does — no restore — but never permadeaths.
//
// The event lives in its own tables (world_boss / world_boss_contrib), outside
// the saveAdvCharacter fan-out, so a character save can never clobber the shared
// pool — the same structural isolation adventure_shadow earns.

const (
	// worldBossWindow is how long the boss camps outside town once it spawns.
	worldBossWindow = 72 * time.Hour
	// worldBossPresenceDays is the any-chat presence lookback used to size the
	// boss to the community that will actually fight it
	// (feedback_presence_is_any_chat).
	worldBossPresenceDays = 14
	// worldBossMinTier floors the boss tier — the Siege is always a serious
	// fight, never a T1/T2 pushover, even for a low-level town.
	worldBossMinTier = 3
	// worldBossBoutsPerPlayer sizes the shared pool: expected full-damage bouts
	// to fell the boss ≈ activePlayers × this. Higher = a longer siege that needs
	// more of the town to turn up. Tunable; watch prod turnout.
	worldBossBoutsPerPlayer = 2.0
	// worldBossMinBouts / worldBossMaxBouts clamp the pool so a near-empty town
	// still gets a beatable boss and a huge town doesn't get an unkillable one.
	worldBossMinBouts = 4
	worldBossMaxBouts = 60
	// worldBossDefeatBaseEuro is the minted bounty a single fight earns a
	// contributor on a defeat; total payout = base × fights fought.
	worldBossDefeatBaseEuro = 1500
	// worldBossTributePct is the fraction of the community pot the boss loots on
	// a survival — a visible pot sink.
	worldBossTributePct = 0.20
	// worldBossTreasureWeight is the (low) weight of the single treasure roll each
	// contributor gets on a defeat.
	worldBossTreasureWeight = advTreasureWeightElite
	// worldBossCacheSize is how many tier consumables each contributor is handed
	// on a defeat.
	worldBossCacheSize = 2
)

// worldBossState mirrors an active world_boss row.
type worldBossState struct {
	ID        int64
	Name      string
	Tier      int
	HPMax     int
	HPCurrent int
	Status    string // active | defeated | survived
	StartsAt  time.Time
	EndsAt    time.Time
}

// worldBossContrib mirrors a world_boss_contrib row — one player's tally against
// one boss.
type worldBossContrib struct {
	BossID        int64
	UserID        id.UserID
	Fights        int
	Damage        int
	LastFightDate string
}

// worldBossNames is the pool a Siege boss is named from — big, ancient,
// town-threatening things. Picked deterministically per event so a given month's
// boss reads the same to everyone.
var worldBossNames = []string{
	"Gorloth the Sunderer", "The Ashen Wyrm", "Kravok, Maw of the Deep",
	"Nyxaris the Devouring", "The Iron Colossus", "Ssath'ra, Coil of Ruin",
	"Ymirok the Frostbound", "The Hollow Leviathan", "Dravmaug, Ender of Walls",
	"The Bone Cathedral", "Vornath the Insatiable", "The Screaming Tide",
}

// worldBossNameFor picks a stable boss name from an event key (the spawn month),
// so the same Siege always reads by the same name.
func worldBossNameFor(eventKey string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(eventKey))
	return worldBossNames[int(h.Sum32())%len(worldBossNames)]
}

// ── Persistence ──────────────────────────────────────────────────────────────

func scanWorldBoss(row interface{ Scan(...any) error }) (*worldBossState, error) {
	b := &worldBossState{}
	err := row.Scan(&b.ID, &b.Name, &b.Tier, &b.HPMax, &b.HPCurrent, &b.Status, &b.StartsAt, &b.EndsAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// loadActiveWorldBoss returns the live Siege, or (nil, nil) if none is camped.
func loadActiveWorldBoss() (*worldBossState, error) {
	b, err := scanWorldBoss(db.Get().QueryRow(
		`SELECT id, name, tier, hp_max, hp_current, status, starts_at, ends_at
		   FROM world_boss WHERE status = 'active' ORDER BY id DESC LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// loadWorldBoss reads one boss by id (history included). (nil, nil) if absent.
func loadWorldBoss(bossID int64) (*worldBossState, error) {
	b, err := scanWorldBoss(db.Get().QueryRow(
		`SELECT id, name, tier, hp_max, hp_current, status, starts_at, ends_at
		   FROM world_boss WHERE id = ?`, bossID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// insertWorldBoss writes a fresh active boss and returns its id.
func insertWorldBoss(name string, tier, hpMax int, startsAt, endsAt time.Time) (int64, error) {
	res, err := db.Get().Exec(
		`INSERT INTO world_boss (name, tier, hp_max, hp_current, status, starts_at, ends_at)
		 VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		name, tier, hpMax, hpMax, startsAt, endsAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// applyWorldBossDamage subtracts damage from the shared pool atomically, clamped
// at zero, only while the boss is still active. Returns the pool's new value and
// whether the pool is now down (killed). The UPDATE and the re-read are separate
// statements, so two concurrent killers can BOTH observe killed=true — that is
// fine: resolveWorldBossDefeated dedupes on the status='active' guard in
// setWorldBossStatus, so only one payout ever fires. killed is just "should the
// caller attempt resolution".
func applyWorldBossDamage(bossID int64, dmg int) (remaining int, killed bool, err error) {
	if dmg < 0 {
		dmg = 0
	}
	_, err = db.Get().Exec(
		`UPDATE world_boss SET hp_current = MAX(0, hp_current - ?)
		   WHERE id = ? AND status = 'active'`, dmg, bossID)
	if err != nil {
		return 0, false, err
	}
	b, err := loadWorldBoss(bossID)
	if err != nil || b == nil {
		return 0, false, err
	}
	return b.HPCurrent, b.HPCurrent <= 0 && b.Status == "active", nil
}

// setWorldBossStatus closes out a boss (defeated | survived) and stamps
// resolved_at. Guarded on status='active' so a double resolution is a no-op.
func setWorldBossStatus(bossID int64, status string) (bool, error) {
	res, err := db.Get().Exec(
		`UPDATE world_boss SET status = ?, resolved_at = CURRENT_TIMESTAMP
		   WHERE id = ? AND status = 'active'`, status, bossID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// upsertWorldBossContrib bumps a player's tally against a boss.
func upsertWorldBossContrib(bossID int64, userID id.UserID, dmg int, dateKey string) error {
	_, err := db.Get().Exec(
		`INSERT INTO world_boss_contrib (boss_id, user_id, fights, damage, last_fight_date)
		 VALUES (?, ?, 1, ?, ?)
		 ON CONFLICT(boss_id, user_id) DO UPDATE SET
		   fights = fights + 1,
		   damage = damage + excluded.damage,
		   last_fight_date = excluded.last_fight_date`,
		bossID, string(userID), dmg, dateKey)
	return err
}

// loadWorldBossContrib reads one player's tally, or (nil, nil) if they haven't
// fought this boss.
func loadWorldBossContrib(bossID int64, userID id.UserID) (*worldBossContrib, error) {
	c := &worldBossContrib{BossID: bossID, UserID: userID}
	err := db.Get().QueryRow(
		`SELECT fights, damage, last_fight_date FROM world_boss_contrib
		   WHERE boss_id = ? AND user_id = ?`, bossID, string(userID)).
		Scan(&c.Fights, &c.Damage, &c.LastFightDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// loadWorldBossContribs reads every contributor to a boss, ordered by fights
// desc then damage desc (the board order).
func loadWorldBossContribs(bossID int64) ([]worldBossContrib, error) {
	rows, err := db.Get().Query(
		`SELECT user_id, fights, damage, last_fight_date FROM world_boss_contrib
		   WHERE boss_id = ? ORDER BY fights DESC, damage DESC`, bossID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []worldBossContrib
	for rows.Next() {
		c := worldBossContrib{BossID: bossID}
		var uid string
		if err := rows.Scan(&uid, &c.Fights, &c.Damage, &c.LastFightDate); err != nil {
			return nil, err
		}
		c.UserID = id.UserID(uid)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── Scaling ──────────────────────────────────────────────────────────────────

// activePlayerIDs returns the user ids that have chatted (any message, not just
// commands) in the last `days` days — the any-chat presence signal
// (feedback_presence_is_any_chat), read off the streaks plugin's daily_activity
// table.
func activePlayerIDs(days int) ([]id.UserID, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := db.Get().Query(
		`SELECT DISTINCT user_id FROM daily_activity WHERE date >= ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []id.UserID
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, id.UserID(uid))
	}
	return out, rows.Err()
}

// combinedAdvLevel is the same "how strong is this adventurer" reducer TwinBee
// uses — combat level plus the three gathering skills.
func combinedAdvLevel(c *AdventureCharacter) int {
	return dndLevelForUser(c.UserID) + c.MiningSkill + c.ForagingSkill + c.FishingSkill
}

// medianInt returns the median of a sorted-or-not int slice (0 for empty).
func medianInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// tierForCombinedLevel buckets a combined adventure level to a zone tier, floored
// at worldBossMinTier. Same thresholds as twinBeeMaxTier (combined 12 → T4,
// 21 → T5), applied here to the median rather than the max.
func tierForCombinedLevel(level int) int {
	switch {
	case level >= 21:
		return 5
	case level >= 12:
		return 4
	default:
		return worldBossMinTier
	}
}

// groupInt renders an int with thousands separators (no currency symbol),
// reusing fmtEuro's grouping.
func groupInt(n int) string {
	return strings.TrimPrefix(fmtEuro(n), "€")
}

// worldBossSpawnPlan computes the boss's tier and pool HP from the players active
// in the presence window. Returns the tier, HP, and the active-player count that
// drove the sizing (0 players ⇒ a floor boss so a manual spawn still works).
func worldBossSpawnPlan() (tier, hpMax, activeN int) {
	ids, err := activePlayerIDs(worldBossPresenceDays)
	if err != nil {
		slog.Warn("worldboss: active player lookup failed", "err", err)
	}
	active := make(map[id.UserID]bool, len(ids))
	for _, uid := range ids {
		active[uid] = true
	}
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Warn("worldboss: char load failed", "err", err)
	}
	var levels []int
	for i := range chars {
		c := &chars[i]
		if !active[c.UserID] || !c.Alive {
			continue
		}
		levels = append(levels, combinedAdvLevel(c))
	}
	activeN = len(levels)
	tier = tierForCombinedLevel(medianInt(levels))

	bouts := int(float64(max(activeN, 1)) * worldBossBoutsPerPlayer)
	if bouts < worldBossMinBouts {
		bouts = worldBossMinBouts
	}
	if bouts > worldBossMaxBouts {
		bouts = worldBossMaxBouts
	}
	perBout, _, _, _, _ := arenaTierBaseStats(tier)
	hpMax = perBout * bouts
	return tier, hpMax, activeN
}

// worldBossTemplate builds the disposable per-bout enemy stat block. The shared
// pool lives in world_boss.hp_current; this template is just the thing a single
// player swings at, so its own HP is the arena tier's boss HP — beatable in a
// bout, which lets a strong player land a full boss's worth of damage on the
// pool.
func worldBossTemplate(name string, tier int) DnDMonsterTemplate {
	hp, ac, atk, ab, cr := arenaTierBaseStats(tier)
	return DnDMonsterTemplate{
		ID:          fmt.Sprintf("worldboss_t%d", tier),
		Name:        name,
		CR:          cr,
		HP:          hp,
		AC:          ac,
		Attack:      atk,
		AttackBonus: ab,
		Speed:       12,
		BlockRate:   0.05,
		XPValue:     arenaTiers[tier-1].BattleXP * 5,
		Notes:       "the Siege",
	}
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

// spawnWorldBoss mints a fresh Siege and announces it. It refuses if one is
// already camped (only one at a time). eventKey names the boss deterministically
// (the spawn month for the auto path, a stamp for a manual one).
func (p *AdventurePlugin) spawnWorldBoss(eventKey string) (*worldBossState, error) {
	if existing, err := loadActiveWorldBoss(); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, fmt.Errorf("a world boss is already active")
	}
	tier, hpMax, activeN := worldBossSpawnPlan()
	name := worldBossNameFor(eventKey)
	now := time.Now().UTC()
	ends := now.Add(worldBossWindow)
	bossID, err := insertWorldBoss(name, tier, hpMax, now, ends)
	if err != nil {
		return nil, err
	}
	boss, err := loadWorldBoss(bossID)
	if err != nil {
		return nil, err
	}
	p.announceWorldBossSpawn(boss, activeN)
	emitSiegeStart(boss)
	slog.Info("worldboss: spawned", "id", bossID, "name", name, "tier", tier, "hp", hpMax, "activeN", activeN)
	return boss, nil
}

// worldBossTick rides the 1-minute event ticker. It auto-spawns the month's
// boss and resolves a live boss whose window has lapsed. The defeat path is not
// here — a bout crossing the pool to zero resolves inline (W2), because the
// ticker never sees the pool between two 60s reads.
func (p *AdventurePlugin) worldBossTick() {
	boss, err := loadActiveWorldBoss()
	if err != nil {
		slog.Warn("worldboss: tick load failed", "err", err)
		return
	}
	now := time.Now().UTC()
	if boss != nil {
		// Safety net: a killing blow commits the pool to 0 inline, but if the
		// process died (or was redeployed) before the bout resolved the status,
		// the boss is left active/0-HP. Resolve it as the defeat it was — never
		// let the survive branch below debit the pot for a boss the town killed.
		if boss.HPCurrent <= 0 {
			p.resolveWorldBossDefeated(boss)
			return
		}
		if now.After(boss.EndsAt) {
			p.resolveWorldBossSurvived(boss)
		}
		return
	}
	// No boss camped — spawn this month's Siege, once.
	//
	// The month key below is the whole dedup, so the rule is simply "one Siege
	// per calendar month, as early as the process is up to run it." It used to
	// additionally require now.Day() == 1, which deadlocked the entire feature:
	// the world boss shipped mid-July 2026 and prod never once ran a first-of-
	// the-month tick with the code in it, so `select count(*) from world_boss`
	// was still 0 weeks later. The day gate also silently skipped any month
	// where the bot happened to be down or redeploying across the 1st, with no
	// catch-up. Dropping it makes a missed 1st self-heal on the next tick
	// instead of costing the town a month.
	monthKey := now.Format("2006-01")
	if db.JobCompleted("worldboss_spawn", monthKey) {
		return
	}
	if _, err := p.spawnWorldBoss(monthKey); err != nil {
		slog.Warn("worldboss: auto-spawn failed", "err", err)
		return
	}
	db.MarkJobCompleted("worldboss_spawn", monthKey)
}

// ── Resolution ───────────────────────────────────────────────────────────────

// worldBossPayout is one contributor's minted defeat reward.
type worldBossPayout struct {
	UserID id.UserID
	Fights int
	Euro   int
}

// computeWorldBossPayouts scales the minted bounty by fights fought (not damage —
// accessibility). Pure so the split is testable without a euro/Matrix stub.
func computeWorldBossPayouts(contribs []worldBossContrib, baseEuro int) []worldBossPayout {
	out := make([]worldBossPayout, 0, len(contribs))
	for _, c := range contribs {
		if c.Fights <= 0 {
			continue
		}
		out = append(out, worldBossPayout{UserID: c.UserID, Fights: c.Fights, Euro: baseEuro * c.Fights})
	}
	return out
}

// resolveWorldBossDefeated pays every contributor a minted bounty scaled by
// fights, plus a consumable cache and one low-rate treasure roll each, then
// announces the kill. Guarded so it fires once.
func (p *AdventurePlugin) resolveWorldBossDefeated(boss *worldBossState) {
	won, err := setWorldBossStatus(boss.ID, "defeated")
	if err != nil {
		slog.Warn("worldboss: defeat close-out failed", "err", err)
		return
	}
	if !won {
		return // someone else already resolved it
	}
	contribs, err := loadWorldBossContribs(boss.ID)
	if err != nil {
		slog.Warn("worldboss: contrib load failed", "err", err)
	}
	payouts := computeWorldBossPayouts(contribs, worldBossDefeatBaseEuro)
	treasureZone := worldBossTreasureZone(boss.Tier)
	for _, pay := range payouts {
		p.euro.Credit(pay.UserID, float64(pay.Euro), "worldboss_bounty")
		for _, item := range consumableCache(boss.Tier, worldBossCacheSize) {
			it := item
			p.grantZoneItem(pay.UserID, &it, "🧪")
		}
		if treasureZone != "" {
			p.rollZoneTreasure(pay.UserID, treasureZone, worldBossTreasureWeight)
		}
	}
	p.announceWorldBossDefeated(boss, payouts)
	emitSiegeWin(boss, len(payouts))
	slog.Info("worldboss: defeated", "id", boss.ID, "contributors", len(payouts))
}

// resolveWorldBossSurvived debits the community pot as a tribute (a sink) and
// announces the town's loss. Guarded so it fires once.
func (p *AdventurePlugin) resolveWorldBossSurvived(boss *worldBossState) {
	won, err := setWorldBossStatus(boss.ID, "survived")
	if err != nil {
		slog.Warn("worldboss: survive close-out failed", "err", err)
		return
	}
	if !won {
		return
	}
	tribute := int(float64(communityPotBalance()) * worldBossTributePct)
	paid := 0
	if tribute > 0 && communityPotDebit(tribute) {
		paid = tribute
	}
	p.announceWorldBossSurvived(boss, paid)
	emitSiegeLoss(boss)
	slog.Info("worldboss: survived", "id", boss.ID, "tribute", paid)
}

// worldBossTreasureZone maps the boss tier to a representative zone whose treasure
// table the defeat roll draws from. "" if no zone of that tier exists.
func worldBossTreasureZone(tier int) ZoneID {
	zs := zonesByTier(ZoneTier(tier))
	if len(zs) == 0 {
		return ""
	}
	return zs[0].ID
}

// ── Announcements (games room) ───────────────────────────────────────────────

// announceWorldBoss posts to the shared games room. No-op if GAMES_ROOM is unset.
func (p *AdventurePlugin) announceWorldBoss(text string) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	if err := p.SendMessage(gr, text); err != nil {
		slog.Warn("worldboss: games-room announce failed", "err", err)
	}
}

func (p *AdventurePlugin) announceWorldBossSpawn(boss *worldBossState, activeN int) {
	p.announceWorldBoss(fmt.Sprintf(
		"⚔️ **The Siege has begun!** **%s** (Tier %d) camps outside town with **%s HP**.\n"+
			"Everyone gets **one bout per day** for the next 72 hours — `!adventure worldboss fight`. "+
			"Every hit chips the shared pool. Fell it together for a bounty; let it stand and it loots the town.",
		boss.Name, boss.Tier, groupInt(boss.HPMax)))
}

func (p *AdventurePlugin) announceWorldBossDefeated(boss *worldBossState, payouts []worldBossPayout) {
	var top string
	if len(payouts) > 0 {
		top = fmt.Sprintf(" Top of the muster: %s (%d fights).",
			p.DisplayName(payouts[0].UserID), payouts[0].Fights)
	}
	p.announceWorldBoss(fmt.Sprintf(
		"🏆 **%s has fallen!** %d adventurer(s) brought it down.%s\n"+
			"Bounties, supply caches, and salvage have been paid out to everyone who fought.",
		boss.Name, len(payouts), top))
}

func (p *AdventurePlugin) announceWorldBossSurvived(boss *worldBossState, tribute int) {
	msg := fmt.Sprintf("💀 **%s still stands.** The Siege is over — the town could not bring it down.", boss.Name)
	if tribute > 0 {
		msg += fmt.Sprintf(" It loots **€%s** from the community pot on its way out.", groupInt(tribute))
	}
	p.announceWorldBoss(msg)
}

// ── Player command (W2) ──────────────────────────────────────────────────────

// handleWorldBossCmd routes `!adventure worldboss [status|fight|spawn]` (alias
// `!adventure siege …`). Bare / "status" shows the board; "fight" takes today's
// bout; "spawn" is an operator override.
func (p *AdventurePlugin) handleWorldBossCmd(ctx MessageContext, args string) error {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "", "status":
		return p.worldBossStatusDM(ctx)
	case "fight":
		return p.fightWorldBoss(ctx)
	case "spawn":
		return p.worldBossOperatorSpawn(ctx)
	}
	return p.SendDM(ctx.Sender, "Usage: `!adventure worldboss [status|fight]`")
}

// worldBossOperatorSpawn lets an admin start a Siege on demand (the override half
// of the "both" spawn decision). Silent to non-admins, like the other admin
// commands.
func (p *AdventurePlugin) worldBossOperatorSpawn(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}
	key := "manual-" + time.Now().UTC().Format("2006-01-02T15:04")
	boss, err := p.spawnWorldBoss(key)
	if err != nil {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Could not spawn the Siege: %v", err))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"Spawned **%s** (Tier %d, %s HP). It camps for 72 hours.",
		boss.Name, boss.Tier, groupInt(boss.HPMax)))
}

// Sentinels for the four ways a bout can be refused, so a headless caller (the
// web action queue, pete_orders.go) can turn each into its own verdict instead of
// parsing a DM. `!adventure worldboss fight` maps them back to the prose it
// always sent.
var (
	errSiegeNoBoss        = errors.New("siege: nothing camped outside town")
	errSiegeNoCharacter   = errors.New("siege: no adventurer")
	errSiegeDead          = errors.New("siege: adventurer is dead")
	errSiegeAlreadyFought = errors.New("siege: today's bout already spent")
)

// takeSiegeBout runs one player's daily bout against the Siege: an arena-style
// solo fight whose damage is subtracted from the shared pool win or lose. Real
// HP cost, no death — a loss leaves the fighter battered (floored at 1 HP) but
// standing. The per-user lock serialises a player's own repeat submits, so the
// once-per-day gate can't be raced by a double-tap — and that same lock is what
// makes it safe for the web queue and a Matrix command to reach for the bout at
// the same moment.
//
// The combat narration is DM'd from here whichever door the bout came through: a
// fight is thirty lines of blow-by-blow and belongs in Matrix, not in a one-line
// verdict on a web page. The caller gets the boss and the result to describe.
func (p *AdventurePlugin) takeSiegeBout(uid id.UserID) (worldBossBoutResult, *worldBossState, error) {
	userMu := p.advUserLock(uid)
	userMu.Lock()
	defer userMu.Unlock()

	boss, err := loadActiveWorldBoss()
	if err != nil {
		return worldBossBoutResult{}, nil, fmt.Errorf("reaching the Siege: %w", err)
	}
	if boss == nil {
		return worldBossBoutResult{}, nil, errSiegeNoBoss
	}

	char, err := loadAdvCharacter(uid)
	if err != nil || char == nil {
		return worldBossBoutResult{}, boss, errSiegeNoCharacter
	}
	if !char.Alive {
		return worldBossBoutResult{}, boss, errSiegeDead
	}

	today := time.Now().UTC().Format("2006-01-02")
	if worldBossBoutUsedToday(boss.ID, uid, today) {
		return worldBossBoutResult{}, boss, errSiegeAlreadyFought
	}

	bout, err := p.resolveWorldBossBout(uid, boss, today)
	if err != nil {
		slog.Error("worldboss: bout failed", "user", uid, "err", err)
		return worldBossBoutResult{}, boss, fmt.Errorf("running the bout: %w", err)
	}

	// Resolve a defeat BEFORE streaming the (multi-second) narration. The pool is
	// already committed to 0; flipping the status now closes the window where a
	// crash or the ticker's survive path could resolve a killed boss as a
	// survival and debit the pot instead of paying bounties.
	if bout.Killed {
		p.resolveWorldBossDefeated(boss)
	}

	playerName, _ := loadDisplayName(uid)
	if playerName == "" {
		playerName = "You"
	}
	phaseMessages := append([]string{
		fmt.Sprintf("⚔️ **The Siege — %s** (Tier %d)", boss.Name, boss.Tier),
	}, RenderCombatLog(bout.Combat, playerName, boss.Name)...)

	<-p.sendZoneCombatMessages(uid, phaseMessages, siegeBoutFooter(bout, boss))
	return bout, boss, nil
}

// siegeBoutFooter is the one-line result of a bout — the damage dealt and what
// the pool looks like now. It closes the Matrix narration and doubles as the web
// verdict, so the two doors can't drift into describing the same fight
// differently.
func siegeBoutFooter(bout worldBossBoutResult, boss *worldBossState) string {
	var footer string
	if bout.Killed {
		footer = fmt.Sprintf("💥 You deal **%d** damage: the killing blow! **%s** falls!", bout.Damage, boss.Name)
	} else {
		footer = fmt.Sprintf("💥 You deal **%d** damage. **%s** has **%s / %s HP** left.",
			bout.Damage, boss.Name, groupInt(bout.Remaining), groupInt(boss.HPMax))
	}
	if bout.Battered {
		footer += "\nYou stagger out of the fight at 1 HP. Rest up before your next outing."
	}
	return footer
}

// fightWorldBoss is `!adventure worldboss fight`: the command framing around
// takeSiegeBout, which does the fight and the narration.
func (p *AdventurePlugin) fightWorldBoss(ctx MessageContext) error {
	_, boss, err := p.takeSiegeBout(ctx.Sender)
	switch {
	case errors.Is(err, errSiegeNoBoss):
		return p.SendDM(ctx.Sender, "No Siege is camped outside town right now.")
	case errors.Is(err, errSiegeNoCharacter):
		return p.SendDM(ctx.Sender, "You need an adventurer first — type `!adventure` to begin.")
	case errors.Is(err, errSiegeDead):
		return p.SendDM(ctx.Sender, "You're dead. The Siege will have to wait until you're back on your feet.")
	case errors.Is(err, errSiegeAlreadyFought):
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You've already taken your bout against **%s** today. Come back tomorrow — one fight per day.", boss.Name))
	case err != nil:
		return p.SendDM(ctx.Sender, "Something went wrong reaching the Siege. Try again in a moment.")
	}
	return nil
}

// worldBossBoutUsedToday reports whether a player has already spent their daily
// bout against this boss.
func worldBossBoutUsedToday(bossID int64, userID id.UserID, dateKey string) bool {
	c, err := loadWorldBossContrib(bossID, userID)
	return err == nil && c != nil && c.LastFightDate == dateKey
}

// worldBossBoutResult is the outcome of one resolved bout, before narration.
type worldBossBoutResult struct {
	Damage    int
	Remaining int
	Killed    bool
	Battered  bool
	Combat    CombatResult
}

// resolveWorldBossBout runs the arena-style fight, applies the real HP cost with
// the no-death floor, subtracts the damage dealt from the shared pool, and
// records the contribution. It performs no DM or games-room I/O, so it is the
// testable core of fightWorldBoss.
func (p *AdventurePlugin) resolveWorldBossBout(userID id.UserID, boss *worldBossState, dateKey string) (worldBossBoutResult, error) {
	monster := worldBossTemplate(boss.Name, boss.Tier)
	result, err := p.runZoneCombat(userID, monster, boss.Tier, bossCombatPhases, 50)
	if err != nil {
		return worldBossBoutResult{}, err
	}

	// No death / no hospital: floor the fighter at 1 HP so a loss never reads as
	// a corpse. runZoneCombat already persisted the real HP cost.
	battered := floorHPAtOne(userID)

	dmg := result.EnemyEntryHP - result.EnemyEndHP
	if dmg < 0 {
		dmg = 0
	}
	// Record the contribution (which also sets the once-per-day gate) BEFORE
	// draining the shared pool. If this write fails we must not have already
	// drained the pool — otherwise the player could refight a pool they emptied
	// and lose the credit for it. A hard error here aborts the bout; the HP cost
	// was already paid, but the pool is untouched and the player can retry.
	if err := upsertWorldBossContrib(boss.ID, userID, dmg, dateKey); err != nil {
		return worldBossBoutResult{}, err
	}
	remaining, killed, err := applyWorldBossDamage(boss.ID, dmg)
	if err != nil {
		return worldBossBoutResult{}, err
	}
	return worldBossBoutResult{
		Damage: dmg, Remaining: remaining, Killed: killed, Battered: battered, Combat: result,
	}, nil
}

// floorHPAtOne raises a player to 1 HP if the fight left them at zero, and
// reports whether it had to. Keeps "no death" honest without undoing the real
// HP cost of a bout the player mostly survived. Shared by the two no-death
// fights: the world-boss bout and a Mischief Makers delivery.
func floorHPAtOne(userID id.UserID) bool {
	cur, _ := dndHPSnapshot(userID)
	if cur > 0 {
		return false
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_character SET hp_current = 1, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
		string(userID)); err != nil {
		slog.Warn("worldboss: hp floor failed", "user", userID, "err", err)
	}
	return true
}

// worldBossStatusDM renders the current Siege board to the caller.
func (p *AdventurePlugin) worldBossStatusDM(ctx MessageContext) error {
	boss, err := loadActiveWorldBoss()
	if err != nil {
		return p.SendDM(ctx.Sender, "Something went wrong reaching the Siege. Try again in a moment.")
	}
	if boss == nil {
		return p.SendDM(ctx.Sender,
			"No Siege is camped outside town right now. The next one arrives at the start of the month.")
	}

	var sb strings.Builder
	pct := 0
	if boss.HPMax > 0 {
		pct = boss.HPCurrent * 100 / boss.HPMax
	}
	fmt.Fprintf(&sb, "⚔️ **The Siege — %s** (Tier %d)\n", boss.Name, boss.Tier)
	fmt.Fprintf(&sb, "Pool: **%s / %s HP** (%d%%)\n", groupInt(boss.HPCurrent), groupInt(boss.HPMax), pct)
	if left := time.Until(boss.EndsAt); left > 0 {
		h := int(left.Hours())
		m := int(left.Minutes()) % 60
		fmt.Fprintf(&sb, "Time left: **%dh %dm**\n", h, m)
	}

	today := time.Now().UTC().Format("2006-01-02")
	if c, _ := loadWorldBossContrib(boss.ID, ctx.Sender); c != nil {
		line := fmt.Sprintf("\nYou: %d fight(s), %s damage dealt.", c.Fights, groupInt(c.Damage))
		if c.LastFightDate == today {
			line += " Your bout is spent for today."
		} else {
			line += " Your bout is ready — `!adventure worldboss fight`."
		}
		sb.WriteString(line + "\n")
	} else {
		sb.WriteString("\nYou haven't fought yet — `!adventure worldboss fight`.\n")
	}

	if contribs, _ := loadWorldBossContribs(boss.ID); len(contribs) > 0 {
		sb.WriteString("\n**Top of the muster:**\n")
		for i, c := range contribs {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&sb, "%d. %s — %d fight(s)\n", i+1, p.DisplayName(c.UserID), c.Fights)
		}
	}
	return p.SendDM(ctx.Sender, sb.String())
}
