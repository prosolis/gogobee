package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// WordlePlugin provides a daily cooperative Wordle game.
type WordlePlugin struct {
	Base
	euro          *EuroPlugin
	apiKey        string
	httpClient    *http.Client
	defaultLength int

	mu      sync.Mutex
	puzzles map[id.RoomID]*WordlePuzzle

	// In-memory cache of validated words per puzzle to avoid redundant API calls.
	validCache map[string]map[string]bool // puzzleID -> word -> valid
}

// NewWordlePlugin creates a new WordlePlugin.
func NewWordlePlugin(client *mautrix.Client, euro *EuroPlugin) *WordlePlugin {
	length := 5
	if v := os.Getenv("WORDLE_DEFAULT_LENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 && n <= 7 {
			length = n
		}
	}

	return &WordlePlugin{
		Base:          NewBase(client),
		euro:          euro,
		apiKey:        os.Getenv("WORDNIK_API_KEY"),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		defaultLength: length,
		puzzles:       make(map[id.RoomID]*WordlePuzzle),
		validCache:    make(map[string]map[string]bool),
	}
}

func (p *WordlePlugin) Name() string { return "wordle" }

func (p *WordlePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "wordle", Description: "Guess today's Wordle", Usage: "!wordle <word>", Category: "Games"},
	}
}

func (p *WordlePlugin) Init() error {
	// Rehydrate today's puzzle from DB if it exists.
	p.rehydratePuzzles()

	// Start the midnight ticker for auto-posting.
	go p.midnightTicker()

	return nil
}

