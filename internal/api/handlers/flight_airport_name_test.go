package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// Airport names are resolved fresh on every response rather than stored with
// the flight, so convertToGeneratedFlight must fill them in from the airport
// database — and must leave them nil for locations that do not resolve, since
// off-airport sites are stored as free text and the client falls back to
// rendering the raw stored value.
func TestConvertToGeneratedFlight_AirportNames(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main Airport", Latitude: 50.0333, Longitude: 8.5706},
		"EDXR": {ICAO: "EDXR", Name: "Rendsburg-Schachtholm", Latitude: 54.3, Longitude: 9.5},
	})
	defer airports.SetTestDB(nil)

	strp := func(s string) *string { return &s }

	tests := []struct {
		name     string
		dep, arr *string
		wantDep  *string
		wantArr  *string
	}{
		{
			name: "known codes resolve to names",
			dep:  strp("EDDF"), arr: strp("EDXR"),
			wantDep: strp("Frankfurt am Main Airport"), wantArr: strp("Rendsburg-Schachtholm"),
		},
		{
			name: "lowercase code still resolves",
			dep:  strp("eddf"), arr: strp("EDDF"),
			wantDep: strp("Frankfurt am Main Airport"), wantArr: strp("Frankfurt am Main Airport"),
		},
		{
			name: "unknown code yields no name",
			dep:  strp("ZZZZ"), arr: strp("EDDF"),
			wantDep: nil, wantArr: strp("Frankfurt am Main Airport"),
		},
		{
			name: "free-text off-airport site yields no name",
			dep:  strp("Meadow strip"), arr: strp("Grandpa's field"),
			wantDep: nil, wantArr: nil,
		},
		{
			name: "nil locations yield no name",
			dep:  nil, arr: nil,
			wantDep: nil, wantArr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &models.Flight{
				ID:            uuid.New(),
				UserID:        uuid.New(),
				DepartureICAO: tt.dep,
				ArrivalICAO:   tt.arr,
			}

			got := convertToGeneratedFlight(f)

			assertNameEqual(t, "departure", got.DepartureAirportName, tt.wantDep)
			assertNameEqual(t, "arrival", got.ArrivalAirportName, tt.wantArr)

			// The stored location itself must always pass through untouched —
			// resolving a name must never rewrite what was logged.
			assertNameEqual(t, "departureIcao", got.DepartureIcao, tt.dep)
			assertNameEqual(t, "arrivalIcao", got.ArrivalIcao, tt.arr)
		})
	}
}

func assertNameEqual(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %q, want %q", field, *got, *want)
	}
}
