package airports

import (
	"math"
	"sort"
	"time"
)

// snapshot is an immutable, fully indexed view of the airport database.
// Reloads build a new snapshot and swap it in atomically, so readers never
// take a lock and never observe a half-built database.
type snapshot struct {
	// list holds every airport sorted by ICAO code. Everything else indexes
	// into it, so a record exists exactly once in memory.
	list []AirportInfo
	// byICAO maps an upper-case ICAO code to its index in list.
	byICAO map[string]int32
	// grid buckets airports into 1°×1° cells so Nearest scans a handful of
	// cells instead of all ~35k airports.
	grid map[gridCell][]int32

	loadedAt time.Time
}

// gridCell is the integer (floor) degree cell an airport falls into.
type gridCell struct {
	lat int16
	lon int16
}

func cellOf(lat, lon float64) gridCell {
	return gridCell{lat: int16(math.Floor(lat)), lon: int16(math.Floor(lon))}
}

// newSnapshot indexes a merged airport set. The input map is not retained.
func newSnapshot(records map[string]AirportInfo, loadedAt time.Time) *snapshot {
	s := &snapshot{
		list:     make([]AirportInfo, 0, len(records)),
		byICAO:   make(map[string]int32, len(records)),
		grid:     make(map[gridCell][]int32, len(records)/8+1),
		loadedAt: loadedAt,
	}
	for _, a := range records {
		s.list = append(s.list, a)
	}
	sort.Slice(s.list, func(i, j int) bool { return s.list[i].ICAO < s.list[j].ICAO })

	for i := range s.list {
		a := &s.list[i]
		s.byICAO[a.ICAO] = int32(i)
		c := cellOf(a.Latitude, a.Longitude)
		s.grid[c] = append(s.grid[c], int32(i))
	}
	return s
}

func (s *snapshot) count() int {
	if s == nil {
		return 0
	}
	return len(s.list)
}

// lookup returns the airport with the given (already upper-cased) ICAO code.
func (s *snapshot) lookup(icao string) *AirportInfo {
	i, ok := s.byICAO[icao]
	if !ok {
		return nil
	}
	a := s.list[i]
	return &a
}

// searchPrefix returns up to limit airports whose ICAO code starts with
// prefix, in ICAO order. The sorted list makes this a binary search plus a
// short scan instead of a full map walk, and makes results deterministic.
func (s *snapshot) searchPrefix(prefix string, limit int) []AirportInfo {
	start := sort.Search(len(s.list), func(i int) bool { return s.list[i].ICAO >= prefix })
	var results []AirportInfo
	for i := start; i < len(s.list) && len(results) < limit; i++ {
		if len(s.list[i].ICAO) < len(prefix) || s.list[i].ICAO[:len(prefix)] != prefix {
			break
		}
		results = append(results, s.list[i])
	}
	return results
}

// polarCosLimit is the cosine below which the longitude span of a 30 NM
// radius exceeds any useful cell count, and a full scan is cheaper than
// walking the grid. Reached only within ~0.5° of a pole.
const polarCosLimit = 0.01

// nearest returns the airport within maxDistNM of the given coordinates,
// or nil when none is close enough.
func (s *snapshot) nearest(lat, lon float64, maxDistNM float64) *AirportInfo {
	best := -1
	bestDist := maxDistNM

	consider := func(idx int32) {
		a := &s.list[idx]
		d := haversineNM(lat, lon, a.Latitude, a.Longitude)
		if d <= bestDist {
			bestDist = d
			best = int(idx)
		}
	}

	// One degree of latitude is ~60 NM; a degree of longitude shrinks with
	// cos(latitude). Widen the longitude span accordingly, using the highest
	// latitude the search box reaches so the span is never too narrow.
	spanDeg := maxDistNM / 60.0
	maxAbsLat := math.Min(math.Abs(lat)+spanDeg, 90)
	cosLat := math.Cos(degToRad(maxAbsLat))

	if cosLat < polarCosLimit {
		// Near the poles every meridian converges; scan everything.
		for i := range s.list {
			consider(int32(i))
		}
	} else {
		lonSpanCells := int(math.Ceil(spanDeg/cosLat)) + 1
		latCell := int(math.Floor(lat))
		lonCell := int(math.Floor(lon))
		latSpanCells := int(math.Ceil(spanDeg)) + 1

		if lonSpanCells >= 180 {
			lonSpanCells = 180
		}
		for dLat := -latSpanCells; dLat <= latSpanCells; dLat++ {
			l := latCell + dLat
			if l < -90 || l > 90 {
				continue
			}
			for dLon := -lonSpanCells; dLon <= lonSpanCells; dLon++ {
				// Longitude wraps: cell -181 is cell 179.
				w := ((lonCell+dLon+180)%360+360)%360 - 180
				for _, idx := range s.grid[gridCell{lat: int16(l), lon: int16(w)}] {
					consider(idx)
				}
			}
		}
	}

	if best < 0 {
		return nil
	}
	a := s.list[best]
	return &a
}