func (p *WordlePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *WordlePlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "wordle") {
		return nil
	}

	if !isGamesRoom(ctx.RoomID) {
		gr := gamesRoom()
		if gr != "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Games are only available in the games channel!")
		}
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "wordle"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!wordle <word>` — guess today's puzzle.\nSee `!wordle help` for all commands.")
	}

	switch {
	case args == "help":
		return p.handleHelp(ctx)
	case args == "stats":
		return p.handleStats(ctx)
	case args == "grid":
		return p.handleGrid(ctx)
	case args == "new" || strings.HasPrefix(args, "new "):
		return p.handleNew(ctx, args)
	case args == "skip":
		return p.handleSkip(ctx)
	default:
		return p.handleGuess(ctx, args)
	}
}

func (p *WordlePlugin) handleHelp(ctx MessageContext) error {
	return p.SendReply(ctx.RoomID, ctx.EventID,
		"🟩 **Wordle Commands**\n\n"+
			"`!wordle <word>` — Submit a guess\n"+
			"`!wordle grid` — Re-post current puzzle grid\n"+
			"`!wordle stats` — All-time leaderboard\n"+
			"`!wordle new` — Start a new puzzle (admin)\n"+
			"`!wordle new <5|6|7>` — New puzzle with specific length (admin)\n"+
			"`!wordle skip` — Reveal answer and end puzzle (admin)")
}

func (p *WordlePlugin) handleGuess(ctx MessageContext, guess string) error {
	guess = strings.ToUpper(strings.TrimSpace(guess))

	p.mu.Lock()
	defer p.mu.Unlock()

	puzzle := p.puzzles[ctx.RoomID]
	if puzzle == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active puzzle. An admin can start one with `!wordle new`.")
	}

	if puzzle.Solved || puzzle.Failed {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Today's puzzle is already over. A new one starts at midnight UTC!")
	}

	// Check length.
	if len([]rune(guess)) != puzzle.WordLength {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Guesses must be %d letters.", puzzle.WordLength))
	}

	// Check for non-alphabetic characters.
	for _, r := range guess {
		if r < 'A' || r > 'Z' {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Guesses must contain only letters.")
		}
	}

	// Check duplicate guess.
	for _, g := range puzzle.Guesses {
		if g.Word == guess {
			return p.SendReply(ctx.RoomID, ctx.EventID,
				fmt.Sprintf("**%s** has already been tried.", guess))
		}
	}

	// Validate word via Wordnik (with caching).
	valid, apiErr := p.isValidWord(puzzle.PuzzleID, guess)
	if apiErr {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Word validation is temporarily unavailable. Try again in a moment.")
	}
	if !valid {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("❌ **%s** is not a valid word.", guess))
	}

	// Get display name.
	displayName := p.DisplayName(ctx.Sender)

	// Score the guess.
	results := scoreGuess(guess, puzzle.Answer)
	updateLetterStates(puzzle.LetterStates, guess, results)

	now := time.Now().UTC()
	g := WordleGuess{
		Word:       guess,
		PlayerID:   ctx.Sender,
		PlayerName: displayName,
		Results:    results,
		Timestamp:  now,
	}
	puzzle.Guesses = append(puzzle.Guesses, g)

	// Check for win.
	if guess == puzzle.Answer {
		puzzle.Solved = true
		puzzle.SolvedAt = &now

		definition := p.fetchDefinition(puzzle.Answer)
		p.updateStats(puzzle)
		p.markPuzzleDone(puzzle)
		payouts := p.awardPrize(puzzle)

		return p.SendMessage(ctx.RoomID, renderSolvedAnnouncement(puzzle, definition, payouts))
	}

	// Check for failure (all guesses used).
	if len(puzzle.Guesses) >= puzzle.MaxGuesses {
		puzzle.Failed = true

		definition := p.fetchDefinition(puzzle.Answer)
		p.updateStats(puzzle)
		p.markPuzzleDone(puzzle)

		return p.SendMessage(ctx.RoomID, renderFailedAnnouncement(puzzle, definition))
	}

	// Post updated grid.
	return p.SendMessage(ctx.RoomID, renderWordleGrid(puzzle))
}

func (p *WordlePlugin) handleGrid(ctx MessageContext) error {
	p.mu.Lock()
	puzzle := p.puzzles[ctx.RoomID]
	p.mu.Unlock()

	if puzzle == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active puzzle.")
	}

	if len(puzzle.Guesses) == 0 {
		hint := ""
		if isCustomAllowedWord(puzzle.Answer) {
			hint = "Today's word is video game related"
		}
		return p.SendReply(ctx.RoomID, ctx.EventID,
			renderWordleStartAnnouncement(puzzle.PuzzleNumber, puzzle.WordLength, hint))
	}

	return p.SendMessage(ctx.RoomID, renderWordleGrid(puzzle))
}

func (p *WordlePlugin) handleNew(ctx MessageContext, args string) error {
	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can start a new puzzle.")
	}

	wordLength := p.defaultLength
	parts := strings.Fields(args)
	if len(parts) > 1 {
		if n, err := strconv.Atoi(parts[1]); err == nil && n >= 5 && n <= 7 {
			wordLength = n
		} else {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Word length must be 5, 6, or 7.")
		}
	}

	p.mu.Lock()
	existing := p.puzzles[ctx.RoomID]
	if existing != nil && !existing.Solved && !existing.Failed {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"There's already an active puzzle. Use `!wordle skip` to end it first.")
	}
	p.mu.Unlock()

	return p.createAndPostPuzzle(ctx.RoomID, wordLength)
}

func (p *WordlePlugin) handleSkip(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can skip a puzzle.")
	}

	p.mu.Lock()
	puzzle := p.puzzles[ctx.RoomID]
	if puzzle == nil || puzzle.Solved || puzzle.Failed {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active puzzle to skip.")
	}
	puzzle.Failed = true
	p.markPuzzleDone(puzzle)
	p.mu.Unlock()

	definition := p.fetchDefinition(puzzle.Answer)
	defLine := ""
	if definition != "" {
		defLine = fmt.Sprintf("\n📖 *%s*\n", definition)
	}

	return p.SendMessage(ctx.RoomID,
		fmt.Sprintf("⏭️ **Puzzle skipped.**\nThe word was **%s**.%s", puzzle.Answer, defLine))
}

func (p *WordlePlugin) handleStats(ctx MessageContext) error {
	d := db.Get()

	// Fetch top 10.
	rows, err := d.Query(
		`SELECT user_id, display_name, total_guesses, puzzles_played, puzzles_solved, winning_guesses
		 FROM wordle_stats ORDER BY puzzles_solved DESC, winning_guesses DESC LIMIT 10`)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load stats.")
	}
	defer rows.Close()

	var stats []WordlePlayerStat
	for rows.Next() {
		var s WordlePlayerStat
		if err := rows.Scan(&s.UserID, &s.DisplayName, &s.TotalGuesses, &s.PuzzlesPlayed, &s.PuzzlesSolved, &s.WinningGuesses); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	if len(stats) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No Wordle stats yet. Play some puzzles first!")
	}

	streak := p.communityStreak()
	return p.SendMessage(ctx.RoomID, renderWordleLeaderboard(stats, streak))
}

// createAndPostPuzzle creates a new puzzle, persists it, and posts the announcement.
func (p *WordlePlugin) createAndPostPuzzle(roomID id.RoomID, wordLength int) error {
	// Use local word list for puzzle selection. Wordnik's randomWord endpoint
	// requires a paid API tier; we keep Wordnik for guess validation and definitions only.
	word := pickFallbackWord(wordLength)
	if word == "" {
		return p.SendMessage(roomID, "Failed to select a puzzle word — no words available for that length.")
	}

	puzzleNumber := p.nextPuzzleNumber()
	today := time.Now().UTC().Format("2006-01-02")
	now := time.Now().UTC()

	puzzle := &WordlePuzzle{
		PuzzleID:     today,
		PuzzleNumber: puzzleNumber,
		RoomID:       roomID,
		Answer:       word,
		WordLength:   wordLength,
		MaxGuesses:   6,
		StartedAt:    now,
		LetterStates: make(map[rune]LetterResult),
	}

	// Persist to DB.
	db.Exec("wordle: persist puzzle",
		`INSERT INTO wordle_puzzles (puzzle_id, room_id, answer, word_length, solved, guess_count, started_at)
		 VALUES (?, ?, ?, ?, 0, 0, ?)
		 ON CONFLICT(puzzle_id, room_id) DO UPDATE SET answer = ?, word_length = ?, solved = 0, guess_count = 0, started_at = ?`,
		today, string(roomID), word, wordLength, now,
		word, wordLength, now,
	)

	p.mu.Lock()
	p.puzzles[roomID] = puzzle
	// Evict stale cache entries from previous days.
	for key := range p.validCache {
		if key != today {
			delete(p.validCache, key)
		}
	}
	p.validCache[today] = make(map[string]bool)
	p.mu.Unlock()

	hint := ""
	if isCustomAllowedWord(word) {
		hint = "Today's word is video game related"
	}
	return p.SendMessage(roomID, renderWordleStartAnnouncement(puzzleNumber, wordLength, hint))
}

// PostDailyPuzzle is called by the scheduler to post today's puzzle.
func (p *WordlePlugin) PostDailyPuzzle(roomID id.RoomID) error {
	today := time.Now().UTC().Format("2006-01-02")

	// Check if already posted today.
	p.mu.Lock()
	existing := p.puzzles[roomID]
	if existing != nil && existing.PuzzleID == today {
		p.mu.Unlock()
		return nil // already posted
	}
	p.mu.Unlock()

	return p.createAndPostPuzzle(roomID, p.defaultLength)
}

// isValidWord checks if a word is valid, using the in-memory cache first,
// then the custom allow-list, then Wordnik.
// Returns (valid, apiError). apiError is true when the API is unreachable.
func (p *WordlePlugin) isValidWord(puzzleID, word string) (bool, bool) {
	cache := p.validCache[puzzleID]
	if cache == nil {
		cache = make(map[string]bool)
		p.validCache[puzzleID] = cache
	}

	if valid, ok := cache[word]; ok {
		return valid, false
	}

	// Check custom allow-list (game titles, etc.) before hitting the API.
	if isCustomAllowedWord(word) {
		cache[word] = true
		return true, false
	}

	valid, apiErr := wordnikValidateWord(p.apiKey, p.httpClient, word)
	if !apiErr {
		cache[word] = valid // only cache definitive results
	}
	return valid, apiErr
}

func (p *WordlePlugin) fetchDefinition(word string) string {
	return wordnikFetchDefinitionText(p.apiKey, p.httpClient, word)
}


func (p *WordlePlugin) nextPuzzleNumber() int {
	d := db.Get()
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM wordle_puzzles`).Scan(&count)
	if err != nil {
		return 1
	}
	return count + 1
}

