package plugin

import (
	"bufio"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
)

var (
	fallbackWords     map[int][]string
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
}

// pickFallbackWord picks a random word of the given length from the fallback list.
func pickFallbackWord(length int) string {
	fallbackWordsOnce.Do(loadFallbackWords)

	words := fallbackWords[length]
	if len(words) == 0 {
		return ""
	}
	return words[rand.IntN(len(words))]
}
