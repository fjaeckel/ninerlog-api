package airports

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestCount_WithData(t *testing.T) {
	setupTestDB()
	count := Count()
	if count != 5 {
		t.Errorf("Count() = %d, want 5", count)
	}
}

func TestCount_NilDB(t *testing.T) {
	db = nil
	count := Count()
	if count != 0 {
		t.Errorf("Count() with nil db = %d, want 0", count)
	}
}

func TestCount_EmptyDB(t *testing.T) {
	db = make(map[string]AirportInfo)
	count := Count()
	if count != 0 {
		t.Errorf("Count() with empty db = %d, want 0", count)
	}
}

func TestFetchAirports_ValidCSV(t *testing.T) {
	// Create a test CSV server
	csvData := strings.Join([]string{
		"id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region,municipality,scheduled_service,gps_code,iata_code,local_code,home_link,wikipedia_link,keywords",
		"1,EDDF,large_airport,Frankfurt Airport,50.0333,8.5706,364,EU,DE,DE-HE,Frankfurt,yes,EDDF,FRA,,,,",
		"2,KJFK,large_airport,John F Kennedy Intl,40.6399,-73.7787,13,NA,US,US-NY,New York,yes,KJFK,JFK,,,,",
		"3,EDDM,medium_airport,Munich Airport,48.3538,11.7861,1487,EU,DE,DE-BY,Munich,yes,EDDM,MUC,,,,",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csvData))
	}))
	defer server.Close()

	// Override URL temporarily — call internal function with test server
	// We'll test the CSV parsing via a direct HTTP test
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		t.Fatalf("Failed to read header: %v", err)
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[col] = i
	}

	// Verify required columns are present
	required := []string{"ident", "type", "name", "latitude_deg", "longitude_deg", "iso_country"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			t.Errorf("Missing required column: %s", col)
		}
	}

	// Count data rows
	rowCount := 0
	for {
		_, err := reader.Read()
		if err != nil {
			break
		}
		rowCount++
	}
	if rowCount != 3 {
		t.Errorf("Expected 3 data rows, got %d", rowCount)
	}
}

func TestLookup_ReturnsCorrectCoordinates(t *testing.T) {
	setupTestDB()
	info := Lookup("EDDF")
	if info == nil {
		t.Fatal("Lookup(EDDF) = nil")
	}
	if info.Latitude < 49.0 || info.Latitude > 51.0 {
		t.Errorf("Latitude = %f, expected ~50.0", info.Latitude)
	}
	if info.Longitude < 7.0 || info.Longitude > 10.0 {
		t.Errorf("Longitude = %f, expected ~8.5", info.Longitude)
	}
}

func TestSearch_ReturnsAllFields(t *testing.T) {
	setupTestDB()
	results := Search("KJFK", 10)
	if len(results) != 1 {
		t.Fatalf("Search(KJFK) count = %d, want 1", len(results))
	}
	r := results[0]
	if r.ICAO != "KJFK" {
		t.Errorf("ICAO = %q, want KJFK", r.ICAO)
	}
	if r.Name != "John F Kennedy Intl" {
		t.Errorf("Name = %q, want John F Kennedy Intl", r.Name)
	}
	if r.Country != "US" {
		t.Errorf("Country = %q, want US", r.Country)
	}
}

// === fetchAirports tests via httptest ===

func testCSVData() string {
	return strings.Join([]string{
		"id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region,municipality,scheduled_service,gps_code,iata_code,local_code,home_link,wikipedia_link,keywords",
		"1,EDDF,large_airport,Frankfurt Airport,50.0333,8.5706,364,EU,DE,DE-HE,Frankfurt,yes,EDDF,FRA,,,,",
		"2,KJFK,large_airport,John F Kennedy Intl,40.6399,-73.7787,13,NA,US,US-NY,New York,yes,KJFK,JFK,,,,",
		"3,EDDM,medium_airport,Munich Airport,48.3538,11.7861,1487,EU,DE,DE-BY,Munich,yes,EDDM,MUC,,,,",
		"4,XX,small_airport,Too Short ICAO,0,0,0,NA,US,US-XX,Nowhere,no,XX,,,,,",
		"5,HELI,heliport,Helipad One,0,0,0,NA,US,US-XX,Nowhere,no,HELI,,,,,",
		"6,CLSD,closed,Closed Airport,0,0,0,NA,US,US-XX,Nowhere,no,CLSD,,,,,",
	}, "\n")
}