func (p *WordlePlugin) markPuzzleDone(puzzle *WordlePuzzle) {
	solved := 0
	if puzzle.Solved {
		solved = 1
	}
	db.Exec("wordle: mark puzzle done",
		`UPDATE wordle_puzzles SET solved = ?, guess_count = ?, solved_at = ? WHERE puzzle_id = ? AND room_id = ?`,
		solved, len(puzzle.Guesses), puzzle.SolvedAt, puzzle.PuzzleID, string(puzzle.RoomID),
	)
}

func (p *WordlePlugin) updateStats(puzzle *WordlePuzzle) {
	d := db.Get()

	// Tally per-player contributions.
	type contrib struct {
		name    string
		guesses int
		solved  bool
	}
	players := map[id.UserID]*contrib{}
	for i, g := range puzzle.Guesses {
		c, ok := players[g.PlayerID]
		if !ok {
			c = &contrib{name: g.PlayerName}
			players[g.PlayerID] = c
		}
		c.guesses++
		if i == len(puzzle.Guesses)-1 && puzzle.Solved {
			c.solved = true
		}
	}

	tx, err := d.Begin()
	if err != nil {
		slog.Error("wordle: begin tx", "err", err)
		return
	}
	defer tx.Rollback()

	for uid, c := range players {
		puzzlesSolved := 0
		if puzzle.Solved {
			puzzlesSolved = 1
		}
		winningGuesses := 0
		if c.solved {
			winningGuesses = 1
		}

		_, err := tx.Exec(
			`INSERT INTO wordle_stats (user_id, display_name, total_guesses, puzzles_played, puzzles_solved, winning_guesses)
			 VALUES (?, ?, ?, 1, ?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET
			   display_name = ?,
			   total_guesses = total_guesses + ?,
			   puzzles_played = puzzles_played + 1,
			   puzzles_solved = puzzles_solved + ?,
			   winning_guesses = winning_guesses + ?,
			   updated_at = CURRENT_TIMESTAMP`,
			string(uid), c.name, c.guesses, puzzlesSolved, winningGuesses,
			c.name, c.guesses, puzzlesSolved, winningGuesses,
		)
		if err != nil {
			slog.Error("wordle: update stats", "user", uid, "err", err)
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("wordle: commit stats", "err", err)
	}
}

// WordlePayout tracks a payout for rendering.
type WordlePayout struct {
	Name   string
	Amount int
	Solver bool
}

// wordleBasePots maps guess count to euro prize pot. Fewer guesses = bigger reward.
var wordleBasePots = [7]int{0, 100, 80, 60, 45, 35, 25}

// awardPrize credits euros to all contributors when a puzzle is solved.
// The solver who got the final guess gets a 50% bonus on their share.
func (p *WordlePlugin) awardPrize(puzzle *WordlePuzzle) []WordlePayout {
	if !puzzle.Solved || p.euro == nil {
		return nil
	}

	guessesUsed := len(puzzle.Guesses)
	pot := 25
	if guessesUsed >= 1 && guessesUsed < len(wordleBasePots) {
		pot = wordleBasePots[guessesUsed]
	}

	// Tally contributors.
	type info struct {
		name   string
		solver bool
	}
	contributors := map[id.UserID]*info{}
	var order []id.UserID
	for i, g := range puzzle.Guesses {
		if _, ok := contributors[g.PlayerID]; !ok {
			contributors[g.PlayerID] = &info{name: g.PlayerName}
			order = append(order, g.PlayerID)
		}
		if i == len(puzzle.Guesses)-1 {
			contributors[g.PlayerID].solver = true
		}
	}

	numPlayers := len(contributors)
	share := pot / numPlayers
	if share < 5 {
		share = 5
	}
	solverBonus := share / 2

	var payouts []WordlePayout
	for _, uid := range order {
		c := contributors[uid]
		amount := share
		if c.solver {
			amount += solverBonus
		}
		p.euro.Credit(uid, float64(amount), "wordle_win")
		payouts = append(payouts, WordlePayout{Name: c.name, Amount: amount, Solver: c.solver})
	}
	return payouts
}

func (p *WordlePlugin) communityStreak() int {
	d := db.Get()
	rows, err := d.Query(
		`SELECT puzzle_id, solved FROM wordle_puzzles ORDER BY puzzle_id DESC LIMIT 100`)
	if err != nil {
		return 0
	}
	defer rows.Close()

	streak := 0
	for rows.Next() {
		var pid string
		var solved int
		if err := rows.Scan(&pid, &solved); err != nil {
			break
		}
		if solved == 1 {
			streak++
		} else {
			break
		}
	}
	return streak
}

func (p *WordlePlugin) rehydratePuzzles() {
	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	// Collect rows first, then close the cursor before doing any nested queries.
	// SQLite deadlocks if a nested query runs while rows are still open on a
	// single-connection pool.
	type puzzleRow struct {
		pid        string
		roomStr    string
		answer     string
		wordLength int
		startedAt  time.Time
	}

	rows, err := d.Query(
		`SELECT puzzle_id, room_id, answer, word_length, solved, guess_count, started_at
		 FROM wordle_puzzles WHERE puzzle_id = ?`, today)
	if err != nil {
		slog.Warn("wordle: rehydrate query failed", "err", err)
		return
	}

	var pending []puzzleRow
	for rows.Next() {
		var pid, roomStr, answer string
		var wordLength, solved, guessCount int
		var startedAt time.Time
		if err := rows.Scan(&pid, &roomStr, &answer, &wordLength, &solved, &guessCount, &startedAt); err != nil {
			continue
		}

		if solved == 1 || guessCount >= 6 {
			continue // already done
		}

		pending = append(pending, puzzleRow{pid, roomStr, answer, wordLength, startedAt})
	}
	rows.Close()

	for _, pr := range pending {
		roomID := id.RoomID(pr.roomStr)

		// Get puzzle number (safe now — no open cursor).
		var puzzleNumber int
		err := d.QueryRow(`SELECT COUNT(*) FROM wordle_puzzles WHERE puzzle_id <= ?`, pr.pid).Scan(&puzzleNumber)
		if err != nil {
			puzzleNumber = 1
		}

		puzzle := &WordlePuzzle{
			PuzzleID:     pr.pid,
			PuzzleNumber: puzzleNumber,
			RoomID:       roomID,
			Answer:       pr.answer,
			WordLength:   pr.wordLength,
			MaxGuesses:   6,
			StartedAt:    pr.startedAt,
			LetterStates: make(map[rune]LetterResult),
		}

		p.mu.Lock()
		p.puzzles[roomID] = puzzle
		p.validCache[pr.pid] = make(map[string]bool)
		p.mu.Unlock()

		slog.Info("wordle: rehydrated puzzle", "room", roomID, "answer_len", pr.wordLength)
	}
}

// midnightTicker checks every minute if it's time to post a new daily puzzle.
func (p *WordlePlugin) midnightTicker() {
	// Check immediately on startup.
	p.checkAndPostDaily()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	lastDate := time.Now().UTC().Format("2006-01-02")
	for range ticker.C {
		today := time.Now().UTC().Format("2006-01-02")
		if today != lastDate {
			lastDate = today
			p.checkAndPostDaily()
		}
	}
}

func (p *WordlePlugin) checkAndPostDaily() {
	gr := gamesRoom()
	if gr == "" {
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	// Check if today's puzzle already exists for this room.
	var exists int
	err := d.QueryRow(
		`SELECT 1 FROM wordle_puzzles WHERE puzzle_id = ? AND room_id = ?`,
		today, string(gr),
	).Scan(&exists)
	if err == nil {
		return // already exists
	}
	if err != sql.ErrNoRows {
		slog.Error("wordle: check daily puzzle", "err", err)
		return
	}

	slog.Info("wordle: posting daily puzzle", "room", gr)
	if err := p.createAndPostPuzzle(gr, p.defaultLength); err != nil {
		slog.Error("wordle: daily puzzle failed", "err", err)
	}
}
