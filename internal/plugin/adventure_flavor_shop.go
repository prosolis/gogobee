package plugin

// ── Luigi's Shop Flavor Text ────────────────────────────────────────────────

var luigiGreetings = []string{
	"_leans on the counter_ Whadda y'all have? Menu is right above me. Everything's in stock, made fresh daily. Mostly.",
	"Welcome to Luigi's! Everything here is fresh out the oven. What can I get started for you today?",
	"_slides menu across the counter_ We've got some great stuff today. The Tier 3 armor just came in. Very fresh. Very fresh.",
	"ORDER'S UP for the last customer! _turns to you_ Hi! Welcome! What are we doing today, quick service or are you browsing?",
	"You look like someone who needs an upgrade. Good news -- Luigi's has got you covered. Fresh stock, hot and ready.",
	"_straightens apron_ We're fully staffed today and the inventory is looking real good. What are you in the market for?",
	"Luigi's! Home of the freshest gear in the region. _genuinely means this_ What can I get you?",
}

var luigiWeaponIntros = []string{
	"Okay so the weapons -- everything here is made with quality ingredients. The higher the tier, the fresher the materials. You can taste the difference. _pause_ Metaphorically. Don't taste the sword.",
	"Weapons are our most popular item. People come in for the sword, stay for the full combo. Can I interest you in the armor to go with that?",
	"Fresh blades today. The Tier 4 came in this morning. I'd move on that before the rush.",
}

var luigiArmorIntros = []string{
	"Armor is our specialty. Every piece is crafted to order. Well. It was crafted to order at some point. It's here now. Still fresh.",
	"We've got full chest protection from Tier 1 to Tier 5. The Tier 5 is premium. Made with only the finest materials. You can tell by the weight of it.",
	"The armor today is looking really good. Really fresh. I had the Tier 3 out front for the morning crowd and it moved fast.",
}

var luigiHelmetIntros = []string{
	"Helmets. Very important. People forget about the helmet and then they're in here asking why things went wrong. Get the helmet.",
	"We've got a full helmet lineup. Everything from the Dented Iron Helm -- which is a great value, honestly, a great value -- up to the Crown of the Fallen. Very premium item. Very fresh.",
}

var luigiBootsIntros = []string{
	"Boots are underrated. I always say that. People come in for the sword and walk out in the same boots they came in with and then wonder why their feet hurt. The boots matter.",
	"We've got boots across all tiers. The Tier 3 Swift Boots just came in. Fresh. Very fresh.",
}

var luigiToolIntros = []string{
	"The tools. Yes. Very important for the miners. We've got everything from the Copper Pickaxe -- which, listen, it does the job, I'm not going to oversell it -- all the way up to the Diamond Pickaxe. Premium tool. Made with the best materials we have.",
}

var luigiCategoryIntros = map[EquipmentSlot][]string{
	SlotWeapon: luigiWeaponIntros,
	SlotArmor:  luigiArmorIntros,
	SlotHelmet: luigiHelmetIntros,
	SlotBoots:  luigiBootsIntros,
	SlotTool:   luigiToolIntros,
}

// ── Purchase Confirmations ──────────────────────────────────────────────────

var luigiPurchaseConfirm = []string{
	"_punches it in_\nORDER UP! %s, one count! _slides it across the counter_\n\n€%d deducted. Good choice. I mean that -- genuinely good choice. Come back and see us.",
	"_punches it in_\nORDER UP! %s! _slides it across the counter with both hands_\n\n€%d deducted. You made the right call today. Luigi's guarantee.",
	"_punches it in_\nORDER UP! One %s, hot and ready! _nods approvingly_\n\n€%d deducted. That's going to serve you well. I stand behind every item on this floor.",
}

var luigiTier5Confirm = []string{
	"_stops what he's doing_ Okay. This is -- I want to put this together myself. Not because the staff can't handle it, they absolutely can, I just -- a Tier 5 order deserves the personal touch. Give me one second.\nORDER UP! _voice cracks slightly_ You made a great call today. A genuinely great call.\n\n€%d deducted for **%s**.",
	"_takes off his apron, puts it back on, adjusts it_ This is a Tier 5 order. I need -- I need a moment. _assembles the order with visible care_\nORDER UP! _barely containing himself_ This is what I'm here for. This right here.\n\n€%d deducted for **%s**.",
}

