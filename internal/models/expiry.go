package models

import "time"

// CalendarDate returns midnight UTC of t's calendar date in UTC.
func CalendarDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// expiryDate returns the calendar date an expiry value names, read in its own
// location.
func expiryDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// DaysUntilExpiry returns the whole calendar days from now's UTC date to the
// expiry date: 0 on the expiry date itself, negative once it has passed.
func DaysUntilExpiry(expiry, now time.Time) int {
	return int(expiryDate(expiry).Sub(CalendarDate(now)).Hours() / 24)
}

// ExpiredAt reports whether an expiry date, the last valid day, has passed by
// now's UTC date. A nil expiry never expires.
func ExpiredAt(expiry *time.Time, now time.Time) bool {
	return expiry != nil && DaysUntilExpiry(*expiry, now) < 0
}

// ExpiringWithin reports whether an unexpired expiry date falls within the
// next `days` calendar days of now's UTC date.
func ExpiringWithin(expiry *time.Time, now time.Time, days int) bool {
	if expiry == nil {
		return false
	}
	d := DaysUntilExpiry(*expiry, now)
	return d >= 0 && d <= days
}
