package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
)

// A CSV this application exports must always be a CSV this application can
// import. It is the one round trip we control both ends of, so it is the one
// that must never regress — a pilot moving between installations, restoring
// from an archived export, or splitting one logbook into two accounts is
// relying on it, and unlike a third-party format there is nobody else to blame
// when it breaks.
//
// The guarantee is exercised over the full export surface: every column layout
// crossed with every date-format and decimal-separator preference.
//
// Only the standard layout currently honours those preferences — the EASA
// layout hardcodes DD.MM.YYYY dates and H:MM durations, and the FAA layout
// hardcodes MM/DD/YYYY, because in both cases the convention is part of the
// regulatory format rather than a user choice (export.go:326, export.go:362).
// The matrix is still run in full against all three: the redundant EASA and FAA
// combinations cost nothing, and they mean that if a preference is ever wired
// into those layouts the round trip is already covered instead of quietly
// becoming untested.
//
// This is the fast, no-Docker half of the guarantee. The e2e suite asserts the
// same invariant through real HTTP against a real database
// (test/e2e/export_import_roundtrip_e2e_test.go).

// roundTripSourceFlight is the flight every round trip starts from. It
// deliberately exercises the fields that are easy to lose: block times, an
// explicit day/night landing split, instrument time, an instructor, and a
// registration/type pair.
func roundTripSourceFlight() *models.Flight {
	dep, arr := "EDDF", "EDDM"
	off, on := "08:15:00", "09:45:00"
	takeoff, landing := "08:22:00", "09:38:00"
	remarks := "Round trip check"
	instructor := "Sam Carter"
	return &models.Flight{
		Date:                    time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
		AircraftReg:             "D-EABC",
		AircraftType:            "C172",
		DepartureICAO:           &dep,
		ArrivalICAO:             &arr,
		OffBlockTime:            &off,
		OnBlockTime:             &on,
		DepartureTime:           &takeoff,
		ArrivalTime:             &landing,
		TotalTime:               90,
		PICTime:                 90,
		IsPIC:                   true,
		LandingsDay:             2,
		LandingsNight:           1,
		AllLandings:             3,
		NightTime:               0,
		IFRTime:                 30,
		ActualInstrumentTime:    18,
		SimulatedInstrumentTime: 12,
		Holds:                   1,
		ApproachesCount:         2,
		Remarks:                 &remarks,
		InstructorName:          &instructor,
	}
}

// exportCSV runs the real export writer for a layout and returns the bytes a
// user would download.
func exportCSV(t *testing.T, layout string, flights []*models.Flight, prefs exportPrefs) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	switch layout {
	case "standard":
		writeStandardCSV(w, flights, prefs)
	case "easa":
		writeEASACSV(w, flights, prefs, "Alex Rivera")
	case "faa":
		writeFAACSV(w, flights, prefs)
	default:
		t.Fatalf("unknown export layout %q", layout)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("export writer failed: %v", err)
	}
	return buf.Bytes()
}