var luigiComboConfirm = []string{
	"Okay so we've got the full spread -- that's a combo right there and I love seeing a combo. Let me get all of that together for you.\nORDER UP! Full combo! _beams like he always does on a full combo_\n\n€%d deducted for **%s**.",
	"_looks at the order_ That's a combo. A real combo. People don't do combos enough and I've never understood why. This is how you shop.\nORDER UP! Full combo! _genuinely emotional_\n\n€%d deducted for **%s**.",
}

// ── Insufficient Funds ──────────────────────────────────────────────────────

var luigiInsufficientFunds = []string{
	"I hear you and I wish I could help but I can't move product at a loss -- that's not how this works and honestly it wouldn't be fair to you either. Come back when you've got the funds and I'll make sure it's here for you. Fresh.",
}

// ── Browsing Without Buying ─────────────────────────────────────────────────

var luigiBrowseTimeout = []string{
	"No rush. _glances at the door_ We've got people coming in so whenever you're ready. No pressure at all. Just -- whenever you're ready.",
	"_reorganizes the display case_ Take your time. I'm not going anywhere. The stock isn't going anywhere. We're all just... here. Waiting. Ready when you are.",
	"I'm going to need you to make a decision, we've got people behind you. _there are no people behind you_",
}

// ── Maxed Out / Fully Kitted ────────────────────────────────────────────────

var luigiMaxedOut = []string{
	"_looks you over_ You're fully kitted out. I respect that. I really do. _pause_ Is there anything I can -- no. You've got everything. _nods once_ Come back if anything needs replacing. Or just come by. The door's always open.",
}

// ── Masterwork / Arena Acknowledgement ──────────────────────────────────────

var luigiMasterworkAck = []string{
	"I see you've got a Masterwork piece in that slot. I can't beat that. But if you ever need a backup...",
	"That's a Masterwork item you've got there. _respectful nod_ I'm not going to pretend I carry anything that competes with that. But the rest of the menu is still worth a look.",
	"_notices the Arena gear_ You earned that. I know what that takes. I'm not here to replace it -- just here if you need anything else.",
}

// ── Show All Comment ────────────────────────────────────────────────────────

var luigiShowAllComment = []string{
	"Sure, I'll show you everything. No judgment.",
	"Full menu? You got it. I respect the thoroughness.",
}

// ── Unprompted Commentary ───────────────────────────────────────────────────

var luigiCommentary = []string{
	"_gestures at the Vorpal Sword_ That one's been sitting there for a while but I want to be clear -- it's still fresh. We rotate stock.",
	"The Dragonscale armor -- I'll be honest with you, I had a hard time sourcing that one. Supply chain issues. But it's here now and it is fresh.",
	"_taps the Dented Iron Helm_ This is a value item. I'm not going to pretend it's premium. But for the price? You're not going to beat it. I stand behind the value items.",
	"The Diamond Pickaxe just came in. Very fresh. Miners know. They always know when the fresh stock arrives.",
	"_quietly, about the Knobby Boots_ We still carry the Tier 0 boots. For... continuity. I don't recommend them. But they're there.",
}

// ── Cancellation ────────────────────────────────────────────────────────────

var luigiCancellation = []string{
	"Understood. No hard feelings. The door's always open and the stock is always fresh.",
	"That's fine. Totally fine. I'm not going to pressure you. _moves the item back behind the counter_ Come back anytime.",
}

// ── Item Descriptions ───────────────────────────────────────────────────────

type luigiItemKey struct {
	Slot EquipmentSlot
	Tier int
}

