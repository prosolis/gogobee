package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// The Hollow King campaign (N5/D1). A light serialized story threaded through
// the zones by way of collectible journal pages. "The Hollow King" is already
// the Forest Shadows (T2) boss; this frames him as a realm-spanning antagonist
// whose fragments turn up wherever players fight elites and open secret rooms.
//
// The fragments are in-world found artifacts — journal entries, torn letters,
// stone inscriptions — not TwinBee's voice. TwinBee's own reactions (D1b) obey
// the first-person voice rules; this text does not.

type journalPage struct {
	Title string
	Text  string
}

// journalPages is the ordered campaign. Reading the discovered pages top to
// bottom tells the fall of a kingdom to the thing its king became. Order is the
// story order; players find them out of sequence, which is why the viewer marks
// gaps.
var journalPages = []journalPage{
	{"The Long Winter", "A crown is only a circle of gold until a man decides never to take it off. Ours decided in a winter that would not end, and the frost took its cue from him."},
	{"The Last Physician", "The court healers were sent home one by one, each promising the king had years left. The last was not sent home. We heard him thank the king for the honour."},
	{"On Shadows", "He stopped casting a shadow before he stopped casting a reflection. The steward struck both from the list of things a servant may notice aloud."},
	{"The Quiet Ledger", "Grain still left the granaries; no mouths were fed. I stopped auditing the difference the night the number began to feel like a name."},
	{"A Bargain Overheard", "Through the chapel door: the king's voice, and a second that used his own words a breath before he did. Only one of them was asking."},
	{"The Hollowing", "They call it a coronation in the records. Those of us who carried the braziers call it what it was. A king was emptied so a crown could keep wearing him."},
	{"The First Knights", "His honour guard did not die. That was the mercy offered and the price paid. They stand at the old gate yet, and they still salute a throne no living thing sits on."},
	{"Letters Home, Unsent", "\"Tell mother the pay is good and the work is guarding.\" The satchel held forty such letters, every hand different, every promise the same, none of them sent."},
	{"The Map Redrawn", "The kingdom did not fall so much as come apart at the seams — a warren here, a drowned temple there, each piece keeping a splinter of him like a tooth in a wound."},
	{"Root and Rot", "The forest north of the manor grew wrong and grew fast. The woodsmen say the trees lean toward the ruin at dusk. The woodsmen no longer go at dusk."},
	{"The Warren Below", "Even the goblins gave the deep tunnels to him and asked nothing back. When a scavenger yields ground for free, ask what it saw down there."},
	{"Water That Remembers", "The temple sank in a single night with the bells still ringing. Divers say the bells ring still, slow, as if something below is counting."},
	{"The Manor's Long Guest", "Blackspire changed hands nine times in a decade. Every deed names a different owner. Every household names the same tenant, and none will write it down."},
	{"The Forge Unbanked", "The underforge keeps a heat with no fuel and no smith. What it makes, no one has seen leave. What it makes, we are told, was promised elsewhere long ago."},
	{"Descent", "The deep roads were a trade route once. Now they are a throat. Everything the surface loses is swallowed the same direction, and the direction has a door at the end."},
	{"The Bright Country", "Past the crossing the colours are too kind and the days too long. It is the most beautiful place I have run from. He is patient there; he can afford to be."},
	{"What the Dragon Keeps", "The wyrm hoards more than gold. Deep in the lair, behind the coin, a single crown sits on no head and is guarded better than the hoard."},
	{"The Portal's Arithmetic", "The abyss gate opens outward. Everyone assumes a door lets things in. This one was built by someone who only ever intended to leave through it."},
	{"The Regent's Confession", "I ruled in his name for thirty years and never once saw him rule. I signed what the crown wanted signed. I am writing this so that one honest page exists."},
	{"The Names of the Guard", "I have set down every knight's name here so that when this is read, they are grieved as men and not feared as things. It is the only rescue left to attempt."},
	{"The Flaw in the Bargain", "The second voice took the king's life and his death both — and a thing that cannot die also cannot be finished. He is not immortal. He is unpaid, and waiting to collect."},
	{"How to Call Him", "He answers only where his fragments gather and only to one who has gathered them. Do not do this to avenge us. Do it to end the account. Come with the whole ledger or do not come."},
	{"The Empty Throne", "I have seen the seat at the heart of it all. It is not empty. It is occupied by the shape of a man who left, kept warm against his return."},
	{"Last Page", "If you are reading in order, you have walked the ruin of everything he emptied to stay. One page is missing from every telling — the one you write by going in. Bring a light. He hates the light. It is the one thing he could never hollow out."},
}

