package plugin

// C2 — player-initiated duels (engagement plan N6).
//
// `!duel @user [stake]` — a staked, no-death arena bout between two real
// characters, resolved through the auto-resolve combat engine. Both players
// escrow the stake on accept; the winner takes 70% of the pooled 2×stake and
// the community pot rakes the remaining 30% (a real pot sink). Records land in
// adventure_rival_records, so one W/L history and one 7-day per-pair cooldown
// covers both duel kinds (the bot-driven RPS rival and this).
//
// Why this is a wrapper and not a combat-engine change: the turn engine is an
// N-players-vs-1-monster model — there is no seat for a second real character
// on the enemy side (see the C2 feasibility note in the engagement plan). So a
// duel does not seat two PCs against each other. Instead each fighter is built
// as their real player Combatant (full kit: weapon, subclass passives, extra
// attacks, race traits) and the *opponent* is synthesised as a monster-shaped
// enemy stat block from their own sheet. That is asymmetric — the "player" side
// runs its full kit while the synthesised enemy is a flat stat block — so a
// single bout would systematically favour whoever is the attacker. The duel
// runs the bout in BOTH orientations and decides on the aggregate HP margin,
// which cancels that edge symmetrically. To-hit is faithful either way (both
// sides roll d20 + AttackBonus vs AC); only damage-per-hit is folded into the
// synthesised enemy's flat Attack. Class-vs-class PvP balance is explicitly out
// of scope (bragging rights, capped stakes).

import (
	"fmt"
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
	duelMinLevel     = 3              // both duellists must be at least this level
	duelAcceptWindow = 24 * time.Hour // an unanswered challenge expires and refunds
	duelStakePerLvl  = 500            // stake cap = challenger level × this
	duelMinStake     = 100            // floor so a "duel" always has something on it
	duelWinnerPct    = 0.70           // winner's cut of the pooled 2×stake; pot rakes the rest
)

