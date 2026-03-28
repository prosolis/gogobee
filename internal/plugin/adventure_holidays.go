package plugin

import (
	"strings"
	"time"
)

// ── Holiday Definitions ─────────────────────────────────────────────────────
//
// On major holidays, active adventurers get two actions instead of one.
// isHolidayToday() checks both fixed and floating holidays against UTC date.
//
// All floating holidays are computed algorithmically — no hardcoded dates.
// Easter: Meeus-Jones-Butcher computus
// Hebrew calendar: molad + dehiyyot (Rosh Hashanah, Yom Kippur, Passover, Hanukkah)
// Islamic calendar: Tabular Islamic calendar (Eid al-Fitr, Eid al-Adha, ±1 day)
// Nth-Monday: Canadian statutory holidays
//
// Holidays we can't reliably compute (Diwali, Vesak, Lunar New Year) are
// intentionally omitted rather than requiring yearly hardcoded updates.

type adventureHoliday struct {
	Month int
	Day   int
	Name  string
}

type adventureFloatingHoliday struct {
	Date time.Time
	Name string
}

// Fixed-date holidays (month/day pairs).
var majorHolidays = []adventureHoliday{
	// Global / US
	{Month: 1, Day: 1, Name: "New Year's Day"},
	{Month: 2, Day: 14, Name: "Valentine's Day"},
	{Month: 3, Day: 17, Name: "St. Patrick's Day"},
	{Month: 4, Day: 1, Name: "April Fools' Day"},
	{Month: 5, Day: 5, Name: "Cinco de Mayo"},
	{Month: 7, Day: 4, Name: "Independence Day"},
	{Month: 10, Day: 31, Name: "Halloween"},
	{Month: 11, Day: 1, Name: "Día de los Muertos / All Saints' Day"},
	{Month: 12, Day: 25, Name: "Christmas"},
	{Month: 12, Day: 31, Name: "New Year's Eve"},

	// Canadian
	{Month: 7, Day: 1, Name: "Canada Day"},
	{Month: 9, Day: 30, Name: "National Day for Truth and Reconciliation"},
	{Month: 11, Day: 11, Name: "Remembrance Day"},
	{Month: 12, Day: 26, Name: "Boxing Day"},

	// Portuguese
	{Month: 4, Day: 25, Name: "Dia da Liberdade"},
	{Month: 5, Day: 1, Name: "Dia do Trabalhador"},
	{Month: 6, Day: 10, Name: "Dia de Portugal"},
	{Month: 8, Day: 15, Name: "Assunção de Nossa Senhora"},
	{Month: 10, Day: 5, Name: "Implantação da República"},
	{Month: 12, Day: 1, Name: "Restauração da Independência"},
	{Month: 12, Day: 8, Name: "Imaculada Conceição"},
}

// floatingHolidays returns all computed floating holidays for the given year.
func floatingHolidays(year int) []adventureFloatingHoliday {
	easter := easterDate(year)

	holidays := []adventureFloatingHoliday{
		// Easter-derived (Gregorian computus)
		{Date: easter.AddDate(0, 0, -47), Name: "Mardi Gras / Carnaval"},
		{Date: easter.AddDate(0, 0, -2), Name: "Good Friday"},
		{Date: easter, Name: "Easter Sunday"},
		{Date: easter.AddDate(0, 0, 60), Name: "Corpo de Deus"}, // Corpus Christi PT

		// Nowruz — tied to spring equinox, March 20 in most years
		{Date: time.Date(year, time.March, 20, 0, 0, 0, 0, time.UTC), Name: "Nowruz"},

		// Canadian Nth-Monday holidays
		{Date: nthMonday(year, time.February, 3), Name: "Family Day"},
		{Date: mondayBeforeMay25(year), Name: "Victoria Day"},
		{Date: nthMonday(year, time.September, 1), Name: "Labour Day"},
		{Date: nthMonday(year, time.October, 2), Name: "Thanksgiving"},

		// Hebrew calendar (molad + dehiyyot)
		{Date: hebrewPassover(year), Name: "Passover"},
		{Date: hebrewRoshHashanah(year), Name: "Rosh Hashanah"},
		{Date: hebrewYomKippur(year), Name: "Yom Kippur"},
		{Date: hebrewHanukkah(year), Name: "Hanukkah"},
	}

	// Tabular Islamic calendar (±1 day from observed)
	// These can occur twice in a single Gregorian year since the Islamic
	// calendar is ~11 days shorter, so we append all occurrences.
	for _, d := range tabularIslamicDates(year, 10, 1) {
		holidays = append(holidays, adventureFloatingHoliday{Date: d, Name: "Eid al-Fitr"})
	}
	for _, d := range tabularIslamicDates(year, 12, 10) {
		holidays = append(holidays, adventureFloatingHoliday{Date: d, Name: "Eid al-Adha"})
	}

	return holidays
}

