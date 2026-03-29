package plugin

import (
	"bufio"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"

	"gogobee/internal/db"
)

var (
	fallbackWords     map[int][]string
	customAllowSet    map[string]bool // words valid as guesses but not in dictionary
	ptWords           map[int][]string
	frWords           map[int][]string
	ptWordSet         map[string]bool
	frWordSet         map[string]bool
	fallbackWordsOnce sync.Once
)

// loadFallbackWords loads the emergency word list from data/wordle_words.txt.
// Each line is one word. Words are grouped by length.
func loadFallbackWords() {
	fallbackWords = make(map[int][]string)

	path := "data/wordle_words.txt"
	f, err := os.Open(path)
	if err != nil {
		slog.Warn("wordle: fallback word list not found", "path", path)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		word = strings.ToUpper(word)
		n := len([]rune(word))
		if n >= 5 && n <= 7 {
			fallbackWords[n] = append(fallbackWords[n], word)
		}
	}

	for n, words := range fallbackWords {
		slog.Info("wordle: loaded fallback words", "length", n, "count", len(words))
	}

	// Load custom word lists (game titles, etc.) — these are added to the
	// puzzle pool AND accepted as valid guesses even if not in the dictionary.
	customAllowSet = make(map[string]bool)
	loadCustomWordFile("data/wordle_games.txt")

	// Load Portuguese and French word lists.
	ptWords, ptWordSet = loadLanguageWordFile("data/wordle_pt.txt", "pt")
	frWords, frWordSet = loadLanguageWordFile("data/wordle_fr.txt", "fr")
}

func loadCustomWordFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		slog.Warn("wordle: custom word list not found", "path", path)
		return
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		word = strings.ToUpper(word)
		n := len([]rune(word))
		if n >= 5 && n <= 7 {
			fallbackWords[n] = append(fallbackWords[n], word)
			customAllowSet[word] = true
			count++
		}
	}
	slog.Info("wordle: loaded custom words", "path", path, "count", count)
}

// loadLanguageWordFile loads a language-specific word list and returns both
// the length-grouped map and a flat set for guess validation.
func loadLanguageWordFile(path, lang string) (map[int][]string, map[string]bool) {
	words := make(map[int][]string)
	wordSet := make(map[string]bool)

	f, err := os.Open(path)
	if err != nil {
		slog.Warn("wordle: language word list not found", "lang", lang, "path", path)
		return words, wordSet
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		word = strings.ToUpper(word)
		n := len([]rune(word))
		if n >= 5 && n <= 7 {
			words[n] = append(words[n], word)
			wordSet[word] = true
		}
	}

	total := 0
	for _, w := range words {
		total += len(w)
	}
	slog.Info("wordle: loaded language words", "lang", lang, "count", total)
	return words, wordSet
}

// pickFallbackWord picks a random word of the given length from the fallback list,
// excluding words used in the last 500 puzzles.
func pickFallbackWord(length int) string {
	fallbackWordsOnce.Do(loadFallbackWords)

	words := fallbackWords[length]
	if len(words) == 0 {
		return ""
	}

	recent := loadRecentWordleAnswers(500)

	var candidates []string
	for _, w := range words {
		if !recent[w] {
			candidates = append(candidates, w)
		}
	}

	// Fall back to full list if all words have been used recently.
	if len(candidates) == 0 {
		candidates = words
	}
	return candidates[rand.IntN(len(candidates))]
}

// pickLanguageWord picks a random word from a language-specific list.
func pickLanguageWord(category WordleCategory, length int) string {
	fallbackWordsOnce.Do(loadFallbackWords)

	var pool map[int][]string
	switch category {
	case WordleCategoryPT:
		pool = ptWords
	case WordleCategoryFR:
		pool = frWords
	default:
		return pickFallbackWord(length)
	}

	words := pool[length]
	if len(words) == 0 {
		return ""
	}

	recent := loadRecentWordleAnswers(500)

	var candidates []string
	for _, w := range words {
		if !recent[w] {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		candidates = words
	}
	return candidates[rand.IntN(len(candidates))]
}

// isLanguageWord checks if a word exists in a language's word set (for guess validation).
func isLanguageWord(category WordleCategory, word string) bool {
	fallbackWordsOnce.Do(loadFallbackWords)
	word = strings.ToUpper(word)
	switch category {
	case WordleCategoryPT:
		return ptWordSet[word]
	case WordleCategoryFR:
		return frWordSet[word]
	}
	return false
}

// isCustomAllowedWord checks if a word is in the custom allow-list (game titles, etc.).
func isCustomAllowedWord(word string) bool {
	fallbackWordsOnce.Do(loadFallbackWords)
	return customAllowSet[strings.ToUpper(word)]
}

// loadRecentWordleAnswers returns a set of answers from the most recent N puzzles.
func loadRecentWordleAnswers(limit int) map[string]bool {
	d := db.Get()
	rows, err := d.Query(`SELECT DISTINCT answer FROM wordle_puzzles
		ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		slog.Error("wordle: failed to load recent answers", "err", err)
		return nil
	}
	defer rows.Close()

	recent := make(map[string]bool)
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			continue
		}
		recent[word] = true
	}
	return recent
}
