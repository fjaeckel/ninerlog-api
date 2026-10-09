package airports

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// snapshot is an immutable, fully indexed view of the airport database,
// swapped in atomically on reload.
type snapshot struct {
	// list holds every airport sorted by ICAO code; the indexes below point
	// into it.
	list []AirportInfo
	// byICAO maps an upper-case ICAO code to its index in list.
	byICAO map[string]int32
	// byName maps each name key to the index of the airport carrying it, or
	// ambiguousName when several do.
	byName map[string]int32
	// folded holds each airport's folded name, parallel to list.
	folded []string
	// byLocal maps each local code to the index of the airport carrying it,
	// or ambiguousName when several do.
	byLocal map[string]int32
	// locals lists the unambiguous local codes in code order.
	locals []localEntry
	// grid buckets airports into 1°×1° cells for nearest.
	grid map[gridCell][]int32

	loadedAt time.Time

	// packOnce guards the lazy build of the downloadable pack.
	packOnce sync.Once
	// packGz is the gzip-compressed pack document.
	packGz []byte
	// packEtag identifies the pack content, independent of loadedAt.
	packEtag string
}

// gridCell is the integer (floor) degree cell an airport falls into.
type gridCell struct {
	lat int16
	lon int16
}

// localEntry is one local code and the index of its airport.
type localEntry struct {
	code string
	idx  int32
}

// ambiguousName marks a name key carried by more than one airport.
const ambiguousName int32 = -1

// addName indexes airport i under key; an empty key is ignored.
func (s *snapshot) addName(key string, i int32) {
	if key == "" {
		return
	}
	if j, ok := s.byName[key]; ok && j != i {
		s.byName[key] = ambiguousName
		return
	}
	s.byName[key] = i
}

func cellOf(lat, lon float64) gridCell {
	return gridCell{lat: int16(math.Floor(lat)), lon: int16(math.Floor(lon))}
}

// newSnapshot indexes a merged airport set. The input map is not retained.
func newSnapshot(records map[string]AirportInfo, loadedAt time.Time) *snapshot {
	s := &snapshot{
		list:     make([]AirportInfo, 0, len(records)),
		byICAO:   make(map[string]int32, len(records)),
		byName:   make(map[string]int32, len(records)),
		byLocal:  make(map[string]int32),
		folded:   make([]string, 0, len(records)),
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
		f := foldName(a.Name)
		s.folded = append(s.folded, f)
		if strings.Contains(f, " ") {
			s.addName(f, int32(i))
		}
		s.addName(shortNameKey(f), int32(i))
		if a.LocalCode != "" {
			if _, ok := s.byLocal[a.LocalCode]; ok {
				s.byLocal[a.LocalCode] = ambiguousName
			} else {
				s.byLocal[a.LocalCode] = int32(i)
			}
		}
		c := cellOf(a.Latitude, a.Longitude)
		s.grid[c] = append(s.grid[c], int32(i))
	}
	for code, i := range s.byLocal {
		if i != ambiguousName {
			s.locals = append(s.locals, localEntry{code: code, idx: i})
		}
	}
	sort.Slice(s.locals, func(i, j int) bool { return s.locals[i].code < s.locals[j].code })
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

// lookupCode returns the airport whose ICAO code equals code, otherwise the
// single airport whose local code does; ambiguous reports that several
// airports carry the local code.
func (s *snapshot) lookupCode(code string) (a *AirportInfo, ambiguous bool) {
	if ap := s.lookup(code); ap != nil {
		return ap, false
	}
	i, ok := s.byLocal[code]
	if !ok {
		return nil, false
	}
	if i == ambiguousName {
		return nil, true
	}
	ap := s.list[i]
	return &ap, false
}

// searchLocal appends to results, up to limit, airports whose unambiguous
// local code starts with prefix, skipping codes already in results.
func (s *snapshot) searchLocal(prefix string, results []AirportInfo, limit int) []AirportInfo {
	seen := make(map[string]bool, len(results))
	for _, a := range results {
		seen[a.ICAO] = true
	}
	start := sort.Search(len(s.locals), func(i int) bool { return s.locals[i].code >= prefix })
	for i := start; i < len(s.locals) && len(results) < limit; i++ {
		if !strings.HasPrefix(s.locals[i].code, prefix) {
			break
		}
		a := s.list[s.locals[i].idx]
		if !seen[a.ICAO] {
			results = append(results, a)
		}
	}
	return results
}

// lookupName returns the single airport whose name key equals the folded
// name; ambiguous reports that several airports carry the key.
func (s *snapshot) lookupName(name string) (a *AirportInfo, ambiguous bool) {
	i, ok := s.byName[foldName(name)]
	if !ok {
		return nil, false
	}
	if i == ambiguousName {
		return nil, true
	}
	ap := s.list[i]
	return &ap, false
}

// searchName appends to results, up to limit, airports whose folded name has
// a word starting with the folded query, skipping codes already in results.
func (s *snapshot) searchName(query string, results []AirportInfo, limit int) []AirportInfo {
	q := foldName(query)
	if q == "" {
		return results
	}
	seen := make(map[string]bool, len(results))
	for _, a := range results {
		seen[a.ICAO] = true
	}
	for i := range s.list {
		if len(results) >= limit {
			break
		}
		if !seen[s.list[i].ICAO] && nameMatches(s.folded[i], q) {
			results = append(results, s.list[i])
		}
	}
	return results
}

// searchPrefix returns up to limit airports whose ICAO code starts with
// prefix, in ICAO order.
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

// polarCosLimit is the latitude cosine below which nearest falls back to a
// full scan (within ~0.5° of a pole).
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

	// Widen the longitude span by cos of the highest latitude the search box
	// reaches.
	spanDeg := maxDistNM / 60.0
	maxAbsLat := math.Min(math.Abs(lat)+spanDeg, 90)
	cosLat := math.Cos(degToRad(maxAbsLat))

	if cosLat < polarCosLimit {
		// Full scan near the poles.
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
