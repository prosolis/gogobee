package plugin

import (
	"fmt"
	"math/rand/v2"
)

const arenaParticipationXP = 60

// ── Closer Lines ───────────────────────────────────────────────────────────

func arenaWinCloser(loserName string, lastRound int) string {
	closers := []string{
		"%s fought. It counts.",
		"%s will be back. The arena keeps score.",
		fmt.Sprintf("%%s has until tomorrow to think about round %d.", lastRound),
		"%s gave you more trouble than you'd like to admit. They don't need to know that.",
		"%s loses this one. The next one is an open question.",
		"%s came here to fight and did. The result is a separate matter.",
		"%s is already planning the rematch. You can feel it.",
	}
	return fmt.Sprintf(closers[rand.IntN(len(closers))], loserName)
}

func arenaLoseCloser(winnerName string, lastRound int) string {
	closers := []string{
		"You fought. It counts.",
		"You'll be back. The arena keeps score.",
		fmt.Sprintf("You have until tomorrow to think about round %d.", lastRound),
		fmt.Sprintf("You gave %s more trouble than they'd like to admit. Small comfort. Still comfort.", winnerName),
		"You lose this one. The next one is an open question.",
		"You came here to fight and did. The result is a separate matter.",
		fmt.Sprintf("%s won this one. You're already planning the rematch.", winnerName),
	}
	return closers[rand.IntN(len(closers))]
}
