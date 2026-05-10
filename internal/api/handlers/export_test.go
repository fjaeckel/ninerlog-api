package handlers

import (
	"bytes"
	"encoding/csv"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestWriteEASACSV_UsesInstructorAsPICNameOnDualFlights(t *testing.T) {
	instructor := "CFI Mueller"
	flights := []*models.Flight{
		{
			Date:           time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			AircraftType:   "C172",
			AircraftReg:    "D-EABC",
			TotalTime:      90,
			DualTime:       90,
			InstructorName: &instructor,
		},
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	writeEASACSV(w, flights, exportPrefs{})
	w.Flush()

	records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("failed reading csv: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header and one row, got %d rows", len(records))
	}

	picNameColumn := -1
	for i, h := range records[0] {
		if h == "PIC Name" {
			picNameColumn = i
			break
		}
	}
	if picNameColumn == -1 {
		t.Fatal("PIC Name column not found")
	}

	if got := records[1][picNameColumn]; got != instructor {
		t.Fatalf("PIC Name = %q, want %q", got, instructor)
	}
}
