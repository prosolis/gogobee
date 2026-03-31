package util

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// MessageStats holds parsed message metrics.
type MessageStats struct {
	Words       int
	Chars       int
	Links       int
	Images      int
	Questions   int
	Exclamations int
	Emojis      int
}

var (
	linkRe  = regexp.MustCompile(`https?://\S+`)
	imageRe = regexp.MustCompile(`\.(png|jpg|jpeg|gif|webp|svg|bmp)(\?[^\s]*)?$`)
	emojiRe = regexp.MustCompile(`[\x{1F600}-\x{1F64F}]|[\x{1F300}-\x{1F5FF}]|[\x{1F680}-\x{1F6FF}]|[\x{1F1E0}-\x{1F1FF}]|[\x{2600}-\x{26FF}]|[\x{2700}-\x{27BF}]`)
)

// ParseMessage extracts stats from a chat message body.
func ParseMessage(body string) MessageStats {
	words := strings.Fields(body)

	links := linkRe.FindAllString(body, -1)
	images := 0
	for _, l := range links {
		if imageRe.MatchString(strings.ToLower(l)) {
			images++
		}
	}

	questions := strings.Count(body, "?")
	exclamations := strings.Count(body, "!")
	emojis := len(emojiRe.FindAllString(body, -1))

	return MessageStats{
		Words:        len(words),
		Chars:        len([]rune(body)),
		Links:        len(links),
		Images:       images,
		Questions:    questions,
		Exclamations: exclamations,
		Emojis:       emojis,
	}
}

// XPForLevel returns the cumulative XP needed to reach level n.
// Uses quadratic curve: level² × 100.
func XPForLevel(level int) int {
	return level * level * 100
}

// LevelFromXP returns the current level for a given XP total.
// Inverse of level² × 100: level = floor(sqrt(xp / 100)).
func LevelFromXP(xp int) int {
	if xp <= 0 {
		return 0
	}
	level := 0
	for (level+1)*(level+1)*100 <= xp {
		level++
	}
	return level
}

// ProgressBar returns a text progress bar like [#####-----] 50%.
func ProgressBar(current, max, width int) string {
	if max <= 0 {
		return strings.Repeat("-", width)
	}
	ratio := float64(current) / float64(max)
	if ratio > 1 {
		ratio = 1
	}
	filled := int(math.Round(ratio * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled
	pct := int(ratio * 100)

	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", empty) + "] " + strconv.Itoa(pct) + "%"
}

// IsCommand checks if body starts with prefix+command (case-insensitive).
func IsCommand(body, prefix, command string) bool {
	cmd := prefix + command
	lower := strings.ToLower(strings.TrimSpace(body))
	if lower == cmd {
		return true
	}
	return strings.HasPrefix(lower, cmd+" ")
}

// GetArgs returns everything after the command prefix.
func GetArgs(body, prefix, command string) string {
	cmd := prefix + command
	trimmed := strings.TrimSpace(body)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, cmd) {
		return ""
	}
	rest := trimmed[len(cmd):]
	return strings.TrimSpace(rest)
}

// HasNonASCII checks if string contains non-ASCII characters.
func HasNonASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}
