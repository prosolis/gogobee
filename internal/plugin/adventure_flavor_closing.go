package plugin

// ── OUTCOME DM CLOSING BLOCKS ─────────────────────────────────────────────────
//
// These are appended after the stats block in every resolution DM.
// They confirm the day is spent, show time to reset, and close
// the interaction in the game's voice.
//
// Substitutions:
//   {location}     — where the player went
//   {reset_time}   — absolute UTC reset time e.g. "00:00 UTC"
//   {time_until}   — relative countdown e.g. "9h 23m"
//   {morning_time} — morning DM time e.g. "08:00 UTC"
//   {summary_time} — evening summary time e.g. "20:00 UTC"
//
// Selection: pick randomly per outcome category.
// Skip last 3 used per player to avoid immediate repetition.
// Death closings are NOT used when death DM is sent — that DM
// has its own closure. These are for survive/succeed/fail outcomes.

// ── SUCCESS ───────────────────────────────────────────────────────────────────
// Tone: dry acknowledgement. A good day happened. Don't oversell it.
// The player earned something. The game acknowledges this without enthusiasm.

var ClosingSuccess = []string{
	"─────────────────────────────\n" +
		"That's your day. {location} has been dealt with.\n\n" +
		"Next action available: {reset_time} ({time_until} from now)\n" +
		"Evening summary: {summary_time} UTC — see what everyone else managed. Or didn't.\n" +
		"Tomorrow's choices: {morning_time} UTC\n\n" +
		"Rest. You've earned it. Questionably, but earned.",

	"─────────────────────────────\n" +
		"Done. That's the day accounted for.\n\n" +
		"The {location} is behind you. The loot is in your inventory.\n" +
		"The rest of today is yours to do nothing useful with.\n\n" +
		"Resets: {reset_time} ({time_until})\n" +
		"Morning DM: {morning_time} UTC\n" +
		"Evening summary: {summary_time} UTC\n\n" +
		"Go rest. You've been through a dungeon.",

	"─────────────────────────────\n" +
		"You're done. The {location} has been dealt with,\n" +
		"or dealt with you, depending on how you're counting.\n\n" +
		"Tomorrow arrives at {morning_time} UTC ({time_until} from now).\n" +
		"The evening summary posts at {summary_time} UTC if you want\n" +
		"to see what everyone else managed. Or didn't.\n\n" +
		"Rest. You've earned it. Questionably.",

	"─────────────────────────────\n" +
		"A day, completed. The {location} has had its say\n" +
		"and you've had yours and the ledger is updated.\n\n" +
		"Nothing more to do until {reset_time} ({time_until}).\n" +
		"Morning DM at {morning_time} UTC with tomorrow's options.\n" +
		"TwinBee's results post at {summary_time} UTC.\n\n" +
		"You did alright today. Don't let it go to your head.",

	"─────────────────────────────\n" +
		"That's it. Day spent. Action taken. Outcome recorded.\n\n" +
		"The {location} will still be there tomorrow, worse for your visit.\n" +
		"Your choices will also be there, at {morning_time} UTC,\n" +
		"refreshed and waiting and not judging yesterday.\n\n" +
		"Reset: {reset_time} · {time_until} remaining\n" +
		"Evening summary: {summary_time} UTC\n\n" +
		"Go do something that isn't this. You've done this for today.",

	"─────────────────────────────\n" +
		"Done for today. The rest of the day is just\n" +
		"you and your choices and none of those choices involve this.\n\n" +
		"Resets: {reset_time} UTC · {time_until} from now\n" +
		"Tomorrow: {morning_time} UTC\n" +
		"Summary tonight: {summary_time} UTC\n\n" +
		"Rest. The {location} will be there when you get back.\n" +
		"The {location} is always there when you get back.",

	"─────────────────────────────\n" +
		"You're done. Good run. Not exceptional, not terrible —\n" +
		"a run. The kind of run that keeps an adventurer\n" +
		"in business and out of the deeper healthcare plans.\n\n" +
		"Next action: {reset_time} ({time_until})\n" +
		"Morning DM: {morning_time} UTC\n" +
		"Evening summary: {summary_time} UTC — TwinBee will be there.\n\n" +
		"So will everyone else who showed up today.",
}

// ── EXCEPTIONAL ───────────────────────────────────────────────────────────────
// Tone: lets the result breathe. Doesn't undercut the achievement.
// The closing is shorter here — the exceptional outcome DM already did
// the heavy lifting. The closing just marks the day as done.

