package plugin

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// categoryMap maps category IDs to human-readable names.
var categoryMap = map[int]string{
	9:  "General Knowledge",
	10: "Books",
	11: "Film",
	12: "Music",
	15: "Video Games",
	17: "Science & Nature",
	18: "Computers",
	21: "Sports",
	22: "Geography",
	23: "History",
}

// categoryNameToID maps lowercase category names to IDs.
var categoryNameToID = map[string]int{
	"general":   9,
	"books":     10,
	"film":      11,
	"movie":     11,
	"movies":    11,
	"music":     12,
	"games":     15,
	"gaming":    15,
	"science":   17,
	"computers": 18,
	"tech":      18,
	"sports":    21,
	"geography": 22,
	"geo":       22,
	"history":   23,
}

type openTDBResponse struct {
	ResponseCode int             `json:"response_code"`
	Results      []openTDBResult `json:"results"`
}

type openTDBResult struct {
	Category         string   `json:"category"`
	Type             string   `json:"type"`
	Difficulty       string   `json:"difficulty"`
	Question         string   `json:"question"`
	CorrectAnswer    string   `json:"correct_answer"`
	IncorrectAnswers []string `json:"incorrect_answers"`
}

type activeSession struct {
	RoomID         id.RoomID
	QuestionEventID id.EventID
	Question       string
	CorrectAnswer  string
	AllAnswers     []string
	CorrectIndex   int
	StartedAt      time.Time
	Difficulty     string
	Category       string
}

// TriviaPlugin handles trivia game sessions.
type TriviaPlugin struct {
	Base
	mu       sync.Mutex
	sessions map[id.RoomID]*activeSession
	enabled  bool
}

// NewTriviaPlugin creates a new TriviaPlugin.
func NewTriviaPlugin(client *mautrix.Client) *TriviaPlugin {
	return &TriviaPlugin{
		Base:     NewBase(client),
		sessions: make(map[id.RoomID]*activeSession),
		enabled:  os.Getenv("FEATURE_TRIVIA") != "false",
	}
}

func (p *TriviaPlugin) Name() string { return "trivia" }

func (p *TriviaPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "trivia", Description: "Start a trivia question", Usage: "!trivia [category] [easy|medium|hard]", Category: "Fun & Games"},
		{Name: "trivia scores", Description: "Room trivia leaderboard", Usage: "!trivia scores", Category: "Fun & Games"},
		{Name: "trivia categories", Description: "List available categories", Usage: "!trivia categories", Category: "Fun & Games"},
		{Name: "trivia fastest", Description: "Fastest correct answers", Usage: "!trivia fastest", Category: "Fun & Games"},
		{Name: "trivia stop", Description: "Stop current trivia session", Usage: "!trivia stop", Category: "Fun & Games"},
	}
}

func (p *TriviaPlugin) Init() error { return nil }

func (p *TriviaPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *TriviaPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		if p.IsCommand(ctx.Body, "trivia") {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Trivia is disabled.")
		}
		return nil
	}

	if p.IsCommand(ctx.Body, "trivia") {
		if !isGamesRoom(ctx.RoomID) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Games are only available in the games channel!")
		}
		args := p.GetArgs(ctx.Body, "trivia")
		return p.handleTrivia(ctx, args)
	}

	// Check if this is a thread reply to an active trivia question
	p.mu.Lock()
	session, ok := p.sessions[ctx.RoomID]
	p.mu.Unlock()

	if ok && session != nil {
		// Only accept answers in the trivia thread
		content := ctx.Event.Content.AsMessage()
		if content != nil && content.RelatesTo != nil &&
			content.RelatesTo.Type == event.RelThread &&
			content.RelatesTo.EventID == session.QuestionEventID {
			return p.handleAnswer(ctx, session)
		}
	}

	return nil
}

func (p *TriviaPlugin) handleTrivia(ctx MessageContext, args string) error {
	lower := strings.ToLower(strings.TrimSpace(args))

	switch {
	case lower == "scores":
		return p.showScores(ctx)
	case lower == "categories":
		return p.showCategories(ctx)
	case lower == "fastest":
		return p.showFastest(ctx)
	case lower == "stop":
		return p.stopSession(ctx)
	default:
		return p.startQuestion(ctx, args)
	}
}

