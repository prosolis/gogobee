package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// wordnikRandomWordResponse is the Wordnik randomWord response.
type wordnikRandomWordResponse struct {
	Word string `json:"word"`
}

// wordnikDefinitionResponse is one definition entry.
type wordnikDefinitionResponse struct {
	Text         string `json:"text"`
	PartOfSpeech string `json:"partOfSpeech"`
}

// wordnikFetchRandomWord fetches a random word of the given length from Wordnik.
// Returns the uppercased word. Retries up to 5 times to avoid bad words.
func wordnikFetchRandomWord(apiKey string, client *http.Client, wordLength int) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		url := fmt.Sprintf(
			"https://api.wordnik.com/v4/words.json/randomWord?hasDictionaryDef=true&minLength=%d&maxLength=%d&minCorpusCount=5000&minDictionaryCount=3&api_key=%s",
			wordLength, wordLength, apiKey,
		)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return "", fmt.Errorf("wordle: create request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("wordle: fetch random word: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("wordle: API returned status %d", resp.StatusCode)
		}

		var result wordnikRandomWordResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("wordle: decode response: %w", err)
		}

		word := strings.ToUpper(strings.TrimSpace(result.Word))

		// Reject words with hyphens, spaces, apostrophes.
		if strings.ContainsAny(word, "-' ") {
			slog.Debug("wordle: rejecting word with special chars", "word", word, "attempt", attempt+1)
			continue
		}

		// Reject if not all alphabetic.
		allAlpha := true
		for _, r := range word {
			if !unicode.IsLetter(r) {
				allAlpha = false
				break
			}
		}
		if !allAlpha {
			slog.Debug("wordle: rejecting non-alpha word", "word", word, "attempt", attempt+1)
			continue
		}

		// Check definition to reject proper nouns.
		if isProperNoun(apiKey, client, word) {
			slog.Debug("wordle: rejecting proper noun", "word", word, "attempt", attempt+1)
			continue
		}

		slog.Info("wordle: selected word", "word", word, "length", wordLength, "attempt", attempt+1)
		return word, nil
	}

	return "", fmt.Errorf("wordle: failed to find suitable word after 5 attempts")
}

// isProperNoun checks if a word's first definition starts with a capital letter.
func isProperNoun(apiKey string, client *http.Client, word string) bool {
	defs, err := wordnikFetchDefinitions(apiKey, client, strings.ToLower(word), 1)
	if err != nil || len(defs) == 0 {
		return false
	}
	text := strings.TrimSpace(defs[0].Text)
	if text == "" {
		return false
	}
	return unicode.IsUpper([]rune(text)[0])
}

// wordnikValidateWord checks if a word exists by looking up its definitions.
// Returns valid bool and an error flag. On API errors, returns false with apiErr=true
// so the caller can show a different message than "not a valid word".
func wordnikValidateWord(apiKey string, client *http.Client, word string) (valid bool, apiErr bool) {
	if apiKey == "" {
		return true, false // no API key = skip validation
	}
	defs, err := wordnikFetchDefinitions(apiKey, client, strings.ToLower(word), 1)
	if err != nil {
		slog.Warn("wordle: validation API error", "word", word, "err", err)
		return false, true
	}
	return len(defs) > 0, false
}

// wordnikFetchDefinitionText fetches a clean definition string for display.
func wordnikFetchDefinitionText(apiKey string, client *http.Client, word string) string {
	defs, err := wordnikFetchDefinitions(apiKey, client, strings.ToLower(word), 3)
	if err != nil || len(defs) == 0 {
		return ""
	}

	// Find the first clean definition.
	for _, d := range defs {
		text := strings.TrimSpace(d.Text)
		if text == "" {
			continue
		}
		// Strip HTML tags if any.
		text = stripHTMLTags(text)
		pos := d.PartOfSpeech
		if pos != "" {
			return fmt.Sprintf("%s (%s): %s", strings.ToLower(word), pos, text)
		}
		return fmt.Sprintf("%s: %s", strings.ToLower(word), text)
	}
	return ""
}

// wordnikFetchDefinitions fetches definitions from Wordnik.
func wordnikFetchDefinitions(apiKey string, client *http.Client, word string, limit int) ([]wordnikDefinitionResponse, error) {
	url := fmt.Sprintf(
		"https://api.wordnik.com/v4/word.json/%s/definitions?limit=%d&sourceDictionaries=all&api_key=%s",
		word, limit, apiKey,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // word not found
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var defs []wordnikDefinitionResponse
	if err := json.NewDecoder(resp.Body).Decode(&defs); err != nil {
		return nil, err
	}
	return defs, nil
}

// stripHTMLTags removes HTML tags from a string.
func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}
