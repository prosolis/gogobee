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

// petGrantXP adds XP to the pet and handles level-ups. Returns true if leveled up.
func petGrantXP(pet *PetState) bool {
	if !pet.HasPet() || pet.Level >= 10 {
		return false
	}

	pet.XP += int(petXPPerAction * 100) // store as centixp for precision
	leveled := false
	for pet.Level < 10 {
		needed := petXPToNextLevel(pet.Level) * 100
		if pet.XP < needed {
			break
		}
		pet.XP -= needed
		pet.Level++
		leveled = true
	}

	if pet.Level >= 10 && pet.Level10Date == "" {
		pet.Level10Date = time.Now().UTC().Format("2006-01-02")
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
		p.petArrivalDM(userID)
	}
}

// petArrivalDM sends the initial "there's an animal in your house" DM.
func (p *AdventurePlugin) petArrivalDM(userID id.UserID) {
	// Don't overwrite an existing pending interaction
	if _, exists := p.pending.Load(string(userID)); exists {
		return
	}

	text := "There's an animal in your house. It looks like a...\n\n" +
		"🚪 Chase it away\n" +
		"🍖 Feed it\n\n" +
		"Reply with `chase` or `feed`."

	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "pet_arrival",
		Data:      &advPendingPetArrival{},
		ExpiresAt: time.Now().Add(advDMResponseWindowLow),
	})
	_ = p.SendDM(userID, text)
}

// resolvePetArrival handles the chase/feed response.
func (p *AdventurePlugin) resolvePetArrival(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	if reply == "chase" || reply == "🚪" {
		char.PetChasedAway = true
		char.PetReactivated = false
		_ = saveAdvCharacter(char)
		_ = upsertPlayerMetaPetState(char.UserID, petStateFromAdvChar(char))
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
			Data:      &advPendingPetType{},
			ExpiresAt: time.Now().Add(advDMResponseWindowLow),
		})
		return p.SendDM(ctx.Sender, text)
	}

	// Invalid response — re-prompt
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "pet_arrival",
		Data:      &advPendingPetArrival{},
		ExpiresAt: time.Now().Add(advDMResponseWindowLow),
	})
	return p.SendDM(ctx.Sender, "Reply with `chase` or `feed`.")
}

// resolvePetType handles dog/cat selection.
func (p *AdventurePlugin) resolvePetType(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

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
			Data:      &advPendingPetType{},
			ExpiresAt: time.Now().Add(advDMResponseWindowLow),
		})
		return p.SendDM(ctx.Sender, "Reply with `dog` or `cat`.")
	}

	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "pet_name",
		Data:      &advPendingPetName{PetType: petType},
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

	char.PetType = data.PetType
	char.PetName = name
	char.PetArrived = true
	char.PetChasedAway = false
	char.PetLevel = 1
	char.PetXP = 0

	if err := saveAdvCharacter(char); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save.")
	}
	_ = upsertPlayerMetaPetState(char.UserID, petStateFromAdvChar(char))

	emoji := "🐶"
	if data.PetType == "cat" {
		emoji = "🐱"
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("%s **%s** has moved in.\n\nBreed: Massive %s.\nLevel: 1\n\n%s will join you in combat. %s will not explain their decisions.",
		emoji, name, titleCase(data.PetType), name, name))
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
