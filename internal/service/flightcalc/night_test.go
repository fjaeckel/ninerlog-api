package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func setupWorldAirports(t *testing.T) {
	t.Helper()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"KFDK": {ICAO: "KFDK", Name: "Frederick Municipal", Latitude: 39.4176, Longitude: -77.3743, Country: "US"},
		"KSFO": {ICAO: "KSFO", Name: "San Francisco Intl", Latitude: 37.6188, Longitude: -122.3750, Country: "US"},
		"YSSY": {ICAO: "YSSY", Name: "Sydney Kingsford Smith", Latitude: -33.9461, Longitude: 151.1772, Country: "AU"},
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706, Country: "DE"},
		"ENSB": {ICAO: "ENSB", Name: "Svalbard Longyear", Latitude: 78.2461, Longitude: 15.4656, Country: "NO"},
		"NZAA": {ICAO: "NZAA", Name: "Auckland", Latitude: -37.0081, Longitude: 174.7917, Country: "NZ"},
		"PHNL": {ICAO: "PHNL", Name: "Honolulu Intl", Latitude: 21.3187, Longitude: -157.9225, Country: "US"},
		"SEQM": {ICAO: "SEQM", Name: "Quito Mariscal Sucre", Latitude: -0.1292, Longitude: -78.3575, Country: "EC"},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })
}

func TestNightDerivations_WorldAirports(t *testing.T) {
	tests := []struct {
		name         string
		icao         string
		date         string
		offBlock     string
		onBlock      string
		total        int
		wantNight    int
		wantTONight  bool
		wantLdgNight bool
	}{
		{"KFDK June evening, civil dusk 01:11:48Z", "KFDK", "2026-06-20", "00:00:00", "01:30:00", 90, 18, false, true},
		{"KSFO July evening, civil dusk 04:01:33Z", "KSFO", "2026-07-15", "03:00:00", "05:00:00", 120, 58, false, true},
		{"YSSY October morning, civil dawn 18:41:12Z", "YSSY", "2026-10-20", "18:00:00", "20:00:00", 120, 42, true, false},
		{"EDDF January evening, civil dusk 16:28:09Z", "EDDF", "2026-01-15", "16:00:00", "17:30:00", 90, 61, false, true},
		{"ENSB December polar night", "ENSB", "2026-12-15", "10:00:00", "12:00:00", 120, 120, true, true},
		{"ENSB June midnight sun across 00:00Z", "ENSB", "2026-06-15", "23:00:00", "01:00:00", 120, 0, false, false},
		{"NZAA October morning, civil dawn 17:03:28Z", "NZAA", "2026-10-19", "17:00:00", "19:00:00", 120, 4, true, false},
		{"PHNL October evening, civil dusk 04:26:38Z", "PHNL", "2026-10-20", "04:00:00", "06:00:00", 120, 93, false, true},
		{"SEQM equinox evening across 00:00Z, civil dusk 23:44:34Z", "SEQM", "2026-03-21", "23:00:00", "01:00:00", 120, 75, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupWorldAirports(t)
			date, err := time.Parse("2006-01-02", tt.date)
			if err != nil {
				t.Fatal(err)
			}
			f := &models.Flight{
				Date:          date,
				DepartureICAO: strPtr(tt.icao),
				ArrivalICAO:   strPtr(tt.icao),
				OffBlockTime:  strPtr(tt.offBlock),
				OnBlockTime:   strPtr(tt.onBlock),
				TotalTime:     tt.total,
				AllLandings:   1,
			}

			calculateNightTime(f)
			if f.NightTime != tt.wantNight {
				t.Errorf("NightTime = %d, want %d", f.NightTime, tt.wantNight)
			}

			calculateTakeoffSplit(f)
			if got := f.TakeoffsNight == 1; got != tt.wantTONight || f.TakeoffsDay+f.TakeoffsNight != 1 {
				t.Errorf("takeoffs day=%d night=%d, want night=%v", f.TakeoffsDay, f.TakeoffsNight, tt.wantTONight)
			}

			calculateLandingSplit(f)
			if got := f.LandingsNight == 1; got != tt.wantLdgNight || f.LandingsDay+f.LandingsNight != 1 {
				t.Errorf("landings day=%d night=%d, want night=%v", f.LandingsDay, f.LandingsNight, tt.wantLdgNight)
			}
		})
	}
}