// journalTotalPages is the campaign length; the drop/viewer/finale all read it
// so the story can grow by appending to journalPages alone.
var journalTotalPages = len(journalPages)

// journalPageDropChance is the per-elite-kill probability that a page turns up.
// Deliberately modest: 24 pages is a long-horizon collection, and secret rooms
// (D4) grant pages on top of this. Tunable.
const journalPageDropChance = 0.22

// setJournalPageBit is the in-memory twin of grantJournalPageDB's bitwise OR —
// the single definition of "page N lives in bit N-1". Out-of-range pages are a
// no-op.
func setJournalPageBit(mask int64, page int) int64 {
	if page < 1 || page > 63 {
		return mask
	}
	return mask | (int64(1) << (page - 1))
}

func journalPageFound(mask int64, page int) bool {
	if page < 1 || page > 63 {
		return false
	}
	return mask&(int64(1)<<(page-1)) != 0
}

func journalPageCount(mask int64) int {
	n := 0
	for i := 1; i <= journalTotalPages; i++ {
		if journalPageFound(mask, i) {
			n++
		}
	}
	return n
}

func journalComplete(mask int64) bool {
	return journalPageCount(mask) >= journalTotalPages
}

// pickUnfoundJournalPage returns a random not-yet-found page number, or 0 when
// the campaign is already complete.
func pickUnfoundJournalPage(mask int64, rng *rand.Rand) int {
	var missing []int
	for i := 1; i <= journalTotalPages; i++ {
		if !journalPageFound(mask, i) {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return 0
	}
	return missing[rngIntN(rng, len(missing))]
}

// maybeDropJournalPage rolls a page reward for an elite kill or secret room and,
// on a hit, grants a random unfound page and returns its narration line. Empty
// string means no drop (missed the roll, DB error, or campaign already
// complete). The roll draws from the same RNG the surrounding loot rolls use
// and never touches SimulateCombat's stream, so the combat golden is unmoved.
func (p *AdventurePlugin) maybeDropJournalPage(userID id.UserID, rng *rand.Rand) string {
	if rngFloat(rng) >= journalPageDropChance {
		return ""
	}
	return p.grantJournalPage(userID, rng)
}

// grantJournalPage grants a random unfound page unconditionally (used by secret
// rooms, which award a page for certain). Returns "" when already complete or on
// error.
func (p *AdventurePlugin) grantJournalPage(userID id.UserID, rng *rand.Rand) string {
	mask, err := loadJournalPages(userID)
	if err != nil {
		return ""
	}
	page := pickUnfoundJournalPage(mask, rng)
	if page == 0 {
		return ""
	}
	if err := grantJournalPageDB(userID, page); err != nil {
		return ""
	}
	// If the page turned up mid-expedition, drop a log beat so the end-of-day
	// digest can have TwinBee react to it. No expedition (legacy !zone, or a
	// secret room opened outside a run) simply means no digest to react in.
	if exp, err := getActiveExpedition(userID); err == nil && exp != nil {
		_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "journal",
			journalPages[page-1].Title, fmt.Sprintf("page %d", page))
	}
	return fmt.Sprintf("📖 A torn journal page — _%s_ (page %d of %d). See `!adventure journal`.",
		journalPages[page-1].Title, page, journalTotalPages)
}

