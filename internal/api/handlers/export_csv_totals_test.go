package handlers

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func totalsTestFlights() []*models.Flight {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	fstd := "FNPT II"
	return []*models.Flight{
		{ID: uuid.New(), Date: day, AircraftReg: "D-EABC", AircraftType: "C172",
			TotalTime: 90, PICTime: 90, NightTime: 30, LandingsDay: 2, LandingsNight: 1,
			TakeoffsDay: 2, AllLandings: 3, Distance: 101.5, Holds: 1, ApproachesCount: 2},
		{ID: uuid.New(), Date: day, AircraftReg: "D-EXYZ", AircraftType: "PA28",
			TotalTime: 45, DualTime: 45, LandingsDay: 5, AllLandings: 5, Distance: 20.25},
		{ID: uuid.New(), Date: day, IsSimulator: true, FSTDType: &fstd, SimulatedFlightTime: 60},
	}
}

func renderCSV(t *testing.T, write func(w *csv.Writer)) [][]string {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	write(w)
	w.Flush()
	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	return records
}

// column returns the value of header name in row.
func column(t *testing.T, records [][]string, row []string, name string) string {
	t.Helper()
	for i, h := range records[0] {
		if h == name {
			return row[i]
		}
	}
	t.Fatalf("no column %q", name)
	return ""
}

func TestCSVTotalsRows(t *testing.T) {
	flights := totalsTestFlights()
	prefs := exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "dot"}

	tests := []struct {
		name  string
		write func(w *csv.Writer)
		want  map[string]string
	}{
		{"standard", func(w *csv.Writer) {
			writeStandardCSV(w, flights, prefs, nil)
			csvWrite(w, standardCSVTotals(flights, prefs))
		}, map[string]string{
			"Date": "Total (3 flights)", "TotalTime": "2.2h", "PIC": "1.5h", "DualReceived": "0.8h",
			"Night": "0.5h", "Distance": "121.8", "AllLandings": "8", "DayTakeoffs": "2",
			"Holds": "1", "ApproachesCount": "2", "SimulatedFlight": "1.0h", "AircraftID": "",
		}},
		{"easa", func(w *csv.Writer) {
			writeEASACSV(w, flights, prefs, "Pilot")
			csvWrite(w, easaCSVTotals(flights))
		}, map[string]string{
			"Date": "Total (3 flights)", "Total Time": "2:15", "Ldg Day": "7", "Ldg Night": "1",
			"Night": "0:30", "Dual": "0:45", "FSTD Time": "1:00", "A/C Reg": "",
		}},
		{"faa", func(w *csv.Writer) {
			writeFAACSV(w, flights, prefs)
			csvWrite(w, faaCSVTotals(flights))
		}, map[string]string{
			"Date": "Total (3 flights)", "Total": "2.2h", "PIC": "1.5h", "Day Ldg": "7",
			"Approaches": "2", "Holds": "1", "Night": "0.5h",
		}},
		{"weblogbook", func(w *csv.Writer) {
			writeWebLogbookCSV(w, flights, "Pilot")
			csvWrite(w, webLogbookCSVTotals(flights))
		}, map[string]string{
			"Date": "Total (3 flights)", "Time Total": "2:15", "Landings Day": "7",
			"Time Dual": "0:45", "SIM Time": "1:00", "Time Night": "0:30",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := renderCSV(t, tt.write)
			if len(records) != len(flights)+2 {
				t.Fatalf("got %d records, want header + %d flights + totals", len(records), len(flights))
			}
			totals := records[len(records)-1]
			if len(totals) != len(records[0]) {
				t.Fatalf("totals row has %d cells, header has %d", len(totals), len(records[0]))
			}
			for name, want := range tt.want {
				if got := column(t, records, totals, name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestCSVTotalsRow_NoFlights(t *testing.T) {
	prefs := exportPrefs{DecimalSeparator: "comma"}
	row := standardCSVTotals(nil, prefs)
	if row[0] != "Total (0 flights)" || row[10] != "0,0h" {
		t.Errorf("unexpected empty totals row: %q", row)
	}
}
