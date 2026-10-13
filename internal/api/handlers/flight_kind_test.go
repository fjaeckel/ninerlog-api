package handlers

import (
	"errors"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestValidateCreateShape_TimeModel(t *testing.T) {
	s := func(v string) *string { return &v }
	n := func(v int) *int { return &v }
	yes := true
	flight := func(off, on, to, ldg *string) *generated.FlightCreate {
		return &generated.FlightCreate{
			AircraftReg: s("D-KFAL"), AircraftType: "SF25",
			DepartureIcao: s("EDBO"), ArrivalIcao: s("EDAZ"), Landings: n(1),
			OffBlockTime: off, OnBlockTime: on, DepartureTime: to, ArrivalTime: ldg,
		}
	}
	tests := []struct {
		name    string
		req     *generated.FlightCreate
		wantErr error
	}{
		{"A2 block times unchanged", flight(s("08:00"), s("09:00"), nil, nil), nil},
		{"L3/K3 take-off and landing only", flight(nil, nil, s("08:10"), s("08:50")), nil},
		{"both pairs", flight(s("08:00"), s("09:00"), s("08:10"), s("08:50")), nil},
		{"block pair with lone take-off", flight(s("08:00"), s("09:00"), s("08:10"), nil), nil},
		{"no times", flight(nil, nil, nil, nil), models.ErrFlightTimesMissing},
		{"lone off-block", flight(s("08:00"), nil, nil, nil), models.ErrFlightTimePairIncomplete},
		{"lone on-block", flight(nil, s("09:00"), nil, nil), models.ErrFlightTimePairIncomplete},
		{"lone take-off", flight(nil, nil, s("08:10"), nil), models.ErrFlightTimePairIncomplete},
		{"lone landing", flight(nil, nil, nil, s("08:50")), models.ErrFlightTimePairIncomplete},
		{"missing route still rejected", &generated.FlightCreate{
			AircraftReg: s("D-KFAL"), AircraftType: "SF25", Landings: n(1),
			DepartureTime: s("08:10"), ArrivalTime: s("08:50"),
		}, errFlightFieldsMissing},
		{"simulator session unchanged", &generated.FlightCreate{
			AircraftType: "A320", IsSimulator: &yes, FstdType: s("FFS"), SimulatedFlightTime: n(120),
		}, nil},
		{"simulator session still rejects block times", &generated.FlightCreate{
			AircraftType: "A320", IsSimulator: &yes, FstdType: s("FFS"), SimulatedFlightTime: n(120),
			OffBlockTime: s("08:00"),
		}, errSessionFieldsUnused},
		{"simulator session still needs its duration", &generated.FlightCreate{
			AircraftType: "A320", IsSimulator: &yes, FstdType: s("FFS"),
		}, errSessionFieldsNeeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateShape(tt.req)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
