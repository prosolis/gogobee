package plugin

import (
	"encoding/json"
	"log/slog"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Types ───────────────────────────────────────────────────────────────────

type lotteryTicket struct {
	ID         int64
	UserID     id.UserID
	WeekStart  string
	Numbers    []int
	MatchCount *int
	Prize      *int
}

type lotteryHistoryRow struct {
	DrawDate       string
	WinningNumbers []int
	JackpotWinners int
	JackpotAmount  int
	Match4Winners  int
	Match3Winners  int
	Match2Winners  int
	Match1Winners  int
	PotTotal       int
	RolledOver     int
}

// ── Week Helpers ────────────────────────────────────────────────────────────

// lotteryCurrentWeekStart returns Monday of the current week as "2006-01-02".
func lotteryCurrentWeekStart() string {
	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	return monday.Format("2006-01-02")
}

// ── Ticket CRUD ─────────────────────────────────────────────────────────────

func lotteryTicketCount(userID id.UserID, weekStart string) int {
	d := db.Get()
	var count int
	_ = d.QueryRow(`SELECT COUNT(*) FROM lottery_tickets WHERE user_id = ? AND week_start = ?`,
		string(userID), weekStart).Scan(&count)
	return count
}

func lotteryTotalTicketCount(weekStart string) int {
	d := db.Get()
	var count int
	_ = d.QueryRow(`SELECT COUNT(*) FROM lottery_tickets WHERE week_start = ?`, weekStart).Scan(&count)
	return count
}

func lotteryInsertTickets(userID id.UserID, weekStart string, tickets [][]int) {
	d := db.Get()
	for _, nums := range tickets {
		data, _ := json.Marshal(nums)
		_, err := d.Exec(`INSERT INTO lottery_tickets (user_id, week_start, numbers) VALUES (?, ?, ?)`,
			string(userID), weekStart, string(data))
		if err != nil {
			slog.Error("lottery: failed to insert ticket", "user", userID, "err", err)
		}
	}
	// Each ticket costs €1 — add to community pot.
	communityPotAdd(len(tickets))
}

func lotteryLoadUserTickets(userID id.UserID, weekStart string) ([]lotteryTicket, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, user_id, week_start, numbers, match_count, prize
		FROM lottery_tickets WHERE user_id = ? AND week_start = ? ORDER BY id`,
		string(userID), weekStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLotteryTickets(rows)
}

func lotteryLoadAllWeekTickets(weekStart string) ([]lotteryTicket, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, user_id, week_start, numbers, match_count, prize
		FROM lottery_tickets WHERE week_start = ? ORDER BY id`, weekStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLotteryTickets(rows)
}

func lotteryUpdateTicketResult(ticketID int64, matchCount, prize int) {
	d := db.Get()
	_, err := d.Exec(`UPDATE lottery_tickets SET match_count = ?, prize = ? WHERE id = ?`,
		matchCount, prize, ticketID)
	if err != nil {
		slog.Error("lottery: failed to update ticket result", "id", ticketID, "err", err)
	}
}

// ── History CRUD ────────────────────────────────────────────────────────────

func lotteryInsertHistory(h *lotteryHistoryRow) {
	d := db.Get()
	winJSON, _ := json.Marshal(h.WinningNumbers)
	_, err := d.Exec(`INSERT INTO lottery_history
		(draw_date, winning_numbers, jackpot_winners, jackpot_amount,
		 match4_winners, match3_winners, match2_winners, match1_winners,
		 pot_total, rolled_over)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.DrawDate, string(winJSON), h.JackpotWinners, h.JackpotAmount,
		h.Match4Winners, h.Match3Winners, h.Match2Winners, h.Match1Winners,
		h.PotTotal, h.RolledOver)
	if err != nil {
		slog.Error("lottery: failed to insert history", "err", err)
	}
}

func lotteryLoadHistory(limit int) ([]lotteryHistoryRow, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT draw_date, winning_numbers, jackpot_winners, jackpot_amount,
		match4_winners, match3_winners, match2_winners, match1_winners,
		pot_total, rolled_over
		FROM lottery_history ORDER BY draw_date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []lotteryHistoryRow
	for rows.Next() {
		var h lotteryHistoryRow
		var winJSON string
		if err := rows.Scan(&h.DrawDate, &winJSON, &h.JackpotWinners, &h.JackpotAmount,
			&h.Match4Winners, &h.Match3Winners, &h.Match2Winners, &h.Match1Winners,
			&h.PotTotal, &h.RolledOver); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(winJSON), &h.WinningNumbers)
		history = append(history, h)
	}
	return history, rows.Err()
}

// ── Cleanup ─────────────────────────────────────────────────────────────────

func lotteryCleanupOldTickets() {
	d := db.Get()
	_, err := d.Exec(`DELETE FROM lottery_tickets WHERE week_start < DATE('now', '-30 days')`)
	if err != nil {
		slog.Error("lottery: failed to cleanup old tickets", "err", err)
	}
}

// ── Scan Helper ─────────────────────────────────────────────────────────────

type lotteryRows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

func scanLotteryTickets(rows lotteryRows) ([]lotteryTicket, error) {
	var tickets []lotteryTicket
	for rows.Next() {
		var t lotteryTicket
		var numsJSON string
		var matchCount, prize *int
		if err := rows.Scan(&t.ID, &t.UserID, &t.WeekStart, &numsJSON, &matchCount, &prize); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(numsJSON), &t.Numbers)
		t.MatchCount = matchCount
		t.Prize = prize
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}
