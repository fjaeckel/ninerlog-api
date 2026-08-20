package airports

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"sort"
	"testing"
)

func testPackData() map[string]AirportInfo {
	return map[string]AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main", Latitude: 50.0333, Longitude: 8.5706, Elevation: 364, Country: "DE"},
		"EDXR": {ICAO: "EDXR", Name: "Rendsburg-Schachtholm", Latitude: 54.3, Longitude: 9.5, Country: "DE"},
		"LOWI": {ICAO: "LOWI", Name: "Innsbruck", Latitude: 47.2602, Longitude: 11.3439, Elevation: 1907, Country: "AT"},
	}
}

func decodePack(t *testing.T, gz []byte) packEnvelope {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading pack: %v", err)
	}
	var env packEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshalling envelope: %v", err)
	}
	return env
}

func TestPack_UnavailableWithoutDB(t *testing.T) {
	SetTestDB(nil)
	if _, _, ok := Pack(); ok {
		t.Error("Pack ok without a database")
	}
	if _, ok := PackInfo(); ok {
		t.Error("PackInfo ok without a database")
	}
}

func TestPack_EnvelopeContent(t *testing.T) {
	SetTestDB(testPackData())
	defer SetTestDB(nil)

	gz, status, ok := Pack()
	if !ok {
		t.Fatal("Pack not ok")
	}
	if status.SizeBytes != len(gz) {
		t.Errorf("SizeBytes = %d, len(gz) = %d", status.SizeBytes, len(gz))
	}

	env := decodePack(t, gz)
	if env.Etag != status.Etag {
		t.Errorf("envelope etag %q != status etag %q", env.Etag, status.Etag)
	}
	if env.Count != 3 || status.Count != 3 {
		t.Errorf("count = %d / %d, want 3", env.Count, status.Count)
	}
	if env.GeneratedAt.IsZero() {
		t.Error("envelope generatedAt is zero")
	}

	var records []packAirport
	if err := json.Unmarshal(env.Airports, &records); err != nil {
		t.Fatalf("unmarshalling airports: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	if !sort.SliceIsSorted(records, func(i, j int) bool { return records[i].ICAO < records[j].ICAO }) {
		t.Error("records not sorted by ICAO")
	}
	if records[0].ICAO != "EDDF" || records[0].Name != "Frankfurt am Main" ||
		records[0].Elevation != 364 || records[0].Country != "DE" {
		t.Errorf("unexpected first record: %+v", records[0])
	}
}

func TestPack_EtagTracksContent(t *testing.T) {
	defer SetTestDB(nil)

	SetTestDB(testPackData())
	_, first, _ := Pack()

	// Same data, new snapshot (new loadedAt): etag must not change.
	SetTestDB(testPackData())
	_, second, _ := Pack()
	if first.Etag != second.Etag {
		t.Errorf("etag changed for identical data: %q vs %q", first.Etag, second.Etag)
	}

	// Changed data: etag must change.
	changed := testPackData()
	changed["LSZH"] = AirportInfo{ICAO: "LSZH", Name: "Zürich", Latitude: 47.4647, Longitude: 8.5492, Country: "CH"}
	SetTestDB(changed)
	_, third, _ := Pack()
	if third.Etag == first.Etag {
		t.Error("etag unchanged after data changed")
	}
	if third.Count != 4 {
		t.Errorf("count = %d, want 4", third.Count)
	}
}

func TestPackInfo_MatchesPack(t *testing.T) {
	SetTestDB(testPackData())
	defer SetTestDB(nil)

	info, ok := PackInfo()
	if !ok {
		t.Fatal("PackInfo not ok")
	}
	gz, status, _ := Pack()
	if info != status {
		t.Errorf("PackInfo %+v != Pack status %+v", info, status)
	}
	if info.SizeBytes != len(gz) {
		t.Errorf("SizeBytes = %d, len(gz) = %d", info.SizeBytes, len(gz))
	}
}
