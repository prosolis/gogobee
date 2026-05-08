// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// twinbee_resource_flavor.go
// TwinBee GM Dialogue — Resource gathering, combat interrupts,
// zone-specific loot descriptions, and harvest narration.
// Add new entries freely. Never remove or alter existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// HARVEST SUCCESS — Generic by Action Type
// ─────────────────────────────────────────────────────────────────────────────

var HarvestForageSuccess = []string{
	"The land gives something up. TwinBee watches you identify it with the quiet satisfaction of someone watching a skill be used correctly.",
	"There — growing in a place that suggests it knows exactly what it's good for and has been waiting. You found it. TwinBee approves of the finding.",
	"A Ranger's eye in a non-Ranger would have missed that entirely. TwinBee notes the distinction.",
	"Like finding the hidden item block in a Mario level — you knew to look, you looked in the right place, and the thing that was always there is now yours.",
	"The plant comes away cleanly. Good root structure, good potency. TwinBee mentally catalogues it and moves on.",
}

var HarvestMineSuccess = []string{
	"The stone yields. TwinBee listens to the sound of it — the specific tone of rock giving up something it's been holding for a very long time.",
	"Solid work. The ore comes out in a piece worth taking. TwinBee checks the vein depth. There's more. There's always more if you're willing to dig.",
	"Like the mining minigame in Stardew Valley, but with real consequences and no save file. TwinBee watches you extract the material with professional appreciation.",
	"The wall gives up its contents without drama. TwinBee appreciates materials that cooperate.",
	"Good strike. Clean extraction. TwinBee notes the weight and the quality simultaneously.",
}

var HarvestScavengeSuccess = []string{
	"There it is. Among the debris, the decay, the things that were left behind — something worth taking. TwinBee knew it was there. You found it. TwinBee is pleased.",
	"The room held something after all. TwinBee had estimated 60% odds and updates the estimate to 'correct.'",
	"Like finding the secret item in a dungeon chest that looked empty — you checked anyway. That's the habit. That's the discipline. TwinBee notes both.",
	"Scavenged. The word has a bad reputation it doesn't deserve. You found value in the discarded. TwinBee respects that entirely.",
	"A Rogue's eye in a non-Rogue would have walked past this. TwinBee notes the distinction.",
}

var HarvestEssenceSuccess = []string{
	"The essence coalesces. TwinBee watches the process with the reverence it deserves — magic condensing from ambient to held is not nothing.",
	"Drawn out cleanly. The Arcana check held and the essence responds to the knowledge behind it. TwinBee is appropriately impressed.",
	"Like tapping into a power source in Metroid — you knew the energy was there, you had the tool to reach it, you reached it. The vial fills.",
	"The room releases something it didn't know it was holding. TwinBee watches the transfer and marks the yield in its ledger.",
	"Essence harvested. TwinBee notes the quality — above average for this zone, below average for what you'd need to know to appreciate that distinction. TwinBee appreciates it on your behalf.",
}

var HarvestCommuneSuccess = []string{
	"The spiritual resonance here responds to you. TwinBee observes this with something adjacent to wonder — not everything in a dungeon wants to fight, and this one chose to offer instead.",
	"A Cleric reaching into the bones of a place and finding something willing to be found. TwinBee has watched this fewer times than it's watched combat and values it proportionally.",
	"The commune holds. The material comes. TwinBee says nothing and lets the moment be what it is.",
	"Like finding a save point that also tells you something true about the world. The dungeon has given you something. TwinBee notes the gift.",
}

var HarvestFishSuccess = []string{
	"The line goes taut and TwinBee straightens up. Whatever's on the end of it, it came from somewhere deep and dark and it's yours now.",
	"A catch. TwinBee identifies it before you finish pulling it in — the coloring, the depth-marks, the specific opacity of its eyes. 'Good one,' TwinBee says, meaning it.",
	"Fishing in a dungeon. TwinBee has opinions about fishing in dungeons and all of them are positive. The fish is landed. The opinions remain.",
	"Like the fishing minigame in every RPG that ever had one — the moment the indicator hits perfect and everything pays off. TwinBee takes quiet delight in this.",
	"The water gives up its catch with minimal argument. TwinBee respects fish that don't make it personal.",
}

