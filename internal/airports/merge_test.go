package airports

import "testing"

func TestMergeSources_PrefersMoreCompleteRecord(t *testing.T) {
	oa := map[string]AirportInfo{
		// Sparse OurAirports record: name and coordinates only.
		"LOWW": {ICAO: "LOWW", Name: "Wien Schwechat", Latitude: 48.11, Longitude: 16.57, Source: sourceOurAirports},
	}
	mw := map[string]AirportInfo{
		"LOWW": {ICAO: "LOWW", Name: "Vienna International Airport", Latitude: 48.1103, Longitude: 16.5697,
			Elevation: 600, Country: "AT", IATA: "VIE", City: "Vienna", Timezone: "Europe/Vienna", Source: sourceMWGG},
	}

	merged, stats := mergeSources(oa, mw)
	if stats.Both != 1 || stats.PreferMWGG != 1 {
		t.Fatalf("stats = %+v, want Both=1 PreferMWGG=1", stats)
	}

	got := merged["LOWW"]
	if got.Name != "Vienna International Airport" {
		t.Errorf("Name = %q, want the mwgg name", got.Name)
	}
	if got.Latitude != 48.1103 || got.Longitude != 16.5697 {
		t.Errorf("coordinates = %f/%f, want the winning record's pair", got.Latitude, got.Longitude)
	}
	if got.Source != sourceMerged {
		t.Errorf("Source = %q, want %q", got.Source, sourceMerged)
	}
}

func TestMergeSources_TiePrefersOurAirports(t *testing.T) {
	base := AirportInfo{Name: "A", Latitude: 50, Longitude: 8, Country: "DE", IATA: "AAA", City: "Town", Elevation: 100}

	oa := map[string]AirportInfo{"EDDF": base}
	oa["EDDF"] = withICAO(oa["EDDF"], "EDDF", sourceOurAirports)
	mw := map[string]AirportInfo{"EDDF": withICAO(base, "EDDF", sourceMWGG)}
	mw["EDDF"] = setName(mw["EDDF"], "B")

	merged, stats := mergeSources(oa, mw)
	if stats.PreferOurAirport != 1 {
		t.Fatalf("stats = %+v, want PreferOurAirport=1", stats)
	}
	if merged["EDDF"].Name != "A" {
		t.Errorf("Name = %q, want A (OurAirports wins ties)", merged["EDDF"].Name)
	}
}

func TestMergeSources_FillsGapsFromLoser(t *testing.T) {
	oa := map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706,
			Elevation: 364, Country: "DE", IATA: "FRA", City: "Frankfurt", Source: sourceOurAirports},
	}
	mw := map[string]AirportInfo{
		// Same completeness minus the elevation, plus a timezone: mwgg wins on
		// score, but the elevation has to survive the merge.
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main International Airport", Latitude: 50.0264, Longitude: 8.5431,
			Country: "DE", IATA: "FRA", City: "Frankfurt-am-Main", Timezone: "Europe/Berlin", Source: sourceMWGG},
	}

	merged, _ := mergeSources(oa, mw)
	got := merged["EDDF"]
	if got.Timezone != "Europe/Berlin" {
		t.Errorf("Timezone = %q, want Europe/Berlin", got.Timezone)
	}
	if got.Elevation != 364 {
		t.Errorf("Elevation = %d, want 364 filled in from OurAirports", got.Elevation)
	}
}

