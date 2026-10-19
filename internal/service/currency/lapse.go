package currency

import (
	"context"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// lapseSearchFloor is the earliest day the lapse search considers.
var lapseSearchFloor = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// maxLapseSearchRounds bounds the number of narrowing rounds in lastCurrentDay.
const maxLapseSearchRounds = 64

// AsOfProvider is a FlightDataProvider that can restrict its reads to
// flights dated on or before a day.
type AsOfProvider interface {
	AsOf(day time.Time) FlightDataProvider
}

type lapseDatesKey struct{}

// WithLapseDates returns a context under which evaluations also fill the
// lapse dates of lapsed items (ClassRatingCurrency.LapsedOn,
// PassengerCurrency.DayLapsedOn/NightLapsedOn, CustomCurrencyResult.LapsedOn).
func WithLapseDates(ctx context.Context) context.Context {
	return context.WithValue(ctx, lapseDatesKey{}, true)
}

// lapseDatesRequested reports whether ctx came from WithLapseDates.
func lapseDatesRequested(ctx context.Context) bool {
	v, _ := ctx.Value(lapseDatesKey{}).(bool)
	return v
}

// metAtFunc reports whether a requirement is met on `day` counting only the
// flights in day's window that are dated on or before `until` (until >= day).
type metAtFunc func(day, until time.Time) (bool, error)

// lastCurrentDay returns the latest day before today on which metAt holds
// with until == day, or nil when there is none on or after floor. metAt must
// be monotone: met on a day implies met on every earlier day for the same
// until.
func lastCurrentDay(today, floor time.Time, metAt metAtFunc) (*time.Time, error) {
	until := today.AddDate(0, 0, -1)
	for round := 0; round < maxLapseSearchRounds && !until.Before(floor); round++ {
		met, err := metAt(until, until)
		if err != nil {
			return nil, err
		}
		if met {
			d := until
			return &d, nil
		}
		met, err = metAt(floor, until)
		if err != nil || !met {
			return nil, err
		}
		lo, hi := floor, until
		for spanDays(lo, hi) > 1 {
			mid := lo.AddDate(0, 0, spanDays(lo, hi)/2)
			met, err := metAt(mid, until)
			if err != nil {
				return nil, err
			}
			if met {
				lo = mid
			} else {
				hi = mid
			}
		}
		met, err = metAt(lo, lo)
		if err != nil {
			return nil, err
		}
		if met {
			d := lo
			return &d, nil
		}
		until = lo.AddDate(0, 0, -1)
	}
	return nil, nil
}

// spanDays returns the whole days from a to b.
func spanDays(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}

// lapseAfter returns the day after last, or nil.
func lapseAfter(last *time.Time) *time.Time {
	if last == nil {
		return nil
	}
	d := last.AddDate(0, 0, 1)
	return &d
}

// paxLapseDate returns the first day a passenger requirement was no longer
// met, from the full landing history (newest date first), or nil when it was
// never met. waiverThrough, when set, is the last day the requirement is
// waived (an IR's expiry date under FCL.060(b)(2)(ii)).
func paxLapseDate(history []LandingDay, required int, count paxLandingCount, waiverThrough *time.Time, today time.Time) *time.Time {
	if required <= 0 {
		return nil
	}
	metAt := func(day, until time.Time) (bool, error) {
		if waiverThrough != nil && !day.After(models.CalendarDate(*waiverThrough)) {
			return true, nil
		}
		start := day.AddDate(0, 0, -paxWindowDays)
		total := 0
		for _, d := range history {
			if d.Date.After(until) {
				continue
			}
			if d.Date.Before(start) {
				break
			}
			total += count(d)
		}
		return total >= required, nil
	}
	last, _ := lastCurrentDay(models.CalendarDate(today), lapseSearchFloor, metAt)
	return lapseAfter(last)
}

// ratingLapseDate returns the first day a rolling-window rule was no longer
// met, or nil when it was never met or dp cannot read as of a past day. The
// rule is re-evaluated at noon UTC of each probed day.
func ratingLapseDate(ctx context.Context, rule *ratingRule, rating *models.ClassRating, license *models.License, peers []*models.ClassRating, dp FlightDataProvider, now time.Time) *time.Time {
	asOf, ok := dp.(AsOfProvider)
	if !ok {
		return nil
	}
	metAt := func(day, until time.Time) (bool, error) {
		r := evalRatingRuleAt(ctx, rule, rating, license, peers, asOf.AsOf(until), day.Add(12*time.Hour))
		return r.Status == StatusCurrent, nil
	}
	last, _ := lastCurrentDay(models.CalendarDate(now), lapseSearchFloor, metAt)
	return lapseAfter(last)
}

// fillPaxLapseDates sets DayLapsedOn and NightLapsedOn for each expired
// requirement under WithLapseDates, reading the full landing history through
// fetch. A failed read sets LapseUnknown.
func fillPaxLapseDates(ctx context.Context, result *PassengerCurrency, fetch func(since time.Time) ([]LandingDay, error), nightWaiverThrough *time.Time, now time.Time) {
	if !lapseDatesRequested(ctx) {
		return
	}
	needDay := result.DayStatus == StatusExpired
	needNight := result.NightPrivilege && result.NightStatus == StatusExpired
	if !needDay && !needNight {
		return
	}
	history, err := fetch(lapseSearchFloor)
	if err != nil {
		result.LapseUnknown = true
		return
	}
	if needDay {
		result.DayLapsedOn = paxLapseDate(history, result.DayRequired, allLandings, nil, now)
	}
	if needNight {
		result.NightLapsedOn = paxLapseDate(history, result.NightRequired, nightLandings, nightWaiverThrough, now)
	}
}

// latestIRExpiry returns the latest expiry date among the IR ratings, or nil.
func latestIRExpiry(ratings []*models.ClassRating) *time.Time {
	var latest *time.Time
	for _, r := range ratings {
		if r == nil || r.ClassType != models.ClassTypeIR || r.ExpiryDate == nil {
			continue
		}
		if latest == nil || r.ExpiryDate.After(*latest) {
			latest = r.ExpiryDate
		}
	}
	return latest
}
