package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Pet XP & Leveling ──────────────────────────────────────────────────────

const petXPPerAction = 1.5

// petMaxLevel is where the curve stops. Named because two things read it for
// different reasons: the level-up loop stops here, and the web push reports "no
// more to earn" from it.
const petMaxLevel = 10

var petNameValid = regexp.MustCompile(`^[a-zA-Z0-9 '\-]+$`)

// petXPToNextLevel returns XP needed for a given pet level.
func petXPToNextLevel(level int) int {
	switch {
	case level < 3:
		return 10
	case level < 5:
		return 20
	case level < 8:
		return 35
	default:
		return 50
	}
}

// petXPNeededCenti is petXPToNextLevel in the unit the stored ledger actually
// uses — centi-XP — and 0 once the pet is capped, so a caller can tell "nothing
// left to earn" from "needs another 10 points". Every comparison against a
// stored PetXP multiplies by 100 (see advancePetLevelsFromXP); anything reading
// the curve for display has to do the same, and doing it in one place is how the
// web push avoids getting it wrong.
func petXPNeededCenti(level int) int {
	if level >= petMaxLevel {
		return 0
	}
	return petXPToNextLevel(level) * 100
}

// petGrantXP adds a per-action XP grant to the pet and handles level-ups.
// Returns true if leveled up. Shares the level-up loop with the babysit trickle
// via advancePetLevelsFromXP.
func petGrantXP(pet *PetState) bool {
	if !pet.HasPet() {
		return false
	}
	return advancePetLevelsFromXP(&pet.XP, &pet.Level, &pet.Level10Date, int(petXPPerAction*100))
}

// grantPetCombatXP pays both pet slots their per-action XP for a fight the
// player won, and returns the names of any pet that leveled so the caller can
// narrate it.
//
// This is the pet's only *earned* XP source. It used to ride the legacy daily
// activity loop, which R1 deleted — and for the whole life of Adventure 2.0
// nothing replaced it, leaving petGrantXP orphaned and every un-babysat pet
// frozen at the level it was adopted with. Pet level is not cosmetic
// (DerivePlayerStats scales PetAttackProc / PetDeflectProc / PetAttackDmg off
// it), so a frozen pet is a permanently dead combat slot.
//
// Both slots earn on the same win, matching the babysit trickle: combat only
// ever reads the two pets' *averaged* procs, so leveling both is not a spike.
//
// Writes go through the narrow per-slot pet upserts rather than
// saveAdvCharacter: this runs on the combat close-out path, which does not
// hold the per-user lock, and a full-row write from here could clobber a
// concurrent character save.
func grantPetCombatXP(userID id.UserID) []string {
	var leveled []string
	slots := []struct {
		n      int
		load   func(id.UserID) (PetState, error)
		upsert func(id.UserID, PetState) error
	}{
		{1, loadPetState, upsertPlayerMetaPetState},
		{2, loadPet2State, upsertPlayerMetaPet2State},
	}
	for _, s := range slots {
		pet, err := s.load(userID)
		if err != nil || !pet.HasPet() {
			continue
		}
		didLevel := petGrantXP(&pet)
		if uerr := s.upsert(userID, pet); uerr != nil {
			slog.Error("adventure: pet xp persist", "user", userID, "slot", s.n, "err", uerr)
			continue
		}
		if didLevel {
			leveled = append(leveled, fmt.Sprintf("%s reached level %d", pet.Name, pet.Level))
		}
	}
	return leveled
}

// advancePetLevelsFromXP adds centi-XP to a pet and applies any level-ups, up
// to the level-10 cap, stamping the level-10 date on first reaching it. Shared
// by both pet slots (the babysit trickle). Returns true if the pet leveled.
func advancePetLevelsFromXP(xp, level *int, level10Date *string, addCentiXP int) bool {
	if *level >= petMaxLevel {
		return false
	}
	*xp += addCentiXP
	leveled := false
	for *level < petMaxLevel {
		needed := petXPNeededCenti(*level)
		if *xp < needed {
			break
		}
		*xp -= needed
		*level++
		leveled = true
	}
	if *level >= petMaxLevel && *level10Date == "" {
		*level10Date = time.Now().UTC().Format("2006-01-02")
	}
	return leveled
}

