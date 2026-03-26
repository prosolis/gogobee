package plugin

import (
	"gogobee/internal/db"
)

// ── Rate Storage ────────────────────────────────────────────────────────────

func fxSaveRate(currency, date string, rate float64) {
	db.Exec("forex: save rate",
		`INSERT OR IGNORE INTO forex_rates (currency, date, rate) VALUES (?, ?, ?)`,
		currency, date, rate)
}

type fxRateRecord struct {
	Date string
	Rate float64
}

// fxGetRates returns rates for a currency in the given date range, ordered by date.
// Uses ORDER BY date DESC LIMIT n when limit > 0 (for "last N trading days").
func fxGetRatesByLimit(currency string, limit int) ([]fxRateRecord, error) {
	d := db.Get()
	rows, err := d.Query(
		`SELECT date, rate FROM forex_rates WHERE currency = ? ORDER BY date DESC LIMIT ?`,
		currency, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []fxRateRecord
	for rows.Next() {
		var r fxRateRecord
		if err := rows.Scan(&r.Date, &r.Rate); err != nil {
			continue
		}
		records = append(records, r)
	}

	// Reverse to chronological order (oldest first)
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}

// fxLatestRate returns the most recent stored rate for a currency.
func fxLatestRate(currency string) (float64, bool) {
	d := db.Get()
	var rate float64
	err := d.QueryRow(
		`SELECT rate FROM forex_rates WHERE currency = ? ORDER BY date DESC LIMIT 1`,
		currency).Scan(&rate)
	if err != nil {
		return 0, false
	}
	return rate, true
}

// ── Alert Storage ───────────────────────────────────────────────────────────

type fxAlertRecord struct {
	UserID    string
	Currency  string
	Threshold float64
	FiredAt   int64
}

func fxSaveAlert(userID, currency string, threshold float64) error {
	d := db.Get()
	_, err := d.Exec(
		`INSERT OR REPLACE INTO forex_alerts (user_id, currency, threshold, fired_at)
		 VALUES (?, ?, ?, 0)`,
		userID, currency, threshold)
	return err
}

func fxAlertsForUser(userID string) ([]fxAlertRecord, error) {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, currency, threshold, fired_at
		 FROM forex_alerts WHERE user_id = ? ORDER BY currency, threshold`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []fxAlertRecord
	for rows.Next() {
		var a fxAlertRecord
		if err := rows.Scan(&a.UserID, &a.Currency, &a.Threshold, &a.FiredAt); err != nil {
			continue
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func fxAllAlerts() ([]fxAlertRecord, error) {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, currency, threshold, fired_at FROM forex_alerts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []fxAlertRecord
	for rows.Next() {
		var a fxAlertRecord
		if err := rows.Scan(&a.UserID, &a.Currency, &a.Threshold, &a.FiredAt); err != nil {
			continue
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func fxDeleteAlert(userID, currency string, threshold float64) error {
	d := db.Get()
	_, err := d.Exec(
		`DELETE FROM forex_alerts WHERE user_id = ? AND currency = ? AND threshold = ?`,
		userID, currency, threshold)
	return err
}

func fxMarkAlertFired(a fxAlertRecord) {
	db.Exec("forex: mark alert fired",
		`UPDATE forex_alerts SET fired_at = unixepoch() WHERE user_id = ? AND currency = ? AND threshold = ?`,
		a.UserID, a.Currency, a.Threshold)
}

// fxResetExpiredAlerts clears fired_at for alerts that fired more than 24 hours ago,
// allowing them to re-fire if the threshold is still met.
func fxResetExpiredAlerts() {
	db.Exec("forex: reset expired alerts",
		`UPDATE forex_alerts SET fired_at = 0 WHERE fired_at > 0 AND fired_at < unixepoch() - 86400`)
}
