package settlement

import "time"

// CTLocation is the US Central Time zone.
var CTLocation *time.Location

func init() {
	var err error
	CTLocation, err = time.LoadLocation("America/Chicago")
	if err != nil {
		panic("failed to load America/Chicago timezone: " + err.Error())
	}
}

// CutoffHour and CutoffMinute define the EOD cutoff: 6:30 PM CT.
const (
	CutoffHour   = 18
	CutoffMinute = 30
)

// CutoffForDate returns the cutoff timestamp in UTC for the given date.
// The cutoff is 6:30 PM CT on the given date, converted to UTC.
func CutoffForDate(date time.Time) time.Time {
	ct := date.In(CTLocation)
	cutoff := time.Date(ct.Year(), ct.Month(), ct.Day(), CutoffHour, CutoffMinute, 0, 0, CTLocation)
	return cutoff.UTC()
}

// CurrentCutoff returns the cutoff for "today" (in CT).
// If now is before today's cutoff → today's cutoff.
// If now is at or after today's cutoff → next business day's cutoff.
func CurrentCutoff(now time.Time) time.Time {
	ct := now.In(CTLocation)
	todayCutoff := CutoffForDate(ct)
	if now.Before(todayCutoff) {
		return todayCutoff
	}
	return CutoffForDate(NextBusinessDay(ct))
}

// BatchCutoffDate returns the business day "cutoff date" a deposit belongs to.
// Deposits submitted strictly before the cutoff are in that day's batch.
// Deposits at or after cutoff go to the next business day.
func BatchCutoffDate(submittedAt time.Time) time.Time {
	ct := submittedAt.In(CTLocation)
	todayCutoff := CutoffForDate(ct)

	if submittedAt.Before(todayCutoff) {
		// Belongs to today's batch — but only if today is a business day
		if IsBusinessDay(ct) {
			return time.Date(ct.Year(), ct.Month(), ct.Day(), 0, 0, 0, 0, CTLocation)
		}
		return NextBusinessDay(ct)
	}
	return NextBusinessDay(ct)
}

// NextBusinessDay returns the next business day after the given date.
func NextBusinessDay(t time.Time) time.Time {
	ct := t.In(CTLocation)
	next := ct.AddDate(0, 0, 1)
	for !IsBusinessDay(next) {
		next = next.AddDate(0, 0, 1)
	}
	return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, CTLocation)
}

// IsBusinessDay returns true if the date is a weekday and not a US federal holiday.
func IsBusinessDay(t time.Time) bool {
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	return !isFederalHoliday(t)
}

// US Federal Banking Holidays (fixed and observed).
// Covers 2025 and 2026 for MVP.
func isFederalHoliday(t time.Time) bool {
	m := t.Month()
	d := t.Day()
	wd := t.Weekday()
	y := t.Year()

	// New Year's Day: January 1 (observed Fri/Mon if weekend)
	if m == time.January && d == 1 {
		return true
	}
	if m == time.January && d == 2 && wd == time.Monday {
		return true // observed
	}
	if m == time.December && d == 31 && wd == time.Friday {
		return true // observed
	}

	// MLK Day: 3rd Monday of January
	if m == time.January && wd == time.Monday && d >= 15 && d <= 21 {
		return true
	}

	// Presidents' Day: 3rd Monday of February
	if m == time.February && wd == time.Monday && d >= 15 && d <= 21 {
		return true
	}

	// Memorial Day: last Monday of May
	if m == time.May && wd == time.Monday && d >= 25 {
		return true
	}

	// Juneteenth: June 19 (observed)
	if m == time.June && d == 19 {
		return true
	}
	if m == time.June && d == 20 && wd == time.Monday {
		return true
	}
	if m == time.June && d == 18 && wd == time.Friday {
		return true
	}

	// Independence Day: July 4 (observed)
	if m == time.July && d == 4 {
		return true
	}
	if m == time.July && d == 5 && wd == time.Monday {
		return true
	}
	if m == time.July && d == 3 && wd == time.Friday {
		return true
	}

	// Labor Day: 1st Monday of September
	if m == time.September && wd == time.Monday && d <= 7 {
		return true
	}

	// Columbus Day: 2nd Monday of October
	if m == time.October && wd == time.Monday && d >= 8 && d <= 14 {
		return true
	}

	// Veterans Day: November 11 (observed)
	if m == time.November && d == 11 {
		return true
	}
	if m == time.November && d == 12 && wd == time.Monday {
		return true
	}
	if m == time.November && d == 10 && wd == time.Friday {
		return true
	}

	// Thanksgiving: 4th Thursday of November
	if m == time.November && wd == time.Thursday && d >= 22 && d <= 28 {
		return true
	}

	// Christmas: December 25 (observed)
	if m == time.December && d == 25 {
		return true
	}
	if m == time.December && d == 26 && wd == time.Monday {
		return true
	}
	if m == time.December && d == 24 && wd == time.Friday {
		return true
	}

	// Check explicit holiday list for edge cases
	for _, h := range explicitHolidays {
		if h.year == y && h.month == m && h.day == d {
			return true
		}
	}

	return false
}

type holiday struct {
	year  int
	month time.Month
	day   int
}

// Explicit holidays for 2025 and 2026 that may not be captured by
// the rule-based logic above (e.g., edge cases).
var explicitHolidays = []holiday{
	// 2025
	{2025, time.January, 1},
	{2025, time.January, 20},
	{2025, time.February, 17},
	{2025, time.May, 26},
	{2025, time.June, 19},
	{2025, time.July, 4},
	{2025, time.September, 1},
	{2025, time.October, 13},
	{2025, time.November, 11},
	{2025, time.November, 27},
	{2025, time.December, 25},
	// 2026
	{2026, time.January, 1},
	{2026, time.January, 19},
	{2026, time.February, 16},
	{2026, time.May, 25},
	{2026, time.June, 19},
	{2026, time.July, 3}, // July 4 = Saturday, observed Friday
	{2026, time.September, 7},
	{2026, time.October, 12},
	{2026, time.November, 11},
	{2026, time.November, 26},
	{2026, time.December, 25},
}
