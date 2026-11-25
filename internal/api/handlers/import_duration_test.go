package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
)

func durationFormatPtr(f generated.ImportColumnMappingDurationFormat) *generated.ImportColumnMappingDurationFormat {
	return &f
}

// faaDecimalRows is a decimal-hour FAA logbook where whole-hour flights carry
// no fraction (issue #297).
func faaDecimalRows() []map[string]string {
	return []map[string]string{
		{"Date": "03/02/2019", "Tail": "N734TB", "From": "KFDK", "To": "KHGR", "Total": "1.3", "Night": "", "XC": "1.3"},
		{"Date": "03/09/2019", "Tail": "N734TB", "From": "KFDK", "To": "KMRB", "Total": "2", "Night": "1", "XC": "2"},
		{"Date": "03/16/2019", "Tail": "N734TB", "From": "KFDK", "To": "KFDK", "Total": "0.8", "Night": "", "XC": ""},
	}
}

func faaDecimalMappings(totalFormat *generated.ImportColumnMappingDurationFormat) map[string]generated.ImportColumnMapping {
	return toMappingLookup([]generated.ImportColumnMapping{
		{SourceColumn: "Date", TargetField: generated.ImportFieldDate},
		{SourceColumn: "Tail", TargetField: generated.ImportFieldAircraftReg},
		{SourceColumn: "From", TargetField: generated.ImportFieldDepartureIcao},
		{SourceColumn: "To", TargetField: generated.ImportFieldArrivalIcao},
		{SourceColumn: "Total", TargetField: generated.ImportFieldTotalTime, DurationFormat: totalFormat},
		{SourceColumn: "Night", TargetField: generated.ImportFieldNightTime},
		{SourceColumn: "XC", TargetField: generated.ImportFieldCrossCountryTime},
	})
}