// bossEpilogues ties each zone boss's death to the Hollow King arc: a 2-3
// sentence capstone appended to the boss-down moment. Forest of Shadows is the
// King himself — but what falls there is a shell he shed, which is why the arc
// (and the finale) outlives it. In-world narration, not TwinBee's voice.
var bossEpilogues = map[ZoneID]string{
	ZoneGoblinWarrens: "Grol dies clutching a coin no goblin minted — a king's face worn smooth by handling. Whatever paid the warren to give up its deep tunnels, it paid in a currency older than these hills.",
	ZoneCryptValdris: "Valdris tried to cheat the grave and managed only to furnish it. In his last rattle he says a name that isn't his — _hollow, hollow_ — as if warning you of a colleague who did it better.",
	ZoneForestShadows: "The Hollow King falls without weight, a coat slipped from its peg — and the woods do not go quiet. What you felled here was a thing he shed, not the thing he is. Somewhere, the account he owes goes on accruing.",
	ZoneSunkenTemple: "The Aboleth's dream breaks and the drowned bells still at last. In the silence you understand what they were counting toward — and that the count did not begin with this temple, and does not end with it.",
	ZoneManorBlackspire: "Aldric was hollowed the same way, by the same hand, and made a poor imitation: a lord kept past his death to hold a house for a guest who never came. He thanks you. It is the first thing he has meant in a century.",
	ZoneUnderforge: "Thyrak's fires gutter out, and the half-made things on the anvils cool into what they were always going to be — regalia, and soldiers, and a crown with no head to fit. The forge was filling an order placed a long time ago.",
	ZoneUnderdark: "Ilvaras ruled the throat that swallows everything downward, toward the door at the bottom of the world. She dies certain she served a queen. She served a direction, and the direction has a name it never told her.",
	ZoneFeywildCrossing: "The Thornmother's garden was the loveliest cage on the road, tended for a patient guest. He can afford patience; you are learning why. She wilts, and the too-kind light dims by exactly one degree.",
	ZoneDragonsLair: "Behind Infernax's hoard, past the last of the gold, a single crown rests on no head — guarded better than the treasure, because it was the one thing here he was ever paid to keep. The dragon dies never knowing what it was.",
	ZoneAbyssPortal: "Belaxath guarded a door that opens outward, built by someone who only ever meant to leave through it. As the demon falls, the gate does not close. It was never meant to keep things out — only to let one thing come home.",
}

// bossEpilogueLine returns the campaign capstone for a zone boss, or "" for
// zones with none (and for the synthetic arena, which has no ZoneID entry).
func bossEpilogueLine(zoneID ZoneID) string {
	return bossEpilogues[zoneID]
}

// twinBeeJournalReactions are TwinBee's morning/digest reactions to pages found
// during the day — first-person, implicit subject, he/him, one line, curious,
// never expository (feedback_twinbee_voice, feedback_twinbee_is_male). Picked
// deterministically so a re-rendered digest reads the same.
var twinBeeJournalReactions = []string{
	"📖 Found a torn page in your kit tonight — been reading it by the fire while you sleep. This king of theirs was not a well man.",
	"📖 Another page. Keep turning them up and I keep piecing him together, and I do not much like the shape.",
	"📖 Read the new page twice. Whoever wrote it was frightened of something patient. I think we are walking toward it.",
	"📖 Slipped the day's page into the others. The story's filling in at the edges, and none of the edges are kind.",
}

// twinBeeJournalReaction picks one reaction line deterministically from the day
// and the number of pages found, so the digest is stable across re-renders.
func twinBeeJournalReaction(day, pagesToday int) string {
	if len(twinBeeJournalReactions) == 0 || pagesToday <= 0 {
		return ""
	}
	idx := (day + pagesToday) % len(twinBeeJournalReactions)
	if idx < 0 {
		idx = -idx
	}
	return twinBeeJournalReactions[idx]
}

// handleJournalCmd renders the player's collected campaign pages.
func (p *AdventurePlugin) handleJournalCmd(ctx MessageContext) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character. Try `!adventure` to create one first.")
	}
	return p.SendDM(ctx.Sender, renderJournal(char.JournalPages))
}

// renderJournal builds the `!adventure journal` view: discovered pages in story
// order, with runs of missing pages collapsed to a single "…".
func renderJournal(mask int64) string {
	found := journalPageCount(mask)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📖 **The Hollow King** — Journal (%d / %d pages)\n",
		found, journalTotalPages))

	if found == 0 {
		b.WriteString("\nYou carry no pages yet. They surface where the realm's fragments gather — in the hands of elites, and behind doors most adventurers walk past.")
		return b.String()
	}

	b.WriteString("\n")
	gapOpen := false
	for i := 1; i <= journalTotalPages; i++ {
		if journalPageFound(mask, i) {
			gapOpen = false
			jp := journalPages[i-1]
			b.WriteString(fmt.Sprintf("\n**%s. %s**\n%s\n", romanNumeral(i), jp.Title, jp.Text))
		} else if !gapOpen {
			gapOpen = true
			b.WriteString("\n…\n")
		}
	}

	if journalComplete(mask) {
		b.WriteString("\nThe ledger is whole. Clear both Tier-5 zones and you can follow him to the end — `!expedition start epilogue`.")
	} else {
		b.WriteString(fmt.Sprintf("\n_%d pages still scattered._", journalTotalPages-found))
	}
	return b.String()
}

