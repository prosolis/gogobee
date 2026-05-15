// twinbee_resource_flavor.go
// TwinBee GM Dialogue — Resource gathering, combat interrupts,
// zone-specific loot descriptions, and harvest narration.
//
// Voice convention (Phase B2): TwinBee speaks in first-person or implicit
// subject. No third-person "TwinBee [verb]" lines. Add freely.
// Don't shorten existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// HARVEST SUCCESS — Generic by Action Type
// ─────────────────────────────────────────────────────────────────────────────

var HarvestForageSuccess = []string{
	"The land gives something up. I watch you identify it with the quiet satisfaction of someone watching a skill be used correctly.",
	"There — growing in a place that suggests it knows exactly what it's good for and has been waiting. You found it. I approve of the finding.",
	"A Ranger's eye in a non-Ranger would have missed that entirely. I note the distinction.",
	"Like finding the hidden item block in a Mario level — you knew to look, you looked in the right place, and the thing that was always there is now yours.",
	"The plant comes away cleanly. Good root structure, good potency. Mentally catalogued. Moving on.",
}

var HarvestMineSuccess = []string{
	"The stone yields. I listen to the sound of it — the specific tone of rock giving up something it's been holding for a very long time.",
	"Solid work. The ore comes out in a piece worth taking. I check the vein depth. There's more. There's always more if you're willing to dig.",
	"Like the mining minigame in Stardew Valley, but with real consequences and no save file. I watch you extract the material with professional appreciation.",
	"The wall gives up its contents without drama. I appreciate materials that cooperate.",
	"Good strike. Clean extraction. I note the weight and the quality simultaneously.",
}

var HarvestScavengeSuccess = []string{
	"There it is. Among the debris, the decay, the things that were left behind — something worth taking. I knew it was there. You found it. Pleased.",
	"The room held something after all. I had estimated 60% odds and am updating the estimate to 'correct.'",
	"Like finding the secret item in a dungeon chest that looked empty — you checked anyway. That's the habit. That's the discipline. I note both.",
	"Scavenged. The word has a bad reputation it doesn't deserve. You found value in the discarded. I respect that entirely.",
	"A Rogue's eye in a non-Rogue would have walked past this. I note the distinction.",
}

var HarvestEssenceSuccess = []string{
	"The essence coalesces. I watch the process with the reverence it deserves — magic condensing from ambient to held is not nothing.",
	"Drawn out cleanly. The Arcana check held and the essence responds to the knowledge behind it. I'm appropriately impressed.",
	"Like tapping into a power source in Metroid — you knew the energy was there, you had the tool to reach it, you reached it. The vial fills.",
	"The room releases something it didn't know it was holding. I watch the transfer and mark the yield in the ledger.",
	"Essence harvested. Quality above average for this zone, below average for what you'd need to know to appreciate that distinction. I appreciate it on your behalf.",
}

var HarvestCommuneSuccess = []string{
	"The spiritual resonance here responds to you. I observe this with something adjacent to wonder — not everything in a dungeon wants to fight, and this one chose to offer instead.",
	"A Cleric reaching into the bones of a place and finding something willing to be found. I've watched this fewer times than I've watched combat and value it proportionally.",
	"The commune holds. The material comes. I say nothing and let the moment be what it is.",
	"Like finding a save point that also tells you something true about the world. The dungeon has given you something. I note the gift.",
}

var HarvestFishSuccess = []string{
	"The line goes taut and I straighten up. Whatever's on the end of it, it came from somewhere deep and dark and it's yours now.",
	"A catch. I identify it before you finish pulling it in — the coloring, the depth-marks, the specific opacity of its eyes. 'Good one,' I say, meaning it.",
	"Fishing in a dungeon. I have opinions about fishing in dungeons and all of them are positive. The fish is landed. The opinions remain.",
	"Like the fishing minigame in every RPG that ever had one — the moment the indicator hits perfect and everything pays off. Quiet delight, on my end.",
	"The water gives up its catch with minimal argument. I respect fish that don't make it personal.",
}

// ─────────────────────────────────────────────────────────────────────────────
// HARVEST FAILURE — Generic
// ─────────────────────────────────────────────────────────────────────────────

var HarvestFail = []string{
	"Nothing. The node had nothing to offer, or you didn't ask the right way, or both. No judgment from me. Try elsewhere.",
	"The attempt fails to produce anything useful. I mark the node and move on. Some rooms are stingier than others.",
	"Not everything that looks like a resource is one. Filed under 'learned' and considered worth the attempt.",
	"Empty-handed. I've seen this before and will see it again. The dungeon doesn't owe you anything. You ask anyway. That's the deal.",
}

// ─────────────────────────────────────────────────────────────────────────────
// HARVEST INTERRUPT — Combat Triggered
// ─────────────────────────────────────────────────────────────────────────────