// ── Pet Combat Action Probabilities ────────────────────────────────────────

// Base probabilities scale with pet level.
// Attack: 3% + 1.5% per level (max 18% at L10)
// Deflect: 2% + 1% per level (max 12% at L10) + armor bonus
// Ditch recovery: 5% + 2% per level (max 25% at L10)

func petAttackChance(level int) float64 {
	return 0.03 + float64(level)*0.015
}

func petDeflectChance(level, armorTier int) float64 {
	base := 0.02 + float64(level)*0.01
	defs := petDogArmor // same bonuses for both types
	for _, d := range defs {
		if d.Tier == armorTier {
			base += d.DeflectBonus
			break
		}
	}
	return base
}

func petDitchRecoveryChance(level int) float64 {
	return 0.05 + float64(level)*0.02
}

// petDitchRecoveryTime returns the death timer based on pet level.
// Level 1: 5h, Level 5: ~3.5h, Level 10: 1h (linear interpolation).
func petDitchRecoveryTime(level int) time.Duration {
	hours := 5.0 - float64(level-1)*4.0/9.0 // 5h at L1, 1h at L10
	if hours < 1 {
		hours = 1
	}
	return time.Duration(hours * float64(time.Hour))
}

// ── Pet Combat Results ─────────────────────────────────────────────────────

type PetCombatResult struct {
	Attacked      bool
	AttackDamage  int
	AttackText    string
	Deflected     bool
	DeflectText   string
	DitchRecovery bool
	DitchText     string
	VictoryText   string
	DeathText     string
}

// petRollCombatActions rolls attack and deflect for a combat round.
func petRollCombatActions(pet PetState, enemyName string) *PetCombatResult {
	if !pet.HasPet() {
		return nil
	}

	result := &PetCombatResult{}

	// Attack roll
	if rand.Float64() < petAttackChance(pet.Level) {
		result.Attacked = true
		result.AttackDamage = 3 + rand.IntN(5) + pet.Level // 3-7 + level
		var pool []string
		if pet.Type == "dog" {
			pool = PetDogAttack
		} else {
			pool = PetCatAttack
		}
		line := pool[rand.IntN(len(pool))]
		line = strings.ReplaceAll(line, "{enemy}", enemyName)
		line = strings.ReplaceAll(line, "{damage}", fmt.Sprintf("%d", result.AttackDamage))
		result.AttackText = line
	}

	// Deflect roll
	if rand.Float64() < petDeflectChance(pet.Level, pet.ArmorTier) {
		result.Deflected = true
		var pool []string
		if pet.Type == "dog" {
			pool = PetDogDeflect
		} else {
			pool = PetCatDeflect
		}
		line := pool[rand.IntN(len(pool))]
		line = strings.ReplaceAll(line, "{enemy}", enemyName)
		result.DeflectText = line
	}

	return result
}

// petRollDitchRecovery rolls for pet intervention on player death.
func petRollDitchRecovery(pet PetState) bool {
	if !pet.HasPet() {
		return false
	}
	return rand.Float64() < petDitchRecoveryChance(pet.Level)
}

// petVictoryText returns a random victory reaction line.
func petVictoryText(pet PetState) string {
	if !pet.HasPet() {
		return ""
	}
	var pool []string
	if pet.Type == "dog" {
		pool = PetDogVictory
	} else {
		pool = PetCatVictory
	}
	return pool[rand.IntN(len(pool))]
}

// petDeathText returns a random death reaction line.
func petDeathText(pet PetState) string {
	if !pet.HasPet() {
		return ""
	}
	var pool []string
	if pet.Type == "dog" {
		pool = PetDogDeath
	} else {
		pool = PetCatDeath
	}
	return pool[rand.IntN(len(pool))]
}

