package airports

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func TestNearest(t *testing.T) {
	SetTestDB(map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt", Latitude: 50.0333, Longitude: 8.5706},
		"EDFE": {ICAO: "EDFE", Name: "Egelsbach", Latitude: 49.9600, Longitude: 8.6458},
		"KJFK": {ICAO: "KJFK", Name: "JFK", Latitude: 40.6398, Longitude: -73.7789},
	})
	defer SetTestDB(nil)

	// A fix on the Frankfurt ramp resolves to EDDF, not nearby Egelsbach
	if got := Nearest(50.03, 8.57); got == nil || got.ICAO != "EDDF" {
		t.Errorf("Nearest(Frankfurt ramp) = %v, want EDDF", got)
	}

	// Egelsbach coordinates resolve to Egelsbach
	if got := Nearest(49.96, 8.64); got == nil || got.ICAO != "EDFE" {
		t.Errorf("Nearest(Egelsbach) = %v, want EDFE", got)
	}

	// Mid-Atlantic fix is further than 30 NM from every airport
	if got := Nearest(45.0, -30.0); got != nil {
		t.Errorf("Nearest(mid-Atlantic) = %v, want nil", got)
	}
}

func TestNearestNilDB(t *testing.T) {
	SetTestDB(nil)
	if got := Nearest(50.0, 8.5); got != nil {
		t.Errorf("Nearest with nil db = %v, want nil", got)
	}
}

// TestNearest_AcrossCellBoundaries pins the cases the grid index can get
// wrong: an airport in the neighbouring degree cell, and the antimeridian.
func TestNearest_AcrossCellBoundaries(t *testing.T) {
	SetTestDB(map[string]AirportInfo{
		// 0.02° north of the query, but in cell 51 while the query is in 50.
		"NORTH": {ICAO: "NORTH", Latitude: 51.001, Longitude: 8.5},
		// Just east of the antimeridian; queried from just west of it.
		"DATEL": {ICAO: "DATEL", Latitude: -17.0, Longitude: -179.99},
	})
	defer SetTestDB(nil)

	if got := Nearest(50.999, 8.5); got == nil || got.ICAO != "NORTH" {
		t.Errorf("Nearest across a latitude cell boundary = %v, want NORTH", got)
	}
	if got := Nearest(-17.0, 179.99); got == nil || got.ICAO != "DATEL" {
		t.Errorf("Nearest across the antimeridian = %v, want DATEL", got)
	}
}

func TestNearest_HighLatitude(t *testing.T) {
	SetTestDB(map[string]AirportInfo{
		// Svalbard: a degree of longitude is ~26 NM here, so the grid has to
		// widen its longitude span or it will miss this.
		"ENSB":  {ICAO: "ENSB", Latitude: 78.246, Longitude: 15.465},
		"NPOLE": {ICAO: "NPOLE", Latitude: 89.9, Longitude: 100},
	})
	defer SetTestDB(nil)

	if got := Nearest(78.246, 15.9); got == nil || got.ICAO != "ENSB" {
		t.Errorf("Nearest(Svalbard) = %v, want ENSB", got)
	}
	if got := Nearest(89.9, -100); got == nil || got.ICAO != "NPOLE" {
		t.Errorf("Nearest(near pole) = %v, want NPOLE", got)
	}
}

// TestNearest_MatchesBruteForce is the real safety net for the grid index:
// over a randomised world-wide dataset, the indexed answer must equal a full
// linear scan for every query.
func TestNearest_MatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(20260730))

	records := make(map[string]AirportInfo, 4000)
	for i := 0; i < 4000; i++ {
		a := AirportInfo{
			ICAO:      fmt.Sprintf("A%03d", i),
			Latitude:  rng.Float64()*180 - 90,
			Longitude: rng.Float64()*360 - 180,
		}
		if !validCoords(a) {
			continue
		}
		records[a.ICAO] = a
	}
	s := newSnapshot(records, time.Now())

	bruteForce := func(lat, lon float64) *AirportInfo {
		var best *AirportInfo
		bestDist := maxNearestDistanceNM
		for i := range s.list {
			d := haversineNM(lat, lon, s.list[i].Latitude, s.list[i].Longitude)
			if d <= bestDist {
				bestDist = d
				best = &s.list[i]
			}
		}
		return best
	}

	for i := 0; i < 3000; i++ {
		lat := rng.Float64()*180 - 90
		lon := rng.Float64()*360 - 180
		got := s.nearest(lat, lon, maxNearestDistanceNM)
		want := bruteForce(lat, lon)

		switch {
		case got == nil && want == nil:
		case got == nil || want == nil:
			t.Fatalf("nearest(%f, %f) = %v, brute force = %v", lat, lon, got, want)
		case haversineNM(lat, lon, got.Latitude, got.Longitude) != haversineNM(lat, lon, want.Latitude, want.Longitude):
			t.Fatalf("nearest(%f, %f) = %s, brute force = %s (different distances)",
				lat, lon, got.ICAO, want.ICAO)
		}
	}
}

func TestSnapshot_CountAndLookupOnNil(t *testing.T) {
	var s *snapshot
	if s.count() != 0 {
		t.Errorf("nil snapshot count() = %d, want 0", s.count())
	}
}

func TestDistanceNM(t *testing.T) {
	// EDDF → EDDM is roughly 165 NM.
	d := DistanceNM(50.0333, 8.5706, 48.3538, 11.7861)
	if d < 160 || d > 175 {
		t.Errorf("DistanceNM(EDDF, EDDM) = %f, want ~165", d)
	}
	if d := DistanceNM(50, 8, 50, 8); d != 0 {
		t.Errorf("DistanceNM(same point) = %f, want 0", d)
	}
}

func BenchmarkLookup(b *testing.B) {
	setupTestDB()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Lookup("EDDF")
	}
}

func BenchmarkNearest(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	records := make(map[string]AirportInfo, 30000)
	for i := 0; i < 30000; i++ {
		code := fmt.Sprintf("A%05d", i)
		records[code] = AirportInfo{
			ICAO:      code,
			Latitude:  rng.Float64()*180 - 90,
			Longitude: rng.Float64()*360 - 180,
		}
	}
	SetTestDB(records)
	defer SetTestDB(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Nearest(50.0333, 8.5706)
	}
}