var ClosingExceptional = []string{
	"─────────────────────────────\n" +
		"That's your day. Write it down.\n\n" +
		"Resets: {reset_time} · {time_until}\n" +
		"Morning DM: {morning_time} UTC\n" +
		"Evening summary: {summary_time} UTC — this one's getting mentioned.",

	"─────────────────────────────\n" +
		"Done. The {location} will not forget this.\n" +
		"Neither should you.\n\n" +
		"Next action: {reset_time} ({time_until})\n" +
		"Summary tonight at {summary_time} UTC.\n" +
		"Tomorrow: {morning_time} UTC.",

	"─────────────────────────────\n" +
		"That's today. Keep the receipt.\n\n" +
		"Resets: {reset_time} · {time_until} remaining\n" +
		"Evening summary: {summary_time} UTC\n" +
		"Tomorrow's choices: {morning_time} UTC\n\n" +
		"Rest. You've more than earned it.",

	"─────────────────────────────\n" +
		"Day spent. Exceptionally.\n\n" +
		"Next action: {reset_time} ({time_until})\n" +
		"The {summary_time} UTC summary is going to be good tonight.\n" +
		"Morning DM at {morning_time} UTC with whatever comes next.\n\n" +
		"It won't be this. But it'll be something.",

	"─────────────────────────────\n" +
		"Done. That's a day worth having had.\n\n" +
		"Resets: {reset_time} · {time_until}\n" +
		"Summary: {summary_time} UTC\n" +
		"Tomorrow: {morning_time} UTC\n\n" +
		"Go rest. Exceptional days are still days and days end.",
}

// ── FAILURE / EMPTY ───────────────────────────────────────────────────────────
// Tone: the game knows it went badly. It's not cruel about it.
// Resigned. Accurate. Leaves a door open for tomorrow.

var ClosingFailure = []string{
	"─────────────────────────────\n" +
		"That's your day. Not the day you wanted, but the day you got.\n\n" +
		"Next action: {reset_time} · {time_until} from now\n" +
		"Tomorrow's choices: {morning_time} UTC\n" +
		"Evening summary: {summary_time} UTC — see how everyone else did.\n\n" +
		"Some days the dungeon wins. Today was that day.\n" +
		"Tomorrow is a different proposition.",

	"─────────────────────────────\n" +
		"Done. The {location} was uncooperative today.\n" +
		"This is noted. This will not be the last time it's noted.\n\n" +
		"Resets: {reset_time} ({time_until})\n" +
		"Morning DM: {morning_time} UTC — new choices, clean slate.\n" +
		"Summary: {summary_time} UTC\n\n" +
		"Rest. Tomorrow the {location} gets another chance to disappoint you.\n" +
		"You also get another chance. That's how this works.",

	"─────────────────────────────\n" +
		"You're done. The {location} gave nothing today.\n" +
		"The {location} does not apologise for this.\n" +
		"The {location} does not apologise for anything.\n\n" +
		"Next action: {reset_time} · {time_until}\n" +
		"Tomorrow: {morning_time} UTC\n" +
		"Summary tonight: {summary_time} UTC\n\n" +
		"Come back tomorrow. Bring the same bad equipment.\n" +
		"Maybe the mountain will be in a better mood.",

	"─────────────────────────────\n" +
		"That's today. Nothing found. XP gained, technically.\n" +
		"The experience of finding nothing is still experience.\n" +
		"The game counts it. The game is generous with definitions.\n\n" +
		"Resets: {reset_time} · {time_until} remaining\n" +
		"Morning DM: {morning_time} UTC\n" +
		"Evening summary: {summary_time} UTC\n\n" +
		"Tomorrow is not today. That's the best thing that can be said about it.",

	"─────────────────────────────\n" +
		"Done. Bad day. It happens.\n\n" +
		"The {location} is still there. It will still be there tomorrow.\n" +
		"So will you, which is the important part.\n\n" +
		"Next action: {reset_time} ({time_until})\n" +
		"Tomorrow's choices at {morning_time} UTC.\n" +
		"TwinBee had a better day. The summary at {summary_time} UTC will confirm this.\n\n" +
		"Rest. You've earned the rest, at least.",

	"─────────────────────────────\n" +
		"That's your day. The {location} was not impressed with you today.\n" +
		"The feeling, presumably, is mutual.\n\n" +
		"Resets: {reset_time} · {time_until}\n" +
		"Morning DM: {morning_time} UTC — fresh options.\n" +
		"Evening summary: {summary_time} UTC\n\n" +
		"Go rest. The {location} will be here when you get back.\n" +
		"The {location} is always here. That's the problem with it.",
}

// ── DEATH ─────────────────────────────────────────────────────────────────────
// Terse — the death resolution DM already handles the emotional weight.
// This closing is a brief bridge to the healthcare lockout.

var ClosingDeath = []string{
	"─────────────────────────────\n" +
		"That's your day. Healthcare has the rest.\n\n" +
		"🏥 A DM from St. Guildmore's is on its way.\n" +
		"Type `!hospital` for same-day revival.",

	"─────────────────────────────\n" +
		"Day over. Healthcare is involved.\n" +
		"🏥 St. Guildmore's has been notified.\n\n" +
		"Check your DMs, or type `!hospital`.",
}
