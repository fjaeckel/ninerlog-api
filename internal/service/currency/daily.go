package currency

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// DailyProgress is the Progress of the flights on one date.
type DailyProgress struct {
	Date time.Time
	Progress
}

// DailyLaunches counts the launches with one method on one date.
type DailyLaunches struct {
	Date     time.Time
	Method   string
	Launches int
}

// DailyFlightDataProvider is a FlightDataProvider that also returns its
// aggregates per date, oldest date first. Each read selects the same flights
// as its aggregate counterpart; dates with no flights are omitted.
type DailyFlightDataProvider interface {
	FlightDataProvider
	// GetDailyProgressByAircraftClass is GetProgressByAircraftClass per date.
	GetDailyProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) ([]DailyProgress, error)
	// GetDailyProgressByULKind is GetProgressByULKind per date.
	GetDailyProgressByULKind(ctx context.Context, userID uuid.UUID, sel ULSelector, includeTowed bool, since time.Time) ([]DailyProgress, error)
	// GetDailyProgressAll is GetProgressAll per date.
	GetDailyProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) ([]DailyProgress, error)
	// GetDailyLaunchCounts is GetLaunchCounts per date and method.
	GetDailyLaunchCounts(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) ([]DailyLaunches, error)
}

// dailyCache is a FlightDataProvider answering every read from per-date rows
// fetched once per scope. A read with a later since than the cached one is
// answered in memory; an earlier since refetches. It records every date it
// has seen. Not safe for concurrent use.
type dailyCache struct {
	dp      DailyFlightDataProvider
	entries map[string]*cacheEntry
	dates   map[time.Time]struct{}
}

type cacheEntry struct {
	since    time.Time
	progress []DailyProgress
	landings []LandingDay
	launches []DailyLaunches
	check    *time.Time
}

// newDailyCache returns a dailyCache over dp.
func newDailyCache(dp DailyFlightDataProvider) *dailyCache {
	return &dailyCache{dp: dp, entries: map[string]*cacheEntry{}, dates: map[time.Time]struct{}{}}
}

// midnightUTC returns midnight UTC on the date of t.
func midnightUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// inWindow reports whether a flight date counts in a window starting at since.
func inWindow(date, since time.Time) bool {
	return !midnightUTC(date).Before(since)
}

// get returns the entry for key covering since, fetching it when absent or
// narrower.
func (c *dailyCache) get(key string, since time.Time, fetch func() (*cacheEntry, error)) (*cacheEntry, error) {
	if e, ok := c.entries[key]; ok && !since.Before(e.since) {
		return e, nil
	}
	e, err := fetch()
	if err != nil {
		return nil, err
	}
	e.since = since
	c.entries[key] = e
	for _, r := range e.progress {
		c.dates[midnightUTC(r.Date)] = struct{}{}
	}
	for _, r := range e.landings {
		c.dates[midnightUTC(r.Date)] = struct{}{}
	}
	for _, r := range e.launches {
		c.dates[midnightUTC(r.Date)] = struct{}{}
	}
	if e.check != nil {
		c.dates[midnightUTC(*e.check)] = struct{}{}
	}
	return e, nil
}

// sumProgress aggregates the rows dated on or after since.
func sumProgress(rows []DailyProgress, since time.Time) *Progress {
	total := &Progress{}
	for _, r := range rows {
		if !inWindow(r.Date, since) {
			continue
		}
		p := r.Progress
		addProgress(total, &p, true)
	}
	return total
}

func (c *dailyCache) progress(key string, since time.Time, fetch func() ([]DailyProgress, error)) (*Progress, error) {
	e, err := c.get(key, since, func() (*cacheEntry, error) {
		rows, err := fetch()
		return &cacheEntry{progress: rows}, err
	})
	if err != nil {
		return nil, err
	}
	return sumProgress(e.progress, since), nil
}

func (c *dailyCache) GetProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) (*Progress, error) {
	key := fmt.Sprintf("class|%s|%v|%t", userID, classTypes, includeTowed)
	return c.progress(key, since, func() ([]DailyProgress, error) {
		return c.dp.GetDailyProgressByAircraftClass(ctx, userID, classTypes, includeTowed, since)
	})
}

func (c *dailyCache) GetProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) (*Progress, error) {
	key := fmt.Sprintf("all|%s", userID)
	return c.progress(key, since, func() ([]DailyProgress, error) {
		return c.dp.GetDailyProgressAll(ctx, userID, since)
	})
}

