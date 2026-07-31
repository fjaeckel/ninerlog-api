// Package airports keeps an in-memory database of the world's airports,
// merged from two upstream datasets and refreshed on a timer.
//
// The database is read on nearly every flight response, so reads are
// lock-free: a reload builds a completely new indexed snapshot and swaps it in
// with a single atomic store. Readers either see the old snapshot or the new
// one, never a partial merge, and a failed refresh leaves the previous
// snapshot serving traffic.
package airports

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// AirportInfo holds metadata about an airport, merged from all sources that
// know about it.
type AirportInfo struct {
	ICAO      string
	Name      string
	Latitude  float64
	Longitude float64
	Elevation int
	Country   string
	// IATA is the 3-letter code, empty for airports that have none.
	IATA string
	// City is the served municipality.
	City string
	// Timezone is the IANA zone name (e.g. "Europe/Berlin"), available for
	// airports covered by the mwgg dataset.
	Timezone string
	// Source records which dataset(s) this record came from: "ourairports",
	// "mwgg", or "merged" when both described the airport.
	Source string
}

var (
	// current holds the live snapshot; nil until the first successful load.
	current atomic.Pointer[snapshot]
	// once guards the one-shot startup load. It is a pointer so tests can
	// swap in a fresh guard without copying a lock.
	once = new(sync.Once)
	// reloadMu serialises reloads so a manual reload and the refresher
	// cannot fetch and swap concurrently.
	reloadMu sync.Mutex
)

// defaultRefreshInterval is how often the database is refetched when
// AIRPORT_REFRESH_INTERVAL is unset. Both upstreams publish at most daily.
const defaultRefreshInterval = 24 * time.Hour

// minRetainFraction rejects a reload whose result is less than this fraction
// of the airports currently loaded. A truncated download or an upstream that
// starts serving a stub would otherwise silently shrink the database.
const minRetainFraction = 0.5

// ErrNoSources is returned by Reload when every upstream failed. The previous
// snapshot, if any, stays live.
var ErrNoSources = errors.New("airports: no source could be loaded")

// ErrSuspectResult is returned when a reload produced far fewer airports than
// the live snapshot, so the swap was rejected.
var ErrSuspectResult = errors.New("airports: reload rejected, result too small")

// Init loads the airport database once, synchronously, at startup. A failure
// is logged and leaves the database empty rather than blocking boot: airport
// lookups degrade to "unknown" instead of taking the API down.
func Init() {
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := Reload(ctx); err != nil {
			slog.Warn("Failed to load airport database; airport lookup will be unavailable", "error", err)
		}
	})
}

// StartRefresher refetches the database every interval until ctx is done.
// An interval of zero or less disables refreshing.
func StartRefresher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		slog.Info("Airport database refresher disabled")
		return
	}
	go func() {
		slog.Info("Airport database refresher started", "interval", interval.String())
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("Airport database refresher stopped")
				return
			case <-ticker.C:
				// Bound each attempt so a hung upstream cannot stall the
				// refresher past its next tick.
				reloadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				if err := Reload(reloadCtx); err != nil {
					slog.Warn("Airport database refresh failed; keeping previous data", "error", err)
				}
				cancel()
			}
		}
	}()
}

// RefreshInterval reads AIRPORT_REFRESH_INTERVAL, defaulting to 24h.
// "0" or "off" disables the periodic refresh.
func RefreshInterval() time.Duration {
	val := strings.TrimSpace(os.Getenv("AIRPORT_REFRESH_INTERVAL"))
	if val == "" {
		return defaultRefreshInterval
	}
	if strings.EqualFold(val, "off") {
		return 0
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		slog.Warn("Invalid AIRPORT_REFRESH_INTERVAL, using default",
			"value", val, "default", defaultRefreshInterval.String())
		return defaultRefreshInterval
	}
	return d
}

