package plugin

import (
	"fmt"
	"strings"
)

// letterEmoji returns the colored emoji for a LetterResult.
func letterEmoji(r LetterResult) string {
	switch r {
	case LetterCorrect:
		return "🟩"
	case LetterPresent:
		return "🟨"
	default:
		return "⬛"
	}
}

// renderGrid renders the full emoji grid for the current puzzle state.
func renderWordleGrid(puzzle *WordlePuzzle) string {
	var sb strings.Builder

	status := fmt.Sprintf("%d/6", len(puzzle.Guesses))
	if puzzle.Solved {
		status += "  🟩🟩🟩🟩🟩"
	}
	sb.WriteString(fmt.Sprintf("🟩 **Wordle #%d** — %s\n\n", puzzle.PuzzleNumber, status))

	for _, g := range puzzle.Guesses {
		// Emoji tiles.
		for _, r := range g.Results {
			sb.WriteString(letterEmoji(r))
		}
		// Word + player name.
		sb.WriteString(fmt.Sprintf("   %s  (%s)", g.Word, g.PlayerName))
		if g.Word == puzzle.Answer {
			sb.WriteString(" ✅")
		}
		sb.WriteString("\n")
	}

	// Keyboard view.
	sb.WriteString("\n")
	sb.WriteString(renderKeyboard(puzzle.LetterStates))

	return sb.String()
}

// renderKeyboard renders the QWERTY keyboard with color-coded letter states.
func renderKeyboard(states map[rune]LetterResult) string {
	rows := []string{
		"QWERTYUIOP",
		"ASDFGHJKL",
		"ZXCVBNM",
	}

	var sb strings.Builder
	for _, row := range rows {
		for i, ch := range row {
			if i > 0 {
				sb.WriteString("  ")
			}
			if state, ok := states[ch]; ok {
				sb.WriteString(letterEmoji(state))
			}
			sb.WriteRune(ch)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderWordleStartAnnouncement renders the puzzle start message.
func renderWordleStartAnnouncement(puzzleNumber, wordLength int, hint string) string {
	base := fmt.Sprintf(
		"🟩 **Daily Wordle #%d**\nA new %d-letter puzzle is ready! Work together — 6 guesses shared.",
		puzzleNumber, wordLength,
	)
	if hint != "" {
		base += fmt.Sprintf("\n🎮 **Hint:** %s", hint)
	}
	base += "\n\nGuess with: `!wordle <word>`"
	return base
}

// renderSolvedAnnouncement renders the solved puzzle message.
func renderSolvedAnnouncement(puzzle *WordlePuzzle, definition string, payouts []WordlePayout) string {
	var sb strings.Builder

	// Find the solver.
	lastGuess := puzzle.Guesses[len(puzzle.Guesses)-1]

	sb.WriteString(fmt.Sprintf("🎉 **Solved in %d/6!**\n", len(puzzle.Guesses)))
	sb.WriteString(fmt.Sprintf("The word was **%s** — solved by %s on guess %d.\n",
		puzzle.Answer, lastGuess.PlayerName, len(puzzle.Guesses)))

	if definition != "" {
		sb.WriteString(fmt.Sprintf("\n📖 *%s*\n", definition))
	}

	// Full grid.
	sb.WriteString("\n")
	for _, g := range puzzle.Guesses {
		for _, r := range g.Results {
			sb.WriteString(letterEmoji(r))
		}
		sb.WriteString(fmt.Sprintf("   %s  (%s)", g.Word, g.PlayerName))
		if g.Word == puzzle.Answer {
			sb.WriteString(" ✅")
		}
		sb.WriteString("\n")
	}

	// Contributors with payouts.
	if len(payouts) > 0 {
		sb.WriteString("\n💰 **Payouts:**\n")
		for _, pay := range payouts {
			bonus := ""
			if pay.Solver {
				bonus = " 🏆 (solver bonus!)"
			}
			sb.WriteString(fmt.Sprintf("  **%s**: +€%d%s\n", pay.Name, pay.Amount, bonus))
		}
	} else {
		sb.WriteString("\n🏆 Today's contributors:\n")
		contributors := wordleContributors(puzzle)
		for _, c := range contributors {
			line := fmt.Sprintf("  %s — %d guess", c.name, c.guesses)
			if c.guesses != 1 {
				line += "es"
			}
			if c.solved {
				line += " 🏆"
			}
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

// renderFailedAnnouncement renders the failed puzzle message.
func renderFailedAnnouncement(puzzle *WordlePuzzle, definition string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("💀 **Puzzle failed — 6/6**\nThe word was **%s**.\n", puzzle.Answer))

	if definition != "" {
		sb.WriteString(fmt.Sprintf("\n📖 *%s*\n", definition))
	}

	// Full grid.
	sb.WriteString("\n")
	for _, g := range puzzle.Guesses {
		for _, r := range g.Results {
			sb.WriteString(letterEmoji(r))
		}
		sb.WriteString(fmt.Sprintf("   %s  (%s)\n", g.Word, g.PlayerName))
	}

	sb.WriteString("\nBetter luck tomorrow. Contributors:\n")
	contributors := wordleContributors(puzzle)
	for _, c := range contributors {
		line := fmt.Sprintf("  %s — %d guess", c.name, c.guesses)
		if c.guesses != 1 {
			line += "es"
		}
		sb.WriteString(line + "\n")
	}

	return sb.String()
}

type wordleContributor struct {
	name    string
	guesses int
	solved  bool
}

// wordleContributors tallies guess counts per player.
func wordleContributors(puzzle *WordlePuzzle) []wordleContributor {
	type entry struct {
		name    string
		guesses int
		solved  bool
	}
	seen := map[string]*entry{}
	var order []string

	for i, g := range puzzle.Guesses {
		key := string(g.PlayerID)
		if e, ok := seen[key]; ok {
			e.guesses++
			if i == len(puzzle.Guesses)-1 && puzzle.Solved {
				e.solved = true
			}
		} else {
			e := &entry{name: g.PlayerName, guesses: 1}
			if i == len(puzzle.Guesses)-1 && puzzle.Solved {
				e.solved = true
			}
			seen[key] = e
			order = append(order, key)
		}
	}

	result := make([]wordleContributor, 0, len(order))
	for _, key := range order {
		e := seen[key]
		result = append(result, wordleContributor{name: e.name, guesses: e.guesses, solved: e.solved})
	}
	return result
}

// renderLeaderboard renders the all-time Wordle leaderboard.
func renderWordleLeaderboard(stats []WordlePlayerStat, streak int) string {
	var sb strings.Builder
	sb.WriteString("📊 **Wordle Leaderboard**\n")

	for i, s := range stats {
		line := fmt.Sprintf("  %d. %s — %d solve", i+1, s.DisplayName, s.PuzzlesSolved)
		if s.PuzzlesSolved != 1 {
			line += "s"
		}
		if s.WinningGuesses > 0 {
			line += fmt.Sprintf(" | %d winning guess", s.WinningGuesses)
			if s.WinningGuesses != 1 {
				line += "es"
			}
			line += " 🏆"
		}
		sb.WriteString(line + "\n")
	}

	if streak > 0 {
		sb.WriteString(fmt.Sprintf("\nCommunity streak: %d day", streak))
		if streak != 1 {
			sb.WriteString("s")
		}
		sb.WriteString(" solved in a row 🔥\n")
	}

	return sb.String()
}
