package plugin

import (
	"encoding/json"
	"fmt"
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

// lotteryCurrentWeekStart returns the Monday of the lottery week currently
// accepting tickets. Mon–Fri returns this week's Monday; Sat–Sun returns
// next week's Monday — the Friday 23:59 UTC draw resets the week boundary,
// so Saturday onward, ticket purchases count toward the next draw.
func lotteryCurrentWeekStart() string {
	now := time.Now().UTC()
	wd := now.Weekday() // Sunday=0, Monday=1, ..., Saturday=6
	var monday time.Time
	switch wd {
	case time.Sunday:
		monday = now.AddDate(0, 0, 1)
	case time.Saturday:
		monday = now.AddDate(0, 0, 2)
	default:
		monday = now.AddDate(0, 0, -(int(wd) - 1))
	}
	return monday.Format("2006-01-02")
}

// ── Ticket CRUD ─────────────────────────────────────────────────────────────

func lotteryTicketCount(userID id.UserID, weekStart string) int {
	d := db.Get()
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM lottery_tickets WHERE user_id = ? AND week_start = ?`,
		string(userID), weekStart).Scan(&count); err != nil {
		slog.Error("lottery: ticket count query failed", "user", userID, "err", err)
	}
	return count
}

func lotteryTotalTicketCount(weekStart string) int {
	d := db.Get()
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM lottery_tickets WHERE week_start = ?`, weekStart).Scan(&count); err != nil {
		slog.Error("lottery: total ticket count query failed", "err", err)
	}
	return count
}

func lotteryInsertTickets(userID id.UserID, weekStart string, tickets [][]int) error {
	d := db.Get()
	tx, err := d.Begin()
	if err != nil {
		slog.Error("lottery: begin tx", "err", err)
		return err
	}
	defer tx.Rollback()

	// Re-check ticket count inside transaction to prevent TOCTOU race.
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM lottery_tickets WHERE user_id = ? AND week_start = ?`,
		string(userID), weekStart).Scan(&existing); err != nil {
		return fmt.Errorf("lottery: count tickets in tx: %w", err)
	}
	if existing+len(tickets) > 100 {
		return fmt.Errorf("lottery: ticket limit exceeded (have %d, buying %d)", existing, len(tickets))
	}

	for _, nums := range tickets {
		data, _ := json.Marshal(nums)
		_, err := tx.Exec(`INSERT INTO lottery_tickets (user_id, week_start, numbers) VALUES (?, ?, ?)`,
			string(userID), weekStart, string(data))
		if err != nil {
			slog.Error("lottery: failed to insert ticket", "user", userID, "err", err)
			return err
		}
	}

	// Each ticket costs €1 — add to community pot (same transaction).
	_, err = tx.Exec(
		`INSERT INTO community_pot (id, balance, updated_at)
		 VALUES (1, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP`,
		len(tickets), len(tickets))
	if err != nil {
		slog.Error("lottery: failed to add to community pot", "err", err)
		return err
	}

	return tx.Commit()
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
		if err := json.Unmarshal([]byte(winJSON), &h.WinningNumbers); err != nil {
			slog.Warn("lottery: corrupt winning_numbers JSON", "draw", h.DrawDate, "err", err)
		}
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
		if err := json.Unmarshal([]byte(numsJSON), &t.Numbers); err != nil {
			slog.Warn("lottery: corrupt ticket numbers JSON", "id", t.ID, "err", err)
		}
		t.MatchCount = matchCount
		t.Prize = prize
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}
