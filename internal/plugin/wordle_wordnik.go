package plugin

import (
	"fmt"
	"log/slog"
	"strings"

	"gogobee/internal/dreamclient"
)

// dictValidateWord checks if a word exists in DreamDict.
// Returns (valid, apiErr). apiErr is true when the service is unreachable.
func dictValidateWord(dict *dreamclient.Client, word, lang string) (valid bool, apiErr bool) {
	if dict == nil {
		return true, false // no client configured = skip validation
	}
	v, err := dict.IsValidWord(strings.ToLower(word), lang)
	if err != nil {
		slog.Warn("wordle: dictionary validation error", "word", word, "lang", lang, "err", err)
		return false, true
	}
	return v, false
}

// dictFetchDefinitionText fetches a clean definition string for display.
// Tries the given language first, falls back to English.
func dictFetchDefinitionText(dict *dreamclient.Client, word, lang string) string {
	if dict == nil {
		return ""
	}

	defs, err := dict.Define(strings.ToLower(word), lang)
	if err != nil {
		slog.Warn("wordle: definition fetch error", "word", word, "lang", lang, "err", err)
		return ""
	}

	// If no definitions in the puzzle language, try English.
	if len(defs) == 0 && lang != "en" {
		defs, err = dict.Define(strings.ToLower(word), "en")
		if err != nil || len(defs) == 0 {
			return ""
		}
	}

	if len(defs) == 0 {
		return ""
	}

	d := defs[0]
	text := strings.TrimSpace(d.Gloss)
	if text == "" {
		return ""
	}
	// Strip HTML tags if any.
	text = stripHTMLTags(text)
	pos := d.POS
	if pos != "" {
		return fmt.Sprintf("%s (%s): %s", strings.ToLower(word), pos, text)
	}
	return fmt.Sprintf("%s: %s", strings.ToLower(word), text)
}

// categoryLang returns the DreamDict language code for a Wordle category.
func categoryLang(category WordleCategory) string {
	switch category {
	case WordleCategoryPT:
		return "pt-PT"
	case WordleCategoryFR:
		return "fr"
	default:
		return "en"
	}
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
