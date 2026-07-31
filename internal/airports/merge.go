package airports

import "math"

// mergeStats records what the last merge did, for logging and metrics.
type mergeStats struct {
	OnlyOurAirports  int // present in OurAirports only
	OnlyMWGG         int // present in mwgg only
	Both             int // present in both, merged into one record
	PreferOurAirport int // of Both: OurAirports supplied the base record
	PreferMWGG       int // of Both: mwgg supplied the base record
	Dropped          int // discarded: no source had usable coordinates
}

// validCoords rejects records that would poison distance and nearest-airport
// lookups. Null Island is not an airport; both datasets use (0,0) as a
// placeholder for "unknown".
func validCoords(a AirportInfo) bool {
	if math.IsNaN(a.Latitude) || math.IsNaN(a.Longitude) {
		return false
	}
	if a.Latitude < -90 || a.Latitude > 90 || a.Longitude < -180 || a.Longitude > 180 {
		return false
	}
	return a.Latitude != 0 || a.Longitude != 0
}

// score rates how complete a record is. Records without usable coordinates
// score below zero and are never chosen. The weights rank the fields by how
// much a logbook depends on them: coordinates and name drive distance
// calculations and display, the rest is enrichment.
func score(a AirportInfo) int {
	if !validCoords(a) {
		return -1
	}
	s := 0
	if a.Name != "" {
		s += 4
	}
	if a.Country != "" {
		s += 2
	}
	if a.IATA != "" {
		s += 2
	}
	if a.City != "" {
		s++
	}
	if a.Timezone != "" {
		s++
	}
	if a.Elevation != 0 {
		s++
	}
	return s
}

// mergeSources combines the two datasets into the map that backs lookups.
//
// For an ICAO code present in both, the more complete record (see score) wins
// and becomes the base; ties go to OurAirports, which is refreshed
// continuously and already filters closed fields and heliports. Fields the
// base is missing are then filled in from the other source — that is how
// OurAirports records pick up mwgg's timezone and curated city, and how mwgg
// records pick up an elevation OurAirports has. Codes unique to one source are
// carried over as-is, provided their coordinates are usable.
func mergeSources(ourAirports, mwgg map[string]AirportInfo) (map[string]AirportInfo, mergeStats) {
	var stats mergeStats
	merged := make(map[string]AirportInfo, len(ourAirports)+len(mwgg)/2)

	for icao, oa := range ourAirports {
		mw, inBoth := mwgg[icao]
		if !inBoth {
			if score(oa) < 0 {
				stats.Dropped++
				continue
			}
			stats.OnlyOurAirports++
			merged[icao] = oa
			continue
		}

		oaScore, mwScore := score(oa), score(mw)
		if oaScore < 0 && mwScore < 0 {
			stats.Dropped++
			continue
		}

		var base, other AirportInfo
		if mwScore > oaScore {
			base, other = mw, oa
			stats.PreferMWGG++
		} else {
			base, other = oa, mw
			stats.PreferOurAirport++
		}
		stats.Both++
		merged[icao] = fillGaps(base, other)
	}

	for icao, mw := range mwgg {
		if _, seen := ourAirports[icao]; seen {
			continue
		}
		if score(mw) < 0 {
			stats.Dropped++
			continue
		}
		stats.OnlyMWGG++
		merged[icao] = mw
	}

	return merged, stats
}

// fillGaps returns base enriched with any field it lacks from other.
// Coordinates are never mixed: they come from the winning record as a pair,
// so a merged airport is always at one real position.
func fillGaps(base, other AirportInfo) AirportInfo {
	if base.Name == "" {
		base.Name = other.Name
	}
	if base.Country == "" {
		base.Country = other.Country
	}
	if base.IATA == "" {
		base.IATA = other.IATA
	}
	if base.City == "" {
		base.City = other.City
	}
	if base.Timezone == "" {
		base.Timezone = other.Timezone
	}
	if base.Elevation == 0 {
		base.Elevation = other.Elevation
	}
	base.Source = sourceMerged
	return base
}
