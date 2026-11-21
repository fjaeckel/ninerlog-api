package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHobbsFlightUpdate(t *testing.T) {
	start, end := 15234, 15247
	off, on := "08:00", "10:00"
	for _, tt := range []struct {
		name, body string
		want       int
		method     string
		invalid    bool
	}{
		{"timestamps never override Hobbs", `{"offBlockTime":"08:00","onBlockTime":"12:00","totalTime":999}`, 78, "hobbs", false},
		{"remarks preserve saved total", `{"remarks":"updated"}`, 78, "hobbs", false},
		{"updated meter reading", `{"hobbsEndTenths":15249}`, 90, "hobbs", false},
		{"missing reading rejected", `{"hobbsEndTenths":null}`, 0, "hobbs", true},
		{"reversed reading rejected", `{"hobbsEndTenths":15233}`, 0, "hobbs", true},
		{"per-flight block override", `{"flightTimeMethod":"block"}`, 120, "block", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := models.Flight{FlightTimeMethod: "hobbs", HobbsStartTenths: &start, HobbsEndTenths: &end, TotalTime: 78, OffBlockTime: &off, OnBlockTime: &on}
			var req generated.UpdateFlightJSONRequestBody
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))
			applyNullable(&f.OffBlockTime, req.OffBlockTime)
			applyNullable(&f.OnBlockTime, req.OnBlockTime)
			err := applyFlightTimeUpdate(&f, &req)
			if tt.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, f.TotalTime)
			require.Equal(t, tt.method, f.FlightTimeMethod)
			if tt.method == "block" {
				require.Equal(t, &start, f.HobbsStartTenths)
				require.Equal(t, &end, f.HobbsEndTenths)
			}
		})
	}
	f := models.Flight{TotalTime: 83, OffBlockTime: &off, OnBlockTime: &on}
	require.NoError(t, applyFlightTimeUpdate(&f, &generated.UpdateFlightJSONRequestBody{}))
	require.Equal(t, 83, f.TotalTime)
	var clear generated.UpdateFlightJSONRequestBody
	require.NoError(t, json.Unmarshal([]byte(`{"offBlockTime":null}`), &clear))
	applyNullable(&f.OffBlockTime, clear.OffBlockTime)
	require.NoError(t, applyFlightTimeUpdate(&f, &clear))
	require.Equal(t, 83, f.TotalTime)

}

func TestHobbsCreateShape(t *testing.T) {
	var req generated.FlightCreate
	require.NoError(t, json.Unmarshal([]byte(`{"date":"2026-11-14","aircraftReg":"N12345","aircraftType":"C172","departureIcao":"KORD","arrivalIcao":"KORD","landings":1,"flightTimeMethod":"hobbs","hobbsStartTenths":15234,"hobbsEndTenths":15247}`), &req))
	require.NoError(t, validateCreateShape(&req))
	f := models.Flight{FlightTimeMethod: "hobbs", HobbsStartTenths: req.HobbsStartTenths, HobbsEndTenths: req.HobbsEndTenths}
	total, err := selectedFlightMinutes(&f)
	require.NoError(t, err)
	require.Equal(t, 78, total)
	sim := true
	req.IsSimulator = &sim
	require.Error(t, validateCreateShape(&req))
	require.Error(t, json.Unmarshal([]byte(`{"hobbsStartTenths":1523.4}`), &req))
}

func TestHobbsCSV_H1RoundTrip(t *testing.T) {
	start, end := 15234, 15247
	off, on, dep, arr := "08:00:00", "10:00:00", "EDDF", "EDDF"
	f := &models.Flight{Date: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), AircraftReg: "N12345", AircraftType: "C172", DepartureICAO: &dep, ArrivalICAO: &arr, OffBlockTime: &off, OnBlockTime: &on, FlightTimeMethod: "hobbs", HobbsStartTenths: &start, HobbsEndTenths: &end, TotalTime: 78, AllLandings: 1}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	writeStandardCSV(w, []*models.Flight{f}, exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "comma"})
	w.Flush()
	rows, err := csv.NewReader(&buf).ReadAll()
	require.NoError(t, err)
	row := map[string]string{}
	for i, h := range rows[0] {
		row[h] = rows[1][i]
	}
	mappings := map[string]generated.ImportColumnMapping{}
	for _, m := range importtemplate.ByID("NINERLOG_CSV").Suggest(rows[0]) {
		mappings[m.SourceColumn] = generated.ImportColumnMapping{SourceColumn: m.SourceColumn, TargetField: generated.ImportField(m.TargetField)}
	}
	imported, extras, errs := mapImportRow(row, mappings, nil, nil)
	require.Empty(t, errs)
	got := buildImportedFlight(uuid.New(), imported, extras)
	flightcalc.ApplyAutoCalculations(&got, "Pilot", nil)
	require.Equal(t, "hobbs", got.FlightTimeMethod)
	require.Equal(t, 78, got.TotalTime)
	require.Equal(t, &start, got.HobbsStartTenths)
	require.Equal(t, &end, got.HobbsEndTenths)
	require.Equal(t, 78, got.PICTime)
}

