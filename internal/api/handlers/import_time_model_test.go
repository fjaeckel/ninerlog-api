package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/google/uuid"
)

func TestImport_TakeoffLandingOnlyRow(t *testing.T) {
	mappings := map[string]generated.ImportColumnMapping{
		"Datum":   {SourceColumn: "Datum", TargetField: "date"},
		"Kennz":   {SourceColumn: "Kennz", TargetField: "aircraftReg"},
		"Muster":  {SourceColumn: "Muster", TargetField: "aircraftType"},
		"Von":     {SourceColumn: "Von", TargetField: "departureIcao"},
		"Nach":    {SourceColumn: "Nach", TargetField: "arrivalIcao"},
		"Start":   {SourceColumn: "Start", TargetField: "departureTime"},
		"Landung": {SourceColumn: "Landung", TargetField: "arrivalTime"},
		"Ldg":     {SourceColumn: "Ldg", TargetField: "landings"},
		"Zeit":    {SourceColumn: "Zeit", TargetField: "totalTime"},
		"OffBlk":  {SourceColumn: "OffBlk", TargetField: "offBlockTime"},
		"OnBlk":   {SourceColumn: "OnBlk", TargetField: "onBlockTime"},
	}
	base := func() map[string]string {
		return map[string]string{
			"Datum": "2026-05-01", "Kennz": "D-1234", "Muster": "ASK21",
			"Von": "EDBO", "Nach": "EDBO", "Ldg": "1",
		}
	}
	tests := []struct {
		name      string
		row       func() map[string]string
		wantTotal int
	}{
		{
			name: "L3/K3 take-off and landing only",
			row: func() map[string]string {
				r := base()
				r["Start"], r["Landung"] = "10:05", "10:47"
				return r
			},
			wantTotal: 42,
		},
		{
			name: "L3/K3 take-off and landing only across midnight",
			row: func() map[string]string {
				r := base()
				r["Start"], r["Landung"] = "23:50", "00:20"
				return r
			},
			wantTotal: 30,
		},
		{
			name: "total-time cell wins over take-off/landing",
			row: func() map[string]string {
				r := base()
				r["Start"], r["Landung"], r["Zeit"] = "10:05", "10:47", "0:40"
				return r
			},
			wantTotal: 40,
		},
		{
			name: "A2 block times unchanged: block span wins",
			row: func() map[string]string {
				r := base()
				r["OffBlk"], r["OnBlk"] = "09:55", "10:55"
				r["Start"], r["Landung"], r["Zeit"] = "10:05", "10:47", "0:40"
				return r
			},
			wantTotal: 60,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, errs := mapRowToFlight(tt.row(), mappings, nil)
			if len(errs) > 0 {
				t.Fatalf("mapping errors: %+v", errs)
			}
			if got := importTotalMinutes(f); got != tt.wantTotal {
				t.Errorf("importTotalMinutes = %d, want %d", got, tt.wantTotal)
			}
		})
	}
}

func TestImport_TakeoffLandingOnlyRowPassesAutoCalculation(t *testing.T) {
	mappings := map[string]generated.ImportColumnMapping{
		"Datum":   {SourceColumn: "Datum", TargetField: "date"},
		"Kennz":   {SourceColumn: "Kennz", TargetField: "aircraftReg"},
		"Von":     {SourceColumn: "Von", TargetField: "departureIcao"},
		"Nach":    {SourceColumn: "Nach", TargetField: "arrivalIcao"},
		"Start":   {SourceColumn: "Start", TargetField: "departureTime"},
		"Landung": {SourceColumn: "Landung", TargetField: "arrivalTime"},
	}
	row := map[string]string{"Datum": "2026-05-01", "Kennz": "D-1234", "Von": "EDBO", "Nach": "EDBO", "Start": "10:05", "Landung": "10:47"}
	f, errs := mapRowToFlight(row, mappings, nil)
	if len(errs) > 0 {
		t.Fatalf("mapping errors: %+v", errs)
	}
	flight := importedFlight(uuid.New(), f)
	flightcalc.ApplyAutoCalculations(&flight, "", nil)
	if !flight.IsValid() {
		t.Fatalf("imported flight is not valid: total=%d reg=%q", flight.TotalTime, flight.AircraftReg)
	}
	if flight.TotalTime != 42 || flight.OffBlockTime != nil || flight.DepartureTime == nil {
		t.Errorf("total=%d offBlock=%v departure=%v, want 42/nil/set", flight.TotalTime, flight.OffBlockTime, flight.DepartureTime)
	}
}
