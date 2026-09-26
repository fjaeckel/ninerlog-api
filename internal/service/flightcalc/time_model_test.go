package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func takeoffLandingFlight(date time.Time, takeoff, landing string) *models.Flight {
	return &models.Flight{
		Date:          date,
		AircraftReg:   "D-KFAL",
		AircraftType:  "SF25",
		DepartureICAO: strPtr("EDBO"),
		ArrivalICAO:   strPtr("EDAZ"),
		DepartureTime: strPtr(takeoff),
		ArrivalTime:   strPtr(landing),
		AllLandings:   1,
	}
}

func setupEDBO(t *testing.T) {
	t.Helper()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDBO": {ICAO: "EDBO", Name: "Oehna", Latitude: 51.899734, Longitude: 13.052809, Country: "DE"},
		"EDAZ": {ICAO: "EDAZ", Name: "Schönhagen", Latitude: 52.204631, Longitude: 13.159526, Country: "DE"},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })
}

func TestApplyAutoCalculations_TimeModel(t *testing.T) {
	dusk := time.Date(2019, 3, 19, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		flight        func() *models.Flight
		wantTotal     int
		wantNight     int
		wantXC        int
		wantTakeoffsN int
		wantLandingsN int
	}{
		{
			name: "L3/K3 take-off and landing only: total and night from take-off to landing",
			flight: func() *models.Flight {
				return takeoffLandingFlight(dusk, "18:56:00", "19:19:00")
			},
			wantTotal: 23, wantNight: 23, wantXC: 23,
			wantTakeoffsN: 1, wantLandingsN: 1,
		},
		{
			name: "L3/K3 take-off and landing only: daytime flight",
			flight: func() *models.Flight {
				return takeoffLandingFlight(dusk, "10:00:00", "11:15:00")
			},
			wantTotal: 75, wantNight: 0, wantXC: 75,
		},
		{
			name: "L3/K3 take-off and landing only: across midnight",
			flight: func() *models.Flight {
				return takeoffLandingFlight(dusk, "23:30:00", "00:15:00")
			},
			wantTotal: 45, wantNight: 45, wantXC: 45,
			wantTakeoffsN: 1, wantLandingsN: 1,
		},
		{
			name: "A2 block times unchanged: block pair decides over take-off/landing",
			flight: func() *models.Flight {
				f := takeoffLandingFlight(dusk, "18:00:00", "18:30:00")
				f.OffBlockTime = strPtr("17:30:00")
				f.OnBlockTime = strPtr("19:19:00")
				f.TotalTime = 109
				return f
			},
			wantTotal: 109, wantNight: 87, wantXC: 109,
			wantTakeoffsN: 0, wantLandingsN: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupEDBO(t)
			f := tt.flight()
			ApplyAutoCalculations(f, "", nil)
			if f.TotalTime != tt.wantTotal {
				t.Errorf("TotalTime = %d, want %d", f.TotalTime, tt.wantTotal)
			}
			if f.NightTime != tt.wantNight {
				t.Errorf("NightTime = %d, want %d", f.NightTime, tt.wantNight)
			}
			if f.CrossCountryTime != tt.wantXC {
				t.Errorf("CrossCountryTime = %d, want %d", f.CrossCountryTime, tt.wantXC)
			}
			if f.TakeoffsNight != tt.wantTakeoffsN {
				t.Errorf("TakeoffsNight = %d, want %d", f.TakeoffsNight, tt.wantTakeoffsN)
			}
			if f.LandingsNight != tt.wantLandingsN {
				t.Errorf("LandingsNight = %d, want %d", f.LandingsNight, tt.wantLandingsN)
			}
			if f.PICTime != tt.wantTotal {
				t.Errorf("PICTime = %d, want %d", f.PICTime, tt.wantTotal)
			}
		})
	}
}
