package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
// best-effort template into a verified one. "real-header" sits between the two:
// the header row came out of the vendor's application, the values did not,
// because the file it came from could not be shared. See
// testdata/importsamples/README.md.

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
	// ExpectRowErrorFields marks a sample whose first row legitimately cannot
	// become a flight — the source recorded something FlightCreate requires and
	// the file does not carry. The listed fields are asserted exactly, so this
	// is a pinned behaviour rather than a known-failure escape hatch.
	ExpectRowErrorFields []string `json:"expectRowErrorFields"`
	// ExpectParseError marks a sample that must be rejected, and the substring
	// the pilot should see. An export of an empty logbook is a real upload, and
	// reporting it as malformed sends them hunting for a problem that is not
	// there. Detection is still asserted, against the header row alone.
	ExpectParseError string `json:"expectParseError"`
	ExpectFirstRow   struct {
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
			if sample.ExpectParseError != "" {
				if err == nil {
					t.Fatalf("sample parsed, manifest says it must be rejected with %q",
						sample.ExpectParseError)
				}
				if !strings.Contains(err.Error(), sample.ExpectParseError) {
					t.Errorf("rejected with %q, manifest says it must mention %q",
						err, sample.ExpectParseError)
				}
				// Detection reads the header row, so it is still assertable —
				// which is the whole value of a header-only sample.
				assertHeaderRowDetects(t, data, sample.ExpectTemplate)
				return
			}
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
			if len(sample.ExpectRowErrorFields) > 0 {
				gotFields := make([]string, 0, len(errs))
				for _, e := range errs {
					gotFields = append(gotFields, e.field)
				}
				sort.Strings(gotFields)
				want := append([]string(nil), sample.ExpectRowErrorFields...)
				sort.Strings(want)
				if strings.Join(gotFields, ",") != strings.Join(want, ",") {
					t.Errorf("row errors = %v, manifest says %v", gotFields, want)
				}
			} else if len(errs) > 0 {
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
			if len(sample.ExpectRowErrorFields) == 0 {
				for i, row := range rows {
					if _, errs := mapRowToFlight(row, toMappingLookup(mappings), nil); len(errs) > 0 {
						t.Errorf("row %d does not map cleanly: %+v", i+1, errs)
					}
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
		switch s.Provenance {
		case "generated", "synthetic", "real", "header-only", "real-header":
		default:
			t.Errorf("%s: provenance %q must be generated, synthetic, real, header-only or real-header",
				s.File, s.Provenance)
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
		// Both extensions the importer accepts. A LogTen tab export arrives as
		// .txt, and a sample the guard cannot see is a sample nobody is testing.
		lower := strings.ToLower(name)
		if e.IsDir() || (!strings.HasSuffix(lower, ".csv") && !strings.HasSuffix(lower, ".txt")) {
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

	// "real-header" counts here for the same reason "real" does: the wanted
	// list asks for a file whose column names came out of the vendor's own
	// application, and a real header row is exactly that. What it does not
	// settle is the value formats, which is why it stays a separate provenance.
	haveReal := make(map[string]bool)
	for _, s := range manifest.Samples {
		if s.Provenance == "real" || s.Provenance == "real-header" {
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

// Wader writes 00:00 into time fields the pilot never recorded. Taken
// literally, the real sample's on-block of 00:00 against an off-block of 11:03
// derives a 777-minute block time for a one-hour flight — and block times take
// precedence over a file's own total-time column, so the error would land in
// the logbook silently and inflate career totals.
func TestImportSample_WaderDropsPlaceholderMidnightTimes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(importSamplesDir, "wader.csv"))
	if err != nil {
		t.Fatal(err)
	}
	columns, rows, _, err := parseCSV(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tpl, _, mappings := detectImportFormat(columns)
	if tpl == nil || tpl.ID != "WADER_CSV" {
		t.Fatalf("detected %v, want WADER_CSV", tpl)
	}

	// The file really does carry the placeholders — if Wader ever stops writing
	// them this test is guarding nothing, so assert the input too.
	if rows[0]["parkingTime"] != "00:00" || rows[0]["takeoffTime"] != "00:00" {
		t.Fatalf("sample no longer carries 00:00 placeholders: parking=%q takeoff=%q",
			rows[0]["parkingTime"], rows[0]["takeoffTime"])
	}

	got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
	if len(errs) > 0 {
		t.Fatalf("row errors: %+v", errs)
	}

	if got.OffBlockTime != "11:03:00" {
		t.Errorf("offBlockTime = %q, want the recorded 11:03:00", got.OffBlockTime)
	}
	if got.OnBlockTime != "" {
		t.Errorf("onBlockTime = %q, want it dropped as a placeholder", got.OnBlockTime)
	}
	if got.DepartureTime != nil {
		t.Errorf("departureTime = %q, want it dropped as a placeholder", *got.DepartureTime)
	}
	if got.ArrivalTime == nil || *got.ArrivalTime != "12:04:00" {
		t.Errorf("arrivalTime = %v, want the recorded 12:04:00", got.ArrivalTime)
	}

	// The whole point: no 12-hour block time is derived from the placeholder.
	if mins := effectiveTotalMinutes(t, got); mins != 0 {
		t.Errorf("derived total = %d min, want 0 — a block time must not be "+
			"derived from a placeholder midnight", mins)
	}

	// pilotName1 is the SELF marker, not a person.
	if got.CrewMembers != nil {
		for _, cm := range *got.CrewMembers {
			t.Errorf("SELF became crew member %q (role %s)", cm.Name, cm.Role)
		}
	}
}

// assertHeaderRowDetects checks detection against a file's header row alone.
// A header-only export cannot go through the import pipeline, but its column
// names are exactly what detection keys off, so the most valuable half of the
// sample is still testable.
func assertHeaderRowDetects(t *testing.T, data []byte, wantTemplate string) {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		t.Fatalf("could not read the header row: %v", err)
	}
	tpl := importtemplate.Detect(records[0])
	if tpl == nil {
		t.Fatalf("header row is not detected as any template: %v", records[0])
	}
	if tpl.ID != wantTemplate {
		t.Errorf("header row detected as %s, manifest says %s", tpl.ID, wantTemplate)
	}
}

// capzlog.aero has no date column: the flight is dated by its off-block
// timestamp. It is also the third product in this catalogue to write a literal
// self-marker into a crew cell, after FLYLOG.io and Wader.
func TestImportSample_CapzlogDatedByOffBlockTimestamp(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(importSamplesDir, "capzlog.csv"))
	if err != nil {
		t.Fatal(err)
	}
	columns, rows, _, err := parseCSV(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tpl, _, mappings := detectImportFormat(columns)
	if tpl == nil || tpl.ID != "CAPZLOG_CSV" {
		t.Fatalf("detected %v, want CAPZLOG_CSV", tpl)
	}

	// Assert the input shape too: if capzlog ever adds a date column or stops
	// timestamping Off Block, this test is guarding nothing.
	for _, c := range columns {
		if strings.EqualFold(strings.TrimSpace(c), "date") {
			t.Fatalf("sample now has a Date column — the derivation is no longer what dates it")
		}
	}
	if !strings.Contains(rows[0]["Off Block"], "/") {
		t.Fatalf("Off Block %q no longer carries a date", rows[0]["Off Block"])
	}

	got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
	if len(errs) > 0 {
		t.Fatalf("row errors: %+v", errs)
	}
	if s := got.Date.String(); s != "2026-08-15" {
		t.Errorf("date = %q, want 2026-08-15 derived from the off-block timestamp", s)
	}
	// The clock half must still reach the block times, or the total is lost.
	if got.OffBlockTime != "04:00:00" || got.OnBlockTime != "06:07:00" {
		t.Errorf("block times = %q/%q, want 04:00:00/06:07:00", got.OffBlockTime, got.OnBlockTime)
	}
	if mins := effectiveTotalMinutes(t, got); mins != 127 {
		t.Errorf("total = %d min, want 127 (the Block column reads 2:07)", mins)
	}
	// "Self" is the logbook owner, not a crew member.
	if got.CrewMembers != nil {
		for _, cm := range *got.CrewMembers {
			t.Errorf("%q was imported as crew (role %s)", cm.Name, cm.Role)
		}
	}
}

// The populated SkyDemon export is the file that settled whether SkyDemon
// logbooks can be imported at all: its time columns carry full timestamps, so
// the flight can be dated even though the format has no date column.
func TestImportSample_SkyDemonIsDatedAndTimed(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(importSamplesDir, "skydemon.csv"))
	if err != nil {
		t.Fatal(err)
	}
	columns, rows, _, err := parseCSV(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tpl, _, mappings := detectImportFormat(columns)
	if tpl == nil || tpl.ID != "SKYDEMON_CSV" {
		t.Fatalf("detected %v, want SKYDEMON_CSV", tpl)
	}

	// Assert the input: if SkyDemon ever drops the date half of these values,
	// its logbooks stop being importable and this must fail loudly.
	if !strings.Contains(rows[0]["Departure Time"], "-") {
		t.Fatalf("Departure Time %q no longer carries a date — SkyDemon has no "+
			"date column, so its flights can no longer be dated",
			rows[0]["Departure Time"])
	}

	got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
	if len(errs) > 0 {
		t.Fatalf("row errors: %+v", errs)
	}
	if s := got.Date.String(); s != "2025-10-11" {
		t.Errorf("date = %q, want 2025-10-11 from the departure timestamp", s)
	}
	// "EDOI Bienenfarm" must reduce to the code.
	if got.DepartureIcao != "EDOI" || got.ArrivalIcao != "EDOI" {
		t.Errorf("airports = %q → %q, want EDOI → EDOI", got.DepartureIcao, got.ArrivalIcao)
	}
	// SkyDemon has no total column; the total comes from the block times and
	// must agree with its own PIC Time of 67 whole minutes.
	if mins := effectiveTotalMinutes(t, got); mins != 67 {
		t.Errorf("total = %d min, want 67 (PIC Time reads 67, i.e. whole minutes)", mins)
	}
	// SkyDemon exports "DEROQ"; the registration is canonicalised on import.
	if got.AircraftReg != "D-EROQ" {
		t.Errorf("aircraftReg = %q, want D-EROQ", got.AircraftReg)
	}
}
