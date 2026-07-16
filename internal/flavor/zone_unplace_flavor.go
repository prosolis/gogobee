// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_unplace_flavor.go
// Tier 6 post-game zone flavor — The Unplace. Additive only.

package flavor

// ROOM ENTRY
var RoomEntryUnplace = []string{
	"You cross into a room the map refuses to draw. The far wall is also the ceiling, depending on which way you were walking when you noticed it. I file this under 'geometry declined to render' and log your position as approximate.",
	"A corridor that loops back on itself the way a bad tilemap does when the artist forgot to cap the edge. You walk forward and arrive behind where you started. I recommend you stop trusting 'forward' as a concept in here.",
	"The room has a horizon. Indoors. A flat line where floor meets sky and there is no reason for a sky. You keep your eyes down and I keep my measurements to myself.",
	"You step through the doorway and the doorway steps through you; for one frame you are the wall and the wall is the exit. I track it as a clipping error and mark you unharmed, mostly.",
	"Corners here are deeper than the room they belong to. You look into one and it looks back and it is not empty. I recommend you treat right angles as hostile terrain until we leave.",
	"A chamber tiled in the same eight tiles, over and over, wrong. Somewhere the wrongness has a seam, and the seam is where the tiles don't quite line up. I file it under 'the level was never finished.'",
	"The floor scrolls under you like a background layer moving faster than the sprite on top of it. You are walking and standing still at the same time. I count the discrepancy and choose not to share the number.",
	"You enter and the room is already occupied by an earlier version of you, mid-stride, three seconds behind. It catches up, overlaps, and is gone. I log it as a duplicate object the engine forgot to despawn.",
	"Water on all four walls, held there by nothing, reflecting a room that is not this one. Through the reflection you can see the way out. Through the way out you cannot. I recommend the reflection and note that I am guessing.",
	"The lighting comes from a source that is behind you no matter which way you turn. Your shadow points at the exit. I have learned to trust the shadow before I trust the room, and I suggest you do the same.",
}

// BOSS ENTRY
var BossEntrySeamstress = []string{
	"The last door opens onto a room being unmade and remade in the same motion, and at the center of it she is kneeling into the tear with half of herself already gone through. The near half does not speak. The far half, the part you cannot see, is doing all the talking, and it is talking to the tear, not to you.",
	"You reach the Seam. Threads of light run from her hands into the wound in the world, and from the wound back into her, and it is impossible to say which is sewing which. She turns the part of her head that is still on this side. 'You're late,' says the half of her voice that arrives before the rest.",
	"The Seamstress has been here for centuries and the room knows it; the walls curve toward her the way a save file curves toward its one corrupted byte. She does not stand. She has stopped needing to. I file this beat under 'boss arena, no cutscene skip.'",
	"She volunteered for this. I want that on the record before we begin. The celestial who came to close the tear early, and stayed, and stitched, and became the stitch. What faces you now is what's left over after the needle. I recommend you not mistake exhaustion for weakness.",
	"The far half of her finishes a sentence you never heard the start of and the near half raises a needle the length of your forearm. Light gathers along it in a pattern I have seen exactly once before, in an arcade cabinet, one frame before the screen filled with bullets. 'Hold still,' both halves say. 'This closes everything.'",
}

// LORE
var LoreLinesUnplace = []string{
	"Belaxath tore the portal over thirty years of patient work. Killing him was supposed to close it. It did not. It only left the tear unattended, and an unattended wound in the world does not scab over. It scars into something with opinions.",
	"This region is not a place. It is the absence where a place used to agree with itself. The locals, before there were no more locals, called it the Unplace. I have found no record of who named it, because the records are also in here and also do not agree with themselves.",
	"Something has been stitching the tear shut from the inside for centuries. I want to be clear that this is not good news. A thing that seals a wound from inside the wound is not trying to let you out.",
	"The rooms that are also other rooms are not a metaphor. Two chambers occupy the same coordinates and take turns being real. I have mapped it twice and gotten two maps that share no walls. I file both under 'correct.'",
	"The Unnumbered is not many demons. It is the question of how many demons, left unanswered so long it grew teeth. Count it from one angle and there are three. From another, nine. The count is not a fact about the demons. It is a fact about you looking.",
	"The Angleworn Horror does not enter rooms. It enters corners, and corners are everywhere, and so it is always already present three rooms before you meet it. By the time it is 'here' it has been eating your flank since the last save point.",
	"The Echo of Belaxath drops nothing because it is not there. It is a recording the Unplace plays because the tear remembers who made it. Fighting it costs you and pays you nothing. I file this under 'toll,' which is the honest word for a thing you pay and do not buy.",
	"The Seamstress arrived to close the tear early, out of mercy, and the tear taught her the price of mercy applied to a wound this size: you do not close it from a distance, you close it with the only thread long enough, which is yourself. Half of her is on the far side now. I do not know what the far side does with the half it holds, and I have decided not to find out.",
}

// SEAMSTRESS SIGNATURE CALLOUTS
var SeamstressSignatureCallouts = []string{
	"Needle Rain. The ceiling fills with descending threads of light. I say: 'That's the pattern from the cabinet — move on the telegraph, not the impact.'",
	"She reels a thread taut across the arena; step over it or it cuts on the return. I call the line and you jump the line.",
	"The far half of her speaks and the near half's attacks land a half-second early. I recommend you dodge the voice, not the hand.",
	"She's threading toward your back seam. Turn into her — the Angleworn taught this room to hunt your flank, and she learned it too.",
	"Barbed sutures across the floor. They tighten. Whatever's inside the circle when they close, stays. I call it: 'Get out of the ring.'",
	"She pulls the room half a tile sideways — your hitbox and your sprite disagree for a beat. Trust where she's aiming, not where you're standing.",
	"A needle the length of a spear, one clean thrust down the lane you're standing in. I flag the lane. You leave the lane.",
	"She stitches a copy of the last attack into the room; it fires again on a delay you can count. I count it out loud so you don't have to.",
	"Threads converge on a single point and that point is your heart, plainly telegraphed. I say: 'Break line of sight or eat it — those are the two options.'",
	"She goes still and sews. Everything she mends on herself now, she means to unmend on you later. I file the pause under 'do damage, this is the window.'",
}

// SEAMSTRESS PHASE TWO
var SeamstressPhaseTwoLines = []string{
	"Below thirty-five percent she pulls the room inside-out and the far half comes forward. Now the threads run backward. I say: 'Watch the pulse — when it flares, what you'd call a wound on her is a gift, and what she calls a mercy on you draws blood.'",
	"Inversion Stitch. The room is sewn wrong-side-round and healing and harm swap seats on the telegraph. Mend her during the flare and it costs her; let her mend herself and it costs you. I recommend you learn to read the pulse faster than she can throw it.",
	"The near half has nothing left to say and the far half has stopped saying it to the tear. It's saying it to you now. Below thirty-five she is not closing the wound anymore — she is deciding you are the last thread she needs, and I recommend you disagree quickly.",
}