func TestManualFlightSourceAndIndependentCredits(t *testing.T) {
	minutes := 90
	f := models.Flight{FlightTimeMethod: models.FlightTimeManual, ManualTotalTime: &minutes, TotalTime: 90, PICTime: 66, DualTime: 90, PICTimeOverride: true, DualTimeOverride: true}
	flightcalc.ApplyAutoCalculations(&f, "Pilot", nil)
	require.Equal(t, 90, f.TotalTime)
	require.Equal(t, 66, f.PICTime)
	require.Equal(t, 90, f.DualTime)
	require.Zero(t, f.SoloTime)
	require.NoError(t, f.ValidateTimeDistribution())
	require.NoError(t, f.ValidateFlightTimeSource())
	var req generated.UpdateFlightJSONRequestBody
	require.NoError(t, json.Unmarshal([]byte(`{"offBlockTime":"08:00","onBlockTime":"12:00","totalTime":240}`), &req))
	applyNullable(&f.OffBlockTime, req.OffBlockTime)
	applyNullable(&f.OnBlockTime, req.OnBlockTime)
	require.NoError(t, applyFlightTimeUpdate(&f, &req))
	require.Equal(t, 90, f.TotalTime)
	require.NoError(t, json.Unmarshal([]byte(`{"picTime":0,"dualTime":null}`), &req))
	applyOverride(&f.PICTime, &f.PICTimeOverride, req.PicTime)
	applyOverride(&f.DualTime, &f.DualTimeOverride, req.DualTime)
	flightcalc.ApplyAutoCalculations(&f, "Pilot", nil)
	require.Zero(t, f.PICTime)
	require.True(t, f.PICTimeOverride)
	require.False(t, f.DualTimeOverride)
	f.PICTime = 91
	require.Error(t, f.ValidateTimeDistribution())
}

func TestManualCSVExactRoundTrip(t *testing.T) {
	for _, credit := range []int{66, 0} {
		t.Run(strconv.Itoa(credit), func(t *testing.T) {
			minutes := 83
			dep, arr := "KORD", "KORD"
			f := &models.Flight{Date: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), AircraftReg: "N12345", AircraftType: "C172", FlightTimeMethod: "manual", ManualTotalTime: &minutes, DepartureICAO: &dep, ArrivalICAO: &arr, TotalTime: 83, PICTime: credit, DualTime: credit, PICTimeOverride: true, DualTimeOverride: true}
			var buf bytes.Buffer
			w := csv.NewWriter(&buf)
			writeStandardCSV(w, []*models.Flight{f}, exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "comma"})
			w.Flush()
			rows, err := csv.NewReader(&buf).ReadAll()
			require.NoError(t, err)
			row := map[string]string{}
			for i, h := range rows[0] {
				row[h] = rows[1][i]
			}
			mappings := map[string]generated.ImportColumnMapping{}
			for _, m := range importtemplate.ByID("NINERLOG_CSV").Suggest(rows[0]) {
				mappings[m.SourceColumn] = generated.ImportColumnMapping{SourceColumn: m.SourceColumn, TargetField: generated.ImportField(m.TargetField)}
			}
			mapped, extras, errs := mapImportRow(row, mappings, nil, nil)
			require.Empty(t, errs)
			restored := buildImportedFlight(uuid.New(), mapped, extras)
			flightcalc.ApplyAutoCalculations(&restored, "Pilot", nil)
			require.Equal(t, 83, restored.TotalTime)
			require.Equal(t, &minutes, restored.ManualTotalTime)
			require.Equal(t, credit, restored.PICTime)
			require.Equal(t, credit, restored.DualTime)
			require.True(t, restored.PICTimeOverride)
			require.True(t, restored.DualTimeOverride)
		})
	}
}
