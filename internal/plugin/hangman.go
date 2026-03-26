package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"unicode"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// ---------------------------------------------------------------------------
// ASCII Gallows
// ---------------------------------------------------------------------------

var gallows = [7]string{
	// Stage 0
	`
  +---+
  |
  |
  |
  |
 ===`,
	// Stage 1
	`
  +---+
  |   |
  |
  |
  |
 ===`,
	// Stage 2
	`
  +---+
  |   |
  |   O
  |
  |
 ===`,
	// Stage 3
	`
  +---+
  |   |
  |   O
  |   |
  |
 ===`,
	// Stage 4
	`
  +---+
  |   |
  |   O
  |  /|
  |
 ===`,
	// Stage 5
	`
  +---+
  |   |
  |   O
  |  /|\
  |
 ===`,
	// Stage 6 (dead)
	`
  +---+
  |   |
  |   O
  |  /|\
  |  / \
 ===`,
}

// ---------------------------------------------------------------------------
// Hangman config and types
// ---------------------------------------------------------------------------

type hangmanTier struct {
	Name  string
	Min   int
	Max   int
	Bonus float64
}

var hangmanTiers = []hangmanTier{
	{"Easy", 8, 20, 25},
	{"Medium", 21, 40, 75},
	{"Hard", 41, 80, 200},
	{"Extreme", 81, 9999, 500},
}

func getTier(phrase string) hangmanTier {
	n := len(phrase)
	for _, t := range hangmanTiers {
		if n >= t.Min && n <= t.Max {
			return t
		}
	}
	return hangmanTiers[0]
}

type hangmanGame struct {
	phrase       string
	runes        []rune // phrase as rune slice for safe indexing
	tier         hangmanTier
	revealed     []bool // true for each rune that is revealed
	wrongGuesses []rune
	maxWrong     int
	participants map[id.UserID]bool
	solved       bool
	solvedBy     id.UserID
	earlySolve   bool
	threadID     id.EventID // thread root event for this game
}

func newHangmanGame(phrase string, maxWrong int) *hangmanGame {
	runes := []rune(phrase)
	revealed := make([]bool, len(runes))
	// Auto-reveal spaces and punctuation
	for i, ch := range runes {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			revealed[i] = true
		}
	}
	return &hangmanGame{
		phrase:       phrase,
		runes:        runes,
		tier:         getTier(phrase),
		revealed:     revealed,
		maxWrong:     maxWrong,
		participants: make(map[id.UserID]bool),
	}
}

func (g *hangmanGame) wrongCount() int {
	return len(g.wrongGuesses)
}

func (g *hangmanGame) remaining() int {
	return g.maxWrong - g.wrongCount()
}

func (g *hangmanGame) isDead() bool {
	return g.wrongCount() >= g.maxWrong
}

func (g *hangmanGame) isFullyRevealed() bool {
	for _, r := range g.revealed {
		if !r {
			return false
		}
	}
	return true
}

func (g *hangmanGame) revealedCount() int {
	count := 0
	for i, ch := range g.runes {
		if (unicode.IsLetter(ch) || unicode.IsDigit(ch)) && g.revealed[i] {
			count++
		}
	}
	return count
}

func (g *hangmanGame) letterCount() int {
	count := 0
	for _, ch := range g.runes {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			count++
		}
	}
	return count
}