var HarvestInterrupt = []string{
	"I see it before you do — movement at the edge of the room, something that was waiting for you to be distracted. 'Company,' I say, which is the politest word for it.",
	"The harvest is interrupted. You were focused on the node; something was focused on you. I note the tactical lesson without rubbing it in. Much.",
	"Like the enemy ambush that fires when you open the treasure chest — the dungeon watched you commit to the harvest and introduced a complication. I did warn about this. Once. Earlier.",
	"Something heard the mining. Sound travels in dungeons. I have mentioned this. The enemy emerging from the corridor has confirmed it.",
	"A patrol. Bad timing, or very good timing from their perspective. I set aside the harvest log and open the combat log.",
	"The forage was going well until it wasn't. I measure the distance between you and the enemy, between the enemy and the door, and start calculating options at speed.",
	"Interrupted. The node is still there. The enemy is also still there, in a more immediate way. I suggest addressing the more immediate thing first.",
}

// ─────────────────────────────────────────────────────────────────────────────
// NODE DEPLETED
// ─────────────────────────────────────────────────────────────────────────────

var NodeDepleted = []string{
	"The node is stripped. You've taken everything it had to give. Marked in the mental map as exhausted until the next rest.",
	"Empty. The resource is gone. Something quietly melancholy about a depleted node and something practical about moving to the next one.",
	"That's all it had. I confirm the node at zero and move on without ceremony.",
	"Harvested clean. The room is now resource-dry until you rest and the dungeon replenishes. It will replenish. It always does.",
}

// ─────────────────────────────────────────────────────────────────────────────
// RICH YIELD
// ─────────────────────────────────────────────────────────────────────────────