func (p *TriviaPlugin) showCategories(ctx MessageContext) error {
	var sb strings.Builder
	sb.WriteString("📚 Trivia Categories:\n")
	for catID, name := range categoryMap {
		sb.WriteString(fmt.Sprintf("  • %s (%d)\n", name, catID))
	}
	sb.WriteString("\nUsage: !trivia [category] [easy|medium|hard]")
	return p.SendMessage(ctx.RoomID, sb.String())
}

func (p *TriviaPlugin) showScores(ctx MessageContext) error {
	rows, err := db.Get().Query(
		`SELECT user_id, correct, wrong, total_score
		 FROM trivia_scores
		 WHERE room_id = ?
		 ORDER BY total_score DESC
		 LIMIT 10`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("trivia: query scores", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch scores.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🏆 Trivia Leaderboard:\n")
	rank := 0
	found := false
	for rows.Next() {
		var userID string
		var correct, wrong, totalScore int
		if err := rows.Scan(&userID, &correct, &wrong, &totalScore); err != nil {
			continue
		}
		found = true
		rank++
		sb.WriteString(fmt.Sprintf("%d. %s — %d pts (%d correct, %d wrong)\n", rank, userID, totalScore, correct, wrong))
	}

	if !found {
		return p.SendMessage(ctx.RoomID, "No trivia scores yet in this room!")
	}

	return p.SendMessage(ctx.RoomID, sb.String())
}

func (p *TriviaPlugin) showFastest(ctx MessageContext) error {
	rows, err := db.Get().Query(
		`SELECT user_id, fastest_ms
		 FROM trivia_scores
		 WHERE room_id = ? AND fastest_ms > 0
		 ORDER BY fastest_ms ASC
		 LIMIT 10`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("trivia: query fastest", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch fastest times.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("⚡ Fastest Correct Answers:\n")
	rank := 0
	found := false
	for rows.Next() {
		var userID string
		var fastestMs int
		if err := rows.Scan(&userID, &fastestMs); err != nil {
			continue
		}
		found = true
		rank++
		sb.WriteString(fmt.Sprintf("%d. %s — %.2fs\n", rank, userID, float64(fastestMs)/1000.0))
	}

	if !found {
		return p.SendMessage(ctx.RoomID, "No fastest times recorded yet!")
	}

	return p.SendMessage(ctx.RoomID, sb.String())
}

func (p *TriviaPlugin) stopSession(ctx MessageContext) error {
	p.mu.Lock()
	_, ok := p.sessions[ctx.RoomID]
	if ok {
		delete(p.sessions, ctx.RoomID)
	}
	p.mu.Unlock()

	if !ok {
		return p.SendMessage(ctx.RoomID, "No active trivia session in this room.")
	}
	return p.SendMessage(ctx.RoomID, "Trivia session stopped.")
}

func (p *TriviaPlugin) startQuestion(ctx MessageContext, args string) error {
	p.mu.Lock()
	if _, ok := p.sessions[ctx.RoomID]; ok {
		p.mu.Unlock()
		return p.SendMessage(ctx.RoomID, "A trivia question is already active! Answer it or use !trivia stop.")
	}
	p.mu.Unlock()

	// Parse category and difficulty from args
	category := 0
	difficulty := ""
	parts := strings.Fields(strings.ToLower(args))

	for _, part := range parts {
		if part == "easy" || part == "medium" || part == "hard" {
			difficulty = part
			continue
		}
		if catID, ok := categoryNameToID[part]; ok {
			category = catID
		}
	}

	// Build API URL
	apiURL := "https://opentdb.com/api.php?amount=1"
	if category > 0 {
		apiURL += fmt.Sprintf("&category=%d", category)
	}
	if difficulty != "" {
		apiURL += fmt.Sprintf("&difficulty=%s", difficulty)
	}

	resp, err := http.Get(apiURL)
	if err != nil {
		slog.Error("trivia: API request failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch trivia question. Try again later.")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("trivia: read response", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to read trivia response.")
	}

	var apiResp openTDBResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		slog.Error("trivia: parse response", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to parse trivia response.")
	}

	if apiResp.ResponseCode != 0 || len(apiResp.Results) == 0 {
		return p.SendMessage(ctx.RoomID, "No trivia questions available for those criteria. Try different options!")
	}

	result := apiResp.Results[0]
	question := html.UnescapeString(result.Question)
	correctAnswer := html.UnescapeString(result.CorrectAnswer)

	// Build answer list
	var allAnswers []string
	correctIndex := 0

	if result.Type == "boolean" {
		allAnswers = []string{"True", "False"}
		if correctAnswer == "False" {
			correctIndex = 1
		}
	} else {
		// Shuffle answers
		incorrectAnswers := make([]string, len(result.IncorrectAnswers))
		for i, a := range result.IncorrectAnswers {
			incorrectAnswers[i] = html.UnescapeString(a)
		}
		allAnswers = append(incorrectAnswers, correctAnswer)
		// Fisher-Yates shuffle
		for i := len(allAnswers) - 1; i > 0; i-- {
			j := rand.IntN(i + 1)
			allAnswers[i], allAnswers[j] = allAnswers[j], allAnswers[i]
		}
		for i, a := range allAnswers {
			if a == correctAnswer {
				correctIndex = i
				break
			}
		}
	}

	// Format question message
	diffLabel := html.UnescapeString(result.Difficulty)
	catLabel := html.UnescapeString(result.Category)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🧠 **Trivia** [%s / %s]\n\n", catLabel, diffLabel))
	sb.WriteString(fmt.Sprintf("%s\n\n", question))

	letters := []string{"A", "B", "C", "D"}
	for i, ans := range allAnswers {
		if i < len(letters) {
			sb.WriteString(fmt.Sprintf("  **%s.** %s\n", letters[i], ans))
		}
	}
	sb.WriteString("\nReply with A, B, C, or D (or True/False). You have 30 seconds!")

	msgText := sb.String()

	// Store in DB
	incorrectJSON, _ := json.Marshal(result.IncorrectAnswers)
	_, err = db.Get().Exec(
		`INSERT INTO trivia_sessions (room_id, category, difficulty, question, correct_answer, incorrect_answers, question_type)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(ctx.RoomID), category, result.Difficulty, question, correctAnswer, string(incorrectJSON), result.Type,
	)
	if err != nil {
		slog.Error("trivia: insert session", "err", err)
	}

	// Send thread root announcement, then post the question in the thread
	threadRootID, err := p.SendMessageID(ctx.RoomID, "🧠 Trivia time! Reply in thread to answer.")
	if err != nil {
		return err
	}
	if err := p.SendThread(ctx.RoomID, threadRootID, msgText); err != nil {
		return err
	}

	session := &activeSession{
		RoomID:          ctx.RoomID,
		QuestionEventID: threadRootID,
		Question:        question,
		CorrectAnswer:   correctAnswer,
		AllAnswers:      allAnswers,
		CorrectIndex:    correctIndex,
		StartedAt:       time.Now(),
		Difficulty:      result.Difficulty,
		Category:        catLabel,
	}

	p.mu.Lock()
	p.sessions[ctx.RoomID] = session
	p.mu.Unlock()

	// Auto-expire after 30 seconds
	go func() {
		time.Sleep(30 * time.Second)
		p.mu.Lock()
		current, ok := p.sessions[ctx.RoomID]
		if ok && current == session {
			delete(p.sessions, ctx.RoomID)
			p.mu.Unlock()
			letters := []string{"A", "B", "C", "D"}
			ansLetter := ""
			if session.CorrectIndex < len(letters) {
				ansLetter = letters[session.CorrectIndex] + ". "
			}
			_ = p.SendThread(ctx.RoomID, session.QuestionEventID, fmt.Sprintf("⏰ Time's up! The correct answer was: **%s%s**", ansLetter, session.CorrectAnswer))
		} else {
			p.mu.Unlock()
		}
	}()

	return nil
}

func (p *TriviaPlugin) handleAnswer(ctx MessageContext, session *activeSession) error {
	answer := strings.TrimSpace(strings.ToLower(ctx.Body))

	// Map letter answers to index
	answerIndex := -1
	switch answer {
	case "a":
		answerIndex = 0
	case "b":
		answerIndex = 1
	case "c":
		answerIndex = 2
	case "d":
		answerIndex = 3
	case "true":
		// Find "True" in answers
		for i, a := range session.AllAnswers {
			if strings.ToLower(a) == "true" {
				answerIndex = i
				break
			}
		}
	case "false":
		for i, a := range session.AllAnswers {
			if strings.ToLower(a) == "false" {
				answerIndex = i
				break
			}
		}
	default:
		// Not a trivia answer, ignore
		return nil
	}

	if answerIndex < 0 || answerIndex >= len(session.AllAnswers) {
		return nil
	}

	elapsed := time.Since(session.StartedAt)
	elapsedMs := elapsed.Milliseconds()

	correct := answerIndex == session.CorrectIndex

	// Remove session
	p.mu.Lock()
	current, ok := p.sessions[ctx.RoomID]
	if !ok || current != session {
		p.mu.Unlock()
		return nil // Session already ended
	}
	delete(p.sessions, ctx.RoomID)
	p.mu.Unlock()

	if correct {
		// Calculate score: 100 at 3s, scaling to 0 at 30s
		score := calculateScore(elapsed)

		// Update scores in DB
		_, err := db.Get().Exec(
			`INSERT INTO trivia_scores (user_id, room_id, correct, wrong, total_score, fastest_ms)
			 VALUES (?, ?, 1, 0, ?, ?)
			 ON CONFLICT(user_id, room_id) DO UPDATE SET
			   correct = correct + 1,
			   total_score = total_score + ?,
			   fastest_ms = CASE WHEN fastest_ms = 0 OR ? < fastest_ms THEN ? ELSE fastest_ms END`,
			string(ctx.Sender), string(ctx.RoomID), score, elapsedMs,
			score, elapsedMs, elapsedMs,
		)
		if err != nil {
			slog.Error("trivia: update score", "err", err)
		}

		// Update session record (use subquery since SQLite doesn't support UPDATE...ORDER BY...LIMIT)
		if _, err := db.Get().Exec(
			`UPDATE trivia_sessions SET ended = 1, winner_id = ?, winner_time_ms = ?
			 WHERE id = (SELECT id FROM trivia_sessions WHERE room_id = ? AND ended = 0 ORDER BY started_at DESC LIMIT 1)`,
			string(ctx.Sender), elapsedMs, string(ctx.RoomID),
		); err != nil {
			slog.Error("trivia: update session", "err", err)
		}

		return p.SendThread(ctx.RoomID, session.QuestionEventID,
			fmt.Sprintf("✅ Correct! %s answered in %.2fs for %d points!", ctx.Sender, float64(elapsedMs)/1000.0, score))
	}

	// Wrong answer
	_, err := db.Get().Exec(
		`INSERT INTO trivia_scores (user_id, room_id, correct, wrong, total_score, fastest_ms)
		 VALUES (?, ?, 0, 1, 0, 0)
		 ON CONFLICT(user_id, room_id) DO UPDATE SET
		   wrong = wrong + 1`,
		string(ctx.Sender), string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("trivia: update wrong score", "err", err)
	}

	letters := []string{"A", "B", "C", "D"}
	ansLetter := ""
	if session.CorrectIndex < len(letters) {
		ansLetter = letters[session.CorrectIndex] + ". "
	}

	return p.SendThread(ctx.RoomID, session.QuestionEventID,
		fmt.Sprintf("❌ Wrong! The correct answer was: **%s%s**", ansLetter, session.CorrectAnswer))
}

// calculateScore returns time-weighted points: 100 at 3s, scaling linearly to 0 at 30s.
func calculateScore(elapsed time.Duration) int {
	secs := elapsed.Seconds()
	if secs <= 3.0 {
		return 100
	}
	if secs >= 30.0 {
		return 0
	}
	// Linear interpolation: 100 at 3s -> 0 at 30s
	score := int(100.0 * (30.0 - secs) / 27.0)
	if score < 0 {
		score = 0
	}
	return score
}