func (g *hangmanGame) displayPhrase() string {
	var sb strings.Builder
	for i, ch := range g.runes {
		if ch == ' ' {
			sb.WriteString("   ") // wide gap for word boundaries
		} else if g.revealed[i] {
			sb.WriteRune(ch)
			sb.WriteRune(' ')
		} else {
			sb.WriteString("_ ")
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

func (g *hangmanGame) guessLetter(ch rune) (hit bool, alreadyGuessed bool) {
	ch = unicode.ToUpper(ch)
	lower := unicode.ToLower(ch)

	// Check if already guessed (wrong list or already revealed)
	for _, w := range g.wrongGuesses {
		if unicode.ToUpper(w) == ch {
			return false, true
		}
	}

	hit = false
	for i, c := range g.runes {
		if unicode.ToUpper(c) == ch || unicode.ToLower(c) == lower {
			if g.revealed[i] {
				// Already revealed — check if ALL instances are revealed
				allRevealed := true
				for j, cc := range g.runes {
					if (unicode.ToUpper(cc) == ch || unicode.ToLower(cc) == lower) && !g.revealed[j] {
						allRevealed = false
					}
				}
				if allRevealed {
					return false, true
				}
			}
			g.revealed[i] = true
			hit = true
		}
	}

	// Reveal adjacent punctuation when a letter is revealed
	if hit {
		for i := range g.runes {
			if g.revealed[i] {
				// Reveal adjacent non-letter chars
				if i > 0 && !unicode.IsLetter(g.runes[i-1]) && !unicode.IsDigit(g.runes[i-1]) {
					g.revealed[i-1] = true
				}
				if i < len(g.runes)-1 && !unicode.IsLetter(g.runes[i+1]) && !unicode.IsDigit(g.runes[i+1]) {
					g.revealed[i+1] = true
				}
			}
		}
	}

	if !hit {
		g.wrongGuesses = append(g.wrongGuesses, ch)
	}
	return hit, false
}

func (g *hangmanGame) guessSolution(attempt string) bool {
	return strings.EqualFold(strings.TrimSpace(attempt), g.phrase)
}

func (g *hangmanGame) wrongGuessStr() string {
	if len(g.wrongGuesses) == 0 {
		return "none"
	}
	parts := make([]string, len(g.wrongGuesses))
	for i, r := range g.wrongGuesses {
		parts[i] = string(unicode.ToUpper(r))
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type HangmanPlugin struct {
	Base
	euro     *EuroPlugin
	phrases  []string
	maxWrong int
	bonusMul float64

	mu    sync.Mutex
	games map[id.RoomID]*hangmanGame
}

func NewHangmanPlugin(client *mautrix.Client, euro *EuroPlugin) *HangmanPlugin {
	return &HangmanPlugin{
		Base:     NewBase(client),
		euro:     euro,
		maxWrong: envInt("HANGMAN_MAX_WRONG_GUESSES", 6),
		bonusMul: envFloat("HANGMAN_SOLUTION_BONUS_MULTIPLIER", 2),
		games:    make(map[id.RoomID]*hangmanGame),
	}
}

func (p *HangmanPlugin) Name() string { return "hangman" }

func (p *HangmanPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "hangman", Description: "Collaborative Hangman game", Usage: "!hangman start [easy|medium|hard|extreme] | !hangman [letter/phrase] | !hangman submit [phrase]", Category: "Games"},
		{Name: "hangboard", Description: "Hangman leaderboard", Usage: "!hangboard", Category: "Games"},
	}
}

func (p *HangmanPlugin) Init() error {
	path := os.Getenv("HANGMAN_PHRASE_FILE")
	if path != "" {
		p.loadPhrases(path)
	}
	return nil
}

func (p *HangmanPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *HangmanPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "hangboard") {
		return p.handleBoard(ctx)
	}

	if !p.IsCommand(ctx.Body, "hangman") {
		// Check if this is a thread reply to an active hangman game
		if !isGamesRoom(ctx.RoomID) {
			return nil
		}

		p.mu.Lock()
		game, active := p.games[ctx.RoomID]
		p.mu.Unlock()

		if active && game != nil {
			content := ctx.Event.Content.AsMessage()
			if content != nil && content.RelatesTo != nil &&
				content.RelatesTo.Type == event.RelThread &&
				content.RelatesTo.EventID == game.threadID {
				guess := strings.TrimSpace(ctx.Body)
				if guess != "" {
					return p.handleGuess(ctx, guess)
				}
			}
		}
		return nil
	}

	if !isGamesRoom(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Games are only available in the games channel!")
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "hangman"))

	lower := strings.ToLower(args)
	switch {
	case args == "" || lower == "start":
		return p.handleStart(ctx, "")
	case strings.HasPrefix(lower, "start "):
		return p.handleStart(ctx, strings.TrimSpace(args[6:]))
	case strings.HasPrefix(lower, "submit "):
		return p.handleSubmit(ctx, strings.TrimSpace(args[7:]))
	case lower == "skip":
		return p.handleSkip(ctx)
	default:
		return p.handleGuess(ctx, args)
	}
}

// ---------------------------------------------------------------------------
// Phrase management
// ---------------------------------------------------------------------------

func (p *HangmanPlugin) loadPhrases(path string) {
	f, err := os.Open(path)
	if err != nil {
		slog.Warn("hangman: failed to load phrases", "path", path, "err", err)
		return
	}
	defer f.Close()

	var phrases []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			phrases = append(phrases, line)
		}
	}
	p.phrases = phrases
	slog.Info("hangman: phrases loaded", "count", len(phrases))
}