// advDuelChallenge is one outstanding duel invitation. Unlike the RPS rival
// challenge it carries no per-round score — a combat duel resolves in a single
// step on accept. The challenger's stake is already escrowed (debited) when the
// row exists; the challenged player's is debited on accept.
type advDuelChallenge struct {
	ChallengeID  string
	ChallengerID id.UserID
	ChallengedID id.UserID
	Stake        int
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// ── Persistence ──────────────────────────────────────────────────────────────

func insertDuelChallenge(c *advDuelChallenge) {
	db.Exec("duel: insert challenge",
		`INSERT INTO adventure_duel_challenges
		 (challenge_id, challenger_id, challenged_id, stake, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		c.ChallengeID, string(c.ChallengerID), string(c.ChallengedID), c.Stake, c.ExpiresAt,
	)
}

func deleteDuelChallenge(challengeID string) {
	db.Exec("duel: delete challenge",
		`DELETE FROM adventure_duel_challenges WHERE challenge_id = ?`, challengeID)
}

// claimDuelChallenge atomically deletes the row and reports whether THIS caller
// is the one that removed it. It is the single serialization point for a
// challenge's terminal fate: accept, decline and the expiry ticker all race to
// claim it, and exactly one wins. Only the winner moves euros — that is what
// stops a stake being both resolved and refunded across the 24h boundary, or a
// double `!duel decline` double-refunding. Mirrors communityPotDebit's
// RowsAffected gate.
func claimDuelChallenge(challengeID string) bool {
	res := db.ExecResult("duel: claim challenge",
		`DELETE FROM adventure_duel_challenges WHERE challenge_id = ?`, challengeID)
	if res == nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func scanDuelChallenge(row interface{ Scan(...any) error }) (*advDuelChallenge, error) {
	c := &advDuelChallenge{}
	err := row.Scan(&c.ChallengeID, &c.ChallengerID, &c.ChallengedID, &c.Stake, &c.ExpiresAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// pendingDuelForChallenged returns the live (unexpired) challenge awaiting this
// player's answer, or nil. Expired rows are ignored here and swept by the
// ticker, so a player is never shown a challenge they can no longer accept.
func pendingDuelForChallenged(userID id.UserID) *advDuelChallenge {
	c := &advDuelChallenge{}
	err := db.Get().QueryRow(`
		SELECT challenge_id, challenger_id, challenged_id, stake, expires_at, created_at
		FROM adventure_duel_challenges
		WHERE challenged_id = ? AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC LIMIT 1`, string(userID)).Scan(
		&c.ChallengeID, &c.ChallengerID, &c.ChallengedID, &c.Stake, &c.ExpiresAt, &c.CreatedAt)
	if err != nil {
		return nil
	}
	return c
}

// duelInvolves reports whether the player has any live challenge in flight, as
// challenger or challenged. Used to enforce one duel at a time per player — the
// escrow makes overlapping challenges a way to double-commit the same euros.
func duelInvolves(userID id.UserID) bool {
	var n int
	db.Get().QueryRow(`
		SELECT COUNT(*) FROM adventure_duel_challenges
		WHERE (challenger_id = ? OR challenged_id = ?) AND expires_at > CURRENT_TIMESTAMP`,
		string(userID), string(userID)).Scan(&n)
	return n > 0
}

func loadExpiredDuelChallenges() []advDuelChallenge {
	rows, err := db.Get().Query(`
		SELECT challenge_id, challenger_id, challenged_id, stake, expires_at, created_at
		FROM adventure_duel_challenges WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []advDuelChallenge
	for rows.Next() {
		if c, err := scanDuelChallenge(rows); err == nil {
			out = append(out, *c)
		}
	}
	return out
}

// ── Stake / gating ───────────────────────────────────────────────────────────

// duelMaxStake caps the stake at level × duelStakePerLvl so a strong player
// can't farm a runaway pot off a weaker one, per the plan's imbalance cap.
func duelMaxStake(level int) int { return level * duelStakePerLvl }

// duelPayout splits the pooled 2×stake: the winner takes duelWinnerPct, the
// community pot rakes the remainder. Both fighters escrowed stake, so the
// winner's net gain is winnerShare − stake and the pot is a real sink.
func duelPayout(stake int) (winnerShare, potShare int) {
	pool := stake * 2
	winnerShare = int(math.Round(float64(pool) * duelWinnerPct))
	potShare = pool - winnerShare
	return
}

// duelEligible reports whether a character can duel at all: alive, set up, and
// at least duelMinLevel. Returns a player-facing reason when it can't.
func (p *AdventurePlugin) duelEligible(char *AdventureCharacter) (int, string) {
	if char == nil {
		return 0, "no adventurer"
	}
	if !char.Alive {
		return 0, "dead"
	}
	lvl := rivalLevelForUser(char)
	if lvl < duelMinLevel {
		return lvl, fmt.Sprintf("below level %d", duelMinLevel)
	}
	return lvl, ""
}

// ── Combat synthesis ─────────────────────────────────────────────────────────

// buildDuelist derives a fighter's full player Combatant — the same build a
// zone encounter uses, so weapon, subclass passives, extra attacks and race
// traits all apply — then resets it to full health. A duel is a fresh arena
// bout, not a continuation of a wounded expedition, so wound carry-over is
// dropped (StartHP=0 = "enter at MaxHP"). Mood 50 is the neutral band: no
// initiative bias, no enemy-attack tilt.
//
// The armed-ability slot is left empty: actively-armed one-shots (a Berserker's
// rage) don't fire, but always-on passives (Extra Attack, Sneak Attack, Divine
// Strike, race traits) still layer in via the class/subclass passive builders.
func (p *AdventurePlugin) buildDuelist(userID id.UserID) (Combatant, error) {
	var noMonster DnDMonsterTemplate
	player, _, _, err := p.buildZoneCombatants(userID, noMonster, 1, 50, "")
	if err != nil {
		return Combatant{}, err
	}
	player.Stats.StartHP = 0
	return player, nil
}

// duelPerHitDamage is a fighter's expected damage on a single landed hit, used
// to size the flat Attack of their synthesised enemy stat block. It mirrors the
// player damage path's terms that the enemy path can't express: weapon dice
// average (or the legacy flat Attack for a weaponless build), the always-on
// per-hit riders (Divine Strike, Sneak Attack, Hunter's Mark, rage), then the
// multiplicative damage bonus. Crit chance and once-per-fight openers are not
// modelled — a coarse mean is enough for a capped-stakes bout decided over two
// orientations.
func duelPerHitDamage(pc Combatant) float64 {
	base := float64(pc.Stats.Attack)
	if w := pc.Stats.Weapon; w != nil {
		if pc.Stats.TwoHandedMode && w.HasProperty(PropVersatile) && w.VersaCount > 0 {
			base = float64(w.VersaCount)*(float64(w.VersaSides)+1)/2 +
				float64(pc.Stats.AbilityModForDamage) + float64(w.MagicBonus)
		} else {
			base = avgWeaponDamage(w, pc.Stats.AbilityModForDamage)
		}
	}
	riders := float64(pc.Mods.DivineStrikePerHit)
	riders += float64(pc.Mods.SneakAttackDie) * 3.5
	riders += float64(pc.Mods.HuntersMarkDie) * 3.5
	if pc.Mods.BerserkerRage {
		riders += float64(pc.Mods.RageMeleeDmg)
	}
	perHit := (base + riders) * (1 + pc.Mods.DamageBonus)
	if perHit < 1 {
		perHit = 1
	}
	return perHit
}

// duelPerRoundProcDamage is the expected per-round damage from a fighter's
// companion strikes — a Cleric's spiritual weapon and a Beastmaster's pet —
// which land once per round on a proc roll rather than per weapon hit. The live
// fighter rolls these natively in the engine; folding their expectation into the
// synth enemy's round damage keeps both orientations comparable for those
// builds (each formula is Dmg + d5, mean +3).
func duelPerRoundProcDamage(pc Combatant) float64 {
	dmg := pc.Mods.SpiritWeaponProc * (float64(pc.Mods.SpiritWeaponDmg) + 3)
	dmg += pc.Mods.PetAttackProc * (float64(pc.Mods.PetAttackDmg) + 3)
	return dmg
}

// synthDuelEnemy turns a fighter's player Combatant into the monster-shaped
// enemy the opposite fighter faces. HP, AC and AttackBonus carry over verbatim
// (to-hit stays faithful — the enemy rolls d20 + AttackBonus vs the attacker's
// AC exactly as the player does). DamageReduct carries over too, so a
// Barbarian's / Monk's flat mitigation is as tanky as the enemy as it is as the
// live PC. Offense is the lossy term: the enemy path deals a flat Attack, so a
// whole round — weapon swings (per-hit × (1 + extra attacks)) plus the expected
// companion-strike damage — is folded into one enemy Attack, preserving expected
// round damage while widening variance (a wash across the two orientations).
//
// Deliberately NOT modelled on the enemy side (accepted scope — the plan frames
// duels as bragging rights and puts class-vs-class PvP balance explicitly out of
// scope): reactive one-shots the enemy engine has no channel for — heal items,
// wards, the Sovereign death-save, the arcane ward buffer — and spell damage,
// which buildZoneCombatants never wires for the live caster either, so a caster
// fights weapon-only in both orientations. Defense is dropped: the weapon-damage
// path players use reads DamageReduct, not the enemy's Defense stat.
func synthDuelEnemy(pc Combatant) Combatant {
	swings := 1 + pc.Mods.ExtraAttacks
	roundDmg := duelPerHitDamage(pc)*float64(swings) + duelPerRoundProcDamage(pc)
	return Combatant{
		Name:     pc.Name,
		IsPlayer: false,
		Stats: CombatStats{
			MaxHP:       pc.Stats.MaxHP,
			AC:          pc.Stats.AC,
			AttackBonus: pc.Stats.AttackBonus,
			Attack:      int(math.Round(roundDmg)),
		},
		Mods: CombatModifiers{DamageReduct: pc.Mods.DamageReduct},
	}
}

// duelResult is the two-orientation verdict. challengerMargin/challengedMargin
// are aggregate remaining-HP fractions across both bouts (0..2 each); the higher
// wins. decisive is true when one fighter won both orientations outright — pure
// narration, the margin is what decides.
type duelResult struct {
	challengerWins bool
	draw           bool
	decisive       bool
	challengerFrac float64
	challengedFrac float64
}

func hpFraction(end, start int) float64 {
	if start <= 0 {
		return 0
	}
	if end < 0 {
		end = 0
	}
	return float64(end) / float64(start)
}

// resolveDuel runs the bout both ways and decides on the aggregate HP margin.
// Orientation 1 seats the challenger as the live fighter against the challenged
// player's stat block; orientation 2 swaps them. Each fighter's margin sums the
// HP fraction they kept as the live fighter in one bout and as the stat block in
// the other, so the attacker's-kit advantage is applied to both fighters once
// and cancels. rng is threaded for the seeded characterization/unit tests; nil
// is production (package-global RNG).
func resolveDuel(challenger, challenged Combatant, rng *rand.Rand) duelResult {
	r1 := simulateCombatWithRNG(challenger, synthDuelEnemy(challenged), defaultCombatPhases, rng)
	r2 := simulateCombatWithRNG(challenged, synthDuelEnemy(challenger), defaultCombatPhases, rng)

	cFrac := hpFraction(r1.PlayerEndHP, r1.PlayerStartHP) + hpFraction(r2.EnemyEndHP, r2.EnemyStartHP)
	dFrac := hpFraction(r1.EnemyEndHP, r1.EnemyStartHP) + hpFraction(r2.PlayerEndHP, r2.PlayerStartHP)

	res := duelResult{challengerFrac: cFrac, challengedFrac: dFrac}
	switch {
	case cFrac > dFrac:
		res.challengerWins = true
	case dFrac > cFrac:
		res.challengerWins = false
	default:
		res.draw = true
	}
	// Decisive = the winner also took both orientations outright.
	if !res.draw {
		if res.challengerWins {
			res.decisive = r1.PlayerWon && !r2.PlayerWon
		} else {
			res.decisive = r2.PlayerWon && !r1.PlayerWon
		}
	}
	return res
}

// ── Command surface ──────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDuelCmd(ctx MessageContext, args string) error {
	args = strings.TrimSpace(args)
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"⚔️ **Duels** — challenge another adventurer to a staked, no-death bout.\n"+
				"`!duel @user [stake]` — throw down · `!duel accept` / `!duel decline` · `!duel status`")
	}
	switch strings.ToLower(fields[0]) {
	case "accept":
		return p.acceptDuel(ctx)
	case "decline":
		return p.declineDuel(ctx)
	case "status":
		return p.duelStatusCmd(ctx)
	default:
		return p.issueDuel(ctx, fields)
	}
}

