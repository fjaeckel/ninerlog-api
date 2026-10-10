package handlers

import (
	"encoding/csv"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

func TestExportPlace_NamesLocalIdents(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		"DE-0249": {Name: "Konz-Könen Glider Field", Latitude: 49.68, Longitude: 6.54},
		"EDHE":    {Name: "Uetersen/Heist Airfield", Latitude: 53.65, Longitude: 9.7},
	})
	defer airports.SetTestDB(nil)

	dep, arr := "DE-0249", "EDHE"
	flights := []*models.Flight{{ID: uuid.New(), Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		AircraftReg: "D-MABC", AircraftType: "C42", TotalTime: 60, DepartureICAO: &dep, ArrivalICAO: &arr}}
	prefs := exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "dot"}

	tests := []struct {
		name     string
		write    func(w *csv.Writer)
		dep, arr string
	}{
		{"standard", func(w *csv.Writer) { writeStandardCSV(w, flights, prefs) }, "From", "To"},
		{"easa", func(w *csv.Writer) { writeEASACSV(w, flights, prefs, "Pilot") }, "Dep Place", "Arr Place"},
		{"faa", func(w *csv.Writer) { writeFAACSV(w, flights, prefs) }, "From", "To"},
		{"weblogbook", func(w *csv.Writer) { writeWebLogbookCSV(w, flights, "Pilot") }, "Departure Place", "Arrival Place"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := renderCSV(t, tt.write)
			if got := column(t, records, records[1], tt.dep); got != "Konz-Könen Glider Field" {
				t.Errorf("%s = %q, want the airport name", tt.dep, got)
			}
			if got := column(t, records, records[1], tt.arr); got != "EDHE" {
				t.Errorf("%s = %q, want the ICAO code", tt.arr, got)
			}
		})
	}
}

func TestExportPlace_NameRoundTripsThroughImport(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		"DE-0249": {ICAO: "DE-0249", Name: "Konz-Könen Glider Field", Latitude: 49.68, Longitude: 6.54},
	})
	defer airports.SetTestDB(nil)

	dep := "DE-0249"
	if got := normalizeLocation(exportPlace(&dep)); got != "DE-0249" {
		t.Errorf("re-imported place = %q, want DE-0249", got)
	}
}

func TestExportPlace_UnknownOrFreeText(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		"DE-0249": {Name: "Konz-Könen Glider Field", Latitude: 49.68, Longitude: 6.54},
	})
	defer airports.SetTestDB(nil)

	for in, want := range map[string]string{
		"DE-0249":      "Konz-Könen Glider Field",
		"XX-9999":      "XX-9999",
		"EDDF":         "EDDF",
		"LF0723":       "LF0723",
		"Meadow strip": "Meadow strip",
	} {
		if got := exportPlace(&in); got != want {
			t.Errorf("exportPlace(%q) = %q, want %q", in, got, want)
		}
	}
	if got := exportPlace(nil); got != "" {
		t.Errorf("exportPlace(nil) = %q, want empty", got)
	}
}

func TestWrapText(t *testing.T) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 5)
	d := &pdfDoc{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}

	lines := d.wrapText("Altisurface Notre-Dame-des-Neiges-Abbaye", 16)
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want a wrap", lines)
	}
	joined := ""
	for i, l := range lines {
		if w := pdf.GetStringWidth(d.tr(l)); w > 16 {
			t.Errorf("line %d %q is %.1fmm wide, want <= 16", i, l, w)
		}
		if i > 0 && lines[i-1][len(lines[i-1])-1] != '-' && joined != "" {
			joined += " "
		}
		joined += l
	}
	if joined != "Altisurface Notre-Dame-des-Neiges-Abbaye" {
		t.Errorf("rejoined = %q, want every character kept", joined)
	}
	for _, l := range lines[1:] {
		if l[0] == '-' {
			t.Errorf("line %q starts with a hyphen, want breaks after it", l)
		}
	}
}
