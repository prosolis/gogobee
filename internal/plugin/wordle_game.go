package plugin

import (
	"time"

	"maunium.net/go/mautrix/id"
)

// LetterResult represents the result of a single letter in a Wordle guess.
type LetterResult int

const (
	LetterAbsent  LetterResult = iota // ⬛ not in word
	LetterPresent                     // 🟨 right letter, wrong position
	LetterCorrect                     // 🟩 right letter, right position
)

// WordleGuess stores a single guess and its evaluation.
type WordleGuess struct {
	Word       string
	PlayerID   id.UserID
	PlayerName string
	Results    []LetterResult
	Timestamp  time.Time
}

// WordleCategory identifies the language/category of a puzzle.
type WordleCategory string

const (
	WordleCategoryEN    WordleCategory = ""     // English (default)
	WordleCategoryPT    WordleCategory = "pt"   // European Portuguese
	WordleCategoryFR    WordleCategory = "fr"   // French
	WordleCategoryGames WordleCategory = "games" // Video game words (English)
)

// WordlePuzzle holds all state for one day's puzzle.
type WordlePuzzle struct {
	PuzzleID     string // YYYY-MM-DD
	PuzzleNumber int
	RoomID       id.RoomID
	Answer       string // uppercased
	WordLength   int
	MaxGuesses   int
	Category     WordleCategory
	Guesses      []WordleGuess
	Solved       bool
	Failed       bool
	StartedAt    time.Time
	SolvedAt     *time.Time
	LetterStates map[rune]LetterResult // best known state per letter
}

// WordlePlayerStat tracks a player's all-time Wordle stats.
type WordlePlayerStat struct {
	UserID         id.UserID
	DisplayName    string
	TotalGuesses   int
	PuzzlesPlayed  int
	PuzzlesSolved  int
	WinningGuesses int
}

// scoreGuess evaluates a guess against the answer using the standard
// two-pass Wordle algorithm for correct duplicate-letter handling.
func scoreGuess(guess, answer string) []LetterResult {
	n := len(answer)
	results := make([]LetterResult, n)
	pool := make([]rune, 0, n)

	guessRunes := []rune(guess)
	answerRunes := []rune(answer)

	// First pass: mark exact matches (Correct).
	used := make([]bool, n)
	for i := 0; i < n; i++ {
		if guessRunes[i] == answerRunes[i] {
			results[i] = LetterCorrect
			used[i] = true
		}
	}

	// Build pool of unmatched answer letters.
	for i := 0; i < n; i++ {
		if !used[i] {
			pool = append(pool, answerRunes[i])
		}
	}

	// Second pass: mark Present or Absent.
	for i := 0; i < n; i++ {
		if results[i] == LetterCorrect {
			continue
		}
		found := false
		for j, r := range pool {
			if guessRunes[i] == r {
				results[i] = LetterPresent
				pool = append(pool[:j], pool[j+1:]...)
				found = true
				break
			}
		}
		if !found {
			results[i] = LetterAbsent
		}
	}

	return results
}

// updateLetterStates updates the keyboard state map with results from a guess.
// A letter's state only upgrades: Absent → Present → Correct.
func updateLetterStates(states map[rune]LetterResult, guess string, results []LetterResult) {
	for i, r := range []rune(guess) {
		existing, ok := states[r]
		if !ok || results[i] > existing {
			states[r] = results[i]
		}
	}
}
