package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
)

// Every sample in testdata/importsamples is run through the real import
// pipeline — the same parse, detect, and map steps an uploaded file goes
// through — and checked against what the manifest says it should produce.
//
// This is the only place a template is tested against a whole file rather than
// a header row it was written from. A header-row test can only confirm that the
// alias table agrees with itself; a sample file catches the things that
// actually break imports: a metadata preamble before the header, a delimiter we
// did not expect, a date convention that differs from the one the template
// declares, a duration in a unit the parser reads as something else.
//
// The manifest's provenance field says how much each sample is worth.
// "generated" samples come from this repository's own export code and are
// authoritative. "synthetic" samples were written from a vendor's documented
// column list and prove only that the template matches what we believe the
// format to be — replacing one with a real anonymised export is what turns a
// best-effort template into a verified one. See testdata/importsamples/README.md.

const importSamplesDir = "testdata/importsamples"

type importSampleManifest struct {
	Samples []importSample `json:"samples"`
	Wanted  []struct {
		Template string `json:"template"`
		Why      string `json:"why"`
	} `json:"wanted"`
}

type importSample struct {
	File           string `json:"file"`
	ExpectTemplate string `json:"expectTemplate"`
	Provenance     string `json:"provenance"`
	Notes          string `json:"notes"`
	ExpectRows     int    `json:"expectRows"`
	ExpectFirstRow struct {
		Date             string `json:"date"`
		AircraftReg      string `json:"aircraftReg"`
		AircraftType     string `json:"aircraftType"`
		DepartureIcao    string `json:"departureIcao"`
		ArrivalIcao      string `json:"arrivalIcao"`
		TotalTimeMinutes int    `json:"totalTimeMinutes"`
		Landings         int    `json:"landings"`
	} `json:"expectFirstRow"`
}

func loadImportSampleManifest(t *testing.T) importSampleManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(importSamplesDir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading sample manifest: %v", err)
	}
	var m importSampleManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parsing sample manifest: %v", err)
	}
	if len(m.Samples) == 0 {
		t.Fatal("sample manifest lists no samples")
	}
	return m
}

func TestImportSamples(t *testing.T) {
	manifest := loadImportSampleManifest(t)

	for _, sample := range manifest.Samples {
		t.Run(sample.File, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(importSamplesDir, sample.File))
			if err != nil {
				t.Fatalf("sample listed in the manifest is missing: %v", err)
			}

			columns, rows, aircraft, err := parseCSV(data)
			if err != nil {
				t.Fatalf("sample does not parse: %v", err)
			}
			if sample.ExpectRows > 0 && len(rows) != sample.ExpectRows {
				t.Errorf("parsed %d rows, manifest says %d", len(rows), sample.ExpectRows)
			}

			tpl, format, mappings := detectImportFormat(columns)
			if tpl == nil {
				t.Fatalf("not detected as any template (recorded as %s)\ncolumns: %v", format, columns)
			}
			if tpl.ID != sample.ExpectTemplate {
				t.Fatalf("detected as %s, manifest says %s", tpl.ID, sample.ExpectTemplate)
			}

			got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
			if len(errs) > 0 {
				t.Fatalf("first row does not map cleanly: %+v\nrow: %v", errs, rows[0])
			}

			want := sample.ExpectFirstRow
			if want.Date != "" && got.Date.String() != want.Date {
				t.Errorf("date = %q, want %q", got.Date.String(), want.Date)
			}
			if want.AircraftReg != "" && got.AircraftReg != want.AircraftReg {
				t.Errorf("aircraftReg = %q, want %q", got.AircraftReg, want.AircraftReg)
			}
			if want.AircraftType != "" && got.AircraftType != want.AircraftType {
				t.Errorf("aircraftType = %q, want %q", got.AircraftType, want.AircraftType)
			}
			if want.DepartureIcao != "" && got.DepartureIcao != want.DepartureIcao {
				t.Errorf("departureIcao = %q, want %q", got.DepartureIcao, want.DepartureIcao)
			}
			if want.ArrivalIcao != "" && got.ArrivalIcao != want.ArrivalIcao {
				t.Errorf("arrivalIcao = %q, want %q", got.ArrivalIcao, want.ArrivalIcao)
			}
			if want.TotalTimeMinutes > 0 {
				// Layouts carrying block times derive the total from them;
				// layouts with only a decimal cell round to 0.1h (6 min).
				if diff := abs(effectiveTotalMinutes(t, got) - want.TotalTimeMinutes); diff > 3 {
					t.Errorf("totalTime = %d min, want %d ±3",
						effectiveTotalMinutes(t, got), want.TotalTimeMinutes)
				}
			}
			if want.Landings > 0 && got.Landings != want.Landings {
				t.Errorf("landings = %d, want %d", got.Landings, want.Landings)
			}

			// Every row must map, not just the first — a file that imports its
			// header row and then fails on row 40 is the worst outcome.
			for i, row := range rows {
				if _, errs := mapRowToFlight(row, toMappingLookup(mappings), nil); len(errs) > 0 {
					t.Errorf("row %d does not map cleanly: %+v", i+1, errs)
				}
			}

			if strings.HasPrefix(sample.File, "foreflight") && len(aircraft) == 0 {
				t.Error("ForeFlight sample's Aircraft Table was not parsed")
			}
		})
	}
}