// ── Pet Arrival Flow ───────────────────────────────────────────────────────

// petShouldArrive checks if pet arrival should trigger.
// Fires randomly after Tier 1 house upgrade is complete. Housing and pet
// state come from player_meta via loadHouseState / loadPetState (L4d/L4e
// reader flip).
func petShouldArrive(pet PetState, house HouseState) bool {
	if pet.Arrived {
		return false
	}
	// Pet arrives after Tier 1 (Livable) upgrade = HouseTier >= 2
	if house.Tier < 2 {
		return false
	}
	// Pet was chased away and reactivated via Misty — allow re-arrival
	if pet.ChasedAway {
		return pet.Reactivated
	}
	// 30% daily chance after house tier 1 (~3 days average to arrival)
	return rand.Float64() < 0.30
}

// petShouldArrive2 gates the SECOND companion (N4/E1). It needs a Tier-4 Estate
// and an established first pet — the second animal wanders in to join the first,
// not an empty house. A chased-away second pet does not re-arrive (Misty's
// reactivation is a pet-1 mechanic).
func petShouldArrive2(pet1, pet2 PetState, house HouseState) bool {
	if pet2.Arrived || pet2.ChasedAway {
		return false
	}
	if house.Tier < 4 {
		return false
	}
	if !pet1.HasPet() {
		return false
	}
	return rand.Float64() < 0.30
}

// maybeRollPetArrivalOnEmerge rolls the pet-arrival check when a player
// surfaces from an expedition (voluntary extract, abandon, or a survived
// forced extraction) or revives after death. The arrival roll lives on the
// emergence seam — not the legacy 08:00 overworld morning DM — because
// expedition players are almost never in the overworld at the scheduled hour,
// so the morning roll never reached them. Story beat: while the player was
// underground, an animal wandered into the empty house looking for food.
//
// Safe to call unconditionally on any emergence: petShouldArrive gates on
// house tier / not-yet-arrived, and petArrivalDM won't clobber an existing
// pending interaction.
func (p *AdventurePlugin) maybeRollPetArrivalOnEmerge(userID id.UserID) {
	pet, _ := loadPetState(userID)
	house, _ := loadHouseState(userID)
	if petShouldArrive(pet, house) {
		p.petArrivalDM(userID, 1)
		return
	}
	// A Tier-4 Estate can draw a second companion once the first is settled.
	pet2, _ := loadPet2State(userID)
	if petShouldArrive2(pet, pet2, house) {
		p.petArrivalDM(userID, 2)
	}
}

// petArrivalDM sends the initial "there's an animal in your house" DM for the
// given slot (1 or 2).
func (p *AdventurePlugin) petArrivalDM(userID id.UserID, slot int) {
	// Don't overwrite an existing pending interaction
	if _, exists := p.pending.Load(string(userID)); exists {
		return
	}

	intro := "There's an animal in your house. It looks like a..."
	if slot == 2 {
		intro = "There's *another* animal in your house. Your first pet seems unbothered. It looks like a..."
	}
	text := intro + "\n\n" +
		"🚪 Chase it away\n" +
		"🍖 Feed it\n\n" +
		"Reply with `chase` or `feed`."

	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "pet_arrival",
		Data:      &advPendingPetArrival{Slot: slot},
		ExpiresAt: time.Now().Add(advDMResponseWindowLow),
	})
	_ = p.SendDM(userID, text)
}

// petSlotFromData reads a Slot off any pending-pet payload, defaulting to slot 1
// for the zero value so older/first-pet interactions are unaffected.
func petSlotFromData(slot int) int {
	if slot == 2 {
		return 2
	}
	return 1
}