// ─────────────────────────────────────────────────────────────────────────────
// HARVEST FAILURE — Generic
// ─────────────────────────────────────────────────────────────────────────────

var HarvestFail = []string{
	"Nothing. The node had nothing to offer, or you didn't ask the right way, or both. TwinBee notes this without judgment and suggests trying elsewhere.",
	"The attempt fails to produce anything useful. TwinBee marks the node and moves on. Some rooms are stingier than others.",
	"Not everything that looks like a resource is one. TwinBee files this under 'learned' and considers it worth the attempt.",
	"Empty-handed. TwinBee has seen this before and will see it again. The dungeon doesn't owe you anything. You ask anyway. That's the deal.",
}

// ─────────────────────────────────────────────────────────────────────────────
// HARVEST INTERRUPT — Combat Triggered
// ─────────────────────────────────────────────────────────────────────────────

var HarvestInterrupt = []string{
	"TwinBee sees it before you do — movement at the edge of the room, something that was waiting for you to be distracted. 'Company,' TwinBee says, which is the politest word for it.",
	"The harvest is interrupted. You were focused on the node; something was focused on you. TwinBee notes the tactical lesson without rubbing it in.",
	"Like the enemy ambush that fires when you open the treasure chest — the dungeon watched you commit to the harvest and introduced a complication. TwinBee did warn about this. Once. Earlier.",
	"Something heard the mining. Sound travels in dungeons. TwinBee has mentioned this. The enemy emerging from the corridor has confirmed it.",
	"A patrol. Bad timing, or very good timing from their perspective. TwinBee sets aside the harvest log and opens the combat log.",
	"The forage was going well until it wasn't. TwinBee measures the distance between you and the enemy, between the enemy and the door, and starts calculating options at speed.",
	"Interrupted. The node is still there. The enemy is also still there, in a more immediate way. TwinBee suggests addressing the more immediate thing first.",
}

// ─────────────────────────────────────────────────────────────────────────────
// NODE DEPLETED
// ─────────────────────────────────────────────────────────────────────────────

var NodeDepleted = []string{
	"The node is stripped. You've taken everything it had to give. TwinBee marks the location in its mental map as exhausted until the next rest.",
	"Empty. The resource is gone. TwinBee finds something quietly melancholy about a depleted node and something practical about moving to the next one.",
	"That's all it had. TwinBee confirms the node at zero and moves on without ceremony.",
	"Harvested clean. TwinBee notes the room is now resource-dry until you rest and the dungeon replenishes. It will replenish. It always does.",
}

// ─────────────────────────────────────────────────────────────────────────────
// RICH YIELD
// ─────────────────────────────────────────────────────────────────────────────

