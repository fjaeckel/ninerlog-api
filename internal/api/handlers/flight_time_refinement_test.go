package handlers

import (
	"encoding/json"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSourceChangesRetainEvidenceAndExplicitOverride(t *testing.T) {
	start, end, override := 15234, 15247, 90
	f := models.Flight{FlightTimeMethod: "hobbs", HobbsStartTenths: &start, HobbsEndTenths: &end, TotalTime: 78}
	update := func(body string) {
		var req generated.UpdateFlightJSONRequestBody
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		require.NoError(t, applyFlightTimeUpdate(&f, &req))
		flightcalc.ApplyAutoCalculations(&f, "Pilot", nil)
		require.NoError(t, f.ValidateFlightTimeSource())
	}
	update(`{"manualTotalTime":90}`)
	require.Equal(t, override, f.TotalTime)
	update(`{"hobbsEndTenths":15248}`)
	require.Equal(t, override, f.TotalTime)
	update(`{"flightTimeMethod":"manual"}`)
	require.Equal(t, start, *f.HobbsStartTenths)
	require.Equal(t, 15248, *f.HobbsEndTenths)
	update(`{"flightTimeMethod":"hobbs","manualTotalTime":null}`)
	require.Equal(t, 84, f.TotalTime)
}

func TestPaperLogbookImportWithExplicitUnits(t *testing.T) {
	unit := generated.ImportColumnMappingDurationUnitDecimalHours
	fields := map[string]string{"Date": "date", "Aircraft": "aircraftReg", "Type": "aircraftType", "Dep": "departureIcao", "Arr": "arrivalIcao", "Comments": "remarks", "Total time": "totalTime", "Sim instrument": "simulatedInstrumentTime", "Actual instrument": "actualInstrumentTime", "Night": "nightTime", "PIC": "picTime", "Dual Received": "dualTime"}
	row := map[string]string{"Date": "11/5/26", "Aircraft": "N5156CH", "Type": "172S", "Dep": "KBEC", "Arr": "KBEC", "Comments": "comments", "Total time": "1.5", "Sim instrument": "1.1", "Actual instrument": "0.0", "Night": "0.0", "PIC": "1.5", "Dual Received": "1.5"}
	mappings := map[string]generated.ImportColumnMapping{}
	for col, field := range fields {
		mappings[col] = generated.ImportColumnMapping{SourceColumn: col, TargetField: generated.ImportField(field), DurationUnit: &unit}
	}
	flight, extras, errs := mapImportRow(row, mappings, nil, nil)
	require.Empty(t, errs)
	f := buildImportedFlight(uuid.New(), flight, extras)
	flightcalc.ApplyAutoCalculations(&f, "Pilot", nil)
	require.Equal(t, "2026-11-05", f.Date.Format("2006-01-02"))
	require.Equal(t, "manual", f.FlightTimeMethod)
	require.Equal(t, 90, f.TotalTime)
	require.Equal(t, 66, f.SimulatedInstrumentTime)
	require.Equal(t, 90, f.PICTime)
	require.Equal(t, 90, f.DualTime)
	require.True(t, f.PICTimeOverride && f.DualTimeOverride && f.NightTimeOverride)
	require.Nil(t, f.OffBlockTime)
	require.NoError(t, f.ValidateTimeDistribution())
	row["Total time"] = "2"
	flight, _, errs = mapImportRow(row, mappings, nil, nil)
	require.Empty(t, errs)
	require.Equal(t, 120, *flight.TotalTime)
}

func TestImportedConflictingTotalRequiresDecision(t *testing.T) {
	row := map[string]string{"Date": "2026-11-05", "Aircraft": "N483QA", "Type": "C172", "Dep": "KBEC", "Arr": "KBEC", "Start": "1523.4", "End": "1524.7", "Total": "2.0"}
	mappings := map[string]generated.ImportColumnMapping{
		"Date": {TargetField: "date"}, "Aircraft": {TargetField: "aircraftReg"}, "Type": {TargetField: "aircraftType"}, "Dep": {TargetField: "departureIcao"}, "Arr": {TargetField: "arrivalIcao"},
		"Start": {TargetField: "hobbsStart"}, "End": {TargetField: "hobbsEnd"}, "Total": {TargetField: "totalTime"},
	}
	_, _, errs := mapImportRow(row, mappings, nil, nil)
	require.NotEmpty(t, errs)
	for _, policy := range []generated.ImportColumnMappingTotalTimePolicy{generated.Recorded, generated.Calculated} {
		mappings["Total"] = generated.ImportColumnMapping{TargetField: "totalTime", TotalTimePolicy: &policy}
		flight, extras, errs := mapImportRow(row, mappings, nil, nil)
		require.Empty(t, errs)
		f := buildImportedFlight(uuid.New(), flight, extras)
		flightcalc.ApplyAutoCalculations(&f, "Pilot", nil)
		want := 78
		if policy == generated.Recorded {
			want = 120
			require.NotNil(t, f.ManualTotalTime)
		}
		require.Equal(t, want, f.TotalTime)
		require.Equal(t, 15234, *f.HobbsStartTenths)
	}
}

func TestClockRemovalRequiresExplicitReview(t *testing.T) {
	off, on := "02:00", "04:00"
	f := models.Flight{FlightTimeMethod: "hobbs", OffBlockTime: &off, OnBlockTime: &on, NightTime: 78, LandingsNight: 1, AllLandings: 1, TakeoffsNight: 1}
	var req generated.UpdateFlightJSONRequestBody
	require.NoError(t, json.Unmarshal([]byte(`{"offBlockTime":null,"onBlockTime":null}`), &req))
	require.Error(t, reviewClockRemoval(&f, &req))
	require.NoError(t, json.Unmarshal([]byte(`{"offBlockTime":null,"onBlockTime":null,"clockTimePolicy":"preserve"}`), &req))
	require.NoError(t, reviewClockRemoval(&f, &req))
	applyNullable(&f.OffBlockTime, req.OffBlockTime)
	applyNullable(&f.OnBlockTime, req.OnBlockTime)
	flightcalc.ApplyAutoCalculations(&f, "Pilot", nil)
	require.Equal(t, 78, f.NightTime)
	require.Equal(t, 1, f.LandingsNight)
	require.True(t, f.NightTimeOverride && f.LandingsNightOverride)
}

func TestStandardRecordedDurationPreservesExactMinutes(t *testing.T) {
	unit := generated.ImportColumnMappingDurationUnitDecimalHours
	for _, separator := range []string{"dot", "comma"} {
		for _, minutes := range []int{0, 1, 66, 83, 90, 121} {
			exported := standardRecordedDuration(minutes, exportPrefs{DecimalSeparator: separator})
			actual, err := parseImportDuration(exported, &unit)
			require.NoError(t, err)
			require.Equal(t, minutes, actual, exported)
		}
	}
}

func TestImportDurationRejectsOverflowBeforeParsing(t *testing.T) {
	decimal := generated.ImportColumnMappingDurationUnitDecimalHours
	for _, unit := range []*generated.ImportColumnMappingDurationUnit{nil, &decimal} {
		for _, value := range []string{"9223372036854775808:00", "9223372036854775808h", "35791395:00", "2147483648m", "1.0e99h", "-1:00"} {
			_, err := parseImportDuration(value, unit)
			require.Error(t, err, value)
		}
		minutes, err := parseImportDuration("35791394:07", unit)
		require.NoError(t, err)
		require.Equal(t, 2147483647, minutes)
	}
}