func (p *AdventurePlugin) issueDuel(ctx MessageContext, fields []string) error {
	targetID, ok := p.ResolveUser(fields[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve that user. Usage: `!duel @user [stake]`")
	}
	if targetID == ctx.Sender {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You can't duel yourself.")
	}

	// Serialise the one-duel gate, balance check and escrow so two rapid `!duel`
	// from the same challenger can't both pass duelInvolves and double-debit.
	lock := p.advUserLock(ctx.Sender)
	lock.Lock()
	defer lock.Unlock()

	challenger, err := loadAdvCharacter(ctx.Sender)
	if err != nil || challenger == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You have no adventurer. Type `!adventure` to create one.")
	}
	lvl, reason := p.duelEligible(challenger)
	if reason != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, duelIneligibleSelf(reason))
	}

	challenged, err := loadAdvCharacter(targetID)
	if err != nil || challenged == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s has no adventurer to duel.", p.DisplayName(targetID)))
	}
	if _, reason := p.duelEligible(challenged); reason != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("%s can't duel right now (%s).", p.DisplayName(targetID), reason))
	}
	if challenged.BabysitActive {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("%s left word with the Babysitting Service — no duels while they're away.", p.DisplayName(targetID)))
	}

	// One duel at a time per player, either side — the escrow makes overlaps a
	// double-commit of the same euros.
	if duelInvolves(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You already have a duel in flight. Resolve it first.")
	}
	if duelInvolves(targetID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s already has a duel in flight.", p.DisplayName(targetID)))
	}

	// Shared 7-day per-pair cooldown across both duel kinds (records table).
	if last := lastDuelBetween(ctx.Sender, targetID); !last.IsZero() && time.Since(last) < rivalSamePairCooldown {
		wait := rivalSamePairCooldown - time.Since(last)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("You've dueled %s too recently. Try again %s.", p.DisplayName(targetID), formatDuration(wait)))
	}

	stake, err := p.parseDuelStake(fields, lvl)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, err.Error())
	}
	if bal := p.euro.GetBalance(ctx.Sender); int(bal) < stake {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("You can't cover a €%d stake — you have €%d.", stake, int(bal)))
	}

	// Escrow the challenger's stake now (debited to nowhere; the pot/winner split
	// redistributes it on resolution, and expiry/decline refunds it).
	if !p.euro.Debit(ctx.Sender, float64(stake), "duel_escrow") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't hold your stake. Try again.")
	}

	ch := &advDuelChallenge{
		ChallengeID:  uuid.NewString(),
		ChallengerID: ctx.Sender,
		ChallengedID: targetID,
		Stake:        stake,
		ExpiresAt:    time.Now().UTC().Add(duelAcceptWindow),
	}
	insertDuelChallenge(ch)

	p.SendDM(targetID, fmt.Sprintf(
		"⚔️ **%s challenges you to a duel!**\n\nStake: €%d each — winner takes 70%% of the pot.\n"+
			"No death, no hospital bill. Just bragging rights.\n\n`!duel accept` or `!duel decline` — you have 24 hours.",
		p.DisplayName(ctx.Sender), stake))

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
		"⚔️ You throw down the gauntlet before **%s** for €%d. They have 24 hours to answer.",
		p.DisplayName(targetID), stake))
}

