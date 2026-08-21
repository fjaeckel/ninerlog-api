package handlers

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func testDate(day int) time.Time {
	return time.Date(2025, 6, day, 0, 0, 0, 0, time.UTC)
}

// passengerLeg builds a flight the pilot was carried on: route and block
// times, no logged time in any column.
func passengerLeg(day int) *models.Flight {
	return &models.Flight{
		Date:         testDate(day),
		AircraftReg:  "D-EPAX",
		AircraftType: "C172",
		IsPassenger:  true,
	}
}

// fstdSession builds an FSTD session: no flight time, session time in the
// FSTD columns.
func fstdSession(day int) *models.Flight {
	kind := "FNPT II"
	return &models.Flight{
		Date:                testDate(day),
		AircraftType:        "FNPT II",
		IsSimulator:         true,
		FSTDType:            &kind,
		SimulatedFlightTime: 90,
	}
}

func loggedFlight(day, minutes int) *models.Flight {
	return &models.Flight{
		Date:         testDate(day),
		AircraftReg:  "D-EABC",
		AircraftType: "C172",
		TotalTime:    minutes,
		PICTime:      minutes,
	}
}

func TestDropEmptyRows(t *testing.T) {
	a, b := loggedFlight(1, 60), loggedFlight(3, 90)
	sim := fstdSession(7)

	tests := []struct {
		name string
		in   []*models.Flight
		want []*models.Flight
	}{
		{"nil slice", nil, []*models.Flight{}},
		{"all logged", []*models.Flight{a, b}, []*models.Flight{a, b}},
		{"passenger legs dropped", []*models.Flight{passengerLeg(2), passengerLeg(4)}, []*models.Flight{}},
		{"fstd session kept", []*models.Flight{sim}, []*models.Flight{sim}},
		{
			"mixed keeps order",
			[]*models.Flight{passengerLeg(2), a, sim, passengerLeg(4), b},
			[]*models.Flight{a, sim, b},
		},
		{"zero-total legacy row dropped", []*models.Flight{loggedFlight(5, 0), a}, []*models.Flight{a}},
		{"nil entry", []*models.Flight{nil, a}, []*models.Flight{a}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dropEmptyRows(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d flights, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestPassengerLegsExcludedFromTotals asserts a passenger leg neither counts
// towards the summary flight count nor shifts pagination.
func TestPassengerLegsExcludedFromTotals(t *testing.T) {
	const n = 12
	logged := buildSamplePDFFlights(n)

	mixed := make([]*models.Flight, 0, n+4)
	for i, f := range logged {
		if i%3 == 0 {
			mixed = append(mixed, passengerLeg(20+i))
		}
		mixed = append(mixed, f)
	}

	kept := dropEmptyRows(mixed)
	if len(kept) != n {
		t.Fatalf("kept %d flights, want %d", len(kept), n)
	}

	want := computeSummaryTotals(logged, nil)
	got := computeSummaryTotals(kept, nil)
	if got != want {
		t.Errorf("summary totals differ after filtering:\n got %+v\nwant %+v", got, want)
	}
	if got.flights != n {
		t.Errorf("summary flight count = %d, want %d", got.flights, n)
	}

	// Pagination must match an export that never held the empty entries.
	g := geometryFor("a4")
	for _, tc := range []struct {
		name   string
		render func([]*models.Flight) int
	}{
		{"easa single", func(fs []*models.Flight) int {
			return renderEASA(fs, g, nil, "Test Pilot", layoutSingle, nil).PageCount()
		}},
		{"faa spread", func(fs []*models.Flight) int {
			return generateFAAPDF(fs, g, "Test Pilot", layoutSpread, nil).PageCount()
		}},
	} {
		if got, want := tc.render(kept), tc.render(logged); got != want {
			t.Errorf("%s: got %d pages, want %d", tc.name, got, want)
		}
	}
}

// TestFSTDSessionSurvivesFiltering guards the regression the plain
// zero-total-time rule would have caused: an FSTD session carries no total
// time but must still print in the FSTD columns.
func TestFSTDSessionSurvivesFiltering(t *testing.T) {
	flights := []*models.Flight{loggedFlight(1, 60), fstdSession(2), passengerLeg(3)}

	kept := dropEmptyRows(flights)
	if len(kept) != 2 {
		t.Fatalf("kept %d rows, want 2", len(kept))
	}
	if !kept[1].IsSimulator {
		t.Fatalf("FSTD session was dropped from the export")
	}
	if got := computeSummaryTotals(kept, nil); got.total != 60 {
		t.Errorf("total block time = %d, want 60 (session time must not be flight time)", got.total)
	}
}