func TestImportDuration_FormatOption(t *testing.T) {
	tests := []struct {
		val    string
		format *generated.ImportColumnMappingDurationFormat
		want   int
		ok     bool
	}{
		{"2", nil, 2, true},
		{"2", durationFormatPtr(generated.ImportColumnMappingDurationFormatAuto), 2, true},
		{"2", durationFormatPtr(generated.ImportColumnMappingDurationFormatDecimal), 120, true},
		{"2.0", durationFormatPtr(generated.ImportColumnMappingDurationFormatDecimal), 120, true},
		{"10.2", durationFormatPtr(generated.ImportColumnMappingDurationFormatDecimal), 612, true},
		{"1,5", durationFormatPtr(generated.ImportColumnMappingDurationFormatDecimal), 90, true},
		{"1:18", durationFormatPtr(generated.ImportColumnMappingDurationFormatDecimal), 78, true},
		{"90", durationFormatPtr(generated.ImportColumnMappingDurationFormatMinutes), 90, true},
		{"1.5", durationFormatPtr(generated.ImportColumnMappingDurationFormatMinutes), 0, false},
	}
	for _, tt := range tests {
		name := tt.val + "/auto"
		if tt.format != nil {
			name = tt.val + "/" + string(*tt.format)
		}
		t.Run(name, func(t *testing.T) {
			got, err := importDuration(tt.val, generated.ImportColumnMapping{DurationFormat: tt.format})
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
			if tt.ok && got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// C1 Tom migrates a decimal-hour FAA logbook: a whole-number total is whole
// hours, not minutes.
func TestResolveDurationFormats_DecimalColumnReadsWholeNumbersAsHours(t *testing.T) {
	rows := faaDecimalRows()
	lookup := faaDecimalMappings(nil)
	resolveDurationFormats(lookup, nil, rows)

	if f := lookup["Total"].DurationFormat; f == nil || *f != generated.ImportColumnMappingDurationFormatDecimal {
		t.Fatalf("Total durationFormat = %v, want decimal", f)
	}
	if f := lookup["XC"].DurationFormat; f == nil || *f != generated.ImportColumnMappingDurationFormatDecimal {
		t.Fatalf("XC durationFormat = %v, want decimal", f)
	}
	if f := lookup["Night"].DurationFormat; f != nil {
		t.Errorf("Night durationFormat = %v, want unset for an integer-only column", *f)
	}

	flight, errs := mapRowToFlight(rows[1], lookup, nil)
	if len(errs) > 0 {
		t.Fatalf("errors = %v", errs)
	}
	if flight.TotalTime == nil || *flight.TotalTime != 120 {
		t.Errorf("totalTime = %v, want 120", flight.TotalTime)
	}
	if flight.CrossCountryTime == nil || *flight.CrossCountryTime != 120 {
		t.Errorf("crossCountryTime = %v, want 120", flight.CrossCountryTime)
	}
}

func TestResolveDurationFormats_ExplicitFormatWins(t *testing.T) {
	rows := faaDecimalRows()

	lookup := faaDecimalMappings(durationFormatPtr(generated.ImportColumnMappingDurationFormatMinutes))
	resolveDurationFormats(lookup, nil, rows)
	flight, errs := mapRowToFlight(rows[1], lookup, nil)
	if len(errs) > 0 {
		t.Fatalf("errors = %v", errs)
	}
	if flight.TotalTime == nil || *flight.TotalTime != 2 {
		t.Errorf("totalTime = %v, want 2 under an explicit minutes format", flight.TotalTime)
	}

	lookup = faaDecimalMappings(nil)
	lookup["Night"] = generated.ImportColumnMapping{
		SourceColumn:   "Night",
		TargetField:    generated.ImportFieldNightTime,
		DurationFormat: durationFormatPtr(generated.ImportColumnMappingDurationFormatDecimal),
	}
	resolveDurationFormats(lookup, nil, rows)
	flight, _ = mapRowToFlight(rows[1], lookup, nil)
	if flight.NightTime == nil || *flight.NightTime != 60 {
		t.Errorf("nightTime = %v, want 60 under an explicit decimal format", flight.NightTime)
	}
}

// Guard: an H:MM column and an integer-minute column import as before.
func TestResolveDurationFormats_HMAndMinuteColumnsUnchanged(t *testing.T) {
	rows := []map[string]string{
		{"Block": "1:30", "Night": "45"},
		{"Block": "2", "Night": "90"},
	}
	lookup := toMappingLookup([]generated.ImportColumnMapping{
		{SourceColumn: "Block", TargetField: generated.ImportFieldTotalTime},
		{SourceColumn: "Night", TargetField: generated.ImportFieldNightTime},
	})
	resolveDurationFormats(lookup, nil, rows)
	for _, col := range []string{"Block", "Night"} {
		if f := lookup[col].DurationFormat; f != nil {
			t.Errorf("%s durationFormat = %v, want unset", col, *f)
		}
	}
	flight, _ := mapRowToFlight(rows[1], lookup, nil)
	if flight.TotalTime == nil || *flight.TotalTime != 2 {
		t.Errorf("totalTime = %v, want 2", flight.TotalTime)
	}
	if flight.NightTime == nil || *flight.NightTime != 90 {
		t.Errorf("nightTime = %v, want 90", flight.NightTime)
	}
}

func TestResolveDurationFormats_AuxiliaryColumn(t *testing.T) {
	headers := []string{"Date", "AircraftID", "From", "To", "TotalTime", "SIC", "PilotComments"}
	tpl, _, mappings := detectImportFormat(headers)
	aux := importtemplate.AuxColumns(tpl, headers)
	if aux["SIC"] != importtemplate.AuxSICTime {
		t.Fatalf("aux = %v, want SIC read as SIC time", aux)
	}
	rows := []map[string]string{
		{"Date": "2026-04-02", "AircraftID": "D-AIZY", "From": "EDDF", "To": "LEPA", "TotalTime": "2.2", "SIC": "2.2"},
		{"Date": "2026-04-03", "AircraftID": "D-AIZY", "From": "LEPA", "To": "EDDF", "TotalTime": "2", "SIC": "2"},
	}
	lookup := toMappingLookup(mappings)
	resolveDurationFormats(lookup, aux, rows)
	flight, _, _ := mapImportRow(rows[1], lookup, aux, nil)
	if flight.TotalTime == nil || *flight.TotalTime != 120 {
		t.Errorf("totalTime = %v, want 120", flight.TotalTime)
	}
	if flight.SicTime == nil || *flight.SicTime != 120 {
		t.Errorf("sicTime = %v, want 120", flight.SicTime)
	}
}

func TestSuggestDurationFormats(t *testing.T) {
	rows := faaDecimalRows()
	suggested := []generated.ImportColumnMapping{
		{SourceColumn: "Date", TargetField: generated.ImportFieldDate},
		{SourceColumn: "Total", TargetField: generated.ImportFieldTotalTime},
		{SourceColumn: "Night", TargetField: generated.ImportFieldNightTime},
	}
	suggestDurationFormats(suggested, rows)
	if suggested[0].DurationFormat != nil {
		t.Errorf("Date durationFormat = %v, want unset", *suggested[0].DurationFormat)
	}
	if f := suggested[1].DurationFormat; f == nil || *f != generated.ImportColumnMappingDurationFormatDecimal {
		t.Errorf("Total durationFormat = %v, want decimal", f)
	}
	if suggested[2].DurationFormat != nil {
		t.Errorf("Night durationFormat = %v, want unset", *suggested[2].DurationFormat)
	}
}