func TestMergeSources_UnionOfBothSources(t *testing.T) {
	oa := map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt", Latitude: 50, Longitude: 8, Source: sourceOurAirports},
		"KJFK": {ICAO: "KJFK", Name: "JFK", Latitude: 40.6, Longitude: -73.8, Source: sourceOurAirports},
	}
	mw := map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main", Latitude: 50, Longitude: 8, Timezone: "Europe/Berlin", Source: sourceMWGG},
		"EDXR": {ICAO: "EDXR", Name: "Rendsburg", Latitude: 54.3, Longitude: 9.5, Source: sourceMWGG},
	}

	merged, stats := mergeSources(oa, mw)
	if len(merged) != 3 {
		t.Errorf("merged count = %d, want 3", len(merged))
	}
	if stats.OnlyOurAirports != 1 || stats.OnlyMWGG != 1 || stats.Both != 1 {
		t.Errorf("stats = %+v, want OnlyOurAirports=1 OnlyMWGG=1 Both=1", stats)
	}
	if merged["KJFK"].Source != sourceOurAirports {
		t.Errorf("KJFK.Source = %q, want %q", merged["KJFK"].Source, sourceOurAirports)
	}
	if merged["EDXR"].Source != sourceMWGG {
		t.Errorf("EDXR.Source = %q, want %q", merged["EDXR"].Source, sourceMWGG)
	}
}

func TestMergeSources_DropsUnusableCoordinates(t *testing.T) {
	oa := map[string]AirportInfo{
		"ZZZZ": {ICAO: "ZZZZ", Name: "Null Island", Latitude: 0, Longitude: 0, Source: sourceOurAirports},
		"YYYY": {ICAO: "YYYY", Name: "Out of range", Latitude: 95, Longitude: 8, Source: sourceOurAirports},
	}
	mw := map[string]AirportInfo{
		"ZZZZ": {ICAO: "ZZZZ", Name: "Null Island", Latitude: 0, Longitude: 0, Source: sourceMWGG},
		"XXXX": {ICAO: "XXXX", Name: "Bad lon", Latitude: 50, Longitude: 200, Source: sourceMWGG},
	}

	merged, stats := mergeSources(oa, mw)
	if len(merged) != 0 {
		t.Errorf("merged count = %d, want 0 — every record has unusable coordinates", len(merged))
	}
	if stats.Dropped != 3 {
		t.Errorf("Dropped = %d, want 3", stats.Dropped)
	}
}

func TestMergeSources_KeepsValidSideOfABadPair(t *testing.T) {
	oa := map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt", Latitude: 0, Longitude: 0, Source: sourceOurAirports},
	}
	mw := map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main", Latitude: 50.0264, Longitude: 8.5431, Source: sourceMWGG},
	}

	merged, stats := mergeSources(oa, mw)
	if stats.Dropped != 0 {
		t.Errorf("Dropped = %d, want 0", stats.Dropped)
	}
	got := merged["EDDF"]
	if got.Latitude == 0 && got.Longitude == 0 {
		t.Error("merged record kept the placeholder coordinates")
	}
}

func TestMergeSources_SingleSource(t *testing.T) {
	mw := map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main", Latitude: 50.0264, Longitude: 8.5431, Source: sourceMWGG},
	}

	merged, stats := mergeSources(nil, mw)
	if len(merged) != 1 || stats.OnlyMWGG != 1 {
		t.Errorf("merged = %d records, stats = %+v, want the mwgg record alone", len(merged), stats)
	}
}

func TestScore_InvalidCoordinatesRankLast(t *testing.T) {
	valid := AirportInfo{Name: "A", Latitude: 50, Longitude: 8}
	if score(valid) < 0 {
		t.Errorf("score(valid) = %d, want >= 0", score(valid))
	}
	for _, bad := range []AirportInfo{
		{Name: "null island", Latitude: 0, Longitude: 0},
		{Name: "lat too high", Latitude: 91, Longitude: 0},
		{Name: "lon too low", Latitude: 0, Longitude: -181},
	} {
		if score(bad) >= 0 {
			t.Errorf("score(%q) = %d, want < 0", bad.Name, score(bad))
		}
	}
}

func withICAO(a AirportInfo, icao, source string) AirportInfo {
	a.ICAO = icao
	a.Source = source
	return a
}

func setName(a AirportInfo, name string) AirportInfo {
	a.Name = name
	return a
}