// resolvePetArrival handles the chase/feed response.
func (p *AdventurePlugin) resolvePetArrival(ctx MessageContext, interaction *advPendingInteraction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	slot := petSlotFromData(interaction.Data.(*advPendingPetArrival).Slot)
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	if reply == "chase" || reply == "🚪" {
		if slot == 2 {
			char.Pet2ChasedAway = true
			char.Pet2Reactivated = false
		} else {
			char.PetChasedAway = true
			char.PetReactivated = false
		}
		_ = saveAdvCharacter(char)
		return p.SendDM(ctx.Sender, "You chased it away. It disappeared around the corner and didn't come back.\n\nThe house is quiet again.")
	}

	if reply == "feed" || reply == "🍖" {
		// Ask what type
		text := "It ate everything you put down. It's still here.\n\n" +
			"It looks like a...\n\n" +
			"🐶 Massive Dog\n" +
			"🐱 Massive Cat\n\n" +
			"Reply with `dog` or `cat`."

		p.pending.Store(string(ctx.Sender), &advPendingInteraction{
			Type:      "pet_type",
			Data:      &advPendingPetType{Slot: slot},
			ExpiresAt: time.Now().Add(advDMResponseWindowLow),
		})
		return p.SendDM(ctx.Sender, text)
	}

	// Invalid response — re-prompt
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "pet_arrival",
		Data:      &advPendingPetArrival{Slot: slot},
		ExpiresAt: time.Now().Add(advDMResponseWindowLow),
	})
	return p.SendDM(ctx.Sender, "Reply with `chase` or `feed`.")
}

// resolvePetType handles dog/cat selection.
func (p *AdventurePlugin) resolvePetType(ctx MessageContext, interaction *advPendingInteraction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	slot := petSlotFromData(interaction.Data.(*advPendingPetType).Slot)
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	petType := ""
	if reply == "dog" || reply == "🐶" {
		petType = "dog"
	} else if reply == "cat" || reply == "🐱" {
		petType = "cat"
	}

	if petType == "" {
		p.pending.Store(string(ctx.Sender), &advPendingInteraction{
			Type:      "pet_type",
			Data:      &advPendingPetType{Slot: slot},
			ExpiresAt: time.Now().Add(advDMResponseWindowLow),
		})
		return p.SendDM(ctx.Sender, "Reply with `dog` or `cat`.")
	}

	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "pet_name",
		Data:      &advPendingPetName{PetType: petType, Slot: slot},
		ExpiresAt: time.Now().Add(advDMResponseWindowLow),
	})

	article := "a"
	if petType == "dog" {
		article = "a"
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("It's %s Massive %s. What would you like to name it?\n\nReply with a name.",
		article, titleCase(petType)))
}

// resolvePetName handles naming the pet. The dispatcher (handlePendingReply)
// has already deleted the pending interaction and passes it in, so this must
// NOT re-load from p.pending — doing so previously made every adoption fail
// silently (the carried PetType was lost and the save never ran).
func (p *AdventurePlugin) resolvePetName(ctx MessageContext, interaction *advPendingInteraction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	data := interaction.Data.(*advPendingPetName)

	name := strings.TrimSpace(ctx.Body)
	if len(name) == 0 || len(name) > 30 {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Name must be 1-30 characters. Try again.")
	}
	if !petNameValid.MatchString(name) {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Name can only contain letters, numbers, spaces, hyphens, and apostrophes. Try again.")
	}

	if petSlotFromData(data.Slot) == 2 {
		char.Pet2Type = data.PetType
		char.Pet2Name = name
		char.Pet2Arrived = true
		char.Pet2ChasedAway = false
		char.Pet2Level = 1
		char.Pet2XP = 0
	} else {
		char.PetType = data.PetType
		char.PetName = name
		char.PetArrived = true
		char.PetChasedAway = false
		char.PetLevel = 1
		char.PetXP = 0
	}

	if err := saveAdvCharacter(char); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save.")
	}

	emoji := "🐶"
	if data.PetType == "cat" {
		emoji = "🐱"
	}
	tail := fmt.Sprintf("%s will join you in combat. %s will not explain their decisions.", name, name)
	if petSlotFromData(data.Slot) == 2 {
		tail = fmt.Sprintf("%s fights alongside your first companion — the two share the spotlight, so together they're about as much help in a scrap as one seasoned pet. %s will not explain their decisions.", name, name)
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("%s **%s** has moved in.\n\nBreed: Massive %s.\nLevel: 1\n\n%s",
		emoji, name, titleCase(data.PetType), tail))
}