// parseDuelStake reads the optional stake argument, defaulting to the cap for
// the challenger's level and clamping into [duelMinStake, duelMaxStake].
func (p *AdventurePlugin) parseDuelStake(fields []string, level int) (int, error) {
	maxStake := duelMaxStake(level)
	if len(fields) < 2 {
		return maxStake, nil
	}
	raw := strings.TrimPrefix(fields[1], "€")
	stake := 0
	if _, err := fmt.Sscanf(raw, "%d", &stake); err != nil || stake <= 0 {
		return 0, fmt.Errorf("that's not a valid stake. Usage: `!duel @user [stake]`")
	}
	if stake < duelMinStake {
		return 0, fmt.Errorf("minimum stake is €%d", duelMinStake)
	}
	if stake > maxStake {
		return 0, fmt.Errorf("your stake is capped at €%d (level %d × €%d)", maxStake, level, duelStakePerLvl)
	}
	return stake, nil
}

func (p *AdventurePlugin) acceptDuel(ctx MessageContext) error {
	// Serialise a player's own accept so a double-submit can't debit twice.
	lock := p.advUserLock(ctx.Sender)
	lock.Lock()
	defer lock.Unlock()

	ch := pendingDuelForChallenged(ctx.Sender)
	if ch == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You have no pending duel to accept.")
	}
	// Cheap validation first — these leave the challenge open (the player can
	// revive/earn and retry, or decline to release the challenger's escrow).
	challenged, err := loadAdvCharacter(ctx.Sender)
	if err != nil || challenged == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You have no adventurer.")
	}
	if _, reason := p.duelEligible(challenged); reason != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, duelIneligibleSelf(reason))
	}
	if bal := p.euro.GetBalance(ctx.Sender); int(bal) < ch.Stake {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("You can't cover the €%d stake — you have €%d. `!duel decline` to back out.", ch.Stake, int(bal)))
	}

	// The challenger may have died or dropped below the gate during the 24h
	// window. If so, cancel the duel: claim the row (so the ticker/decline can't
	// also refund) and hand the challenger their escrow back.
	challenger, cerr := loadAdvCharacter(ch.ChallengerID)
	if cerr != nil || challenger == nil {
		if claimDuelChallenge(ch.ChallengeID) {
			p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_challenger_gone")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, "Your challenger is no longer available. The duel is off.")
	}
	if _, reason := p.duelEligible(challenger); reason != "" {
		if claimDuelChallenge(ch.ChallengeID) {
			p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_challenger_ineligible")
			p.SendDM(ch.ChallengerID, fmt.Sprintf(
				"⚔️ Your duel with **%s** was called off — you can't fight right now. Your €%d stake is refunded.",
				p.DisplayName(ctx.Sender), ch.Stake))
		}
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("**%s** can't duel right now (%s). The challenge is off.", p.DisplayName(ch.ChallengerID), reason))
	}

	// Build both fighters BEFORE debiting the challenged player, so the deep
	// sheet-loading / combat code (the panic-prone part) runs while nobody but
	// the challenger is committed. A build failure refunds only the challenger.
	challengerC, err1 := p.buildDuelist(ch.ChallengerID)
	challengedC, err2 := p.buildDuelist(ctx.Sender)
	if err1 != nil || err2 != nil {
		if claimDuelChallenge(ch.ChallengeID) {
			p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_error")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, "The duel couldn't be staged. The challenger's stake is refunded.")
	}

	// Commit: claim the row (exactly one path wins), then take the challenged
	// player's stake. From here both stakes are held and resolution is pure
	// arithmetic — no further build or DB read that could panic mid-payout.
	if !claimDuelChallenge(ch.ChallengeID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "That duel is no longer available.")
	}
	if !p.euro.Debit(ctx.Sender, float64(ch.Stake), "duel_escrow") {
		p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_error")
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't hold your stake. The duel is off.")
	}

	return p.settleDuel(ctx, ch, challengerC, challengedC)
}