func (c *dailyCache) GetProgressByULKind(ctx context.Context, userID uuid.UUID, sel ULSelector, includeTowed bool, since time.Time) (*Progress, error) {
	key := fmt.Sprintf("ul|%s|%v|%t", userID, sel, includeTowed)
	return c.progress(key, since, func() ([]DailyProgress, error) {
		return c.dp.GetDailyProgressByULKind(ctx, userID, sel, includeTowed, since)
	})
}

func (c *dailyCache) GetLastFlightReview(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	return c.dp.GetLastFlightReview(ctx, userID)
}

func (c *dailyCache) lastCheck(key string, since time.Time, fetch func() (*time.Time, error)) (*time.Time, error) {
	e, err := c.get(key, since, func() (*cacheEntry, error) {
		d, err := fetch()
		return &cacheEntry{check: d}, err
	})
	if err != nil {
		return nil, err
	}
	if e.check == nil || !inWindow(*e.check, since) {
		return nil, nil
	}
	d := *e.check
	return &d, nil
}

func (c *dailyCache) GetLastProficiencyCheck(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, since time.Time) (*time.Time, error) {
	key := fmt.Sprintf("check|%s|%v", userID, classTypes)
	return c.lastCheck(key, since, func() (*time.Time, error) {
		return c.dp.GetLastProficiencyCheck(ctx, userID, classTypes, since)
	})
}

func (c *dailyCache) GetLastProficiencyCheckByULKind(ctx context.Context, userID uuid.UUID, sel ULSelector, since time.Time) (*time.Time, error) {
	key := fmt.Sprintf("ulcheck|%s|%v", userID, sel)
	return c.lastCheck(key, since, func() (*time.Time, error) {
		return c.dp.GetLastProficiencyCheckByULKind(ctx, userID, sel, since)
	})
}

func (c *dailyCache) GetLaunchCounts(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (map[string]int, error) {
	key := fmt.Sprintf("launch|%s|%s", userID, classType)
	e, err := c.get(key, since, func() (*cacheEntry, error) {
		rows, err := c.dp.GetDailyLaunchCounts(ctx, userID, classType, since)
		return &cacheEntry{launches: rows}, err
	})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range e.launches {
		if inWindow(r.Date, since) {
			counts[r.Method] += r.Launches
		}
	}
	return counts, nil
}

func (c *dailyCache) landingDays(key string, since time.Time, fetch func() ([]LandingDay, error)) ([]LandingDay, error) {
	e, err := c.get(key, since, func() (*cacheEntry, error) {
		rows, err := fetch()
		return &cacheEntry{landings: rows}, err
	})
	if err != nil {
		return nil, err
	}
	var out []LandingDay
	for _, d := range e.landings {
		if inWindow(d.Date, since) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (c *dailyCache) GetLandingDaysByAircraftClass(ctx context.Context, userID uuid.UUID, classType models.ClassType, includeTowed, picOnly bool, since time.Time) ([]LandingDay, error) {
	key := fmt.Sprintf("landing|%s|%s|%t|%t", userID, classType, includeTowed, picOnly)
	return c.landingDays(key, since, func() ([]LandingDay, error) {
		return c.dp.GetLandingDaysByAircraftClass(ctx, userID, classType, includeTowed, picOnly, since)
	})
}

func (c *dailyCache) GetLandingDaysByULKind(ctx context.Context, userID uuid.UUID, sel ULSelector, includeTowed bool, since time.Time) ([]LandingDay, error) {
	key := fmt.Sprintf("ullanding|%s|%v|%t", userID, sel, includeTowed)
	return c.landingDays(key, since, func() ([]LandingDay, error) {
		return c.dp.GetLandingDaysByULKind(ctx, userID, sel, includeTowed, since)
	})
}

// lastDaysCounted returns, ascending, the last date each recorded flight date
// still counts in window w, keeping those on or after from.
func (c *dailyCache) lastDaysCounted(w windowSpec, from time.Time) []time.Time {
	seen := map[time.Time]struct{}{}
	var out []time.Time
	for d := range c.dates {
		last := lastDayCounted(d, w)
		if last.Before(from) {
			continue
		}
		if _, dup := seen[last]; dup {
			continue
		}
		seen[last] = struct{}{}
		out = append(out, last)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// lastDayCounted returns the last date on which a flight dated d counts in a
// rolling window w: the latest date D with D − w before d.
func lastDayCounted(d time.Time, w windowSpec) time.Time {
	d = midnightUTC(d)
	back := func(t time.Time) time.Time { return t.AddDate(-w.years, -w.months, -w.days) }
	last := d.AddDate(w.years, w.months, w.days).AddDate(0, 0, -1)
	for !back(last).Before(d) {
		last = last.AddDate(0, 0, -1)
	}
	for back(last.AddDate(0, 0, 1)).Before(d) {
		last = last.AddDate(0, 0, 1)
	}
	return last
}