// ── Morning Pet Events ─────────────────────────────────────────────────────

// petMorningEvent rolls for a morning pet event and returns flavor text.
// Returns empty string if no event fires.
func petMorningEvent(pet PetState) string {
	if !pet.HasPet() {
		return ""
	}

	// 25% chance of morning event
	if rand.Float64() >= 0.25 {
		return ""
	}

	if pet.Type == "cat" {
		line := PetCatOffering[rand.IntN(len(PetCatOffering))]
		return strings.ReplaceAll(line, "{pet_name}", pet.Name)
	}

	line := PetDogSmothering[rand.IntN(len(PetDogSmothering))]
	return strings.ReplaceAll(line, "{pet_name}", pet.Name)
}

// ── Pet Supply Shop Unlock Check ───────────────────────────────────────────

// petCheckSupplyShopUnlock checks if 1 week has passed since pet hit level 10.
func petCheckSupplyShopUnlock(pet PetState) bool {
	if pet.SupplyShopUnlocked || pet.Level10Date == "" {
		return false
	}
	d, err := time.Parse("2006-01-02", pet.Level10Date)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(d.Add(7 * 24 * time.Hour))
}

// ── Misty Housing Hint ─────────────────────────────────────────────────────

// mistyHousingHint returns a hint about housing/pets if conditions are met.
// Fires once, after 2+ Misty encounters, player has no house. Housing
// state comes from player_meta (L4e reader flip). Misty NPC counters still
// live on AdvCharacter (NPC migration is a later phase) so the caller
// passes them in directly.
func mistyHousingHint(mistyEncounterCount, mistyDonatedCount int, house HouseState) string {
	if house.HasHouse() || mistyEncounterCount < 2 {
		return ""
	}
	if mistyDonatedCount > 0 {
		return "You seem like a good person. I can imagine you sitting in a house caring for a pet someday."
	}
	return "Thank you for your time."
}

// ── Misty Pet Reactivation ─────────────────────────────────────────────────

// mistyReactivatePet returns true when a chased-away pet should be marked
// as reactivated. Caller is responsible for flipping the flag on both
// AdvCharacter and player_meta.
func mistyReactivatePet(pet PetState) bool {
	return pet.ChasedAway && !pet.Reactivated
}

// ── Pet Level 10 Ticker (runs daily at midnight) ───────────────────────────

func (p *AdventurePlugin) petMidnightCheck() {
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("pet: failed to load characters", "err", err)
		return
	}

	for _, char := range chars {
		if !char.HasPet() {
			continue
		}

		// Check supply shop unlock
		pet, _ := loadPetState(char.UserID)
		if petCheckSupplyShopUnlock(pet) {
			char.PetSupplyShopUnlocked = true
			_ = saveAdvCharacter(&char)
			_ = upsertPlayerMetaPetState(char.UserID, petStateFromAdvChar(&char))
			slog.Info("pet: supply shop unlocked", "user", char.UserID, "pet", char.PetName)
		}
	}
}

// ── Ditch Recovery Text ────────────────────────────────────────────────────

func petDitchRecoveryGameRoom(username, petName string, petInvolved bool) string {
	base := fmt.Sprintf("🏥 %s was brought into St. Guildmore's on a stretcher. They have been returned to the ditch. They'll be fine eventually.", username)
	if petInvolved {
		base = fmt.Sprintf("🏥 St. Guildmore's notes that %s has once again intervened before our personnel could reach the scene. %s has been reminded that the ditch is not a medical facility. %s did not respond to this reminder.", petName, petName, petName)
	}
	return base
}
