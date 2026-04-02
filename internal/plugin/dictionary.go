package plugin

import (
	"fmt"
	"log/slog"
	"strings"

	"gogobee/internal/dreamclient"

	"maunium.net/go/mautrix"
)

// DictionaryPlugin provides extended dictionary commands using DreamDict.
type DictionaryPlugin struct {
	Base
	dict *dreamclient.Client
}

func NewDictionaryPlugin(client *mautrix.Client, dict *dreamclient.Client) *DictionaryPlugin {
	return &DictionaryPlugin{
		Base: NewBase(client),
		dict: dict,
	}
}

func (p *DictionaryPlugin) Name() string { return "dictionary" }

func (p *DictionaryPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "antonym", Description: "Look up antonyms for a word", Usage: "!antonym <word> [lang]", Category: "Lookup & Reference"},
		{Name: "pronounce", Description: "Look up pronunciation of a word", Usage: "!pronounce <word> [lang]", Category: "Lookup & Reference"},
		{Name: "etymology", Description: "Look up the etymology of a word", Usage: "!etymology <word> [lang]", Category: "Lookup & Reference"},
		{Name: "difficulty", Description: "Show difficulty score for a word", Usage: "!difficulty <word> [lang]", Category: "Lookup & Reference"},
		{Name: "rhyme", Description: "Find rhyming words (English)", Usage: "!rhyme <word>", Category: "Lookup & Reference"},
	}
}

func (p *DictionaryPlugin) Init() error { return nil }

func (p *DictionaryPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *DictionaryPlugin) OnMessage(ctx MessageContext) error {
	var handler func(MessageContext) error
	switch {
	case p.IsCommand(ctx.Body, "antonym"):
		handler = p.handleAntonym
	case p.IsCommand(ctx.Body, "pronounce"):
		handler = p.handlePronounce
	case p.IsCommand(ctx.Body, "etymology"):
		handler = p.handleEtymology
	case p.IsCommand(ctx.Body, "difficulty"):
		handler = p.handleDifficulty
	case p.IsCommand(ctx.Body, "rhyme"):
		handler = p.handleRhyme
	default:
		return nil
	}
	go func() {
		if err := handler(ctx); err != nil {
			slog.Error("dictionary: handler error", "err", err)
		}
	}()
	return nil
}

const dictUnavailable = "Dictionary service unavailable. Try again shortly."
const dictSupportedLangs = "en, fr, pt-PT, zh"

// parseWordLang extracts word and language from command args. Returns empty word if missing.
func parseWordLang(args string) (word, lang string) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "", ""
	}
	word = strings.ToLower(parts[0])
	lang = "en"
	if len(parts) >= 2 {
		if l := normaliseLangExt(parts[len(parts)-1]); l != "" {
			lang = l
			if len(parts) > 2 {
				word = strings.ToLower(strings.Join(parts[:len(parts)-1], " "))
			}
		}
	}
	return word, lang
}

// normaliseLangExt normalises a language tag, including zh.
func normaliseLangExt(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "en":
		return "en"
	case "fr":
		return "fr"
	case "pt-pt", "pt":
		return "pt-PT"
	case "zh":
		return "zh"
	default:
		return ""
	}
}