// romanNumeral renders 1..24 as upper-case Roman numerals for the page headers.
// The campaign never exceeds a couple dozen pages, so a small table beats a
// general algorithm here.
func romanNumeral(n int) string {
	if n < 1 || n > len(romanNumerals) {
		return fmt.Sprintf("%d", n)
	}
	return romanNumerals[n-1]
}

// ── D1c: the finale ─────────────────────────────────────────────────────────

// finaleTitle is the unique role awarded for the first finale clear.
const finaleTitle = "Kingsbane"

const epilogueEntryFlavor = "You bring the whole ledger to the Empty Throne. The shape kept warm against his return stirs, and stands, and is finally, fully here."

// hollowKingFinaleMonster is the finale stat block: Belaxath's block — the abyss
// boss whose gate "was meant to let one thing come home" — re-dressed as the
// King returned in full, with HP bumped for a capstone. A variant stat block
// with no new mechanics, per the plan. Built on the fly (not registered in
// dndBestiary) because runZoneCombat takes the template directly, exactly as the
// arena bosses do.
func hollowKingFinaleMonster() DnDMonsterTemplate {
	m := dndBestiary["boss_belaxath"]
	m.ID = "boss_hollow_king_finale"
	m.Name = "The Hollow King, Unhoused"
	m.HP = int(float64(m.HP) * 1.25)
	m.Notes = "Finale encounter — the Hollow King returned in full through the abyss gate. Variant of the Balor stat block; no new mechanics."
	return m
}

// epilogueUnlocked gates the finale on the whole ledger plus both Tier-5 clears.
func (p *AdventurePlugin) epilogueUnlocked(char *AdventureCharacter) (bool, string) {
	if !journalComplete(char.JournalPages) {
		return false, fmt.Sprintf(
			"The trail runs cold. You've recovered **%d of %d** journal pages — find them all before you can follow him to the end. (`!adventure journal`)",
			journalPageCount(char.JournalPages), journalTotalPages)
	}
	if !clearedEveryZoneOfTier(db.Get(), char.UserID, ZoneTier(5)) {
		return false, "You hold the whole ledger, but not the standing to use it. Clear **both** Tier-5 zones — the Dragon's Lair and the Abyss Portal — then come back to the throne."
	}
	return true, ""
}