func TestExportImportRoundTrip(t *testing.T) {
	// Layouts and their total-time tolerance in minutes.
	//
	// The standard and EASA layouts write block times, and the importer derives
	// the total from those, so the total survives exactly. The FAA layout has no
	// time-of-day columns at all: its total is the decimal-hours cell, which
	// FormatDecimal rounds to one decimal place (0.1h = 6 min), so the worst
	// case is half a step.
	//
	// wantTemplate pins which template each layout must be recognised as. A
	// layout drifting onto another template still "imports", but silently under
	// a different column map — so the identity is part of the contract, not an
	// implementation detail.
	layouts := []struct {
		name               string
		wantTemplate       string
		totalTimeTolerance int
	}{
		{"standard", "NINERLOG_CSV", 0},
		{"easa", "EASA_CSV", 0},
		{"faa", "FAA_CSV", 3},
	}
	dateFormats := []string{"DD.MM.YYYY", "MM/DD/YYYY", "YYYY-MM-DD"}
	separators := []string{"dot", "comma"}

	src := roundTripSourceFlight()

	for _, layout := range layouts {
		for _, df := range dateFormats {
			for _, sep := range separators {
				name := fmt.Sprintf("%s/%s/%s", layout.name, df, sep)
				t.Run(name, func(t *testing.T) {
					prefs := exportPrefs{DateFormat: df, DecimalSeparator: sep}
					data := exportCSV(t, layout.name, []*models.Flight{src}, prefs)

					columns, rows, _, err := parseCSV(data)
					if err != nil {
						t.Fatalf("exported CSV does not parse: %v\n%s", err, data)
					}
					if len(rows) != 1 {
						t.Fatalf("parsed %d rows, want 1\n%s", len(rows), data)
					}

					tpl, format, mappings := detectImportFormat(columns)
					if tpl == nil {
						t.Fatalf("our own %s export was not recognised (format=%s)\ncolumns: %v",
							layout.name, format, columns)
					}
					if tpl.ID != layout.wantTemplate {
						t.Errorf("%s export detected as %s, want %s",
							layout.name, tpl.ID, layout.wantTemplate)
					}
					if string(format) != layout.wantTemplate {
						t.Errorf("recorded format = %s, want %s", format, layout.wantTemplate)
					}

					mappingLookup := toMappingLookup(mappings)
					got, errs := mapRowToFlight(rows[0], mappingLookup, nil)
					if len(errs) > 0 {
						t.Fatalf("re-importing our own %s export produced errors: %+v\nrow: %v",
							layout.name, errs, rows[0])
					}

					// Date must survive every date-format preference.
					if want := src.Date.Format("2006-01-02"); got.Date.String() != want {
						t.Errorf("date = %q, want %q (detected %s)", got.Date.String(), want, tpl.ID)
					}
					if safeStr(got.AircraftReg) != src.AircraftReg {
						t.Errorf("aircraftReg = %q, want %q", safeStr(got.AircraftReg), src.AircraftReg)
					}
					if safeStr(got.DepartureIcao) != *src.DepartureICAO {
						t.Errorf("departureIcao = %q, want %q", safeStr(got.DepartureIcao), *src.DepartureICAO)
					}
					if safeStr(got.ArrivalIcao) != *src.ArrivalICAO {
						t.Errorf("arrivalIcao = %q, want %q", safeStr(got.ArrivalIcao), *src.ArrivalICAO)
					}

					// Total time: from block times where the layout carries
					// them, else the rounded decimal cell.
					gotTotal := effectiveTotalMinutes(t, got)
					if diff := abs(gotTotal - src.TotalTime); diff > layout.totalTimeTolerance {
						t.Errorf("totalTime = %d min, want %d ±%d",
							gotTotal, src.TotalTime, layout.totalTimeTolerance)
					}

					// Landings must not be lost or double-counted.
					if getIntOrDefault(got.Landings, 0) != src.AllLandings {
						t.Errorf("landings = %d, want %d", getIntOrDefault(got.Landings, 0), src.AllLandings)
					}

					if got.Remarks == nil || !strings.Contains(*got.Remarks, "Round trip check") {
						t.Errorf("remarks = %v, want them to survive", got.Remarks)
					}
				})
			}
		}
	}
}

// The aircraft type is only carried by the layouts that have a column for it.
// Where it is present it must survive, because losing it silently creates a
// fleet entry whose type is the registration.
func TestExportImportRoundTrip_AircraftType(t *testing.T) {
	src := roundTripSourceFlight()
	prefs := exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "dot"}

	for _, layout := range []string{"standard", "easa", "faa"} {
		t.Run(layout, func(t *testing.T) {
			data := exportCSV(t, layout, []*models.Flight{src}, prefs)
			columns, rows, _, err := parseCSV(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, _, mappings := detectImportFormat(columns)
			got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
			if len(errs) > 0 {
				t.Fatalf("errors: %+v", errs)
			}
			if got.AircraftType != src.AircraftType {
				t.Errorf("aircraftType = %q, want %q", got.AircraftType, src.AircraftType)
			}
		})
	}
}

// An empty logbook must still export a file that parses as one of our formats,
// rather than something the importer rejects outright.
func TestExportImportRoundTrip_HeaderOnlyExportIsRecognised(t *testing.T) {
	prefs := exportPrefs{DateFormat: "DD.MM.YYYY", DecimalSeparator: "dot"}
	for _, layout := range []string{"standard", "easa", "faa"} {
		t.Run(layout, func(t *testing.T) {
			data := exportCSV(t, layout, nil, prefs)
			r := csv.NewReader(bytes.NewReader(data))
			records, err := r.ReadAll()
			if err != nil || len(records) == 0 {
				t.Fatalf("empty export unreadable: %v", err)
			}
			tpl := importtemplate.Detect(records[0])
			if tpl == nil {
				t.Errorf("empty %s export header row is not recognised: %v", layout, records[0])
			}
		})
	}
}

// --- helpers ---

func toMappingLookup(mappings []generated.ImportColumnMapping) map[string]generated.ImportColumnMapping {
	out := make(map[string]generated.ImportColumnMapping, len(mappings))
	for _, m := range mappings {
		out[m.SourceColumn] = m
	}
	return out
}

// effectiveTotalMinutes mirrors what ConfirmImport does when deciding a
// flight's total: block times win over the explicit total cell.
func effectiveTotalMinutes(t *testing.T, f generated.FlightCreate) int {
	t.Helper()
	if safeStr(f.OffBlockTime) != "" && safeStr(f.OnBlockTime) != "" {
		if mins, err := calculateBlockTime(safeStr(f.OffBlockTime), safeStr(f.OnBlockTime)); err == nil {
			return mins
		}
	}
	if f.TotalTime != nil {
		return *f.TotalTime
	}
	return 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