// luigiItemDescriptions — full multi-sentence descriptions for the confirm view.
var luigiItemDescriptions = map[luigiItemKey]string{
	// Weapons
	{SlotWeapon, 1}: "I'm not going to dress it up -- it's an entry level blade. Iron. Holds an edge if you treat it right. Good for someone just getting started and I mean that genuinely. Everyone starts somewhere and this is a solid somewhere.",
	{SlotWeapon, 2}: "Step up from the iron. You can feel it in the grip -- the steel is fresher, better balance. Not going to win any awards but I've seen people clear serious dungeons with this and come back smiling. Honest work. I stand behind it.",
	{SlotWeapon, 3}: "Now we're talking. Silver edge, proper weight, made with quality materials. This is the one people come back for and I'll tell you why -- it performs. Every time. If you're going to make one investment in your kit today, this is the one I'd point you to.",
	{SlotWeapon, 4}: "Premium item. Enchanted, fresh, made with the finest materials we source. I'm not going to tell you it's a limited time offer because it isn't -- we keep it stocked. What I will tell you is it moves fast because people who try it don't go back. That's not a sales pitch. That's just what happens.",
	{SlotWeapon, 5}: "_lowers voice_ The Vorpal Sword. I want to be straight with you -- I don't know everything that goes into this. The supplier doesn't share the full recipe and I've learned not to push on it. What I know is the quality. I've held one. You feel it immediately. This is the item. If you're here for this, you already know.",

	// Armor
	{SlotArmor, 1}: "The name. I know. It's the supplier's name, not mine, and I'll be upfront -- there's some wear on it. It's not damage, it's use history, and it's been inspected. For the price it's the best value protection I carry and I wouldn't sell it if I didn't mean that.",
	{SlotArmor, 2}: "Okay so -- the name. I hear it every day and I'll tell you what I tell everyone: I questioned it too when it first came in. Had my guy look at every link. Solid. The name came from the original batch which had issues. This isn't that batch. This batch passed everything. I wouldn't have it on the floor otherwise.",
	{SlotArmor, 3}: "Full plate. Heavy -- and that's a good thing, that weight is quality steel, you don't get that from cheap materials. This is the real thing and if you're at the point in your adventure where you need real protection, this is what real looks like.",
	{SlotArmor, 4}: "My personal recommendation for anyone who's serious about staying alive out there. Enchanted, properly made, and I've had customers come back three times for this. Not because it broke -- because they wanted a second one. That tells you everything.",
	{SlotArmor, 5}: "_takes a breath_ I'm going to be straight with you about the Dragonscale. I know what it's made of. I know where it comes from. I just -- I don't ask the details because some things you don't need to know and the quality makes the conversation unnecessary. What I can tell you is it's the finest piece of armor in this shop and I've never had a complaint. Not one. In this business that's the only thing that matters.",

	// Helmets
	{SlotHelmet, 1}: "It's an iron pot. I know what it looks like. I know the history. But listen -- it stops things from hitting your head and that's what a helmet does. For the price you are not going to find better head protection. I guarantee that personally.",
	{SlotHelmet, 2}: "The provenance is questionable. I'll give you that. The scratches were there when we got it and the previous owner didn't leave a forwarding address. But the steel is sound, the fit is decent, and it'll keep your skull in one piece. Nobody will compliment this helmet. Nobody needs to.",
	{SlotHelmet, 3}: "Reinforced, fitted properly -- or close enough. This is the helmet you buy when you're serious about not dying from the top down. Doesn't make you look competent but competence is what you do with the gear, not what the gear does to your face.",
	{SlotHelmet, 4}: "Guardian-grade. This helm has seen real battles, kept real heads intact, and carries itself with the quiet dignity you are only just beginning to deserve. I don't say that to be harsh. I say it because the helm is that good.",
	{SlotHelmet, 5}: "Crown of the Fallen. Every previous owner died in it. None of them died because of it. It will outlast you too. _pauses_ I want to be clear -- that's a selling point. This is the most premium headwear I carry and it's not even close.",

	// Boots
	{SlotBoots, 1}: "Taken off a corpse. I'm not going to sugarcoat it. The corpse didn't need them anymore and honestly these boots have more life in them than some of the Tier 2 stock. For the price? It's a no-brainer. Don't think about the previous owner.",
	{SlotBoots, 2}: "They've been places. Bad places. Places that did things to the leather you'd rather not examine. But they've held together through all of it and that tells you something about the construction. Mild discomfort is a feature. It means they're working.",
	{SlotBoots, 3}: "Light enough to move in, grip decent enough to trust. These boots are built for someone who moves with purpose, which -- and I'm being honest with you -- you are in the process of becoming. The boots believe in you. Let them.",
	{SlotBoots, 4}: "Ranger's boots. You move quieter. Faster. Longer. The ground cooperates with these in a way I can't fully explain. The forest notices. Something shifts. I've had rangers come in and refuse to take them off to try a different pair. That's how good they are.",
	{SlotBoots, 5}: "The wind doesn't slow you. Terrain offers suggestions you are free to decline. These boots are an affront to the concept of obstacles. I had a pair behind the counter once. I don't anymore because someone bought them within an hour. That's the demand we're talking about here.",

	// Tools
	{SlotTool, 1}: "Copper. Soft. It gets the job done if you hit very hard and the ore is feeling cooperative. I'm not going to oversell it. But for someone who's just getting into mining, it's the right starting point and I mean that.",
	{SlotTool, 2}: "Iron. Chipped to hell but bites the rock with something approaching intention. This is a pickaxe that exists and functions. The chip is cosmetic. Mostly. It won't affect performance in any way you'd notice if you weren't looking for it.",
	{SlotTool, 3}: "Steel, properly weighted, properly edged. The mountain will acknowledge this pickaxe. Not respect it -- that takes time -- but acknowledge it. This is the tool for someone who's ready to be taken seriously by the ore.",
	{SlotTool, 4}: "Mithril. Weighs nothing. Hits like consequence. The ores don't resist this so much as rearrange themselves out of respect. I've had miners come in after using one of these and I can see it in their eyes. Everything changed.",
	{SlotTool, 5}: "Diamond. Breaks anything short of fate and occasionally that too. I'm going to be honest -- this is the finest mining tool I have ever carried and I've been doing this long enough to know the difference. The only limits left are your arm, your nerve, and the number of hours in a day.",
}

