package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func setupOvernightAirports(t *testing.T) {
	t.Helper()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EGLL": {ICAO: "EGLL", Name: "London Heathrow", Latitude: 51.4706, Longitude: -0.4619, Country: "GB"},
		"BIKF": {ICAO: "BIKF", Name: "Keflavik Intl", Latitude: 63.985, Longitude: -22.6056, Country: "IS"},
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706, Country: "DE"},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })
}

// BIKF civil twilight: dusk 2026-07-25 01:11:22Z; dusk 2026-07-26 00:56:58Z,
// dawn 2026-07-26 02:27:18Z. 01:05Z is day on 07-25 and night on 07-26.
func TestDayNightSplit_OvernightFlight(t *testing.T) {
	tests := []struct {
		name     string
		dep, arr string
		date     string
		offBlock string
		onBlock  string
		total    int
		landings int
		setup    func(f *models.Flight)

		wantNight      int
		wantTODay      int
		wantTONight    int
		wantLdgDay     int
		wantLdgNight   int
		wantAllLanding int
	}{
		{
			name: "overnight EGLL-BIKF, on-block 01:05Z falls on D+1 and is night",
			dep:  "EGLL", arr: "BIKF", date: "2026-07-25", offBlock: "22:30", onBlock: "01:05",
			total: 155, landings: 1,
			wantNight: 155, wantTONight: 1, wantLdgNight: 1, wantAllLanding: 1,
		},
		{
			name: "same-day EGLL-BIKF midday is day",
			dep:  "EGLL", arr: "BIKF", date: "2026-07-25", offBlock: "12:00", onBlock: "15:00",
			total: 180, landings: 1,
			wantNight: 0, wantTODay: 1, wantLdgDay: 1, wantAllLanding: 1,
		},
		{
			name: "same-day EDDF evening, landing after civil dusk 16:28:09Z",
			dep:  "EDDF", arr: "EDDF", date: "2026-01-15", offBlock: "16:00", onBlock: "17:30",
			total: 90, landings: 2,
			wantNight: 61, wantTODay: 2, wantLdgNight: 2, wantAllLanding: 2,
		},
		{
			name: "overnight with landing overrides keeps the declared split",
			dep:  "EGLL", arr: "BIKF", date: "2026-07-25", offBlock: "22:30", onBlock: "01:05",
			total: 155,
			setup: func(f *models.Flight) {
				f.LandingsDay, f.LandingsDayOverride = 1, true
				f.LandingsNight, f.LandingsNightOverride = 0, true
			},
			wantNight: 155, wantTONight: 1, wantLdgDay: 1, wantAllLanding: 1,
		},
		{
			name: "overnight with takeoff and night-time overrides keeps the declared values",
			dep:  "EGLL", arr: "BIKF", date: "2026-07-25", offBlock: "22:30", onBlock: "01:05",
			total: 155, landings: 1,
			setup: func(f *models.Flight) {
				f.TakeoffsDay, f.TakeoffsDayOverride = 1, true
				f.TakeoffsNight, f.TakeoffsNightOverride = 0, true
				f.NightTime, f.NightTimeOverride = 30, true
			},
			wantNight: 30, wantTODay: 1, wantLdgNight: 1, wantAllLanding: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupOvernightAirports(t)
			date, err := time.Parse("2006-01-02", tt.date)
			if err != nil {
				t.Fatal(err)
			}
			f := &models.Flight{
				Date:          date,
				AircraftReg:   "D-EFGH",
				AircraftType:  "C172",
				DepartureICAO: strPtr(tt.dep),
				ArrivalICAO:   strPtr(tt.arr),
				OffBlockTime:  strPtr(tt.offBlock),
				OnBlockTime:   strPtr(tt.onBlock),
				TotalTime:     tt.total,
				AllLandings:   tt.landings,
			}
			if tt.setup != nil {
				tt.setup(f)
			}
			ApplyAutoCalculations(f, "", nil)

			if f.NightTime != tt.wantNight {
				t.Errorf("NightTime = %d, want %d", f.NightTime, tt.wantNight)
			}
			if f.TakeoffsDay != tt.wantTODay || f.TakeoffsNight != tt.wantTONight {
				t.Errorf("takeoffs day/night = %d/%d, want %d/%d",
					f.TakeoffsDay, f.TakeoffsNight, tt.wantTODay, tt.wantTONight)
			}
			if f.LandingsDay != tt.wantLdgDay || f.LandingsNight != tt.wantLdgNight {
				t.Errorf("landings day/night = %d/%d, want %d/%d",
					f.LandingsDay, f.LandingsNight, tt.wantLdgDay, tt.wantLdgNight)
			}
			if f.AllLandings != tt.wantAllLanding {
				t.Errorf("AllLandings = %d, want %d", f.AllLandings, tt.wantAllLanding)
			}
		})
	}
}

func TestInstantsOf_RollsOnBlockPastMidnight(t *testing.T) {
	date := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		off, on *string
		wantOff time.Time
		wantOn  time.Time
	}{
		{"overnight", strPtr("22:30"), strPtr("01:05"),
			time.Date(2026, 7, 25, 22, 30, 0, 0, time.UTC), time.Date(2026, 7, 26, 1, 5, 0, 0, time.UTC)},
		{"same day", strPtr("12:00:00"), strPtr("15:00:00"),
			time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)},
		{"equal times stay on D", strPtr("12:00"), strPtr("12:00"),
			time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)},
		{"no off-block", nil, strPtr("01:05"),
			time.Time{}, time.Date(2026, 7, 25, 1, 5, 0, 0, time.UTC)},
		{"unparseable on-block", strPtr("22:30"), strPtr(""),
			time.Date(2026, 7, 25, 22, 30, 0, 0, time.UTC), time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := instantsOf(&models.Flight{Date: date, OffBlockTime: tt.off, OnBlockTime: tt.on})
			if !in.offBlock.Equal(tt.wantOff) || !in.onBlock.Equal(tt.wantOn) {
				t.Errorf("instantsOf = %v / %v, want %v / %v", in.offBlock, in.onBlock, tt.wantOff, tt.wantOn)
			}
		})
	}
}
