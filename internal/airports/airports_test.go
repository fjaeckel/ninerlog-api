package airports

import (
	"testing"
)

func setupTestDB() {
	db = map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706, Elevation: 364, Country: "DE"},
		"EDDH": {ICAO: "EDDH", Name: "Hamburg Airport", Latitude: 53.6304, Longitude: 9.9882, Elevation: 53, Country: "DE"},
		"EDDM": {ICAO: "EDDM", Name: "Munich Airport", Latitude: 48.3538, Longitude: 11.7861, Elevation: 1487, Country: "DE"},
		"KJFK": {ICAO: "KJFK", Name: "John F Kennedy Intl", Latitude: 40.6399, Longitude: -73.7787, Elevation: 13, Country: "US"},
		"EGLL": {ICAO: "EGLL", Name: "London Heathrow", Latitude: 51.4706, Longitude: -0.4619, Elevation: 83, Country: "GB"},
	}
}

func TestLookup_Found(t *testing.T) {
	setupTestDB()
	info := Lookup("EDDF")
	if info == nil {
		t.Fatal("Lookup(EDDF) = nil, want airport info")
	}
	if info.Name != "Frankfurt Airport" {
		t.Errorf("Name = %s, want Frankfurt Airport", info.Name)
	}
	if info.Country != "DE" {
		t.Errorf("Country = %s, want DE", info.Country)
	}
}

func TestLookup_CaseInsensitive(t *testing.T) {
	setupTestDB()
	info := Lookup("eddf")
	if info == nil {
		t.Fatal("Lookup(eddf) = nil")
	}
	if info.ICAO != "EDDF" {
		t.Errorf("ICAO = %s, want EDDF", info.ICAO)
	}
}

func TestLookup_NotFound(t *testing.T) {
	setupTestDB()
	info := Lookup("XXXX")
	if info != nil {
		t.Errorf("Lookup(XXXX) should be nil")
	}
}

func TestLookup_NilDB(t *testing.T) {
	db = nil
	info := Lookup("EDDF")
	if info != nil {
		t.Errorf("Lookup with nil db should be nil")
	}
}

func TestSearch_ByPrefix(t *testing.T) {
	setupTestDB()
	results := Search("EDD", 10)
	if len(results) != 3 {
		t.Errorf("Search(EDD) count = %d, want 3", len(results))
	}
}

func TestSearch_WithLimit(t *testing.T) {
	setupTestDB()
	results := Search("EDD", 2)
	if len(results) > 2 {
		t.Errorf("Search(EDD, limit=2) count = %d, want <= 2", len(results))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	setupTestDB()
	results := Search("ZZZ", 10)
	if len(results) != 0 {
		t.Errorf("Search(ZZZ) count = %d, want 0", len(results))
	}
}

func TestSearch_EmptyPrefix(t *testing.T) {
	setupTestDB()
	results := Search("", 10)
	if results != nil {
		t.Error("Search with empty prefix should return nil")
	}
}

func TestSearch_NilDB(t *testing.T) {
	db = nil
	results := Search("EDD", 10)
	if results != nil {
		t.Error("Search with nil db should return nil")
	}
}

func TestSearch_SingleChar(t *testing.T) {
	setupTestDB()
	results := Search("K", 10)
	if len(results) != 1 {
		t.Errorf("Search(K) count = %d, want 1", len(results))
	}
	if results[0].ICAO != "KJFK" {
		t.Errorf("Result ICAO = %s, want KJFK", results[0].ICAO)
	}
}

func TestAirportInfoFields(t *testing.T) {
	setupTestDB()
	info := Lookup("KJFK")
	if info == nil {
		t.Fatal("Lookup(KJFK) = nil")
	}
	if info.Elevation != 13 {
		t.Errorf("Elevation = %d, want 13", info.Elevation)
	}
}