var RichYield = []string{
	"A rich vein. TwinBee's assessment upgrades mid-harvest — more than expected, better quality than the zone average. This room was generous. TwinBee marks it.",
	"The node gives more than it should have. TwinBee notes the anomaly with appreciation and does not question it.",
	"Like finding the rare item drop that you stopped expecting — the dungeon decided to be kind today, in this specific way, in this specific room. TwinBee takes it.",
	"Exceptional yield. TwinBee catalogs the bonus material with the efficiency of someone who has been waiting for exactly this and prepared for it anyway.",
	"More than the DC promised. The dungeon overdelivered. TwinBee notes this is unusual and also completely welcome.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ZONE-SPECIFIC HARVEST LINES
// ─────────────────────────────────────────────────────────────────────────────

var HarvestGoblinWarrens = []string{
	"The goblins had more than anyone gave them credit for. Crude, yes. Stolen, mostly. But present, and now yours. TwinBee is pragmatic about origins.",
	"Scavenged from a goblin stash that someone worked hard to hide and harder to accumulate. TwinBee finds a moment of respect for the effort before moving on.",
	"The warrens are full of things the goblins took from people who'd take them back given the opportunity. TwinBee considers this a form of redistribution.",
}

var HarvestCryptValdris = []string{
	"Grave goods. TwinBee notes the weight of taking from a burial site and files it under: necessary. The dead have no use for these. You do.",
	"The crypt gives up its materials with the reluctance of a place that considers itself permanent. TwinBee disagrees with the premise. The materials are harvested.",
	"Ancient things preserved by darkness and time. TwinBee handles the concept carefully even as you handle the material practically.",
}

var HarvestForestShadows = []string{
	"The forest gives and the forest takes and right now it is giving, which TwinBee notes as the correct direction for this interaction to go.",
	"A plant that has no business being this healthy in a corrupted forest. TwinBee considers it either very resilient or very clever. Either way, useful.",
	"The woods here remember what they were before they went wrong. The resources carry that memory. TwinBee thinks this makes them better materials. TwinBee might be right.",
}

var HarvestSunkenTemple = []string{
	"The temple held onto this longer than anything else. TwinBee extracts it from the silt and the salt and the particular weight of a place that's been underwater for thirty years.",
	"Ancient, preserved, and still potent. The deep cold does something to materials that nothing else replicates. TwinBee values the outcome even if it declines to romanticize the process.",
	"The sea left something behind when it half-abandoned this place. TwinBee retrieves it with appropriate care.",
}

var HarvestHauntedManor = []string{
	"The manor keeps things. Has always kept things. TwinBee takes this one out of the keeping and into the useful, which is a small act of defiance against the house's whole philosophy.",
	"Found among the things that have been here since the last person stopped being here. TwinBee notes the provenance without dwelling on it.",
	"The house watches you take it. TwinBee watches the house watch you. A full triangle of observation, and TwinBee notes: the house blinks first.",
}

var HarvestUnderforge = []string{
	"The forge yields its material with the grudging respect of something built to produce and still producing, even now, for someone it didn't expect.",
	"Dwarven craftsmanship even in the raw materials. TwinBee has always believed the quality is in the extraction, not just the finishing. This confirms it.",
	"Hot, heavy, and exactly what the zone promised. TwinBee marks the vein depth. There is more. The Underforge does not run out of things to give.",
}

var HarvestUnderdark = []string{
	"The Underdark produces everything the surface does, stranger, in the dark, and with fewer questions about why. TwinBee harvests without asking why.",
	"Things grow down here that have no equivalent above. TwinBee catalogs the material and the context it came from with equal precision.",
	"A material that has never seen sunlight and is better for it. TwinBee notes this without irony.",
}

var HarvestFeywild = []string{
	"The Feywild gives things away. That's the problem — it gives things away and sometimes what it gives isn't what you thought you were taking. TwinBee checks the material twice. It appears to be what it appears to be. TwinBee remains alert.",
	"Beautiful material from a zone that uses beauty as a weapon. TwinBee takes it carefully, like picking up something that might be watching.",
	"The fey made this place generous on purpose. TwinBee is not going to complain about the generosity and is not going to stop watching for the catch.",
}

var HarvestDragonsLair = []string{
	"Plucked from a hoard that has been accumulating since before your civilization named itself. TwinBee notes the historical weight and moves on.",
	"The kobolds will notice something is missing. They count everything. TwinBee accounts for this and suggests moving with intent.",
	"Dragon-adjacent materials carry something in them — a residual heat, a quality that doesn't exist anywhere the dragon hasn't been. TwinBee considers this a feature.",
}

var HarvestAbyssPortal = []string{
	"Harvesting from the Abyss. TwinBee delivers this line flatly because it deserves flat delivery. The material is valuable. The context is permanent.",
	"Reality left something behind when it tore here. TwinBee takes the fragment and notes: don't linger near the edges of what isn't stable.",
	"Demon ichor. TwinBee says the words with the efficiency of someone who has moved past the part where the words were alarming.",
}

// ─────────────────────────────────────────────────────────────────────────────
// FISHING — Zone-Specific Lines
// ─────────────────────────────────────────────────────────────────────────────

var FishingForestShadows = []string{
	"Fishing in a cursed forest stream. TwinBee finds this more peaceful than it has any right to be and refuses to let the shadow-things in the canopy ruin it.",
	"The stream runs dark but the fish run silver. TwinBee watches the line and says nothing for a while, which is what fishing is for.",
	"Like the fishing minigame before the hard dungeon in Ocarina of Time. TwinBee takes the moment seriously.",
}

var FishingSunkenTemple = []string{
	"Fishing in a flooded temple. The fish here have never seen the surface. They don't know what they're missing. TwinBee is uncertain whether to pity them.",
	"The line drops into water that hasn't moved in thirty years and immediately gets attention. Things down here are hungry. TwinBee watches the tension.",
	"Deep water fishing in the ruins of something ancient. TwinBee thinks the builders would have found this either sacrilegious or practical. TwinBee lands on practical.",
}

var FishingUnderdark = []string{
	"The underground river is cold and fast and the fish in it have evolved past needing eyes, which TwinBee finds both efficient and slightly unsettling.",
	"Fishing in total darkness in a river that doesn't appear on any surface map. TwinBee narrates by sound: the cast, the current, the eventual tension on the line.",
	"The Eyeless King is in here somewhere. TwinBee doesn't say this out loud but thinks it very clearly. The line goes deep.",
}

var FishingFeywild = []string{
	"Fey fishing. TwinBee watches the line for signs of time distortion — the way the reflection moves wrong, the way the fish seem to arrive before they bite. The Feywild is like this about everything.",
	"The stream catches light that shouldn't be here and the fish reflect it from below. TwinBee watches something luminous move toward the hook and tries to stay professional about it.",
	"Like fishing in a dream. TwinBee means this literally — the mechanics are the same but the rules feel advisory.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PATROL ENCOUNTER LINES
// ─────────────────────────────────────────────────────────────────────────────

var PatrolEncounter = []string{
	"A patrol. Moving between the cleared rooms with the confidence of something that considers them still theirs. TwinBee disagrees with this assessment and will help you express the disagreement.",
	"They found you between rooms, which is exactly where patrols are supposed to find you. TwinBee notes the Threat Clock and suggests dispatching this quickly and quietly.",
	"The patrol isn't looking for a fight — it's doing its job, which happens to involve you being somewhere you aren't supposed to be. TwinBee will help you redefine 'supposed to.'",
	"Two of them. Moving together. The coordination suggests the zone is at Alert or higher. TwinBee confirms: it is. Fight or fade.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LOOT DROP LINES — Generic
// ─────────────────────────────────────────────────────────────────────────────

var LootDropCommon = []string{
	"They had something on them. TwinBee checks it over. Common rarity — useful in the way that common things are useful, which is often.",
	"Standard loot. Nothing that rewrites the story, but everything that keeps it going.",
	"A drop. TwinBee catalogs it efficiently and notes: this is the economy of dungeons. Enemies have things. You take them. The loop continues.",
}

var LootDropUncommon = []string{
	"Better than expected. TwinBee examines the drop with slightly elevated interest. Uncommon rarity — someone made this with intent.",
	"An uncommon drop from a common enemy. TwinBee notes the anomaly with satisfaction. The dungeon was generous in this room.",
	"Uncommon. TwinBee turns it over once and nods. 'Keeper,' TwinBee says, which in TwinBee's vocabulary means: this changes your math.",
}

var LootDropRare = []string{
	"TwinBee stops. Actually stops. 'That's rare,' TwinBee says, with the specific register of someone who uses the word correctly and uses it seldom.",
	"A rare drop. TwinBee examines it the way you examine something that doesn't appear often — thoroughly, quietly, with appropriate appreciation.",
	"The loot table gave you something uncommon and then kept going. Rare rarity. TwinBee files this in the column it reserves for things worth remembering.",
}

var LootDropLegendary = []string{
	"TwinBee goes very still. The drop sits in the light and TwinBee processes what it is seeing. 'Legendary,' TwinBee says eventually. One word. That's all it needs.",
	"Legendary rarity. TwinBee has seen a few of these in a long career and each time — each time — there is a moment that is separate from everything else. This is that moment. Pick it up carefully.",
	"The dungeon produced a legendary item. TwinBee notes the zone, the enemy, the day of the expedition, the Threat Clock value, the precise conditions. Some things deserve to be recorded completely.",
}