// Reload fetches both sources in parallel, merges them, and swaps the result
// in. It returns an error without touching the live snapshot when no source
// could be loaded or the result looks truncated; a single failing source is
// not fatal — the other one is used on its own.
func Reload(ctx context.Context) error {
	reloadMu.Lock()
	defer reloadMu.Unlock()

	start := time.Now()
	defer func() { LoadDurationSeconds.Observe(time.Since(start).Seconds()) }()

	// fetchOne runs one source and records its outcome. A source failure is
	// returned to the caller, which decides whether the reload as a whole
	// still has enough data to be worth swapping in.
	fetchOne := func(source string, fn func(context.Context) (map[string]AirportInfo, int64, error)) (map[string]AirportInfo, error) {
		fetchStart := time.Now()
		data, size, err := fn(ctx)
		FetchDurationSeconds.WithLabelValues(source).Observe(time.Since(fetchStart).Seconds())
		if err != nil {
			FetchTotal.WithLabelValues(source, "error").Inc()
			FetchErrorsTotal.WithLabelValues(source, reasonOf(err)).Inc()
			slog.Warn("Airport source fetch failed", "source", source, "error", err)
			return nil, err
		}
		FetchTotal.WithLabelValues(source, "success").Inc()
		FetchBytes.WithLabelValues(source).Set(float64(size))
		SourceRecords.WithLabelValues(source).Set(float64(len(data)))
		slog.Info("Fetched airport source", "source", source,
			"records", len(data), "bytes", size,
			"duration", time.Since(fetchStart).Round(time.Millisecond).String())
		return data, nil
	}

	var (
		wg             sync.WaitGroup
		oaData, mwData map[string]AirportInfo
		oaErr, mwErr   error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		oaData, oaErr = fetchOne(sourceOurAirports, fetchOurAirports)
	}()
	go func() {
		defer wg.Done()
		mwData, mwErr = fetchOne(sourceMWGG, fetchMWGG)
	}()
	wg.Wait()

	if oaErr != nil && mwErr != nil {
		ReloadTotal.WithLabelValues("failed").Inc()
		return errors.Join(ErrNoSources, oaErr, mwErr)
	}

	mergeStart := time.Now()
	merged, stats := mergeSources(oaData, mwData)
	next := newSnapshot(merged, time.Now())
	MergeDurationSeconds.Observe(time.Since(mergeStart).Seconds())

	if live := current.Load(); live != nil {
		if float64(next.count()) < float64(live.count())*minRetainFraction {
			ReloadTotal.WithLabelValues("rejected").Inc()
			slog.Warn("Airport database reload rejected; keeping previous snapshot",
				"new_count", next.count(), "current_count", live.count())
			return ErrSuspectResult
		}
	}

	current.Store(next)
	publishSnapshotMetrics(next, stats)

	result := "success"
	if oaErr != nil || mwErr != nil {
		result = "partial"
	}
	ReloadTotal.WithLabelValues(result).Inc()

	slog.Info("Loaded airport database",
		"count", next.count(),
		"result", result,
		"only_ourairports", stats.OnlyOurAirports,
		"only_mwgg", stats.OnlyMWGG,
		"both", stats.Both,
		"preferred_ourairports", stats.PreferOurAirport,
		"preferred_mwgg", stats.PreferMWGG,
		"dropped", stats.Dropped,
		"duration", time.Since(start).Round(time.Millisecond).String())

	return nil
}

func publishSnapshotMetrics(s *snapshot, stats mergeStats) {
	Airports.Set(float64(s.count()))
	RecordsByOrigin.WithLabelValues(sourceOurAirports).Set(float64(stats.OnlyOurAirports))
	RecordsByOrigin.WithLabelValues(sourceMWGG).Set(float64(stats.OnlyMWGG))
	RecordsByOrigin.WithLabelValues("both").Set(float64(stats.Both))
	MergePreferred.WithLabelValues(sourceOurAirports).Set(float64(stats.PreferOurAirport))
	MergePreferred.WithLabelValues(sourceMWGG).Set(float64(stats.PreferMWGG))
	DroppedRecords.Set(float64(stats.Dropped))
	LastSuccessTimestampSeconds.Set(float64(s.loadedAt.Unix()))
}

// snapshotAgeSeconds backs the airport_db_age_seconds gauge.
func snapshotAgeSeconds() float64 {
	s := current.Load()
	if s == nil {
		return 0
	}
	return time.Since(s.loadedAt).Seconds()
}

// Lookup returns airport info by ICAO code, or nil if not found.
func Lookup(icao string) *AirportInfo {
	s := current.Load()
	if s == nil {
		lookupUnavailable.Inc()
		return nil
	}
	a := s.lookup(strings.ToUpper(icao))
	if a == nil {
		lookupMiss.Inc()
		return nil
	}
	lookupHit.Inc()
	return a
}

// Count returns the number of airports in the database.
func Count() int {
	return current.Load().count()
}

// LoadedAt reports when the live snapshot was built; the zero time means no
// database is loaded.
func LoadedAt() time.Time {
	s := current.Load()
	if s == nil {
		return time.Time{}
	}
	return s.loadedAt
}

// Search returns airports whose ICAO code starts with prefix
// (case-insensitive), in ICAO order, up to limit results.
func Search(prefix string, limit int) []AirportInfo {
	if prefix == "" || limit <= 0 {
		return nil
	}
	s := current.Load()
	if s == nil {
		searchUnavailable.Inc()
		return nil
	}
	start := time.Now()
	results := s.searchPrefix(strings.ToUpper(prefix), limit)
	searchDuration.Observe(time.Since(start).Seconds())
	if len(results) == 0 {
		searchMiss.Inc()
	} else {
		searchHit.Inc()
	}
	return results
}

// maxNearestDistanceNM bounds Nearest lookups: a fix further than this from
// every known airport (mid-ocean coordinates, bogus GPS data) returns nil
// rather than a misleading "nearest" hundreds of miles away.
const maxNearestDistanceNM = 30.0

// Nearest returns the airport closest to the given coordinates, or nil when
// the database is unavailable or no airport lies within 30 NM. Used to
// resolve a phone's GPS fix to a departure/arrival airport for tap-to-log.
func Nearest(lat, lon float64) *AirportInfo {
	s := current.Load()
	if s == nil {
		nearestUnavailable.Inc()
		return nil
	}
	start := time.Now()
	a := s.nearest(lat, lon, maxNearestDistanceNM)
	nearestDuration.Observe(time.Since(start).Seconds())
	if a == nil {
		nearestMiss.Inc()
		return nil
	}
	nearestHit.Inc()
	return a
}

// DistanceNM returns the great-circle distance between two points in nautical
// miles.
func DistanceNM(lat1, lon1, lat2, lon2 float64) float64 {
	return haversineNM(lat1, lon1, lat2, lon2)
}

// haversineNM returns the great-circle distance between two points in
// nautical miles.
func haversineNM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusNM = 3440.065
	dLat := degToRad(lat2 - lat1)
	dLon := degToRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degToRad(lat1))*math.Cos(degToRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusNM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func degToRad(d float64) float64 {
	return d * math.Pi / 180
}
