package handlers

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// writeWebLogbookRows runs the Web Logbook writer and returns its rows keyed by
// header, so assertions read like the column names Web Logbook maps on.
func writeWebLogbookRows(t *testing.T, flights []*models.Flight) []map[string]string {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	writeWebLogbookCSV(w, flights, "Alex Rivera")
	w.Flush()
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("export does not parse: %v", err)
	}
	var rows []map[string]string
	for _, rec := range records[1:] {
		row := make(map[string]string, len(rec))
		for i, v := range rec {
			row[records[0][i]] = v
		}
		rows = append(rows, row)
	}
	return rows
}

// Web Logbook's "Apply Web Logbook Mapping" profile matches these names
// verbatim; a renamed or reordered column silently drops out of the one-click
// mapping.
func TestWebLogbookCSV_HeaderMatchesWebLogbookExport(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	writeWebLogbookCSV(w, nil, "")
	w.Flush()

	want := "Date,Departure Place,Departure Time,Arrival Place,Arrival Time,Aircraft Model,Aircraft Reg," +
		"Time SE,Time ME,Time MCC,Time Total,Landings Day,Landings Night,Time Night,Time IFR,Time PIC," +
		"Time CoPilot,Time Dual,Time Instructor,SIM Type,SIM Time,PIC Name,Remarks,Tags"
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Errorf("header =\n%s\nwant\n%s", got, want)
	}
}

// Web Logbook's importer passes values through unconverted, so they must
// already be in its storage formats whatever the user's display preferences.
func TestWebLogbookCSV_FlightUsesWebLogbookFormats(t *testing.T) {
	f := roundTripSourceFlight()
	self := "SELF"
	f.PICName = &self
	f.InstructorName = nil
	f.IsIPC = true
	f.PICUSTime = 15

	rows := writeWebLogbookRows(t, []*models.Flight{f})
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	got := rows[0]

	want := map[string]string{
		"Date":            "07/03/2026",
		"Departure Place": "EDDF",
		"Departure Time":  "0815",
		"Arrival Place":   "EDDM",
		"Arrival Time":    "0945",
		"Aircraft Model":  "C172",
		"Aircraft Reg":    "D-EABC",
		"Time SE":         "1:30",
		"Time ME":         "",
		"Time MCC":        "",
		"Time Total":      "1:30",
		"Landings Day":    "2",
		"Landings Night":  "1",
		"Time Night":      "0:20",
		"Time IFR":        "0:30",
		"Time PIC":        "1:45", // PIC + PICUS, breakdown in remarks
		"Time CoPilot":    "",
		"SIM Type":        "",
		"SIM Time":        "",
		"PIC Name":        "Self",
		"Tags":            "IPC",
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("%s = %q, want %q", col, got[col], w)
		}
	}
	for _, part := range []string{"Round trip check", "[PICUS 0:15]", "[IPC]"} {
		if !strings.Contains(got["Remarks"], part) {
			t.Errorf("Remarks = %q, want it to contain %q", got["Remarks"], part)
		}
	}
}

// An FSTD session becomes a Web Logbook simulator record: no places, so its
// duplicate check and totals treat it as a sim session rather than a flight.
func TestWebLogbookCSV_SimulatorSession(t *testing.T) {
	dep := "EDDF"
	fstd := "FNPT II"
	f := &models.Flight{
		Date:                time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		AircraftReg:         "SIM",
		AircraftType:        "FNPT",
		DepartureICAO:       &dep,
		ArrivalICAO:         &dep,
		IsSimulator:         true,
		FSTDType:            &fstd,
		SimulatedFlightTime: 120,
	}

	got := writeWebLogbookRows(t, []*models.Flight{f})[0]
	want := map[string]string{
		"Date":            "15/01/2026",
		"Departure Place": "",
		"Arrival Place":   "",
		"Aircraft Model":  "",
		"Time Total":      "",
		"PIC Name":        "",
		"SIM Type":        "FNPT II",
		"SIM Time":        "2:00",
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("%s = %q, want %q", col, got[col], w)
		}
	}
}

// Multi-pilot time goes to Time MCC and the co-pilot column carries SIC plus
// relief, mirroring the EASA layout.
func TestWebLogbookCSV_MultiPilotCoPilot(t *testing.T) {
	f := roundTripSourceFlight()
	f.PICTime = 0
	f.IsPIC = false
	f.SICTime = 80
	f.ReliefTime = 10
	f.MultiPilotTime = 90

	got := writeWebLogbookRows(t, []*models.Flight{f})[0]
	want := map[string]string{
		"Time SE":      "",
		"Time MCC":     "1:30",
		"Time PIC":     "",
		"Time CoPilot": "1:30",
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("%s = %q, want %q", col, got[col], w)
		}
	}
}