func TestFetchAirports_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(testCSVData()))
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	defer func() { airportsURL = origURL }()

	result, err := fetchAirports()
	if err != nil {
		t.Fatalf("fetchAirports() error = %v", err)
	}

	// Should only include 4-char ICAO, not heliports, not closed
	if len(result) != 3 {
		t.Errorf("fetchAirports() count = %d, want 3 (EDDF, KJFK, EDDM)", len(result))
	}

	eddf, ok := result["EDDF"]
	if !ok {
		t.Fatal("EDDF not found in results")
	}
	if eddf.Name != "Frankfurt Airport" {
		t.Errorf("EDDF.Name = %q, want Frankfurt Airport", eddf.Name)
	}
	if eddf.Elevation != 364 {
		t.Errorf("EDDF.Elevation = %d, want 364", eddf.Elevation)
	}
	if eddf.Country != "DE" {
		t.Errorf("EDDF.Country = %q, want DE", eddf.Country)
	}

	// Verify heliports and closed airports are excluded
	if _, ok := result["HELI"]; ok {
		t.Error("Heliport should be excluded")
	}
	if _, ok := result["CLSD"]; ok {
		t.Error("Closed airport should be excluded")
	}
	// Short ICAO codes excluded
	if _, ok := result["XX"]; ok {
		t.Error("Short ICAO code should be excluded")
	}
}

func TestFetchAirports_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	defer func() { airportsURL = origURL }()

	_, err := fetchAirports()
	if err == nil {
		t.Error("fetchAirports() should fail on HTTP 500")
	}
}

func TestFetchAirports_InvalidCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not,a,valid,csv,for,airports"))
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	defer func() { airportsURL = origURL }()

	_, err := fetchAirports()
	if err == nil {
		t.Error("fetchAirports() should fail on missing required columns")
	}
}

func TestFetchAirports_EmptyResult(t *testing.T) {
	// CSV with header but no data rows
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region\n"))
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	defer func() { airportsURL = origURL }()

	_, err := fetchAirports()
	if err == nil {
		t.Error("fetchAirports() should fail on 0 airports parsed")
	}
}

func TestFetchAirports_MalformedCoordinates(t *testing.T) {
	csvData := strings.Join([]string{
		"id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region,municipality,scheduled_service,gps_code,iata_code,local_code,home_link,wikipedia_link,keywords",
		"1,EDDF,large_airport,Frankfurt Airport,invalid,8.5706,364,EU,DE,DE-HE,Frankfurt,yes,EDDF,FRA,,,,",
		"2,KJFK,large_airport,John F Kennedy Intl,40.6399,-73.7787,13,NA,US,US-NY,New York,yes,KJFK,JFK,,,,",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(csvData))
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	defer func() { airportsURL = origURL }()

	result, err := fetchAirports()
	if err != nil {
		t.Fatalf("fetchAirports() error = %v", err)
	}
	// EDDF should be skipped (invalid latitude), KJFK should be present
	if len(result) != 1 {
		t.Errorf("fetchAirports() count = %d, want 1 (only KJFK)", len(result))
	}
	if _, ok := result["KJFK"]; !ok {
		t.Error("KJFK should be present")
	}
}

func TestInit_LoadsFromServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testCSVData()))
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	// Reset the sync.Once so Init() runs again
	origOnce := once
	once = sync.Once{}
	db = nil
	defer func() {
		airportsURL = origURL
		once = origOnce
	}()

	Init()

	if db == nil {
		t.Fatal("Init() did not set db")
	}
	if len(db) != 3 {
		t.Errorf("Init() loaded %d airports, want 3", len(db))
	}
	if Count() != 3 {
		t.Errorf("Count() = %d, want 3", Count())
	}
}

func TestInit_FailsGracefully(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origURL := airportsURL
	airportsURL = server.URL
	origOnce := once
	once = sync.Once{}
	db = nil
	defer func() {
		airportsURL = origURL
		once = origOnce
	}()

	Init()

	// Should create empty map, not nil
	if db == nil {
		t.Fatal("Init() should create empty map on failure, not nil")
	}
	if len(db) != 0 {
		t.Errorf("Init() on failure should have 0 airports, got %d", len(db))
	}
}