func (p *AdventurePlugin) declineDuel(ctx MessageContext) error {
	// Same lock as accept so a player's own accept/decline can't interleave; the
	// claim is the real guard, but the lock keeps the read→claim window tight.
	lock := p.advUserLock(ctx.Sender)
	lock.Lock()
	defer lock.Unlock()

	ch := pendingDuelForChallenged(ctx.Sender)
	if ch == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You have no pending duel to decline.")
	}
	if !claimDuelChallenge(ch.ChallengeID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You have no pending duel to decline.")
	}
	p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_declined")
	p.SendDM(ch.ChallengerID, fmt.Sprintf(
		"🛡️ **%s** declined your duel. Your €%d stake is refunded.", p.DisplayName(ctx.Sender), ch.Stake))
	return p.SendReply(ctx.RoomID, ctx.EventID, "You decline the duel. No harm done.")
}

func (p *AdventurePlugin) duelStatusCmd(ctx MessageContext) error {
	if ch := pendingDuelForChallenged(ctx.Sender); ch != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"⚔️ **%s** has challenged you for €%d. `!duel accept` / `!duel decline` (expires %s).",
			p.DisplayName(ch.ChallengerID), ch.Stake, formatDuration(time.Until(ch.ExpiresAt))))
	}
	// Any challenge we issued that's still pending?
	c := &advDuelChallenge{}
	err := db.Get().QueryRow(`
		SELECT challenge_id, challenger_id, challenged_id, stake, expires_at, created_at
		FROM adventure_duel_challenges
		WHERE challenger_id = ? AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC LIMIT 1`, string(ctx.Sender)).Scan(
		&c.ChallengeID, &c.ChallengerID, &c.ChallengedID, &c.Stake, &c.ExpiresAt, &c.CreatedAt)
	if err == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf(
			"⚔️ You've challenged **%s** for €%d. Waiting on their answer (expires %s).",
			p.DisplayName(c.ChallengedID), c.Stake, formatDuration(time.Until(c.ExpiresAt))))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, "You have no duels in flight. `!duel @user [stake]` to start one.")
}