// luigiItemOneLiners — short quotes for the category listing view.
var luigiItemOneLiners = map[luigiItemKey]string{
	// Weapons
	{SlotWeapon, 1}: "Entry level. Honest work.",
	{SlotWeapon, 2}: "Fresher steel. Better balance. Honest work.",
	{SlotWeapon, 3}: "The one people come back for. Every time.",
	{SlotWeapon, 4}: "Moves fast. People who try it don't go back.",
	{SlotWeapon, 5}: "This is the item. You already know.",

	// Armor
	{SlotArmor, 1}: "Best value protection I carry. I mean that.",
	{SlotArmor, 2}: "The name came from the old batch. This batch is solid.",
	{SlotArmor, 3}: "This is what real protection looks like.",
	{SlotArmor, 4}: "My personal recommendation. Customers come back three times.",
	{SlotArmor, 5}: "The finest piece of armor in this shop. Not one complaint.",

	// Helmets
	{SlotHelmet, 1}: "Stops things from hitting your head. Great value.",
	{SlotHelmet, 2}: "The steel is sound. Nobody will compliment it. Nobody needs to.",
	{SlotHelmet, 3}: "For when you're serious about not dying from the top down.",
	{SlotHelmet, 4}: "Guardian-grade. Quiet dignity. The real deal.",
	{SlotHelmet, 5}: "Every previous owner died in it. None because of it.",

	// Boots
	{SlotBoots, 1}: "Don't think about the previous owner.",
	{SlotBoots, 2}: "Mild discomfort is a feature. Means they're working.",
	{SlotBoots, 3}: "Built for someone who moves with purpose.",
	{SlotBoots, 4}: "Rangers refuse to take them off. That's how good they are.",
	{SlotBoots, 5}: "An affront to the concept of obstacles.",

	// Tools
	{SlotTool, 1}: "Gets the job done if the ore cooperates.",
	{SlotTool, 2}: "Exists and functions. The chip is cosmetic. Mostly.",
	{SlotTool, 3}: "The mountain will acknowledge this pickaxe.",
	{SlotTool, 4}: "Weighs nothing. Hits like consequence.",
	{SlotTool, 5}: "Breaks anything short of fate.",
}