// isHolidayToday checks both fixed and floating holidays against the current UTC date.
// Returns (true, displayName) if today is a holiday, (false, "") otherwise.
// When multiple holidays fall on the same date, all names are joined with " / ".
func isHolidayToday() (bool, string) {
	now := time.Now().UTC()
	month := int(now.Month())
	day := now.Day()
	year := now.Year()

	var names []string

	// Check fixed holidays
	for _, h := range majorHolidays {
		if h.Month == month && h.Day == day {
			names = append(names, h.Name)
		}
	}

	// Check floating holidays
	todayDate := time.Date(year, now.Month(), day, 0, 0, 0, 0, time.UTC)
	for _, fh := range floatingHolidays(year) {
		if fh.Date.Equal(todayDate) {
			names = append(names, fh.Name)
		}
	}

	if len(names) == 0 {
		return false, ""
	}
	return true, strings.Join(names, " / ")
}

// ── Easter Computation (Anonymous Gregorian / Meeus-Jones-Butcher) ──────────

func easterDate(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// ── Nth Monday / Monday Before ──────────────────────────────────────────────

// nthMonday returns the nth Monday of the given month/year (1-indexed).
func nthMonday(year int, month time.Month, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(time.Monday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, offset+(n-1)*7)
}

// mondayBeforeMay25 returns the last Monday on or before May 24 (Victoria Day).
func mondayBeforeMay25(year int) time.Time {
	may24 := time.Date(year, time.May, 24, 0, 0, 0, 0, time.UTC)
	offset := (int(may24.Weekday()) - int(time.Monday) + 7) % 7
	return may24.AddDate(0, 0, -offset)
}

// ── Hebrew Calendar (Molad + Dehiyyot) ──────────────────────────────────────
//
// Computes Rosh Hashanah from the mean lunation (molad) with the four
// postponement rules (dehiyyot). All other Hebrew holidays derive from it.
//
// Day-of-week convention: 0=Mon, 1=Tue, 2=Wed, 3=Thu, 4=Fri, 5=Sat, 6=Sun.
// Time is measured in halakim (parts): 1 hour = 1080 parts.
//
// Reference: Dershowitz & Reingold, "Calendrical Calculations"

func hebrewLeapYear(hYear int) bool {
	return (7*hYear+1)%19 < 7
}

// hebrewNewYearDay returns the day number (from the Hebrew epoch) of 1 Tishrei
// for the given Hebrew year, after applying the four dehiyyot.
func hebrewNewYearDay(hYear int) int {
	y := hYear - 1
	months := 235*(y/19) + 12*(y%19) + (7*(y%19)+1)/19

	// Molad BaHaRaD: day 2, 5 hours, 204 parts (Hebrew time, hours from 6 PM)
	// Lunar month: 29 days, 12 hours, 793 parts
	p := 204 + 793*(months%1080)
	h := 5 + 12*months + 793*(months/1080) + p/1080
	parts := p%1080 + 1080*(h%24) // time-of-day in parts (Hebrew hours from 6 PM)
	day := 2 + 29*months + h/24   // +2 for BaHaRaD day offset (day 2 = Monday)

	// Weekday: 0=Sat, 1=Sun, 2=Mon, 3=Tue, 4=Wed, 5=Thu, 6=Fri
	dow := day % 7

	// Apply dehiyyot (postponement rules)
	alt := parts >= 19440 || // Molad Zaken: molad at or after 18h (= noon civil)
		(dow == 3 && parts >= 9924 && !hebrewLeapYear(hYear)) || // GaTaRaD: Tue >= 9h204p
		(dow == 2 && parts >= 16789 && hebrewLeapYear(hYear-1)) // BeTuTaKPaT: Mon >= 15h589p
	if alt {
		day++
		dow = day % 7
	}

	// Lo ADU: Rosh Hashanah cannot fall on Sun(1), Wed(4), or Fri(6)
	if dow == 1 || dow == 4 || dow == 6 {
		day++
	}

	return day
}

// hebrewEpochJDN: day 2 in the Hebrew count = JDN 347998 (Monday Oct 7, 3761 BCE Julian),
// so day N maps to JDN = N + 347996.
const hebrewEpochJDN = 347996

// hebrewRoshHashanah returns the Gregorian date of 1 Tishrei for the
// Hebrew year starting in the given Gregorian year's autumn.
func hebrewRoshHashanah(gregYear int) time.Time {
	hYear := gregYear + 3761
	return gregorianFromJDN(hebrewNewYearDay(hYear) + hebrewEpochJDN)
}

// hebrewYomKippur returns the Gregorian date of 10 Tishrei (Rosh Hashanah + 9 days).
func hebrewYomKippur(gregYear int) time.Time {
	return hebrewRoshHashanah(gregYear).AddDate(0, 0, 9)
}

// hebrewPassover returns the Gregorian date of 15 Nisan for the given year.
// 15 Nisan is always exactly 163 days before the next Rosh Hashanah.
func hebrewPassover(gregYear int) time.Time {
	hYear := gregYear + 3761 // RH of this Hebrew year falls in gregYear's autumn
	nextRHDay := hebrewNewYearDay(hYear)
	return gregorianFromJDN(nextRHDay - 163 + hebrewEpochJDN)
}

// hebrewHanukkah returns the Gregorian date of 25 Kislev for the Hebrew year
// starting in the given Gregorian year's autumn.
// Offset from RH depends on whether Cheshvan has 29 or 30 days (year type).
func hebrewHanukkah(gregYear int) time.Time {
	hYear := gregYear + 3761
	rhDay := hebrewNewYearDay(hYear)
	yearLen := hebrewNewYearDay(hYear+1) - rhDay

	// Cheshvan has 30 days in "complete" years (355 or 385 days)
	cheshvan := 29
	if yearLen == 355 || yearLen == 385 {
		cheshvan = 30
	}

	// 25 Kislev = Tishrei(30) + Cheshvan + 24 days into Kislev
	offset := 30 + cheshvan + 24
	return gregorianFromJDN(rhDay + offset + hebrewEpochJDN)
}

// ── Tabular Islamic Calendar ────────────────────────────────────────────────
//
// Approximates Islamic dates using the standard 30-year intercalation cycle.
// Accuracy: ±1 day from observed dates (which depend on moon sighting).
//
// Reference: Dershowitz & Reingold, "Calendrical Calculations"

// islamicEpochRD is the Rata Die of 1 Muharram 1 AH (July 19, 622 CE Gregorian).
const islamicEpochRD = 227015

// islamicToRD converts an Islamic date to Rata Die (days since Jan 1, 1 CE).
func islamicToRD(year, month, day int) int {
	return islamicEpochRD - 1 +
		(year-1)*354 +
		(3+11*year)/30 +
		29*(month-1) +
		month/2 +
		day
}

// tabularIslamicDates returns all Gregorian dates in the given Gregorian year
// that correspond to the specified Islamic month/day. Usually returns one date,
// but can return two since the Islamic calendar is ~11 days shorter per year.
func tabularIslamicDates(gregYear, iMonth, iDay int) []time.Time {
	// Approximate Islamic year from Gregorian year
	approxYear := int(float64(gregYear-622) * 33.0 / 32.0)

	var dates []time.Time
	for y := approxYear - 1; y <= approxYear+1; y++ {
		rd := islamicToRD(y, iMonth, iDay)
		d := gregorianFromRD(rd)
		if d.Year() == gregYear {
			dates = append(dates, d)
		}
	}
	return dates
}

// gregorianFromRD converts a Rata Die to a Gregorian date.
// R.D. 1 = January 1, 1 CE = JDN 1721426.
func gregorianFromRD(rd int) time.Time {
	return gregorianFromJDN(rd + 1721425)
}

// ── Julian Day Number ↔ Gregorian ──────────────────────────────────────────
//
// Meeus, "Astronomical Algorithms" — standard JDN conversion.

func gregorianFromJDN(jdn int) time.Time {
	a := jdn + 32044
	b := (4*a + 3) / 146097
	c := a - 146097*b/4
	d := (4*c + 3) / 1461
	e := c - 1461*d/4
	m := (5*e + 2) / 153

	day := e - (153*m+2)/5 + 1
	month := m + 3 - 12*(m/10)
	year := 100*b + d - 4800 + m/10

	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