// ── Resolution ───────────────────────────────────────────────────────────────

// settleDuel resolves the two-orientation bout, splits the pooled 2×stake,
// records the result, and DMs both players. By the time it runs the challenge
// row is claimed, both fighters are built, and both stakes are held — so from
// here it is pure arithmetic and message sends, nothing that can panic and
// strand the pool. On an exact HP-margin tie both stakes are refunded.
func (p *AdventurePlugin) settleDuel(ctx MessageContext, ch *advDuelChallenge, challengerC, challengedC Combatant) error {
	res := resolveDuel(challengerC, challengedC, nil)

	if res.draw {
		// Exact HP-margin tie: refund both, record nothing, no cooldown burned.
		p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_draw")
		p.euro.Credit(ch.ChallengedID, float64(ch.Stake), "duel_refund_draw")
		draw := fmt.Sprintf("⚔️ **%s** and **%s** fought to a standstill. Both stakes refunded.",
			p.DisplayName(ch.ChallengerID), p.DisplayName(ch.ChallengedID))
		p.SendDM(ch.ChallengerID, draw)
		p.SendDM(ch.ChallengedID, draw)
		return p.SendReply(ctx.RoomID, ctx.EventID, draw)
	}

	winnerID, loserID := ch.ChallengedID, ch.ChallengerID
	if res.challengerWins {
		winnerID, loserID = ch.ChallengerID, ch.ChallengedID
	}

	winnerShare, potShare := duelPayout(ch.Stake)
	p.euro.Credit(winnerID, float64(winnerShare), "duel_win")
	if potShare > 0 {
		communityPotAdd(potShare)
		// The rake came out of a pool both funded; attribute it half each.
		trackTaxPaid(winnerID, potShare/2)
		trackTaxPaid(loserID, potShare-potShare/2)
	}

	upsertRivalRecord(winnerID, loserID, true)
	upsertRivalRecord(loserID, winnerID, false)

	// One BULLETIN dispatch per settled duel (from the winner's side, so it
	// fires once). Character names only; no-op unless the seam is enabled.
	if wn, ln := charName(winnerID), charName(loserID); wn != "" && ln != "" {
		ts := nowUnix()
		disc := fmt.Sprintf("rival:%d", ts)
		emitFact(peteclient.Fact{
			GUID:       fmt.Sprintf("rival_result:%s:%s:%d", eventToken(winnerID, disc), eventToken(loserID, disc), ts),
			EventType:  "rival_result",
			Tier:       "bulletin",
			Subject:    wn,
			Opponent:   ln,
			Outcome:    "won",
			OccurredAt: ts,
		}, winnerID, loserID)
	}

	p.announceDuel(ctx, winnerID, loserID, ch.Stake, winnerShare, potShare, res.decisive)
	return nil
}

