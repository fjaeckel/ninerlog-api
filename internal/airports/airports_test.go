package airports

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupTestDB() {
	SetTestDB(map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706, Elevation: 364, Country: "DE"},
		"EDDH": {ICAO: "EDDH", Name: "Hamburg Airport", Latitude: 53.6304, Longitude: 9.9882, Elevation: 53, Country: "DE"},
		"EDDM": {ICAO: "EDDM", Name: "Munich Airport", Latitude: 48.3538, Longitude: 11.7861, Elevation: 1487, Country: "DE"},
		"KJFK": {ICAO: "KJFK", Name: "John F Kennedy Intl", Latitude: 40.6399, Longitude: -73.7787, Elevation: 13, Country: "US"},
		"EGLL": {ICAO: "EGLL", Name: "London Heathrow", Latitude: 51.4706, Longitude: -0.4619, Elevation: 83, Country: "GB"},
	})
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
	SetTestDB(nil)
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

func TestSearch_SortedByICAO(t *testing.T) {
	setupTestDB()
	results := Search("EDD", 10)
	want := []string{"EDDF", "EDDH", "EDDM"}
	if len(results) != len(want) {
		t.Fatalf("Search(EDD) count = %d, want %d", len(results), len(want))
	}
	for i, code := range want {
		if results[i].ICAO != code {
			t.Errorf("results[%d].ICAO = %s, want %s", i, results[i].ICAO, code)
		}
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

func TestSearch_CaseInsensitive(t *testing.T) {
	setupTestDB()
	results := Search("edd", 10)
	if len(results) != 3 {
		t.Errorf("Search(edd) count = %d, want 3", len(results))
	}
}

func TestSearch_NilDB(t *testing.T) {
	SetTestDB(nil)
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
	SetTestDB(nil)
	count := Count()
	if count != 0 {
		t.Errorf("Count() with nil db = %d, want 0", count)
	}
}

func TestCount_EmptyDB(t *testing.T) {
	SetTestDB(map[string]AirportInfo{})
	count := Count()
	if count != 0 {
		t.Errorf("Count() with empty db = %d, want 0", count)
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

func TestLookup_ReturnsCopy(t *testing.T) {
	setupTestDB()
	info := Lookup("EDDF")
	if info == nil {
		t.Fatal("Lookup(EDDF) = nil")
	}
	info.Name = "mutated"

	again := Lookup("EDDF")
	if again.Name != "Frankfurt Airport" {
		t.Errorf("mutating a returned record changed the database: Name = %q", again.Name)
	}
}

// === source fetching ===

func testCSVData() string {
	return strings.Join([]string{
		"id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region,municipality,scheduled_service,gps_code,iata_code,local_code,home_link,wikipedia_link,keywords",
		"1,EDDF,large_airport,Frankfurt Airport,50.0333,8.5706,364,EU,DE,DE-HE,Frankfurt,yes,EDDF,FRA,,,,",
		"2,KJFK,large_airport,John F Kennedy Intl,40.6399,-73.7787,13,NA,US,US-NY,New York,yes,KJFK,JFK,,,,",
		"3,EDDM,medium_airport,Munich Airport,48.3538,11.7861,1487,EU,DE,DE-BY,Munich,yes,EDDM,MUC,,,,",
		"4,XX,small_airport,Too Short ICAO,1,1,0,NA,US,US-XX,Nowhere,no,XX,,,,,",
		"5,HELI,heliport,Helipad One,1,1,0,NA,US,US-XX,Nowhere,no,HELI,,,,,",
		"6,CLSD,closed,Closed Airport,1,1,0,NA,US,US-XX,Nowhere,no,CLSD,,,,,",
	}, "\n")
}

func testJSONData() string {
	return `{
	  "EDDF": {"icao":"EDDF","iata":"FRA","name":"Frankfurt am Main International Airport","city":"Frankfurt-am-Main","state":"Hesse","country":"DE","elevation":364,"lat":50.0264015198,"lon":8.543129921,"tz":"Europe/Berlin"},
	  "LOWW": {"icao":"LOWW","iata":"VIE","name":"Vienna International Airport","city":"Vienna","state":"Lower-Austria","country":"AT","elevation":600,"lat":48.1102981567,"lon":16.5697002411,"tz":"Europe/Vienna"},
	  "BAD":  {"icao":"BAD","iata":"","name":"Short Code","city":"","state":"","country":"XX","elevation":0,"lat":1,"lon":1,"tz":""}
	}`
}

// serveSources points both source URLs at test servers and restores them
// afterwards. A nil body for a source makes it return HTTP 500.
func serveSources(t *testing.T, csvBody, jsonBody *string) {
	t.Helper()
	newServer := func(body *string, contentType string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", contentType)
			_, _ = w.Write([]byte(*body))
		}))
	}
	csvSrv := newServer(csvBody, "text/csv")
	jsonSrv := newServer(jsonBody, "application/json")

	origCSV, origJSON := ourAirportsURL, mwggURL
	ourAirportsURL, mwggURL = csvSrv.URL, jsonSrv.URL
	t.Cleanup(func() {
		ourAirportsURL, mwggURL = origCSV, origJSON
		csvSrv.Close()
		jsonSrv.Close()
	})
}

func strptr(s string) *string { return &s }

func TestFetchOurAirports_Success(t *testing.T) {
	serveSources(t, strptr(testCSVData()), nil)

	result, size, err := fetchOurAirports(context.Background())
	if err != nil {
		t.Fatalf("fetchOurAirports() error = %v", err)
	}
	if size <= 0 {
		t.Errorf("fetchOurAirports() size = %d, want > 0", size)
	}

	// Only 4-char ICAO codes, no heliports, no closed airports.
	if len(result) != 3 {
		t.Errorf("fetchOurAirports() count = %d, want 3 (EDDF, KJFK, EDDM)", len(result))
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
	if eddf.IATA != "FRA" {
		t.Errorf("EDDF.IATA = %q, want FRA", eddf.IATA)
	}
	if eddf.City != "Frankfurt" {
		t.Errorf("EDDF.City = %q, want Frankfurt", eddf.City)
	}
	if eddf.Source != sourceOurAirports {
		t.Errorf("EDDF.Source = %q, want %q", eddf.Source, sourceOurAirports)
	}

	for _, code := range []string{"HELI", "CLSD", "XX"} {
		if _, ok := result[code]; ok {
			t.Errorf("%s should be excluded", code)
		}
	}
}

func TestFetchOurAirports_HTTPError(t *testing.T) {
	serveSources(t, nil, nil)

	_, _, err := fetchOurAirports(context.Background())
	if err == nil {
		t.Fatal("fetchOurAirports() should fail on HTTP 500")
	}
	if reasonOf(err) != "status" {
		t.Errorf("reason = %q, want status", reasonOf(err))
	}
}

func TestFetchOurAirports_InvalidCSV(t *testing.T) {
	serveSources(t, strptr("not,a,valid,csv,for,airports"), nil)

	_, _, err := fetchOurAirports(context.Background())
	if err == nil {
		t.Fatal("fetchOurAirports() should fail on missing required columns")
	}
	if reasonOf(err) != "decode" {
		t.Errorf("reason = %q, want decode", reasonOf(err))
	}
}

func TestFetchOurAirports_EmptyResult(t *testing.T) {
	serveSources(t, strptr("id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region\n"), nil)

	_, _, err := fetchOurAirports(context.Background())
	if err == nil {
		t.Fatal("fetchOurAirports() should fail on 0 airports parsed")
	}
	if reasonOf(err) != "empty" {
		t.Errorf("reason = %q, want empty", reasonOf(err))
	}
}

func TestFetchOurAirports_MalformedCoordinates(t *testing.T) {
	csvData := strings.Join([]string{
		"id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,continent,iso_country,iso_region,municipality,scheduled_service,gps_code,iata_code,local_code,home_link,wikipedia_link,keywords",
		"1,EDDF,large_airport,Frankfurt Airport,invalid,8.5706,364,EU,DE,DE-HE,Frankfurt,yes,EDDF,FRA,,,,",
		"2,KJFK,large_airport,John F Kennedy Intl,40.6399,-73.7787,13,NA,US,US-NY,New York,yes,KJFK,JFK,,,,",
	}, "\n")
	serveSources(t, strptr(csvData), nil)

	result, _, err := fetchOurAirports(context.Background())
	if err != nil {
		t.Fatalf("fetchOurAirports() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("fetchOurAirports() count = %d, want 1 (only KJFK)", len(result))
	}
	if _, ok := result["KJFK"]; !ok {
		t.Error("KJFK should be present")
	}
}

func TestFetchMWGG_Success(t *testing.T) {
	serveSources(t, nil, strptr(testJSONData()))

	result, size, err := fetchMWGG(context.Background())
	if err != nil {
		t.Fatalf("fetchMWGG() error = %v", err)
	}
	if size <= 0 {
		t.Errorf("fetchMWGG() size = %d, want > 0", size)
	}
	if len(result) != 2 {
		t.Errorf("fetchMWGG() count = %d, want 2 (EDDF, LOWW)", len(result))
	}
	if _, ok := result["BAD"]; ok {
		t.Error("non-4-char code should be excluded")
	}

	eddf := result["EDDF"]
	if eddf.IATA != "FRA" {
		t.Errorf("EDDF.IATA = %q, want FRA", eddf.IATA)
	}
	if eddf.Timezone != "Europe/Berlin" {
		t.Errorf("EDDF.Timezone = %q, want Europe/Berlin", eddf.Timezone)
	}
	if eddf.City != "Frankfurt-am-Main" {
		t.Errorf("EDDF.City = %q, want Frankfurt-am-Main", eddf.City)
	}
	if eddf.Source != sourceMWGG {
		t.Errorf("EDDF.Source = %q, want %q", eddf.Source, sourceMWGG)
	}
}

func TestFetchMWGG_InvalidJSON(t *testing.T) {
	serveSources(t, nil, strptr("{not json"))

	_, _, err := fetchMWGG(context.Background())
	if err == nil {
		t.Fatal("fetchMWGG() should fail on invalid JSON")
	}
	if reasonOf(err) != "decode" {
		t.Errorf("reason = %q, want decode", reasonOf(err))
	}
}

func TestFetchMWGG_EmptyResult(t *testing.T) {
	serveSources(t, nil, strptr("{}"))

	_, _, err := fetchMWGG(context.Background())
	if err == nil {
		t.Fatal("fetchMWGG() should fail on 0 airports parsed")
	}
	if reasonOf(err) != "empty" {
		t.Errorf("reason = %q, want empty", reasonOf(err))
	}
}

// === reload ===

func TestReload_MergesBothSources(t *testing.T) {
	serveSources(t, strptr(testCSVData()), strptr(testJSONData()))
	SetTestDB(nil)
	t.Cleanup(func() { SetTestDB(nil) })

	if err := Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// EDDF (both), KJFK + EDDM (OurAirports only), LOWW (mwgg only)
	if Count() != 4 {
		t.Errorf("Count() = %d, want 4", Count())
	}

	eddf := Lookup("EDDF")
	if eddf == nil {
		t.Fatal("Lookup(EDDF) = nil after reload")
	}
	if eddf.Source != sourceMerged {
		t.Errorf("EDDF.Source = %q, want %q", eddf.Source, sourceMerged)
	}
	// mwgg's record is the more complete one (it carries a timezone), so it
	// wins the base and EDDF ends up with the curated name…
	if eddf.Name != "Frankfurt am Main International Airport" {
		t.Errorf("EDDF.Name = %q, want the mwgg name (its record scores higher)", eddf.Name)
	}
	// …while still carrying the fields both sources agree on.
	if eddf.Timezone != "Europe/Berlin" {
		t.Errorf("EDDF.Timezone = %q, want Europe/Berlin", eddf.Timezone)
	}
	if eddf.IATA != "FRA" {
		t.Errorf("EDDF.IATA = %q, want FRA", eddf.IATA)
	}

	if loww := Lookup("LOWW"); loww == nil {
		t.Error("Lookup(LOWW) = nil, want the mwgg-only airport")
	}
	if kjfk := Lookup("KJFK"); kjfk == nil || kjfk.Source != sourceOurAirports {
		t.Errorf("Lookup(KJFK) = %v, want an ourairports-only record", kjfk)
	}
	if LoadedAt().IsZero() {
		t.Error("LoadedAt() is zero after a successful reload")
	}
}

func TestReload_PartialFailureUsesSurvivingSource(t *testing.T) {
	serveSources(t, nil, strptr(testJSONData())) // CSV source returns 500
	SetTestDB(nil)
	t.Cleanup(func() { SetTestDB(nil) })

	if err := Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v, want success from the surviving source", err)
	}
	if Count() != 2 {
		t.Errorf("Count() = %d, want 2 (mwgg only)", Count())
	}
}

func TestReload_AllSourcesFailKeepsSnapshot(t *testing.T) {
	setupTestDB()
	t.Cleanup(func() { SetTestDB(nil) })
	before := Count()

	serveSources(t, nil, nil)

	err := Reload(context.Background())
	if !errors.Is(err, ErrNoSources) {
		t.Fatalf("Reload() error = %v, want ErrNoSources", err)
	}
	if Count() != before {
		t.Errorf("Count() = %d, want %d — the previous snapshot must survive", Count(), before)
	}
	if Lookup("EDDF") == nil {
		t.Error("previous snapshot must still serve lookups after a failed reload")
	}
}

func TestReload_RejectsSuspiciouslySmallResult(t *testing.T) {
	// A live snapshot of 20 airports vs. a reload yielding 4: below the
	// retain fraction, so the swap must be refused.
	big := make(map[string]AirportInfo, 20)
	for i := 0; i < 20; i++ {
		code := string(rune('A'+i)) + "ZZZ"
		big[code] = AirportInfo{ICAO: code, Name: code, Latitude: float64(i) + 1, Longitude: float64(i) + 1}
	}
	SetTestDB(big)
	t.Cleanup(func() { SetTestDB(nil) })

	serveSources(t, strptr(testCSVData()), strptr(testJSONData()))

	err := Reload(context.Background())
	if !errors.Is(err, ErrSuspectResult) {
		t.Fatalf("Reload() error = %v, want ErrSuspectResult", err)
	}
	if Count() != 20 {
		t.Errorf("Count() = %d, want 20 — the rejected reload must not swap", Count())
	}
}

func TestInit_LoadsOnce(t *testing.T) {
	serveSources(t, strptr(testCSVData()), strptr(testJSONData()))
	origOnce := once
	once = new(sync.Once)
	SetTestDB(nil)
	t.Cleanup(func() {
		once = origOnce
		SetTestDB(nil)
	})

	Init()
	if Count() != 4 {
		t.Errorf("Init() loaded %d airports, want 4", Count())
	}

	// A second Init() must not refetch.
	SetTestDB(map[string]AirportInfo{"EDDF": {ICAO: "EDDF", Latitude: 50, Longitude: 8}})
	Init()
	if Count() != 1 {
		t.Errorf("Init() reloaded on the second call: Count() = %d, want 1", Count())
	}
}

func TestInit_FailsGracefully(t *testing.T) {
	serveSources(t, nil, nil)
	origOnce := once
	once = new(sync.Once)
	SetTestDB(nil)
	t.Cleanup(func() {
		once = origOnce
		SetTestDB(nil)
	})

	Init()

	// No database, but lookups must not panic.
	if Count() != 0 {
		t.Errorf("Count() = %d, want 0 after a failed Init()", Count())
	}
	if Lookup("EDDF") != nil {
		t.Error("Lookup after a failed Init() should be nil")
	}
	if Search("ED", 5) != nil {
		t.Error("Search after a failed Init() should be nil")
	}
	if Nearest(50, 8) != nil {
		t.Error("Nearest after a failed Init() should be nil")
	}
}

func TestStartRefresher_RefetchesOnInterval(t *testing.T) {
	csv := testCSVData()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(csv))
	}))
	defer srv.Close()

	origCSV, origJSON := ourAirportsURL, mwggURL
	ourAirportsURL, mwggURL = srv.URL, srv.URL // JSON decode of CSV fails; CSV source carries the reload
	SetTestDB(nil)
	t.Cleanup(func() {
		ourAirportsURL, mwggURL = origCSV, origJSON
		SetTestDB(nil)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartRefresher(ctx, 20*time.Millisecond)

	deadline := time.After(2 * time.Second)
	for {
		if Count() > 0 {
			return // the refresher performed a reload
		}
		select {
		case <-deadline:
			t.Fatal("refresher did not reload within 2s")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestStartRefresher_DisabledOnZeroInterval(t *testing.T) {
	serveSources(t, strptr(testCSVData()), strptr(testJSONData()))
	SetTestDB(nil)
	t.Cleanup(func() { SetTestDB(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartRefresher(ctx, 0)

	time.Sleep(50 * time.Millisecond)
	if Count() != 0 {
		t.Errorf("Count() = %d, want 0 — a zero interval must disable refreshing", Count())
	}
}

func TestRefreshInterval(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"default when unset", "", defaultRefreshInterval},
		{"explicit duration", "6h", 6 * time.Hour},
		{"off disables", "off", 0},
		{"zero disables", "0s", 0},
		{"invalid falls back", "not-a-duration", defaultRefreshInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AIRPORT_REFRESH_INTERVAL", tt.env)
			if got := RefreshInterval(); got != tt.want {
				t.Errorf("RefreshInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}