// A sample file that nobody registered proves nothing, and a manifest entry
// with no file is a silently skipped test. Neither is allowed.
func TestImportSamplesAreAllRegistered(t *testing.T) {
	manifest := loadImportSampleManifest(t)

	registered := make(map[string]bool, len(manifest.Samples))
	for _, s := range manifest.Samples {
		registered[s.File] = true
		if s.Provenance != "generated" && s.Provenance != "synthetic" && s.Provenance != "real" {
			t.Errorf("%s: provenance %q must be generated, synthetic or real", s.File, s.Provenance)
		}
		if s.ExpectTemplate == "" {
			t.Errorf("%s: no expectTemplate", s.File)
		}
	}

	entries, err := os.ReadDir(importSamplesDir)
	if err != nil {
		t.Fatalf("reading sample directory: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".csv") {
			continue
		}
		if !registered[name] {
			t.Errorf("%s is not in manifest.json — add an entry saying which template "+
				"it must be detected as and what its first row should produce", name)
		}
	}
}

// The wanted list is the standing request for real exports. It must stay in
// step with the catalogue: a template nobody has verified should be asking for
// a sample, and a template with a real sample should have stopped asking.
func TestImportSampleWantedListMatchesUnverifiedTemplates(t *testing.T) {
	manifest := loadImportSampleManifest(t)

	haveReal := make(map[string]bool)
	for _, s := range manifest.Samples {
		if s.Provenance == "real" {
			haveReal[s.ExpectTemplate] = true
		}
	}
	wanted := make(map[string]bool, len(manifest.Wanted))
	for _, w := range manifest.Wanted {
		wanted[w.Template] = true
		if w.Why == "" {
			t.Errorf("wanted entry %s does not say why", w.Template)
		}
		if haveReal[w.Template] {
			t.Errorf("%s has a real sample but is still on the wanted list", w.Template)
		}
	}

	for _, tpl := range importTemplatesNeedingRealSamples() {
		if !wanted[tpl] && !haveReal[tpl] {
			t.Errorf("%s is a best-effort template with no real sample, but is not on "+
				"the wanted list in manifest.json", tpl)
		}
	}
}

// importTemplatesNeedingRealSamples is every template whose column list was
// inferred rather than verified. The generic CSV entry is excluded: it is a
// fallback alias table, not a format anyone can export.
func importTemplatesNeedingRealSamples() []string {
	var out []string
	for _, tpl := range importtemplate.All() {
		if tpl.ID == importtemplate.FormatGenericCSV {
			continue
		}
		if tpl.Confidence == importtemplate.ConfidenceBestEffort {
			out = append(out, tpl.ID)
		}
	}
	return out
}

// FLYLOG.io records the logbook's owner as the literal string SELF in whichever
// NAME_* column the pilot occupied. Taken at face value it produces a crew
// member — and then a contact — called "SELF", linked to every such flight.
//
// The real sample is a training flight: the owner is the student (SELF) and the
// instructor is a named third party who is also PIC of record. Exactly one crew
// member must come out of it.
func TestImportSample_FlylogDropsSelfCrewSentinel(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(importSamplesDir, "flylog.csv"))
	if err != nil {
		t.Fatal(err)
	}
	columns, rows, _, err := parseCSV(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tpl, _, mappings := detectImportFormat(columns)
	if tpl == nil || tpl.ID != "FLYLOG_CSV" {
		t.Fatalf("detected %v, want FLYLOG_CSV", tpl)
	}

	got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
	if len(errs) > 0 {
		t.Fatalf("row errors: %+v", errs)
	}

	if got.CrewMembers == nil {
		t.Fatal("expected the named instructor as a crew member")
	}
	for _, cm := range *got.CrewMembers {
		if isSelfCrewSentinel(cm.Name) {
			t.Errorf("SELF was imported as a crew member (role %s) — it denotes "+
				"the logbook owner and would become a contact", cm.Role)
		}
	}
	if len(*got.CrewMembers) != 1 {
		t.Fatalf("got %d crew members, want 1: %+v", len(*got.CrewMembers), *got.CrewMembers)
	}
	crew := (*got.CrewMembers)[0]
	if crew.Name != "Alex Rivera" || string(crew.Role) != "Instructor" {
		t.Errorf("crew = %q/%s, want Alex Rivera/Instructor", crew.Name, crew.Role)
	}
	if got.InstructorName == nil || *got.InstructorName != "Alex Rivera" {
		t.Errorf("instructorName = %v, want Alex Rivera", got.InstructorName)
	}
}

func TestIsSelfCrewSentinel(t *testing.T) {
	for _, in := range []string{"SELF", "self", " Self ", "sELF"} {
		if !isSelfCrewSentinel(in) {
			t.Errorf("isSelfCrewSentinel(%q) = false, want true", in)
		}
	}
	// A real name that merely contains the word must survive: dropping a crew
	// member is silent data loss, which is worse than one odd contact.
	for _, in := range []string{"", "Self Loading Freight", "Selfridge", "Me", "S. Elf"} {
		if isSelfCrewSentinel(in) {
			t.Errorf("isSelfCrewSentinel(%q) = true, want false", in)
		}
	}
}