func (p *HangmanPlugin) addPhrase(phrase string) error {
	path := os.Getenv("HANGMAN_PHRASE_FILE")
	if path == "" {
		return fmt.Errorf("no phrase file configured")
	}

	// Duplicate check
	for _, existing := range p.phrases {
		if strings.EqualFold(existing, phrase) {
			return fmt.Errorf("duplicate phrase")
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, phrase)
	if err != nil {
		return err
	}

	p.phrases = append(p.phrases, phrase)
	return nil
}

// ---------------------------------------------------------------------------
// Game commands
// ---------------------------------------------------------------------------

func (p *HangmanPlugin) handleStart(ctx MessageContext, difficulty string) error {
	p.mu.Lock()

	if _, active := p.games[ctx.RoomID]; active {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "A Hangman game is already in progress!")
	}

	if len(p.phrases) == 0 {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No phrases loaded. Ask an admin to set up HANGMAN_PHRASE_FILE.")
	}

	// Filter by difficulty if specified
	pool := p.phrases
	if difficulty != "" {
		var tier *hangmanTier
		for i := range hangmanTiers {
			if strings.EqualFold(hangmanTiers[i].Name, difficulty) {
				tier = &hangmanTiers[i]
				break
			}
		}
		if tier == nil {
			p.mu.Unlock()
			names := make([]string, len(hangmanTiers))
			for i, t := range hangmanTiers {
				names[i] = t.Name
			}
			return p.SendReply(ctx.RoomID, ctx.EventID,
				fmt.Sprintf("Unknown difficulty. Options: %s", strings.Join(names, ", ")))
		}
		var filtered []string
		for _, ph := range p.phrases {
			if n := len(ph); n >= tier.Min && n <= tier.Max {
				filtered = append(filtered, ph)
			}
		}
		if len(filtered) == 0 {
			p.mu.Unlock()
			return p.SendReply(ctx.RoomID, ctx.EventID,
				fmt.Sprintf("No phrases available for **%s** difficulty.", tier.Name))
		}
		pool = filtered
	}

	phrase := pool[rand.IntN(len(pool))]
	game := newHangmanGame(phrase, p.maxWrong)

	// Reserve the slot so no concurrent start can race us
	p.games[ctx.RoomID] = game
	p.mu.Unlock()

	// Create thread root message (network I/O outside mutex)
	threadRoot := fmt.Sprintf(
		"🎮 **Hangman!** Tier: **%s** | %d guesses allowed\n\n```\n%s\n```\n%s\n\nReply in this thread with a letter or the full phrase to guess!",
		game.tier.Name, game.maxWrong, gallows[0], game.displayPhrase(),
	)
	eventID, err := p.SendMessageID(ctx.RoomID, threadRoot)
	if err != nil {
		// Roll back reservation
		p.mu.Lock()
		delete(p.games, ctx.RoomID)
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	game.threadID = eventID
	p.mu.Unlock()

	// Send an initial thread reply to materialize the thread in clients
	return p.SendThread(ctx.RoomID, eventID, fmt.Sprintf(
		"```\n%s\n```\n%s\n\nWrong guesses: none",
		gallows[0], game.displayPhrase(),
	))
}

func (p *HangmanPlugin) handleGuess(ctx MessageContext, guess string) error {
	p.mu.Lock()
	game, active := p.games[ctx.RoomID]
	if !active {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No Hangman game in progress. Start one with `!hangman start`")
	}

	// Game exists but thread not yet created (start in progress)
	if game.threadID == "" {
		p.mu.Unlock()
		return nil
	}

	// Register participant (under lock)
	game.participants[ctx.Sender] = true
	p.mu.Unlock()

	guess = strings.TrimSpace(guess)

	// Single letter guess
	if len([]rune(guess)) == 1 {
		ch := []rune(guess)[0]
		if !unicode.IsLetter(ch) {
			return p.SendThread(ctx.RoomID, game.threadID, "Please guess a letter.")
		}
		return p.processLetterGuess(ctx, game, ch)
	}

	// Full solution attempt
	return p.processSolutionGuess(ctx, game, guess)
}

func (p *HangmanPlugin) processLetterGuess(ctx MessageContext, game *hangmanGame, ch rune) error {
	// All game state mutations under lock
	p.mu.Lock()
	hit, already := game.guessLetter(ch)

	if already {
		threadID := game.threadID
		p.mu.Unlock()
		return p.SendThread(ctx.RoomID, threadID,
			fmt.Sprintf("'%c' was already guessed.", unicode.ToUpper(ch)))
	}

	// Snapshot state for messages before unlocking
	display := game.displayPhrase()
	wrongStr := game.wrongGuessStr()
	remaining := game.remaining()
	fullyRevealed := false
	dead := false

	if hit {
		fullyRevealed = game.isFullyRevealed()
		if fullyRevealed {
			game.solved = true
			game.solvedBy = ctx.Sender
		}
	} else {
		dead = game.isDead()
	}

	wrongCount := game.wrongCount()
	p.mu.Unlock()

	if hit {
		if fullyRevealed {
			return p.endGame(ctx.RoomID, game)
		}
		return p.SendThread(ctx.RoomID, game.threadID, fmt.Sprintf(
			"✅ '%c' is in the phrase! (%d guesses remaining)\n%s\n\nWrong guesses: %s",
			unicode.ToUpper(ch), remaining, display, wrongStr,
		))
	}

	// Wrong guess
	if dead {
		return p.endGame(ctx.RoomID, game)
	}

	return p.SendThread(ctx.RoomID, game.threadID, fmt.Sprintf(
		"❌ Wrong! (%d guesses remaining)\n```\n%s\n```\n%s\n\nWrong guesses: %s",
		remaining, gallows[wrongCount], display, wrongStr,
	))
}

func (p *HangmanPlugin) processSolutionGuess(ctx MessageContext, game *hangmanGame, guess string) error {
	p.mu.Lock()
	if game.guessSolution(guess) {
		game.solved = true
		game.solvedBy = ctx.Sender
		if game.revealedCount() < game.letterCount()/2 {
			game.earlySolve = true
		}
		p.mu.Unlock()
		return p.endGame(ctx.RoomID, game)
	}

	// Wrong solution — costs a life
	game.wrongGuesses = append(game.wrongGuesses, '?')
	dead := game.isDead()
	remaining := game.remaining()
	wrongCount := game.wrongCount()
	display := game.displayPhrase()
	wrongStr := game.wrongGuessStr()
	p.mu.Unlock()

	if dead {
		return p.endGame(ctx.RoomID, game)
	}

	return p.SendThread(ctx.RoomID, game.threadID, fmt.Sprintf(
		"❌ Wrong solution! (%d guesses remaining)\n```\n%s\n```\n%s\n\nWrong guesses: %s",
		remaining, gallows[wrongCount], display, wrongStr,
	))
}

func (p *HangmanPlugin) endGame(roomID id.RoomID, game *hangmanGame) error {
	p.mu.Lock()
	delete(p.games, roomID)
	p.mu.Unlock()

	if !game.solved {
		// Loss — broadcast to room
		return p.SendMessage(roomID, fmt.Sprintf(
			"💀 Game over! The phrase was:\n\"%s\"\n```\n%s\n```\nNobody solved it this time.",
			game.phrase, gallows[6],
		))
	}

	// Calculate payouts
	eligibleParticipants := make([]id.UserID, 0, len(game.participants))
	for uid := range game.participants {
		eligibleParticipants = append(eligibleParticipants, uid)
	}

	solverName := p.DisplayName(game.solvedBy)
	participantCount := len(eligibleParticipants)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎉 Solved by **%s**!\n\"%s\"\nTier: %s | Participants: %d\n\n",
		solverName, game.phrase, game.tier.Name, participantCount,
	))

	if participantCount > 0 {
		basePot := game.tier.Bonus
		share := basePot / float64(participantCount)

		sb.WriteString("**Payouts:**\n")
		for _, uid := range eligibleParticipants {
			payout := share
			label := ""
			if uid == game.solvedBy && game.earlySolve {
				payout = share * p.bonusMul
				label = " (early solve bonus!)"
			}
			p.euro.Credit(uid, payout, "hangman_win")
			p.recordHangmanScore(uid, payout)
			name := p.DisplayName(uid)
			sb.WriteString(fmt.Sprintf("  **%s**: +€%d%s\n", name, int(payout), label))
		}
	}

	return p.SendMessage(roomID, sb.String())
}