var RichYield = []string{
	"A rich vein. My assessment upgrades mid-harvest — more than expected, better quality than the zone average. This room was generous. Marked.",
	"The node gives more than it should have. I note the anomaly with appreciation and decline to question it.",
	"Like finding the rare item drop that you stopped expecting — the dungeon decided to be kind today, in this specific way, in this specific room. I take it. We take it. We do not look it in the mouth.",
	"Exceptional yield. I catalog the bonus material with the efficiency of someone who's been waiting for exactly this and prepared for it anyway.",
	"More than the DC promised. The dungeon overdelivered. Unusual. Also completely welcome.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ZONE-SPECIFIC HARVEST LINES
// ─────────────────────────────────────────────────────────────────────────────

var HarvestGoblinWarrens = []string{
	"The goblins had more than anyone gave them credit for. Crude, yes. Stolen, mostly. But present, and now yours. I'm pragmatic about origins.",
	"Scavenged from a goblin stash that someone worked hard to hide and harder to accumulate. A moment of respect for the effort, then we move on.",
	"The warrens are full of things the goblins took from people who'd take them back given the opportunity. I consider this a form of redistribution.",
}

var HarvestCryptValdris = []string{
	"Grave goods. The weight of taking from a burial site, filed under: necessary. The dead have no use for these. You do.",
	"The crypt gives up its materials with the reluctance of a place that considers itself permanent. I disagree with the premise. The materials are harvested.",
	"Ancient things preserved by darkness and time. I handle the concept carefully even as you handle the material practically.",
}

var HarvestForestShadows = []string{
	"The forest gives and the forest takes and right now it is giving, which I note as the correct direction for this interaction to go.",
	"A plant that has no business being this healthy in a corrupted forest. Either very resilient or very clever. Either way, useful.",
	"The woods here remember what they were before they went wrong. The resources carry that memory. I think this makes them better materials. I might be right.",
}

var HarvestSunkenTemple = []string{
	"The temple held onto this longer than anything else. I extract it from the silt and the salt and the particular weight of a place that's been underwater for thirty years.",
	"Ancient, preserved, and still potent. The deep cold does something to materials that nothing else replicates. I value the outcome even if I decline to romanticize the process.",
	"The sea left something behind when it half-abandoned this place. Retrieved with appropriate care.",
}

var HarvestHauntedManor = []string{
	"The manor keeps things. Has always kept things. I take this one out of the keeping and into the useful, which is a small act of defiance against the house's whole philosophy.",
	"Found among the things that have been here since the last person stopped being here. Provenance noted, not dwelt on.",
	"The house watches you take it. I watch the house watch you. A full triangle of observation — the house blinks first.",
}

var HarvestUnderforge = []string{
	"The forge yields its material with the grudging respect of something built to produce and still producing, even now, for someone it didn't expect.",
	"Dwarven craftsmanship even in the raw materials. I've always believed the quality is in the extraction, not just the finishing. This confirms it.",
	"Hot, heavy, and exactly what the zone promised. I mark the vein depth. There is more. The Underforge does not run out of things to give.",
}

var HarvestUnderdark = []string{
	"The Underdark produces everything the surface does, stranger, in the dark, and with fewer questions about why. I harvest without asking why.",
	"Things grow down here that have no equivalent above. I catalog the material and the context it came from with equal precision.",
	"A material that has never seen sunlight and is better for it. Noted without irony.",
}

var HarvestFeywild = []string{
	"The Feywild gives things away. That's the problem — it gives things away and sometimes what it gives isn't what you thought you were taking. I check the material twice. It appears to be what it appears to be. Remaining alert.",
	"Beautiful material from a zone that uses beauty as a weapon. I take it carefully, like picking up something that might be watching.",
	"The fey made this place generous on purpose. I'm not going to complain about the generosity and I'm not going to stop watching for the catch.",
}

var HarvestDragonsLair = []string{
	"Plucked from a hoard that has been accumulating since before your civilization named itself. Historical weight noted. Moving on.",
	"The kobolds will notice something is missing. They count everything. I account for this and suggest moving with intent.",
	"Dragon-adjacent materials carry something in them — a residual heat, a quality that doesn't exist anywhere the dragon hasn't been. I consider this a feature.",
}

var HarvestAbyssPortal = []string{
	"Harvesting from the Abyss. Flatly delivered, because the line deserves flat delivery. The material is valuable. The context is permanent.",
	"Reality left something behind when it tore here. I take the fragment and note: don't linger near the edges of what isn't stable.",
	"Demon ichor. I say the words with the efficiency of someone who has moved past the part where the words were alarming.",
}

// ─────────────────────────────────────────────────────────────────────────────
// FISHING — Zone-Specific Lines
// ─────────────────────────────────────────────────────────────────────────────

var FishingForestShadows = []string{
	"Fishing in a cursed forest stream. More peaceful than it has any right to be, and I refuse to let the shadow-things in the canopy ruin it.",
	"The stream runs dark but the fish run silver. I watch the line and say nothing for a while, which is what fishing is for.",
	"Like the fishing minigame before the hard dungeon in Ocarina of Time. I take the moment seriously.",
}

var FishingSunkenTemple = []string{
	"Fishing in a flooded temple. The fish here have never seen the surface. They don't know what they're missing. I'm uncertain whether to pity them.",
	"The line drops into water that hasn't moved in thirty years and immediately gets attention. Things down here are hungry. I watch the tension.",
	"Deep water fishing in the ruins of something ancient. The builders would have found this either sacrilegious or practical. I land on practical.",
}

var FishingUnderdark = []string{
	"The underground river is cold and fast and the fish in it have evolved past needing eyes, which I find both efficient and slightly unsettling.",
	"Fishing in total darkness in a river that doesn't appear on any surface map. I narrate by sound: the cast, the current, the eventual tension on the line.",
	"The Eyeless King is in here somewhere. I don't say this out loud but think it very clearly. The line goes deep.",
}

var FishingFeywild = []string{
	"Fey fishing. I watch the line for signs of time distortion — the way the reflection moves wrong, the way the fish seem to arrive before they bite. The Feywild is like this about everything.",
	"The stream catches light that shouldn't be here and the fish reflect it from below. I watch something luminous move toward the hook and try to stay professional about it.",
	"Like fishing in a dream. I mean this literally — the mechanics are the same but the rules feel advisory.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PATROL ENCOUNTER LINES
// ─────────────────────────────────────────────────────────────────────────────

var PatrolEncounter = []string{
	"A patrol. Moving between the cleared rooms with the confidence of something that considers them still theirs. I disagree with this assessment and will help you express the disagreement.",
	"They found you between rooms, which is exactly where patrols are supposed to find you. Threat Clock noted; I suggest dispatching this quickly and quietly.",
	"The patrol isn't looking for a fight — it's doing its job, which happens to involve you being somewhere you aren't supposed to be. I'll help you redefine 'supposed to.'",
	"Two of them. Moving together. The coordination suggests the zone is at Alert or higher. Confirmed: it is. Fight or fade.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LOOT DROP LINES — Generic
// ─────────────────────────────────────────────────────────────────────────────

var LootDropCommon = []string{
	"They had something on them. I check it over. Common rarity — useful in the way that common things are useful, which is often.",
	"Standard loot. Nothing that rewrites the story, but everything that keeps it going.",
	"A drop. I catalog it efficiently and note: this is the economy of dungeons. Enemies have things. You take them. The loop continues.",
}

var LootDropUncommon = []string{
	"Better than expected. I examine the drop with slightly elevated interest. Uncommon rarity — someone made this with intent.",
	"An uncommon drop from a common enemy. Anomaly noted with satisfaction. The dungeon was generous in this room.",
	"Uncommon. I turn it over once and nod. 'Keeper,' I say — which in my vocabulary means: this changes your math.",
}

var LootDropRare = []string{
	"I stop. Actually stop. 'That's rare,' I say, with the specific register of someone who uses the word correctly and uses it seldom.",
	"A rare drop. I examine it the way you examine something that doesn't appear often — thoroughly, quietly, with appropriate appreciation.",
	"The loot table gave you something uncommon and then kept going. Rare rarity. Filed in the column I reserve for things worth remembering.",
}

var LootDropLegendary = []string{
	"I go very still. The drop sits in the light and I process what I am seeing. 'Legendary,' I say eventually. One word. That's all it needs.",
	"Legendary rarity. I've seen a few of these in a long career and each time — each time — there is a moment that is separate from everything else. This is that moment. Pick it up carefully.",
	"The dungeon produced a legendary item. I note the zone, the enemy, the day of the expedition, the Threat Clock value, the precise conditions. Some things deserve to be recorded completely.",
}