func (p *AdventurePlugin) announceDuel(ctx MessageContext, winnerID, loserID id.UserID, stake, winnerShare, potShare int, decisive bool) {
	winName, loseName := p.DisplayName(winnerID), p.DisplayName(loserID)
	flair := pickDuelFlavor(duelWinFlavor)
	if decisive {
		flair = pickDuelFlavor(duelDecisiveFlavor)
	}

	p.SendDM(winnerID, fmt.Sprintf(
		"⚔️ **You beat %s!**\n\n*%s*\n\nYou take €%d (net +€%d). €%d went to the community pot.",
		loseName, flair, winnerShare, winnerShare-stake, potShare))
	p.SendDM(loserID, fmt.Sprintf(
		"⚔️ **%s bested you.**\n\n*%s*\n\nYou're down €%d — but you walk away, no worse for wear.",
		winName, pickDuelFlavor(duelLossFlavor), stake))

	// Broadcast the result to the games room for bragging rights — unless the
	// duel was accepted there, in which case the SendReply below already covers
	// it and a second line would just be noise.
	if gr := gamesRoom(); gr != "" && gr != ctx.RoomID {
		verb := "defeated"
		if decisive {
			verb = "decisively defeated"
		}
		p.SendMessage(gr, fmt.Sprintf("⚔️ **%s** %s **%s** in a duel for €%d.", winName, verb, loseName, stake))
	}
	// Confirm in the room the challenge was accepted from too.
	p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("⚔️ **%s** wins the duel, taking €%d.", winName, winnerShare))
}

// expireDuelChallenges refunds and clears any challenge whose 24h window lapsed.
// Rides the 1-minute event ticker (no new goroutine); only the challenger was
// escrowed, so only they are refunded.
func (p *AdventurePlugin) expireDuelChallenges() {
	for _, ch := range loadExpiredDuelChallenges() {
		// Claim before refunding: if an accept/decline already took this row at
		// the 24h boundary, we must not also refund the challenger.
		if !claimDuelChallenge(ch.ChallengeID) {
			continue
		}
		p.euro.Credit(ch.ChallengerID, float64(ch.Stake), "duel_refund_expired")
		p.SendDM(ch.ChallengerID, fmt.Sprintf(
			"⌛ **%s** never answered your duel. Your €%d stake is refunded.",
			p.DisplayName(ch.ChallengedID), ch.Stake))
	}
}

func duelIneligibleSelf(reason string) string {
	switch reason {
	case "dead":
		return "You can't duel while dead. Visit `!hospital` first."
	case "no adventurer":
		return "You have no adventurer. Type `!adventure` to create one."
	default:
		return fmt.Sprintf("You need to be at least level %d to duel.", duelMinLevel)
	}
}

// ── Flavor ───────────────────────────────────────────────────────────────────

var duelWinFlavor = []string{
	"Steel rang, and the crowd knew its champion.",
	"A clean finish — you left them nothing to answer with.",
	"They fought well. You fought better.",
}

var duelDecisiveFlavor = []string{
	"It was over before it began. Utterly one-sided.",
	"You never gave them a foothold. A rout.",
	"The arena barely had time to hush.",
}

var duelLossFlavor = []string{
	"You'll want that one back. Next time.",
	"A hard lesson — but only your pride is bleeding.",
	"Close, in places. Not close enough.",
}

func pickDuelFlavor(pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}