func (p *HangmanPlugin) handleSkip(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}

	p.mu.Lock()
	game, active := p.games[ctx.RoomID]
	if !active {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No game in progress.")
	}
	delete(p.games, ctx.RoomID)
	p.mu.Unlock()

	return p.SendMessage(ctx.RoomID, fmt.Sprintf(
		"⏭️ Game skipped! The phrase was:\n\"%s\"", game.phrase))
}

func (p *HangmanPlugin) handleSubmit(ctx MessageContext, phrase string) error {
	if len(phrase) < 8 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Phrase must be at least 8 characters.")
	}

	// LLM screening
	ollamaHost := os.Getenv("OLLAMA_HOST")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaHost == "" || ollamaModel == "" {
		// No LLM available — add directly
		if err := p.addPhrase(phrase); err != nil {
			if err.Error() == "duplicate phrase" {
				return p.SendReply(ctx.RoomID, ctx.EventID, "That phrase already exists.")
			}
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save phrase.")
		}
		_ = p.SendDM(ctx.Sender, "Your phrase has been added to the Hangman pool. Thanks!")
		return p.SendReply(ctx.RoomID, ctx.EventID, "Phrase submitted and added!")
	}

	prompt := fmt.Sprintf(`You are screening community submissions for a Hangman game. Evaluate only whether the phrase is offensive, hateful, sexually explicit, or otherwise inappropriate for a general adult audience.

Respond only in JSON:
{ "approved": true }
or
{ "approved": false, "reason": "one sentence explanation" }

Phrase: %s`, phrase)

	result, err := callOllama(ollamaHost, ollamaModel, prompt)
	if err != nil {
		slog.Error("hangman: LLM screening failed", "err", err)
		// Fail open — add it
		if addErr := p.addPhrase(phrase); addErr != nil {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save phrase.")
		}
		_ = p.SendDM(ctx.Sender, "Your phrase has been added to the Hangman pool. Thanks!")
		return p.SendReply(ctx.RoomID, ctx.EventID, "Phrase submitted and added!")
	}

	var screening struct {
		Approved bool   `json:"approved"`
		Reason   string `json:"reason"`
	}

	// Extract JSON from response
	jsonStr := result
	if idx := strings.Index(result, "{"); idx >= 0 {
		if end := strings.LastIndex(result, "}"); end >= idx {
			jsonStr = result[idx : end+1]
		}
	}

	if err := json.Unmarshal([]byte(jsonStr), &screening); err != nil {
		// Parse failed — add it
		if addErr := p.addPhrase(phrase); addErr != nil {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save phrase.")
		}
		_ = p.SendDM(ctx.Sender, "Your phrase has been added to the Hangman pool. Thanks!")
		return p.SendReply(ctx.RoomID, ctx.EventID, "Phrase submitted and added!")
	}

	if !screening.Approved {
		_ = p.SendDM(ctx.Sender, fmt.Sprintf("Your Hangman phrase was not approved: %s", screening.Reason))
		return p.SendReply(ctx.RoomID, ctx.EventID, "Phrase reviewed — check your DMs for details.")
	}

	if err := p.addPhrase(phrase); err != nil {
		if err.Error() == "duplicate phrase" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "That phrase already exists.")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save phrase.")
	}

	_ = p.SendDM(ctx.Sender, "Your phrase has been added to the Hangman pool. Thanks!")
	return p.SendReply(ctx.RoomID, ctx.EventID, "Phrase submitted and added!")
}

// ---------------------------------------------------------------------------
// Leaderboard
// ---------------------------------------------------------------------------

func (p *HangmanPlugin) handleBoard(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, total_earned, games_won FROM hangman_scores ORDER BY total_earned DESC LIMIT 10`,
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch leaderboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🎮 **Hangman Leaderboard**\n\n")
	rank := 0
	for rows.Next() {
		var userID string
		var earned float64
		var won int
		rows.Scan(&userID, &earned, &won)
		rank++
		name := p.DisplayName(id.UserID(userID))
		sb.WriteString(fmt.Sprintf("%d. **%s** — €%d earned (%d wins)\n", rank, name, int(earned), won))
	}

	if rank == 0 {
		sb.WriteString("No games played yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *HangmanPlugin) recordHangmanScore(userID id.UserID, earned float64) {
	db.Exec("hangman: record score",
		`INSERT INTO hangman_scores (user_id, total_earned, games_played, games_won)
		 VALUES (?, ?, 1, 1)
		 ON CONFLICT(user_id) DO UPDATE SET
		   total_earned = total_earned + ?,
		   games_played = games_played + 1,
		   games_won = games_won + 1`,
		string(userID), earned, earned,
	)
}