// handleEpilogueEncounter runs the finale: a single auto-resolved boss fight
// against the Hollow King's true form, reached via `!expedition start epilogue`.
// Reward drops only on the first clear (reward-once); later clears are a
// flavour-only rematch. A loss costs a death, as any boss fight does.
func (p *AdventurePlugin) handleEpilogueEncounter(ctx MessageContext) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character. Try `!adventure` first.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're in no shape to face him. Mend at `!hospital` first.")
	}
	if ok, why := p.epilogueUnlocked(char); !ok {
		return p.SendDM(ctx.Sender, why)
	}
	// Busy guards — the finale is a synchronous auto-resolved fight, so refuse if
	// anything else is in flight and could collide on HP or run state.
	if exp, _ := getActiveExpedition(ctx.Sender); exp != nil {
		return p.SendDM(ctx.Sender, "Finish your expedition before you walk to the Empty Throne.")
	}
	if run, _ := getActiveZoneRun(ctx.Sender); run != nil {
		return p.SendDM(ctx.Sender, "Finish your current zone run first.")
	}
	if sess, _ := getActiveCombatSession(ctx.Sender); sess != nil {
		return p.SendDM(ctx.Sender, "You're already in a fight. Finish it first.")
	}

	monster := hollowKingFinaleMonster()
	displayName, _ := loadDisplayName(ctx.Sender)
	if displayName == "" {
		displayName = "You"
	}

	preHP, _ := dndHPSnapshot(ctx.Sender)
	result, err := p.runZoneCombat(ctx.Sender, monster, 5, bossCombatPhases, 50)
	if err != nil {
		slog.Error("epilogue: combat failed", "user", ctx.Sender, "err", err)
		return p.SendDM(ctx.Sender, "Something went wrong at the throne. Try again in a moment.")
	}
	postHP, maxHP := dndHPSnapshot(ctx.Sender)
	nat20s, nat1s := countNat20sAnd1s(result)

	intro := fmt.Sprintf("👑 **The Empty Throne — %s** (HP %d, AC %d)\n_%s_",
		monster.Name, monster.HP, monster.AC, epilogueEntryFlavor)
	phases := RenderCombatLog(result, displayName, monster.Name)
	outcome := renderBossOutcome(BossOutcomeInputs{
		// Synthetic ZoneArena keeps twinBeeLine's deterministic pickers off any
		// real zone's pool, exactly as the arena does.
		ZoneID:          ZoneArena,
		RunID:           "epilogue-" + string(ctx.Sender),
		RoomIdx:         0,
		Monster:         monster,
		Result:          result,
		PreHP:           preHP,
		PostHP:          postHP,
		MaxHP:           maxHP,
		PhaseTwoAt:      0.40,
		Nat20s:          nat20s,
		Nat1s:           nat1s,
		DefeatHeadline:  "💀 The Hollow King folds you into the quiet he keeps. The account goes unpaid a while longer.",
		VictoryHeadline: fmt.Sprintf("🏆 **The Hollow King falls — and this time stays down.** You finished at **%d/%d HP**.", postHP, maxHP),
	})

	var tail string
	if !result.PlayerWon {
		markAdventureDead(ctx.Sender, "adventure", "the Empty Throne")
	} else {
		tail = p.finishEpilogueWin(ctx.Sender, char.EpilogueCleared)
	}

	_ = p.sendZoneCombatMessages(ctx.Sender, append([]string{intro}, phases...), joinLootLines(outcome, tail))
	return nil
}

// finishEpilogueWin grants the reward on a first clear and returns the reward
// narration; a repeat clear is flavour only. The character is reloaded before
// the title write so runZoneCombat's post-fight HP persist isn't clobbered.
func (p *AdventurePlugin) finishEpilogueWin(userID id.UserID, alreadyCleared bool) string {
	if alreadyCleared {
		return "_The throne stays empty. You came to be sure. You are sure._"
	}

	var b strings.Builder
	b.WriteString("🎖️ **The account is closed.** You are named **" + finaleTitle + "** — the one who unhoused the Hollow King.\n")
	if mi, ok := pickMagicItemForRarity(RarityLegendary, nil); ok {
		if line := p.dropMagicItemLoot(userID, mi, LootTierLegendary); line != "" {
			b.WriteString(line + "\n")
		}
	} else {
		slog.Error("epilogue: no legendary in registry", "user", userID)
	}

	if fresh, err := loadAdvCharacter(userID); err == nil && fresh != nil {
		if fresh.Title != finaleTitle {
			fresh.Title = finaleTitle
			if err := saveAdvCharacter(fresh); err != nil {
				slog.Error("epilogue: title save failed", "user", userID, "err", err)
			}
		}
	}
	if err := markEpilogueClearedDB(userID); err != nil {
		slog.Error("epilogue: mark cleared failed", "user", userID, "err", err)
	}

	if gr := gamesRoom(); gr != "" {
		if dn, _ := loadDisplayName(userID); dn != "" {
			p.SendMessage(id.RoomID(gr), fmt.Sprintf(
				"👑 **%s** carried the whole ledger to the Empty Throne and closed the account. The Hollow King is ended. **%s.**",
				dn, finaleTitle))
		}
	}
	return b.String()
}

var romanNumerals = []string{
	"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X",
	"XI", "XII", "XIII", "XIV", "XV", "XVI", "XVII", "XVIII", "XIX", "XX",
	"XXI", "XXII", "XXIII", "XXIV", "XXV", "XXVI", "XXVII", "XXVIII", "XXIX", "XXX",
}
