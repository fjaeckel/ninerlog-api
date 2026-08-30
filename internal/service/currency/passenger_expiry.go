package currency

import (
	"fmt"
	"time"
)

// paxWindowDays is the passenger-currency look-back window shared by
// EASA FCL.060(b), FAA §61.57(a)/(b) and LuftPersV.
const paxWindowDays = 90

// paxWindowStart returns the inclusive start of the passenger-currency window:
// midnight UTC on the date paxWindowDays before now.
func paxWindowStart(now time.Time) time.Time {
	d := now.UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -paxWindowDays)
}

// paxLandingCount selects the landings one date contributes to a requirement.
type paxLandingCount func(LandingDay) int

// allLandings counts day and night landings; both count toward day currency.
func allLandings(d LandingDay) int { return d.DayLandings + d.NightLandings }

// nightLandings counts night landings only.
func nightLandings(d LandingDay) int { return d.NightLandings }

// paxTotals sums the landings across every date in the window.
func paxTotals(days []LandingDay) (landings, night int) {
	for _, d := range days {
		landings += allLandings(d)
		night += d.NightLandings
	}
	return landings, night
}

// paxExpiryDate returns the last date on which a satisfied requirement still
// holds with no further flying: the oldest landing needed to reach `required`
// leaves the window paxWindowDays after it was flown. Returns nil when
// `required` is zero or the landings in `days` do not reach it.
//
// days must be ordered newest date first.
func paxExpiryDate(days []LandingDay, required int, count paxLandingCount) *time.Time {
	if required <= 0 {
		return nil
	}
	total := 0
	for _, d := range days {
		total += count(d)
		if total >= required {
			expires := d.Date.AddDate(0, 0, paxWindowDays)
			return &expires
		}
	}
	return nil
}

// paxExpiryString formats an expiry date as YYYY-MM-DD.
func paxExpiryString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// paxExpiryNote renders the trailing expiry note appended to a passenger
// currency message. Empty when neither expiry is known.
func paxExpiryNote(day, night *string) string {
	switch {
	case day != nil && night != nil && *day == *night:
		return fmt.Sprintf(" — expires %s", *day)
	case day != nil && night != nil:
		return fmt.Sprintf(" — day expires %s, night expires %s", *day, *night)
	case day != nil:
		return fmt.Sprintf(" — day expires %s", *day)
	case night != nil:
		return fmt.Sprintf(" — night expires %s", *night)
	}
	return ""
}