func (p *DictionaryPlugin) handleAntonym(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "antonym"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!antonym <word> [lang]`\nSupported languages: "+dictSupportedLangs)
	}

	word, lang := parseWordLang(args)
	if p.dict == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	// Check word exists.
	valid, err := p.dict.IsValidWord(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}
	if !valid {
		langName := langDisplayName(lang)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("\"**%s**\" not found in %s.", word, langName))
	}

	ants, err := p.dict.Antonyms(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	if len(ants) == 0 {
		langName := langDisplayName(lang)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("🔄 No antonyms found for \"**%s**\" in %s.", word, langName))
	}

	display := ants
	more := ""
	if len(display) > 6 {
		more = fmt.Sprintf(" (+%d more)", len(display)-6)
		display = display[:6]
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("🔄 Antonyms of **%s** (%s): %s%s", word, lang, strings.Join(display, ", "), more))
}

func (p *DictionaryPlugin) handlePronounce(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "pronounce"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!pronounce <word> [lang]`\nSupported languages: "+dictSupportedLangs)
	}

	word, lang := parseWordLang(args)
	if p.dict == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	valid, err := p.dict.IsValidWord(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}
	if !valid {
		langName := langDisplayName(lang)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("\"**%s**\" not found in %s.", word, langName))
	}

	prons, err := p.dict.Pronunciations(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	if len(prons) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("🔊 No pronunciation data found for \"**%s**\" in %s.", word, langDisplayName(lang)))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔊 **%s** (%s)\n", word, lang))

	for _, pr := range prons {
		label := strings.ToUpper(pr.Format)
		if lang == "zh" && pr.Format == "ipa" {
			label = "Pinyin"
		}
		sb.WriteString(fmt.Sprintf("  %s:  %s\n", label, pr.Value))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *DictionaryPlugin) handleEtymology(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "etymology"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!etymology <word> [lang]`\nSupported languages: "+dictSupportedLangs)
	}

	word, lang := parseWordLang(args)
	if p.dict == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	valid, err := p.dict.IsValidWord(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}
	if !valid {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("\"**%s**\" not found in %s.", word, langDisplayName(lang)))
	}

	etym, err := p.dict.Etymology(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	if etym == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("📜 No etymology data found for \"**%s**\" in %s.", word, langDisplayName(lang)))
	}

	// Truncate at 400 chars for standalone command.
	if len(etym) > 400 {
		// Try to truncate at sentence boundary.
		if idx := strings.LastIndex(etym[:400], "."); idx > 200 {
			etym = etym[:idx+1] + "..."
		} else {
			etym = etym[:400] + "..."
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("📜 Etymology of **%s** (%s)\n  %s", word, lang, etym))
}

func (p *DictionaryPlugin) handleDifficulty(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "difficulty"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!difficulty <word> [lang]`\nSupported languages: "+dictSupportedLangs)
	}

	word, lang := parseWordLang(args)
	if p.dict == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	valid, err := p.dict.IsValidWord(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}
	if !valid {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("\"**%s**\" not found in %s.", word, langDisplayName(lang)))
	}

	diff, err := p.dict.Difficulty(word, lang)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}
	if diff < 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("📊 No difficulty score available for \"**%s**\" in %s.", word, langDisplayName(lang)))
	}

	tier := difficultyTier(diff)

	// Get frequency for display.
	freq, _ := p.dict.Frequency(word, lang)
	freqLabel := frequencyLabel(freq)

	wordLen := len([]rune(word))

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("📊 **%s** (%s) -- %s (%.2f)\n   Frequency: %s  |  Length: %d",
			word, lang, tier, diff, freqLabel, wordLen))
}

func (p *DictionaryPlugin) handleRhyme(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "rhyme"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!rhyme <word>`\nNote: rhyme matching is English only.")
	}

	parts := strings.Fields(args)
	word := strings.ToLower(parts[0])
	showAll := false
	for _, arg := range parts[1:] {
		if arg == "--all" {
			showAll = true
		}
	}

	if p.dict == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	rhymes, err := p.dict.Rhymes(word, 50)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, dictUnavailable)
	}

	if len(rhymes) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("🎵 No rhymes found for \"**%s**\". (This is not surprising.)", word))
	}

	display := rhymes
	more := ""
	if !showAll && len(display) > 7 {
		more = fmt.Sprintf(" (+%d more)\n   Use `!rhyme %s --all` to see all results", len(display)-7, word)
		display = display[:7]
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("🎵 Rhymes with **%s**: %s%s", word, strings.Join(display, ", "), more))
}

// difficultyTier maps a 0.0-1.0 score to a human-readable label.
func difficultyTier(score float64) string {
	switch {
	case score <= 0.25:
		return "Easy"
	case score <= 0.50:
		return "Medium"
	case score <= 0.75:
		return "Hard"
	default:
		return "Brutal"
	}
}

// frequencyLabel maps a frequency score to a display label.
func frequencyLabel(freq int) string {
	switch {
	case freq > 500:
		return "high"
	case freq >= 100:
		return "medium"
	case freq > 0:
		return "low"
	default:
		return "unknown"
	}
}

// langDisplayName returns a human-readable name for a language code.
func langDisplayName(lang string) string {
	switch lang {
	case "en":
		return "English"
	case "fr":
		return "French"
	case "pt-PT":
		return "Portuguese"
	case "zh":
		return "Chinese"
	default:
		return lang
	}
}
